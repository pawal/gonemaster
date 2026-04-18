package server

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
)

// AnalysisStatusResponse reports whether the server is configured with a
// storage backend that supports analysis materialization.
type AnalysisStatusResponse struct {
	BackendSupported   bool   `json:"backend_supported"`
	ControllerEnabled  bool   `json:"controller_enabled"`
	UnsupportedMessage string `json:"unsupported_message,omitempty"`
}

// handleAnalysisStatus reports the configuration state of the analysis
// subsystem so the admin UI can warn when the current backend cannot persist
// materialized facts (e.g. the in-memory store).
func (s *Server) handleAnalysisStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
		return
	}
	resp := AnalysisStatusResponse{
		BackendSupported:  s.analysisBackendSupported(),
		ControllerEnabled: s.analysis != nil,
	}
	if !resp.BackendSupported {
		resp.UnsupportedMessage = "The current storage backend does not persist analysis facts. Start the server with --db-driver sqlite (or another SQL backend) to enable cohort materialization."
	}
	writeJSON(w, http.StatusOK, resp)
}

// handleAnalysisCohorts routes GET and POST on /api/v1/analysis/cohorts.
func (s *Server) handleAnalysisCohorts(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.store.ListAnalysisCohorts())
	case http.MethodPost:
		if !enforceCSRF(w, r) {
			return
		}
		s.handleCreateAnalysisCohort(w, r)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
	}
}

// handleCreateAnalysisCohort creates a tag-backed analysis cohort catalog entry.
func (s *Server) handleCreateAnalysisCohort(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SourceType      string `json:"source_type"`
		SourceTag       string `json:"source_tag"`
		Label           string `json:"label"`
		Description     string `json:"description"`
		AnalysisEnabled bool   `json:"analysis_enabled"`
		PublicEnabled   bool   `json:"public_enabled"`
		IsDefault       bool   `json:"is_default"`
		SortOrder       int    `json:"sort_order"`
	}
	if err := readJSON(r, s.cfg.MaxBodySize, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", err.Error(), nil)
		return
	}
	if strings.TrimSpace(req.SourceType) == "" {
		req.SourceType = "tag"
	}
	if req.SourceType != "tag" {
		writeError(w, http.StatusBadRequest, "unsupported_source_type", "only source_type=tag is supported in V1", nil)
		return
	}
	req.SourceTag = strings.TrimSpace(req.SourceTag)
	if req.SourceTag == "" {
		writeError(w, http.StatusBadRequest, "missing_source_tag", "source_tag is required", nil)
		return
	}
	if req.PublicEnabled && !req.AnalysisEnabled {
		writeError(w, http.StatusBadRequest, "invalid_catalog", "public_enabled=true requires analysis_enabled=true", nil)
		return
	}
	if req.IsDefault && (!req.PublicEnabled || !req.AnalysisEnabled) {
		writeError(w, http.StatusBadRequest, "invalid_catalog", "is_default=true requires analysis_enabled and public_enabled", nil)
		return
	}
	if _, exists := s.store.GetAnalysisCohortBySource(req.SourceType, req.SourceTag); exists {
		writeError(w, http.StatusConflict, "cohort_exists", "analysis cohort already exists for that source", nil)
		return
	}
	if req.SourceType == "tag" {
		if _, tagExists := s.store.GetTag(req.SourceTag); !tagExists {
			if err := s.store.CreateTag(req.SourceTag, ""); err != nil {
				writeError(w, http.StatusInternalServerError, "store_error", err.Error(), nil)
				return
			}
		}
	}
	beforeCatalog := s.store.ListAnalysisCohorts()
	created, err := s.store.UpsertAnalysisCohort(AnalysisCohort{
		SourceType:      req.SourceType,
		SourceTag:       req.SourceTag,
		Label:           strings.TrimSpace(req.Label),
		Description:     strings.TrimSpace(req.Description),
		AnalysisEnabled: req.AnalysisEnabled,
		PublicEnabled:   req.PublicEnabled,
		IsDefault:       req.IsDefault,
		SortOrder:       req.SortOrder,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "store_error", err.Error(), nil)
		return
	}
	if created.IsDefault {
		if err := s.enforceSingleDefaultCohort(created.ID); err != nil {
			s.writeAnalysisCatalogRollbackError(w, beforeCatalog, err)
			return
		}
	}
	if s.analysis != nil && created.AnalysisEnabled {
		if err := s.analysis.RebuildCohort(r.Context(), created.ID); err != nil {
			s.writeAnalysisCatalogRollbackError(w, beforeCatalog, err)
			return
		}
	}
	if latest, ok := s.store.GetAnalysisCohort(created.ID); ok {
		created = latest
	}
	writeJSON(w, http.StatusCreated, created)
}

// handleAnalysisCohortByID routes GET, PATCH, and DELETE on
// /api/v1/analysis/cohorts/{id}.
func (s *Server) handleAnalysisCohortByID(w http.ResponseWriter, r *http.Request) {
	id, ok := parseCohortID(w, r)
	if !ok {
		return
	}
	cohort, found := s.store.GetAnalysisCohort(id)
	if !found {
		writeError(w, http.StatusNotFound, "not_found", "cohort not found", nil)
		return
	}
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, cohort)
	case http.MethodPatch:
		if !enforceCSRF(w, r) {
			return
		}
		s.handlePatchAnalysisCohort(w, r, cohort)
	case http.MethodDelete:
		if !enforceCSRF(w, r) {
			return
		}
		s.handleDeleteAnalysisCohort(w, r, cohort)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
	}
}

// handleDeleteAnalysisCohort clears the cohort's materialized rows and removes
// the catalog entry entirely.
func (s *Server) handleDeleteAnalysisCohort(w http.ResponseWriter, r *http.Request, cohort AnalysisCohort) {
	if s.analysis != nil {
		if err := s.analysis.ClearCohort(r.Context(), cohort.ID); err != nil {
			writeError(w, http.StatusInternalServerError, "clear_failed", err.Error(), nil)
			return
		}
	}
	if err := s.store.DeleteAnalysisCohort(cohort.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "store_error", err.Error(), nil)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handlePatchAnalysisCohort applies a partial update to one cohort catalog row
// and reconciles any analysis_enabled transition through the controller.
func (s *Server) handlePatchAnalysisCohort(w http.ResponseWriter, r *http.Request, before AnalysisCohort) {
	var req struct {
		Label           *string `json:"label,omitempty"`
		Description     *string `json:"description,omitempty"`
		AnalysisEnabled *bool   `json:"analysis_enabled,omitempty"`
		PublicEnabled   *bool   `json:"public_enabled,omitempty"`
		IsDefault       *bool   `json:"is_default,omitempty"`
		SortOrder       *int    `json:"sort_order,omitempty"`
	}
	if err := readJSON(r, s.cfg.MaxBodySize, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", err.Error(), nil)
		return
	}
	desired := before
	if req.Label != nil {
		desired.Label = strings.TrimSpace(*req.Label)
	}
	if req.Description != nil {
		desired.Description = strings.TrimSpace(*req.Description)
	}
	if req.AnalysisEnabled != nil {
		desired.AnalysisEnabled = *req.AnalysisEnabled
	}
	if req.PublicEnabled != nil {
		desired.PublicEnabled = *req.PublicEnabled
	}
	if req.IsDefault != nil {
		desired.IsDefault = *req.IsDefault
	}
	if req.SortOrder != nil {
		desired.SortOrder = *req.SortOrder
	}
	if desired.PublicEnabled && !desired.AnalysisEnabled {
		writeError(w, http.StatusBadRequest, "invalid_catalog", "public_enabled=true requires analysis_enabled=true", nil)
		return
	}
	if desired.IsDefault && (!desired.PublicEnabled || !desired.AnalysisEnabled) {
		writeError(w, http.StatusBadRequest, "invalid_catalog", "is_default=true requires analysis_enabled and public_enabled", nil)
		return
	}
	beforeCatalog := s.store.ListAnalysisCohorts()
	updated, err := s.store.UpsertAnalysisCohort(desired)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "store_error", err.Error(), nil)
		return
	}
	if updated.IsDefault {
		if err := s.enforceSingleDefaultCohort(updated.ID); err != nil {
			s.writeAnalysisCatalogRollbackError(w, beforeCatalog, err)
			return
		}
	}
	if s.analysis != nil {
		if err := s.analysis.ReconcileCohortChange(r.Context(), before, updated); err != nil {
			s.writeAnalysisCatalogRollbackError(w, beforeCatalog, err)
			return
		}
	}
	if latest, ok := s.store.GetAnalysisCohort(updated.ID); ok {
		updated = latest
	}
	writeJSON(w, http.StatusOK, updated)
}

// handleAnalysisCohortRebuild triggers a full rebuild for one cohort.
func (s *Server) handleAnalysisCohortRebuild(w http.ResponseWriter, r *http.Request) {
	id, ok := parseCohortID(w, r)
	if !ok {
		return
	}
	if !enforceCSRF(w, r) {
		return
	}
	if _, found := s.store.GetAnalysisCohort(id); !found {
		writeError(w, http.StatusNotFound, "not_found", "cohort not found", nil)
		return
	}
	if s.analysis == nil {
		writeError(w, http.StatusServiceUnavailable, "analysis_unavailable", "analysis controller not configured", nil)
		return
	}
	if err := s.analysis.RebuildCohort(r.Context(), id); err != nil {
		if errors.Is(err, ErrAnalysisCohortNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "cohort not found", nil)
			return
		}
		writeError(w, http.StatusInternalServerError, "rebuild_failed", err.Error(), nil)
		return
	}
	cohort, _ := s.store.GetAnalysisCohort(id)
	writeJSON(w, http.StatusOK, cohort)
}

// handleAnalysisCohortClear removes all materialized rows for one cohort.
func (s *Server) handleAnalysisCohortClear(w http.ResponseWriter, r *http.Request) {
	id, ok := parseCohortID(w, r)
	if !ok {
		return
	}
	if !enforceCSRF(w, r) {
		return
	}
	if _, found := s.store.GetAnalysisCohort(id); !found {
		writeError(w, http.StatusNotFound, "not_found", "cohort not found", nil)
		return
	}
	if s.analysis == nil {
		writeError(w, http.StatusServiceUnavailable, "analysis_unavailable", "analysis controller not configured", nil)
		return
	}
	if err := s.analysis.ClearCohort(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, "clear_failed", err.Error(), nil)
		return
	}
	cohort, _ := s.store.GetAnalysisCohort(id)
	writeJSON(w, http.StatusOK, cohort)
}

// enforceSingleDefaultCohort clears is_default on every cohort except keepID.
func (s *Server) enforceSingleDefaultCohort(keepID int64) error {
	for _, c := range s.store.ListAnalysisCohorts() {
		if c.ID == keepID || !c.IsDefault {
			continue
		}
		c.IsDefault = false
		if _, err := s.store.UpsertAnalysisCohort(c); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) restoreAnalysisCohortCatalog(snapshot []AnalysisCohort) error {
	keep := make(map[int64]struct{}, len(snapshot))
	for _, cohort := range snapshot {
		keep[cohort.ID] = struct{}{}
		if _, err := s.store.UpsertAnalysisCohort(cohort); err != nil {
			return err
		}
	}
	for _, cohort := range s.store.ListAnalysisCohorts() {
		if _, ok := keep[cohort.ID]; ok {
			continue
		}
		if err := s.store.DeleteAnalysisCohort(cohort.ID); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) writeAnalysisCatalogRollbackError(w http.ResponseWriter, before []AnalysisCohort, cause error) {
	if rollbackErr := s.restoreAnalysisCohortCatalog(before); rollbackErr != nil {
		writeError(w, http.StatusInternalServerError, "analysis_rollback_failed",
			cause.Error()+"; rollback failed: "+rollbackErr.Error(), nil)
		return
	}
	writeError(w, http.StatusInternalServerError, "analysis_change_failed", cause.Error(), nil)
}

func parseCohortID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	raw := r.PathValue("id")
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusNotFound, "not_found", "cohort not found", nil)
		return 0, false
	}
	return id, true
}
