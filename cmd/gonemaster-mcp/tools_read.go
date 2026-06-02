package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var terminalStatuses = map[string]bool{
	"succeeded": true,
	"failed":    true,
	"canceled":  true,
	"expired":   true,
}

// Overridable in tests.
var (
	defaultTestTimeout = 120 * time.Second
	pollInterval       = 1 * time.Second
)

const maxTestTimeout = 600 * time.Second

type finding struct {
	Tag      string `json:"tag" jsonschema:"the testcase message tag, e.g. SOATIME"`
	Level    string `json:"level" jsonschema:"severity: DEBUG, INFO, NOTICE, WARNING, ERROR, or CRITICAL"`
	Module   string `json:"module" jsonschema:"the testcase module that produced the finding"`
	Testcase string `json:"testcase,omitempty" jsonschema:"the testcase identifier"`
	Message  string `json:"message,omitempty" jsonschema:"the rendered, human-readable message"`
}

type testResult struct {
	Domain            string     `json:"domain"`
	RunID             string     `json:"run_id,omitempty" jsonschema:"the run/job id; pass to run_get"`
	BatchID           string     `json:"batch_id,omitempty" jsonschema:"the batch this run belongs to; empty for ad-hoc single-domain runs"`
	Status            string     `json:"status" jsonschema:"succeeded, failed, canceled, or expired"`
	Grade             string     `json:"grade,omitempty" jsonschema:"letter grade A+ to F when scoring is available"`
	Score             *int       `json:"score,omitempty" jsonschema:"numeric score 0-100 when scoring is available"`
	DurationMs        int64      `json:"duration_ms,omitempty"`
	Findings          []finding  `json:"findings" jsonschema:"all log entries; filter by level for issues"`
	NameserverTimings []nsTiming `json:"nameserver_timings,omitempty" jsonschema:"per-nameserver query response-time statistics, in milliseconds"`
	Error             string     `json:"error,omitempty" jsonschema:"failure reason when status is not succeeded"`
}

type nsTiming struct {
	Nameserver string  `json:"nameserver"`
	Address    string  `json:"address,omitempty"`
	MedianMS   float64 `json:"median_ms"`
	AvgMS      float64 `json:"avg_ms"`
	MinMS      float64 `json:"min_ms,omitempty"`
	MaxMS      float64 `json:"max_ms,omitempty"`
	Count      int     `json:"count,omitempty" jsonschema:"number of timed queries"`
	Status     string  `json:"status,omitempty" jsonschema:"ok, unreachable, or unresolved"`
}

func registerReadTools(srv *mcp.Server, api *apiClient) {
	registerTestDomain(srv, api)
	registerRunGet(srv, api)
	registerLatestFor(srv, api)
}

type testDomainInput struct {
	Domain         string `json:"domain" jsonschema:"the domain name to test, e.g. example.com"`
	ProfileID      int64  `json:"profile_id,omitempty" jsonschema:"optional test profile id; omit for the default profile"`
	TimeoutSeconds int    `json:"timeout_seconds,omitempty" jsonschema:"max seconds to wait for the test (default 120, max 600)"`
}

func registerTestDomain(srv *mcp.Server, api *apiClient) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "test_domain",
		Description: "Run a DNS test for a domain and wait for the result. Returns grade, score, findings, and per-nameserver response times.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in testDomainInput) (*mcp.CallToolResult, testResult, error) {
		domain := strings.TrimSpace(in.Domain)
		if domain == "" {
			return nil, testResult{}, errors.New("domain is required")
		}
		timeout := defaultTestTimeout
		if in.TimeoutSeconds > 0 {
			timeout = time.Duration(in.TimeoutSeconds) * time.Second
			if timeout > maxTestTimeout {
				timeout = maxTestTimeout
			}
		}

		req := createJobRequest{Domain: domain}
		if in.ProfileID > 0 {
			req.ProfileID = &in.ProfileID
		}
		job, err := api.createJob(ctx, req)
		if err != nil {
			return nil, testResult{}, toolError("submit test", err)
		}

		pollCtx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		for !terminalStatuses[job.Status] {
			select {
			case <-pollCtx.Done():
				return nil, testResult{}, pollTimeout(timeout, domain, job)
			case <-time.After(pollInterval):
			}
			next, err := api.getJob(pollCtx, job.ID)
			if err != nil {
				// The deadline can fire during the request, not just at the
				// select; report it as a timeout, not a generic poll error.
				if pollCtx.Err() != nil {
					return nil, testResult{}, pollTimeout(timeout, domain, job)
				}
				return nil, testResult{}, toolError("poll job", err)
			}
			job = next
		}

		out := testResult{Domain: domain, RunID: job.ID, Status: job.Status, DurationMs: durationMs(job), Error: job.Error, Findings: []finding{}}
		fillResult(ctx, api, job.ID, "en", &out)
		return nil, out, nil
	})
}

type runGetInput struct {
	ID   string `json:"id" jsonschema:"the run or job id"`
	Lang string `json:"lang,omitempty" jsonschema:"language for rendered messages (default en)"`
}

func registerRunGet(srv *mcp.Server, api *apiClient) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "run_get",
		Description: "Fetch a stored run's result by id: grade, score, findings, and per-nameserver response times.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in runGetInput) (*mcp.CallToolResult, testResult, error) {
		id := strings.TrimSpace(in.ID)
		if id == "" {
			return nil, testResult{}, errors.New("id is required")
		}
		lang := strings.TrimSpace(in.Lang)
		if lang == "" {
			lang = "en"
		}
		run, err := api.getRun(ctx, id)
		if err != nil {
			return nil, testResult{}, toolError("get run", err)
		}
		out := testResult{Domain: run.Domain, RunID: run.ID, BatchID: run.BatchID, Status: run.Status, DurationMs: run.DurationMs, Error: run.Error, Findings: []finding{}}
		fillResult(ctx, api, id, lang, &out)
		return nil, out, nil
	})
}

type latestForInput struct {
	Domain string `json:"domain" jsonschema:"the domain name"`
	Limit  int    `json:"limit,omitempty" jsonschema:"max runs to return (default 5)"`
}

type runSummary struct {
	RunID      string `json:"run_id"`
	Domain     string `json:"domain"`
	BatchID    string `json:"batch_id,omitempty"`
	Status     string `json:"status"`
	Grade      string `json:"grade,omitempty"`
	Score      *int   `json:"score,omitempty"`
	WorstLevel string `json:"worst_level,omitempty"`
	DurationMs int64  `json:"duration_ms,omitempty"`
	FinishedAt string `json:"finished_at,omitempty"`
}

type latestForOutput struct {
	Domain string       `json:"domain"`
	Count  int          `json:"count"`
	Runs   []runSummary `json:"runs" jsonschema:"completed runs, most recent first"`
}

func registerLatestFor(srv *mcp.Server, api *apiClient) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "latest_for",
		Description: "List the most recent completed runs for a domain, newest first.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in latestForInput) (*mcp.CallToolResult, latestForOutput, error) {
		domain := strings.TrimSpace(in.Domain)
		if domain == "" {
			return nil, latestForOutput{}, errors.New("domain is required")
		}
		limit := in.Limit
		if limit <= 0 {
			limit = 5
		}
		list, err := api.listRuns(ctx, domain, limit)
		if err != nil {
			return nil, latestForOutput{}, toolError("list runs", err)
		}
		out := latestForOutput{Domain: domain, Runs: []runSummary{}}
		for _, r := range list.Items {
			out.Runs = append(out.Runs, toRunSummary(r))
		}
		out.Count = len(out.Runs)
		return nil, out, nil
	})
}

// toRunSummary maps a run record to the LLM-facing summary.
func toRunSummary(r runView) runSummary {
	rs := runSummary{RunID: r.ID, Domain: r.Domain, BatchID: r.BatchID, Status: r.Status, Score: r.Score, WorstLevel: r.WorstLevel, DurationMs: r.DurationMs}
	if r.Grade != nil {
		rs.Grade = *r.Grade
	}
	if !r.FinishedAt.IsZero() {
		rs.FinishedAt = r.FinishedAt.UTC().Format(time.RFC3339)
	}
	return rs
}

// fillResult fetches a run's result and fills grade, score, and findings into
// out. A missing result body (e.g. a job that failed before producing entries)
// is not fatal: out keeps its status and error.
func fillResult(ctx context.Context, api *apiClient, id, lang string, out *testResult) {
	res, err := api.getResult(ctx, id, lang)
	if err != nil {
		return
	}
	if res.Score != nil {
		out.Grade = res.Score.Grade
		score := res.Score.Score
		out.Score = &score
	}
	if res.Raw != nil {
		for _, e := range res.Raw.Entries {
			out.Findings = append(out.Findings, finding{Tag: e.Tag, Level: e.Level, Module: e.Module, Testcase: e.Testcase, Message: e.Message})
		}
	}
	for _, t := range res.NameserverTimings {
		out.NameserverTimings = append(out.NameserverTimings, nsTiming{
			Nameserver: t.Nameserver, Address: t.Address,
			MedianMS: t.MedianMS, AvgMS: t.AvgMS, MinMS: t.MinMS, MaxMS: t.MaxMS,
			Count: t.Count, Status: t.Status,
		})
	}
}

func pollTimeout(timeout time.Duration, domain string, j jobView) error {
	return fmt.Errorf("timed out after %s waiting for %s (run %s still %s); fetch later with run_get", timeout, domain, j.ID, j.Status)
}

func durationMs(j jobView) int64 {
	if j.StartedAt.IsZero() || j.FinishedAt.IsZero() || !j.FinishedAt.After(j.StartedAt) {
		return 0
	}
	return j.FinishedAt.Sub(j.StartedAt).Milliseconds()
}

// toolError turns an HTTP failure into a stable, LLM-readable message. A 401
// carries the fix; 4xx are caller errors; 5xx and transport failures are
// server-side.
func toolError(action string, err error) error {
	var he *httpError
	if errors.As(err, &he) {
		switch {
		case he.status == 401:
			return fmt.Errorf("%s failed: unauthorized (401); set GONEMASTER_TOKEN to a valid admin token", action)
		case he.status == 404:
			return fmt.Errorf("%s failed: not found (404)", action)
		case he.status >= 400 && he.status < 500:
			return fmt.Errorf("%s failed: request rejected (%d): %s", action, he.status, he.body)
		default:
			return fmt.Errorf("%s failed: server error (%d): %s", action, he.status, he.body)
		}
	}
	return fmt.Errorf("%s failed: %v", action, err)
}
