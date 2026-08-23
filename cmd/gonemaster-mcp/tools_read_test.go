package main

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"codeberg.org/pawal/gonemaster/cmd/internal/mcptest"
	"codeberg.org/pawal/gonemaster/internal/apitest"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// callTool runs one tools/call over an in-memory MCP session. On a successful
// (non-error) result it decodes the structured output into out.
func callTool(t *testing.T, api *apiClient, name string, args map[string]any, out any) *mcp.CallToolResult {
	t.Helper()
	var res *mcp.CallToolResult
	mcptest.Session(t, newMCPServer(api, true), func(ctx context.Context, session *mcp.ClientSession) {
		var err error
		res, err = session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
		if err != nil {
			t.Fatalf("call %s: %v", name, err)
		}
		if out != nil && !res.IsError && res.StructuredContent != nil {
			raw, _ := json.Marshal(res.StructuredContent)
			if e := json.Unmarshal(raw, out); e != nil {
				t.Fatalf("decode %s output: %v", name, e)
			}
		}
	})
	return res
}

func errorText(res *mcp.CallToolResult) string {
	var b strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			b.WriteString(tc.Text)
		}
	}
	return b.String()
}

func TestTestDomainSucceeds(t *testing.T) {
	api := fakeAPI(t, apitest.Opts{
		PollsUntilDone: 1,
		Result: &apitest.Result{
			JobID:  "job_1",
			Status: "succeeded",
			Score:  &apitest.Score{Score: 72, Grade: "C"},
			Raw: &apitest.ResultRaw{Entries: []apitest.Entry{
				{Module: "consistency", Testcase: "Consistency01", Tag: "SOATIME", Level: "WARNING", Message: "SOA times differ"},
			}},
		},
	})

	orig := pollInterval
	pollInterval = 5 * time.Millisecond
	defer func() { pollInterval = orig }()

	var out testResult
	res := callTool(t, api, "test_domain", map[string]any{"domain": "example.com"}, &out)
	if res.IsError {
		t.Fatalf("unexpected tool error: %s", errorText(res))
	}
	if out.Status != "succeeded" {
		t.Errorf("status = %q", out.Status)
	}
	if out.Grade != "C" || out.Score == nil || *out.Score != 72 {
		t.Errorf("grade/score wrong: %+v", out)
	}
	if out.DurationMs != 8000 {
		t.Errorf("duration_ms = %d, want 8000", out.DurationMs)
	}
	if len(out.Findings) != 1 || out.Findings[0].Tag != "SOATIME" || out.Findings[0].Level != "WARNING" {
		t.Errorf("findings wrong: %+v", out.Findings)
	}
}

func TestTestDomainTimeout(t *testing.T) {
	api := fakeAPI(t, apitest.Opts{PollsUntilDone: 1 << 30}) // never terminal

	origP, origT := pollInterval, defaultTestTimeout
	pollInterval = 5 * time.Millisecond
	defaultTestTimeout = 40 * time.Millisecond
	defer func() { pollInterval = origP; defaultTestTimeout = origT }()

	res := callTool(t, api, "test_domain", map[string]any{"domain": "example.com"}, nil)
	if !res.IsError {
		t.Fatalf("expected a timeout tool error")
	}
	if !strings.Contains(errorText(res), "timed out") {
		t.Errorf("expected 'timed out', got: %s", errorText(res))
	}
}

func TestRunGet(t *testing.T) {
	grade := "A"
	api := fakeAPI(t, apitest.Opts{
		Run: &apitest.Run{ID: "run_9", Domain: "good.example", Status: "succeeded", DurationMs: 1234, Grade: &grade},
		Result: &apitest.Result{
			JobID:  "run_9",
			Status: "succeeded",
			Score:  &apitest.Score{Score: 95, Grade: "A"},
			Raw:    &apitest.ResultRaw{Entries: []apitest.Entry{{Module: "nameserver", Tag: "NS01", Level: "INFO"}}},
		},
	})

	var out testResult
	res := callTool(t, api, "run_get", map[string]any{"id": "run_9"}, &out)
	if res.IsError {
		t.Fatalf("unexpected tool error: %s", errorText(res))
	}
	if out.Domain != "good.example" || out.Status != "succeeded" {
		t.Errorf("run metadata wrong: %+v", out)
	}
	if out.Grade != "A" || out.Score == nil || *out.Score != 95 {
		t.Errorf("score wrong: %+v", out)
	}
	if out.DurationMs != 1234 {
		t.Errorf("duration_ms = %d, want 1234", out.DurationMs)
	}
	if len(out.Findings) != 1 || out.Findings[0].Tag != "NS01" {
		t.Errorf("findings wrong: %+v", out.Findings)
	}
}

func TestRunGetIncludesNameserverTimings(t *testing.T) {
	api := fakeAPI(t, apitest.Opts{
		Run: &apitest.Run{ID: "run_5", Domain: "timed.example", Status: "succeeded"},
		Result: &apitest.Result{
			JobID:  "run_5",
			Status: "succeeded",
			NameserverTimings: []apitest.NSTiming{
				{Nameserver: "ns1.example", Address: "192.0.2.1", AvgMS: 13, MedianMS: 12.5, MinMS: 9, MaxMS: 22, Count: 5, Status: "ok"},
				{Nameserver: "ns2.example", Address: "192.0.2.2", Status: "unreachable"},
			},
		},
	})

	var out testResult
	res := callTool(t, api, "run_get", map[string]any{"id": "run_5"}, &out)
	if res.IsError {
		t.Fatalf("unexpected tool error: %s", errorText(res))
	}
	if len(out.NameserverTimings) != 2 {
		t.Fatalf("expected 2 nameserver timings, got %d", len(out.NameserverTimings))
	}
	if out.NameserverTimings[0].Nameserver != "ns1.example" || out.NameserverTimings[0].MedianMS != 12.5 || out.NameserverTimings[0].Count != 5 {
		t.Errorf("first timing wrong: %+v", out.NameserverTimings[0])
	}
	if out.NameserverTimings[1].Status != "unreachable" {
		t.Errorf("expected unreachable status, got %+v", out.NameserverTimings[1])
	}
}

func TestLatestFor(t *testing.T) {
	g1, g2 := "B", "C"
	s1, s2 := 80, 70
	api := fakeAPI(t, apitest.Opts{Runs: []apitest.Run{
		{ID: "run_2", Domain: "x.example", Status: "succeeded", Grade: &g1, Score: &s1, WorstLevel: "WARNING", DurationMs: 500, FinishedAt: time.Unix(2000, 0)},
		{ID: "run_1", Domain: "x.example", Status: "succeeded", Grade: &g2, Score: &s2, WorstLevel: "ERROR", DurationMs: 600, FinishedAt: time.Unix(1000, 0)},
	}})

	var out latestForOutput
	res := callTool(t, api, "latest_for", map[string]any{"domain": "x.example"}, &out)
	if res.IsError {
		t.Fatalf("unexpected tool error: %s", errorText(res))
	}
	if out.Count != 2 || len(out.Runs) != 2 {
		t.Fatalf("count = %d", out.Count)
	}
	if out.Runs[0].RunID != "run_2" || out.Runs[0].Grade != "B" || out.Runs[0].Score == nil || *out.Runs[0].Score != 80 {
		t.Errorf("first run wrong: %+v", out.Runs[0])
	}
	if out.Runs[0].FinishedAt == "" {
		t.Errorf("expected finished_at to be set")
	}
}

func TestRunOutputsExposeBatchID(t *testing.T) {
	// run_get and the run-summary tools must surface the batch a run belongs
	// to so an agent can pivot from a run to its cohort.
	api := fakeAPI(t, apitest.Opts{
		Run: &apitest.Run{ID: "run_b", Domain: "x.example", BatchID: "batch_42", Status: "succeeded"},
		Runs: []apitest.Run{
			{ID: "run_b", Domain: "x.example", BatchID: "batch_42", Status: "succeeded", FinishedAt: time.Unix(2000, 0)},
		},
	})

	var got testResult
	if res := callTool(t, api, "run_get", map[string]any{"id": "run_b"}, &got); res.IsError {
		t.Fatalf("run_get error: %s", errorText(res))
	}
	if got.BatchID != "batch_42" {
		t.Errorf("run_get batch_id = %q, want batch_42", got.BatchID)
	}

	var latest latestForOutput
	if res := callTool(t, api, "latest_for", map[string]any{"domain": "x.example"}, &latest); res.IsError {
		t.Fatalf("latest_for error: %s", errorText(res))
	}
	if len(latest.Runs) != 1 || latest.Runs[0].BatchID != "batch_42" {
		t.Errorf("latest_for batch_id not surfaced: %+v", latest.Runs)
	}
}

func TestRunOutputsExposePublicID(t *testing.T) {
	// run_get and latest_for must surface public_id so an agent can build a
	// shareable report link.
	api := fakeAPI(t, apitest.Opts{
		Run: &apitest.Run{ID: "run_p", Domain: "x.example", PublicID: "Ab3xZ9k0", Status: "succeeded"},
		Runs: []apitest.Run{
			{ID: "run_p", Domain: "x.example", PublicID: "Ab3xZ9k0", Status: "succeeded", FinishedAt: time.Unix(2000, 0)},
		},
	})

	var got testResult
	if res := callTool(t, api, "run_get", map[string]any{"id": "run_p"}, &got); res.IsError {
		t.Fatalf("run_get error: %s", errorText(res))
	}
	if got.PublicID != "Ab3xZ9k0" {
		t.Errorf("run_get public_id = %q, want Ab3xZ9k0", got.PublicID)
	}

	var latest latestForOutput
	if res := callTool(t, api, "latest_for", map[string]any{"domain": "x.example"}, &latest); res.IsError {
		t.Fatalf("latest_for error: %s", errorText(res))
	}
	if len(latest.Runs) != 1 || latest.Runs[0].PublicID != "Ab3xZ9k0" {
		t.Errorf("latest_for public_id not surfaced: %+v", latest.Runs)
	}
}

func TestRunOutputsOmitPublicIDWhenAbsent(t *testing.T) {
	// A run without a public_id must not invent one.
	api := fakeAPI(t, apitest.Opts{Run: &apitest.Run{ID: "run_np", Domain: "x.example", Status: "succeeded"}})
	var got testResult
	if res := callTool(t, api, "run_get", map[string]any{"id": "run_np"}, &got); res.IsError {
		t.Fatalf("run_get error: %s", errorText(res))
	}
	if got.PublicID != "" {
		t.Errorf("public_id should be empty when the run has none, got %q", got.PublicID)
	}
}

func TestRunGetUnauthorizedGivesTokenHint(t *testing.T) {
	api := fakeAPI(t, apitest.Opts{RequireToken: "gm_secret", Run: &apitest.Run{ID: "r", Domain: "d", Status: "succeeded"}})

	// No token configured: the server returns 401.
	res := callTool(t, api, "run_get", map[string]any{"id": "r"}, nil)
	if !res.IsError {
		t.Fatalf("expected an unauthorized tool error")
	}
	txt := errorText(res)
	if !strings.Contains(txt, "401") || !strings.Contains(txt, "GONEMASTER_TOKEN") {
		t.Errorf("expected 401 + token hint, got: %s", txt)
	}
}

func TestRunGetForwardsToken(t *testing.T) {
	api := fakeAPI(t, apitest.Opts{
		RequireToken: "gm_secret",
		Run:          &apitest.Run{ID: "r", Domain: "d", Status: "succeeded"},
		Result:       &apitest.Result{JobID: "r", Status: "succeeded", Score: &apitest.Score{Score: 100, Grade: "A+"}},
	})

	var out testResult
	res := callTool(t, clientFor(t, api.baseURL, "gm_secret"), "run_get", map[string]any{"id": "r"}, &out)
	if res.IsError {
		t.Fatalf("unexpected tool error with valid token: %s", errorText(res))
	}
	if out.Grade != "A+" {
		t.Errorf("grade = %q, want A+", out.Grade)
	}
}
