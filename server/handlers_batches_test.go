package server

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
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
	seedGraduatedRun(t, srv.store, runSpec{
		ID:            "job_" + batchID,
		BatchID:       batchID,
		At:            now,
		Entries:       systemStartEntry(),
		ResolveDomain: true,
	})
}

func TestHandleBatchDeletePreviewReturnsCounts(t *testing.T) {
	srv := newTestServer(t)
	seedGraduatedBatch(t, srv, "batch_xyz", "tld")

	resp := doJSON(t, srv, http.MethodGet, "/api/v1/batches/batch_xyz/delete-preview", nil)
	preview := mustJSON[BatchDeletePreview](t, resp, http.StatusOK)
	if !preview.Exists || preview.BatchID != "batch_xyz" {
		t.Fatalf("unexpected preview: %+v", preview)
	}
	if preview.CompletedRuns != 1 {
		t.Fatalf("CompletedRuns = %d, want 1", preview.CompletedRuns)
	}
}

func TestHandleBatchDeletePreviewMissingReturns404(t *testing.T) {
	srv := newTestServer(t)
	resp := doJSON(t, srv, http.MethodGet, "/api/v1/batches/missing/delete-preview", nil)
	wantStatus(t, resp, http.StatusNotFound)
}

func TestHandleDeleteBatchReturns204AndPurges(t *testing.T) {
	srv := newTestServer(t)
	seedGraduatedBatch(t, srv, "batch_del", "tld")

	resp := doJSON(t, srv, http.MethodDelete, "/api/v1/batches/batch_del", nil)
	wantStatus(t, resp, http.StatusNoContent)
	if _, ok := srv.store.GetBatch("batch_del"); ok {
		t.Fatal("batch should be gone")
	}
	if _, ok := srv.store.GetRun("job_batch_del"); ok {
		t.Fatal("run should be gone")
	}
}

func TestHandleDeleteBatchIgnoresTerminalJobRows(t *testing.T) {
	srv := newTestServer(t)
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

	resp := doJSON(t, srv, http.MethodDelete, "/api/v1/batches/batch_terminal_jobs", nil)
	wantStatus(t, resp, http.StatusNoContent)
	if _, ok := srv.store.GetBatch("batch_terminal_jobs"); ok {
		t.Fatal("batch should be gone")
	}
	if _, ok := srv.store.Get("job-terminal"); ok {
		t.Fatal("terminal job row should be gone")
	}
}

func TestHandleDeleteBatchCancelsStaleRunningJobRows(t *testing.T) {
	srv := newTestServer(t)
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

	resp := doJSON(t, srv, http.MethodDelete, "/api/v1/batches/batch_stale_running", nil)
	wantStatus(t, resp, http.StatusNoContent)
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
	forEachAdminSnapshotFixture(t, func(t *testing.T, f *analysisFixture) {
		pinPath := fmt.Sprintf("/api/v1/analysis/cohorts/%d/snapshots/%s", f.cohort.ID, f.snapshot.Slug)
		wantStatus(t, f.call(http.MethodPost, pinPath, `{"is_default":true}`), http.StatusOK)

		resp := doJSON(t, f.srv, http.MethodDelete, "/api/v1/batches/"+f.snapshot.BatchID, nil)
		wantStatus(t, resp, http.StatusNoContent)

		cohort, _ := f.store.GetAnalysisCohort(f.cohort.ID)
		if cohort.DefaultSnapshotPolicy != DefaultSnapshotPolicyAutoLatest {
			t.Fatalf("policy = %q, want auto_latest after deleting pinned snapshot's batch", cohort.DefaultSnapshotPolicy)
		}
		if cohort.DefaultSnapshotID != nil {
			t.Fatalf("expected default_snapshot_id cleared, got %v", cohort.DefaultSnapshotID)
		}
	})
}

func TestHandlePatchBatchTogglesSnapshotIntent(t *testing.T) {
	srv := newTestServer(t)
	seedGraduatedBatch(t, srv, "batch_snap", "tld")
	if b, _ := srv.store.GetBatch("batch_snap"); b.SnapshotIntent {
		t.Fatalf("seed batch should default to snapshot_intent=false")
	}

	body := strings.NewReader(`{"snapshot_intent":true}`)
	resp := doJSON(t, srv, http.MethodPatch, "/api/v1/batches/batch_snap", body, noContentType())
	wantStatus(t, resp, http.StatusOK)
	if b, _ := srv.store.GetBatch("batch_snap"); !b.SnapshotIntent {
		t.Fatalf("snapshot_intent should be true after PATCH")
	}

	body = strings.NewReader(`{"snapshot_intent":false}`)
	resp = doJSON(t, srv, http.MethodPatch, "/api/v1/batches/batch_snap", body, noContentType())
	wantStatus(t, resp, http.StatusOK)
	if b, _ := srv.store.GetBatch("batch_snap"); b.SnapshotIntent {
		t.Fatalf("snapshot_intent should be false after second PATCH")
	}
}

func TestHandlePatchBatchMissingReturns404(t *testing.T) {
	srv := newTestServer(t)
	body := strings.NewReader(`{"snapshot_intent":true}`)
	resp := doJSON(t, srv, http.MethodPatch, "/api/v1/batches/missing", body, noContentType())
	wantStatus(t, resp, http.StatusNotFound)
}

func TestHandlePatchBatchRequiresField(t *testing.T) {
	srv := newTestServer(t)
	seedGraduatedBatch(t, srv, "batch_req", "tld")
	body := strings.NewReader(`{}`)
	resp := doJSON(t, srv, http.MethodPatch, "/api/v1/batches/batch_req", body, noContentType())
	wantStatus(t, resp, http.StatusBadRequest)
}

func TestHandleDeleteBatchMissingReturns404(t *testing.T) {
	srv := newTestServer(t)
	resp := doJSON(t, srv, http.MethodDelete, "/api/v1/batches/missing", nil)
	wantStatus(t, resp, http.StatusNotFound)
}

func TestHandleDeleteBatchRejectsMismatchedOrigin(t *testing.T) {
	srv := newTestServer(t)
	seedGraduatedBatch(t, srv, "batch_csrf", "tld")

	resp := doJSON(t, srv, http.MethodDelete, "/api/v1/batches/batch_csrf", nil, withHost("example.com"), withOrigin("https://attacker.example"))
	wantStatus(t, resp, http.StatusForbidden)
	if _, ok := srv.store.GetBatch("batch_csrf"); !ok {
		t.Fatal("batch should still exist after CSRF rejection")
	}
}

func TestHandleTagBatchesReturnsRecent(t *testing.T) {
	srv := newTestServer(t)
	if err := srv.store.CreateTag("tld", "tld tag"); err != nil {
		t.Fatalf("CreateTag: %v", err)
	}
	seedGraduatedBatch(t, srv, "batch_a", "tld")
	seedGraduatedBatch(t, srv, "batch_b", "tld")
	seedGraduatedBatch(t, srv, "batch_other", "muni")

	resp := doJSON(t, srv, http.MethodGet, "/api/v1/tags/tld/batches", nil)
	list := mustJSON[BatchList](t, resp, http.StatusOK)
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
	srv := newTestServer(t)
	resp := doJSON(t, srv, http.MethodGet, "/api/v1/tags/does-not-exist/batches", nil)
	wantStatus(t, resp, http.StatusNotFound)
}

// TestDeleteBatchRestoresQueueDepth covers the regression where deleting
// a batch cancelled its queued jobs without reporting the transitions, so
// gonemaster_queue_depth stayed high by the number of jobs cancelled and
// never recovered for the lifetime of the process. A batch cancel must
// leave the same metrics as cancelling each job individually.
func TestDeleteBatchRestoresQueueDepth(t *testing.T) {
	srv := newTestServer(t)

	// Pause first so the submitted jobs stay queued.
	resp := doJSON(t, srv, http.MethodPost, "/api/v1/queue/pause", nil)
	wantStatus(t, resp, http.StatusOK)

	resp = doJSON(t, srv, http.MethodPost, "/api/v1/jobs/batch", `{"domains":["example.com","example.net","example.org"]}`)
	out := mustJSON[JobBatchResponse](t, resp, http.StatusAccepted)
	if len(out.JobIDs) != 3 {
		t.Fatalf("expected 3 job ids, got %d", len(out.JobIDs))
	}

	snapshot := getMetricsSnapshot(t, srv)
	if snapshot.Health.QueueDepth != 3 {
		t.Fatalf("queue_depth before delete = %d, want 3", snapshot.Health.QueueDepth)
	}

	resp = doJSON(t, srv, http.MethodDelete, "/api/v1/batches/"+out.BatchID, nil)
	wantStatus(t, resp, http.StatusNoContent)

	snapshot = getMetricsSnapshot(t, srv)
	if snapshot.Health.QueueDepth != 0 {
		t.Fatalf("queue_depth after delete = %d, want 0", snapshot.Health.QueueDepth)
	}
	if snapshot.Jobs.StatusCounts[string(JobQueued)] != 0 {
		t.Fatalf("status_counts[queued] = %d, want 0", snapshot.Jobs.StatusCounts[string(JobQueued)])
	}
	if snapshot.Jobs.CanceledTotal != 3 {
		t.Fatalf("canceled_total = %d, want 3", snapshot.Jobs.CanceledTotal)
	}
	if snapshot.Jobs.CompletedTotal != 3 {
		t.Fatalf("completed_total = %d, want 3", snapshot.Jobs.CompletedTotal)
	}
}

// TestDeleteBatchClearsInFlightForStaleRunningJob covers the same gap on
// the stale-running branch: a running row with no registered cancel
// function is graduated inline, which must also release its in-flight
// slot.
func TestDeleteBatchClearsInFlightForStaleRunningJob(t *testing.T) {
	srv := newTestServer(t)
	now := time.Now().UTC()
	if err := srv.store.CreateBatch(Batch{
		ID:        "batch_stale_inflight",
		Tag:       "tld",
		CreatedAt: now,
	}); err != nil {
		t.Fatalf("CreateBatch: %v", err)
	}
	if _, err := srv.store.Create(Job{
		ID:        "job-stale-inflight",
		BatchID:   "batch_stale_inflight",
		Domain:    "example.com",
		Status:    JobRunning,
		CreatedAt: now,
		StartedAt: now,
	}); err != nil {
		t.Fatalf("Create running job: %v", err)
	}
	// The store row is seeded directly, so tell the collector about it.
	srv.metrics.ObserveJobStatusTransition("", JobRunning)
	if snapshot := getMetricsSnapshot(t, srv); snapshot.Health.InFlightJobs != 1 {
		t.Fatalf("in_flight_jobs before delete = %d, want 1", snapshot.Health.InFlightJobs)
	}

	resp := doJSON(t, srv, http.MethodDelete, "/api/v1/batches/batch_stale_inflight", nil)
	wantStatus(t, resp, http.StatusNoContent)

	snapshot := getMetricsSnapshot(t, srv)
	if snapshot.Health.InFlightJobs != 0 {
		t.Fatalf("in_flight_jobs after delete = %d, want 0", snapshot.Health.InFlightJobs)
	}
	if snapshot.Jobs.CanceledTotal != 1 {
		t.Fatalf("canceled_total = %d, want 1", snapshot.Jobs.CanceledTotal)
	}
}
