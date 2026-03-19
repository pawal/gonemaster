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

// JobStatus describes the current state of a job.
type JobStatus string

const (
	// Job status values.
	JobQueued JobStatus = "queued"
	// JobRunning indicates a job currently executing.
	JobRunning JobStatus = "running"
	// JobSucceeded indicates a job completed successfully.
	JobSucceeded JobStatus = "succeeded"
	// JobFailed indicates a job completed with an error.
	JobFailed JobStatus = "failed"
	// JobCanceled indicates a job was canceled.
	JobCanceled JobStatus = "canceled"
	// JobExpired indicates a job expired before completion.
	JobExpired JobStatus = "expired"
	// JobPaused indicates a job is paused in the queue.
	JobPaused JobStatus = "paused"
)

// JobSort controls ordering in list responses.
type JobSort string

const (
	// Job sort values for list and batch endpoints.
	JobSortCreatedAtDesc JobSort = "created_at_desc"
	// JobSortCreatedAtAsc sorts by creation time ascending.
	JobSortCreatedAtAsc JobSort = "created_at_asc"
	// JobSortStartedAtDesc sorts by start time descending.
	JobSortStartedAtDesc JobSort = "started_at_desc"
	// JobSortStartedAtAsc sorts by start time ascending.
	JobSortStartedAtAsc JobSort = "started_at_asc"
	// JobSortDomainAsc sorts by domain ascending.
	JobSortDomainAsc JobSort = "domain_asc"
	// JobSortDomainDesc sorts by domain descending.
	JobSortDomainDesc JobSort = "domain_desc"
	// JobSortBatchIDAsc sorts by batch id ascending.
	JobSortBatchIDAsc JobSort = "batch_id_asc"
	// JobSortBatchIDDesc sorts by batch id descending.
	JobSortBatchIDDesc JobSort = "batch_id_desc"
	// JobSortErrorDesc sorts by error-heavy jobs first.
	JobSortErrorDesc JobSort = "error_desc"
	// JobSortCriticalDesc sorts by critical-heavy jobs first.
	JobSortCriticalDesc JobSort = "critical_desc"
)

// JobSeverityFilter controls severity-based list filtering.
type JobSeverityFilter string

const (
	// Job severity filter values for list endpoints.
	JobSeverityWarningsPlus JobSeverityFilter = "warnings_plus"
	// JobSeverityErrorsOnly filters to jobs with errors or criticals.
	JobSeverityErrorsOnly JobSeverityFilter = "errors_only"
)

// Job represents a single test job.
type Job struct {
	ID             string                         `json:"id"`
	PublicID       string                         `json:"public_id,omitempty"`
	BatchID        string                         `json:"batch_id,omitempty"`
	Domain         string                         `json:"domain"`
	SeverityTotals map[string]int                 `json:"severity_totals,omitempty"`
	Tests          []string                       `json:"-"`
	Overrides      map[string]any                 `json:"-"`
	UndelegatedNS  []engine.UndelegatedNameserver `json:"-"`
	UndelegatedDS  []engine.UndelegatedDSInfo     `json:"-"`
	MinLevel       string                         `json:"-"`
	Status         JobStatus                      `json:"status"`
	CreatedAt      time.Time                      `json:"created_at"`
	StartedAt      time.Time                      `json:"started_at,omitempty"`
	FinishedAt     time.Time                      `json:"finished_at,omitempty"`
	Progress       int                            `json:"progress"`
	ResultURL      string                         `json:"result_url,omitempty"`
	Error          string                         `json:"error,omitempty"`
}

// JobCreateRequest is the payload for a single job.
type JobCreateRequest struct {
	Domain           string                       `json:"domain"`
	Tests            []string                     `json:"tests,omitempty"`
	ProfileOverrides map[string]any               `json:"profile_overrides,omitempty"`
	Nameservers      []UndelegatedNameserverInput `json:"nameservers,omitempty"`
	DSInfo           []UndelegatedDSInput         `json:"ds_info,omitempty"`
	MinLevel         string                       `json:"min_level,omitempty"`
}

// JobBatchRequest is the payload for a batch submission.
type JobBatchRequest struct {
	Domains          []string                      `json:"domains"`
	Tests            []string                      `json:"tests,omitempty"`
	ProfileOverrides map[string]any                `json:"profile_overrides,omitempty"`
	Nameservers      *[]UndelegatedNameserverInput `json:"nameservers,omitempty"`
	DSInfo           *[]UndelegatedDSInput         `json:"ds_info,omitempty"`
	MinLevel         string                        `json:"min_level,omitempty"`
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

// JobList represents list results.
type JobList struct {
	Items      []Job  `json:"items"`
	Total      int    `json:"total"`
	Limit      int    `json:"limit,omitempty"`
	Offset     int    `json:"offset,omitempty"`
	NextCursor string `json:"next_cursor,omitempty"`
	PrevCursor string `json:"prev_cursor,omitempty"`
	Sort       string `json:"sort,omitempty"`
}

// JobResult holds output for a job.
type JobResult struct {
	JobID   string         `json:"job_id"`
	BatchID string         `json:"batch_id,omitempty"`
	Status  JobStatus      `json:"status"`
	Summary map[string]any `json:"summary,omitempty"`
	Raw     *JobResultRaw  `json:"raw,omitempty"`
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

// BatchSummary aggregates jobs for a batch.
type BatchSummary struct {
	BatchID      string         `json:"batch_id"`
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
