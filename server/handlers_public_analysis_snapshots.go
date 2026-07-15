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
	Slug            string    `json:"slug"`
	Label           string    `json:"label,omitempty"`
	Description     string    `json:"description,omitempty"`
	CapturedAt      time.Time `json:"captured_at"`
	FirstRunAt      time.Time `json:"first_run_at"`
	LastRunAt       time.Time `json:"last_run_at"`
	RunCount        int       `json:"run_count"`
	DomainCount     int       `json:"domain_count"`
	ProfileName     string    `json:"profile_name,omitempty"`
	IsDefault       bool      `json:"is_default,omitempty"`
	TagViewMinLevel string    `json:"tag_view_min_level,omitempty"`
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
	FirstRunAt  time.Time                  `json:"first_run_at"`
	LastRunAt   time.Time                  `json:"last_run_at"`
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
	FirstRunAt time.Time       `json:"first_run_at"`
	LastRunAt  time.Time       `json:"last_run_at"`
	Payload    json.RawMessage `json:"payload"`
}

// PublicAnalysisFactKeyMeta is per-key display metadata sent alongside
// trend points so the UI mirrors factCategoryDisplays without a copy.
type PublicAnalysisFactKeyMeta struct {
	Label string `json:"label"`
	Tone  string `json:"tone"`
	Order int    `json:"order"`
}

// PublicAnalysisTrendResponse is the /trends response. One time series per
// request; callers asking for multiple categories make multiple requests.
type PublicAnalysisTrendResponse struct {
	DatasetTag string                               `json:"dataset_tag"`
	Category   string                               `json:"category"`
	Points     []PublicAnalysisTrendPoint           `json:"points"`
	KeyMeta    map[string]PublicAnalysisFactKeyMeta `json:"key_meta,omitempty"`
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

// PublicAnalysisTagDiffEntry is one finding-tag row in a granularity=tags
// diff. from_* fields are zero for appeared tags; to_* for cleared tags.
type PublicAnalysisTagDiffEntry struct {
	Tag             string `json:"tag"`
	Module          string `json:"module,omitempty"`
	Testcase        string `json:"testcase,omitempty"`
	FromLevel       string `json:"from_level,omitempty"`
	ToLevel         string `json:"to_level,omitempty"`
	FromDomainCount int    `json:"from_domain_count"`
	ToDomainCount   int    `json:"to_domain_count"`
	DomainDelta     int    `json:"domain_delta"`
}

// PublicAnalysisTagDiffResponse is the granularity=tags diff shape: which
// finding tags appeared, cleared, or changed worst severity cohort-wide.
type PublicAnalysisTagDiffResponse struct {
	DatasetTag   string                       `json:"dataset_tag"`
	FromSlug     string                       `json:"from_slug"`
	ToSlug       string                       `json:"to_slug"`
	Granularity  string                       `json:"granularity"`
	Appeared     []PublicAnalysisTagDiffEntry `json:"appeared"`
	Cleared      []PublicAnalysisTagDiffEntry `json:"cleared"`
	LevelChanged []PublicAnalysisTagDiffEntry `json:"level_changed"`
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
			Slug:            snap.Slug,
			Label:           snap.Label,
			Description:     snap.Description,
			CapturedAt:      snap.CapturedAt,
			FirstRunAt:      snap.FirstRunAt,
			LastRunAt:       snap.LastRunAt,
			RunCount:        snap.RunCount,
			DomainCount:     snap.DomainCount,
			ProfileName:     snap.ProfileName,
			IsDefault:       snap.ID == defaultID,
			TagViewMinLevel: snap.TagViewMinLevel,
		})
	}
	w.Header().Set("Cache-Control", "public, max-age=60")
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
		category = FactCategorySeverity
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
		KeyMeta:    buildTrendKeyMeta(category, points),
	})
}

// buildTrendKeyMeta resolves label/tone/order from factCategoryDisplays
// for every key seen across the points' {key: count} payloads.
func buildTrendKeyMeta(category string, points []PublicAnalysisTrendPoint) map[string]PublicAnalysisFactKeyMeta {
	display, known := factCategoryDisplays[category]
	if !known {
		return nil
	}
	seen := map[string]struct{}{}
	for _, p := range points {
		if len(p.Payload) == 0 {
			continue
		}
		var counts map[string]json.RawMessage
		if err := json.Unmarshal(p.Payload, &counts); err != nil {
			continue
		}
		for key := range counts {
			seen[key] = struct{}{}
		}
	}
	if len(seen) == 0 {
		return nil
	}
	out := make(map[string]PublicAnalysisFactKeyMeta, len(seen))
	for key := range seen {
		meta := PublicAnalysisFactKeyMeta{Label: key, Tone: "neutral", Order: 1 << 30}
		if display.KeyLabel != nil {
			meta.Label = display.KeyLabel(key)
		}
		if display.KeyTone != nil {
			meta.Tone = display.KeyTone(key)
		}
		if display.KeyOrder != nil {
			meta.Order = display.KeyOrder(key)
		}
		out[key] = meta
	}
	return out
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

	// Optional tag-level granularity: which finding tags moved cohort-wide.
	switch strings.ToLower(strings.TrimSpace(r.URL.Query().Get("granularity"))) {
	case "", "domains":
		// Domain-level diff below.
	case "tags":
		s.writeTagDiff(w, cohort, fromSnap, toSnap, readStore)
		return
	default:
		writeError(w, http.StatusBadRequest, "invalid_granularity",
			"granularity must be omitted, 'domains', or 'tags'", nil)
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

// writeTagDiff computes and writes the granularity=tags diff between two
// snapshots from their materialized tag views.
func (s *Server) writeTagDiff(w http.ResponseWriter, cohort AnalysisCohort, fromSnap, toSnap AnalysisCohortSnapshot, readStore AnalysisReadStore) {
	appeared, cleared, levelChanged := diffTagViews(
		readStore.ListSnapshotTagViews(fromSnap.ID),
		readStore.ListSnapshotTagViews(toSnap.ID),
	)
	writeJSON(w, http.StatusOK, PublicAnalysisTagDiffResponse{
		DatasetTag:   cohort.SourceTag,
		FromSlug:     fromSnap.Slug,
		ToSlug:       toSnap.Slug,
		Granularity:  "tags",
		Appeared:     appeared,
		Cleared:      cleared,
		LevelChanged: levelChanged,
	})
}

// diffTagViews classifies finding tags between two snapshots' tag views:
// appeared (only in to), cleared (only in from), and level-changed (present
// in both with a different worst severity). Each slice is sorted
// most-impactful-first for stable, useful output.
func diffTagViews(fromViews, toViews []AnalysisSnapshotTagView) (appeared, cleared, levelChanged []PublicAnalysisTagDiffEntry) {
	fromByTag := indexTagViewsByTag(fromViews)
	toByTag := indexTagViewsByTag(toViews)

	appeared = []PublicAnalysisTagDiffEntry{}
	cleared = []PublicAnalysisTagDiffEntry{}
	levelChanged = []PublicAnalysisTagDiffEntry{}

	for tag, to := range toByTag {
		if _, ok := fromByTag[tag]; ok {
			continue
		}
		appeared = append(appeared, PublicAnalysisTagDiffEntry{
			Tag: tag, Module: to.Module, Testcase: to.Testcase,
			ToLevel: to.Level, ToDomainCount: to.DomainCount,
			DomainDelta: to.DomainCount,
		})
	}
	for tag, from := range fromByTag {
		if _, ok := toByTag[tag]; ok {
			continue
		}
		cleared = append(cleared, PublicAnalysisTagDiffEntry{
			Tag: tag, Module: from.Module, Testcase: from.Testcase,
			FromLevel: from.Level, FromDomainCount: from.DomainCount,
			DomainDelta: -from.DomainCount,
		})
	}
	for tag, to := range toByTag {
		from, ok := fromByTag[tag]
		if !ok || strings.EqualFold(from.Level, to.Level) {
			continue
		}
		levelChanged = append(levelChanged, PublicAnalysisTagDiffEntry{
			Tag:             tag,
			Module:          firstNonEmptyStr(to.Module, from.Module),
			Testcase:        firstNonEmptyStr(to.Testcase, from.Testcase),
			FromLevel:       from.Level,
			ToLevel:         to.Level,
			FromDomainCount: from.DomainCount,
			ToDomainCount:   to.DomainCount,
			DomainDelta:     to.DomainCount - from.DomainCount,
		})
	}

	sortTagDiffEntries(appeared)
	sortTagDiffEntries(cleared)
	sortTagDiffEntries(levelChanged)
	return appeared, cleared, levelChanged
}

func indexTagViewsByTag(rows []AnalysisSnapshotTagView) map[string]AnalysisSnapshotTagView {
	out := make(map[string]AnalysisSnapshotTagView, len(rows))
	for _, row := range rows {
		if row.Tag == "" {
			continue
		}
		out[row.Tag] = row
	}
	return out
}

// sortTagDiffEntries orders by the larger of the two domain counts (impact)
// descending, then tag ascending, so output is deterministic.
func sortTagDiffEntries(items []PublicAnalysisTagDiffEntry) {
	sort.Slice(items, func(i, j int) bool {
		li := max(items[i].FromDomainCount, items[i].ToDomainCount)
		lj := max(items[j].FromDomainCount, items[j].ToDomainCount)
		if li != lj {
			return li > lj
		}
		return items[i].Tag < items[j].Tag
	})
}

func firstNonEmptyStr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
