package server

import (
	"context"
	"testing"
	"time"
)

func TestInMemoryQueueEnqueueDequeue(t *testing.T) {
	q := NewInMemoryQueue()
	if err := q.Enqueue("job1"); err != nil {
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
	if err := q.Enqueue("job1"); err != nil {
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
	_ = q.Enqueue("job1")
	_ = q.Enqueue("job2")

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

func TestInMemoryQueueClose(t *testing.T) {
	q := NewInMemoryQueue()
	if err := q.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if err := q.Enqueue("job1"); err == nil {
		t.Fatalf("expected enqueue to fail after close")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, err := q.Dequeue(ctx); err == nil {
		t.Fatalf("expected dequeue to fail after close")
	}
}
