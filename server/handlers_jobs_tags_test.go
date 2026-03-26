package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// --- POST /api/v1/jobs with tags ---------------------------------------------

func TestCreateJobWithTags(t *testing.T) {
	srv := New(DefaultConfig())
	createTag(t, srv, "tld", "")

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/jobs",
		bytes.NewBufferString(`{"domain":"example.com","tags":["tld"]}`))
	req.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", resp.Code, resp.Body)
	}

	// Domain should be tagged.
	d, ok := srv.store.GetDomainByName("example.com")
	if !ok {
		t.Fatal("expected domain to exist")
	}
	tags := srv.store.GetDomainTags(d.ID)
	if len(tags) != 1 || tags[0] != "tld" {
		t.Fatalf("expected domain tagged with tld, got %v", tags)
	}
}

func TestCreateJobWithUnknownTag(t *testing.T) {
	srv := New(DefaultConfig())

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/jobs",
		bytes.NewBufferString(`{"domain":"example.com","tags":["ghost"]}`))
	req.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.Code)
	}
}

func TestCreateJobNoTagsUnchanged(t *testing.T) {
	// Submitting a job without tags should still work as before.
	srv := New(DefaultConfig())

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/jobs",
		bytes.NewBufferString(`{"domain":"example.com"}`))
	req.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", resp.Code, resp.Body)
	}
}

// --- POST /api/v1/jobs/batch with tags ---------------------------------------

func TestBatchJobWithTags(t *testing.T) {
	srv := New(DefaultConfig())
	createTag(t, srv, "tld", "")

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/jobs/batch",
		bytes.NewBufferString(`{"domains":["example.com","example.net"],"tags":["tld"]}`))
	req.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", resp.Code, resp.Body)
	}

	// Both domains should be tagged.
	tag, ok := srv.store.GetTag("tld")
	if !ok {
		t.Fatal("tag not found")
	}
	if tag.DomainCount != 2 {
		t.Fatalf("expected domain_count=2, got %d", tag.DomainCount)
	}
}

func TestBatchJobWithUnknownTag(t *testing.T) {
	srv := New(DefaultConfig())

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/jobs/batch",
		bytes.NewBufferString(`{"domains":["example.com"],"tags":["ghost"]}`))
	req.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.Code)
	}
}

// --- POST /api/v1/jobs/batch with from_tag -----------------------------------

func TestBatchJobFromTag(t *testing.T) {
	srv := New(DefaultConfig())
	createTag(t, srv, "tld", "")
	d1 := makeGraduatedJob(t, srv, "example.com", JobSucceeded)
	d2 := makeGraduatedJob(t, srv, "example.net", JobSucceeded)
	makeGraduatedJob(t, srv, "other.org", JobSucceeded) // not in tag
	if err := srv.store.TagDomains("tld", []int64{d1.ID, d2.ID}); err != nil {
		t.Fatalf("tag domains: %v", err)
	}

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/jobs/batch",
		bytes.NewBufferString(`{"from_tag":"tld"}`))
	req.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", resp.Code, resp.Body)
	}

	var batchResp JobBatchResponse
	if err := json.NewDecoder(resp.Body).Decode(&batchResp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(batchResp.JobIDs) != 2 {
		t.Fatalf("expected 2 jobs from tag, got %d", len(batchResp.JobIDs))
	}
}

func TestBatchJobFromTagNotFound(t *testing.T) {
	srv := New(DefaultConfig())

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/jobs/batch",
		bytes.NewBufferString(`{"from_tag":"ghost"}`))
	req.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.Code)
	}
}

func TestBatchJobFromTagAndDomainsMutuallyExclusive(t *testing.T) {
	srv := New(DefaultConfig())
	createTag(t, srv, "tld", "")

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/jobs/batch",
		bytes.NewBufferString(`{"from_tag":"tld","domains":["example.com"]}`))
	req.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.Code)
	}
}
