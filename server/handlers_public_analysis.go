package server

import (
	"errors"
	"log"
	"net/http"
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
	// DefaultSnapshot carries the source timing and counts for the snapshot
	// auto-latest resolution picks for this cohort. Nil for cohorts with no
	// captured public snapshot yet.
	DefaultSnapshot *PublicAnalysisSnapshotView `json:"default_snapshot,omitempty"`
	// SnapshotCount is the total number of captured public snapshots in
	// the cohort. Used by the UI to decide whether to render the
	// snapshot selector chip.
	SnapshotCount int `json:"snapshot_count"`
}

// PublicAnalysisCatalogResponse is the response for GET /pub/api/v1/analysis/catalog.
type PublicAnalysisCatalogResponse struct {
	DefaultTag       string                     `json:"default_tag,omitempty"`
	Cohorts          []PublicAnalysisCohortView `json:"cohorts"`
	SelectorEnabled  bool                       `json:"selector_enabled"`
	BackendSupported bool                       `json:"backend_supported"`
}

// PublicAnalysisOverviewResponse is the consolidated overview payload for
// GET /pub/api/v1/analysis/overview. The Overview field carries the
// totals/distributions/top-N lists the overview tab renders, so the page
// loads with one read instead of fanning out to four endpoints.
type PublicAnalysisOverviewResponse struct {
	DatasetTag            string                      `json:"dataset_tag"`
	Label                 string                      `json:"label"`
	Description           string                      `json:"description,omitempty"`
	MaterializationStatus string                      `json:"materialization_status"`
	LastMaterializedAt    *time.Time                  `json:"last_materialized_at,omitempty"`
	IsDefault             bool                        `json:"is_default"`
	Snapshot              *PublicAnalysisSnapshotView `json:"snapshot,omitempty"`
	Status                string                      `json:"status,omitempty"`
	Overview              *SnapshotOverviewV2         `json:"overview,omitempty"`
}

func (s *Server) publicAnalysisCohortView(cohort AnalysisCohort) PublicAnalysisCohortView {
	view := PublicAnalysisCohortView{
		DatasetTag:  cohort.SourceTag,
		Label:       cohort.Label,
		Description: cohort.Description,
		IsDefault:   cohort.IsDefault,
		SortOrder:   cohort.SortOrder,
	}
	if readStore, ok := s.store.(AnalysisReadStore); ok {
		snapshots := readStore.ListAnalysisCohortSnapshots(cohort.ID)
		publicCount := 0
		for _, snap := range snapshots {
			if isPublicSnapshot(snap) {
				publicCount++
			}
		}
		view.SnapshotCount = publicCount
		if def, found := readStore.GetDefaultSnapshotForCohort(cohort.ID); found {
			snapshotView := publicAnalysisSnapshotView(def)
			view.DefaultSnapshot = &snapshotView
		}
	}
	return view
}

// handlePublicAnalysisCatalog handles GET /pub/api/v1/analysis/catalog.
func (s *Server) handlePublicAnalysisCatalog(w http.ResponseWriter, r *http.Request) {
	selectable, err := SelectableAnalysisCohorts(s.store.ListAnalysisCohorts())
	if err != nil && !errors.Is(err, ErrNoSelectableAnalysisCohorts) {
		log.Printf("public analysis catalog: %v", err)
		writeError(w, http.StatusInternalServerError, "invalid_catalog", "analysis catalog unavailable", nil)
		return
	}
	views := make([]PublicAnalysisCohortView, 0, len(selectable))
	var defaultTag string
	for _, c := range selectable {
		views = append(views, s.publicAnalysisCohortView(c))
		if c.IsDefault {
			defaultTag = c.SourceTag
		}
	}
	w.Header().Set("Cache-Control", "public, max-age=60")
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
		log.Printf("public analysis cohorts: %v", err)
		writeError(w, http.StatusInternalServerError, "invalid_catalog", "analysis catalog unavailable", nil)
		return
	}
	views := make([]PublicAnalysisCohortView, 0, len(selectable))
	for _, c := range selectable {
		views = append(views, s.publicAnalysisCohortView(c))
	}
	w.Header().Set("Cache-Control", "public, max-age=60")
	writeJSON(w, http.StatusOK, views)
}

// handlePublicAnalysisOverview handles GET /pub/api/v1/analysis/overview.
// When dataset_tag is missing the default public cohort is resolved
// server-side; when ?snapshot= is missing the cohort's auto-latest
// captured snapshot is used.
func (s *Server) handlePublicAnalysisOverview(w http.ResponseWriter, r *http.Request) {
	cohort, snapshot, ok := s.resolvePublicAnalysisCohortAndSnapshot(w, r)
	if !ok {
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
	if snapshot.ID == 0 {
		resp.Status = PublicAnalysisStatusNoSnapshot
	} else {
		snapshotView := publicAnalysisSnapshotView(snapshot)
		resp.Snapshot = &snapshotView
		if readStore, canRead := s.store.(AnalysisReadStore); canRead {
			if overview, ok := loadSnapshotOverviewV2(readStore, snapshot.ID); ok {
				resp.Overview = &overview
			}
		}
	}
	writeSnapshotCacheHeaders(w, r, snapshot)
	writeJSON(w, http.StatusOK, resp)
}

// loadSnapshotOverviewV2 reads the per-snapshot overview row. Returns
// ok=false when the row is missing so callers can fall back gracefully.
func loadSnapshotOverviewV2(store AnalysisReadStore, snapshotID int64) (SnapshotOverviewV2, bool) {
	return store.GetSnapshotOverview(snapshotID)
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
		log.Printf("public analysis: invalid catalog: %v", err)
		writeError(w, http.StatusInternalServerError, "invalid_catalog", "analysis catalog unavailable", nil)
	default:
		log.Printf("public analysis: catalog error: %v", err)
		writeError(w, http.StatusInternalServerError, "catalog_error", "analysis request failed", nil)
	}
}
