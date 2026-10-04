package server

import (
	"net/http"
	"time"
)

// schedulePreviewCount is how many occurrences the preview returns.
const schedulePreviewCount = 5

// AnalysisCohortSchedulePutRequest is the body of PUT /analysis/cohorts/{id}/schedule.
type AnalysisCohortSchedulePutRequest struct {
	ScheduleRuleFields
	// Enabled and CatchUp default to true when absent.
	Enabled        *bool  `json:"enabled"`
	ProfileID      *int64 `json:"profile_id"`
	PromoteDefault bool   `json:"promote_default"`
	CatchUp        *bool  `json:"catch_up"`
}

// SchedulePreviewResponse lists the next occurrences of a rule.
type SchedulePreviewResponse struct {
	Summary string      `json:"summary"`
	Next    []time.Time `json:"next"`
}

// requireAnalysis writes 503 when no analysis controller runs the scheduler.
func (s *Server) requireAnalysis(w http.ResponseWriter) bool {
	if s.analysis == nil {
		writeError(w, http.StatusServiceUnavailable, "analysis_unavailable", "analysis controller not configured", nil)
		return false
	}
	return true
}

// handleAnalysisCohortSchedule routes GET, PUT and DELETE on /analysis/cohorts/{id}/schedule.
func (s *Server) handleAnalysisCohortSchedule(w http.ResponseWriter, r *http.Request) {
	id, ok := parseCohortID(w, r)
	if !ok {
		return
	}
	cohort, found := s.store.GetAnalysisCohort(id)
	if !found {
		writeError(w, http.StatusNotFound, "not_found", "cohort not found", nil)
		return
	}
	if !s.requireAnalysis(w) {
		return
	}
	switch r.Method {
	case http.MethodGet:
		sched, ok := s.store.GetAnalysisCohortSchedule(id)
		if !ok {
			writeError(w, http.StatusNotFound, "no_schedule", "cohort has no schedule", nil)
			return
		}
		writeJSON(w, http.StatusOK, sched)
	case http.MethodPut:
		s.handlePutAnalysisCohortSchedule(w, r, cohort)
	case http.MethodDelete:
		if err := s.store.DeleteAnalysisCohortSchedule(id); err != nil {
			writeError(w, http.StatusInternalServerError, "store_error", err.Error(), nil)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
	}
}

// handlePutAnalysisCohortSchedule creates or replaces a schedule; next_run_at restarts from now.
func (s *Server) handlePutAnalysisCohortSchedule(w http.ResponseWriter, r *http.Request, cohort AnalysisCohort) {
	var req AnalysisCohortSchedulePutRequest
	if err := readJSON(r, s.cfg.MaxBodySize, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", err.Error(), nil)
		return
	}
	if !cohort.AnalysisEnabled {
		writeError(w, http.StatusConflict, "cohort_analysis_disabled", "analysis is disabled for this cohort", nil)
		return
	}
	rule, err := req.Rule()
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_schedule", err.Error(), nil)
		return
	}
	if _, code, message := s.resolveStoredProfile(req.ProfileID, false); code != "" {
		writeError(w, http.StatusBadRequest, code, message, nil)
		return
	}
	next, err := rule.Next(time.Now().UTC())
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_schedule", err.Error(), nil)
		return
	}
	stored, err := s.store.PutAnalysisCohortSchedule(AnalysisCohortSchedule{
		CohortID:       cohort.ID,
		Enabled:        derefBoolOr(req.Enabled, true),
		Rule:           rule,
		ProfileID:      req.ProfileID,
		PromoteDefault: req.PromoteDefault,
		CatchUp:        derefBoolOr(req.CatchUp, true),
		NextRunAt:      next,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "store_error", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, stored)
}

// handleAnalysisSchedules handles GET /analysis/schedules.
func (s *Server) handleAnalysisSchedules(w http.ResponseWriter, _ *http.Request) {
	if !s.requireAnalysis(w) {
		return
	}
	cohorts := map[int64]AnalysisCohort{}
	for _, c := range s.store.ListAnalysisCohorts() {
		cohorts[c.ID] = c
	}
	out := []AnalysisCohortSchedule{}
	for _, sched := range s.store.ListAnalysisCohortSchedules() {
		sched.SourceTag, sched.Label = cohorts[sched.CohortID].SourceTag, cohorts[sched.CohortID].Label
		out = append(out, sched)
	}
	writeJSON(w, http.StatusOK, out)
}

// handleAnalysisSchedulePreview handles POST /analysis/schedules/preview.
func (s *Server) handleAnalysisSchedulePreview(w http.ResponseWriter, r *http.Request) {
	if !s.requireAnalysis(w) {
		return
	}
	var req AnalysisCohortSchedulePutRequest
	if err := readJSON(r, s.cfg.MaxBodySize, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", err.Error(), nil)
		return
	}
	rule, err := req.Rule()
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_schedule", err.Error(), nil)
		return
	}
	next, err := rule.Preview(time.Now().UTC(), schedulePreviewCount)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_schedule", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, SchedulePreviewResponse{Summary: rule.String(), Next: next})
}

func derefBoolOr(p *bool, fallback bool) bool {
	if p == nil {
		return fallback
	}
	return *p
}
