package server

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestInMemoryQueueEnqueueDequeue(t *testing.T) {
	q := NewInMemoryQueue()
	if err := q.Enqueue("job1", PriorityNormal); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	jobID, err := q.Dequeue(ctx)
	if err != nil {
		t.Fatalf("dequeue: %v", err)
	}
	if jobID != "job1" {
		t.Fatalf("expected job1, got %s", jobID)
	}
}

func TestInMemoryQueuePauseResume(t *testing.T) {
	q := NewInMemoryQueue()
	if err := q.Pause(); err != nil {
		t.Fatalf("pause: %v", err)
	}
	if err := q.Enqueue("job1", PriorityNormal); err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	_, err := q.Dequeue(ctx)
	if err == nil {
		t.Fatalf("expected dequeue to block while paused")
	}

	if err := q.Resume(); err != nil {
		t.Fatalf("resume: %v", err)
	}

	ctx2, cancel2 := context.WithTimeout(context.Background(), time.Second)
	defer cancel2()
	jobID, err := q.Dequeue(ctx2)
	if err != nil {
		t.Fatalf("dequeue after resume: %v", err)
	}
	if jobID != "job1" {
		t.Fatalf("expected job1, got %s", jobID)
	}
}

func TestInMemoryQueueReorder(t *testing.T) {
	q := NewInMemoryQueue()
	_ = q.Enqueue("job1", PriorityNormal)
	_ = q.Enqueue("job2", PriorityNormal)

	if err := q.Reorder([]string{"job2", "job1"}); err != nil {
		t.Fatalf("reorder: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	jobID, err := q.Dequeue(ctx)
	if err != nil {
		t.Fatalf("dequeue: %v", err)
	}
	if jobID != "job2" {
		t.Fatalf("expected job2, got %s", jobID)
	}
}

func TestInMemoryQueueRemove(t *testing.T) {
	q := NewInMemoryQueue()
	_ = q.Enqueue("job1", PriorityNormal)
	_ = q.Enqueue("job2", PriorityNormal)

	if err := q.Remove("job1"); err != nil {
		t.Fatalf("remove: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	jobID, err := q.Dequeue(ctx)
	if err != nil {
		t.Fatalf("dequeue: %v", err)
	}
	if jobID != "job2" {
		t.Fatalf("expected job2, got %s", jobID)
	}
	if err := q.Remove("missing"); err == nil {
		t.Fatalf("expected remove to fail for missing job")
	}
}

func TestInMemoryQueueClose(t *testing.T) {
	q := NewInMemoryQueue()
	if err := q.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if err := q.Enqueue("job1", PriorityNormal); err == nil {
		t.Fatalf("expected enqueue to fail after close")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, err := q.Dequeue(ctx); err == nil {
		t.Fatalf("expected dequeue to fail after close")
	}
}

func TestInMemoryQueueManyBlockedDequeuers(t *testing.T) {
	t.Parallel()

	const (
		workers = 32
		jobs    = 128
	)

	q := NewInMemoryQueue()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	results := make(chan string, jobs)
	errs := make(chan error, workers)

	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				jobID, err := q.Dequeue(ctx)
				if err != nil {
					if ctx.Err() != nil || isQueueClosedError(err) {
						return
					}
					select {
					case errs <- err:
					default:
					}
					return
				}
				select {
				case results <- jobID:
				case <-ctx.Done():
					return
				}
			}
		}()
	}

	// Give workers a short moment to block in Dequeue.
	time.Sleep(50 * time.Millisecond)

	expected := make(map[string]bool, jobs)
	for i := 0; i < jobs; i++ {
		jobID := fmt.Sprintf("job-%03d", i)
		expected[jobID] = true
		if err := q.Enqueue(jobID, PriorityNormal); err != nil {
			t.Fatalf("enqueue %s: %v", jobID, err)
		}
	}

	received := make(map[string]bool, jobs)
	for len(received) < jobs {
		select {
		case err := <-errs:
			t.Fatalf("worker error: %v", err)
		case jobID := <-results:
			if !expected[jobID] {
				t.Fatalf("unexpected job id %q", jobID)
			}
			if received[jobID] {
				t.Fatalf("duplicate job id %q", jobID)
			}
			received[jobID] = true
		case <-ctx.Done():
			t.Fatalf("timed out waiting for dequeued jobs: got=%d want=%d", len(received), jobs)
		}
	}

	cancel()
	wg.Wait()
}

func TestInMemoryQueueBurstEnqueueDequeueConcurrent(t *testing.T) {
	t.Parallel()

	const (
		producers       = 8
		perProducerJobs = 150
		consumers       = 16
	)
	totalJobs := producers * perProducerJobs

	q := NewInMemoryQueue()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var consumed atomic.Int64
	var producerWG sync.WaitGroup
	var consumerWG sync.WaitGroup
	var seenMu sync.Mutex
	seen := make(map[string]bool, totalJobs)
	errs := make(chan error, 1)

	for i := 0; i < consumers; i++ {
		consumerWG.Add(1)
		go func() {
			defer consumerWG.Done()
			for {
				if int(consumed.Load()) >= totalJobs {
					return
				}
				jobID, err := q.Dequeue(ctx)
				if err != nil {
					if ctx.Err() != nil || isQueueClosedError(err) {
						return
					}
					select {
					case errs <- err:
					default:
					}
					return
				}
				seenMu.Lock()
				if seen[jobID] {
					seenMu.Unlock()
					select {
					case errs <- fmt.Errorf("duplicate job id %q", jobID):
					default:
					}
					return
				}
				seen[jobID] = true
				seenMu.Unlock()
				consumed.Add(1)
			}
		}()
	}

	for p := 0; p < producers; p++ {
		producerID := p
		producerWG.Add(1)
		go func() {
			defer producerWG.Done()
			for i := 0; i < perProducerJobs; i++ {
				jobID := fmt.Sprintf("p%02d-job-%03d", producerID, i)
				if err := q.Enqueue(jobID, PriorityNormal); err != nil {
					select {
					case errs <- err:
					default:
					}
					return
				}
			}
		}()
	}

	producerWG.Wait()

	for int(consumed.Load()) < totalJobs {
		select {
		case err := <-errs:
			t.Fatalf("concurrent queue error: %v", err)
		case <-ctx.Done():
			t.Fatalf("timed out waiting for jobs to drain: got=%d want=%d", consumed.Load(), totalJobs)
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}

	cancel()
	consumerWG.Wait()

	seenMu.Lock()
	defer seenMu.Unlock()
	if len(seen) != totalJobs {
		t.Fatalf("unexpected unique dequeued jobs: got=%d want=%d", len(seen), totalJobs)
	}
}

func TestInMemoryQueuePauseResumeUnderLoad(t *testing.T) {
	t.Parallel()

	const (
		workers = 24
		jobs    = 240
	)

	q := NewInMemoryQueue()
	if err := q.Pause(); err != nil {
		t.Fatalf("pause: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	results := make(chan string, jobs)
	errs := make(chan error, workers)
	var wg sync.WaitGroup

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				jobID, err := q.Dequeue(ctx)
				if err != nil {
					if ctx.Err() != nil || isQueueClosedError(err) {
						return
					}
					select {
					case errs <- err:
					default:
					}
					return
				}
				select {
				case results <- jobID:
				case <-ctx.Done():
					return
				}
			}
		}()
	}

	expected := make(map[string]bool, jobs)
	for i := 0; i < jobs; i++ {
		jobID := fmt.Sprintf("job-%03d", i)
		expected[jobID] = true
		if err := q.Enqueue(jobID, PriorityNormal); err != nil {
			t.Fatalf("enqueue %s: %v", jobID, err)
		}
	}

	// While paused, no worker should receive jobs.
	select {
	case jobID := <-results:
		t.Fatalf("received job %q while queue paused", jobID)
	case err := <-errs:
		t.Fatalf("worker error while paused: %v", err)
	case <-time.After(200 * time.Millisecond):
	}

	if err := q.Resume(); err != nil {
		t.Fatalf("resume: %v", err)
	}

	received := make(map[string]bool, jobs)
	for len(received) < jobs {
		select {
		case err := <-errs:
			t.Fatalf("worker error: %v", err)
		case jobID := <-results:
			if !expected[jobID] {
				t.Fatalf("unexpected job id %q", jobID)
			}
			if received[jobID] {
				t.Fatalf("duplicate job id %q", jobID)
			}
			received[jobID] = true
		case <-ctx.Done():
			t.Fatalf("timed out waiting for resumed queue drain: got=%d want=%d", len(received), jobs)
		}
	}

	cancel()
	wg.Wait()
}

func isQueueClosedError(err error) bool {
	return err != nil && strings.Contains(err.Error(), "queue closed")
}

func TestInMemoryQueueNormalBeforesBatch(t *testing.T) {
	q := NewInMemoryQueue()
	defer func() { _ = q.Close() }()

	// Enqueue several batch jobs first, then a normal job.
	for i := 0; i < 3; i++ {
		if err := q.Enqueue(fmt.Sprintf("batch-%d", i), PriorityBatch); err != nil {
			t.Fatalf("enqueue batch: %v", err)
		}
	}
	if err := q.Enqueue("normal-0", PriorityNormal); err != nil {
		t.Fatalf("enqueue normal: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	// The normal job must come out first despite being enqueued last.
	first, err := q.Dequeue(ctx)
	if err != nil {
		t.Fatalf("dequeue: %v", err)
	}
	if first != "normal-0" {
		t.Fatalf("expected normal-0 first, got %s", first)
	}

	// The remaining three dequeues should all be batch jobs in FIFO order.
	for i := 0; i < 3; i++ {
		id, err := q.Dequeue(ctx)
		if err != nil {
			t.Fatalf("dequeue batch %d: %v", i, err)
		}
		want := fmt.Sprintf("batch-%d", i)
		if id != want {
			t.Fatalf("expected %s, got %s", want, id)
		}
	}
}

func TestQueueInterfaceEnqueueAcceptsPriority(t *testing.T) {
	// Verify that the Queue interface accepts a JobPriority argument and that
	// both priority values are accepted without error.
	var q Queue = NewInMemoryQueue()
	defer func() { _ = q.Close() }()

	if err := q.Enqueue("n1", PriorityNormal); err != nil {
		t.Fatalf("Enqueue PriorityNormal: %v", err)
	}
	if err := q.Enqueue("b1", PriorityBatch); err != nil {
		t.Fatalf("Enqueue PriorityBatch: %v", err)
	}
}
