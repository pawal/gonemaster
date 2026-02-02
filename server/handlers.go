package server

import (
	"fmt"
	"net/http"
	"strings"
	"time"
)

func (s *Server) handleJobs(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		s.handleCreateJob(w, r)
	case http.MethodGet:
		s.handleListJobs(w, r)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
	}
}

func (s *Server) handleJobsBatch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
		return
	}
	var req JobBatchRequest
	if err := readJSON(r, s.cfg.MaxBodySize, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", err.Error(), nil)
		return
	}
	if len(req.Domains) == 0 {
		writeError(w, http.StatusBadRequest, "missing_domain", "domains is required", nil)
		return
	}

	batchID := newID("batch")
	jobIDs := make([]string, 0, len(req.Domains))
	for _, domain := range req.Domains {
		job := Job{
			ID:        newID("job"),
			BatchID:   batchID,
			Domain:    domain,
			Tests:     req.Tests,
			Overrides: req.ProfileOverrides,
			MinLevel:  req.MinLevel,
			Status:    JobQueued,
			CreatedAt: time.Now().UTC(),
		}
		created, err := s.store.Create(job)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "store_error", err.Error(), nil)
			return
		}
		_ = s.queue.Enqueue(created.ID)
		jobIDs = append(jobIDs, created.ID)
	}

	writeJSON(w, http.StatusAccepted, JobBatchResponse{BatchID: batchID, JobIDs: jobIDs})
}

func (s *Server) handleJobByID(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/jobs/")
	if path == "" || path == r.URL.Path {
		writeError(w, http.StatusNotFound, "not_found", "not found", nil)
		return
	}
	parts := strings.Split(path, "/")
	jobID := parts[0]
	if jobID == "" {
		writeError(w, http.StatusNotFound, "not_found", "not found", nil)
		return
	}

	if len(parts) == 1 {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
			return
		}
		s.handleGetJob(w, r, jobID)
		return
	}

	switch parts[1] {
	case "result":
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
			return
		}
		s.handleGetJobResult(w, r, jobID)
	case "events":
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
			return
		}
		s.handleJobEvents(w, r, jobID)
	case "cancel":
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
			return
		}
		s.handleCancelJob(w, r, jobID)
	default:
		writeError(w, http.StatusNotFound, "not_found", "not found", nil)
	}
}

func (s *Server) handleBatchByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/batches/")
	if path == "" || path == r.URL.Path {
		writeError(w, http.StatusNotFound, "not_found", "not found", nil)
		return
	}
	batchID := strings.TrimSpace(path)
	if batchID == "" {
		writeError(w, http.StatusNotFound, "not_found", "not found", nil)
		return
	}

	list := s.store.List(JobFilter{BatchID: batchID, Limit: 1000})
	if list.Total == 0 {
		writeError(w, http.StatusNotFound, "not_found", "batch not found", nil)
		return
	}

	statusCounts := map[string]int{}
	var createdAt time.Time
	var startedAt *time.Time
	var finishedAtLatest time.Time
	allFinished := true

	for _, job := range list.Items {
		statusCounts[string(job.Status)]++
		if createdAt.IsZero() || job.CreatedAt.Before(createdAt) {
			createdAt = job.CreatedAt
		}
		if !job.StartedAt.IsZero() {
			if startedAt == nil || job.StartedAt.Before(*startedAt) {
				t := job.StartedAt
				startedAt = &t
			}
		}
		if job.FinishedAt.IsZero() {
			allFinished = false
		} else if finishedAtLatest.IsZero() || job.FinishedAt.After(finishedAtLatest) {
			finishedAtLatest = job.FinishedAt
		}
	}

	var finishedAt *time.Time
	if allFinished && !finishedAtLatest.IsZero() {
		finishedAt = &finishedAtLatest
	}

	summary := BatchSummary{
		BatchID:      batchID,
		Total:        list.Total,
		StatusCounts: statusCounts,
		Items:        list.Items,
		CreatedAt:    createdAt,
		StartedAt:    startedAt,
		FinishedAt:   finishedAt,
	}
	writeJSON(w, http.StatusOK, summary)
}

func (s *Server) handleCreateJob(w http.ResponseWriter, r *http.Request) {
	var req JobCreateRequest
	if err := readJSON(r, s.cfg.MaxBodySize, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", err.Error(), nil)
		return
	}
	if strings.TrimSpace(req.Domain) == "" {
		writeError(w, http.StatusBadRequest, "missing_domain", "domain is required", nil)
		return
	}

	job := Job{
		ID:        newID("job"),
		Domain:    req.Domain,
		Tests:     req.Tests,
		Overrides: req.ProfileOverrides,
		MinLevel:  req.MinLevel,
		Status:    JobQueued,
		CreatedAt: time.Now().UTC(),
	}
	created, err := s.store.Create(job)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "store_error", err.Error(), nil)
		return
	}
	_ = s.queue.Enqueue(created.ID)

	writeJSON(w, http.StatusCreated, created)
}

func (s *Server) handleListJobs(w http.ResponseWriter, r *http.Request) {
	filter := JobFilter{Limit: 100}
	status := r.URL.Query().Get("status")
	if status != "" {
		filter.Status = JobStatus(status)
	}
	filter.BatchID = r.URL.Query().Get("batch_id")
	if createdAfter := r.URL.Query().Get("created_after"); createdAfter != "" {
		timestamp, err := parseTime(createdAfter)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_created_after", "created_after must be RFC3339", nil)
			return
		}
		filter.CreatedAfter = timestamp
	}
	if limit := r.URL.Query().Get("limit"); limit != "" {
		var parsed int
		_, err := fmt.Sscanf(limit, "%d", &parsed)
		if err == nil {
			filter.Limit = parsed
		}
	}
	if offset := r.URL.Query().Get("offset"); offset != "" {
		var parsed int
		_, err := fmt.Sscanf(offset, "%d", &parsed)
		if err == nil {
			filter.Offset = parsed
		}
	}

	writeJSON(w, http.StatusOK, s.store.List(filter))
}

func (s *Server) handleGetJob(w http.ResponseWriter, _ *http.Request, jobID string) {
	job, ok := s.store.Get(jobID)
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "job not found", nil)
		return
	}
	writeJSON(w, http.StatusOK, job)
}

func (s *Server) handleGetJobResult(w http.ResponseWriter, _ *http.Request, jobID string) {
	result, ok := s.store.GetResult(jobID)
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "job result not found", nil)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleCancelJob(w http.ResponseWriter, _ *http.Request, jobID string) {
	job, ok := s.store.Get(jobID)
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "job not found", nil)
		return
	}
	job.Status = JobCanceled
	job.FinishedAt = time.Now().UTC()
	if err := s.store.Update(job); err != nil {
		writeError(w, http.StatusInternalServerError, "store_error", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, job)
}

func (s *Server) handleJobEvents(w http.ResponseWriter, r *http.Request, jobID string) {
	_, ok := s.store.Get(jobID)
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "job not found", nil)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)

	if _, err := fmt.Fprintf(w, "event: state\ndata: {\"status\":\"queued\"}\n\n"); err != nil {
		return
	}
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}

	<-r.Context().Done()
}

func (s *Server) handleQueuePause(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
		return
	}
	if err := s.queue.Pause(); err != nil {
		writeError(w, http.StatusInternalServerError, "queue_error", err.Error(), nil)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleQueueResume(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
		return
	}
	if err := s.queue.Resume(); err != nil {
		writeError(w, http.StatusInternalServerError, "queue_error", err.Error(), nil)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleQueueReorder(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
		return
	}
	var req QueueReorderRequest
	if err := readJSON(r, s.cfg.MaxBodySize, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", err.Error(), nil)
		return
	}
	if err := s.queue.Reorder(req.JobIDs); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_queue", err.Error(), nil)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
		return
	}
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("# TODO: metrics\n"))
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
