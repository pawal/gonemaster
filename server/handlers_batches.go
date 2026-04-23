package server

import (
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const batchDeleteCancelTimeout = 5 * time.Second

// handleTagBatches handles GET /api/v1/tags/{name}/batches and returns
// batches whose tag matches {name}, newest first, paginated.
func (s *Server) handleTagBatches(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.PathValue("name"))
	if name == "" {
		writeError(w, http.StatusBadRequest, "missing_name", "tag name is required", nil)
		return
	}
	if _, ok := s.store.GetTag(name); !ok {
		writeError(w, http.StatusNotFound, "not_found", "tag not found", nil)
		return
	}
	limit, offset := parseLimitOffset(r, 20, 100)
	list := s.store.ListBatchesByTag(name, limit, offset)
	if list.Items == nil {
		list.Items = []Batch{}
	}
	writeJSON(w, http.StatusOK, list)
}

// handleBatchDeletePreview handles GET /api/v1/batches/{id}/delete-preview.
// Returns row counts plus the cohort snapshots that will be removed,
// including whether any of them is the cohort's current default.
func (s *Server) handleBatchDeletePreview(w http.ResponseWriter, r *http.Request) {
	batchID := strings.TrimSpace(r.PathValue("id"))
	if batchID == "" {
		writeError(w, http.StatusBadRequest, "missing_batch_id", "batch id is required", nil)
		return
	}
	preview, err := s.store.BatchDeletePreviewStats(batchID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "store_error", err.Error(), nil)
		return
	}
	if !preview.Exists {
		writeError(w, http.StatusNotFound, "not_found", "batch not found", nil)
		return
	}
	preview.Snapshots = s.snapshotImpactForBatch(batchID)
	writeJSON(w, http.StatusOK, preview)
}

// handleDeleteBatch handles DELETE /api/v1/batches/{id}. Cancels any
// in-flight jobs for the batch, waits briefly for them to drain, then
// removes the batch and every row derived from it.
func (s *Server) handleDeleteBatch(w http.ResponseWriter, r *http.Request) {
	if !enforceCSRF(w, r) {
		return
	}
	batchID := strings.TrimSpace(r.PathValue("id"))
	if batchID == "" {
		writeError(w, http.StatusBadRequest, "missing_batch_id", "batch id is required", nil)
		return
	}
	preview, err := s.store.BatchDeletePreviewStats(batchID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "store_error", err.Error(), nil)
		return
	}
	if !preview.Exists {
		writeError(w, http.StatusNotFound, "not_found", "batch not found", nil)
		return
	}

	if err := s.cancelBatchJobs(batchID); err != nil {
		writeError(w, http.StatusInternalServerError, "cancel_error", err.Error(), nil)
		return
	}
	if !s.waitForBatchJobsTerminal(batchID, batchDeleteCancelTimeout) {
		writeError(w, http.StatusConflict, "jobs_not_terminal",
			"batch has jobs that did not cancel in time; try again", nil)
		return
	}

	snapshotIDs, err := s.store.DeleteBatch(batchID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "store_error", err.Error(), nil)
		return
	}
	s.evictSnapshotMatCache(snapshotIDs)
	log.Printf("batch_delete: id=%s tag=%s runs=%d entries=%d snapshots=%d",
		batchID, preview.Tag, preview.CompletedRuns, preview.Entries, len(snapshotIDs))
	w.WriteHeader(http.StatusNoContent)
}

// snapshotImpactForBatch returns the list of cohort snapshots that
// would be removed if the batch is deleted. Safe to call when the
// store does not implement the snapshot interface (returns nil).
func (s *Server) snapshotImpactForBatch(batchID string) []BatchDeletePreviewSnapshot {
	if s.store == nil {
		return nil
	}
	cohorts := s.store.ListAnalysisCohorts()
	if len(cohorts) == 0 {
		return nil
	}
	store, ok := s.store.(adminSnapshotStore)
	if !ok {
		return nil
	}
	var out []BatchDeletePreviewSnapshot
	for _, cohort := range cohorts {
		for _, snap := range store.ListAnalysisCohortSnapshots(cohort.ID) {
			if snap.BatchID != batchID {
				continue
			}
			out = append(out, BatchDeletePreviewSnapshot{
				CohortID:      cohort.ID,
				CohortLabel:   cohort.Label,
				SnapshotSlug:  snap.Slug,
				SnapshotLabel: snap.Label,
				IsDefault:     snap.IsDefault,
			})
		}
	}
	return out
}

// cancelBatchJobs removes queued jobs from the queue and cancels any
// running jobs for the batch. Queued jobs graduate to canceled here;
// running jobs drain asynchronously and are awaited by the caller.
func (s *Server) cancelBatchJobs(batchID string) error {
	list := s.store.List(JobFilter{BatchID: batchID, Limit: 10000})
	for _, job := range list.Items {
		switch job.Status {
		case JobQueued:
			_ = s.queue.Remove(job.ID)
			updated := job
			updated.Status = JobCanceled
			updated.Error = "deleted_by_admin"
			updated.FinishedAt = time.Now().UTC()
			updated.Progress = 100
			if err := s.store.GraduateJob(updated, nil); err != nil {
				return err
			}
		case JobRunning:
			_ = s.cancelJob(job.ID)
		}
	}
	return nil
}

// waitForBatchJobsTerminal polls until every job row with the matching
// batch_id has drained from the in-flight jobs table or timeout
// elapses. Returns true on a clean drain.
func (s *Server) waitForBatchJobsTerminal(batchID string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for {
		list := s.store.List(JobFilter{BatchID: batchID, Limit: 1})
		if list.Total == 0 {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// evictSnapshotMatCache removes the materialization cache entries for
// snapshots that were just deleted so public analysis requests can not
// serve stale data.
func (s *Server) evictSnapshotMatCache(snapshotIDs []int64) {
	if len(snapshotIDs) == 0 {
		return
	}
	s.analysisMatCacheMu.Lock()
	defer s.analysisMatCacheMu.Unlock()
	for _, id := range snapshotIDs {
		delete(s.analysisMatCache, id)
	}
}

// parseLimitOffset reads ?limit= and ?offset= from the request,
// clamped to [1, max] for limit and [0, ∞) for offset.
func parseLimitOffset(r *http.Request, defaultLimit, maxLimit int) (int, int) {
	limit := defaultLimit
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			if n > maxLimit {
				n = maxLimit
			}
			limit = n
		}
	}
	offset := 0
	if raw := r.URL.Query().Get("offset"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n >= 0 {
			offset = n
		}
	}
	return limit, offset
}
