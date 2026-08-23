package apitest_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"codeberg.org/pawal/gonemaster/internal/apitest"
)

// doGet issues a GET against baseURL+path with an optional bearer token.
func doGet(t *testing.T, baseURL, path, token string, out any) int {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, baseURL+path, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("get %s: %v", path, err)
	}
	defer resp.Body.Close()
	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			t.Fatalf("decode %s: %v", path, err)
		}
	}
	return resp.StatusCode
}

func TestJobPollsThenReportsTerminal(t *testing.T) {
	srv := apitest.New(t, apitest.Opts{PollsUntilDone: 2, JobError: "boom", FinalStatus: "failed"})

	var job struct {
		Status string `json:"status"`
		Error  string `json:"error"`
	}
	for i := range 2 {
		doGet(t, srv.URL, "/api/v1/jobs/job_1", "", &job)
		if job.Status != "running" {
			t.Fatalf("poll %d: status = %q, want running", i+1, job.Status)
		}
	}
	doGet(t, srv.URL, "/api/v1/jobs/job_1", "", &job)
	if job.Status != "failed" || job.Error != "boom" {
		t.Fatalf("terminal poll = %+v, want failed/boom", job)
	}
}

func TestMissingBodiesAreNotFound(t *testing.T) {
	srv := apitest.New(t, apitest.Opts{})

	for _, path := range []string{
		"/api/v1/jobs/job_1/result",
		"/api/v1/runs/run_1",
		"/api/v1/batches/batch_1",
		"/api/v1/batches/batch_1/tag-values",
		"/api/v1/spec/testcases/basic01",
	} {
		if code := doGet(t, srv.URL, path, "", nil); code != http.StatusNotFound {
			t.Errorf("GET %s = %d, want 404", path, code)
		}
	}
}

func TestResultsByIDWinOverResult(t *testing.T) {
	shared := apitest.Result{JobID: "shared", Status: "succeeded"}
	srv := apitest.New(t, apitest.Opts{
		Result:      &shared,
		ResultsByID: map[string]apitest.Result{"special": {JobID: "special", Status: "failed"}},
	})

	var got apitest.Result
	doGet(t, srv.URL, "/api/v1/jobs/special/result", "", &got)
	if got.JobID != "special" || got.Status != "failed" {
		t.Fatalf("per-id result = %+v", got)
	}
	doGet(t, srv.URL, "/api/v1/runs/other/result", "", &got)
	if got.JobID != "shared" {
		t.Fatalf("fallback result = %+v", got)
	}
}

func TestRequireTokenGuardsEveryRouteButWhoami(t *testing.T) {
	srv := apitest.New(t, apitest.Opts{RequireToken: "gm_secret", WhoamiMode: "token"})

	if code := doGet(t, srv.URL, "/api/v1/runs", "", nil); code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated GET /runs = %d, want 401", code)
	}
	if code := doGet(t, srv.URL, "/api/v1/runs", "gm_secret", nil); code != http.StatusOK {
		t.Fatalf("authenticated GET /runs = %d, want 200", code)
	}

	// whoami has to answer without a token: that is how a client discovers
	// that the server is in token mode at all.
	var who apitest.Whoami
	if code := doGet(t, srv.URL, "/api/v1/whoami", "", &who); code != http.StatusOK {
		t.Fatalf("whoami without a token = %d, want 200", code)
	}
	if who.Mode != "token" || who.Authenticated {
		t.Fatalf("whoami without a token = %+v, want token/false", who)
	}
	doGet(t, srv.URL, "/api/v1/whoami", "gm_secret", &who)
	if !who.Authenticated {
		t.Fatalf("whoami with the token = %+v, want authenticated", who)
	}
}

func TestWhoamiDefaultsToOpenMode(t *testing.T) {
	srv := apitest.New(t, apitest.Opts{})
	var who apitest.Whoami
	doGet(t, srv.URL, "/api/v1/whoami", "", &who)
	if who.Mode != "open" || !who.Authenticated {
		t.Fatalf("whoami = %+v, want open/true", who)
	}
}

func TestRunsPageOnlyWhenAsked(t *testing.T) {
	runs := make([]apitest.Run, 7)
	for i := range runs {
		runs[i] = apitest.Run{ID: "run-" + string(rune('a'+i))}
	}
	var query url.Values
	var requests []string
	srv := apitest.New(t, apitest.Opts{Runs: runs, RunsQuery: &query, RunsRequests: &requests})

	var list apitest.RunList
	doGet(t, srv.URL, "/api/v1/runs", "", &list)
	if len(list.Items) != 7 || list.Total != 7 {
		t.Fatalf("unpaged = %d items, total %d, want 7/7", len(list.Items), list.Total)
	}

	doGet(t, srv.URL, "/api/v1/runs?limit=3&offset=6", "", &list)
	if len(list.Items) != 1 || list.Items[0].ID != "run-g" || list.Total != 7 {
		t.Fatalf("last page = %+v, total %d", list.Items, list.Total)
	}
	if query.Get("offset") != "6" {
		t.Fatalf("captured query = %v", query)
	}
	if len(requests) != 2 {
		t.Fatalf("recorded requests = %v, want 2", requests)
	}
}

func TestRunsRejectsAnOverLargeLimit(t *testing.T) {
	srv := apitest.New(t, apitest.Opts{Runs: []apitest.Run{{ID: "run-1"}}})
	if code := doGet(t, srv.URL, "/api/v1/runs?limit=501", "", nil); code != http.StatusBadRequest {
		t.Fatalf("limit=501 = %d, want 400", code)
	}
}

func TestEntriesFilterByLevel(t *testing.T) {
	srv := apitest.New(t, apitest.Opts{Entries: []apitest.EntryRecord{
		{Domain: "a.example", Level: "WARNING"},
		{Domain: "b.example", Level: "ERROR"},
	}})

	var list apitest.EntryList
	doGet(t, srv.URL, "/api/v1/entries?level=ERROR", "", &list)
	if len(list.Items) != 1 || list.Items[0].Domain != "b.example" {
		t.Fatalf("filtered entries = %+v", list.Items)
	}
	doGet(t, srv.URL, "/api/v1/entries", "", &list)
	if len(list.Items) != 2 {
		t.Fatalf("unfiltered entries = %+v", list.Items)
	}
}

func TestBatchCreateCapturesTheRequest(t *testing.T) {
	var req apitest.BatchCreateRequest
	srv := apitest.New(t, apitest.Opts{BatchReq: &req})

	body := `{"domains":["a.example","b.example"],"profile":"strict"}`
	resp, err := http.Post(srv.URL+"/api/v1/jobs/batch", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("post batch: %v", err)
	}
	defer resp.Body.Close()
	var out apitest.BatchCreateResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(out.JobIDs) != 2 || out.BatchID != "batch_new" {
		t.Fatalf("batch create = %+v", out)
	}
	if len(req.Domains) != 2 || req.Profile != "strict" {
		t.Fatalf("captured request = %+v", req)
	}
}

func TestTransportAnswersWithoutASocket(t *testing.T) {
	client := &http.Client{Transport: apitest.Transport(t, apitest.Opts{
		Run: &apitest.Run{ID: "run_9", Domain: "x.example", Status: "succeeded"},
	})}

	resp, err := client.Get("http://gonemaster.invalid/api/v1/runs/run_9")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	var run apitest.Run
	if err := json.NewDecoder(resp.Body).Decode(&run); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if run.ID != "run_9" {
		t.Fatalf("run = %+v", run)
	}
}

func TestStubClientRestoresTheSeam(t *testing.T) {
	sentinel := &http.Client{Timeout: 42 * time.Second}
	seam := apitest.ClientFunc(func(time.Duration) *http.Client { return sentinel })

	t.Run("stubbed", func(t *testing.T) {
		apitest.StubFake(t, &seam, apitest.Opts{})
		if seam(time.Second) == sentinel {
			t.Fatal("expected the stub to replace the seam")
		}
	})

	if seam(time.Second) != sentinel {
		t.Fatal("expected the seam restored after the subtest")
	}
}

func TestJSONResponseCarriesItsBody(t *testing.T) {
	resp := apitest.JSONResponse(http.StatusTeapot, `{"a":1}`)
	if resp.StatusCode != http.StatusTeapot {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if string(body) != `{"a":1}` {
		t.Fatalf("body = %q", body)
	}
	if got := resp.Header.Get("Content-Type"); got != "application/json" {
		t.Fatalf("content type = %q", got)
	}
}

// TestDiffPairCoversEveryDeltaKind guards the shared fixture itself: both
// CLIs' run-diff tests assert one added, one removed and one changed tag, so
// a fixture that lost a kind would turn those into silent passes.
func TestDiffPairCoversEveryDeltaKind(t *testing.T) {
	before, after := apitest.DiffPair()

	level := func(entries []apitest.Entry, tag string) (string, bool) {
		for _, e := range entries {
			if e.Tag == tag {
				return e.Level, true
			}
		}
		return "", false
	}

	if l, ok := level(before, apitest.DiffTagUnchanged); !ok || l != "INFO" {
		t.Errorf("unchanged tag before = %q (present=%v), want INFO", l, ok)
	}
	if l, ok := level(after, apitest.DiffTagUnchanged); !ok || l != "INFO" {
		t.Errorf("unchanged tag after = %q (present=%v), want INFO", l, ok)
	}
	if _, ok := level(before, apitest.DiffTagRemoved); !ok {
		t.Error("removed tag must be in before")
	}
	if _, ok := level(after, apitest.DiffTagRemoved); ok {
		t.Error("removed tag must not be in after")
	}
	if _, ok := level(before, apitest.DiffTagAdded); ok {
		t.Error("added tag must not be in before")
	}
	if _, ok := level(after, apitest.DiffTagAdded); !ok {
		t.Error("added tag must be in after")
	}
	from, okFrom := level(before, apitest.DiffTagChanged)
	to, okTo := level(after, apitest.DiffTagChanged)
	if !okFrom || !okTo || from == to {
		t.Errorf("changed tag = %q -> %q (present=%v/%v), want two different levels", from, to, okFrom, okTo)
	}
}

func TestDiffResultsCarryTheGrades(t *testing.T) {
	results := apitest.DiffResults("a", "b")
	if len(results) != 2 {
		t.Fatalf("results = %d, want 2", len(results))
	}
	if results["a"].Score.Grade != "A" || results["b"].Score.Grade != "C" {
		t.Fatalf("grades = %q -> %q, want A -> C", results["a"].Score.Grade, results["b"].Score.Grade)
	}
	before, after := apitest.DiffPair()
	if len(results["a"].Raw.Entries) != len(before) || len(results["b"].Raw.Entries) != len(after) {
		t.Fatal("results must carry the DiffPair entries")
	}
}
