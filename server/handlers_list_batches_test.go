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

// graduateRunInBatch creates and graduates one succeeded run for a batch at the
// given finish time, so the batch accrues a completed run.
func graduateRunInBatch(t *testing.T, srv *Server, batchID, domain string, finishedAt time.Time) {
	t.Helper()
	d, err := srv.store.GetOrCreateDomain(domain)
	if err != nil {
		t.Fatalf("GetOrCreateDomain: %v", err)
	}
	job := Job{
		ID: fmt.Sprintf("job_%s_%s", batchID, domain), BatchID: batchID, Domain: domain, DomainID: d.ID,
		Status: JobSucceeded, CreatedAt: finishedAt, StartedAt: finishedAt, FinishedAt: finishedAt,
	}
	if _, err := srv.store.Create(job); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := srv.store.GraduateJob(job, []engine.LogEntry{{Timestamp: 1.0, Module: "System", Tag: "MODULE_START", Level: "INFO"}}); err != nil {
		t.Fatalf("GraduateJob: %v", err)
	}
}

// queueJobInBatch creates a still-queued job for a batch, leaving it in-flight.
func queueJobInBatch(t *testing.T, srv *Server, batchID, domain string) {
	t.Helper()
	d, err := srv.store.GetOrCreateDomain(domain)
	if err != nil {
		t.Fatalf("GetOrCreateDomain: %v", err)
	}
	if _, err := srv.store.Create(Job{
		ID: fmt.Sprintf("jobq_%s_%s", batchID, domain), BatchID: batchID, Domain: domain, DomainID: d.ID,
		Status: JobQueued, CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("Create queued: %v", err)
	}
}

func listBatches(t *testing.T, srv *Server, query string) BatchListResponse {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/batches"+query, nil)
	resp := httptest.NewRecorder()
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.String())
	}
	var body BatchListResponse
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return body
}

func TestHandleListBatches(t *testing.T) {
	srv := New(DefaultConfig())
	old := time.Now().UTC().Add(-2 * time.Hour)
	recent := time.Now().UTC().Add(-1 * time.Hour)

	// A finished cohort: 2 domains, both graduated.
	if err := srv.store.CreateBatch(Batch{ID: "b_done", Tag: "tld-weekly", DomainCount: 2, CreatedAt: old}); err != nil {
		t.Fatalf("CreateBatch: %v", err)
	}
	graduateRunInBatch(t, srv, "b_done", "a.example", old.Add(5*time.Minute))
	graduateRunInBatch(t, srv, "b_done", "b.example", old.Add(6*time.Minute))

	// An in-progress cohort: 4 domains, 1 graduated, 1 still queued.
	if err := srv.store.CreateBatch(Batch{ID: "b_running", Tag: "se-batch", DomainCount: 4, CreatedAt: recent}); err != nil {
		t.Fatalf("CreateBatch: %v", err)
	}
	graduateRunInBatch(t, srv, "b_running", "c.example", recent.Add(time.Minute))
	queueJobInBatch(t, srv, "b_running", "d.example")

	body := listBatches(t, srv, "")
	if body.Total != 2 || len(body.Items) != 2 {
		t.Fatalf("expected two batches, got %+v", body)
	}
	// Newest first: b_running precedes b_done.
	if body.Items[0].BatchID != "b_running" || body.Items[1].BatchID != "b_done" {
		t.Fatalf("ordering wrong: %v, %v", body.Items[0].BatchID, body.Items[1].BatchID)
	}

	running := body.Items[0]
	if running.Status != "running" {
		t.Errorf("b_running status = %q, want running", running.Status)
	}
	if running.Completed != 1 || running.Total != 4 || running.Completion != 25 {
		t.Errorf("b_running progress wrong: %+v", running)
	}
	if running.FinishedAt != nil {
		t.Errorf("running batch must not report finished_at, got %v", running.FinishedAt)
	}

	done := body.Items[1]
	if done.Status != "done" {
		t.Errorf("b_done status = %q, want done", done.Status)
	}
	if done.Completed != 2 || done.Total != 2 || done.Completion != 100 {
		t.Errorf("b_done progress wrong: %+v", done)
	}
	if done.FinishedAt == nil {
		t.Errorf("done batch must report finished_at")
	}
}

func TestHandleListBatchesEmpty(t *testing.T) {
	srv := New(DefaultConfig())
	body := listBatches(t, srv, "")
	if body.Total != 0 || len(body.Items) != 0 {
		t.Fatalf("empty store should yield no batches, got %+v", body)
	}
}

func TestHandleListBatchesLabelFilter(t *testing.T) {
	srv := New(DefaultConfig())
	now := time.Now().UTC()
	if err := srv.store.CreateBatch(Batch{ID: "b1", Tag: "tld-weekly", DomainCount: 1, CreatedAt: now}); err != nil {
		t.Fatalf("CreateBatch: %v", err)
	}
	if err := srv.store.CreateBatch(Batch{ID: "b2", Tag: "se-batch", DomainCount: 1, CreatedAt: now}); err != nil {
		t.Fatalf("CreateBatch: %v", err)
	}

	body := listBatches(t, srv, "?label=tld")
	if body.Total != 1 || len(body.Items) != 1 || body.Items[0].BatchID != "b1" {
		t.Fatalf("label=tld should match only b1, got %+v", body)
	}
}
