package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

// ── jobs create --tag ──────────────────────────────────────────────────────────

func TestJobsCreateWithTag(t *testing.T) {
	var gotReq jobCreateRequest
	old := newHTTPClient
	defer func() { newHTTPClient = old }()
	newHTTPClient = func(_ time.Duration) *http.Client {
		return &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if err := json.NewDecoder(r.Body).Decode(&gotReq); err != nil {
				t.Fatalf("decode request: %v", err)
			}
			body := `{"id":"job_1","domain":"example.com","status":"queued","created_at":"2026-02-03T00:00:00Z","progress":0}`
			return &http.Response{
				StatusCode: http.StatusCreated,
				Body:       io.NopCloser(bytes.NewBufferString(body)),
				Header:     http.Header{"Content-Type": []string{"application/json"}},
			}, nil
		})}
	}
	var out, errOut bytes.Buffer
	code := run([]string{"jobs", "create", "--domain", "example.com", "--tag", "tld", "--tag", "test"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d: %s", code, errOut.String())
	}
	if len(gotReq.Tags) != 2 || gotReq.Tags[0] != "tld" || gotReq.Tags[1] != "test" {
		t.Fatalf("expected tags [tld test], got %v", gotReq.Tags)
	}
}

func TestJobsCreateNoTagOmitsField(t *testing.T) {
	var gotReq jobCreateRequest
	old := newHTTPClient
	defer func() { newHTTPClient = old }()
	newHTTPClient = func(_ time.Duration) *http.Client {
		return &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if err := json.NewDecoder(r.Body).Decode(&gotReq); err != nil {
				t.Fatalf("decode request: %v", err)
			}
			body := `{"id":"job_1","domain":"example.com","status":"queued","created_at":"2026-02-03T00:00:00Z","progress":0}`
			return &http.Response{
				StatusCode: http.StatusCreated,
				Body:       io.NopCloser(bytes.NewBufferString(body)),
				Header:     http.Header{"Content-Type": []string{"application/json"}},
			}, nil
		})}
	}
	var out, errOut bytes.Buffer
	code := run([]string{"jobs", "create", "--domain", "example.com"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d: %s", code, errOut.String())
	}
	if len(gotReq.Tags) != 0 {
		t.Fatalf("expected no tags, got %v", gotReq.Tags)
	}
}

// ── jobs batch --tag / --from-tag ─────────────────────────────────────────────

func TestJobsBatchWithTag(t *testing.T) {
	var gotReq jobBatchRequest
	old := newHTTPClient
	defer func() { newHTTPClient = old }()
	newHTTPClient = func(_ time.Duration) *http.Client {
		return &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if err := json.NewDecoder(r.Body).Decode(&gotReq); err != nil {
				t.Fatalf("decode request: %v", err)
			}
			body := `{"batch_id":"batch_1","job_ids":["job_1","job_2"]}`
			return jsonResponse(http.StatusAccepted, body), nil
		})}
	}
	var out, errOut bytes.Buffer
	code := run([]string{"jobs", "batch", "--domain", "example.com", "--domain", "example.net",
		"--tag", "tld"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d: %s", code, errOut.String())
	}
	if len(gotReq.Tags) != 1 || gotReq.Tags[0] != "tld" {
		t.Fatalf("expected tags [tld], got %v", gotReq.Tags)
	}
	if len(gotReq.Domains) != 2 {
		t.Fatalf("expected 2 domains, got %v", gotReq.Domains)
	}
}

func TestJobsBatchWithFromTag(t *testing.T) {
	var gotReq jobBatchRequest
	old := newHTTPClient
	defer func() { newHTTPClient = old }()
	newHTTPClient = func(_ time.Duration) *http.Client {
		return &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if err := json.NewDecoder(r.Body).Decode(&gotReq); err != nil {
				t.Fatalf("decode request: %v", err)
			}
			body := `{"batch_id":"batch_1","job_ids":["job_1","job_2"]}`
			return jsonResponse(http.StatusAccepted, body), nil
		})}
	}
	var out, errOut bytes.Buffer
	code := run([]string{"jobs", "batch", "--from-tag", "tld"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d: %s", code, errOut.String())
	}
	if gotReq.FromTag != "tld" {
		t.Fatalf("expected from_tag=tld, got %q", gotReq.FromTag)
	}
	if len(gotReq.Domains) != 0 {
		t.Fatalf("expected no domains, got %v", gotReq.Domains)
	}
}

func TestJobsBatchFromTagAndDomainMutuallyExclusive(t *testing.T) {
	old := newHTTPClient
	defer func() { newHTTPClient = old }()
	newHTTPClient = func(_ time.Duration) *http.Client {
		return &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
			return jsonResponse(200, `{}`), nil
		})}
	}
	var out, errOut bytes.Buffer
	code := run([]string{"jobs", "batch", "--from-tag", "tld", "--domain", "example.com"}, &out, &errOut)
	if code == 0 {
		t.Fatal("expected non-zero exit when --from-tag and --domain are both provided")
	}
	if !strings.Contains(errOut.String(), "mutually exclusive") {
		t.Fatalf("expected 'mutually exclusive' in error: %s", errOut.String())
	}
}

func TestJobsBatchNoDomainAndNoFromTag(t *testing.T) {
	old := newHTTPClient
	defer func() { newHTTPClient = old }()
	newHTTPClient = func(_ time.Duration) *http.Client {
		return &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
			return jsonResponse(200, `{}`), nil
		})}
	}
	var out, errOut bytes.Buffer
	code := run([]string{"jobs", "batch"}, &out, &errOut)
	if code == 0 {
		t.Fatal("expected non-zero exit when no domains provided")
	}
}

// ── jobs list: in-flight note ──────────────────────────────────────────────────

func TestJobsListHelpContainsInFlightNote(t *testing.T) {
	var out, errOut bytes.Buffer
	// Trigger usage via --help.
	run([]string{"jobs", "list", "--help"}, &out, &errOut)
	combined := out.String() + errOut.String()
	if !strings.Contains(combined, "in-flight") {
		t.Fatalf("expected 'in-flight' in jobs list help, got:\n%s", combined)
	}
	if !strings.Contains(combined, "runs list") {
		t.Fatalf("expected 'runs list' reference in jobs list help, got:\n%s", combined)
	}
}

// ── jobs get: fallback to run ──────────────────────────────────────────────────

func TestJobsGetFallsBackToRun(t *testing.T) {
	old := newHTTPClient
	defer func() { newHTTPClient = old }()
	newHTTPClient = func(_ time.Duration) *http.Client {
		return &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if r.URL.Path == "/api/v1/jobs/run_1" {
				return jsonResponse(http.StatusNotFound,
					`{"error":{"code":"not_found","message":"not found"}}`), nil
			}
			if r.URL.Path == "/api/v1/runs/run_1" {
				body, _ := json.Marshal(runRecord{
					ID:     "run_1",
					Domain: "example.com",
					Status: "succeeded",
				})
				return jsonResponse(http.StatusOK, string(body)), nil
			}
			t.Fatalf("unexpected path: %s", r.URL.Path)
			return nil, nil
		})}
	}
	var out, errOut bytes.Buffer
	code := run([]string{"--format", "json", "jobs", "get", "run_1"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d: %s", code, errOut.String())
	}
	if !strings.Contains(out.String(), "example.com") {
		t.Fatalf("expected domain in output: %s", out.String())
	}
}

// ── jobs results: fallback to run result ──────────────────────────────────────

func TestJobsResultsFallsBackToRunResult(t *testing.T) {
	old := newHTTPClient
	defer func() { newHTTPClient = old }()
	newHTTPClient = func(_ time.Duration) *http.Client {
		return &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			switch r.URL.Path {
			case "/api/v1/jobs/run_1":
				// Return 404 for job lookup; fetchJobInfo falls back to runs.
				return jsonResponse(http.StatusNotFound,
					`{"error":{"code":"not_found","message":"not found"}}`), nil
			case "/api/v1/runs/run_1":
				body, _ := json.Marshal(runRecord{
					ID:     "run_1",
					Domain: "example.com",
					Status: "succeeded",
				})
				return jsonResponse(http.StatusOK, string(body)), nil
			case "/api/v1/jobs/run_1/result":
				return jsonResponse(http.StatusNotFound,
					`{"error":{"code":"not_found","message":"not found"}}`), nil
			case "/api/v1/runs/run_1/result":
				body := `{"job_id":"run_1","entries":[]}`
				return jsonResponse(http.StatusOK, body), nil
			default:
				t.Fatalf("unexpected path: %s", r.URL.Path)
				return nil, nil
			}
		})}
	}
	var out, errOut bytes.Buffer
	code := run([]string{"--format", "json", "jobs", "results", "run_1"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d: %s", code, errOut.String())
	}
}
