package server

import (
	"errors"
	"sync"
)

// Queue holds queued job ids.
type Queue interface {
	Enqueue(jobID string) error
	Pause() error
	Resume() error
	Reorder(jobIDs []string) error
}

// InMemoryQueue is a simple in-memory queue.
type InMemoryQueue struct {
	mu     sync.Mutex
	jobs   []string
	paused bool
}

func NewInMemoryQueue() *InMemoryQueue {
	return &InMemoryQueue{}
}

func (q *InMemoryQueue) Enqueue(jobID string) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.jobs = append(q.jobs, jobID)
	return nil
}

func (q *InMemoryQueue) Pause() error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.paused = true
	return nil
}

func (q *InMemoryQueue) Resume() error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.paused = false
	return nil
}

func (q *InMemoryQueue) Reorder(jobIDs []string) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(jobIDs) != len(q.jobs) {
		return errors.New("job id list does not match queue length")
	}
	seen := map[string]bool{}
	for _, id := range q.jobs {
		seen[id] = true
	}
	for _, id := range jobIDs {
		if !seen[id] {
			return errors.New("job id not in queue")
		}
	}
	q.jobs = append([]string(nil), jobIDs...)
	return nil
}
