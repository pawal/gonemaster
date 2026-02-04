package server

import "time"

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

// Job represents a single test job.
type Job struct {
	ID         string         `json:"id"`
	BatchID    string         `json:"batch_id,omitempty"`
	Domain     string         `json:"domain"`
	Tests      []string       `json:"-"`
	Overrides  map[string]any `json:"-"`
	MinLevel   string         `json:"-"`
	Status     JobStatus      `json:"status"`
	CreatedAt  time.Time      `json:"created_at"`
	StartedAt  time.Time      `json:"started_at,omitempty"`
	FinishedAt time.Time      `json:"finished_at,omitempty"`
	Progress   int            `json:"progress"`
	ResultURL  string         `json:"result_url,omitempty"`
	Error      string         `json:"error,omitempty"`
}

// JobCreateRequest is the payload for a single job.
type JobCreateRequest struct {
	Domain           string         `json:"domain"`
	Tests            []string       `json:"tests,omitempty"`
	ProfileOverrides map[string]any `json:"profile_overrides,omitempty"`
	MinLevel         string         `json:"min_level,omitempty"`
}

// JobBatchRequest is the payload for a batch submission.
type JobBatchRequest struct {
	Domains          []string       `json:"domains"`
	Tests            []string       `json:"tests,omitempty"`
	ProfileOverrides map[string]any `json:"profile_overrides,omitempty"`
	MinLevel         string         `json:"min_level,omitempty"`
}

// JobBatchResponse describes the batch submission result.
type JobBatchResponse struct {
	BatchID string   `json:"batch_id"`
	JobIDs  []string `json:"job_ids"`
}

// JobList represents list results.
type JobList struct {
	Items []Job `json:"items"`
	Total int   `json:"total"`
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
	Status       JobStatus
	BatchID      string
	CreatedAfter time.Time
	Limit        int
	Offset       int
}
