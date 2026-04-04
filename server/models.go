package server

import (
	"crypto/rand"
	"math/big"
	"time"

	"codeberg.org/pawal/gonemaster/engine"
)

const publicIDAlphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
const publicIDLen = 8

// GeneratePublicID returns an 8-character base62 string using crypto/rand.
func GeneratePublicID() string {
	b := make([]byte, publicIDLen)
	alphabetLen := big.NewInt(int64(len(publicIDAlphabet)))
	for i := range b {
		n, err := rand.Int(rand.Reader, alphabetLen)
		if err != nil {
			panic("crypto/rand unavailable: " + err.Error())
		}
		b[i] = publicIDAlphabet[n.Int64()]
	}
	return string(b)
}

// JobPriority controls queue ordering. Normal jobs are always dequeued before
// batch jobs. Within each tier, FIFO order is preserved.
type JobPriority int

const (
	PriorityNormal JobPriority = 0 // interactive: single-domain submit
	PriorityBatch  JobPriority = 1 // background: batch / tag sweep
)

// JobStatus describes the current state of a job.
type JobStatus string

const (
	JobQueued    JobStatus = "queued"
	JobRunning   JobStatus = "running"
	JobSucceeded JobStatus = "succeeded"
	JobFailed    JobStatus = "failed"
	JobCanceled  JobStatus = "canceled"
	JobExpired   JobStatus = "expired"
	JobPaused    JobStatus = "paused"
)

// JobSort controls ordering in list responses.
type JobSort string

const (
	JobSortCreatedAtDesc JobSort = "created_at_desc"
	JobSortCreatedAtAsc  JobSort = "created_at_asc"
	JobSortStartedAtDesc JobSort = "started_at_desc"
	JobSortStartedAtAsc  JobSort = "started_at_asc"
	JobSortDomainAsc     JobSort = "domain_asc"
	JobSortDomainDesc    JobSort = "domain_desc"
	JobSortBatchIDAsc    JobSort = "batch_id_asc"
	JobSortBatchIDDesc   JobSort = "batch_id_desc"
	JobSortErrorDesc     JobSort = "error_desc"
	JobSortCriticalDesc  JobSort = "critical_desc"
)

// JobSeverityFilter controls severity-based list filtering.
type JobSeverityFilter string

const (
	JobSeverityWarningsPlus JobSeverityFilter = "warnings_plus"
	JobSeverityErrorsOnly   JobSeverityFilter = "errors_only"
)

// Job represents a single test job (queue entry).
// Once completed, the job graduates to a Run + Entries.
// FinishedAt and SeverityTotals are populated from the associated Run when
// the job is terminal; they are not stored in the jobs table itself.
type Job struct {
	ID       string    `json:"id"`
	PublicID string    `json:"public_id,omitempty"`
	BatchID  string    `json:"batch_id,omitempty"`
	DomainID int64     `json:"-"`
	Domain   string    `json:"domain"`
	Status   JobStatus `json:"status"`
	// FinishedAt is populated from the Run after graduation; zero for in-flight jobs.
	FinishedAt       time.Time      `json:"finished_at,omitempty"`
	SeverityTotals   map[string]int `json:"severity_totals,omitempty"`
	CreatedAt        time.Time      `json:"created_at"`
	StartedAt        time.Time      `json:"started_at,omitempty"`
	Priority         JobPriority    `json:"priority"`
	Progress         int            `json:"progress"`
	Error            string         `json:"error,omitempty"`
	Profile          string         `json:"-"`
	ProfileID        *int64         `json:"profile_id,omitempty"`
	ProfileName      string         `json:"profile_name,omitempty"`
	EffectiveProfile string         `json:"-"`
	// Config fields stored as config_json in the DB.
	Tests         []string                       `json:"-"`
	Overrides     map[string]any                 `json:"-"`
	UndelegatedNS []engine.UndelegatedNameserver `json:"-"`
	UndelegatedDS []engine.UndelegatedDSInfo     `json:"-"`
	MinLevel      string                         `json:"-"`
}

// Domain is a persistent domain registry entry.
type Domain struct {
	ID           int64     `json:"id"`
	Name         string    `json:"name"`
	LatestRunID  string    `json:"latest_run_id,omitempty"`
	LatestRunAt  time.Time `json:"latest_run_at,omitempty"`
	LatestStatus string    `json:"latest_status,omitempty"`
	LatestLevel  string    `json:"latest_level,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	RunCount     int       `json:"run_count"`
	Tags         []string  `json:"tags,omitempty"`
}

// Tag is a named domain collection.
type Tag struct {
	Name             string    `json:"name"`
	Description      string    `json:"description,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
	DomainCount      int       `json:"domain_count"`
	DefaultProfileID *int64    `json:"default_profile_id,omitempty"`
}

// TagSummary holds per-severity domain counts for a tag.
type TagSummary struct {
	Tag         string `json:"tag"`
	DomainCount int    `json:"domain_count"`
	OK          int    `json:"ok"`
	Notice      int    `json:"notice"`
	Warning     int    `json:"warning"`
	Error       int    `json:"error"`
	Critical    int    `json:"critical"`
}

// Run is a completed execution, graduated from a Job.
type Run struct {
	ID               string      `json:"id"`
	DomainID         int64       `json:"domain_id"`
	Domain           string      `json:"domain"`
	BatchID          string      `json:"batch_id,omitempty"`
	Status           JobStatus   `json:"status"`
	CreatedAt        time.Time   `json:"created_at"`
	StartedAt        time.Time   `json:"started_at,omitempty"`
	FinishedAt       time.Time   `json:"finished_at,omitempty"`
	DurationMs       int64       `json:"duration_ms,omitempty"`
	SevNotice        int         `json:"sev_notice"`
	SevWarning       int         `json:"sev_warning"`
	SevError         int         `json:"sev_error"`
	SevCritical      int         `json:"sev_critical"`
	WorstLevel       string      `json:"worst_level,omitempty"`
	EntryCount       int         `json:"entry_count"`
	Priority         JobPriority `json:"priority"`
	Profile          string      `json:"profile,omitempty"`
	ProfileID        *int64      `json:"profile_id,omitempty"`
	ProfileName      string      `json:"profile_name,omitempty"`
	EffectiveProfile string      `json:"effective_profile,omitempty"`
	PublicID         string      `json:"public_id,omitempty"`
	// SeverityTotals mirrors the sev_* columns as a map for API compat.
	SeverityTotals map[string]int `json:"severity_totals,omitempty"`
}

// Entry is a single engine log entry stored as a row for SQL analysis.
type Entry struct {
	ID        int64          `json:"id"`
	RunID     string         `json:"run_id"`
	DomainID  int64          `json:"domain_id"`
	Domain    string         `json:"domain,omitempty"`
	Timestamp float64        `json:"timestamp"`
	Module    string         `json:"module"`
	Testcase  string         `json:"testcase"`
	Tag       string         `json:"tag"`
	Level     string         `json:"level"`
	Args      map[string]any `json:"args,omitempty"`
}

// Batch is metadata for a batch submission.
type Batch struct {
	ID          string    `json:"id"`
	Tag         string    `json:"tag,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	DomainCount int       `json:"domain_count"`
	Description string    `json:"description,omitempty"`
}

// StoredProfile is a named, server-stored test configuration.
type StoredProfile struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Config      string    `json:"config"` // JSON, same schema as profile_overrides
	Public      bool      `json:"public"` // visible in public UI dropdown
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Profile is the API representation of a stored profile.
type Profile struct {
	ID          int64          `json:"id"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Config      map[string]any `json:"config"`
	Public      bool           `json:"public"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
}

// ── Filter types ──────────────────────────────────────────────────────────────

// JobFilter controls listing behavior.
type JobFilter struct {
	Status        JobStatus
	BatchID       string
	Domain        string
	Severity      JobSeverityFilter
	CreatedAfter  time.Time
	CreatedBefore time.Time
	Limit         int
	Offset        int
	Sort          JobSort
}

// DomainFilter filters domain list queries.
type DomainFilter struct {
	Tag         string
	Name        string
	LatestLevel string
	MinLevel    string // minimum severity threshold (inclusive); "WARNING" matches WARNING/ERROR/CRITICAL
	Limit       int
	Offset      int
}

// RunFilter filters run list queries.
type RunFilter struct {
	DomainID       int64
	Domain         string
	BatchID        string
	Tag            string
	Status         JobStatus
	WorstLevel     string
	FinishedAfter  time.Time
	FinishedBefore time.Time
	Limit          int
	Offset         int
	Sort           JobSort
}

// EntryFilter filters cross-run entry queries.
type EntryFilter struct {
	RunID      string // exact run ID
	DomainID   int64  // exact domain ID
	Tag        string // domain tag filter (join via domain_tags)
	Module     string // exact module name
	Testcase   string // exact testcase name
	EntryTag   string // log event tag (entries.tag column)
	Level      string // exact level
	LatestOnly bool   // restrict to entries from each domain's latest run
	BatchID    string // restrict to runs from a batch
	Limit      int
	Offset     int
}

// ── List result types ─────────────────────────────────────────────────────────

// JobList represents paginated job results.
type JobList struct {
	Items      []Job  `json:"items"`
	Total      int    `json:"total"`
	Limit      int    `json:"limit,omitempty"`
	Offset     int    `json:"offset,omitempty"`
	NextCursor string `json:"next_cursor,omitempty"`
	PrevCursor string `json:"prev_cursor,omitempty"`
	Sort       string `json:"sort,omitempty"`
}

// DomainList represents paginated domain results.
type DomainList struct {
	Items      []Domain `json:"items"`
	Total      int      `json:"total"`
	Limit      int      `json:"limit,omitempty"`
	Offset     int      `json:"offset,omitempty"`
	NextCursor string   `json:"next_cursor,omitempty"`
	PrevCursor string   `json:"prev_cursor,omitempty"`
}

// RunList represents paginated run results.
type RunList struct {
	Items      []Run  `json:"items"`
	Total      int    `json:"total"`
	Limit      int    `json:"limit,omitempty"`
	Offset     int    `json:"offset,omitempty"`
	NextCursor string `json:"next_cursor,omitempty"`
	PrevCursor string `json:"prev_cursor,omitempty"`
}

// EntryList represents paginated entry results.
type EntryList struct {
	Items      []Entry `json:"items"`
	Total      int     `json:"total"`
	Limit      int     `json:"limit,omitempty"`
	Offset     int     `json:"offset,omitempty"`
	NextCursor string  `json:"next_cursor,omitempty"`
	PrevCursor string  `json:"prev_cursor,omitempty"`
}

// ── Result types (unchanged shape for API compat) ────────────────────────────

// JobResult holds the assembled output for a job/run.
type JobResult struct {
	JobID                string            `json:"job_id"`
	BatchID              string            `json:"batch_id,omitempty"`
	Status               JobStatus         `json:"status"`
	Summary              map[string]any    `json:"summary,omitempty"`
	Raw                  *JobResultRaw     `json:"raw,omitempty"`
	TestcaseDescriptions map[string]string `json:"testcase_descriptions,omitempty"`
}

// JobResultRaw contains the raw log entries for a job.
type JobResultRaw struct {
	Locale  string           `json:"locale,omitempty"`
	Entries []JobResultEntry `json:"entries,omitempty"`
}

// JobResultEntry is a single log entry for API consumers.
type JobResultEntry struct {
	Timestamp float64        `json:"timestamp"`
	Module    string         `json:"module"`
	Testcase  string         `json:"testcase"`
	Tag       string         `json:"tag"`
	Level     string         `json:"level"`
	Args      map[string]any `json:"args,omitempty"`
	Message   string         `json:"message,omitempty"`
	Raw       string         `json:"raw,omitempty"`
}

// ── Request/response types ────────────────────────────────────────────────────

// JobCreateRequest is the payload for a single job.
type JobCreateRequest struct {
	Domain           string                       `json:"domain"`
	Tests            []string                     `json:"tests,omitempty"`
	ProfileOverrides map[string]any               `json:"profile_overrides,omitempty"`
	Nameservers      []UndelegatedNameserverInput `json:"nameservers,omitempty"`
	DSInfo           []UndelegatedDSInput         `json:"ds_info,omitempty"`
	MinLevel         string                       `json:"min_level,omitempty"`
	Tags             []string                     `json:"tags,omitempty"`
	Profile          string                       `json:"profile,omitempty"`
}

// JobBatchRequest is the payload for a batch submission.
type JobBatchRequest struct {
	Domains          []string                      `json:"domains,omitempty"`
	FromTag          string                        `json:"from_tag,omitempty"`
	Tests            []string                      `json:"tests,omitempty"`
	ProfileOverrides map[string]any                `json:"profile_overrides,omitempty"`
	Nameservers      *[]UndelegatedNameserverInput `json:"nameservers,omitempty"`
	DSInfo           *[]UndelegatedDSInput         `json:"ds_info,omitempty"`
	MinLevel         string                        `json:"min_level,omitempty"`
	Tags             []string                      `json:"tags,omitempty"`
	Profile          string                        `json:"profile,omitempty"`
	Description      string                        `json:"description,omitempty"`
}

// UndelegatedNameserverInput represents one undelegated nameserver row.
type UndelegatedNameserverInput struct {
	NS string `json:"ns"`
	IP string `json:"ip,omitempty"`
}

// UndelegatedDSInput represents one undelegated DS row.
type UndelegatedDSInput struct {
	KeyTag    int    `json:"keytag"`
	Algorithm int    `json:"algorithm"`
	DigType   int    `json:"digtype"`
	Digest    string `json:"digest"`
}

// JobBatchResponse describes the batch submission result.
type JobBatchResponse struct {
	BatchID string   `json:"batch_id"`
	JobIDs  []string `json:"job_ids"`
}

// BatchSummary aggregates jobs for a batch.
type BatchSummary struct {
	BatchID      string         `json:"batch_id"`
	Tag          string         `json:"tag,omitempty"`
	Total        int            `json:"total"`
	StatusCounts map[string]int `json:"status_counts"`
	Items        []Job          `json:"items"`
	Limit        int            `json:"limit,omitempty"`
	Offset       int            `json:"offset,omitempty"`
	NextCursor   string         `json:"next_cursor,omitempty"`
	PrevCursor   string         `json:"prev_cursor,omitempty"`
	Sort         string         `json:"sort,omitempty"`
	CreatedAt    time.Time      `json:"created_at"`
	StartedAt    *time.Time     `json:"started_at,omitempty"`
	FinishedAt   *time.Time     `json:"finished_at,omitempty"`
}

// QueueReorderRequest reorders queued jobs.
type QueueReorderRequest struct {
	JobIDs []string `json:"job_ids"`
}

// QueueRemoveRequest removes queued jobs by id.
type QueueRemoveRequest struct {
	JobIDs []string `json:"job_ids"`
}

// QueueRemoveResponse reports removed jobs.
type QueueRemoveResponse struct {
	Removed []string `json:"removed"`
}

// ErrorResponse is the standard error response body.
type ErrorResponse struct {
	Error ErrorBody `json:"error"`
}

// ErrorBody contains error details.
type ErrorBody struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}
