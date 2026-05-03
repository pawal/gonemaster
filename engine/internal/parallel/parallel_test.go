package parallel

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"
)

func TestRunOrderedSequentialPreservesOrder(t *testing.T) {
	ctx := context.Background()
	var mu sync.Mutex
	seen := []int{}

	const count = 5
	tasks := make([]Task[int], count)
	for i := range count {
		idx := i
		tasks[i] = func(ctx context.Context) (int, error) {
			mu.Lock()
			seen = append(seen, idx)
			mu.Unlock()
			return idx, nil
		}
	}

	results := RunOrdered(ctx, tasks, Options{Limit: 1})
	for i, res := range results {
		if res.Err != nil {
			t.Fatalf("unexpected error at %d: %v", i, res.Err)
		}
		if res.Value != i {
			t.Fatalf("expected result %d, got %d", i, res.Value)
		}
	}
	if !reflect.DeepEqual(seen, []int{0, 1, 2, 3, 4}) {
		t.Fatalf("expected sequential execution order, got %v", seen)
	}
}

func TestRunOrderedConcurrentPreservesResultOrder(t *testing.T) {
	ctx := context.Background()
	const count = 4

	release := make([]chan struct{}, count)
	for i := range release {
		release[i] = make(chan struct{})
	}

	tasks := make([]Task[int], count)
	for i := range count {
		idx := i
		tasks[i] = func(ctx context.Context) (int, error) {
			<-release[idx]
			return idx, nil
		}
	}

	resultsCh := make(chan []Result[int], 1)
	go func() {
		resultsCh <- RunOrdered(ctx, tasks, Options{Limit: 2})
	}()

	for i := count - 1; i >= 0; i-- {
		close(release[i])
	}

	results := <-resultsCh
	for i, res := range results {
		if res.Err != nil {
			t.Fatalf("unexpected error at %d: %v", i, res.Err)
		}
		if res.Value != i {
			t.Fatalf("expected result %d, got %d", i, res.Value)
		}
	}
}

func TestRunOrderedCancelOnErrorSkipsRemainingTasks(t *testing.T) {
	ctx := context.Background()
	var mu sync.Mutex
	seen := []int{}

	tasks := []Task[int]{
		func(ctx context.Context) (int, error) {
			mu.Lock()
			seen = append(seen, 0)
			mu.Unlock()
			return 0, nil
		},
		func(ctx context.Context) (int, error) {
			mu.Lock()
			seen = append(seen, 1)
			mu.Unlock()
			return 1, errors.New("boom")
		},
		func(ctx context.Context) (int, error) {
			mu.Lock()
			seen = append(seen, 2)
			mu.Unlock()
			return 2, nil
		},
	}

	results := RunOrdered(ctx, tasks, Options{Limit: 1, CancelOnError: true})
	if !reflect.DeepEqual(seen, []int{0, 1}) {
		t.Fatalf("expected tasks after error to be skipped, got %v", seen)
	}
	if results[1].Err == nil {
		t.Fatalf("expected error on task 1")
	}
	if !errors.Is(results[2].Err, context.Canceled) {
		t.Fatalf("expected task 2 error to be context cancellation, got %v", results[2].Err)
	}
}
