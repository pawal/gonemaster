package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"codeberg.org/pawal/gonemaster/engine"
)

// seedGraduatedBatch inserts a batch + one graduated succeeded run so
// the handler tests have a realistic batch to exercise.
func seedGraduatedBatch(t *testing.T, srv *Server, batchID, tag string) {
	t.Helper()
	now := time.Now().UTC()
	if err := srv.store.CreateBatch(Batch{
		ID:        batchID,
		Tag:       tag,
		CreatedAt: now,
	}); err != nil {
		t.Fatalf("CreateBatch: %v", err)
	}
	domain, err := srv.store.GetOrCreateDomain("example.com")
	if err != nil {
		t.Fatalf("GetOrCreateDomain: %v", err)
	}
	job := Job{
		ID:         "job_" + batchID,
		BatchID:    batchID,
		Domain:     "example.com",
		DomainID:   domain.ID,
		Status:     JobSucceeded,
		CreatedAt:  now,
		StartedAt:  now,
		FinishedAt: now,
	}
	if _, err := srv.store.Create(job); err != nil {
		t.Fatalf("Create job: %v", err)
	}
	if err := srv.store.GraduateJob(job, []engine.LogEntry{
		{Timestamp: 1.0, Module: "System", Tag: "MODULE_START", Level: "INFO"},
	}); err != nil {
		t.Fatalf("GraduateJob: %v", err)
	}
}

func TestHandleBatchDeletePreviewReturnsCounts(t *testing.T) {
	srv := New(DefaultConfig())
	seedGraduatedBatch(t, srv, "batch_xyz", "tld")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/batches/batch_xyz/delete-preview", nil)
	resp := httptest.NewRecorder()
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.String())
	}
	var preview BatchDeletePreview
	if err := json.Unmarshal(resp.Body.Bytes(), &preview); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !preview.Exists || preview.BatchID != "batch_xyz" {
		t.Fatalf("unexpected preview: %+v", preview)
	}
	if preview.CompletedRuns != 1 {
		t.Fatalf("CompletedRuns = %d, want 1", preview.CompletedRuns)
	}
}

func TestHandleBatchDeletePreviewMissingReturns404(t *testing.T) {
	srv := New(DefaultConfig())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/batches/missing/delete-preview", nil)
	resp := httptest.NewRecorder()
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.Code)
	}
}

func TestHandleDeleteBatchReturns204AndPurges(t *testing.T) {
	srv := New(DefaultConfig())
	seedGraduatedBatch(t, srv, "batch_del", "tld")

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/batches/batch_del", nil)
	resp := httptest.NewRecorder()
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", resp.Code, resp.Body.String())
	}
	if _, ok := srv.store.GetBatch("batch_del"); ok {
		t.Fatal("batch should be gone")
	}
	if _, ok := srv.store.GetRun("job_batch_del"); ok {
		t.Fatal("run should be gone")
	}
}

func TestHandleDeleteBatchIgnoresTerminalJobRows(t *testing.T) {
	srv := New(DefaultConfig())
	now := time.Now().UTC()
	if err := srv.store.CreateBatch(Batch{
		ID:        "batch_terminal_jobs",
		Tag:       "tld",
		CreatedAt: now,
	}); err != nil {
		t.Fatalf("CreateBatch: %v", err)
	}
	if _, err := srv.store.Create(Job{
		ID:         "job-terminal",
		BatchID:    "batch_terminal_jobs",
		Domain:     "example.com",
		Status:     JobFailed,
		Error:      "server restarted during job",
		CreatedAt:  now,
		FinishedAt: now,
	}); err != nil {
		t.Fatalf("Create failed job: %v", err)
	}

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/batches/batch_terminal_jobs", nil)
	resp := httptest.NewRecorder()
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", resp.Code, resp.Body.String())
	}
	if _, ok := srv.store.GetBatch("batch_terminal_jobs"); ok {
		t.Fatal("batch should be gone")
	}
	if _, ok := srv.store.Get("job-terminal"); ok {
		t.Fatal("terminal job row should be gone")
	}
}

func TestHandleDeleteBatchCancelsStaleRunningJobRows(t *testing.T) {
	srv := New(DefaultConfig())
	now := time.Now().UTC()
	if err := srv.store.CreateBatch(Batch{
		ID:        "batch_stale_running",
		Tag:       "tld",
		CreatedAt: now,
	}); err != nil {
		t.Fatalf("CreateBatch: %v", err)
	}
	if _, err := srv.store.Create(Job{
		ID:        "job-stale-running",
		BatchID:   "batch_stale_running",
		Domain:    "example.com",
		Status:    JobRunning,
		CreatedAt: now,
		StartedAt: now,
	}); err != nil {
		t.Fatalf("Create running job: %v", err)
	}

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/batches/batch_stale_running", nil)
	resp := httptest.NewRecorder()
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", resp.Code, resp.Body.String())
	}
	if _, ok := srv.store.GetBatch("batch_stale_running"); ok {
		t.Fatal("batch should be gone")
	}
	if _, ok := srv.store.Get("job-stale-running"); ok {
		t.Fatal("stale running job should be gone")
	}
}

// TestHandleDeleteBatchUnpinsCohortDefault covers the regression where
// deleting the batch behind a cohort's pinned default snapshot left a
// dangling default_snapshot_id, hiding any remaining captured snapshots
// from the analysis UI. The handler must revert the cohort to
// auto_latest the same way snapshot retire/purge does.
func TestHandleDeleteBatchUnpinsCohortDefault(t *testing.T) {
	f := newAdminSnapshotFixture(t)
	pinPath := fmt.Sprintf("/api/v1/analysis/cohorts/%d/snapshots/%s", f.cohort.ID, f.snapshot.Slug)
	if resp := f.call(http.MethodPost, pinPath, `{"is_default":true}`); resp.Code != http.StatusOK {
		t.Fatalf("pin: got %d: %s", resp.Code, resp.Body)
	}

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/batches/"+f.snapshot.BatchID, nil)
	resp := httptest.NewRecorder()
	f.srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusNoContent {
		t.Fatalf("delete batch: got %d, want 204: %s", resp.Code, resp.Body)
	}

	cohort, _ := f.store.GetAnalysisCohort(f.cohort.ID)
	if cohort.DefaultSnapshotPolicy != DefaultSnapshotPolicyAutoLatest {
		t.Fatalf("policy = %q, want auto_latest after deleting pinned snapshot's batch", cohort.DefaultSnapshotPolicy)
	}
	if cohort.DefaultSnapshotID != nil {
		t.Fatalf("expected default_snapshot_id cleared, got %v", cohort.DefaultSnapshotID)
	}
}

func TestHandleDeleteBatchMissingReturns404(t *testing.T) {
	srv := New(DefaultConfig())
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/batches/missing", nil)
	resp := httptest.NewRecorder()
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.Code)
	}
}

func TestHandleDeleteBatchRejectsMismatchedOrigin(t *testing.T) {
	srv := New(DefaultConfig())
	seedGraduatedBatch(t, srv, "batch_csrf", "tld")

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/batches/batch_csrf", nil)
	req.Host = "example.com"
	req.Header.Set("Origin", "https://attacker.example")
	resp := httptest.NewRecorder()
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", resp.Code)
	}
	if _, ok := srv.store.GetBatch("batch_csrf"); !ok {
		t.Fatal("batch should still exist after CSRF rejection")
	}
}

func TestHandleTagBatchesReturnsRecent(t *testing.T) {
	srv := New(DefaultConfig())
	if err := srv.store.CreateTag("tld", "tld tag"); err != nil {
		t.Fatalf("CreateTag: %v", err)
	}
	seedGraduatedBatch(t, srv, "batch_a", "tld")
	seedGraduatedBatch(t, srv, "batch_b", "tld")
	seedGraduatedBatch(t, srv, "batch_other", "muni")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/tags/tld/batches", nil)
	resp := httptest.NewRecorder()
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.String())
	}
	var list BatchList
	if err := json.Unmarshal(resp.Body.Bytes(), &list); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if list.Total != 2 {
		t.Fatalf("Total = %d, want 2", list.Total)
	}
	for _, b := range list.Items {
		if b.Tag != "tld" {
			t.Fatalf("item tag = %q, want \"tld\"", b.Tag)
		}
	}
}

func TestHandleTagBatchesUnknownTagReturns404(t *testing.T) {
	srv := New(DefaultConfig())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/tags/does-not-exist/batches", nil)
	resp := httptest.NewRecorder()
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.Code)
	}
}
