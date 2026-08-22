package server

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
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

	synctest.Test(t, func(t *testing.T) {
		testManyBlockedDequeuers(t)
	})
}

func testManyBlockedDequeuers(t *testing.T) {
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
	for range workers {
		wg.Go(func() {
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
		})
	}

	// Returns once every worker is blocked in Dequeue.
	synctest.Wait()

	expected := make(map[string]bool, jobs)
	for i := range jobs {
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

	synctest.Test(t, func(t *testing.T) {
		testBurstEnqueueDequeue(t)
	})
}

func testBurstEnqueueDequeue(t *testing.T) {
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

	for range consumers {
		consumerWG.Go(func() {
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
		})
	}

	for p := range producers {
		producerID := p
		producerWG.Go(func() {
			for i := range perProducerJobs {
				jobID := fmt.Sprintf("p%02d-job-%03d", producerID, i)
				if err := q.Enqueue(jobID, PriorityNormal); err != nil {
					select {
					case errs <- err:
					default:
					}
					return
				}
			}
		})
	}

	producerWG.Wait()

	// Every consumer back in Dequeue means the queue has drained.
	synctest.Wait()

	select {
	case err := <-errs:
		t.Fatalf("concurrent queue error: %v", err)
	default:
	}
	if int(consumed.Load()) != totalJobs {
		t.Fatalf("consumed=%d want=%d", consumed.Load(), totalJobs)
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

	synctest.Test(t, func(t *testing.T) {
		testPauseResumeUnderLoad(t)
	})
}

func testPauseResumeUnderLoad(t *testing.T) {
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

	for range workers {
		wg.Go(func() {
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
		})
	}

	expected := make(map[string]bool, jobs)
	for i := range jobs {
		jobID := fmt.Sprintf("job-%03d", i)
		expected[jobID] = true
		if err := q.Enqueue(jobID, PriorityNormal); err != nil {
			t.Fatalf("enqueue %s: %v", jobID, err)
		}
	}

	// Every worker back to blocking with the queue full means the pause held.
	synctest.Wait()
	select {
	case jobID := <-results:
		t.Fatalf("received job %q while queue paused", jobID)
	case err := <-errs:
		t.Fatalf("worker error while paused: %v", err)
	default:
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

func TestReorderRejectsCrossTier(t *testing.T) {
	q := NewInMemoryQueue()
	_ = q.Enqueue("n1", PriorityNormal)
	_ = q.Enqueue("b1", PriorityBatch)

	// Putting a batch job before a normal job must be rejected.
	if err := q.Reorder([]string{"b1", "n1"}); err == nil {
		t.Fatal("expected error when placing batch job before normal job")
	}
}

func TestReorderWithinTierPreservesOtherTier(t *testing.T) {
	q := NewInMemoryQueue()
	_ = q.Enqueue("n1", PriorityNormal)
	_ = q.Enqueue("n2", PriorityNormal)
	_ = q.Enqueue("b1", PriorityBatch)
	_ = q.Enqueue("b2", PriorityBatch)

	// Reorder within each tier: normals reversed, batches reversed.
	if err := q.Reorder([]string{"n2", "n1", "b2", "b1"}); err != nil {
		t.Fatalf("reorder: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	for _, want := range []string{"n2", "n1", "b2", "b1"} {
		id, err := q.Dequeue(ctx)
		if err != nil {
			t.Fatalf("dequeue: %v", err)
		}
		if id != want {
			t.Fatalf("expected %s, got %s", want, id)
		}
	}
}

// TestQueueNormalAlwaysBeforeBatch verifies that all normal-priority jobs are
// dequeued before any batch-priority job, regardless of enqueue order.
func TestQueueNormalAlwaysBeforeBatch(t *testing.T) {
	q := NewInMemoryQueue()
	defer func() { _ = q.Close() }()

	// Interleave batch and normal enqueues.
	_ = q.Enqueue("b0", PriorityBatch)
	_ = q.Enqueue("n0", PriorityNormal)
	_ = q.Enqueue("b1", PriorityBatch)
	_ = q.Enqueue("n1", PriorityNormal)
	_ = q.Enqueue("b2", PriorityBatch)
	_ = q.Enqueue("n2", PriorityNormal)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	// All three normal jobs must come out first in FIFO order.
	for _, want := range []string{"n0", "n1", "n2"} {
		id, err := q.Dequeue(ctx)
		if err != nil {
			t.Fatalf("dequeue: %v", err)
		}
		if id != want {
			t.Fatalf("expected %s (normal tier), got %s", want, id)
		}
	}

	// Then all three batch jobs in FIFO order.
	for _, want := range []string{"b0", "b1", "b2"} {
		id, err := q.Dequeue(ctx)
		if err != nil {
			t.Fatalf("dequeue: %v", err)
		}
		if id != want {
			t.Fatalf("expected %s (batch tier), got %s", want, id)
		}
	}
}

func TestInMemoryQueueNormalBeforesBatch(t *testing.T) {
	q := NewInMemoryQueue()
	defer func() { _ = q.Close() }()

	// Enqueue several batch jobs first, then a normal job.
	for i := range 3 {
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
	for i := range 3 {
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
