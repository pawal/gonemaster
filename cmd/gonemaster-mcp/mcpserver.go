package main

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// newMCPServer builds the server and registers tools. Write tools register only
// when allowWrite is set. SDK use is confined to this file so a later SDK swap
// stays contained.
func newMCPServer(api *apiClient, allowWrite bool) *mcp.Server {
	opts := &mcp.ServerOptions{Instructions: serverInstructions(allowWrite)}
	srv := mcp.NewServer(&mcp.Implementation{Name: serverName, Version: version}, opts)
	registerPing(srv, api)
	registerReadTools(srv, api)
	registerSpecTools(srv, api)
	registerHistoryTools(srv, api)
	registerBatchTools(srv, api)
	if allowWrite {
		registerWriteTools(srv, api)
	}
	return srv
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

Tags are stable identifiers; messages are human-readable renderings in the requested language (default en). ping reports connectivity and auth status.`
	if allowWrite {
		s += `

Write tools (enabled on this server)
  - batch_enqueue starts a cohort test from an explicit domain list or an existing tag, returning a batch id to poll with batch_get.
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

// registerPing adds a connectivity + auth smoke-test tool.
func registerPing(srv *mcp.Server, api *apiClient) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "ping",
		Description: "Check connectivity to gonemaster-server and report its auth mode and whether this bridge is authenticated.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ pingInput) (*mcp.CallToolResult, pingOutput, error) {
		out := pingOutput{ServerURL: api.baseURL}
		who, err := api.whoami(ctx)
		if err != nil {
			out.Detail = "gonemaster-server unreachable: " + err.Error()
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
