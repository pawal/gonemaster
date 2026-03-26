package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

// --- GET /api/v1/runs --------------------------------------------------------

func TestListRunsEmpty(t *testing.T) {
	srv := New(DefaultConfig())
	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/runs", nil)
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}
	var list RunList
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if list.Total != 0 {
		t.Fatalf("expected total=0, got %d", list.Total)
	}
}

func TestListRunsReturnsRuns(t *testing.T) {
	srv := New(DefaultConfig())
	makeGraduatedJob(t, srv, "alpha.com", JobSucceeded)
	makeGraduatedJob(t, srv, "beta.com", JobSucceeded)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/runs", nil)
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}
	var list RunList
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if list.Total != 2 {
		t.Fatalf("expected total=2, got %d", list.Total)
	}
}

func TestListRunsFilterByDomain(t *testing.T) {
	srv := New(DefaultConfig())
	makeGraduatedJob(t, srv, "alpha.com", JobSucceeded)
	makeGraduatedJob(t, srv, "beta.com", JobSucceeded)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/runs?domain=alpha", nil)
	srv.Handler().ServeHTTP(resp, req)
	var list RunList
	_ = json.NewDecoder(resp.Body).Decode(&list)
	if list.Total != 1 {
		t.Fatalf("expected total=1, got %d", list.Total)
	}
	if list.Items[0].Domain != "alpha.com" {
		t.Fatalf("expected domain=alpha.com, got %q", list.Items[0].Domain)
	}
}

func TestListRunsFilterByTag(t *testing.T) {
	srv := New(DefaultConfig())
	createTag(t, srv, "tld", "")
	d := makeGraduatedJob(t, srv, "example.com", JobSucceeded)
	makeGraduatedJob(t, srv, "other.org", JobSucceeded)
	if err := srv.store.TagDomains("tld", []int64{d.ID}); err != nil {
		t.Fatalf("tag domain: %v", err)
	}

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/runs?tag=tld", nil)
	srv.Handler().ServeHTTP(resp, req)
	var list RunList
	_ = json.NewDecoder(resp.Body).Decode(&list)
	if list.Total != 1 {
		t.Fatalf("expected total=1, got %d", list.Total)
	}
}

func TestListRunsInvalidLimit(t *testing.T) {
	srv := New(DefaultConfig())
	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/runs?limit=bad", nil)
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.Code)
	}
}

func TestListRunsInvalidTimeRange(t *testing.T) {
	srv := New(DefaultConfig())
	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/runs?finished_after=2026-01-02T00:00:00Z&finished_before=2026-01-01T00:00:00Z", nil)
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.Code)
	}
}

// --- GET /api/v1/runs/{id} ---------------------------------------------------

func TestGetRun(t *testing.T) {
	srv := New(DefaultConfig())
	d := makeGraduatedJob(t, srv, "example.com", JobSucceeded)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/runs/"+d.LatestRunID, nil)
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body)
	}
	var run Run
	if err := json.NewDecoder(resp.Body).Decode(&run); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if run.Domain != "example.com" {
		t.Fatalf("expected domain=example.com, got %q", run.Domain)
	}
}

func TestGetRunNotFound(t *testing.T) {
	srv := New(DefaultConfig())
	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/runs/no-such-run", nil)
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.Code)
	}
}

// --- GET /api/v1/runs/{id}/result --------------------------------------------

func TestGetRunResult(t *testing.T) {
	srv := New(DefaultConfig())
	d := makeGraduatedJob(t, srv, "example.com", JobSucceeded)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/runs/%s/result", d.LatestRunID), nil)
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body)
	}
	var result JobResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if result.Status != JobSucceeded {
		t.Fatalf("expected status=succeeded, got %q", result.Status)
	}
}

func TestGetRunResultNotFound(t *testing.T) {
	srv := New(DefaultConfig())
	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/runs/no-such-run/result", nil)
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.Code)
	}
}
