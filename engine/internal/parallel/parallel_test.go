package parallel

import (
	"bytes"
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

// panicTask panics in the task's own frame.
func panicTask(context.Context) (int, error) { panic("boom") }

// rethrown runs fn and returns the *Panic it raises.
func rethrown(t *testing.T, fn func()) (p *Panic) {
	t.Helper()
	defer func() {
		v := recover()
		got, ok := v.(*Panic)
		if !ok {
			t.Fatalf("recovered %#v, want *Panic", v)
		}
		p = got
	}()
	fn()
	return nil
}

func TestRunOrderedTaskPanicReachesCaller(t *testing.T) {
	ok := func(context.Context) (int, error) { return 1, nil }
	cases := []struct {
		name  string
		limit int
		tasks []Task[int]
	}{
		{"one of four", 2, []Task[int]{ok, panicTask, ok, ok}},
		{"one of four, a worker each", 4, []Task[int]{ok, panicTask, ok, ok}},
		{"every task", 2, []Task[int]{panicTask, panicTask, panicTask, panicTask, panicTask, panicTask}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := rethrown(t, func() { RunOrdered(t.Context(), tc.tasks, Options{Limit: tc.limit}) })
			if p.Value != "boom" {
				t.Fatalf("value = %v, want boom", p.Value)
			}
			if !bytes.Contains(p.Stack, []byte("panicTask")) {
				t.Fatalf("stack does not name panicTask:\n%s", p.Stack)
			}
		})
	}
}

func TestCatcherKeepsFirstPanic(t *testing.T) {
	var c Catcher
	for _, v := range []string{"first", "second"} {
		func() {
			defer c.Capture()
			panic(v)
		}()
	}
	if p := rethrown(t, c.Rethrow); p.Value != "first" {
		t.Fatalf("value = %v, want first", p.Value)
	}
}

func TestCatcherKeepsCarriedPanic(t *testing.T) {
	var c Catcher
	inner := &Panic{Value: "inner", Stack: []byte("inner stack")}
	func() {
		defer c.Capture()
		panic(inner)
	}()
	if p := rethrown(t, c.Rethrow); p != inner {
		t.Fatalf("rethrown %#v, want the carried panic", p)
	}
}

func TestCatcherRethrowWithoutPanic(t *testing.T) {
	var c Catcher
	c.Rethrow()
	if c.Caught() {
		t.Fatal("Caught = true, want false")
	}
}
