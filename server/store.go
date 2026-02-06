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
	Update(job Job) error
	List(filter JobFilter) JobList
	SetResult(jobID string, result JobResult) error
	GetResult(jobID string) (JobResult, bool)
}

// InMemoryJobStore stores jobs in memory.
type InMemoryJobStore struct {
	mu      sync.Mutex
	jobs    map[string]Job
	results map[string]JobResult
}

func NewInMemoryJobStore() *InMemoryJobStore {
	return &InMemoryJobStore{
		jobs:    map[string]Job{},
		results: map[string]JobResult{},
	}
}

func (s *InMemoryJobStore) Create(job Job) (Job, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.jobs[job.ID]; exists {
		return Job{}, errors.New("job already exists")
	}
	s.jobs[job.ID] = job
	return job, nil
}

func (s *InMemoryJobStore) Get(id string) (Job, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	job, ok := s.jobs[id]
	return job, ok
}

func (s *InMemoryJobStore) Update(job Job) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.jobs[job.ID]; !exists {
		return errors.New("job not found")
	}
	s.jobs[job.ID] = job
	return nil
}

func (s *InMemoryJobStore) List(filter JobFilter) JobList {
	s.mu.Lock()
	defer s.mu.Unlock()

	items := make([]Job, 0, len(s.jobs))
	for _, job := range s.jobs {
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
	severityByJobID := make(map[string]map[string]int, len(items))
	for _, job := range items {
		totals := zeroSeverityTotals()
		if result, ok := s.results[job.ID]; ok {
			totals = severityTotalsFromSummary(result.Summary)
		}
		severityByJobID[job.ID] = totals
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

func (s *InMemoryJobStore) SetResult(jobID string, result JobResult) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.jobs[jobID]; !exists {
		return errors.New("job not found")
	}
	s.results[jobID] = result
	return nil
}

func (s *InMemoryJobStore) GetResult(jobID string) (JobResult, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	result, ok := s.results[jobID]
	return result, ok
}

func parseTime(value string) (time.Time, error) {
	if value == "" {
		return time.Time{}, nil
	}
	return time.Parse(time.RFC3339, value)
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
