package server

import (
	"errors"
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

// handleBatchTagValues handles GET /api/v1/batches/{id}/tag-values.
func (s *Server) handleBatchTagValues(w http.ResponseWriter, r *http.Request) {
	batchID := strings.TrimSpace(r.PathValue("id"))
	if batchID == "" {
		writeError(w, http.StatusBadRequest, "missing_batch_id", "batch id is required", nil)
		return
	}

	tag := strings.TrimSpace(r.URL.Query().Get("tag"))
	if tag == "" {
		writeError(w, http.StatusBadRequest, "missing_tag", "tag is required", nil)
		return
	}
	arg := strings.TrimSpace(r.URL.Query().Get("arg"))
	if arg == "" {
		writeError(w, http.StatusBadRequest, "missing_arg", "arg is required", nil)
		return
	}
	minCount, ok := parsePositiveQuery(r, "min_count", 1)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_min_count", "min_count must be a positive integer", nil)
		return
	}
	limit, ok := parsePositiveQuery(r, "limit", 50)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_limit", "limit must be a positive integer", nil)
		return
	}
	if limit > 500 {
		limit = 500
	}
	weightByScore := false
	if raw := strings.TrimSpace(r.URL.Query().Get("weight_by_score")); raw != "" {
		b, err := strconv.ParseBool(raw)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_weight_by_score", "weight_by_score must be a boolean", nil)
			return
		}
		weightByScore = b
	}

	if !s.batchExists(batchID) {
		writeError(w, http.StatusNotFound, "not_found", "batch not found", nil)
		return
	}
	if s.store.List(JobFilter{BatchID: batchID, Limit: 1}).Total > 0 {
		writeError(w, http.StatusConflict, "batch_not_complete", "batch not complete", nil)
		return
	}

	values := s.batchTagValues(batchID, tag, arg, minCount, limit, weightByScore)
	if values == nil {
		values = []TagValueRollup{}
	}
	writeJSON(w, http.StatusOK, BatchTagValuesResponse{
		BatchID:       batchID,
		Tag:           tag,
		Arg:           arg,
		MinCount:      minCount,
		WeightByScore: weightByScore,
		Values:        values,
	})
}

// batchExists checks the batches table, then in-flight jobs, then runs.
func (s *Server) batchExists(batchID string) bool {
	if _, ok := s.store.GetBatch(batchID); ok {
		return true
	}
	if s.store.List(JobFilter{BatchID: batchID, Limit: 1}).Total > 0 {
		return true
	}
	return s.store.ListRuns(RunFilter{BatchID: batchID, Limit: 1}).Total > 0
}

// parsePositiveQuery reads an optional positive int; empty yields def.
func parsePositiveQuery(r *http.Request, name string, def int) (int, bool) {
	raw := strings.TrimSpace(r.URL.Query().Get(name))
	if raw == "" {
		return def, true
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 {
		return 0, false
	}
	return n, true
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

// handlePatchBatch handles PATCH /api/v1/batches/{id}. Only mutable
// field is snapshot_intent.
func (s *Server) handlePatchBatch(w http.ResponseWriter, r *http.Request) {
	if !s.enforceCSRF(w, r) {
		return
	}
	batchID := strings.TrimSpace(r.PathValue("id"))
	if batchID == "" {
		writeError(w, http.StatusBadRequest, "missing_batch_id", "batch id is required", nil)
		return
	}
	var req struct {
		SnapshotIntent *bool `json:"snapshot_intent"`
	}
	if err := readJSON(r, s.cfg.MaxBodySize, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", err.Error(), nil)
		return
	}
	if req.SnapshotIntent == nil {
		writeError(w, http.StatusBadRequest, "missing_field", "snapshot_intent is required", nil)
		return
	}
	if err := s.store.SetBatchSnapshotIntent(batchID, *req.SnapshotIntent); err != nil {
		if errors.Is(err, ErrBatchNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "batch not found", nil)
			return
		}
		writeError(w, http.StatusInternalServerError, "store_error", err.Error(), nil)
		return
	}
	batch, ok := s.store.GetBatch(batchID)
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "batch not found", nil)
		return
	}
	writeJSON(w, http.StatusOK, batch)
}

// handleDeleteBatch handles DELETE /api/v1/batches/{id}. Cancels any
// in-flight jobs for the batch, waits briefly for them to drain, then
// removes the batch and every row derived from it.
func (s *Server) handleDeleteBatch(w http.ResponseWriter, r *http.Request) {
	if !s.enforceCSRF(w, r) {
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
	if err := s.unpinCohortsForDeletedSnapshots(snapshotIDs); err != nil {
		writeError(w, http.StatusInternalServerError, "store_error", err.Error(), nil)
		return
	}
	log.Printf("batch_delete: id=%s tag=%s runs=%d entries=%d snapshots=%d",
		batchID, preview.Tag, preview.CompletedRuns, preview.Entries, len(snapshotIDs))
	w.WriteHeader(http.StatusNoContent)
}

// unpinCohortsForDeletedSnapshots reverts any cohort whose pinned
// default snapshot was just hard-deleted back to auto_latest. Without
// this the cohort retains a dangling default_snapshot_id and
// GetDefaultSnapshotForCohort returns nothing, hiding any remaining
// captured snapshots from the analysis UI.
func (s *Server) unpinCohortsForDeletedSnapshots(deletedIDs []int64) error {
	if len(deletedIDs) == 0 {
		return nil
	}
	store, ok := s.store.(adminSnapshotStore)
	if !ok {
		return nil
	}
	deleted := make(map[int64]struct{}, len(deletedIDs))
	for _, id := range deletedIDs {
		deleted[id] = struct{}{}
	}
	for _, cohort := range s.store.ListAnalysisCohorts() {
		if cohort.DefaultSnapshotID == nil {
			continue
		}
		if _, hit := deleted[*cohort.DefaultSnapshotID]; !hit {
			continue
		}
		if err := s.unpinCohortDefaultSnapshot(store, cohort, 0); err != nil {
			return err
		}
	}
	return nil
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
		case JobQueued, JobPaused:
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
			if s.cancelJob(job.ID) {
				continue
			}
			// A running row without a registered cancel function is stale
			// in this server process. Graduate it synchronously so delete
			// can continue instead of timing out forever.
			updated := job
			updated.Status = JobCanceled
			updated.Error = "deleted_by_admin"
			updated.FinishedAt = time.Now().UTC()
			updated.Progress = 100
			if err := s.store.GraduateJob(updated, nil); err != nil {
				return err
			}
		}
	}
	return nil
}

// waitForBatchJobsTerminal polls until non-terminal job rows with the
// matching batch_id have drained from the in-flight jobs table or
// timeout elapses. Terminal rows can remain in jobs after recovery and
// are removed by the delete transaction itself.
func (s *Server) waitForBatchJobsTerminal(batchID string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for {
		if s.countActiveBatchJobs(batchID) == 0 {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func (s *Server) countActiveBatchJobs(batchID string) int {
	total := 0
	for _, status := range []JobStatus{JobQueued, JobRunning, JobPaused} {
		total += s.store.List(JobFilter{
			BatchID: batchID,
			Status:  status,
			Limit:   1,
		}).Total
	}
	return total
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
