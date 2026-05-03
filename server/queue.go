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
	normal  []string // PriorityNormal jobs
	batch   []string // PriorityBatch jobs
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

// Enqueue adds a job id to the tail of the appropriate priority slice.
func (q *InMemoryQueue) Enqueue(jobID string, priority JobPriority) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closed {
		return errors.New("queue closed")
	}
	if priority == PriorityBatch {
		q.batch = append(q.batch, jobID)
	} else {
		q.normal = append(q.normal, jobID)
	}
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
		if !q.paused {
			if len(q.normal) > 0 {
				jobID := q.normal[0]
				q.normal = q.normal[1:]
				q.mu.Unlock()
				return jobID, nil
			}
			if len(q.batch) > 0 {
				jobID := q.batch[0]
				q.batch = q.batch[1:]
				q.mu.Unlock()
				return jobID, nil
			}
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
	for i, id := range q.normal {
		if id == jobID {
			q.normal = append(q.normal[:i], q.normal[i+1:]...)
			return nil
		}
	}
	for i, id := range q.batch {
		if id == jobID {
			q.batch = append(q.batch[:i], q.batch[i+1:]...)
			return nil
		}
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
	total := len(q.normal) + len(q.batch)
	if len(jobIDs) != total {
		return errors.New("job id list does not match queue length")
	}
	tierOf := make(map[string]JobPriority, total)
	for _, id := range q.normal {
		tierOf[id] = PriorityNormal
	}
	for _, id := range q.batch {
		tierOf[id] = PriorityBatch
	}
	seenBatch := false
	for _, id := range jobIDs {
		tier, ok := tierOf[id]
		if !ok {
			return errors.New("job id not in queue")
		}
		if tier == PriorityBatch {
			seenBatch = true
		} else if seenBatch {
			return errors.New("cannot place a normal job after a batch job")
		}
	}
	var newNormal, newBatch []string
	for _, id := range jobIDs {
		if tierOf[id] == PriorityBatch {
			newBatch = append(newBatch, id)
		} else {
			newNormal = append(newNormal, id)
		}
	}
	q.normal = newNormal
	q.batch = newBatch
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
	total := len(q.normal) + len(q.batch)
	if q.waiters < 1 || total < 1 {
		return
	}
	remaining := min(q.waiters, total)
	for range remaining {
		select {
		case q.notify <- struct{}{}:
		default:
			return
		}
	}
}
