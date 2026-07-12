package server

import (
	"net/http"
	"strconv"
	"strings"
)

// handleListRuns handles GET /api/v1/runs.
// Accepts: tag, event_tag, domain, batch, status, level, finished_after, finished_before, limit, offset.
func (s *Server) handleListRuns(w http.ResponseWriter, r *http.Request) {
	filter := RunFilter{Limit: 100}
	q := r.URL.Query()
	filter.Tag = strings.TrimSpace(q.Get("tag"))
	filter.EntryTag = strings.TrimSpace(q.Get("event_tag"))
	filter.Domain = strings.TrimSpace(q.Get("domain"))
	filter.BatchID = strings.TrimSpace(q.Get("batch"))
	if v := strings.TrimSpace(q.Get("status")); v != "" {
		filter.Status = JobStatus(v)
	}
	filter.WorstLevel = strings.TrimSpace(q.Get("level"))
	filter.Grade = strings.TrimSpace(q.Get("grade"))

	if v := strings.TrimSpace(q.Get("finished_after")); v != "" {
		t, err := parseTime(v)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_finished_after", "finished_after must be RFC3339", nil)
			return
		}
		filter.FinishedAfter = t
	}
	if v := strings.TrimSpace(q.Get("finished_before")); v != "" {
		t, err := parseTime(v)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_finished_before", "finished_before must be RFC3339", nil)
			return
		}
		filter.FinishedBefore = t
	}
	if !filter.FinishedAfter.IsZero() && !filter.FinishedBefore.IsZero() && filter.FinishedBefore.Before(filter.FinishedAfter) {
		writeError(w, http.StatusBadRequest, "invalid_time_range", "finished_before must be >= finished_after", nil)
		return
	}

	if limitRaw := strings.TrimSpace(q.Get("limit")); limitRaw != "" {
		v, err := strconv.Atoi(limitRaw)
		if err != nil || v <= 0 || v > maxListLimit {
			writeError(w, http.StatusBadRequest, "invalid_limit", "limit must be between 1 and 500", nil)
			return
		}
		filter.Limit = v
	}
	if offsetRaw := strings.TrimSpace(q.Get("offset")); offsetRaw != "" {
		v, err := strconv.Atoi(offsetRaw)
		if err != nil || v < 0 {
			writeError(w, http.StatusBadRequest, "invalid_offset", "offset must be a non-negative integer", nil)
			return
		}
		filter.Offset = v
	}

	list := s.store.ListRuns(filter)
	if !s.cfg.ShowScoreAdmin {
		for i := range list.Items {
			list.Items[i].Score = nil
			list.Items[i].Grade = nil
		}
	}
	writeJSON(w, http.StatusOK, list)
}

// handleGetRun handles GET /api/v1/runs/{id}.
func (s *Server) handleGetRun(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	run, ok := s.store.GetRun(id)
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "run not found", nil)
		return
	}
	if !s.cfg.ShowScoreAdmin {
		run.Score = nil
		run.Grade = nil
	}
	writeJSON(w, http.StatusOK, run)
}

// handleGetRunResult handles GET /api/v1/runs/{id}/result.
// Returns the same JSON shape as GET /api/v1/jobs/{id}/result.
func (s *Server) handleGetRunResult(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	result, ok := s.store.GetResult(id)
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "run not found", nil)
		return
	}
	if result.Raw != nil && len(result.Raw.Entries) > 0 {
		locale := strings.TrimSpace(r.URL.Query().Get("locale"))
		if locale == "" {
			locale = "en"
		}
		raw := *result.Raw
		raw.Locale = locale
		raw.Entries = localizeResultEntries(result.Raw.Entries, locale)
		result.Raw = &raw
	}
	if !s.cfg.ShowScoreAdmin {
		result.Score = nil
	}
	if !s.cfg.ShowNameserverTimingsAdmin {
		result.NameserverTimings = nil
	}
	writeJSON(w, http.StatusOK, result)
}

// handleGetRunDNSSECChain handles GET /api/v1/runs/{id}/dnssec-chain.
// Admins see stored data directly; only public runs ever have a blob.
func (s *Server) handleGetRunDNSSECChain(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	chain, ok, err := s.store.GetRunDNSSECChain(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "lookup_failed", "could not load DNSSEC chain data", nil)
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "no DNSSEC chain data for this run", nil)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(chain))
}
