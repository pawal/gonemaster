package apitest

import "time"

// The views below are the wire shapes gonemaster-server sends. They are
// deliberately separate from each binary's decode types, so a test encodes
// with one set of json tags and the code under test decodes with its own.

// Job is a job from POST /jobs and GET /jobs/{id}.
type Job struct {
	ID         string    `json:"id"`
	BatchID    string    `json:"batch_id,omitempty"`
	Domain     string    `json:"domain"`
	Status     string    `json:"status"`
	Progress   int       `json:"progress"`
	Error      string    `json:"error,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	StartedAt  time.Time `json:"started_at"`
	FinishedAt time.Time `json:"finished_at"`
}

// Run is a run from GET /runs and GET /runs/{id}.
type Run struct {
	ID         string    `json:"id"`
	Domain     string    `json:"domain"`
	BatchID    string    `json:"batch_id,omitempty"`
	PublicID   string    `json:"public_id,omitempty"`
	Status     string    `json:"status"`
	DurationMs int64     `json:"duration_ms"`
	WorstLevel string    `json:"worst_level,omitempty"`
	Score      *int      `json:"score"`
	Grade      *string   `json:"grade"`
	FinishedAt time.Time `json:"finished_at"`
	Error      string    `json:"error,omitempty"`
}

// RunList is GET /runs.
type RunList struct {
	Items []Run `json:"items"`
	Total int   `json:"total"`
}

// Result is GET /jobs/{id}/result and the identical /runs/{id}/result.
type Result struct {
	JobID             string         `json:"job_id"`
	BatchID           string         `json:"batch_id,omitempty"`
	Status            string         `json:"status"`
	Summary           map[string]any `json:"summary,omitempty"`
	Raw               *ResultRaw     `json:"raw,omitempty"`
	Score             *Score         `json:"score,omitempty"`
	NameserverTimings []NSTiming     `json:"nameserver_timings,omitempty"`
}

// ResultRaw is the raw entry list inside a result.
type ResultRaw struct {
	Locale  string  `json:"locale,omitempty"`
	Entries []Entry `json:"entries"`
}

// Entry is one log entry of a raw result.
type Entry struct {
	Timestamp float64        `json:"timestamp,omitempty"`
	Module    string         `json:"module"`
	Testcase  string         `json:"testcase"`
	Tag       string         `json:"tag"`
	Level     string         `json:"level"`
	Args      map[string]any `json:"args,omitempty"`
	Message   string         `json:"message,omitempty"`
}

// Score is the scoring block of a result.
type Score struct {
	Score int    `json:"score"`
	Grade string `json:"grade"`
}

// NSTiming is one row of nameserver_timings.
type NSTiming struct {
	Nameserver   string  `json:"nameserver"`
	Address      string  `json:"address"`
	AvgMS        float64 `json:"avg_ms"`
	MinMS        float64 `json:"min_ms"`
	MaxMS        float64 `json:"max_ms"`
	MedianMS     float64 `json:"median_ms"`
	Count        int     `json:"count"`
	Status       string  `json:"status"`
	TimeoutCount int     `json:"timeout_count,omitempty"`
	RefusedCount int     `json:"refused_count,omitempty"`
}

// BatchSummary is GET /batches/{id}.
type BatchSummary struct {
	BatchID      string         `json:"batch_id"`
	Tag          string         `json:"tag,omitempty"`
	Total        int            `json:"total"`
	StatusCounts map[string]int `json:"status_counts"`
	Grades       map[string]int `json:"grades,omitempty"`
	Items        []Job          `json:"items,omitempty"`
	Limit        int            `json:"limit,omitempty"`
	Offset       int            `json:"offset,omitempty"`
	NextCursor   string         `json:"next_cursor,omitempty"`
	CreatedAt    time.Time      `json:"created_at"`
	FinishedAt   *time.Time     `json:"finished_at,omitempty"`
}

// BatchListItem is one row of GET /batches.
type BatchListItem struct {
	BatchID     string     `json:"batch_id"`
	Tag         string     `json:"tag,omitempty"`
	Description string     `json:"description,omitempty"`
	Status      string     `json:"status"`
	Total       int        `json:"total"`
	Completed   int        `json:"completed"`
	Completion  int        `json:"completion"`
	CreatedAt   time.Time  `json:"created_at"`
	FinishedAt  *time.Time `json:"finished_at,omitempty"`
}

// BatchList is GET /batches.
type BatchList struct {
	Items []BatchListItem `json:"items"`
	Total int             `json:"total"`
}

// BatchCreateRequest is the POST /jobs/batch payload the fake decodes.
type BatchCreateRequest struct {
	Domains  []string `json:"domains,omitempty"`
	FromTag  string   `json:"from_tag,omitempty"`
	Profile  string   `json:"profile,omitempty"`
	Tags     []string `json:"tags,omitempty"`
	Tests    []string `json:"tests,omitempty"`
	MinLevel string   `json:"min_level,omitempty"`
}

// BatchCreateResponse is POST /jobs/batch.
type BatchCreateResponse struct {
	BatchID string   `json:"batch_id"`
	JobIDs  []string `json:"job_ids"`
}

// TagValue is one row of GET /batches/{id}/tag-values.
type TagValue struct {
	Value         string   `json:"value"`
	Count         int      `json:"count"`
	AvgScore      *float64 `json:"avg_score,omitempty"`
	SampleDomains []string `json:"sample_domains"`
}

// TagValues is GET /batches/{id}/tag-values.
type TagValues struct {
	BatchID       string     `json:"batch_id"`
	Tag           string     `json:"tag"`
	Arg           string     `json:"arg"`
	MinCount      int        `json:"min_count"`
	WeightByScore bool       `json:"weight_by_score,omitempty"`
	Values        []TagValue `json:"values"`
}

// EntryRecord is one row of GET /entries.
type EntryRecord struct {
	Domain string `json:"domain"`
	Module string `json:"module"`
	Tag    string `json:"tag"`
	Level  string `json:"level"`
}

// EntryList is GET /entries.
type EntryList struct {
	Items []EntryRecord `json:"items"`
	Total int           `json:"total"`
}

// SpecTestcase is one item of GET /spec/testcases.
type SpecTestcase struct {
	ID          string `json:"id"`
	Module      string `json:"module"`
	Description string `json:"description"`
}

// SpecTestcaseList is GET /spec/testcases.
type SpecTestcaseList struct {
	Items []SpecTestcase `json:"items"`
	Total int            `json:"total"`
}

// SpecTag is one tag of a testcase detail.
type SpecTag struct {
	Tag     string `json:"tag"`
	Message string `json:"message"`
}

// SpecTestcaseDetail is GET /spec/testcases/{id}.
type SpecTestcaseDetail struct {
	ID          string    `json:"id"`
	Module      string    `json:"module"`
	Description string    `json:"description"`
	Locale      string    `json:"locale"`
	Tags        []SpecTag `json:"tags"`
}

// Whoami is GET /whoami.
type Whoami struct {
	Mode          string `json:"mode"`
	Authenticated bool   `json:"authenticated"`
}

// The tags of the run pair DiffPair returns, so a test can name what it
// expects rather than repeat a literal.
const (
	DiffTagUnchanged = "B01_CHILD_FOUND"
	DiffTagRemoved   = "N11_NO_RESPONSE"
	DiffTagAdded     = "MULTIPLE_SOA_SERIALS"
	DiffTagChanged   = "ONE_SOA_SERIAL"
)

// DiffPair is the before/after run the run-diff tests on both CLIs share: one
// tag unchanged, one gone, one new, and one whose severity rises. Every delta
// kind a diff reports appears exactly once, so a diff that misses a kind
// cannot pass.
func DiffPair() (before, after []Entry) {
	before = []Entry{
		{Module: "BASIC", Tag: DiffTagUnchanged, Level: "INFO"},
		{Module: "NAMESERVER", Tag: DiffTagRemoved, Level: "WARNING"},
		{Module: "CONSISTENCY", Tag: DiffTagChanged, Level: "INFO"},
	}
	after = []Entry{
		{Module: "BASIC", Tag: DiffTagUnchanged, Level: "INFO"},
		{Module: "CONSISTENCY", Tag: DiffTagChanged, Level: "NOTICE"},
		{Module: "CONSISTENCY", Tag: DiffTagAdded, Level: "ERROR"},
	}
	return before, after
}

// DiffResults is DiffPair as two scored results keyed by run id, for the
// endpoints that serve a whole result rather than raw entries.
func DiffResults(idBefore, idAfter string) map[string]Result {
	before, after := DiffPair()
	return map[string]Result{
		idBefore: {
			JobID: idBefore, Status: "succeeded",
			Score: &Score{Score: 90, Grade: "A"},
			Raw:   &ResultRaw{Entries: before},
		},
		idAfter: {
			JobID: idAfter, Status: "succeeded",
			Score: &Score{Score: 70, Grade: "C"},
			Raw:   &ResultRaw{Entries: after},
		},
	}
}

// AnalysisCohortView is one cohort of GET /pub/api/v1/analysis/cohorts.
type AnalysisCohortView struct {
	DatasetTag string `json:"dataset_tag"`
	Label      string `json:"label"`
	IsDefault  bool   `json:"is_default"`
}

// AnalysisCatalog is GET /pub/api/v1/analysis/catalog.
type AnalysisCatalog struct {
	DefaultTag string               `json:"default_tag,omitempty"`
	Cohorts    []AnalysisCohortView `json:"cohorts"`
}

// AnalysisSnapshot is one row of a cohort's snapshot list.
type AnalysisSnapshot struct {
	Slug                string    `json:"slug"`
	Label               string    `json:"label,omitempty"`
	CapturedAt          time.Time `json:"captured_at"`
	DomainCount         int       `json:"domain_count"`
	EngineVersion       string    `json:"engine_version,omitempty"`
	IsDefault           bool      `json:"is_default,omitempty"`
	VocabularyAvailable bool      `json:"vocabulary_available"`
}

// AnalysisSnapshotList is GET /pub/api/v1/analysis/cohorts/{tag}/snapshots,
// newest snapshot first.
type AnalysisSnapshotList struct {
	DatasetTag string             `json:"dataset_tag"`
	Label      string             `json:"label"`
	Snapshots  []AnalysisSnapshot `json:"snapshots"`
}

// AnalysisVocabularyEntry is one tag only one side's profile levels.
type AnalysisVocabularyEntry struct {
	Tag    string `json:"tag"`
	Module string `json:"module,omitempty"`
	Level  string `json:"level,omitempty"`
}

// AnalysisVocabularyChange is one tag the two profiles level differently.
type AnalysisVocabularyChange struct {
	Tag       string `json:"tag"`
	Module    string `json:"module,omitempty"`
	FromLevel string `json:"from_level"`
	ToLevel   string `json:"to_level"`
}

// AnalysisVocabularyDelta is the tag vocabulary comparison of two snapshots.
type AnalysisVocabularyDelta struct {
	FromAvailable bool                       `json:"from_available"`
	ToAvailable   bool                       `json:"to_available"`
	FromTagCount  int                        `json:"from_tag_count"`
	ToTagCount    int                        `json:"to_tag_count"`
	Added         []AnalysisVocabularyEntry  `json:"added"`
	Removed       []AnalysisVocabularyEntry  `json:"removed"`
	LevelChanged  []AnalysisVocabularyChange `json:"level_changed"`
}

// AnalysisReportSide identifies one snapshot in the report header.
type AnalysisReportSide struct {
	Slug              string    `json:"slug"`
	Label             string    `json:"label,omitempty"`
	CapturedAt        time.Time `json:"captured_at"`
	EngineVersion     string    `json:"engine_version,omitempty"`
	ProfileName       string    `json:"profile_name,omitempty"`
	ScoringConfigHash string    `json:"scoring_config_hash,omitempty"`
	TagViewMinLevel   string    `json:"tag_view_min_level,omitempty"`
	DomainCount       int       `json:"domain_count"`
}

// AnalysisEngineDelta is the engine provenance of a snapshot pair.
type AnalysisEngineDelta struct {
	FromEngineVersion string `json:"from_engine_version,omitempty"`
	ToEngineVersion   string `json:"to_engine_version,omitempty"`
	Crossed           bool   `json:"crossed_engine_versions"`
	Unknown           bool   `json:"engine_version_unknown,omitempty"`
}

// AnalysisReportHeader is the provenance block of a report.
type AnalysisReportHeader struct {
	From                 AnalysisReportSide      `json:"from"`
	To                   AnalysisReportSide      `json:"to"`
	Engine               AnalysisEngineDelta     `json:"engine"`
	Vocabulary           AnalysisVocabularyDelta `json:"vocabulary"`
	ScoringConfigChanged string                  `json:"scoring_config_changed"`
	TagFloor             string                  `json:"tag_floor,omitempty"`
}

// AnalysisReportTotals is the headline count of what moved.
type AnalysisReportTotals struct {
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

// AnalysisReportTagEntry is one cohort-wide tag row with its classification.
type AnalysisReportTagEntry struct {
	Tag             string `json:"tag"`
	Module          string `json:"module,omitempty"`
	Testcase        string `json:"testcase,omitempty"`
	FromLevel       string `json:"from_level,omitempty"`
	ToLevel         string `json:"to_level,omitempty"`
	FromDomainCount int    `json:"from_domain_count"`
	ToDomainCount   int    `json:"to_domain_count"`
	DomainDelta     int    `json:"domain_delta"`
	Classification  string `json:"classification"`
}

// AnalysisReportTags groups the cohort-wide tag rows.
type AnalysisReportTags struct {
	Appeared     []AnalysisReportTagEntry `json:"appeared"`
	Cleared      []AnalysisReportTagEntry `json:"cleared"`
	LevelChanged []AnalysisReportTagEntry `json:"level_changed"`
}

// AnalysisReportTagChange is one tag that moved on one domain.
type AnalysisReportTagChange struct {
	Tag            string `json:"tag"`
	Module         string `json:"module,omitempty"`
	FromLevel      string `json:"from_level,omitempty"`
	ToLevel        string `json:"to_level,omitempty"`
	Classification string `json:"classification"`
}

// AnalysisReportDomain is one domain whose score or grade moved.
type AnalysisReportDomain struct {
	Domain           string                    `json:"domain"`
	FromScore        *int                      `json:"from_score,omitempty"`
	ToScore          *int                      `json:"to_score,omitempty"`
	ScoreDelta       *int                      `json:"score_delta,omitempty"`
	FromGrade        string                    `json:"from_grade,omitempty"`
	ToGrade          string                    `json:"to_grade,omitempty"`
	GradeChanged     bool                      `json:"grade_changed"`
	Category         string                    `json:"category"`
	ExplainedDelta   int                       `json:"explained_delta"`
	UnexplainedDelta int                       `json:"unexplained_delta"`
	Appeared         []AnalysisReportTagChange `json:"appeared"`
	Cleared          []AnalysisReportTagChange `json:"cleared"`
	LevelChanged     []AnalysisReportTagChange `json:"level_changed"`
}

// AnalysisReportClusterDimension is one dimension a cluster shares.
type AnalysisReportClusterDimension struct {
	Dimension    string `json:"dimension"`
	Value        string `json:"value"`
	Label        string `json:"label,omitempty"`
	TotalDomains int    `json:"total_domains"`
}

// AnalysisReportCluster is a set of domains that moved together.
type AnalysisReportCluster struct {
	Dimensions []AnalysisReportClusterDimension `json:"dimensions"`
	Domains    []string                         `json:"domains"`
	Size       int                              `json:"size"`
	MinDelta   int                              `json:"min_delta"`
	MaxDelta   int                              `json:"max_delta"`
	Direction  string                           `json:"direction"`
}

// AnalysisReport is GET /pub/api/v1/analysis/cohorts/{tag}/report.
type AnalysisReport struct {
	DatasetTag string                  `json:"dataset_tag"`
	FromSlug   string                  `json:"from_slug"`
	ToSlug     string                  `json:"to_slug"`
	MinCluster int                     `json:"min_cluster"`
	MaxSpread  int                     `json:"max_spread"`
	Header     AnalysisReportHeader    `json:"header"`
	Totals     AnalysisReportTotals    `json:"totals"`
	Tags       AnalysisReportTags      `json:"tags"`
	Domains    []AnalysisReportDomain  `json:"domains"`
	Clusters   []AnalysisReportCluster `json:"clusters"`
}

// Identifiers of the report fixture, so both CLIs assert on names.
const (
	ReportDatasetTag  = "kommuner"
	ReportFromSlug    = "2026-06"
	ReportToSlug      = "2026-09"
	ReportEngineTag   = "Z09_NO_RESPONSE_MX_QUERY"
	ReportCohortTag   = "DS08_DNSKEY_RRSIG_EXPIRED"
	ReportRealDomain  = "osteraker.se"
	ReportMeasDomain  = "salem.se"
	ReportClusterHost = "ns1.example.net"
)

// SnapshotPair is the two-snapshot list the report fixture belongs to,
// newest first.
func SnapshotPair() AnalysisSnapshotList {
	return AnalysisSnapshotList{
		DatasetTag: ReportDatasetTag,
		Label:      "Kommuner",
		Snapshots: []AnalysisSnapshot{
			{Slug: ReportToSlug, CapturedAt: time.Unix(1789700000, 0).UTC(), DomainCount: 3, EngineVersion: "1.7.10", IsDefault: true, VocabularyAvailable: true},
			{Slug: ReportFromSlug, CapturedAt: time.Unix(1780400000, 0).UTC(), DomainCount: 3, EngineVersion: "1.7.0", VocabularyAvailable: true},
		},
	}
}

// SampleReport carries one row of every classification, one domain per
// category, and one cluster, so a renderer that drops a section fails.
func SampleReport() AnalysisReport {
	fromMean, toMean := 91.07, 91.21
	fromScore, toScore := 88, 76
	measFrom, measTo := 82, 90
	regressed, improved := -12, 8
	return AnalysisReport{
		DatasetTag: ReportDatasetTag,
		FromSlug:   ReportFromSlug,
		ToSlug:     ReportToSlug,
		MinCluster: 3,
		MaxSpread:  3,
		Header: AnalysisReportHeader{
			From: AnalysisReportSide{Slug: ReportFromSlug, CapturedAt: time.Unix(1780400000, 0).UTC(), EngineVersion: "1.7.0", ProfileName: "default", DomainCount: 3},
			To:   AnalysisReportSide{Slug: ReportToSlug, CapturedAt: time.Unix(1789700000, 0).UTC(), EngineVersion: "1.7.10", ProfileName: "default", DomainCount: 3},
			Engine: AnalysisEngineDelta{
				FromEngineVersion: "1.7.0", ToEngineVersion: "1.7.10", Crossed: true,
			},
			Vocabulary: AnalysisVocabularyDelta{
				FromAvailable: true, ToAvailable: true, FromTagCount: 570, ToTagCount: 571,
				Added:        []AnalysisVocabularyEntry{{Tag: ReportEngineTag, Module: "ZONE", Level: "WARNING"}},
				Removed:      []AnalysisVocabularyEntry{},
				LevelChanged: []AnalysisVocabularyChange{{Tag: "N11_NO_RESPONSE", Module: "NAMESERVER", FromLevel: "NOTICE", ToLevel: "WARNING"}},
			},
			ScoringConfigChanged: "false",
			TagFloor:             "NOTICE",
		},
		Totals: AnalysisReportTotals{
			FromDomainCount: 3, ToDomainCount: 3, BothDomainCount: 3,
			IdenticalScore: 1, Improved: 1, Regressed: 1,
			FromMeanScore: &fromMean, ToMeanScore: &toMean,
			FromGrades:       map[string]int{"A": 2, "B": 1},
			ToGrades:         map[string]int{"A": 2, "D": 1},
			DomainCategories: map[string]int{"real": 1, "measurement": 1},
		},
		Tags: AnalysisReportTags{
			Appeared: []AnalysisReportTagEntry{
				{Tag: ReportEngineTag, Module: "ZONE", ToLevel: "WARNING", ToDomainCount: 2, DomainDelta: 2, Classification: "new_in_engine"},
				{Tag: ReportCohortTag, Module: "DNSSEC", ToLevel: "ERROR", ToDomainCount: 1, DomainDelta: 1, Classification: "cohort_change"},
			},
			Cleared: []AnalysisReportTagEntry{
				{Tag: "N15_SOFTWARE_VERSION", Module: "NAMESERVER", FromLevel: "NOTICE", FromDomainCount: 1, DomainDelta: -1, Classification: "removed_from_engine"},
			},
			LevelChanged: []AnalysisReportTagEntry{
				{Tag: "N11_NO_RESPONSE", Module: "NAMESERVER", FromLevel: "NOTICE", ToLevel: "WARNING", FromDomainCount: 1, ToDomainCount: 1, Classification: "level_reclassified"},
			},
		},
		Domains: []AnalysisReportDomain{
			{
				Domain: ReportRealDomain, FromScore: &fromScore, ToScore: &toScore, ScoreDelta: &regressed,
				FromGrade: "B", ToGrade: "D", GradeChanged: true, Category: "real",
				ExplainedDelta: -12, UnexplainedDelta: 0,
				Appeared: []AnalysisReportTagChange{{Tag: ReportCohortTag, Module: "DNSSEC", ToLevel: "ERROR", Classification: "cohort_change"}},
			},
			{
				Domain: ReportMeasDomain, FromScore: &measFrom, ToScore: &measTo, ScoreDelta: &improved,
				FromGrade: "A", ToGrade: "A", Category: "measurement",
				ExplainedDelta: 8, UnexplainedDelta: 0,
				Cleared: []AnalysisReportTagChange{{Tag: "N15_SOFTWARE_VERSION", Module: "NAMESERVER", FromLevel: "NOTICE", Classification: "removed_from_engine"}},
			},
		},
		Clusters: []AnalysisReportCluster{
			{
				Dimensions: []AnalysisReportClusterDimension{{Dimension: "nameserver", Value: ReportClusterHost, TotalDomains: 9}},
				Domains:    []string{ReportMeasDomain, "degerfors.se", "mala.se"},
				Size:       3, MinDelta: 8, MaxDelta: 9, Direction: "improved",
			},
		},
	}
}
