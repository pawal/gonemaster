package server

import (
	"context"
	"errors"
	"sync"
)

// Queue holds queued job ids.
type Queue interface {
	Enqueue(jobID string) error
	Dequeue(ctx context.Context) (string, error)
	Pause() error
	Resume() error
	Reorder(jobIDs []string) error
	Close() error
}

// InMemoryQueue is a simple in-memory queue.
type InMemoryQueue struct {
	mu     sync.Mutex
	jobs   []string
	paused bool
	closed bool
	notify chan struct{}
}

func NewInMemoryQueue() *InMemoryQueue {
	return &InMemoryQueue{notify: make(chan struct{})}
}

func (q *InMemoryQueue) Enqueue(jobID string) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closed {
		return errors.New("queue closed")
	}
	q.jobs = append(q.jobs, jobID)
	q.signalLocked()
	return nil
}

func (q *InMemoryQueue) Dequeue(ctx context.Context) (string, error) {
	for {
		q.mu.Lock()
		if q.closed {
			q.mu.Unlock()
			return "", errors.New("queue closed")
		}
		if !q.paused && len(q.jobs) > 0 {
			jobID := q.jobs[0]
			q.jobs = q.jobs[1:]
			q.mu.Unlock()
			return jobID, nil
		}
		notify := q.notify
		q.mu.Unlock()

		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-notify:
		}
	}
}

func (q *InMemoryQueue) Pause() error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closed {
		return errors.New("queue closed")
	}
	q.paused = true
	q.signalLocked()
	return nil
}

func (q *InMemoryQueue) Resume() error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closed {
		return errors.New("queue closed")
	}
	q.paused = false
	q.signalLocked()
	return nil
}

func (q *InMemoryQueue) Reorder(jobIDs []string) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closed {
		return errors.New("queue closed")
	}
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
	q.signalLocked()
	return nil
}

func (q *InMemoryQueue) Close() error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closed {
		return nil
	}
	q.closed = true
	q.signalLocked()
	return nil
}

func (q *InMemoryQueue) signalLocked() {
	close(q.notify)
	q.notify = make(chan struct{})
}
