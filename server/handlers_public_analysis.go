package server

import (
	"errors"
	"net/http"
	"strings"
	"time"
)

// PublicAnalysisCohortView is the redacted public shape for one analysis
// cohort. It intentionally omits the numeric ID, materialization status,
// analysis_enabled, and public_enabled flags.
type PublicAnalysisCohortView struct {
	DatasetTag  string `json:"dataset_tag"`
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
	IsDefault   bool   `json:"is_default"`
	SortOrder   int    `json:"sort_order,omitempty"`
}

// PublicAnalysisCatalogResponse is the response for GET /pub/api/v1/analysis/catalog.
type PublicAnalysisCatalogResponse struct {
	DefaultTag       string                     `json:"default_tag,omitempty"`
	Cohorts          []PublicAnalysisCohortView `json:"cohorts"`
	SelectorEnabled  bool                       `json:"selector_enabled"`
	BackendSupported bool                       `json:"backend_supported"`
}

// PublicAnalysisOverviewResponse is the minimal overview payload for
// GET /pub/api/v1/analysis/overview. Aggregated counts and charts are added
// later once the materialized query surface is in place.
type PublicAnalysisOverviewResponse struct {
	DatasetTag            string     `json:"dataset_tag"`
	Label                 string     `json:"label"`
	Description           string     `json:"description,omitempty"`
	MaterializationStatus string     `json:"materialization_status"`
	LastMaterializedAt    *time.Time `json:"last_materialized_at,omitempty"`
	IsDefault             bool       `json:"is_default"`
}

func publicAnalysisCohortView(cohort AnalysisCohort) PublicAnalysisCohortView {
	return PublicAnalysisCohortView{
		DatasetTag:  cohort.SourceTag,
		Label:       cohort.Label,
		Description: cohort.Description,
		IsDefault:   cohort.IsDefault,
		SortOrder:   cohort.SortOrder,
	}
}

// handlePublicAnalysisCatalog handles GET /pub/api/v1/analysis/catalog.
func (s *Server) handlePublicAnalysisCatalog(w http.ResponseWriter, r *http.Request) {
	selectable, err := SelectableAnalysisCohorts(s.store.ListAnalysisCohorts())
	if err != nil && !errors.Is(err, ErrNoSelectableAnalysisCohorts) {
		writeError(w, http.StatusInternalServerError, "invalid_catalog", err.Error(), nil)
		return
	}
	views := make([]PublicAnalysisCohortView, 0, len(selectable))
	var defaultTag string
	for _, c := range selectable {
		views = append(views, publicAnalysisCohortView(c))
		if c.IsDefault {
			defaultTag = c.SourceTag
		}
	}
	writeJSON(w, http.StatusOK, PublicAnalysisCatalogResponse{
		DefaultTag:       defaultTag,
		Cohorts:          views,
		SelectorEnabled:  len(views) > 1,
		BackendSupported: s.analysisBackendSupported(),
	})
}

// handlePublicAnalysisCohorts handles GET /pub/api/v1/analysis/cohorts. It
// returns the selectable cohorts without the surrounding catalog envelope.
func (s *Server) handlePublicAnalysisCohorts(w http.ResponseWriter, r *http.Request) {
	selectable, err := SelectableAnalysisCohorts(s.store.ListAnalysisCohorts())
	if err != nil && !errors.Is(err, ErrNoSelectableAnalysisCohorts) {
		writeError(w, http.StatusInternalServerError, "invalid_catalog", err.Error(), nil)
		return
	}
	views := make([]PublicAnalysisCohortView, 0, len(selectable))
	for _, c := range selectable {
		views = append(views, publicAnalysisCohortView(c))
	}
	writeJSON(w, http.StatusOK, views)
}

// handlePublicAnalysisOverview handles GET /pub/api/v1/analysis/overview.
// When dataset_tag is missing the default public cohort is resolved server-side.
func (s *Server) handlePublicAnalysisOverview(w http.ResponseWriter, r *http.Request) {
	datasetTag := strings.TrimSpace(r.URL.Query().Get("dataset_tag"))
	cohort, err := ResolveAnalysisCohort(s.store.ListAnalysisCohorts(), datasetTag, "")
	if err != nil {
		writePublicAnalysisResolutionError(w, err)
		return
	}
	resp := PublicAnalysisOverviewResponse{
		DatasetTag:            cohort.SourceTag,
		Label:                 cohort.Label,
		Description:           cohort.Description,
		MaterializationStatus: cohort.MaterializationStatus,
		IsDefault:             cohort.IsDefault,
	}
	if !cohort.LastMaterializedAt.IsZero() {
		t := cohort.LastMaterializedAt
		resp.LastMaterializedAt = &t
	}
	writeJSON(w, http.StatusOK, resp)
}

// writePublicAnalysisResolutionError maps cohort-resolution sentinels to
// status codes suitable for the public API surface.
func writePublicAnalysisResolutionError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrNoSelectableAnalysisCohorts):
		writeError(w, http.StatusServiceUnavailable, "no_selectable_cohorts", "no public analysis cohorts are configured", nil)
	case errors.Is(err, ErrNoDefaultAnalysisCohort):
		writeError(w, http.StatusServiceUnavailable, "no_default_cohort", "no default public analysis cohort configured", nil)
	case errors.Is(err, ErrMultipleDefaultAnalysisCohorts):
		writeError(w, http.StatusInternalServerError, "multiple_default_cohorts", "multiple default public cohorts configured", nil)
	case errors.Is(err, ErrAnalysisCohortNotFound):
		writeError(w, http.StatusNotFound, "cohort_not_found", "requested dataset_tag is not a public analysis cohort", nil)
	case errors.Is(err, ErrInvalidAnalysisCohortCatalog):
		writeError(w, http.StatusInternalServerError, "invalid_catalog", err.Error(), nil)
	default:
		writeError(w, http.StatusInternalServerError, "catalog_error", err.Error(), nil)
	}
}
