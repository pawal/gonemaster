package apitest

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// maxRunsLimit mirrors the cap the real /runs endpoint enforces, so a client
// that fails to page is caught rather than silently truncated.
const maxRunsLimit = 500

// Opts is the fake server's canned state. A zero Opts answers every route
// with an empty or not-found body.
type Opts struct {
	// RequireToken, when set, makes every route answer 401 unless the
	// request carries the matching Bearer token.
	RequireToken string
	// WhoamiMode selects the GET /whoami answer: "open" reports
	// authenticated, "token" reports authenticated only for RequireToken.
	WhoamiMode string

	// PollsUntilDone keeps GET /jobs/{id} on "running" for that many polls.
	PollsUntilDone int
	// FinalStatus is the terminal job status, "succeeded" when empty.
	FinalStatus string
	// JobError is job.error at the terminal poll.
	JobError string

	// Result answers GET /jobs|runs/{id}/result; nil yields 404.
	Result *Result
	// ResultsByID answers per id and is consulted before Result.
	ResultsByID map[string]Result

	// Run answers GET /runs/{id}; nil yields 404.
	Run *Run
	// Runs are the items of GET /runs, paged when the query asks for it.
	Runs []Run
	// RunsQuery, when set, captures the GET /runs query of every request.
	RunsQuery *url.Values
	// RunsRequests, when set, records the raw query of every GET /runs.
	RunsRequests *[]string

	// Batch answers GET /batches/{id}; nil yields 404.
	Batch *BatchSummary
	// BatchesByID answers per id and is consulted before Batch.
	BatchesByID map[string]BatchSummary
	// BatchList answers GET /batches.
	BatchList *BatchList
	// BatchListQuery, when set, captures the GET /batches query.
	BatchListQuery *url.Values
	// TagValues answers GET /batches/{id}/tag-values; nil yields 404.
	TagValues *TagValues
	// TagValuesQuery, when set, captures the tag-values query.
	TagValuesQuery *url.Values
	// BatchReq, when set, captures the POST /jobs/batch body.
	BatchReq *BatchCreateRequest

	// Entries are the items of GET /entries, filtered by the level query.
	Entries []EntryRecord

	// SpecList answers GET /spec/testcases.
	SpecList *SpecTestcaseList
	// SpecDetail answers GET /spec/testcases/{id}; nil yields 404.
	SpecDetail *SpecTestcaseDetail

	// AnalysisCatalog answers GET /pub/api/v1/analysis/catalog.
	AnalysisCatalog *AnalysisCatalog
	// AnalysisSnapshots answers a cohort's public snapshot list.
	AnalysisSnapshots *AnalysisSnapshotList
	// AnalysisReport answers a cohort's public report; nil yields 404.
	AnalysisReport *AnalysisReport
	// AnalysisReportQuery, when set, captures the report query.
	AnalysisReportQuery *url.Values
}

// terminal reports whether a job status is final.
func terminal(status string) bool {
	switch status {
	case "succeeded", "failed", "canceled":
		return true
	}
	return false
}

// Handler builds the fake's routes. Every request path is under /api/v1.
func Handler(t testing.TB, opts Opts) http.Handler {
	t.Helper()
	if opts.FinalStatus == "" {
		opts.FinalStatus = "succeeded"
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
		if opts.RequireToken != "" && r.Header.Get("Authorization") != "Bearer "+opts.RequireToken {
			writeJSON(w, http.StatusUnauthorized, errBody("unauthorized"))
			return false
		}
		return true
	}
	// route wraps a handler in the token guard.
	route := func(fn func(http.ResponseWriter, *http.Request)) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if guard(w, r) {
				fn(w, r)
			}
		}
	}

	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/v1/whoami", func(w http.ResponseWriter, r *http.Request) {
		resp := Whoami{Mode: opts.WhoamiMode}
		switch opts.WhoamiMode {
		case "token":
			resp.Authenticated = r.Header.Get("Authorization") == "Bearer "+opts.RequireToken
		default:
			resp.Mode = "open"
			resp.Authenticated = true
		}
		writeJSON(w, http.StatusOK, resp)
	})

	mux.HandleFunc("POST /api/v1/jobs", route(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusCreated, Job{ID: "job_1", Domain: "example.com", Status: "queued"})
	}))
	mux.HandleFunc("GET /api/v1/jobs/{id}", route(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		polls++
		p := polls
		mu.Unlock()
		status := "running"
		if p > opts.PollsUntilDone {
			status = opts.FinalStatus
		}
		j := Job{ID: r.PathValue("id"), Domain: "example.com", Status: status}
		if terminal(status) {
			j.StartedAt = time.Unix(1000, 0)
			j.FinishedAt = time.Unix(1008, 0) // 8000 ms
			j.Error = opts.JobError
		}
		writeJSON(w, http.StatusOK, j)
	}))
	mux.HandleFunc("POST /api/v1/jobs/{id}/cancel", route(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, Job{ID: r.PathValue("id"), Status: "canceled"})
	}))
	mux.HandleFunc("POST /api/v1/jobs/batch", route(func(w http.ResponseWriter, r *http.Request) {
		var req BatchCreateRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		if opts.BatchReq != nil {
			*opts.BatchReq = req
		}
		ids := []string{}
		for range req.Domains {
			ids = append(ids, "job_x")
		}
		if len(ids) == 0 && req.FromTag != "" {
			ids = []string{"job_x"}
		}
		writeJSON(w, http.StatusCreated, BatchCreateResponse{BatchID: "batch_new", JobIDs: ids})
	}))

	result := func(w http.ResponseWriter, r *http.Request) {
		if res, ok := opts.ResultsByID[r.PathValue("id")]; ok {
			writeJSON(w, http.StatusOK, res)
			return
		}
		if opts.Result == nil {
			writeJSON(w, http.StatusNotFound, errBody("no result"))
			return
		}
		writeJSON(w, http.StatusOK, *opts.Result)
	}
	mux.HandleFunc("GET /api/v1/jobs/{id}/result", route(result))
	mux.HandleFunc("GET /api/v1/runs/{id}/result", route(result))

	mux.HandleFunc("GET /api/v1/runs/{id}", route(func(w http.ResponseWriter, _ *http.Request) {
		if opts.Run == nil {
			writeJSON(w, http.StatusNotFound, errBody("no run"))
			return
		}
		writeJSON(w, http.StatusOK, *opts.Run)
	}))
	mux.HandleFunc("GET /api/v1/runs", route(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if opts.RunsQuery != nil {
			*opts.RunsQuery = q
		}
		limit, _ := strconv.Atoi(q.Get("limit"))
		if limit > maxRunsLimit {
			writeJSON(w, http.StatusBadRequest, errBody("limit too high"))
			return
		}
		if opts.RunsRequests != nil {
			*opts.RunsRequests = append(*opts.RunsRequests, r.URL.RawQuery)
		}
		items := opts.Runs
		if limit > 0 {
			offset, _ := strconv.Atoi(q.Get("offset"))
			items = page(opts.Runs, offset, limit)
		}
		writeJSON(w, http.StatusOK, RunList{Items: items, Total: len(opts.Runs)})
	}))

	mux.HandleFunc("GET /api/v1/batches", route(func(w http.ResponseWriter, r *http.Request) {
		if opts.BatchListQuery != nil {
			*opts.BatchListQuery = r.URL.Query()
		}
		if opts.BatchList == nil {
			writeJSON(w, http.StatusOK, BatchList{})
			return
		}
		writeJSON(w, http.StatusOK, *opts.BatchList)
	}))
	mux.HandleFunc("GET /api/v1/batches/{id}", route(func(w http.ResponseWriter, r *http.Request) {
		if b, ok := opts.BatchesByID[r.PathValue("id")]; ok {
			writeJSON(w, http.StatusOK, b)
			return
		}
		if opts.Batch == nil {
			writeJSON(w, http.StatusNotFound, errBody("no batch"))
			return
		}
		writeJSON(w, http.StatusOK, *opts.Batch)
	}))
	mux.HandleFunc("DELETE /api/v1/batches/{id}", route(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	mux.HandleFunc("GET /api/v1/batches/{id}/tag-values", route(func(w http.ResponseWriter, r *http.Request) {
		if opts.TagValuesQuery != nil {
			*opts.TagValuesQuery = r.URL.Query()
		}
		if opts.TagValues == nil {
			writeJSON(w, http.StatusNotFound, errBody("no batch"))
			return
		}
		writeJSON(w, http.StatusOK, *opts.TagValues)
	}))

	mux.HandleFunc("GET /api/v1/entries", route(func(w http.ResponseWriter, r *http.Request) {
		level := r.URL.Query().Get("level")
		items := []EntryRecord{}
		for _, e := range opts.Entries {
			if level == "" || e.Level == level {
				items = append(items, e)
			}
		}
		writeJSON(w, http.StatusOK, EntryList{Items: items, Total: len(items)})
	}))

	mux.HandleFunc("GET /api/v1/spec/testcases", route(func(w http.ResponseWriter, _ *http.Request) {
		if opts.SpecList == nil {
			writeJSON(w, http.StatusOK, SpecTestcaseList{})
			return
		}
		writeJSON(w, http.StatusOK, *opts.SpecList)
	}))
	mux.HandleFunc("GET /api/v1/spec/testcases/{id}", route(func(w http.ResponseWriter, _ *http.Request) {
		if opts.SpecDetail == nil {
			writeJSON(w, http.StatusNotFound, errBody("no testcase"))
			return
		}
		writeJSON(w, http.StatusOK, *opts.SpecDetail)
	}))

	// Public analysis reads. They carry no token guard, as on the server.
	mux.HandleFunc("GET /pub/api/v1/analysis/catalog", func(w http.ResponseWriter, _ *http.Request) {
		if opts.AnalysisCatalog == nil {
			writeJSON(w, http.StatusOK, AnalysisCatalog{})
			return
		}
		writeJSON(w, http.StatusOK, *opts.AnalysisCatalog)
	})
	mux.HandleFunc("GET /pub/api/v1/analysis/cohorts/{tag}/snapshots", func(w http.ResponseWriter, _ *http.Request) {
		if opts.AnalysisSnapshots == nil {
			writeJSON(w, http.StatusOK, AnalysisSnapshotList{})
			return
		}
		writeJSON(w, http.StatusOK, *opts.AnalysisSnapshots)
	})
	mux.HandleFunc("GET /pub/api/v1/analysis/cohorts/{tag}/report", func(w http.ResponseWriter, r *http.Request) {
		if opts.AnalysisReportQuery != nil {
			mu.Lock()
			*opts.AnalysisReportQuery = r.URL.Query()
			mu.Unlock()
		}
		if opts.AnalysisReport == nil {
			writeJSON(w, http.StatusNotFound, errBody("no report"))
			return
		}
		writeJSON(w, http.StatusOK, *opts.AnalysisReport)
	})

	return mux
}

// page returns the items of one offset/limit window.
func page[T any](items []T, offset, limit int) []T {
	if offset >= len(items) {
		return []T{}
	}
	end := min(offset+limit, len(items))
	return items[offset:end]
}

// New starts the fake over a real socket and closes it when the test ends.
func New(t testing.TB, opts Opts) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(Handler(t, opts))
	t.Cleanup(srv.Close)
	return srv
}

// handlerTransport answers requests in-process, with no socket.
type handlerTransport struct{ h http.Handler }

func (ht handlerTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	rec := httptest.NewRecorder()
	ht.h.ServeHTTP(rec, r)
	return rec.Result(), nil
}

// Transport answers the fake's routes without a socket, for a client whose
// http.Client is injectable.
func Transport(t testing.TB, opts Opts) http.RoundTripper {
	t.Helper()
	return handlerTransport{h: Handler(t, opts)}
}

// RoundTripFunc adapts a function to http.RoundTripper, for the tests that
// need a handler of their own rather than the fake.
type RoundTripFunc func(*http.Request) (*http.Response, error)

func (fn RoundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return fn(r) }

// JSONResponse builds a canned JSON response, for use inside a RoundTripFunc.
func JSONResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     http.Header{"Content-Type": []string{"application/json"}},
	}
}

// ClientFunc aliases the http.Client factory the CLIs expose as a seam, so
// *ClientFunc accepts a pointer to a package main var of that shape.
type ClientFunc = func(time.Duration) *http.Client

// StubClient points *seam at a client using rt and restores it at test end.
func StubClient(t testing.TB, seam *ClientFunc, rt http.RoundTripper) {
	t.Helper()
	previous := *seam
	t.Cleanup(func() { *seam = previous })
	*seam = func(time.Duration) *http.Client { return &http.Client{Transport: rt} }
}

// StubFake points *seam at the fake server's routes, with no socket.
func StubFake(t testing.TB, seam *ClientFunc, opts Opts) {
	t.Helper()
	StubClient(t, seam, Transport(t, opts))
}

// Capture is a RoundTripper that decodes each request body into a caller
// struct and answers with a canned JSON response, recording the method and
// path of the last request it served.
type Capture struct {
	t      testing.TB
	into   any
	status int
	body   string

	mu     sync.Mutex
	method string
	path   string
}

// CaptureJSON returns a Capture that decodes each request body into into (a
// pointer; nil skips the decode) and replies with status and body.
func CaptureJSON(t testing.TB, into any, status int, body string) *Capture {
	t.Helper()
	return &Capture{t: t, into: into, status: status, body: body}
}

func (c *Capture) RoundTrip(r *http.Request) (*http.Response, error) {
	c.mu.Lock()
	c.method, c.path = r.Method, r.URL.Path
	c.mu.Unlock()
	if c.into != nil && r.Body != nil && r.Body != http.NoBody {
		if err := json.NewDecoder(r.Body).Decode(c.into); err != nil {
			c.t.Errorf("decode %s %s request: %v", r.Method, r.URL.Path, err)
		}
	}
	return JSONResponse(c.status, c.body), nil
}

// Method returns the method of the last request served.
func (c *Capture) Method() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.method
}

// Path returns the path of the last request served.
func (c *Capture) Path() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.path
}
