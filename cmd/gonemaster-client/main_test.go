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

	"codeberg.org/pawal/gonemaster/engine/normalization"
)

func TestNormalizeBaseURL(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"http://localhost:8080", "http://localhost:8080/api/v1"},
		{"http://localhost:8080/api/v1", "http://localhost:8080/api/v1"},
		{"localhost:8080", "http://localhost:8080/api/v1"},
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
	var out bytes.Buffer
	var errOut bytes.Buffer

	code := run([]string{"--version"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
	if !strings.Contains(out.String(), "Gonemaster version") {
		t.Fatalf("expected gonemaster version output, got %q", out.String())
	}
	if !strings.Contains(out.String(), "Miekg DNS version") {
		t.Fatalf("expected miekg version output, got %q", out.String())
	}
	if errOut.Len() != 0 {
		t.Fatalf("expected no stderr output, got %q", errOut.String())
	}
}

func TestJobsCreateSendsNormalizedDomain(t *testing.T) {
	var gotDomain string
	oldFactory := newHTTPClient
	defer func() { newHTTPClient = oldFactory }()
	newHTTPClient = func(_ time.Duration) *http.Client {
		return &http.Client{
			Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
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
			}),
		}
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	code := run([]string{"--server", "http://example.test", "--format", "json", "jobs", "create", "--domain", "räksmörgås.se"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("run returned %d, stderr=%s", code, errOut.String())
	}
	errs, normalized := normalization.NormalizeName("räksmörgås.se")
	if len(errs) != 0 {
		t.Fatalf("normalization errors: %v", errs)
	}
	if gotDomain != normalized {
		t.Fatalf("expected domain %q, got %q", normalized, gotDomain)
	}
}

func TestBatchesCancelCallsCancelForQueuedAndRunning(t *testing.T) {
	var canceled []string
	oldFactory := newHTTPClient
	defer func() { newHTTPClient = oldFactory }()
	newHTTPClient = func(_ time.Duration) *http.Client {
		return &http.Client{
			Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				switch {
				case r.Method == http.MethodGet && r.URL.Path == "/api/v1/batches/batch_1":
					body := `{
  "batch_id":"batch_1",
  "total":3,
  "status_counts":{"queued":1,"running":1,"succeeded":1},
  "items":[
    {"id":"job_q","domain":"example.com","status":"queued","created_at":"2026-02-03T00:00:00Z","progress":0},
    {"id":"job_r","domain":"example.net","status":"running","created_at":"2026-02-03T00:00:00Z","progress":50},
    {"id":"job_s","domain":"example.org","status":"succeeded","created_at":"2026-02-03T00:00:00Z","progress":100}
  ],
  "created_at":"2026-02-03T00:00:00Z"
}`
					return jsonResponse(http.StatusOK, body), nil
				case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/cancel"):
					parts := strings.Split(r.URL.Path, "/")
					if len(parts) < 5 {
						t.Fatalf("unexpected cancel path %s", r.URL.Path)
					}
					jobID := parts[len(parts)-2]
					canceled = append(canceled, jobID)
					body := fmt.Sprintf(`{"id":"%s","domain":"example.com","status":"canceled","created_at":"2026-02-03T00:00:00Z","progress":100}`, jobID)
					return jsonResponse(http.StatusOK, body), nil
				default:
					t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
				}
				return nil, nil
			}),
		}
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	code := run([]string{"--server", "http://example.test", "--format", "json", "batches", "cancel", "batch_1"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("run returned %d, stderr=%s", code, errOut.String())
	}
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
	oldFactory := newHTTPClient
	defer func() { newHTTPClient = oldFactory }()
	newHTTPClient = func(_ time.Duration) *http.Client {
		return &http.Client{
			Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				switch {
				case r.Method == http.MethodGet && r.URL.Path == "/api/v1/batches/batch_2":
					body := `{
  "batch_id":"batch_2",
  "total":2,
  "status_counts":{"queued":1,"running":1},
  "items":[
    {"id":"job_q","domain":"example.com","status":"queued","created_at":"2026-02-03T00:00:00Z","progress":0},
    {"id":"job_r","domain":"example.net","status":"running","created_at":"2026-02-03T00:00:00Z","progress":50}
  ],
  "created_at":"2026-02-03T00:00:00Z"
}`
					return jsonResponse(http.StatusOK, body), nil
				case r.Method == http.MethodPost && r.URL.Path == "/api/v1/queue/remove":
					var req queueRemoveRequest
					if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
						t.Fatalf("decode queue remove: %v", err)
					}
					removed = append(removed, req.JobIDs...)
					body := `{"removed":["job_q"]}`
					return jsonResponse(http.StatusOK, body), nil
				default:
					t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
				}
				return nil, nil
			}),
		}
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	code := run([]string{"--server", "http://example.test", "--format", "json", "batches", "remove", "batch_2"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("run returned %d, stderr=%s", code, errOut.String())
	}
	if len(removed) != 1 || removed[0] != "job_q" {
		t.Fatalf("expected queue remove for job_q, got %v", removed)
	}
}

func TestBatchesRemoveCancelRunning(t *testing.T) {
	var removed []string
	var canceled []string
	oldFactory := newHTTPClient
	defer func() { newHTTPClient = oldFactory }()
	newHTTPClient = func(_ time.Duration) *http.Client {
		return &http.Client{
			Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				switch {
				case r.Method == http.MethodGet && r.URL.Path == "/api/v1/batches/batch_3":
					body := `{
  "batch_id":"batch_3",
  "total":3,
  "status_counts":{"queued":1,"running":1,"succeeded":1},
  "items":[
    {"id":"job_q","domain":"example.com","status":"queued","created_at":"2026-02-03T00:00:00Z","progress":0},
    {"id":"job_r","domain":"example.net","status":"running","created_at":"2026-02-03T00:00:00Z","progress":50},
    {"id":"job_s","domain":"example.org","status":"succeeded","created_at":"2026-02-03T00:00:00Z","progress":100}
  ],
  "created_at":"2026-02-03T00:00:00Z"
}`
					return jsonResponse(http.StatusOK, body), nil
				case r.Method == http.MethodPost && r.URL.Path == "/api/v1/queue/remove":
					var req queueRemoveRequest
					if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
						t.Fatalf("decode queue remove: %v", err)
					}
					removed = append(removed, req.JobIDs...)
					body := `{"removed":["job_q"]}`
					return jsonResponse(http.StatusOK, body), nil
				case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/cancel"):
					parts := strings.Split(r.URL.Path, "/")
					jobID := parts[len(parts)-2]
					canceled = append(canceled, jobID)
					body := fmt.Sprintf(`{"id":"%s","domain":"example.com","status":"canceled","created_at":"2026-02-03T00:00:00Z","progress":100}`, jobID)
					return jsonResponse(http.StatusOK, body), nil
				default:
					t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
				}
				return nil, nil
			}),
		}
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	code := run([]string{"--server", "http://example.test", "--format", "json", "batches", "remove", "--cancel-running", "batch_3"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("run returned %d, stderr=%s", code, errOut.String())
	}
	if len(removed) != 1 || removed[0] != "job_q" {
		t.Fatalf("expected queue remove for job_q, got %v", removed)
	}
	if len(canceled) != 1 || canceled[0] != "job_r" {
		t.Fatalf("expected cancel for job_r, got %v", canceled)
	}
}

func TestFetchBatchSummaryPaginatesItems(t *testing.T) {
	var pageCalls int
	oldFactory := newHTTPClient
	defer func() { newHTTPClient = oldFactory }()
	newHTTPClient = func(_ time.Duration) *http.Client {
		return &http.Client{
			Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				switch {
				case r.Method == http.MethodGet && r.URL.Path == "/api/v1/batches/batch_paginated" && r.URL.RawQuery == "":
					body := `{
  "batch_id":"batch_paginated",
  "total":3,
  "status_counts":{"queued":2,"running":1},
  "items":[
    {"id":"job_q1","domain":"example.com","status":"queued","created_at":"2026-02-03T00:00:00Z","progress":0},
    {"id":"job_r1","domain":"example.net","status":"running","created_at":"2026-02-03T00:00:00Z","progress":50}
  ],
  "created_at":"2026-02-03T00:00:00Z",
  "limit":2,
  "offset":0,
  "next_cursor":"2"
}`
					return jsonResponse(http.StatusOK, body), nil
				case r.Method == http.MethodGet && r.URL.Path == "/api/v1/batches/batch_paginated":
					if r.URL.Query().Get("offset") != "2" {
						t.Fatalf("expected second request offset=2, got %q", r.URL.Query().Get("offset"))
					}
					pageCalls++
					body := `{
  "batch_id":"batch_paginated",
  "total":3,
  "status_counts":{"queued":2,"running":1},
  "items":[
    {"id":"job_q2","domain":"example.org","status":"queued","created_at":"2026-02-03T00:00:00Z","progress":0}
  ],
  "created_at":"2026-02-03T00:00:00Z",
  "limit":500,
  "offset":2
}`
					return jsonResponse(http.StatusOK, body), nil
				default:
					t.Fatalf("unexpected request %s %s?%s", r.Method, r.URL.Path, r.URL.RawQuery)
				}
				return nil, nil
			}),
		}
	}

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
	oldFactory := newHTTPClient
	defer func() { newHTTPClient = oldFactory }()
	newHTTPClient = func(_ time.Duration) *http.Client {
		return &http.Client{
			Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				switch {
				case r.Method == http.MethodGet && r.URL.Path == "/api/v1/batches/batch_4" && r.URL.RawQuery == "":
					body := `{
  "batch_id":"batch_4",
  "total":3,
  "status_counts":{"queued":2,"running":1},
  "items":[
    {"id":"job_q1","domain":"example.com","status":"queued","created_at":"2026-02-03T00:00:00Z","progress":0},
    {"id":"job_r1","domain":"example.net","status":"running","created_at":"2026-02-03T00:00:00Z","progress":50}
  ],
  "created_at":"2026-02-03T00:00:00Z",
  "limit":2,
  "offset":0,
  "next_cursor":"2"
}`
					return jsonResponse(http.StatusOK, body), nil
				case r.Method == http.MethodGet && r.URL.Path == "/api/v1/batches/batch_4":
					if r.URL.Query().Get("offset") != "2" {
						t.Fatalf("expected second request offset=2, got %q", r.URL.Query().Get("offset"))
					}
					body := `{
  "batch_id":"batch_4",
  "total":3,
  "status_counts":{"queued":2,"running":1},
  "items":[
    {"id":"job_q2","domain":"example.org","status":"queued","created_at":"2026-02-03T00:00:00Z","progress":0}
  ],
  "created_at":"2026-02-03T00:00:00Z",
  "limit":500,
  "offset":2
}`
					return jsonResponse(http.StatusOK, body), nil
				case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/cancel"):
					parts := strings.Split(r.URL.Path, "/")
					jobID := parts[len(parts)-2]
					canceled = append(canceled, jobID)
					body := fmt.Sprintf(`{"id":"%s","domain":"example.com","status":"canceled","created_at":"2026-02-03T00:00:00Z","progress":100}`, jobID)
					return jsonResponse(http.StatusOK, body), nil
				default:
					t.Fatalf("unexpected request %s %s?%s", r.Method, r.URL.Path, r.URL.RawQuery)
				}
				return nil, nil
			}),
		}
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	code := run([]string{"--server", "http://example.test", "--format", "json", "batches", "cancel", "batch_4"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("run returned %d, stderr=%s", code, errOut.String())
	}
	if len(canceled) != 3 {
		t.Fatalf("expected 3 canceled jobs from paginated batch, got %d (%v)", len(canceled), canceled)
	}
}

func TestFetchJobResultRetriesOnNotFound(t *testing.T) {
	var resultCalls int
	oldFactory := newHTTPClient
	defer func() { newHTTPClient = oldFactory }()
	newHTTPClient = func(_ time.Duration) *http.Client {
		return &http.Client{
			Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				switch {
				case r.Method == http.MethodGet && r.URL.Path == "/api/v1/jobs/job_1":
					body := `{"id":"job_1","domain":"example.com","status":"succeeded","created_at":"2026-02-03T00:00:00Z","progress":100}`
					return jsonResponse(http.StatusOK, body), nil
				case r.Method == http.MethodGet && r.URL.Path == "/api/v1/jobs/job_1/result":
					resultCalls++
					if resultCalls == 1 {
						body := `{"error":{"code":"not_found","message":"job result not found"}}`
						return jsonResponse(http.StatusNotFound, body), nil
					}
					body := `{"job_id":"job_1","status":"succeeded","summary":{"levels":{"NOTICE":1},"total":1}}`
					return jsonResponse(http.StatusOK, body), nil
				default:
					t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
				}
				return nil, nil
			}),
		}
	}

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
	oldFactory := newHTTPClient
	defer func() { newHTTPClient = oldFactory }()
	newHTTPClient = func(_ time.Duration) *http.Client {
		return &http.Client{
			Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				switch {
				case r.Method == http.MethodGet && r.URL.Path == "/api/v1/jobs/job_1":
					body := `{"id":"job_1","domain":"example.com","status":"succeeded","created_at":"2026-02-03T00:00:00Z","progress":100}`
					return jsonResponse(http.StatusOK, body), nil
				case r.Method == http.MethodGet && r.URL.Path == "/api/v1/jobs/job_1/result":
					body := `{"job_id":"job_1","status":"succeeded","summary":{"levels":{"NOTICE":1},"total":1}}`
					return jsonResponse(http.StatusOK, body), nil
				default:
					t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
				}
				return nil, nil
			}),
		}
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	code := run([]string{"--server", "http://example.test", "--format", "json", "jobs", "results", "job_1", "--view", "translated"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("run returned %d, stderr=%s", code, errOut.String())
	}
}

func TestJobsPurgeSendsPostAndPrintsPretty(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody map[string]int
	oldFactory := newHTTPClient
	defer func() { newHTTPClient = oldFactory }()
	newHTTPClient = func(_ time.Duration) *http.Client {
		return &http.Client{
			Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				gotMethod = r.Method
				gotPath = r.URL.Path
				_ = json.NewDecoder(r.Body).Decode(&gotBody)
				return jsonResponse(http.StatusOK, `{"purged_jobs":42}`), nil
			}),
		}
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	code := run([]string{"--server", "http://example.test", "jobs", "purge", "--older-than", "90"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("run returned %d, stderr=%s", code, errOut.String())
	}
	if gotMethod != http.MethodPost {
		t.Fatalf("expected POST, got %s", gotMethod)
	}
	if gotPath != "/api/v1/jobs/purge" {
		t.Fatalf("expected /api/v1/jobs/purge, got %s", gotPath)
	}
	if gotBody["older_than_days"] != 90 {
		t.Fatalf("expected older_than_days=90, got %v", gotBody)
	}
	if !strings.Contains(out.String(), "42") {
		t.Fatalf("expected 42 in output, got %q", out.String())
	}
}

func TestJobsPurgeJSONOutput(t *testing.T) {
	oldFactory := newHTTPClient
	defer func() { newHTTPClient = oldFactory }()
	newHTTPClient = func(_ time.Duration) *http.Client {
		return &http.Client{
			Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
				return jsonResponse(http.StatusOK, `{"purged_jobs":7}`), nil
			}),
		}
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	code := run([]string{"--server", "http://example.test", "--format", "json", "jobs", "purge", "--older-than", "30"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("run returned %d, stderr=%s", code, errOut.String())
	}
	var resp map[string]int64
	if err := json.NewDecoder(&out).Decode(&resp); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if resp["purged_jobs"] != 7 {
		t.Fatalf("expected purged_jobs=7, got %v", resp)
	}
}

func TestJobsPurgeZeroOlderThanSendsZero(t *testing.T) {
	var gotBody map[string]int
	oldFactory := newHTTPClient
	defer func() { newHTTPClient = oldFactory }()
	newHTTPClient = func(_ time.Duration) *http.Client {
		return &http.Client{
			Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				_ = json.NewDecoder(r.Body).Decode(&gotBody)
				return jsonResponse(http.StatusOK, `{"purged_jobs":0}`), nil
			}),
		}
	}

	var out, errOut bytes.Buffer
	code := run([]string{"--server", "http://example.test", "jobs", "purge"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("run returned %d, stderr=%s", code, errOut.String())
	}
	if gotBody["older_than_days"] != 0 {
		t.Fatalf("expected older_than_days=0, got %v", gotBody)
	}
}

func TestJobsPurgeInUsage(t *testing.T) {
	var out, errOut bytes.Buffer
	run([]string{"help"}, &out, &errOut)
	combined := out.String() + errOut.String()
	if !strings.Contains(combined, "purge") {
		t.Fatalf("expected 'purge' in usage output, got:\n%s", combined)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return fn(r)
}

func jsonResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(bytes.NewBufferString(body)),
		Header:     http.Header{"Content-Type": []string{"application/json"}},
	}
}
