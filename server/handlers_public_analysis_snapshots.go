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
	for _, agg := range readStore.ListSnapshotAggregates(snap.ID) {
		aggregates[agg.Category] = json.RawMessage(agg.PayloadJSON)
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
// Returns one time series of aggregate payloads spanning captured public
// snapshots. Missing category defaults to severity_distribution so the
// common landing-page chart works without a parameter.
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
	// Order oldest-first by the source batch's run window, not by the later
	// moment when analysis aggregates were written.
	sort.SliceStable(all, func(i, j int) bool {
		left := analysisSnapshotSourceTime(all[i])
		right := analysisSnapshotSourceTime(all[j])
		if left.Equal(right) {
			return all[i].ID < all[j].ID
		}
		return left.Before(right)
	})
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
		for _, agg := range readStore.ListSnapshotAggregates(snap.ID) {
			if agg.Category != category {
				continue
			}
			points = append(points, PublicAnalysisTrendPoint{
				Slug:       snap.Slug,
				Label:      snap.Label,
				CapturedAt: snap.CapturedAt,
				FirstRunAt: snap.FirstRunAt,
				LastRunAt:  snap.LastRunAt,
				Payload:    json.RawMessage(agg.PayloadJSON),
			})
			break
		}
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

	fromData := s.latestMaterializationForSnapshot(cohort, fromSnap)
	toData := s.latestMaterializationForSnapshot(cohort, toSnap)

	type domainKey struct {
		id int64
	}
	fromByID := map[int64]AnalysisRunDomainSummary{}
	for _, pair := range fromData.latest {
		fromByID[pair.summary.DomainID] = pair.summary
	}
	toByID := map[int64]AnalysisRunDomainSummary{}
	for _, pair := range toData.latest {
		toByID[pair.summary.DomainID] = pair.summary
	}
	nameFor := func(id int64) string {
		if name, ok := fromData.domainNames[id]; ok {
			return name
		}
		return toData.domainNames[id]
	}

	added := []PublicAnalysisDiffEntry{}
	removed := []PublicAnalysisDiffEntry{}
	gradeChanged := []PublicAnalysisDiffEntry{}
	levelChanged := []PublicAnalysisDiffEntry{}
	for id, toSum := range toByID {
		if _, ok := fromByID[id]; ok {
			continue
		}
		added = append(added, PublicAnalysisDiffEntry{
			Domain:     nameFor(id),
			ToGrade:    toSum.Grade,
			ToLevel:    toSum.WorstLevel,
			WorstLevel: toSum.WorstLevel,
		})
	}
	for id, fromSum := range fromByID {
		if _, ok := toByID[id]; ok {
			continue
		}
		removed = append(removed, PublicAnalysisDiffEntry{
			Domain:     nameFor(id),
			FromGrade:  fromSum.Grade,
			FromLevel:  fromSum.WorstLevel,
			WorstLevel: fromSum.WorstLevel,
		})
	}
	for id, toSum := range toByID {
		fromSum, ok := fromByID[id]
		if !ok {
			continue
		}
		if stringPtrEqual(fromSum.Grade, toSum.Grade) && strings.EqualFold(fromSum.WorstLevel, toSum.WorstLevel) {
			continue
		}
		entry := PublicAnalysisDiffEntry{
			Domain:     nameFor(id),
			FromGrade:  fromSum.Grade,
			ToGrade:    toSum.Grade,
			FromLevel:  fromSum.WorstLevel,
			ToLevel:    toSum.WorstLevel,
			WorstLevel: toSum.WorstLevel,
		}
		if !stringPtrEqual(fromSum.Grade, toSum.Grade) {
			gradeChanged = append(gradeChanged, entry)
		}
		if !strings.EqualFold(fromSum.WorstLevel, toSum.WorstLevel) {
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
