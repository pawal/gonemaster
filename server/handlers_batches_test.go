package server

import (
	"encoding/json"
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
