package server

import (
	"errors"
	"sort"
	"sync"
	"time"
)

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
		if !filter.CreatedAfter.IsZero() && job.CreatedAt.Before(filter.CreatedAfter) {
			continue
		}
		items = append(items, job)
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].CreatedAt.Before(items[j].CreatedAt)
	})

	total := len(items)
	if filter.Offset < 0 {
		filter.Offset = 0
	}
	if filter.Limit <= 0 {
		filter.Limit = 100
	}
	start := filter.Offset
	if start > len(items) {
		start = len(items)
	}
	end := start + filter.Limit
	if end > len(items) {
		end = len(items)
	}

	return JobList{Items: items[start:end], Total: total}
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
