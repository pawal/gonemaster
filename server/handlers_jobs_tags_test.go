package server

import (
	"bytes"
	"encoding/json"
	"fmt"
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

func TestCreateJobWithProfileID(t *testing.T) {
	srv := New(DefaultConfig())
	profile := createProfile(t, srv, `{"name":"strict","config":{"net":{"ipv4":true}}}`)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/jobs",
		bytes.NewBufferString(fmt.Sprintf(`{"domain":"example.com","profile_id":%d}`, profile.ID)))
	req.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", resp.Code, resp.Body)
	}

	var created Job
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if created.ProfileID == nil || *created.ProfileID != profile.ID {
		t.Fatalf("ProfileID: got %v, want %d", created.ProfileID, profile.ID)
	}
	if created.ProfileName != "strict" {
		t.Fatalf("ProfileName: got %q", created.ProfileName)
	}

	stored, ok := srv.store.Get(created.ID)
	if !ok {
		t.Fatal("expected stored job")
	}
	if stored.ProfileID == nil || *stored.ProfileID != profile.ID {
		t.Fatalf("stored ProfileID: got %v, want %d", stored.ProfileID, profile.ID)
	}
	if stored.ProfileName != "strict" {
		t.Fatalf("stored ProfileName: got %q", stored.ProfileName)
	}
}

func TestCreateJobUsesTagDefaultProfile(t *testing.T) {
	srv := New(DefaultConfig())
	createTag(t, srv, "tld", "")
	profile := createProfile(t, srv, `{"name":"strict","config":{"net":{"ipv4":true}}}`)
	if err := srv.store.SetTagDefaultProfile("tld", &profile.ID); err != nil {
		t.Fatalf("SetTagDefaultProfile: %v", err)
	}

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/jobs",
		bytes.NewBufferString(`{"domain":"example.com","tags":["tld"]}`))
	req.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", resp.Code, resp.Body)
	}

	var created Job
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if created.ProfileID == nil || *created.ProfileID != profile.ID {
		t.Fatalf("ProfileID: got %v, want %d", created.ProfileID, profile.ID)
	}
	if created.ProfileName != "strict" {
		t.Fatalf("ProfileName: got %q", created.ProfileName)
	}
}

func TestCreateJobDoesNotUseTagDefaultWhenOverridesProvided(t *testing.T) {
	srv := New(DefaultConfig())
	createTag(t, srv, "tld", "")
	profile := createProfile(t, srv, `{"name":"strict","config":{"net":{"ipv4":true}}}`)
	if err := srv.store.SetTagDefaultProfile("tld", &profile.ID); err != nil {
		t.Fatalf("SetTagDefaultProfile: %v", err)
	}

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/jobs",
		bytes.NewBufferString(`{"domain":"example.com","tags":["tld"],"profile_overrides":{"net":{"ipv6":false}}}`))
	req.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", resp.Code, resp.Body)
	}

	var created Job
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if created.ProfileID != nil {
		t.Fatalf("expected no ProfileID when overrides are provided, got %v", *created.ProfileID)
	}
	if created.ProfileName != "" {
		t.Fatalf("expected empty ProfileName, got %q", created.ProfileName)
	}
}

func TestCreateJobRejectsConflictingTagDefaultProfiles(t *testing.T) {
	srv := New(DefaultConfig())
	createTag(t, srv, "alpha", "")
	createTag(t, srv, "beta", "")
	profile1 := createProfile(t, srv, `{"name":"one","config":{"net":{"ipv4":true}}}`)
	profile2 := createProfile(t, srv, `{"name":"two","config":{"net":{"ipv6":false}}}`)
	if err := srv.store.SetTagDefaultProfile("alpha", &profile1.ID); err != nil {
		t.Fatalf("SetTagDefaultProfile alpha: %v", err)
	}
	if err := srv.store.SetTagDefaultProfile("beta", &profile2.ID); err != nil {
		t.Fatalf("SetTagDefaultProfile beta: %v", err)
	}

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/jobs",
		bytes.NewBufferString(`{"domain":"example.com","tags":["alpha","beta"]}`))
	req.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", resp.Code, resp.Body)
	}

	var out ErrorResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.Error.Code != "ambiguous_profile" {
		t.Fatalf("expected ambiguous_profile, got %q", out.Error.Code)
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

// TestBatchJobAcceptsSnapshotIntent pins the Phase 6 admin-UI flow: the
// "Capture as cohort snapshot" checkbox is a flag on the POST /jobs/batch
// payload, and the server stores it on the batch record so the projector
// can gate snapshot accumulation on it.
func TestBatchJobAcceptsSnapshotIntent(t *testing.T) {
	srv := New(DefaultConfig())

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/jobs/batch",
		bytes.NewBufferString(`{"domains":["example.com"],"snapshot_intent":true}`))
	req.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", resp.Code, resp.Body)
	}
	var batchResp JobBatchResponse
	if err := json.NewDecoder(resp.Body).Decode(&batchResp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	batch, ok := srv.store.GetBatch(batchResp.BatchID)
	if !ok {
		t.Fatalf("batch not found: %s", batchResp.BatchID)
	}
	if !batch.SnapshotIntent {
		t.Fatal("SnapshotIntent should round-trip as true on the batch record")
	}

	// Default (flag omitted) must stay false so ad-hoc batches never
	// become snapshot-intent by accident.
	resp = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/v1/jobs/batch",
		bytes.NewBufferString(`{"domains":["example.net"]}`))
	req.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", resp.Code, resp.Body)
	}
	batchResp = JobBatchResponse{}
	if err := json.NewDecoder(resp.Body).Decode(&batchResp); err != nil {
		t.Fatalf("decode default: %v", err)
	}
	batch, _ = srv.store.GetBatch(batchResp.BatchID)
	if batch.SnapshotIntent {
		t.Fatal("SnapshotIntent defaulted to true without the request flag")
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

func TestBatchJobFromTagUsesDefaultProfile(t *testing.T) {
	srv := New(DefaultConfig())
	createTag(t, srv, "tld", "")
	profile := createProfile(t, srv, `{"name":"strict","config":{"net":{"ipv4":true}}}`)
	if err := srv.store.SetTagDefaultProfile("tld", &profile.ID); err != nil {
		t.Fatalf("SetTagDefaultProfile: %v", err)
	}
	d1 := makeGraduatedJob(t, srv, "example.com", JobSucceeded)
	d2 := makeGraduatedJob(t, srv, "example.net", JobSucceeded)
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
	for _, jobID := range batchResp.JobIDs {
		job, ok := srv.store.Get(jobID)
		if !ok {
			t.Fatalf("expected batch job %s", jobID)
		}
		if job.ProfileID == nil || *job.ProfileID != profile.ID {
			t.Fatalf("job %s ProfileID: got %v, want %d", jobID, job.ProfileID, profile.ID)
		}
		if job.ProfileName != "strict" {
			t.Fatalf("job %s ProfileName: got %q", jobID, job.ProfileName)
		}
	}
}

func TestBatchJobWithExplicitProfileID(t *testing.T) {
	srv := New(DefaultConfig())
	profile := createProfile(t, srv, `{"name":"strict","config":{"net":{"ipv4":true}}}`)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/jobs/batch",
		bytes.NewBufferString(fmt.Sprintf(`{"domains":["example.com","example.net"],"profile_id":%d}`, profile.ID)))
	req.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", resp.Code, resp.Body)
	}

	var batchResp JobBatchResponse
	if err := json.NewDecoder(resp.Body).Decode(&batchResp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	for _, jobID := range batchResp.JobIDs {
		job, ok := srv.store.Get(jobID)
		if !ok {
			t.Fatalf("expected batch job %s", jobID)
		}
		if job.ProfileID == nil || *job.ProfileID != profile.ID {
			t.Fatalf("job %s ProfileID: got %v, want %d", jobID, job.ProfileID, profile.ID)
		}
		if job.ProfileName != "strict" {
			t.Fatalf("job %s ProfileName: got %q", jobID, job.ProfileName)
		}
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
