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
