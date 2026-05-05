package server

import (
	"net/http"
	"strings"
	"time"
)

// adminSnapshotStore is the narrow store surface the admin snapshot
// handlers need. The in-memory store does not persist snapshot rows, so
// the handlers type-assert at the entry point rather than widening the
// JobStore interface and forcing a no-op implementation.
type adminSnapshotStore interface {
	GetAnalysisCohortSnapshotBySlug(cohortID int64, slug string) (AnalysisCohortSnapshot, bool)
	ListAnalysisCohortSnapshots(cohortID int64) []AnalysisCohortSnapshot
	UpsertAnalysisCohortSnapshot(snap AnalysisCohortSnapshot) (AnalysisCohortSnapshot, error)
}

// adminSnapshotPurger is the optional surface the purge action needs.
type adminSnapshotPurger interface {
	DeleteAnalysisCohortSnapshot(id int64) error
}

// adminSnapshotAggregator is the optional surface the rematerialize
// action needs.
type adminSnapshotAggregator interface {
	ComputeSnapshotOverview(cohortID int64, batchID string) (SnapshotOverviewV2, error)
	ReplaceSnapshotOverview(snapshotID int64, overview SnapshotOverviewV2) error
	ComputeSnapshotEntityViews(cohortID int64, batchID string, minLevel string) (SnapshotEntityViews, error)
	ReplaceSnapshotEntityViews(snapshotID int64, views SnapshotEntityViews) error
	TagViewMinLevel() string
}

// AdminAnalysisSnapshotView is the admin shape for one snapshot row. It
// carries the internal id and the lifecycle flags the admin UI edits;
// the public surface redacts these via PublicAnalysisSnapshotView.
type AdminAnalysisSnapshotView struct {
	ID                  int64     `json:"id"`
	CohortID            int64     `json:"cohort_id"`
	BatchID             string    `json:"batch_id"`
	Slug                string    `json:"slug"`
	Label               string    `json:"label,omitempty"`
	Description         string    `json:"description,omitempty"`
	ProfileID           *int64    `json:"profile_id,omitempty"`
	ProfileName         string    `json:"profile_name,omitempty"`
	CapturedAt          time.Time `json:"captured_at"`
	FirstRunAt          time.Time `json:"first_run_at"`
	LastRunAt           time.Time `json:"last_run_at"`
	RunCount            int       `json:"run_count"`
	DomainCount         int       `json:"domain_count"`
	Status              string    `json:"status"`
	IsDefault           bool      `json:"is_default"`
	IsPublic            bool      `json:"is_public"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
	SourceRunsAvailable bool      `json:"source_runs_available"`
}

func adminAnalysisSnapshotView(snap AnalysisCohortSnapshot, sourceRunsAvailable bool) AdminAnalysisSnapshotView {
	return AdminAnalysisSnapshotView{
		ID:                  snap.ID,
		CohortID:            snap.CohortID,
		BatchID:             snap.BatchID,
		Slug:                snap.Slug,
		Label:               snap.Label,
		Description:         snap.Description,
		ProfileID:           snap.ProfileID,
		ProfileName:         snap.ProfileName,
		CapturedAt:          snap.CapturedAt,
		FirstRunAt:          snap.FirstRunAt,
		LastRunAt:           snap.LastRunAt,
		RunCount:            snap.RunCount,
		DomainCount:         snap.DomainCount,
		Status:              snap.Status,
		IsDefault:           snap.IsDefault,
		IsPublic:            snap.IsPublic,
		CreatedAt:           snap.CreatedAt,
		UpdatedAt:           snap.UpdatedAt,
		SourceRunsAvailable: sourceRunsAvailable,
	}
}

// handleAnalysisCohortSnapshots handles GET on
// /api/v1/analysis/cohorts/{id}/snapshots. Returns every snapshot for
// the cohort - including retired and mixed-profile rows the public
// API hides - so the admin UI can render its full management list.
func (s *Server) handleAnalysisCohortSnapshots(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
		return
	}
	id, ok := parseCohortID(w, r)
	if !ok {
		return
	}
	cohort, found := s.store.GetAnalysisCohort(id)
	if !found {
		writeError(w, http.StatusNotFound, "not_found", "cohort not found", nil)
		return
	}
	store, ok := s.adminSnapshotStore(w)
	if !ok {
		return
	}
	snapshots := store.ListAnalysisCohortSnapshots(cohort.ID)
	out := make([]AdminAnalysisSnapshotView, 0, len(snapshots))
	for _, snap := range snapshots {
		out = append(out, adminAnalysisSnapshotView(snap, s.store.BatchHasRuns(snap.BatchID)))
	}
	writeJSON(w, http.StatusOK, out)
}

// handleAnalysisCohortSnapshotByID routes POST and DELETE on
// /api/v1/analysis/cohorts/{id}/snapshots/{slug}. POST edits the
// snapshot's admin-facing fields (label, description, is_public,
// is_default); DELETE retires the snapshot soft by default and hard-
// deletes when ?purge=true is supplied.
func (s *Server) handleAnalysisCohortSnapshotByID(w http.ResponseWriter, r *http.Request) {
	store, ok := s.adminSnapshotStore(w)
	if !ok {
		return
	}
	cohort, snap, ok := s.resolveAdminCohortSnapshot(w, r, store)
	if !ok {
		return
	}
	switch r.Method {
	case http.MethodPost:
		if !s.enforceCSRF(w, r) {
			return
		}
		s.handlePatchAnalysisCohortSnapshot(w, r, store, cohort, snap)
	case http.MethodDelete:
		if !s.enforceCSRF(w, r) {
			return
		}
		s.handleRetireAnalysisCohortSnapshot(w, r, store, cohort, snap)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
	}
}

// handleAnalysisCohortSnapshotRematerialize handles POST on
// /api/v1/analysis/cohorts/{id}/snapshots/{slug}/rematerialize. Rebuilds
// the overview row and entity views from current facts, bumps
// CapturedAt, and returns the refreshed snapshot row.
func (s *Server) handleAnalysisCohortSnapshotRematerialize(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
		return
	}
	if !s.enforceCSRF(w, r) {
		return
	}
	store, ok := s.adminSnapshotStore(w)
	if !ok {
		return
	}
	cohort, snap, ok := s.resolveAdminCohortSnapshot(w, r, store)
	if !ok {
		return
	}
	if !s.store.BatchHasRuns(snap.BatchID) {
		writeError(w, http.StatusConflict, "source_runs_purged",
			"source job data for this snapshot has been purged; rematerialize is no longer possible", nil)
		return
	}
	aggWriter, ok := s.store.(adminSnapshotAggregator)
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "analysis_unavailable", "analysis store does not support rematerialize", nil)
		return
	}
	overview, err := aggWriter.ComputeSnapshotOverview(cohort.ID, snap.BatchID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "compute_failed", err.Error(), nil)
		return
	}
	if err := aggWriter.ReplaceSnapshotOverview(snap.ID, overview); err != nil {
		writeError(w, http.StatusInternalServerError, "write_failed", err.Error(), nil)
		return
	}
	floor := cohort.TagViewMinLevel
	if !IsValidTagViewMinLevel(floor) {
		floor = aggWriter.TagViewMinLevel()
	}
	views, err := aggWriter.ComputeSnapshotEntityViews(cohort.ID, snap.BatchID, floor)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "compute_failed", err.Error(), nil)
		return
	}
	if err := aggWriter.ReplaceSnapshotEntityViews(snap.ID, views); err != nil {
		writeError(w, http.StatusInternalServerError, "write_failed", err.Error(), nil)
		return
	}
	snap.TagViewMinLevel = floor
	snap.CapturedAt = time.Now().UTC()
	snap.UpdatedAt = snap.CapturedAt
	if _, err := store.UpsertAnalysisCohortSnapshot(snap); err != nil {
		writeError(w, http.StatusInternalServerError, "store_error", err.Error(), nil)
		return
	}
	refreshed, _ := store.GetAnalysisCohortSnapshotBySlug(cohort.ID, snap.Slug)
	writeJSON(w, http.StatusOK, adminAnalysisSnapshotView(refreshed, s.store.BatchHasRuns(refreshed.BatchID)))
}

// handlePatchAnalysisCohortSnapshot applies a partial update to one
// snapshot row. Renaming via slug is allowed but constrained to be
// unique within the cohort; is_default transitions pin or unpin the
// cohort's default_snapshot_policy.
func (s *Server) handlePatchAnalysisCohortSnapshot(w http.ResponseWriter, r *http.Request, store adminSnapshotStore, cohort AnalysisCohort, snap AnalysisCohortSnapshot) {
	var req struct {
		Slug        *string `json:"slug,omitempty"`
		Label       *string `json:"label,omitempty"`
		Description *string `json:"description,omitempty"`
		IsPublic    *bool   `json:"is_public,omitempty"`
		IsDefault   *bool   `json:"is_default,omitempty"`
		// Status lets the admin UI restore a retired snapshot. Only
		// `captured` (restore) and `retired` (re-retire) are honoured;
		// the pending and failed_mixed_profiles lifecycle states are
		// projector-owned and not admin-writable.
		Status *string `json:"status,omitempty"`
	}
	if err := readJSON(r, s.cfg.MaxBodySize, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", err.Error(), nil)
		return
	}
	desired := snap
	if req.Slug != nil {
		newSlug := strings.TrimSpace(*req.Slug)
		if newSlug == "" {
			writeError(w, http.StatusBadRequest, "invalid_slug", "slug must not be empty", nil)
			return
		}
		if newSlug != snap.Slug {
			if existing, found := store.GetAnalysisCohortSnapshotBySlug(cohort.ID, newSlug); found && existing.ID != snap.ID {
				writeError(w, http.StatusConflict, "slug_exists", "slug is already in use within this cohort", nil)
				return
			}
			desired.Slug = newSlug
		}
	}
	if req.Label != nil {
		desired.Label = strings.TrimSpace(*req.Label)
	}
	if req.Description != nil {
		desired.Description = strings.TrimSpace(*req.Description)
	}
	if req.IsPublic != nil {
		desired.IsPublic = *req.IsPublic
	}
	if req.IsDefault != nil {
		desired.IsDefault = *req.IsDefault
	}
	if req.Status != nil {
		next := strings.ToLower(strings.TrimSpace(*req.Status))
		switch next {
		case AnalysisSnapshotStatusCaptured, AnalysisSnapshotStatusRetired:
			desired.Status = next
		default:
			writeError(w, http.StatusBadRequest, "invalid_status",
				"status must be one of captured, retired", nil)
			return
		}
	}

	if _, err := store.UpsertAnalysisCohortSnapshot(desired); err != nil {
		writeError(w, http.StatusInternalServerError, "store_error", err.Error(), nil)
		return
	}
	if req.IsDefault != nil {
		if *req.IsDefault {
			if err := s.pinCohortDefaultSnapshot(store, cohort, desired.ID); err != nil {
				writeError(w, http.StatusInternalServerError, "store_error", err.Error(), nil)
				return
			}
		} else if snap.IsDefault {
			if err := s.unpinCohortDefaultSnapshot(store, cohort, desired.ID); err != nil {
				writeError(w, http.StatusInternalServerError, "store_error", err.Error(), nil)
				return
			}
		}
	}
	refreshed, _ := store.GetAnalysisCohortSnapshotBySlug(cohort.ID, desired.Slug)
	writeJSON(w, http.StatusOK, adminAnalysisSnapshotView(refreshed, s.store.BatchHasRuns(refreshed.BatchID)))
}

// handleRetireAnalysisCohortSnapshot soft-deletes a snapshot by default
// (status=retired, is_public=false). When ?purge=true is supplied, the
// snapshot row and its aggregates are hard-deleted instead.
func (s *Server) handleRetireAnalysisCohortSnapshot(w http.ResponseWriter, r *http.Request, store adminSnapshotStore, cohort AnalysisCohort, snap AnalysisCohortSnapshot) {
	purge := strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("purge")), "true")
	if purge {
		purger, ok := s.store.(adminSnapshotPurger)
		if !ok {
			writeError(w, http.StatusServiceUnavailable, "analysis_unavailable", "analysis store does not support purge", nil)
			return
		}
		if err := purger.DeleteAnalysisCohortSnapshot(snap.ID); err != nil {
			writeError(w, http.StatusInternalServerError, "store_error", err.Error(), nil)
			return
		}
	} else {
		snap.Status = AnalysisSnapshotStatusRetired
		snap.IsPublic = false
		if _, err := store.UpsertAnalysisCohortSnapshot(snap); err != nil {
			writeError(w, http.StatusInternalServerError, "store_error", err.Error(), nil)
			return
		}
	}
	if snap.IsDefault {
		// Retiring or purging the pinned default snapshot would leave the
		// cohort with a dangling pin. Revert to auto_latest so the next
		// captured snapshot wins.
		if err := s.unpinCohortDefaultSnapshot(store, cohort, 0); err != nil {
			writeError(w, http.StatusInternalServerError, "store_error", err.Error(), nil)
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

// adminSnapshotStore type-asserts the job store to the admin snapshot
// surface. In-memory stores return a 503 since they don't persist
// snapshot rows; the admin UI already hides these controls when
// analysis_backend_supported is false.
func (s *Server) adminSnapshotStore(w http.ResponseWriter) (adminSnapshotStore, bool) {
	store, ok := s.store.(adminSnapshotStore)
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "analysis_unavailable", "analysis store not configured", nil)
		return nil, false
	}
	return store, true
}

// resolveAdminCohortSnapshot parses {id} + {slug} from the URL, looks up
// the cohort and snapshot rows, and writes a 404 when either is
// missing. Returns (cohort, snapshot, true) on the happy path.
func (s *Server) resolveAdminCohortSnapshot(w http.ResponseWriter, r *http.Request, store adminSnapshotStore) (AnalysisCohort, AnalysisCohortSnapshot, bool) {
	id, ok := parseCohortID(w, r)
	if !ok {
		return AnalysisCohort{}, AnalysisCohortSnapshot{}, false
	}
	cohort, found := s.store.GetAnalysisCohort(id)
	if !found {
		writeError(w, http.StatusNotFound, "not_found", "cohort not found", nil)
		return AnalysisCohort{}, AnalysisCohortSnapshot{}, false
	}
	slug := strings.TrimSpace(r.PathValue("slug"))
	if slug == "" {
		writeError(w, http.StatusNotFound, "not_found", "snapshot not found", nil)
		return AnalysisCohort{}, AnalysisCohortSnapshot{}, false
	}
	snap, found := store.GetAnalysisCohortSnapshotBySlug(cohort.ID, slug)
	if !found {
		writeError(w, http.StatusNotFound, "not_found", "snapshot not found", nil)
		return AnalysisCohort{}, AnalysisCohortSnapshot{}, false
	}
	return cohort, snap, true
}

// pinCohortDefaultSnapshot flips the cohort to pinned policy pointing at
// snapshotID, clears is_default on every other snapshot in the same
// cohort, and keeps snapshotID's is_default=true. Idempotent when
// called for the already-pinned snapshot.
func (s *Server) pinCohortDefaultSnapshot(store adminSnapshotStore, cohort AnalysisCohort, snapshotID int64) error {
	for _, existing := range store.ListAnalysisCohortSnapshots(cohort.ID) {
		if existing.ID == snapshotID || !existing.IsDefault {
			continue
		}
		existing.IsDefault = false
		if _, err := store.UpsertAnalysisCohortSnapshot(existing); err != nil {
			return err
		}
	}
	cohort.DefaultSnapshotPolicy = DefaultSnapshotPolicyPinned
	id := snapshotID
	cohort.DefaultSnapshotID = &id
	if _, err := s.store.UpsertAnalysisCohort(cohort); err != nil {
		return err
	}
	return nil
}

// unpinCohortDefaultSnapshot reverts the cohort to auto_latest and
// clears the default_snapshot_id pointer. exceptID lets the caller
// keep one snapshot's is_default flag set without clearing it (used
// when a different snapshot is being promoted in the same request);
// pass 0 to clear every snapshot's is_default.
func (s *Server) unpinCohortDefaultSnapshot(store adminSnapshotStore, cohort AnalysisCohort, exceptID int64) error {
	for _, existing := range store.ListAnalysisCohortSnapshots(cohort.ID) {
		if existing.ID == exceptID || !existing.IsDefault {
			continue
		}
		existing.IsDefault = false
		if _, err := store.UpsertAnalysisCohortSnapshot(existing); err != nil {
			return err
		}
	}
	cohort.DefaultSnapshotPolicy = DefaultSnapshotPolicyAutoLatest
	cohort.DefaultSnapshotID = nil
	if _, err := s.store.UpsertAnalysisCohort(cohort); err != nil {
		return err
	}
	return nil
}
