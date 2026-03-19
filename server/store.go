package server

import (
	"errors"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

var severityLevels = []string{"NOTICE", "WARNING", "ERROR", "CRITICAL"}

// JobStore persists job metadata and results.
type JobStore interface {
	Create(job Job) (Job, error)
	Get(id string) (Job, bool)
	GetByPublicID(publicID string) (Job, bool)
	Update(job Job) error
	List(filter JobFilter) JobList
	SetResult(jobID string, result JobResult) error
	GetResult(jobID string) (JobResult, bool)
	// PurgeOlderThan deletes terminal-status jobs whose finished_at is before
	// cutoff, along with their results. Returns the number of jobs deleted.
	PurgeOlderThan(cutoff time.Time) (int64, error)
}

// InMemoryJobStore stores jobs in memory.
type InMemoryJobStore struct {
	jobsMu    sync.RWMutex
	resultsMu sync.RWMutex
	jobs      map[string]Job
	results   map[string]JobResult
	publicIDs map[string]string // publicID → job ID
}

// NewInMemoryJobStore creates an empty in-memory job store.
func NewInMemoryJobStore() *InMemoryJobStore {
	return &InMemoryJobStore{
		jobs:      map[string]Job{},
		results:   map[string]JobResult{},
		publicIDs: map[string]string{},
	}
}

// Create inserts a new job and fails if the id already exists.
// A PublicID is generated automatically if the job does not already have one.
func (s *InMemoryJobStore) Create(job Job) (Job, error) {
	s.jobsMu.Lock()
	defer s.jobsMu.Unlock()
	if _, exists := s.jobs[job.ID]; exists {
		return Job{}, errors.New("job already exists")
	}
	if job.PublicID == "" {
		job.PublicID = GeneratePublicID()
	}
	s.jobs[job.ID] = job
	s.publicIDs[job.PublicID] = job.ID
	return job, nil
}

// GetByPublicID returns a job looked up by its public ID.
func (s *InMemoryJobStore) GetByPublicID(publicID string) (Job, bool) {
	s.jobsMu.RLock()
	defer s.jobsMu.RUnlock()
	id, ok := s.publicIDs[publicID]
	if !ok {
		return Job{}, false
	}
	job, ok := s.jobs[id]
	return job, ok
}

// Get returns a job by id.
func (s *InMemoryJobStore) Get(id string) (Job, bool) {
	s.jobsMu.RLock()
	defer s.jobsMu.RUnlock()
	job, ok := s.jobs[id]
	return job, ok
}

// Update replaces an existing job by id.
func (s *InMemoryJobStore) Update(job Job) error {
	s.jobsMu.Lock()
	defer s.jobsMu.Unlock()
	if _, exists := s.jobs[job.ID]; !exists {
		return errors.New("job not found")
	}
	s.jobs[job.ID] = job
	return nil
}

// List returns jobs matching filter with sorting and pagination applied.
func (s *InMemoryJobStore) List(filter JobFilter) JobList {
	s.jobsMu.RLock()
	jobsSnapshot := make([]Job, 0, len(s.jobs))
	for _, job := range s.jobs {
		jobsSnapshot = append(jobsSnapshot, job)
	}
	s.jobsMu.RUnlock()

	items := make([]Job, 0, len(jobsSnapshot))
	for _, job := range jobsSnapshot {
		if filter.Status != "" && job.Status != filter.Status {
			continue
		}
		if filter.BatchID != "" && job.BatchID != filter.BatchID {
			continue
		}
		if filter.Domain != "" && !strings.Contains(strings.ToLower(job.Domain), strings.ToLower(filter.Domain)) {
			continue
		}
		if !filter.CreatedAfter.IsZero() && job.CreatedAt.Before(filter.CreatedAfter) {
			continue
		}
		if !filter.CreatedBefore.IsZero() && job.CreatedAt.After(filter.CreatedBefore) {
			continue
		}
		items = append(items, job)
	}

	normalizedSort := normalizeJobSort(filter.Sort)
	normalizedSeverity := normalizeJobSeverityFilter(filter.Severity)
	severityByJobID := make(map[string]map[string]int, len(items))
	s.resultsMu.RLock()
	for _, job := range items {
		totals := zeroSeverityTotals()
		if result, ok := s.results[job.ID]; ok {
			totals = severityTotalsFromSummary(result.Summary)
		}
		severityByJobID[job.ID] = totals
	}
	s.resultsMu.RUnlock()
	if normalizedSeverity != "" {
		filteredBySeverity := make([]Job, 0, len(items))
		for _, job := range items {
			if matchesJobSeverityFilter(severityByJobID[job.ID], normalizedSeverity) {
				filteredBySeverity = append(filteredBySeverity, job)
			}
		}
		items = filteredBySeverity
	}

	sort.Slice(items, func(i, j int) bool {
		left := items[i]
		right := items[j]
		switch normalizedSort {
		case JobSortCreatedAtAsc:
			if !left.CreatedAt.Equal(right.CreatedAt) {
				return left.CreatedAt.Before(right.CreatedAt)
			}
		case JobSortCreatedAtDesc:
			if !left.CreatedAt.Equal(right.CreatedAt) {
				return left.CreatedAt.After(right.CreatedAt)
			}
		case JobSortStartedAtDesc:
			leftStarted := effectiveStartTime(left)
			rightStarted := effectiveStartTime(right)
			if !leftStarted.Equal(rightStarted) {
				return leftStarted.After(rightStarted)
			}
		case JobSortStartedAtAsc:
			leftStarted := effectiveStartTime(left)
			rightStarted := effectiveStartTime(right)
			if !leftStarted.Equal(rightStarted) {
				return leftStarted.Before(rightStarted)
			}
		case JobSortDomainAsc:
			leftDomain := strings.ToLower(left.Domain)
			rightDomain := strings.ToLower(right.Domain)
			if leftDomain != rightDomain {
				return leftDomain < rightDomain
			}
		case JobSortDomainDesc:
			leftDomain := strings.ToLower(left.Domain)
			rightDomain := strings.ToLower(right.Domain)
			if leftDomain != rightDomain {
				return leftDomain > rightDomain
			}
		case JobSortBatchIDAsc:
			leftBatchID := strings.ToLower(left.BatchID)
			rightBatchID := strings.ToLower(right.BatchID)
			if leftBatchID != rightBatchID {
				return leftBatchID < rightBatchID
			}
		case JobSortBatchIDDesc:
			leftBatchID := strings.ToLower(left.BatchID)
			rightBatchID := strings.ToLower(right.BatchID)
			if leftBatchID != rightBatchID {
				return leftBatchID > rightBatchID
			}
		case JobSortErrorDesc:
			leftTotals := severityByJobID[left.ID]
			rightTotals := severityByJobID[right.ID]
			leftErrors := leftTotals["ERROR"] + leftTotals["CRITICAL"]
			rightErrors := rightTotals["ERROR"] + rightTotals["CRITICAL"]
			if leftErrors != rightErrors {
				return leftErrors > rightErrors
			}
			if leftTotals["CRITICAL"] != rightTotals["CRITICAL"] {
				return leftTotals["CRITICAL"] > rightTotals["CRITICAL"]
			}
		case JobSortCriticalDesc:
			leftTotals := severityByJobID[left.ID]
			rightTotals := severityByJobID[right.ID]
			if leftTotals["CRITICAL"] != rightTotals["CRITICAL"] {
				return leftTotals["CRITICAL"] > rightTotals["CRITICAL"]
			}
			leftErrors := leftTotals["ERROR"] + leftTotals["CRITICAL"]
			rightErrors := rightTotals["ERROR"] + rightTotals["CRITICAL"]
			if leftErrors != rightErrors {
				return leftErrors > rightErrors
			}
		default:
			leftStarted := effectiveStartTime(left)
			rightStarted := effectiveStartTime(right)
			if !leftStarted.Equal(rightStarted) {
				return leftStarted.After(rightStarted)
			}
		}
		if !left.CreatedAt.Equal(right.CreatedAt) {
			return left.CreatedAt.After(right.CreatedAt)
		}
		return left.ID < right.ID
	})

	total := len(items)
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}
	limit := filter.Limit
	if limit <= 0 {
		limit = 100
	}
	start := offset
	if start > len(items) {
		start = len(items)
	}
	end := start + limit
	if end > len(items) {
		end = len(items)
	}

	pageItems := make([]Job, end-start)
	copy(pageItems, items[start:end])
	for i := range pageItems {
		if totals, ok := severityByJobID[pageItems[i].ID]; ok {
			pageItems[i].SeverityTotals = totals
		} else {
			pageItems[i].SeverityTotals = zeroSeverityTotals()
		}
	}

	list := JobList{
		Items:  pageItems,
		Total:  total,
		Limit:  limit,
		Offset: offset,
		Sort:   string(normalizedSort),
	}
	if start > 0 {
		prevOffset := start - limit
		if prevOffset < 0 {
			prevOffset = 0
		}
		list.PrevCursor = strconv.Itoa(prevOffset)
	}
	if end < total {
		list.NextCursor = strconv.Itoa(end)
	}
	return list
}

// SetResult stores a result payload for an existing job.
func (s *InMemoryJobStore) SetResult(jobID string, result JobResult) error {
	s.jobsMu.RLock()
	if _, exists := s.jobs[jobID]; !exists {
		s.jobsMu.RUnlock()
		return errors.New("job not found")
	}
	s.jobsMu.RUnlock()

	s.resultsMu.Lock()
	s.results[jobID] = result
	s.resultsMu.Unlock()
	return nil
}

// GetResult returns a stored result payload by job id.
func (s *InMemoryJobStore) GetResult(jobID string) (JobResult, bool) {
	s.resultsMu.RLock()
	defer s.resultsMu.RUnlock()
	result, ok := s.results[jobID]
	return result, ok
}

// PurgeOlderThan deletes terminal-status jobs whose FinishedAt is before
// cutoff, along with their associated results.
func (s *InMemoryJobStore) PurgeOlderThan(cutoff time.Time) (int64, error) {
	s.jobsMu.Lock()
	s.resultsMu.Lock()
	defer s.jobsMu.Unlock()
	defer s.resultsMu.Unlock()

	var count int64
	for id, job := range s.jobs {
		if !isTerminalStatus(job.Status) {
			continue
		}
		if job.FinishedAt.IsZero() || !job.FinishedAt.Before(cutoff) {
			continue
		}
		delete(s.publicIDs, job.PublicID)
		delete(s.jobs, id)
		delete(s.results, id)
		count++
	}
	return count, nil
}

func isTerminalStatus(status JobStatus) bool {
	switch status {
	case JobSucceeded, JobFailed, JobCanceled, JobExpired:
		return true
	default:
		return false
	}
}

func parseTime(value string) (time.Time, error) {
	if value == "" {
		return time.Time{}, nil
	}
	t, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, err
	}
	return t.UTC(), nil
}

func normalizeJobSort(value JobSort) JobSort {
	switch value {
	case JobSortCreatedAtDesc, JobSortCreatedAtAsc, JobSortStartedAtDesc, JobSortStartedAtAsc, JobSortDomainAsc, JobSortDomainDesc, JobSortBatchIDAsc, JobSortBatchIDDesc, JobSortErrorDesc, JobSortCriticalDesc:
		return value
	default:
		return JobSortStartedAtDesc
	}
}

func isValidJobSort(value JobSort) bool {
	switch value {
	case JobSortCreatedAtDesc, JobSortCreatedAtAsc, JobSortStartedAtDesc, JobSortStartedAtAsc, JobSortDomainAsc, JobSortDomainDesc, JobSortBatchIDAsc, JobSortBatchIDDesc, JobSortErrorDesc, JobSortCriticalDesc:
		return true
	default:
		return false
	}
}

func isValidJobSeverityFilter(value JobSeverityFilter) bool {
	switch value {
	case JobSeverityWarningsPlus, JobSeverityErrorsOnly:
		return true
	default:
		return false
	}
}

func normalizeJobSeverityFilter(value JobSeverityFilter) JobSeverityFilter {
	switch value {
	case JobSeverityWarningsPlus, JobSeverityErrorsOnly:
		return value
	default:
		return ""
	}
}

func matchesJobSeverityFilter(totals map[string]int, filter JobSeverityFilter) bool {
	switch filter {
	case JobSeverityWarningsPlus:
		return totals["WARNING"] > 0 || totals["ERROR"] > 0 || totals["CRITICAL"] > 0
	case JobSeverityErrorsOnly:
		return totals["ERROR"] > 0 || totals["CRITICAL"] > 0
	default:
		return true
	}
}

func effectiveStartTime(job Job) time.Time {
	if !job.StartedAt.IsZero() {
		return job.StartedAt
	}
	return job.CreatedAt
}

func zeroSeverityTotals() map[string]int {
	return map[string]int{
		"NOTICE":   0,
		"WARNING":  0,
		"ERROR":    0,
		"CRITICAL": 0,
	}
}

func severityTotalsFromSummary(summary map[string]any) map[string]int {
	out := zeroSeverityTotals()
	if summary == nil {
		return out
	}
	levelsRaw, ok := summary["levels"]
	if !ok {
		return out
	}

	switch levels := levelsRaw.(type) {
	case map[string]int:
		for _, level := range severityLevels {
			if levels[level] > 0 {
				out[level] = levels[level]
			}
		}
	case map[string]any:
		for _, level := range severityLevels {
			out[level] = intFromAny(levels[level])
		}
	}
	return out
}

func intFromAny(value any) int {
	switch numeric := value.(type) {
	case int:
		return numeric
	case int32:
		return int(numeric)
	case int64:
		return int(numeric)
	case float32:
		return int(numeric)
	case float64:
		return int(numeric)
	default:
		return 0
	}
}
