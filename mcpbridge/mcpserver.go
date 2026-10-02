package mcpbridge

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Options configures NewServer.
type Options struct {
	// AllowWrite registers batch_enqueue, batch_cancel and cancel_job.
	AllowWrite bool
	// Name and Version identify the server to the client; defaults gonemaster-mcp and dev.
	Name    string
	Version string
	// Logger receives one line per tool call; nil logs nothing.
	Logger *slog.Logger
	// OnToolCall observes each tool call, for metrics.
	OnToolCall func(tool string, d time.Duration, failed bool)
}

// NewServer registers the tools against api; write tools only with AllowWrite.
func NewServer(api *Client, opts Options) *mcp.Server {
	if opts.Name == "" {
		opts.Name = "gonemaster-mcp"
	}
	if opts.Version == "" {
		opts.Version = "dev"
	}
	srv := mcp.NewServer(&mcp.Implementation{Name: opts.Name, Version: opts.Version},
		&mcp.ServerOptions{Instructions: serverInstructions(opts.AllowWrite)})
	srv.AddReceivingMiddleware(callObserver(opts))
	registerPing(srv, api)
	registerReadTools(srv, api)
	registerSpecTools(srv, api)
	registerDiscoveryTools(srv, api)
	registerHistoryTools(srv, api)
	registerBatchTools(srv, api)
	if opts.AllowWrite {
		registerWriteTools(srv, api)
	}
	return srv
}

// callObserver logs each tools/call and reports it to OnToolCall.
func callObserver(opts Options) mcp.Middleware {
	return func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			if method != "tools/call" {
				return next(ctx, method, req)
			}
			name := "?"
			if p, ok := req.GetParams().(*mcp.CallToolParamsRaw); ok && p != nil {
				name = p.Name
			}
			start := time.Now()
			res, err := next(ctx, method, req)
			failed := err != nil
			if r, ok := res.(*mcp.CallToolResult); ok && r != nil && r.IsError {
				failed = true
			}
			d := time.Since(start)
			if opts.Logger != nil {
				opts.Logger.Info("mcp tool call", "tool", name, "duration_ms", d.Milliseconds(), "failed", failed)
			}
			if opts.OnToolCall != nil {
				opts.OnToolCall(name, d, failed)
			}
			return res, err
		}
	}
}

// serverInstructions is the connect-time blurb shown to the MCP client.
func serverInstructions(allowWrite bool) string {
	s := `gonemaster runs DNS and delegation health tests for domains and exposes the results for analysis.

A test checks a domain's nameservers, delegation, DNSSEC, zone consistency, addresses, and SOA/timing. It produces:
  - findings: tagged log messages, each at a severity from DEBUG to CRITICAL (WARNING and above are issues),
  - a letter grade (A+ to F) and numeric score (0-100), and
  - per-nameserver query response-time statistics in milliseconds.

Testing
  - test_domain submits one domain and waits for the verdict (grade, score, findings, timings).
  - latest_for / run_get fetch recent or specific stored runs by domain or run id.
  - Findings include every stored level by default; pass min_level (e.g. NOTICE) to keep only issues. level_counts always shows the full distribution.

Analysing a single domain over time
  - run_search finds completed runs by domain, tag, status, severity, grade, or finish-time range.
  - run_diff compares two runs at the tag level (which findings appeared, cleared, or changed severity), e.g. before vs after a fix.

Cohort analysis (a batch is one test run over many domains)
  - batch_list / batch_get discover batches and poll their completion.
  - cohort_stats gives the grade and worst-severity distribution across the batch.
  - failures_by_tag ranks the message tags driving failures, with example domains.
  - cohort_tag_values rolls up the values a given (tag, arg) pair takes across the batch (e.g. which NSID strings or AS numbers are in use, and which domains carry each), optionally weighted by domain score.

Reference
  - spec_list_testcases / spec_get_testcase describe the implemented testcases and the message tags each can emit. Use them to learn valid tag and arg names for run_search and cohort_tag_values.
  - profile_list names the test profiles; pass the id as test_domain's profile_id.
  - domain_tag_list names the user domain tags: groups of domains that batches are built from. They are unrelated to finding tags.
  - cohort_list names the analysis cohorts and their snapshots, whose slugs feed cohort_report.

Tags are stable identifiers; messages are human-readable renderings in the requested language (default en). ping reports connectivity and auth status.`
	if allowWrite {
		s += `

Write tools (enabled on this server)
  - batch_enqueue starts a cohort test from an explicit domain list or an existing tag, returning a batch id to poll with batch_get. Its profile is a profile_list name and its from_tag a domain_tag_list name.
  - batch_cancel and cancel_job stop a batch or a single job.`
	}
	return s
}

type pingInput struct{}

type pingOutput struct {
	ServerURL     string `json:"server_url" jsonschema:"the gonemaster-server admin API URL this bridge targets"`
	Reachable     bool   `json:"reachable" jsonschema:"whether the gonemaster-server responded"`
	AuthMode      string `json:"auth_mode,omitempty" jsonschema:"server auth mode: open or token"`
	Authenticated bool   `json:"authenticated" jsonschema:"whether this bridge's credential is accepted (always true in open mode)"`
	Detail        string `json:"detail,omitempty" jsonschema:"human-readable status or error detail"`
}

// readOnly marks a tool that only reads stored data.
func readOnly(title string) *mcp.ToolAnnotations {
	closed := false
	return &mcp.ToolAnnotations{Title: title, ReadOnlyHint: true, OpenWorldHint: &closed}
}

// writeHint marks a tool that changes server state.
func writeHint(title string, destructive, openWorld bool) *mcp.ToolAnnotations {
	return &mcp.ToolAnnotations{Title: title, DestructiveHint: &destructive, OpenWorldHint: &openWorld}
}

// registerPing adds a connectivity + auth smoke-test tool.
func registerPing(srv *mcp.Server, api *Client) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "ping",
		Description: "Check connectivity to gonemaster-server and report its auth mode and whether this bridge is authenticated.",
		Annotations: readOnly("Ping gonemaster-server"),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ pingInput) (*mcp.CallToolResult, pingOutput, error) {
		out := pingOutput{ServerURL: api.baseURL}
		who, err := api.whoami(ctx)
		if err != nil {
			out.Detail = api.pingFailure(err)
			return nil, out, nil
		}
		out.Reachable = true
		out.AuthMode = who.Mode
		out.Authenticated = who.Authenticated
		if who.Mode == "token" && !who.Authenticated {
			out.Detail = "server is in token mode but no valid GONEMASTER_TOKEN was provided"
		} else {
			out.Detail = "ok"
		}
		return nil, out, nil
	})
}

// pingFailure separates a dead address from a URL that is not the admin API.
func (c *Client) pingFailure(err error) string {
	var he *httpError
	if errors.As(err, &he) {
		return fmt.Sprintf("%s/whoami answered http %d: not a gonemaster-server admin API, %s", c.baseURL, he.status, c.urlHint)
	}
	return "gonemaster-server unreachable: " + err.Error()
}
