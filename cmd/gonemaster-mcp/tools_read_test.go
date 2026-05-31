package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// fakeOpts configures the stand-in gonemaster-server.
type fakeOpts struct {
	pollsUntilDone int                     // GET /jobs/{id} returns "running" for this many polls, then finalStatus
	finalStatus    string                  // terminal status (default "succeeded")
	jobError       string                  // job.Error at the terminal poll
	result         *resultView             // body for GET /jobs/{id}/result; nil yields 404
	resultsByID    map[string]resultView   // per-id bodies for GET /jobs/{id}/result (checked first)
	run            *runView                // body for GET /runs/{id}; nil yields 404
	runs           []runView               // items for GET /runs
	runsQuery      *url.Values             // when set, captures the GET /runs query
	specList       *specTestcaseListView   // body for GET /spec/testcases
	specDetail     *specTestcaseDetailView // body for GET /spec/testcases/{id}; nil yields 404
	requireToken   string                  // when set, endpoints return 401 unless the Bearer token matches
}

func newFakeServer(t *testing.T, opts fakeOpts) *httptest.Server {
	t.Helper()
	if opts.finalStatus == "" {
		opts.finalStatus = "succeeded"
	}
	var mu sync.Mutex
	polls := 0

	writeJSON := func(w http.ResponseWriter, code int, v any) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		_ = json.NewEncoder(w).Encode(v)
	}
	errBody := func(msg string) any {
		return map[string]any{"error": map[string]string{"code": "x", "message": msg}}
	}
	guard := func(w http.ResponseWriter, r *http.Request) bool {
		if opts.requireToken != "" && r.Header.Get("Authorization") != "Bearer "+opts.requireToken {
			writeJSON(w, http.StatusUnauthorized, errBody("unauthorized"))
			return false
		}
		return true
	}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/jobs", func(w http.ResponseWriter, r *http.Request) {
		if !guard(w, r) {
			return
		}
		writeJSON(w, http.StatusCreated, jobView{ID: "job_1", Domain: "example.com", Status: "queued"})
	})
	mux.HandleFunc("GET /api/v1/jobs/{id}", func(w http.ResponseWriter, r *http.Request) {
		if !guard(w, r) {
			return
		}
		mu.Lock()
		polls++
		p := polls
		mu.Unlock()
		status := "running"
		if p > opts.pollsUntilDone {
			status = opts.finalStatus
		}
		j := jobView{ID: r.PathValue("id"), Domain: "example.com", Status: status}
		if terminalStatuses[status] {
			j.StartedAt = time.Unix(1000, 0)
			j.FinishedAt = time.Unix(1008, 0) // 8000 ms
			j.Error = opts.jobError
		}
		writeJSON(w, http.StatusOK, j)
	})
	mux.HandleFunc("GET /api/v1/jobs/{id}/result", func(w http.ResponseWriter, r *http.Request) {
		if !guard(w, r) {
			return
		}
		if res, ok := opts.resultsByID[r.PathValue("id")]; ok {
			writeJSON(w, http.StatusOK, res)
			return
		}
		if opts.result == nil {
			writeJSON(w, http.StatusNotFound, errBody("no result"))
			return
		}
		writeJSON(w, http.StatusOK, opts.result)
	})
	mux.HandleFunc("GET /api/v1/runs/{id}", func(w http.ResponseWriter, r *http.Request) {
		if !guard(w, r) {
			return
		}
		if opts.run == nil {
			writeJSON(w, http.StatusNotFound, errBody("no run"))
			return
		}
		writeJSON(w, http.StatusOK, *opts.run)
	})
	mux.HandleFunc("GET /api/v1/runs", func(w http.ResponseWriter, r *http.Request) {
		if !guard(w, r) {
			return
		}
		if opts.runsQuery != nil {
			*opts.runsQuery = r.URL.Query()
		}
		writeJSON(w, http.StatusOK, runListView{Items: opts.runs, Total: len(opts.runs)})
	})
	mux.HandleFunc("GET /api/v1/spec/testcases", func(w http.ResponseWriter, r *http.Request) {
		if !guard(w, r) {
			return
		}
		if opts.specList == nil {
			writeJSON(w, http.StatusOK, specTestcaseListView{})
			return
		}
		writeJSON(w, http.StatusOK, *opts.specList)
	})
	mux.HandleFunc("GET /api/v1/spec/testcases/{id}", func(w http.ResponseWriter, r *http.Request) {
		if !guard(w, r) {
			return
		}
		if opts.specDetail == nil {
			writeJSON(w, http.StatusNotFound, errBody("no testcase"))
			return
		}
		writeJSON(w, http.StatusOK, *opts.specDetail)
	})
	return httptest.NewServer(mux)
}

// callTool runs one tools/call over an in-memory MCP session. On a successful
// (non-error) result it decodes the structured output into out.
func callTool(t *testing.T, api *apiClient, name string, args map[string]any, out any) *mcp.CallToolResult {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	// Join the server goroutine before returning so a test that restores
	// overridden package vars (pollInterval, defaultTestTimeout) cannot race it.
	done := make(chan struct{})
	defer func() { cancel(); <-done }()

	clientT, serverT := mcp.NewInMemoryTransports()
	srv := newMCPServer(api)
	go func() { _ = srv.Run(ctx, serverT); close(done) }()

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "test"}, nil)
	session, err := client.Connect(ctx, clientT, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	defer session.Close()

	res, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("call %s: %v", name, err)
	}
	if out != nil && !res.IsError && res.StructuredContent != nil {
		raw, _ := json.Marshal(res.StructuredContent)
		if e := json.Unmarshal(raw, out); e != nil {
			t.Fatalf("decode %s output: %v", name, e)
		}
	}
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
	ts := newFakeServer(t, fakeOpts{
		pollsUntilDone: 1,
		result: &resultView{
			JobID:  "job_1",
			Status: "succeeded",
			Score:  &resultScore{Score: 72, Grade: "C"},
			Raw: &resultRawView{Entries: []entryView{
				{Module: "consistency", Testcase: "Consistency01", Tag: "SOATIME", Level: "WARNING", Message: "SOA times differ"},
			}},
		},
	})
	defer ts.Close()

	orig := pollInterval
	pollInterval = 5 * time.Millisecond
	defer func() { pollInterval = orig }()

	var out testResult
	res := callTool(t, clientFor(t, ts.URL, ""), "test_domain", map[string]any{"domain": "example.com"}, &out)
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
	ts := newFakeServer(t, fakeOpts{pollsUntilDone: 1 << 30}) // never terminal
	defer ts.Close()

	origP, origT := pollInterval, defaultTestTimeout
	pollInterval = 5 * time.Millisecond
	defaultTestTimeout = 40 * time.Millisecond
	defer func() { pollInterval = origP; defaultTestTimeout = origT }()

	res := callTool(t, clientFor(t, ts.URL, ""), "test_domain", map[string]any{"domain": "example.com"}, nil)
	if !res.IsError {
		t.Fatalf("expected a timeout tool error")
	}
	if !strings.Contains(errorText(res), "timed out") {
		t.Errorf("expected 'timed out', got: %s", errorText(res))
	}
}

func TestRunGet(t *testing.T) {
	grade := "A"
	ts := newFakeServer(t, fakeOpts{
		run: &runView{ID: "run_9", Domain: "good.example", Status: "succeeded", DurationMs: 1234, Grade: &grade},
		result: &resultView{
			JobID:  "run_9",
			Status: "succeeded",
			Score:  &resultScore{Score: 95, Grade: "A"},
			Raw:    &resultRawView{Entries: []entryView{{Module: "nameserver", Tag: "NS01", Level: "INFO"}}},
		},
	})
	defer ts.Close()

	var out testResult
	res := callTool(t, clientFor(t, ts.URL, ""), "run_get", map[string]any{"id": "run_9"}, &out)
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

func TestLatestFor(t *testing.T) {
	g1, g2 := "B", "C"
	s1, s2 := 80, 70
	ts := newFakeServer(t, fakeOpts{runs: []runView{
		{ID: "run_2", Domain: "x.example", Status: "succeeded", Grade: &g1, Score: &s1, WorstLevel: "WARNING", DurationMs: 500, FinishedAt: time.Unix(2000, 0)},
		{ID: "run_1", Domain: "x.example", Status: "succeeded", Grade: &g2, Score: &s2, WorstLevel: "ERROR", DurationMs: 600, FinishedAt: time.Unix(1000, 0)},
	}})
	defer ts.Close()

	var out latestForOutput
	res := callTool(t, clientFor(t, ts.URL, ""), "latest_for", map[string]any{"domain": "x.example"}, &out)
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

func TestRunGetUnauthorizedGivesTokenHint(t *testing.T) {
	ts := newFakeServer(t, fakeOpts{requireToken: "gm_secret", run: &runView{ID: "r", Domain: "d", Status: "succeeded"}})
	defer ts.Close()

	// No token configured: the server returns 401.
	res := callTool(t, clientFor(t, ts.URL, ""), "run_get", map[string]any{"id": "r"}, nil)
	if !res.IsError {
		t.Fatalf("expected an unauthorized tool error")
	}
	txt := errorText(res)
	if !strings.Contains(txt, "401") || !strings.Contains(txt, "GONEMASTER_TOKEN") {
		t.Errorf("expected 401 + token hint, got: %s", txt)
	}
}

func TestRunGetForwardsToken(t *testing.T) {
	ts := newFakeServer(t, fakeOpts{
		requireToken: "gm_secret",
		run:          &runView{ID: "r", Domain: "d", Status: "succeeded"},
		result:       &resultView{JobID: "r", Status: "succeeded", Score: &resultScore{Score: 100, Grade: "A+"}},
	})
	defer ts.Close()

	var out testResult
	res := callTool(t, clientFor(t, ts.URL, "gm_secret"), "run_get", map[string]any{"id": "r"}, &out)
	if res.IsError {
		t.Fatalf("unexpected tool error with valid token: %s", errorText(res))
	}
	if out.Grade != "A+" {
		t.Errorf("grade = %q, want A+", out.Grade)
	}
}
