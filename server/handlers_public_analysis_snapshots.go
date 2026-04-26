package server

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"time"
)

// PublicAnalysisSnapshotListEntry is one row in the /snapshots list
// response. Keyed on slug, which is the addressable identifier.
type PublicAnalysisSnapshotListEntry struct {
	Slug        string    `json:"slug"`
	Label       string    `json:"label,omitempty"`
	Description string    `json:"description,omitempty"`
	CapturedAt  time.Time `json:"captured_at"`
	FirstRunAt  time.Time `json:"first_run_at,omitempty"`
	LastRunAt   time.Time `json:"last_run_at,omitempty"`
	RunCount    int       `json:"run_count"`
	DomainCount int       `json:"domain_count"`
	ProfileName string    `json:"profile_name,omitempty"`
	IsDefault   bool      `json:"is_default,omitempty"`
}

// PublicAnalysisSnapshotListResponse envelopes the list + cohort anchor.
type PublicAnalysisSnapshotListResponse struct {
	DatasetTag string                            `json:"dataset_tag"`
	Label      string                            `json:"label"`
	Snapshots  []PublicAnalysisSnapshotListEntry `json:"snapshots"`
}

// PublicAnalysisSnapshotDetail is the /snapshots/{slug} response shape:
// metadata plus the captured aggregates the trends/diff views consume.
type PublicAnalysisSnapshotDetail struct {
	DatasetTag  string                     `json:"dataset_tag"`
	Slug        string                     `json:"slug"`
	Label       string                     `json:"label,omitempty"`
	Description string                     `json:"description,omitempty"`
	CapturedAt  time.Time                  `json:"captured_at"`
	FirstRunAt  time.Time                  `json:"first_run_at,omitempty"`
	LastRunAt   time.Time                  `json:"last_run_at,omitempty"`
	RunCount    int                        `json:"run_count"`
	DomainCount int                        `json:"domain_count"`
	ProfileName string                     `json:"profile_name,omitempty"`
	IsDefault   bool                       `json:"is_default"`
	Aggregates  map[string]json.RawMessage `json:"aggregates,omitempty"`
}

// PublicAnalysisTrendPoint is one (snapshot, payload) pair in a trend series.
type PublicAnalysisTrendPoint struct {
	Slug       string          `json:"slug"`
	Label      string          `json:"label,omitempty"`
	CapturedAt time.Time       `json:"captured_at"`
	FirstRunAt time.Time       `json:"first_run_at,omitempty"`
	LastRunAt  time.Time       `json:"last_run_at,omitempty"`
	Payload    json.RawMessage `json:"payload"`
}

// PublicAnalysisTrendResponse is the /trends response. One time series per
// request; callers asking for multiple categories make multiple requests.
type PublicAnalysisTrendResponse struct {
	DatasetTag string                     `json:"dataset_tag"`
	Category   string                     `json:"category"`
	Points     []PublicAnalysisTrendPoint `json:"points"`
}

// PublicAnalysisDiffEntry is one domain row in a /diff response.
type PublicAnalysisDiffEntry struct {
	Domain     string  `json:"domain"`
	FromGrade  *string `json:"from_grade,omitempty"`
	ToGrade    *string `json:"to_grade,omitempty"`
	FromLevel  string  `json:"from_level,omitempty"`
	ToLevel    string  `json:"to_level,omitempty"`
	WorstLevel string  `json:"worst_level,omitempty"`
}

// PublicAnalysisDiffResponse is the /diff response shape, grouped by the
// kind of change between the two snapshots.
type PublicAnalysisDiffResponse struct {
	DatasetTag   string                    `json:"dataset_tag"`
	FromSlug     string                    `json:"from_slug"`
	ToSlug       string                    `json:"to_slug"`
	Added        []PublicAnalysisDiffEntry `json:"added"`
	Removed      []PublicAnalysisDiffEntry `json:"removed"`
	GradeChanged []PublicAnalysisDiffEntry `json:"grade_changed"`
	LevelChanged []PublicAnalysisDiffEntry `json:"level_changed"`
}

// handlePublicAnalysisSnapshots handles GET /pub/api/v1/analysis/cohorts/{dataset_tag}/snapshots.
// Returns every captured public snapshot for the cohort, newest-first.
func (s *Server) handlePublicAnalysisSnapshots(w http.ResponseWriter, r *http.Request) {
	datasetTag, ok := pathValueNonEmpty(w, r, "dataset_tag", "cohort")
	if !ok {
		return
	}
	cohort, err := ResolveAnalysisCohort(s.store.ListAnalysisCohorts(), datasetTag, "")
	if err != nil {
		writePublicAnalysisResolutionError(w, err)
		return
	}
	readStore, ok := s.analysisReadStore(w)
	if !ok {
		return
	}
	all := readStore.ListAnalysisCohortSnapshots(cohort.ID)
	var defaultID int64
	if def, found := readStore.GetDefaultSnapshotForCohort(cohort.ID); found {
		defaultID = def.ID
	}
	entries := make([]PublicAnalysisSnapshotListEntry, 0, len(all))
	for _, snap := range all {
		if !isPublicSnapshot(snap) {
			continue
		}
		entries = append(entries, PublicAnalysisSnapshotListEntry{
			Slug:        snap.Slug,
			Label:       snap.Label,
			Description: snap.Description,
			CapturedAt:  snap.CapturedAt,
			FirstRunAt:  snap.FirstRunAt,
			LastRunAt:   snap.LastRunAt,
			RunCount:    snap.RunCount,
			DomainCount: snap.DomainCount,
			ProfileName: snap.ProfileName,
			IsDefault:   snap.ID == defaultID,
		})
	}
	writeJSON(w, http.StatusOK, PublicAnalysisSnapshotListResponse{
		DatasetTag: cohort.SourceTag,
		Label:      cohort.Label,
		Snapshots:  entries,
	})
}

// handlePublicAnalysisSnapshotDetail handles GET /pub/api/v1/analysis/cohorts/{dataset_tag}/snapshots/{slug}.
func (s *Server) handlePublicAnalysisSnapshotDetail(w http.ResponseWriter, r *http.Request) {
	datasetTag, ok := pathValueNonEmpty(w, r, "dataset_tag", "cohort")
	if !ok {
		return
	}
	slug, ok := pathValueNonEmpty(w, r, "slug", "snapshot")
	if !ok {
		return
	}
	cohort, err := ResolveAnalysisCohort(s.store.ListAnalysisCohorts(), datasetTag, "")
	if err != nil {
		writePublicAnalysisResolutionError(w, err)
		return
	}
	readStore, ok := s.analysisReadStore(w)
	if !ok {
		return
	}
	snap, found := readStore.GetAnalysisCohortSnapshotBySlug(cohort.ID, slug)
	if !found || !isPublicSnapshot(snap) {
		writeError(w, http.StatusNotFound, "snapshot_not_found",
			"requested snapshot is not available on the public path", nil)
		return
	}
	aggregates := map[string]json.RawMessage{}
	if overview, ok := readStore.GetSnapshotOverview(snap.ID); ok {
		if payloads, err := overview.AsCategoryPayloads(); err == nil {
			aggregates = payloads
		}
	}
	def, _ := readStore.GetDefaultSnapshotForCohort(cohort.ID)
	resp := PublicAnalysisSnapshotDetail{
		DatasetTag:  cohort.SourceTag,
		Slug:        snap.Slug,
		Label:       snap.Label,
		Description: snap.Description,
		CapturedAt:  snap.CapturedAt,
		FirstRunAt:  snap.FirstRunAt,
		LastRunAt:   snap.LastRunAt,
		RunCount:    snap.RunCount,
		DomainCount: snap.DomainCount,
		ProfileName: snap.ProfileName,
		IsDefault:   snap.ID == def.ID,
		Aggregates:  aggregates,
	}
	writeJSON(w, http.StatusOK, resp)
}

// handlePublicAnalysisTrends handles GET /pub/api/v1/analysis/cohorts/{dataset_tag}/trends?category=&from=&to=.
// Returns one time series of overview payloads, sliced to the requested
// category. Defaults to severity_distribution so the landing-page chart
// works without a parameter.
func (s *Server) handlePublicAnalysisTrends(w http.ResponseWriter, r *http.Request) {
	datasetTag, ok := pathValueNonEmpty(w, r, "dataset_tag", "cohort")
	if !ok {
		return
	}
	cohort, err := ResolveAnalysisCohort(s.store.ListAnalysisCohorts(), datasetTag, "")
	if err != nil {
		writePublicAnalysisResolutionError(w, err)
		return
	}
	readStore, ok := s.analysisReadStore(w)
	if !ok {
		return
	}
	category := strings.TrimSpace(r.URL.Query().Get("category"))
	if category == "" {
		category = SnapshotAggregateSeverityDistribution
	}
	fromSlug := strings.TrimSpace(r.URL.Query().Get("from"))
	toSlug := strings.TrimSpace(r.URL.Query().Get("to"))

	all := readStore.ListAnalysisCohortSnapshots(cohort.ID)
	// Order oldest-first by the source batch's run window, not by the
	// later moment the snapshot row was captured.
	sort.SliceStable(all, func(i, j int) bool {
		left := analysisSnapshotSourceTime(all[i])
		right := analysisSnapshotSourceTime(all[j])
		if left.Equal(right) {
			return all[i].ID < all[j].ID
		}
		return left.Before(right)
	})
	snapshotIDs := make([]int64, 0, len(all))
	for _, snap := range all {
		if isPublicSnapshot(snap) {
			snapshotIDs = append(snapshotIDs, snap.ID)
		}
	}
	overviews := readStore.ListSnapshotOverviewsByIDs(snapshotIDs)
	points := make([]PublicAnalysisTrendPoint, 0, len(all))
	inRange := fromSlug == ""
	for _, snap := range all {
		if !isPublicSnapshot(snap) {
			continue
		}
		if !inRange {
			if snap.Slug == fromSlug {
				inRange = true
			} else {
				continue
			}
		}
		overview, ok := overviews[snap.ID]
		if !ok {
			if toSlug != "" && snap.Slug == toSlug {
				break
			}
			continue
		}
		payloads, err := overview.AsCategoryPayloads()
		if err != nil {
			if toSlug != "" && snap.Slug == toSlug {
				break
			}
			continue
		}
		payload, has := payloads[category]
		if !has {
			if toSlug != "" && snap.Slug == toSlug {
				break
			}
			continue
		}
		points = append(points, PublicAnalysisTrendPoint{
			Slug:       snap.Slug,
			Label:      snap.Label,
			CapturedAt: snap.CapturedAt,
			FirstRunAt: snap.FirstRunAt,
			LastRunAt:  snap.LastRunAt,
			Payload:    payload,
		})
		if toSlug != "" && snap.Slug == toSlug {
			break
		}
	}
	writeJSON(w, http.StatusOK, PublicAnalysisTrendResponse{
		DatasetTag: cohort.SourceTag,
		Category:   category,
		Points:     points,
	})
}

// handlePublicAnalysisDiff handles GET /pub/api/v1/analysis/cohorts/{dataset_tag}/diff?from=&to=.
// Returns the delta between two snapshots: domains added, removed, with
// grade changes, or with worst-level changes.
func (s *Server) handlePublicAnalysisDiff(w http.ResponseWriter, r *http.Request) {
	datasetTag, ok := pathValueNonEmpty(w, r, "dataset_tag", "cohort")
	if !ok {
		return
	}
	cohort, err := ResolveAnalysisCohort(s.store.ListAnalysisCohorts(), datasetTag, "")
	if err != nil {
		writePublicAnalysisResolutionError(w, err)
		return
	}
	readStore, ok := s.analysisReadStore(w)
	if !ok {
		return
	}
	fromSlug := strings.TrimSpace(r.URL.Query().Get("from"))
	toSlug := strings.TrimSpace(r.URL.Query().Get("to"))
	if fromSlug == "" || toSlug == "" {
		writeError(w, http.StatusBadRequest, "missing_snapshots",
			"diff requires both ?from=<slug> and ?to=<slug>", nil)
		return
	}
	fromSnap, ok := readStore.GetAnalysisCohortSnapshotBySlug(cohort.ID, fromSlug)
	if !ok || !isPublicSnapshot(fromSnap) {
		writeError(w, http.StatusNotFound, "snapshot_not_found",
			"from snapshot is not available on the public path", nil)
		return
	}
	toSnap, ok := readStore.GetAnalysisCohortSnapshotBySlug(cohort.ID, toSlug)
	if !ok || !isPublicSnapshot(toSnap) {
		writeError(w, http.StatusNotFound, "snapshot_not_found",
			"to snapshot is not available on the public path", nil)
		return
	}

	fromByName := indexDomainViewsByName(readStore.ListSnapshotDomainViews(fromSnap.ID))
	toByName := indexDomainViewsByName(readStore.ListSnapshotDomainViews(toSnap.ID))

	added := []PublicAnalysisDiffEntry{}
	removed := []PublicAnalysisDiffEntry{}
	gradeChanged := []PublicAnalysisDiffEntry{}
	levelChanged := []PublicAnalysisDiffEntry{}
	for name, to := range toByName {
		if _, ok := fromByName[name]; ok {
			continue
		}
		added = append(added, PublicAnalysisDiffEntry{
			Domain:     name,
			ToGrade:    optionalString(to.Grade),
			ToLevel:    to.WorstLevel,
			WorstLevel: to.WorstLevel,
		})
	}
	for name, from := range fromByName {
		if _, ok := toByName[name]; ok {
			continue
		}
		removed = append(removed, PublicAnalysisDiffEntry{
			Domain:     name,
			FromGrade:  optionalString(from.Grade),
			FromLevel:  from.WorstLevel,
			WorstLevel: from.WorstLevel,
		})
	}
	for name, to := range toByName {
		from, ok := fromByName[name]
		if !ok {
			continue
		}
		if from.Grade == to.Grade && strings.EqualFold(from.WorstLevel, to.WorstLevel) {
			continue
		}
		entry := PublicAnalysisDiffEntry{
			Domain:     name,
			FromGrade:  optionalString(from.Grade),
			ToGrade:    optionalString(to.Grade),
			FromLevel:  from.WorstLevel,
			ToLevel:    to.WorstLevel,
			WorstLevel: to.WorstLevel,
		}
		if from.Grade != to.Grade {
			gradeChanged = append(gradeChanged, entry)
		}
		if !strings.EqualFold(from.WorstLevel, to.WorstLevel) {
			levelChanged = append(levelChanged, entry)
		}
	}

	sortDiffEntries(added)
	sortDiffEntries(removed)
	sortDiffEntries(gradeChanged)
	sortDiffEntries(levelChanged)

	writeJSON(w, http.StatusOK, PublicAnalysisDiffResponse{
		DatasetTag:   cohort.SourceTag,
		FromSlug:     fromSnap.Slug,
		ToSlug:       toSnap.Slug,
		Added:        added,
		Removed:      removed,
		GradeChanged: gradeChanged,
		LevelChanged: levelChanged,
	})
}

func indexDomainViewsByName(rows []AnalysisSnapshotDomainView) map[string]AnalysisSnapshotDomainView {
	out := make(map[string]AnalysisSnapshotDomainView, len(rows))
	for _, row := range rows {
		if row.DomainName == "" {
			continue
		}
		out[row.DomainName] = row
	}
	return out
}

func optionalString(v string) *string {
	if v == "" {
		return nil
	}
	out := v
	return &out
}

func sortDiffEntries(items []PublicAnalysisDiffEntry) {
	sort.Slice(items, func(i, j int) bool { return items[i].Domain < items[j].Domain })
}

func stringPtrEqual(a, b *string) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}
