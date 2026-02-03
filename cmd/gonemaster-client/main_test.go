package main

import (
	"bytes"
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
