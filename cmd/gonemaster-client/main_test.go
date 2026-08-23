package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"codeberg.org/pawal/gonemaster/cmd/internal/clitest"
	"codeberg.org/pawal/gonemaster/engine/normalization"
	"codeberg.org/pawal/gonemaster/internal/apitest"
)

func TestCLIMDDocumentsPurge(t *testing.T) {
	clitest.FileContains(t, "../../docs/client/jobs.md",
		"jobs purge",
		"--older-than",
		"purged_jobs",
		"retention_days",
	)
}

func TestNormalizeBaseURL(t *testing.T) {
	// The same eight cases the MCP bridge's normalizeBaseURL is pinned on;
	// the two implementations have to agree.
	tests := []struct {
		input string
		want  string
	}{
		{"", defaultServer},
		{"http://localhost:8080", "http://localhost:8080/api/v1"},
		{"http://localhost:8080/", "http://localhost:8080/api/v1"},
		{"http://localhost:8080/api/v1", "http://localhost:8080/api/v1"},
		{"http://localhost:8080/api/v1/", "http://localhost:8080/api/v1"},
		{"localhost:9000", "http://localhost:9000/api/v1"},
		{"https://gm.example.com/api/v1/", "https://gm.example.com/api/v1"},
		{"http://host/prefix", "http://host/prefix/api/v1"},
	}
	for _, tt := range tests {
		got, err := normalizeBaseURL(tt.input)
		if err != nil {
			t.Fatalf("normalizeBaseURL(%q) error: %v", tt.input, err)
		}
		if got != tt.want {
			t.Fatalf("normalizeBaseURL(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestParseOverrideValue(t *testing.T) {
	if got := parseOverrideValue("5"); got.(float64) != 5 {
		t.Fatalf("expected numeric override, got %#v", got)
	}
	if got := parseOverrideValue("true"); got.(bool) != true {
		t.Fatalf("expected boolean override, got %#v", got)
	}
	if got := parseOverrideValue("hello"); got.(string) != "hello" {
		t.Fatalf("expected string override, got %#v", got)
	}
}

func TestRunVersion(t *testing.T) {
	res := clitest.Run(t, run, "--version")
	res.RequireCode(t, 0)
	res.RequireOutContains(t, "Gonemaster version")
	res.RequireOutContains(t, "Miekg DNS version")
	if len(res.Err) != 0 {
		t.Fatalf("expected no stderr output, got %q", res.Err)
	}
}

func TestJobsCreateSendsNormalizedDomain(t *testing.T) {
	var gotDomain string
	apitest.StubClient(t, &newHTTPClient, apitest.RoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/api/v1/jobs" {
			t.Fatalf("expected /api/v1/jobs, got %s", r.URL.Path)
		}
		var req jobCreateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		gotDomain = req.Domain
		body := fmt.Sprintf(`{"id":"job_1","domain":"%s","status":"queued","created_at":"2026-02-03T00:00:00Z","progress":0}`, req.Domain)
		resp := &http.Response{
			StatusCode: http.StatusCreated,
			Body:       io.NopCloser(bytes.NewBufferString(body)),
			Header:     http.Header{"Content-Type": []string{"application/json"}},
		}
		return resp, nil
	}))

	res := clitest.Run(t, run, "--server", "http://example.test", "--format", "json", "jobs", "create", "--domain", "räksmörgås.se")
	res.RequireCode(t, 0)
	errs, normalized := normalization.NormalizeName("räksmörgås.se")
	if len(errs) != 0 {
		t.Fatalf("normalization errors: %v", errs)
	}
	if gotDomain != normalized {
		t.Fatalf("expected domain %q, got %q", normalized, gotDomain)
	}
}

const fixtureTime = "2026-02-03T00:00:00Z"

// batchItem is one job row of a fake batch-summary response.
type batchItem struct {
	id       string
	domain   string
	status   string
	progress int
}

// batchPage is the slice of a batch one response carries. A zero page is the
// whole batch, with no pagination fields.
type batchPage struct {
	items      []batchItem
	limit      int
	offset     int
	nextCursor string
}

// batchFixture renders one batch-summary response with the field names the
// server sends; total and status_counts always describe the whole batch.
func batchFixture(id string, all []batchItem, page batchPage) string {
	counts := map[string]int{}
	for _, it := range all {
		counts[it.status]++
	}
	countsJSON, err := json.Marshal(counts)
	if err != nil {
		panic(err)
	}

	items := page.items
	if items == nil {
		items = all
	}
	rows := make([]string, len(items))
	for i, it := range items {
		rows[i] = fmt.Sprintf(`{"id":%q,"domain":%q,"status":%q,"created_at":%q,"progress":%d}`,
			it.id, it.domain, it.status, fixtureTime, it.progress)
	}

	body := fmt.Sprintf(`{"batch_id":%q,"total":%d,"status_counts":%s,"items":[%s],"created_at":%q`,
		id, len(all), countsJSON, strings.Join(rows, ","), fixtureTime)
	if page.limit > 0 {
		body += fmt.Sprintf(`,"limit":%d,"offset":%d`, page.limit, page.offset)
	}
	if page.nextCursor != "" {
		body += fmt.Sprintf(`,"next_cursor":%q`, page.nextCursor)
	}
	return body + "}"
}

// cancelArm answers POST /api/v1/jobs/{id}/cancel and records the job id.
func cancelArm(t *testing.T, r *http.Request, canceled *[]string) *http.Response {
	t.Helper()
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 5 {
		t.Fatalf("unexpected cancel path %s", r.URL.Path)
	}
	jobID := parts[len(parts)-2]
	*canceled = append(*canceled, jobID)
	return apitest.JSONResponse(http.StatusOK, fmt.Sprintf(
		`{"id":%q,"domain":"example.com","status":"canceled","created_at":%q,"progress":100}`, jobID, fixtureTime))
}

// queueRemoveArm answers POST /api/v1/queue/remove and records the job ids.
func queueRemoveArm(t *testing.T, r *http.Request, removed *[]string) *http.Response {
	t.Helper()
	var req queueRemoveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		t.Fatalf("decode queue remove: %v", err)
	}
	*removed = append(*removed, req.JobIDs...)
	return apitest.JSONResponse(http.StatusOK, `{"removed":["job_q"]}`)
}

var (
	jobQueued    = batchItem{id: "job_q", domain: "example.com", status: "queued"}
	jobRunning   = batchItem{id: "job_r", domain: "example.net", status: "running", progress: 50}
	jobSucceeded = batchItem{id: "job_s", domain: "example.org", status: "succeeded", progress: 100}

	// The paginated batches answer the first page with two of three items.
	pagedItems = []batchItem{
		{id: "job_q1", domain: "example.com", status: "queued"},
		{id: "job_r1", domain: "example.net", status: "running", progress: 50},
		{id: "job_q2", domain: "example.org", status: "queued"},
	}
	pagedFirst  = batchPage{items: pagedItems[:2], limit: 2, nextCursor: "2"}
	pagedSecond = batchPage{items: pagedItems[2:], limit: 500, offset: 2}
)

func TestBatchesCancelCallsCancelForQueuedAndRunning(t *testing.T) {
	var canceled []string
	all := []batchItem{jobQueued, jobRunning, jobSucceeded}
	apitest.StubClient(t, &newHTTPClient, apitest.RoundTripFunc(func(r *http.Request) (*http.Response, error) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/batches/batch_1":
			return apitest.JSONResponse(http.StatusOK, batchFixture("batch_1", all, batchPage{})), nil
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/cancel"):
			return cancelArm(t, r, &canceled), nil
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		return nil, nil
	}))

	res := clitest.Run(t, run, "--server", "http://example.test", "--format", "json", "batches", "cancel", "batch_1")
	res.RequireCode(t, 0)
	if len(canceled) != 2 {
		t.Fatalf("expected 2 canceled jobs, got %d", len(canceled))
	}
	want := map[string]bool{"job_q": true, "job_r": true}
	for _, id := range canceled {
		if !want[id] {
			t.Fatalf("unexpected canceled job %s", id)
		}
		delete(want, id)
	}
	if len(want) != 0 {
		t.Fatalf("missing canceled jobs: %v", want)
	}
}

func TestBatchesRemoveCallsQueueRemoveForQueued(t *testing.T) {
	var removed []string
	all := []batchItem{jobQueued, jobRunning}
	apitest.StubClient(t, &newHTTPClient, apitest.RoundTripFunc(func(r *http.Request) (*http.Response, error) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/batches/batch_2":
			return apitest.JSONResponse(http.StatusOK, batchFixture("batch_2", all, batchPage{})), nil
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/queue/remove":
			return queueRemoveArm(t, r, &removed), nil
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		return nil, nil
	}))

	res := clitest.Run(t, run, "--server", "http://example.test", "--format", "json", "batches", "remove", "batch_2")
	res.RequireCode(t, 0)
	if len(removed) != 1 || removed[0] != "job_q" {
		t.Fatalf("expected queue remove for job_q, got %v", removed)
	}
}

func TestBatchesRemoveCancelRunning(t *testing.T) {
	var removed, canceled []string
	all := []batchItem{jobQueued, jobRunning, jobSucceeded}
	apitest.StubClient(t, &newHTTPClient, apitest.RoundTripFunc(func(r *http.Request) (*http.Response, error) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/batches/batch_3":
			return apitest.JSONResponse(http.StatusOK, batchFixture("batch_3", all, batchPage{})), nil
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/queue/remove":
			return queueRemoveArm(t, r, &removed), nil
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/cancel"):
			return cancelArm(t, r, &canceled), nil
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		return nil, nil
	}))

	res := clitest.Run(t, run, "--server", "http://example.test", "--format", "json", "batches", "remove", "--cancel-running", "batch_3")
	res.RequireCode(t, 0)
	if len(removed) != 1 || removed[0] != "job_q" {
		t.Fatalf("expected queue remove for job_q, got %v", removed)
	}
	if len(canceled) != 1 || canceled[0] != "job_r" {
		t.Fatalf("expected cancel for job_r, got %v", canceled)
	}
}

// pagedBatchArm answers the two GET pages of a paginated batch, counting the
// follow-up requests.
func pagedBatchArm(t *testing.T, r *http.Request, id string, pageCalls *int) (*http.Response, bool) {
	t.Helper()
	if r.Method != http.MethodGet || r.URL.Path != "/api/v1/batches/"+id {
		return nil, false
	}
	if r.URL.RawQuery == "" {
		return apitest.JSONResponse(http.StatusOK, batchFixture(id, pagedItems, pagedFirst)), true
	}
	if got := r.URL.Query().Get("offset"); got != "2" {
		t.Fatalf("expected second request offset=2, got %q", got)
	}
	*pageCalls++
	return apitest.JSONResponse(http.StatusOK, batchFixture(id, pagedItems, pagedSecond)), true
}

func TestFetchBatchSummaryPaginatesItems(t *testing.T) {
	var pageCalls int
	apitest.StubClient(t, &newHTTPClient, apitest.RoundTripFunc(func(r *http.Request) (*http.Response, error) {
		resp, ok := pagedBatchArm(t, r, "batch_paginated", &pageCalls)
		if !ok {
			t.Fatalf("unexpected request %s %s?%s", r.Method, r.URL.Path, r.URL.RawQuery)
		}
		return resp, nil
	}))

	client := &apiClient{
		baseURL:    "http://example.test/api/v1",
		httpClient: newHTTPClient(5 * time.Second),
		headers:    http.Header{},
		locale:     "en",
	}

	summary, err := fetchBatchSummary(context.Background(), client, "batch_paginated")
	if err != nil {
		t.Fatalf("fetchBatchSummary returned error: %v", err)
	}
	if pageCalls != 1 {
		t.Fatalf("expected one paginated follow-up request, got %d", pageCalls)
	}
	if len(summary.Items) != 3 {
		t.Fatalf("expected 3 items after pagination, got %d", len(summary.Items))
	}
	if summary.Items[2].ID != "job_q2" {
		t.Fatalf("expected final item job_q2, got %q", summary.Items[2].ID)
	}
}

func TestBatchesCancelIncludesPaginatedBatchItems(t *testing.T) {
	var canceled []string
	var pageCalls int
	apitest.StubClient(t, &newHTTPClient, apitest.RoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if resp, ok := pagedBatchArm(t, r, "batch_4", &pageCalls); ok {
			return resp, nil
		}
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/cancel") {
			return cancelArm(t, r, &canceled), nil
		}
		t.Fatalf("unexpected request %s %s?%s", r.Method, r.URL.Path, r.URL.RawQuery)
		return nil, nil
	}))

	res := clitest.Run(t, run, "--server", "http://example.test", "--format", "json", "batches", "cancel", "batch_4")
	res.RequireCode(t, 0)
	if len(canceled) != 3 {
		t.Fatalf("expected 3 canceled jobs from paginated batch, got %d (%v)", len(canceled), canceled)
	}
}

func TestFetchJobResultRetriesOnNotFound(t *testing.T) {
	var resultCalls int
	apitest.StubClient(t, &newHTTPClient, apitest.RoundTripFunc(func(r *http.Request) (*http.Response, error) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/jobs/job_1":
			body := `{"id":"job_1","domain":"example.com","status":"succeeded","created_at":"2026-02-03T00:00:00Z","progress":100}`
			return apitest.JSONResponse(http.StatusOK, body), nil
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/jobs/job_1/result":
			resultCalls++
			if resultCalls == 1 {
				body := `{"error":{"code":"not_found","message":"job result not found"}}`
				return apitest.JSONResponse(http.StatusNotFound, body), nil
			}
			body := `{"job_id":"job_1","status":"succeeded","summary":{"levels":{"NOTICE":1},"total":1}}`
			return apitest.JSONResponse(http.StatusOK, body), nil
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		return nil, nil
	}))

	client := &apiClient{
		baseURL:    "http://example.test/api/v1",
		httpClient: newHTTPClient(5 * time.Second),
		headers:    http.Header{},
		locale:     "en",
	}

	_, err := fetchJobResult(context.Background(), client, "job_1")
	if err != nil {
		t.Fatalf("expected retry success, got %v", err)
	}
	if resultCalls < 2 {
		t.Fatalf("expected retries, got %d calls", resultCalls)
	}
}

func TestJobsResultsFlagsAfterID(t *testing.T) {
	apitest.StubClient(t, &newHTTPClient, apitest.RoundTripFunc(func(r *http.Request) (*http.Response, error) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/jobs/job_1":
			body := `{"id":"job_1","domain":"example.com","status":"succeeded","created_at":"2026-02-03T00:00:00Z","progress":100}`
			return apitest.JSONResponse(http.StatusOK, body), nil
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/jobs/job_1/result":
			body := `{"job_id":"job_1","status":"succeeded","summary":{"levels":{"NOTICE":1},"total":1}}`
			return apitest.JSONResponse(http.StatusOK, body), nil
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		return nil, nil
	}))

	res := clitest.Run(t, run, "--server", "http://example.test", "--format", "json", "jobs", "results", "job_1", "--view", "translated")
	res.RequireCode(t, 0)
}

func TestJobsPurgeSendsPostAndPrintsPretty(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody map[string]int
	apitest.StubClient(t, &newHTTPClient, apitest.RoundTripFunc(func(r *http.Request) (*http.Response, error) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		return apitest.JSONResponse(http.StatusOK, `{"purged_jobs":42}`), nil
	}))

	res := clitest.Run(t, run, "--server", "http://example.test", "jobs", "purge", "--older-than", "90")
	res.RequireCode(t, 0)
	if gotMethod != http.MethodPost {
		t.Fatalf("expected POST, got %s", gotMethod)
	}
	if gotPath != "/api/v1/jobs/purge" {
		t.Fatalf("expected /api/v1/jobs/purge, got %s", gotPath)
	}
	if gotBody["older_than_days"] != 90 {
		t.Fatalf("expected older_than_days=90, got %v", gotBody)
	}
	res.RequireOutContains(t, "42")
}

func TestJobsPurgeJSONOutput(t *testing.T) {
	apitest.StubClient(t, &newHTTPClient, apitest.RoundTripFunc(func(_ *http.Request) (*http.Response, error) {
		return apitest.JSONResponse(http.StatusOK, `{"purged_jobs":7}`), nil
	}))

	res := clitest.Run(t, run, "--server", "http://example.test", "--format", "json", "jobs", "purge", "--older-than", "30")
	res.RequireCode(t, 0)
	var resp map[string]int64
	if err := json.Unmarshal([]byte(res.Out), &resp); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if resp["purged_jobs"] != 7 {
		t.Fatalf("expected purged_jobs=7, got %v", resp)
	}
}

func TestJobsPurgeZeroOlderThanSendsZero(t *testing.T) {
	var gotBody map[string]int
	apitest.StubClient(t, &newHTTPClient, apitest.RoundTripFunc(func(r *http.Request) (*http.Response, error) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		return apitest.JSONResponse(http.StatusOK, `{"purged_jobs":0}`), nil
	}))

	res := clitest.Run(t, run, "--server", "http://example.test", "jobs", "purge")
	res.RequireCode(t, 0)
	if gotBody["older_than_days"] != 0 {
		t.Fatalf("expected older_than_days=0, got %v", gotBody)
	}
}

func TestJobsPurgeInUsage(t *testing.T) {
	res := clitest.Run(t, run, "help")
	combined := res.Out + res.Err
	if !strings.Contains(combined, "purge") {
		t.Fatalf("expected 'purge' in usage output, got:\n%s", combined)
	}
}
