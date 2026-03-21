package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestPublicCreateJobReturnsPublicIDNotUUID(t *testing.T) {
	srv := New(DefaultConfig())

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/pub/api/v1/jobs",
		bytes.NewBufferString(`{"domain":"example.com"}`))
	req.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(resp, req)

	if resp.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", resp.Code, resp.Body.String())
	}
	var view map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&view); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if view["public_id"] == "" || view["public_id"] == nil {
		t.Fatal("expected public_id in response")
	}
	if _, hasID := view["id"]; hasID {
		t.Fatal("internal UUID must not appear in public API response")
	}
}

func TestPublicCreateJobReturnsDomainStatusProgress(t *testing.T) {
	srv := New(DefaultConfig())

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/pub/api/v1/jobs",
		bytes.NewBufferString(`{"domain":"example.com"}`))
	req.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(resp, req)

	var view PublicJobView
	if err := json.NewDecoder(resp.Body).Decode(&view); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if view.Domain != "example.com" {
		t.Fatalf("domain: got %q, want %q", view.Domain, "example.com")
	}
	if view.Status != JobQueued {
		t.Fatalf("status: got %q, want %q", view.Status, JobQueued)
	}
	if view.Progress != 0 {
		t.Fatalf("progress: got %d, want 0", view.Progress)
	}
}

func TestPublicCreateJobMissingDomainReturns400(t *testing.T) {
	srv := New(DefaultConfig())

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/pub/api/v1/jobs",
		bytes.NewBufferString(`{}`))
	req.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(resp, req)

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.Code)
	}
}

func TestPublicGetJobReturnsPublicIDNotUUID(t *testing.T) {
	srv := New(DefaultConfig())

	// Create via public API.
	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/pub/api/v1/jobs",
		bytes.NewBufferString(`{"domain":"example.com"}`))
	req.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(resp, req)
	var created PublicJobView
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatalf("decode create: %v", err)
	}

	// Get via public ID.
	resp = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/pub/api/v1/jobs/"+created.PublicID, nil)
	srv.Handler().ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.String())
	}
	var got map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode get: %v", err)
	}
	if got["public_id"] != created.PublicID {
		t.Fatalf("public_id: got %v, want %q", got["public_id"], created.PublicID)
	}
	if _, hasID := got["id"]; hasID {
		t.Fatal("internal UUID must not appear in public get response")
	}
}

func TestPublicGetJobUnknownPublicIDReturns404(t *testing.T) {
	srv := New(DefaultConfig())

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/pub/api/v1/jobs/notexist1", nil)
	srv.Handler().ServeHTTP(resp, req)

	if resp.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.Code)
	}
}

func TestPublicGetResultReturnsResultByPublicID(t *testing.T) {
	srv := New(DefaultConfig())

	// Create a job and manually store a result for it.
	job := Job{
		ID:        newID("job"),
		Domain:    "example.com",
		Status:    JobSucceeded,
		CreatedAt: time.Now().UTC(),
		Progress:  100,
	}
	created, err := srv.store.Create(job)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	_ = srv.store.SetResult(created.ID, JobResult{
		JobID:  created.ID,
		Status: JobSucceeded,
	})

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/pub/api/v1/jobs/"+created.PublicID+"/result", nil)
	srv.Handler().ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.String())
	}
	var result JobResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if result.Status != JobSucceeded {
		t.Fatalf("status: got %q, want %q", result.Status, JobSucceeded)
	}
}

func TestPublicGetResultUnknownPublicIDReturns404(t *testing.T) {
	srv := New(DefaultConfig())

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/pub/api/v1/jobs/notexist1/result", nil)
	srv.Handler().ServeHTTP(resp, req)

	if resp.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.Code)
	}
}

func TestPublicLocalesEndpointAccessible(t *testing.T) {
	srv := New(DefaultConfig())

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/pub/api/v1/locales", nil)
	srv.Handler().ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200 from /pub/api/v1/locales, got %d", resp.Code)
	}
}

func TestPublicGetResultNoResultYetReturns404(t *testing.T) {
	srv := New(DefaultConfig())

	// Create a job but do not store a result for it.
	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/pub/api/v1/jobs",
		bytes.NewBufferString(`{"domain":"example.com"}`))
	req.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(resp, req)
	var created PublicJobView
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatalf("decode: %v", err)
	}

	resp = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/pub/api/v1/jobs/"+created.PublicID+"/result", nil)
	srv.Handler().ServeHTTP(resp, req)

	if resp.Code != http.StatusNotFound {
		t.Fatalf("expected 404 when result not yet available, got %d", resp.Code)
	}
}

func TestPublicGetResultRespectsLocaleParam(t *testing.T) {
	srv := New(DefaultConfig())

	job := Job{
		ID:        newID("job"),
		Domain:    "example.com",
		Status:    JobSucceeded,
		CreatedAt: time.Now().UTC(),
		Progress:  100,
	}
	created, _ := srv.store.Create(job)
	_ = srv.store.SetResult(created.ID, JobResult{
		JobID:  created.ID,
		Status: JobSucceeded,
		Raw: &JobResultRaw{
			Entries: []JobResultEntry{
				{Module: "BASIC", Testcase: "basic01", Tag: "BASIC01", Level: "NOTICE"},
			},
		},
	})

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/pub/api/v1/jobs/"+created.PublicID+"/result?locale=sv", nil)
	srv.Handler().ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}
	var result JobResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if result.Raw == nil || result.Raw.Locale != "sv" {
		t.Fatalf("expected locale=sv in result, got %v", result.Raw)
	}
}

func TestPublicAPIJobCreatedViaInternalAPIFetchableByPublicID(t *testing.T) {
	srv := New(DefaultConfig())

	// Create via internal API.
	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/jobs",
		bytes.NewBufferString(`{"domain":"example.com"}`))
	req.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusCreated {
		t.Fatalf("internal create: expected 201, got %d", resp.Code)
	}
	var internalJob Job
	if err := json.NewDecoder(resp.Body).Decode(&internalJob); err != nil {
		t.Fatalf("decode internal job: %v", err)
	}
	if internalJob.PublicID == "" {
		t.Fatal("internal API response must include public_id")
	}

	// Fetch via public API using the public_id from the internal response.
	resp = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/pub/api/v1/jobs/"+internalJob.PublicID, nil)
	srv.Handler().ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("public get: expected 200, got %d", resp.Code)
	}
	var view PublicJobView
	if err := json.NewDecoder(resp.Body).Decode(&view); err != nil {
		t.Fatalf("decode public view: %v", err)
	}
	if view.PublicID != internalJob.PublicID {
		t.Fatalf("public_id mismatch: got %q, want %q", view.PublicID, internalJob.PublicID)
	}
}

func TestPublicAPIEndpointsUnreachableViaInternalPrefix(t *testing.T) {
	srv := New(DefaultConfig())

	// Admin-only endpoints must not be reachable via /pub/
	for _, path := range []string{
		"/pub/api/v1/metrics",
		"/pub/api/v1/queue/pause",
		"/pub/api/v1/batches",
	} {
		resp := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		srv.Handler().ServeHTTP(resp, req)
		if resp.Code == http.StatusOK {
			t.Errorf("path %q should not return 200 via public prefix", path)
		}
	}
}
