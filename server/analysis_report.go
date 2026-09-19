package server

import (
	"encoding/json"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"codeberg.org/pawal/gonemaster/engine/profile"
	"codeberg.org/pawal/gonemaster/scoring"
)

// Classification of one finding change between two snapshots. A change is
// engine-driven when the tag vocabulary moved with it, and a cohort change
// otherwise. Unknown means the vocabulary could not be read, and never
// collapses into a cohort change.
const (
	ReportChangeNewInEngine       = "new_in_engine"
	ReportChangeRemovedFromEngine = "removed_from_engine"
	ReportChangeLevelReclassified = "level_reclassified"
	ReportChangeCohort            = "cohort_change"
	ReportChangeUnknown           = "unknown"
)

// Category of one moving domain, rolled up from its tag classifications.
const (
	ReportCategoryReal        = "real"
	ReportCategoryMeasurement = "measurement"
	ReportCategoryMixed       = "mixed"
	ReportCategoryUnknown     = "unknown"
)

// Tri-state for a provenance fact the report cannot always establish.
const (
	ReportStateTrue    = "true"
	ReportStateFalse   = "false"
	ReportStateUnknown = "unknown"
)

// Dimensions a cluster can be detected over.
const (
	ReportDimensionNameserver = "nameserver"
	ReportDimensionASN        = "asn"
	ReportDimensionPrefix     = "prefix"
)

// Cluster detection defaults. Both are request parameters: how many domains
// make a cluster and how far their score moves may spread.
const (
	defaultReportMinCluster = 3
	defaultReportMaxSpread  = 3
	maxReportMinCluster     = 500
	maxReportMaxSpread      = 100
)

// Movers are paged like every other public list.
const (
	defaultReportDomainLimit = 500
	maxReportDomainLimit     = 500
)

// PublicAnalysisVocabularyEntry is one tag present in only one of the two
// snapshots' profiles.
type PublicAnalysisVocabularyEntry struct {
	Tag    string `json:"tag"`
	Module string `json:"module,omitempty"`
	Level  string `json:"level,omitempty"`
}

// PublicAnalysisVocabularyChange is one tag the two profiles level
// differently.
type PublicAnalysisVocabularyChange struct {
	Tag       string `json:"tag"`
	Module    string `json:"module,omitempty"`
	FromLevel string `json:"from_level"`
	ToLevel   string `json:"to_level"`
}

// PublicAnalysisVocabularyDelta is the tag vocabulary comparison of the two
// snapshots. Unavailable on either side means no change can be attributed.
type PublicAnalysisVocabularyDelta struct {
	FromAvailable bool                             `json:"from_available"`
	ToAvailable   bool                             `json:"to_available"`
	FromTagCount  int                              `json:"from_tag_count"`
	ToTagCount    int                              `json:"to_tag_count"`
	Added         []PublicAnalysisVocabularyEntry  `json:"added"`
	Removed       []PublicAnalysisVocabularyEntry  `json:"removed"`
	LevelChanged  []PublicAnalysisVocabularyChange `json:"level_changed"`
}

// PublicAnalysisReportSide identifies one snapshot in the report header.
type PublicAnalysisReportSide struct {
	Slug               string    `json:"slug"`
	Label              string    `json:"label,omitempty"`
	CapturedAt         time.Time `json:"captured_at"`
	EngineVersion      string    `json:"engine_version,omitempty"`
	MixedEngineVersion bool      `json:"mixed_engine_version,omitempty"`
	ProfileName        string    `json:"profile_name,omitempty"`
	ScoringConfigHash  string    `json:"scoring_config_hash,omitempty"`
	TagViewMinLevel    string    `json:"tag_view_min_level,omitempty"`
	DomainCount        int       `json:"domain_count"`
}

// PublicAnalysisReportHeader carries the provenance a reader needs before
// trusting any row below it.
type PublicAnalysisReportHeader struct {
	From       PublicAnalysisReportSide      `json:"from"`
	To         PublicAnalysisReportSide      `json:"to"`
	Engine     PublicAnalysisEngineDelta     `json:"engine"`
	Vocabulary PublicAnalysisVocabularyDelta `json:"vocabulary"`
	// ScoringConfigChanged is "true", "false" or "unknown".
	ScoringConfigChanged string `json:"scoring_config_changed"`
	// TagFloor is the stricter of the two snapshots' tag view floors.
	// Findings below it are absent from the per-domain lists.
	TagFloor string `json:"tag_floor,omitempty"`
}

// PublicAnalysisReportTotals is the headline count of what moved.
type PublicAnalysisReportTotals struct {
	FromDomainCount  int            `json:"from_domain_count"`
	ToDomainCount    int            `json:"to_domain_count"`
	BothDomainCount  int            `json:"both_domain_count"`
	Added            int            `json:"added"`
	Removed          int            `json:"removed"`
	IdenticalScore   int            `json:"identical_score"`
	Improved         int            `json:"improved"`
	Regressed        int            `json:"regressed"`
	FromMeanScore    *float64       `json:"from_mean_score,omitempty"`
	ToMeanScore      *float64       `json:"to_mean_score,omitempty"`
	FromGrades       map[string]int `json:"from_grades,omitempty"`
	ToGrades         map[string]int `json:"to_grades,omitempty"`
	DomainCategories map[string]int `json:"domain_categories,omitempty"`
}

// PublicAnalysisReportTagEntry is one cohort-wide tag row with the
// classification that says whether the engine or the cohort moved.
type PublicAnalysisReportTagEntry struct {
	PublicAnalysisTagDiffEntry
	Classification string `json:"classification"`
}

// PublicAnalysisReportTags groups the cohort-wide tag rows.
type PublicAnalysisReportTags struct {
	Appeared     []PublicAnalysisReportTagEntry `json:"appeared"`
	Cleared      []PublicAnalysisReportTagEntry `json:"cleared"`
	LevelChanged []PublicAnalysisReportTagEntry `json:"level_changed"`
}

// PublicAnalysisReportTagChange is one tag that moved on one domain.
type PublicAnalysisReportTagChange struct {
	Tag            string `json:"tag"`
	Module         string `json:"module,omitempty"`
	Testcase       string `json:"testcase,omitempty"`
	FromLevel      string `json:"from_level,omitempty"`
	ToLevel        string `json:"to_level,omitempty"`
	Classification string `json:"classification"`
}

// PublicAnalysisReportDomain is one domain whose score or grade moved.
type PublicAnalysisReportDomain struct {
	Domain       string `json:"domain"`
	FromScore    *int   `json:"from_score,omitempty"`
	ToScore      *int   `json:"to_score,omitempty"`
	ScoreDelta   *int   `json:"score_delta,omitempty"`
	FromGrade    string `json:"from_grade,omitempty"`
	ToGrade      string `json:"to_grade,omitempty"`
	GradeChanged bool   `json:"grade_changed"`
	Category     string `json:"category"`
	// ExplainedDelta is the score movement the listed findings account for
	// under the active scoring configuration.
	ExplainedDelta int `json:"explained_delta"`
	// UnexplainedDelta is what is left over; non-zero names a cause the
	// report cannot see.
	UnexplainedDelta int                             `json:"unexplained_delta"`
	Appeared         []PublicAnalysisReportTagChange `json:"appeared"`
	Cleared          []PublicAnalysisReportTagChange `json:"cleared"`
	LevelChanged     []PublicAnalysisReportTagChange `json:"level_changed"`
}

// PublicAnalysisReportClusterDimension is one dimension that produced a
// cluster, with how many of the cohort's domains carry the value.
type PublicAnalysisReportClusterDimension struct {
	Dimension    string `json:"dimension"`
	Value        string `json:"value"`
	Label        string `json:"label,omitempty"`
	TotalDomains int    `json:"total_domains"`
}

// PublicAnalysisReportCluster is a set of domains that moved together and
// share at least one dimension value.
type PublicAnalysisReportCluster struct {
	Dimensions []PublicAnalysisReportClusterDimension `json:"dimensions"`
	Domains    []string                               `json:"domains"`
	Size       int                                    `json:"size"`
	MinDelta   int                                    `json:"min_delta"`
	MaxDelta   int                                    `json:"max_delta"`
	// Direction is "improved" or "regressed"; a cluster never mixes both.
	Direction string `json:"direction"`
}

// PublicAnalysisReportResponse is the /report response: the mechanical part
// of a cohort comparison, with every change classified.
type PublicAnalysisReportResponse struct {
	DatasetTag string                        `json:"dataset_tag"`
	FromSlug   string                        `json:"from_slug"`
	ToSlug     string                        `json:"to_slug"`
	MinCluster int                           `json:"min_cluster"`
	MaxSpread  int                           `json:"max_spread"`
	Header     PublicAnalysisReportHeader    `json:"header"`
	Totals     PublicAnalysisReportTotals    `json:"totals"`
	Tags       PublicAnalysisReportTags      `json:"tags"`
	Domains    []PublicAnalysisReportDomain  `json:"domains"`
	Clusters   []PublicAnalysisReportCluster `json:"clusters"`
	// DomainTotal is every mover, of which Domains carries one page.
	DomainTotal  int `json:"domain_total"`
	DomainLimit  int `json:"domain_limit,omitempty"`
	DomainOffset int `json:"domain_offset,omitempty"`
}

// pageDomains returns resp carrying one page of movers. Totals, tags and
// clusters stay whole; they are computed over every mover.
func pageDomains(resp PublicAnalysisReportResponse, limit, offset int) PublicAnalysisReportResponse {
	resp.DomainLimit, resp.DomainOffset = limit, offset
	if offset >= len(resp.Domains) {
		resp.Domains = []PublicAnalysisReportDomain{}
		return resp
	}
	end := min(offset+limit, len(resp.Domains))
	resp.Domains = resp.Domains[offset:end]
	return resp
}

// reportInput is everything the report needs, so the computation stays a
// pure function over rows the handler already read.
type reportInput struct {
	DatasetTag  string
	From        AnalysisCohortSnapshot
	To          AnalysisCohortSnapshot
	FromDomains []AnalysisSnapshotDomainView
	ToDomains   []AnalysisSnapshotDomainView
	FromTags    []AnalysisSnapshotTagView
	ToTags      []AnalysisSnapshotTagView
	Scoring     scoring.Config
	MinCluster  int
	MaxSpread   int
}

// reportVocabulary is one snapshot's tag vocabulary flattened to
// tag -> level, plus the module each tag belongs to.
type reportVocabulary struct {
	available bool
	levels    map[string]string
	modules   map[string]string
}

// parseReportVocabulary reads a snapshot's stored test_levels table.
func parseReportVocabulary(raw string) reportVocabulary {
	out := reportVocabulary{levels: map[string]string{}, modules: map[string]string{}}
	if strings.TrimSpace(raw) == "" {
		return out
	}
	var table map[string]map[string]string
	if err := json.Unmarshal([]byte(raw), &table); err != nil || len(table) == 0 {
		return out
	}
	for module, tags := range table {
		for tag, level := range tags {
			key := strings.ToUpper(tag)
			out.levels[key] = level
			out.modules[key] = module
		}
	}
	out.available = len(out.levels) > 0
	return out
}

// asProfile wraps the vocabulary so the comparison runs through the engine's
// own test_levels diff.
func (v reportVocabulary) asProfile() *profile.Profile {
	table := map[string]map[string]string{}
	for tag, level := range v.levels {
		module := v.modules[tag]
		if table[module] == nil {
			table[module] = map[string]string{}
		}
		table[module][tag] = level
	}
	return &profile.Profile{TestLevels: table}
}

// reportVocabularyDelta is the engine-side comparison driving every
// classification below.
type reportVocabularyDelta struct {
	available  bool
	added      map[string]string
	removed    map[string]string
	relevelled map[string][2]string
	payload    PublicAnalysisVocabularyDelta
}

// diffReportVocabularies compares the two snapshots' vocabularies. When
// either side is unavailable the delta carries no tags and classifies
// everything as unknown.
func diffReportVocabularies(from, to reportVocabulary) reportVocabularyDelta {
	out := reportVocabularyDelta{
		available:  from.available && to.available,
		added:      map[string]string{},
		removed:    map[string]string{},
		relevelled: map[string][2]string{},
		payload: PublicAnalysisVocabularyDelta{
			FromAvailable: from.available,
			ToAvailable:   to.available,
			FromTagCount:  len(from.levels),
			ToTagCount:    len(to.levels),
			Added:         []PublicAnalysisVocabularyEntry{},
			Removed:       []PublicAnalysisVocabularyEntry{},
			LevelChanged:  []PublicAnalysisVocabularyChange{},
		},
	}
	if !out.available {
		return out
	}
	diff := profile.DiffTestLevels(from.asProfile(), to.asProfile())
	for _, e := range diff.Added {
		out.added[strings.ToUpper(e.Tag)] = e.Level
		out.payload.Added = append(out.payload.Added,
			PublicAnalysisVocabularyEntry{Tag: e.Tag, Module: e.Module, Level: e.Level})
	}
	for _, e := range diff.Removed {
		out.removed[strings.ToUpper(e.Tag)] = e.Level
		out.payload.Removed = append(out.payload.Removed,
			PublicAnalysisVocabularyEntry{Tag: e.Tag, Module: e.Module, Level: e.Level})
	}
	for _, e := range diff.LevelChanged {
		out.relevelled[strings.ToUpper(e.Tag)] = [2]string{e.From, e.To}
		out.payload.LevelChanged = append(out.payload.LevelChanged,
			PublicAnalysisVocabularyChange{Tag: e.Tag, Module: e.Module, FromLevel: e.From, ToLevel: e.To})
	}
	return out
}

// classifyAppeared classifies a tag seen only on the to side.
func (d reportVocabularyDelta) classifyAppeared(tag string) string {
	if !d.available {
		return ReportChangeUnknown
	}
	if _, ok := d.added[strings.ToUpper(tag)]; ok {
		return ReportChangeNewInEngine
	}
	return ReportChangeCohort
}

// classifyCleared classifies a tag seen only on the from side.
func (d reportVocabularyDelta) classifyCleared(tag string) string {
	if !d.available {
		return ReportChangeUnknown
	}
	if _, ok := d.removed[strings.ToUpper(tag)]; ok {
		return ReportChangeRemovedFromEngine
	}
	return ReportChangeCohort
}

// classifyLevelMove classifies a tag whose level moved. It is engine-driven
// only when the move matches the vocabulary's own re-level for that tag.
func (d reportVocabularyDelta) classifyLevelMove(tag, from, to string) string {
	if !d.available {
		return ReportChangeUnknown
	}
	if move, ok := d.relevelled[strings.ToUpper(tag)]; ok &&
		strings.EqualFold(move[0], from) && strings.EqualFold(move[1], to) {
		return ReportChangeLevelReclassified
	}
	return ReportChangeCohort
}

// engineDriven reports whether a classification says the engine moved.
func engineDriven(classification string) bool {
	switch classification {
	case ReportChangeNewInEngine, ReportChangeRemovedFromEngine, ReportChangeLevelReclassified:
		return true
	}
	return false
}

// buildAnalysisReport computes the whole report from two snapshots' view
// rows and their vocabularies.
func buildAnalysisReport(in reportInput) PublicAnalysisReportResponse {
	fromVocab := parseReportVocabulary(in.From.Vocabulary)
	toVocab := parseReportVocabulary(in.To.Vocabulary)
	delta := diffReportVocabularies(fromVocab, toVocab)
	floor := alignFloorRank(in.From.TagViewMinLevel, in.To.TagViewMinLevel)

	resp := PublicAnalysisReportResponse{
		DatasetTag: in.DatasetTag,
		FromSlug:   in.From.Slug,
		ToSlug:     in.To.Slug,
		MinCluster: in.MinCluster,
		MaxSpread:  in.MaxSpread,
		Header: PublicAnalysisReportHeader{
			From:                 reportSide(in.From, len(in.FromDomains)),
			To:                   reportSide(in.To, len(in.ToDomains)),
			Engine:               engineDelta(in.From, in.To),
			Vocabulary:           delta.payload,
			ScoringConfigChanged: scoringConfigChanged(in.From, in.To),
			TagFloor:             stricterFloor(in.From.TagViewMinLevel, in.To.TagViewMinLevel),
		},
		Tags:     buildReportTags(atFloorTagViews(in.FromTags, floor), atFloorTagViews(in.ToTags, floor), delta),
		Domains:  []PublicAnalysisReportDomain{},
		Clusters: []PublicAnalysisReportCluster{},
	}

	fromByName := indexDomainViewsByName(atFloorDomainViews(in.FromDomains, floor))
	toByName := indexDomainViewsByName(atFloorDomainViews(in.ToDomains, floor))
	resp.Domains = buildReportDomains(fromByName, toByName, delta, in.Scoring)
	resp.Totals = buildReportTotals(fromByName, toByName, resp.Domains)
	resp.Clusters = buildReportClusters(toByName, resp.Domains, in.MinCluster, in.MaxSpread)
	resp.DomainTotal = len(resp.Domains)
	return resp
}

func reportSide(snap AnalysisCohortSnapshot, domainCount int) PublicAnalysisReportSide {
	return PublicAnalysisReportSide{
		Slug:               snap.Slug,
		Label:              snap.Label,
		CapturedAt:         snap.CapturedAt,
		EngineVersion:      snap.EngineVersion,
		MixedEngineVersion: snap.MixedEngineVersion,
		ProfileName:        snap.ProfileName,
		ScoringConfigHash:  snap.ScoringConfigHash,
		TagViewMinLevel:    snap.TagViewMinLevel,
		DomainCount:        domainCount,
	}
}

// scoringConfigChanged compares the two captured scoring identities. An
// unstamped side leaves the answer unknown rather than assuming no change.
func scoringConfigChanged(from, to AnalysisCohortSnapshot) string {
	if from.ScoringConfigHash == "" || to.ScoringConfigHash == "" {
		return ReportStateUnknown
	}
	if from.ScoringConfigHash == to.ScoringConfigHash {
		return ReportStateFalse
	}
	return ReportStateTrue
}

// alignFloorRank is the severity both sides must clear when the two views
// were floored differently. Zero leaves the lists as captured.
func alignFloorRank(from, to string) int {
	fromRank, toRank := severityRank(from), severityRank(to)
	if fromRank == toRank {
		return 0
	}
	return max(fromRank, toRank)
}

// atFloorTagViews drops the cohort-wide rows below rank.
func atFloorTagViews(views []AnalysisSnapshotTagView, rank int) []AnalysisSnapshotTagView {
	if rank == 0 {
		return views
	}
	out := make([]AnalysisSnapshotTagView, 0, len(views))
	for _, v := range views {
		if severityRank(v.Level) >= rank {
			out = append(out, v)
		}
	}
	return out
}

// atFloorDomainViews drops each domain's findings below rank.
func atFloorDomainViews(views []AnalysisSnapshotDomainView, rank int) []AnalysisSnapshotDomainView {
	if rank == 0 {
		return views
	}
	out := make([]AnalysisSnapshotDomainView, 0, len(views))
	for _, view := range views {
		tags := make([]DomainViewTag, 0, len(view.Tags))
		for _, t := range view.Tags {
			if severityRank(t.Level) >= rank {
				tags = append(tags, t)
			}
		}
		view.Tags = tags
		out = append(out, view)
	}
	return out
}

// stricterFloor returns the higher of two tag view floors, which is the one
// that governs what the per-domain lists can show.
func stricterFloor(from, to string) string {
	if severityRank(to) > severityRank(from) {
		return to
	}
	return from
}

// buildReportTags classifies every cohort-wide tag row.
func buildReportTags(fromViews, toViews []AnalysisSnapshotTagView, delta reportVocabularyDelta) PublicAnalysisReportTags {
	appeared, cleared, levelChanged := diffTagViews(fromViews, toViews)
	out := PublicAnalysisReportTags{
		Appeared:     make([]PublicAnalysisReportTagEntry, 0, len(appeared)),
		Cleared:      make([]PublicAnalysisReportTagEntry, 0, len(cleared)),
		LevelChanged: make([]PublicAnalysisReportTagEntry, 0, len(levelChanged)),
	}
	for _, e := range appeared {
		out.Appeared = append(out.Appeared, PublicAnalysisReportTagEntry{
			PublicAnalysisTagDiffEntry: e,
			Classification:             delta.classifyAppeared(e.Tag),
		})
	}
	for _, e := range cleared {
		out.Cleared = append(out.Cleared, PublicAnalysisReportTagEntry{
			PublicAnalysisTagDiffEntry: e,
			Classification:             delta.classifyCleared(e.Tag),
		})
	}
	for _, e := range levelChanged {
		out.LevelChanged = append(out.LevelChanged, PublicAnalysisReportTagEntry{
			PublicAnalysisTagDiffEntry: e,
			Classification:             delta.classifyLevelMove(e.Tag, e.FromLevel, e.ToLevel),
		})
	}
	return out
}

// buildReportDomains returns one row per domain present on both sides whose
// score or grade moved, sorted by the size of the fall.
func buildReportDomains(
	fromByName, toByName map[string]AnalysisSnapshotDomainView,
	delta reportVocabularyDelta,
	cfg scoring.Config,
) []PublicAnalysisReportDomain {
	out := []PublicAnalysisReportDomain{}
	for name, to := range toByName {
		from, ok := fromByName[name]
		if !ok {
			continue
		}
		scoreMoved := !sameScore(from.Score, to.Score)
		gradeMoved := from.Grade != to.Grade
		if !scoreMoved && !gradeMoved {
			continue
		}
		row := PublicAnalysisReportDomain{
			Domain:       name,
			FromScore:    copyIntPtr(from.Score),
			ToScore:      copyIntPtr(to.Score),
			FromGrade:    from.Grade,
			ToGrade:      to.Grade,
			GradeChanged: gradeMoved,
		}
		if from.Score != nil && to.Score != nil {
			d := *to.Score - *from.Score
			row.ScoreDelta = &d
		}
		appeared, cleared, changed := diffDomainTags(from.Tags, to.Tags)
		engineCount, cohortCount := 0, 0
		row.Appeared = classifyDomainTags(appeared, func(m reportTagMove) string {
			return delta.classifyAppeared(m.Tag)
		}, &engineCount, &cohortCount)
		row.Cleared = classifyDomainTags(cleared, func(m reportTagMove) string {
			return delta.classifyCleared(m.Tag)
		}, &engineCount, &cohortCount)
		row.LevelChanged = classifyDomainTags(changed, func(m reportTagMove) string {
			return delta.classifyLevelMove(m.Tag, m.FromLevel, m.ToLevel)
		}, &engineCount, &cohortCount)
		row.Category = domainCategory(delta.available, engineCount, cohortCount)
		row.ExplainedDelta = explainScoreDelta(cfg, appeared, cleared, changed)
		if row.ScoreDelta != nil {
			row.UnexplainedDelta = *row.ScoreDelta - row.ExplainedDelta
		}
		out = append(out, row)
	}
	sort.Slice(out, func(i, j int) bool {
		di, dj := scoreDeltaOr(out[i], 0), scoreDeltaOr(out[j], 0)
		if di != dj {
			return di < dj
		}
		return out[i].Domain < out[j].Domain
	})
	return out
}

func scoreDeltaOr(row PublicAnalysisReportDomain, fallback int) int {
	if row.ScoreDelta == nil {
		return fallback
	}
	return *row.ScoreDelta
}

// domainCategory rolls the domain's tag classifications up. A domain whose
// score moved with no visible tag change stays unknown: the cause is below
// the tag floor or outside the findings.
func domainCategory(vocabularyAvailable bool, engineCount, cohortCount int) string {
	if !vocabularyAvailable {
		return ReportCategoryUnknown
	}
	switch {
	case cohortCount > 0 && engineCount > 0:
		return ReportCategoryMixed
	case cohortCount > 0:
		return ReportCategoryReal
	case engineCount > 0:
		return ReportCategoryMeasurement
	}
	return ReportCategoryUnknown
}

// reportTagMove is one tag that moved on one domain. An appeared tag has no
// from level, a cleared tag no to level.
type reportTagMove struct {
	Tag       string
	Module    string
	Testcase  string
	FromLevel string
	ToLevel   string
}

// diffDomainTags splits one domain's two tag lists into appeared, cleared
// and level-changed moves.
func diffDomainTags(from, to []DomainViewTag) (appeared, cleared, changed []reportTagMove) {
	fromByTag := indexDomainViewTags(from)
	toByTag := indexDomainViewTags(to)
	for tag, t := range toByTag {
		f, ok := fromByTag[tag]
		if !ok {
			appeared = append(appeared, reportTagMove{
				Tag: t.Tag, Module: t.Module, Testcase: t.Testcase, ToLevel: t.Level,
			})
			continue
		}
		if !strings.EqualFold(f.Level, t.Level) {
			changed = append(changed, reportTagMove{
				Tag: t.Tag, Module: firstNonEmptyStr(t.Module, f.Module),
				Testcase:  firstNonEmptyStr(t.Testcase, f.Testcase),
				FromLevel: f.Level, ToLevel: t.Level,
			})
		}
	}
	for tag, f := range fromByTag {
		if _, ok := toByTag[tag]; ok {
			continue
		}
		cleared = append(cleared, reportTagMove{
			Tag: f.Tag, Module: f.Module, Testcase: f.Testcase, FromLevel: f.Level,
		})
	}
	sortTagMoves(appeared)
	sortTagMoves(cleared)
	sortTagMoves(changed)
	return appeared, cleared, changed
}

func indexDomainViewTags(rows []DomainViewTag) map[string]DomainViewTag {
	out := make(map[string]DomainViewTag, len(rows))
	for _, row := range rows {
		if row.Tag == "" {
			continue
		}
		out[strings.ToUpper(row.Tag)] = row
	}
	return out
}

func sortTagMoves(moves []reportTagMove) {
	sort.Slice(moves, func(i, j int) bool { return moves[i].Tag < moves[j].Tag })
}

// classifyDomainTags turns moves into response rows, counting how many of
// them the engine drove and how many the cohort did.
func classifyDomainTags(
	moves []reportTagMove,
	classify func(reportTagMove) string,
	engineCount, cohortCount *int,
) []PublicAnalysisReportTagChange {
	out := make([]PublicAnalysisReportTagChange, 0, len(moves))
	for _, m := range moves {
		class := classify(m)
		if engineDriven(class) {
			*engineCount++
		} else if class == ReportChangeCohort {
			*cohortCount++
		}
		out = append(out, PublicAnalysisReportTagChange{
			Tag: m.Tag, Module: m.Module, Testcase: m.Testcase,
			FromLevel: m.FromLevel, ToLevel: m.ToLevel, Classification: class,
		})
	}
	return out
}

// tagPenalty resolves what one tag at one level costs: the tag override when
// the configuration carries one, otherwise the severity penalty.
func tagPenalty(cfg scoring.Config, tag, level string) int {
	if p, ok := cfg.TagPenalties[strings.ToUpper(tag)]; ok {
		return p
	}
	return cfg.SeverityPenalties[strings.ToUpper(level)]
}

// explainScoreDelta converts the moved findings into the score movement they
// account for, weighting each module's penalty the way scoring.Compute does.
// Sub-score clamping, the CRITICAL override, the A+ bonus and repeated
// entries are not modelled, so a gap is expected on a broken domain.
func explainScoreDelta(cfg scoring.Config, appeared, cleared, changed []reportTagMove) int {
	var totalWeight float64
	for _, w := range cfg.CategoryWeights {
		totalWeight += w
	}
	if totalWeight == 0 {
		return 0
	}
	byCategory := map[string]int{}
	for _, moves := range [][]reportTagMove{appeared, cleared, changed} {
		for _, m := range moves {
			category, ok := cfg.ModuleCategories[strings.ToUpper(m.Module)]
			if !ok {
				continue
			}
			if m.ToLevel != "" {
				byCategory[category] += tagPenalty(cfg, m.Tag, m.ToLevel)
			}
			if m.FromLevel != "" {
				byCategory[category] -= tagPenalty(cfg, m.Tag, m.FromLevel)
			}
		}
	}
	var weighted float64
	for category, penalty := range byCategory {
		weighted += float64(penalty) * cfg.CategoryWeights[category]
	}
	return -int(math.Round(weighted / totalWeight))
}

// buildReportTotals counts membership, score movement and grade spread on
// both sides.
func buildReportTotals(
	fromByName, toByName map[string]AnalysisSnapshotDomainView,
	domains []PublicAnalysisReportDomain,
) PublicAnalysisReportTotals {
	totals := PublicAnalysisReportTotals{
		FromDomainCount:  len(fromByName),
		ToDomainCount:    len(toByName),
		FromGrades:       gradeCounts(fromByName),
		ToGrades:         gradeCounts(toByName),
		FromMeanScore:    meanScore(fromByName),
		ToMeanScore:      meanScore(toByName),
		DomainCategories: map[string]int{},
	}
	for name := range toByName {
		if _, ok := fromByName[name]; ok {
			totals.BothDomainCount++
		} else {
			totals.Added++
		}
	}
	for name := range fromByName {
		if _, ok := toByName[name]; !ok {
			totals.Removed++
		}
	}
	moved := map[string]struct{}{}
	for _, row := range domains {
		totals.DomainCategories[row.Category]++
		if row.ScoreDelta == nil {
			continue
		}
		moved[row.Domain] = struct{}{}
		switch {
		case *row.ScoreDelta > 0:
			totals.Improved++
		case *row.ScoreDelta < 0:
			totals.Regressed++
		}
	}
	for name, to := range toByName {
		from, ok := fromByName[name]
		if !ok {
			continue
		}
		if _, changed := moved[name]; changed {
			continue
		}
		if sameScore(from.Score, to.Score) {
			totals.IdenticalScore++
		}
	}
	return totals
}

func gradeCounts(byName map[string]AnalysisSnapshotDomainView) map[string]int {
	out := map[string]int{}
	for _, v := range byName {
		if v.Grade == "" {
			continue
		}
		out[v.Grade]++
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func meanScore(byName map[string]AnalysisSnapshotDomainView) *float64 {
	sum, count := 0, 0
	for _, v := range byName {
		if v.Score == nil {
			continue
		}
		sum += *v.Score
		count++
	}
	if count == 0 {
		return nil
	}
	mean := math.Round(float64(sum)/float64(count)*100) / 100
	return &mean
}

func sameScore(a, b *int) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func copyIntPtr(v *int) *int {
	if v == nil {
		return nil
	}
	out := *v
	return &out
}

// reportDimValue is one (dimension, value) pair a domain carries.
type reportDimValue struct {
	Dimension string
	Value     string
	Label     string
}

// reportDimensionValues returns the dimensions one domain can cluster under,
// deduplicated.
func reportDimensionValues(v AnalysisSnapshotDomainView) []reportDimValue {
	seen := map[reportDimValue]struct{}{}
	out := []reportDimValue{}
	add := func(dim, value, label string) {
		if strings.TrimSpace(value) == "" {
			return
		}
		item := reportDimValue{Dimension: dim, Value: value, Label: label}
		if _, dup := seen[item]; dup {
			return
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	for _, ns := range v.Nameservers {
		add(ReportDimensionNameserver, ns.Name, "")
	}
	for _, addr := range v.Addresses {
		if addr.ASN != nil {
			add(ReportDimensionASN, strconv.FormatInt(*addr.ASN, 10), addr.ASNLabel)
		}
		add(ReportDimensionPrefix, addr.Prefix, "")
	}
	add(FactCategoryDNSSECPosture, v.DNSSECPosture, "")
	if v.DNSKEYAlgoWeakest != nil {
		label, _ := signingAlgoDisplay(v.DNSKEYAlgoWeakest)
		add(FactCategoryDNSKEYAlgoWeakest, strconv.Itoa(*v.DNSKEYAlgoWeakest), label)
	}
	add(FactCategorySoftwareVersion, v.SoftwareVersion, "")
	return out
}

// buildReportClusters groups the movers by every dimension value they share
// and keeps the groups that moved together. Groups with identical member
// sets collapse into one cluster naming every dimension that produced it.
func buildReportClusters(
	toByName map[string]AnalysisSnapshotDomainView,
	domains []PublicAnalysisReportDomain,
	minCluster, maxSpread int,
) []PublicAnalysisReportCluster {
	type dimKey struct{ dimension, value string }

	totals := map[dimKey]int{}
	labels := map[dimKey]string{}
	for _, view := range toByName {
		for _, dv := range reportDimensionValues(view) {
			key := dimKey{dv.Dimension, dv.Value}
			totals[key]++
			if dv.Label != "" {
				labels[key] = dv.Label
			}
		}
	}

	deltas := map[string]int{}
	membersByKey := map[dimKey][]string{}
	for _, row := range domains {
		if row.ScoreDelta == nil || *row.ScoreDelta == 0 {
			continue
		}
		view, ok := toByName[row.Domain]
		if !ok {
			continue
		}
		deltas[row.Domain] = *row.ScoreDelta
		for _, dv := range reportDimensionValues(view) {
			key := dimKey{dv.Dimension, dv.Value}
			membersByKey[key] = append(membersByKey[key], row.Domain)
		}
	}

	// Collapse on the member set so one operator's domains are reported
	// once, listing every dimension that produced the group.
	byMembers := map[string]*PublicAnalysisReportCluster{}
	for key, members := range membersByKey {
		if len(members) < minCluster {
			continue
		}
		sort.Strings(members)
		minDelta, maxDelta := deltas[members[0]], deltas[members[0]]
		sameDirection := true
		for _, domain := range members {
			d := deltas[domain]
			if (d > 0) != (minDelta > 0) {
				sameDirection = false
				break
			}
			minDelta = min(minDelta, d)
			maxDelta = max(maxDelta, d)
		}
		if !sameDirection || maxDelta-minDelta > maxSpread {
			continue
		}
		dimension := PublicAnalysisReportClusterDimension{
			Dimension:    key.dimension,
			Value:        key.value,
			Label:        labels[key],
			TotalDomains: totals[key],
		}
		signature := strings.Join(members, "\n")
		if existing, ok := byMembers[signature]; ok {
			existing.Dimensions = append(existing.Dimensions, dimension)
			continue
		}
		direction := "regressed"
		if minDelta > 0 {
			direction = "improved"
		}
		byMembers[signature] = &PublicAnalysisReportCluster{
			Dimensions: []PublicAnalysisReportClusterDimension{dimension},
			Domains:    members,
			Size:       len(members),
			MinDelta:   minDelta,
			MaxDelta:   maxDelta,
			Direction:  direction,
		}
	}

	out := make([]PublicAnalysisReportCluster, 0, len(byMembers))
	for _, cluster := range byMembers {
		sort.Slice(cluster.Dimensions, func(i, j int) bool {
			if cluster.Dimensions[i].Dimension != cluster.Dimensions[j].Dimension {
				return cluster.Dimensions[i].Dimension < cluster.Dimensions[j].Dimension
			}
			return cluster.Dimensions[i].Value < cluster.Dimensions[j].Value
		})
		out = append(out, *cluster)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Size != out[j].Size {
			return out[i].Size > out[j].Size
		}
		si := out[i].MaxDelta - out[i].MinDelta
		sj := out[j].MaxDelta - out[j].MinDelta
		if si != sj {
			return si < sj
		}
		return out[i].Domains[0] < out[j].Domains[0]
	})
	return out
}
