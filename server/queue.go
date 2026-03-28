package server

import (
	"context"
	"errors"
	"sync"
)

// Queue holds queued job ids.
type Queue interface {
	Enqueue(jobID string, priority JobPriority) error
	Dequeue(ctx context.Context) (string, error)
	Remove(jobID string) error
	Pause() error
	Resume() error
	Reorder(jobIDs []string) error
	Close() error
}

// InMemoryQueue is a simple in-memory queue.
type InMemoryQueue struct {
	mu      sync.Mutex
	jobs    []string
	paused  bool
	closed  bool
	waiters int
	notify  chan struct{}
	closeCh chan struct{}
}

// NewInMemoryQueue creates an empty in-memory queue.
func NewInMemoryQueue() *InMemoryQueue {
	return &InMemoryQueue{
		notify:  make(chan struct{}, 1024),
		closeCh: make(chan struct{}),
	}
}

// Enqueue adds a job id to the tail of the queue.
func (q *InMemoryQueue) Enqueue(jobID string, priority JobPriority) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closed {
		return errors.New("queue closed")
	}
	q.jobs = append(q.jobs, jobID)
	if !q.paused {
		q.signalOneLocked()
	}
	return nil
}

// Dequeue blocks until a job id is available or ctx is canceled.
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
		q.waiters++
		notify := q.notify
		closeCh := q.closeCh
		q.mu.Unlock()

		select {
		case <-ctx.Done():
			q.mu.Lock()
			if q.waiters > 0 {
				q.waiters--
			}
			q.mu.Unlock()
			return "", ctx.Err()
		case <-closeCh:
			q.mu.Lock()
			if q.waiters > 0 {
				q.waiters--
			}
			q.mu.Unlock()
		case <-notify:
			q.mu.Lock()
			if q.waiters > 0 {
				q.waiters--
			}
			q.mu.Unlock()
		}
	}
}

// Remove deletes a queued job id.
func (q *InMemoryQueue) Remove(jobID string) error {
	if jobID == "" {
		return errors.New("job id required")
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closed {
		return errors.New("queue closed")
	}
	for i, id := range q.jobs {
		if id != jobID {
			continue
		}
		q.jobs = append(q.jobs[:i], q.jobs[i+1:]...)
		return nil
	}
	return errors.New("job id not in queue")
}

// Pause stops dequeuing while keeping queued jobs intact.
func (q *InMemoryQueue) Pause() error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closed {
		return errors.New("queue closed")
	}
	q.paused = true
	return nil
}

// Resume re-enables dequeuing after Pause.
func (q *InMemoryQueue) Resume() error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closed {
		return errors.New("queue closed")
	}
	q.paused = false
	q.signalAvailableLocked()
	return nil
}

// Reorder replaces queued job order with the provided ids.
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
	if !q.paused {
		q.signalAvailableLocked()
	}
	return nil
}

// Close permanently closes the queue and wakes blocked dequeuers.
func (q *InMemoryQueue) Close() error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closed {
		return nil
	}
	q.closed = true
	close(q.closeCh)
	return nil
}

func (q *InMemoryQueue) signalOneLocked() {
	if q.waiters < 1 {
		return
	}
	select {
	case q.notify <- struct{}{}:
	default:
	}
}

func (q *InMemoryQueue) signalAvailableLocked() {
	if q.waiters < 1 || len(q.jobs) < 1 {
		return
	}
	remaining := len(q.jobs)
	if q.waiters < remaining {
		remaining = q.waiters
	}
	for i := 0; i < remaining; i++ {
		select {
		case q.notify <- struct{}{}:
		default:
			return
		}
	}
}
