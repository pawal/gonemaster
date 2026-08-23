package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"codeberg.org/pawal/gonemaster/cmd/internal/clitest"
	"codeberg.org/pawal/gonemaster/internal/apitest"
)

// ── jobs create --tag ──────────────────────────────────────────────────────────

func TestJobsCreateWithTag(t *testing.T) {
	var gotReq jobCreateRequest
	apitest.StubClient(t, &newHTTPClient, apitest.CaptureJSON(t, &gotReq, http.StatusCreated,
		`{"id":"job_1","domain":"example.com","status":"queued","created_at":"2026-02-03T00:00:00Z","progress":0}`))
	res := clitest.Run(t, run, "jobs", "create", "--domain", "example.com", "--tag", "tld", "--tag", "test")
	res.RequireCode(t, 0)
	if len(gotReq.Tags) != 2 || gotReq.Tags[0] != "tld" || gotReq.Tags[1] != "test" {
		t.Fatalf("expected tags [tld test], got %v", gotReq.Tags)
	}
}

func TestJobsCreateNoTagOmitsField(t *testing.T) {
	var gotReq jobCreateRequest
	apitest.StubClient(t, &newHTTPClient, apitest.CaptureJSON(t, &gotReq, http.StatusCreated,
		`{"id":"job_1","domain":"example.com","status":"queued","created_at":"2026-02-03T00:00:00Z","progress":0}`))
	res := clitest.Run(t, run, "jobs", "create", "--domain", "example.com")
	res.RequireCode(t, 0)
	if len(gotReq.Tags) != 0 {
		t.Fatalf("expected no tags, got %v", gotReq.Tags)
	}
}

// ── jobs batch --tag / --from-tag ─────────────────────────────────────────────

func TestJobsBatchWithTag(t *testing.T) {
	var gotReq jobBatchRequest
	apitest.StubClient(t, &newHTTPClient, apitest.CaptureJSON(t, &gotReq, http.StatusAccepted,
		`{"batch_id":"batch_1","job_ids":["job_1","job_2"]}`))
	res := clitest.Run(t, run, "jobs", "batch", "--domain", "example.com", "--domain", "example.net", "--tag", "tld")
	res.RequireCode(t, 0)
	if len(gotReq.Tags) != 1 || gotReq.Tags[0] != "tld" {
		t.Fatalf("expected tags [tld], got %v", gotReq.Tags)
	}
	if len(gotReq.Domains) != 2 {
		t.Fatalf("expected 2 domains, got %v", gotReq.Domains)
	}
}

func TestJobsBatchWithFromTag(t *testing.T) {
	var gotReq jobBatchRequest
	apitest.StubClient(t, &newHTTPClient, apitest.CaptureJSON(t, &gotReq, http.StatusAccepted,
		`{"batch_id":"batch_1","job_ids":["job_1","job_2"]}`))
	res := clitest.Run(t, run, "jobs", "batch", "--from-tag", "tld")
	res.RequireCode(t, 0)
	if gotReq.FromTag != "tld" {
		t.Fatalf("expected from_tag=tld, got %q", gotReq.FromTag)
	}
	if len(gotReq.Domains) != 0 {
		t.Fatalf("expected no domains, got %v", gotReq.Domains)
	}
}

func TestJobsBatchFromTagAndDomainMutuallyExclusive(t *testing.T) {
	apitest.StubClient(t, &newHTTPClient, apitest.CaptureJSON(t, nil, http.StatusOK, `{}`))
	res := clitest.Run(t, run, "jobs", "batch", "--from-tag", "tld", "--domain", "example.com")
	if res.Code == 0 {
		t.Fatal("expected non-zero exit when --from-tag and --domain are both provided")
	}
	res.RequireErrContains(t, "mutually exclusive")
}

func TestJobsBatchNoDomainAndNoFromTag(t *testing.T) {
	apitest.StubClient(t, &newHTTPClient, apitest.CaptureJSON(t, nil, http.StatusOK, `{}`))
	res := clitest.Run(t, run, "jobs", "batch")
	if res.Code == 0 {
		t.Fatal("expected non-zero exit when no domains provided")
	}
}

// ── jobs list: in-flight note ──────────────────────────────────────────────────

func TestJobsListHelpContainsInFlightNote(t *testing.T) {
	// Trigger usage via --help.
	res := clitest.Run(t, run, "jobs", "list", "--help")
	combined := res.Out + res.Err
	if !strings.Contains(combined, "in-flight") {
		t.Fatalf("expected 'in-flight' in jobs list help, got:\n%s", combined)
	}
	if !strings.Contains(combined, "runs list") {
		t.Fatalf("expected 'runs list' reference in jobs list help, got:\n%s", combined)
	}
}

// ── jobs get: fallback to run ──────────────────────────────────────────────────

func TestJobsGetFallsBackToRun(t *testing.T) {
	apitest.StubClient(t, &newHTTPClient, apitest.RoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/api/v1/jobs/run_1" {
			return apitest.JSONResponse(http.StatusNotFound,
				`{"error":{"code":"not_found","message":"not found"}}`), nil
		}
		if r.URL.Path == "/api/v1/runs/run_1" {
			body, _ := json.Marshal(runRecord{
				ID:     "run_1",
				Domain: "example.com",
				Status: "succeeded",
			})
			return apitest.JSONResponse(http.StatusOK, string(body)), nil
		}
		t.Fatalf("unexpected path: %s", r.URL.Path)
		return nil, nil
	}))
	res := clitest.Run(t, run, "--format", "json", "jobs", "get", "run_1")
	res.RequireCode(t, 0)
	res.RequireOutContains(t, "example.com")
}

// ── jobs results: fallback to run result ──────────────────────────────────────

func TestJobsResultsFallsBackToRunResult(t *testing.T) {
	apitest.StubClient(t, &newHTTPClient, apitest.RoundTripFunc(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/api/v1/jobs/run_1":
			// Return 404 for job lookup; fetchJobInfo falls back to runs.
			return apitest.JSONResponse(http.StatusNotFound,
				`{"error":{"code":"not_found","message":"not found"}}`), nil
		case "/api/v1/runs/run_1":
			body, _ := json.Marshal(runRecord{
				ID:     "run_1",
				Domain: "example.com",
				Status: "succeeded",
			})
			return apitest.JSONResponse(http.StatusOK, string(body)), nil
		case "/api/v1/jobs/run_1/result":
			return apitest.JSONResponse(http.StatusNotFound,
				`{"error":{"code":"not_found","message":"not found"}}`), nil
		case "/api/v1/runs/run_1/result":
			body := `{"job_id":"run_1","entries":[]}`
			return apitest.JSONResponse(http.StatusOK, body), nil
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
			return nil, nil
		}
	}))
	res := clitest.Run(t, run, "--format", "json", "jobs", "results", "run_1")
	res.RequireCode(t, 0)
}
