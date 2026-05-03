package parallel

import (
	"context"
	"sync"
)

// Task defines a unit of work that should honor the provided context.
type Task[T any] func(context.Context) (T, error)

// Result captures the outcome of a task.
type Result[T any] struct {
	// Value is the task result value.
	Value T
	// Err is the task error, if any.
	Err error
}

// Options controls task execution behavior.
type Options struct {
	// Limit bounds the number of concurrent workers. Values <= 1 run sequentially.
	Limit int
	// CancelOnError cancels remaining work after the first error.
	CancelOnError bool
}

// RunOrdered executes tasks with a bounded worker pool and returns results in input order.
//
// When CancelOnError is true, the context is canceled after the first error and
// remaining unscheduled tasks are marked with the context error.
func RunOrdered[T any](ctx context.Context, tasks []Task[T], opts Options) []Result[T] {
	results := make([]Result[T], len(tasks))
	if len(tasks) == 0 {
		return results
	}

	limit := opts.Limit
	if limit <= 1 {
		return runSequential(ctx, tasks, opts)
	}
	if limit > len(tasks) {
		limit = len(tasks)
	}

	cancel := func() {}
	if opts.CancelOnError {
		ctx, cancel = context.WithCancel(ctx)
	}
	defer cancel()

	idxCh := make(chan int)
	var wg sync.WaitGroup
	wg.Add(limit)
	for i := 0; i < limit; i++ {
		go func() {
			defer wg.Done()
			for idx := range idxCh {
				// Once a task is dispatched it must run; the task is responsible
				// for honoring ctx cancellation. Skipping here would race with
				// the dispatcher and could drop tasks that already passed the
				// dispatcher's gate.
				value, err := tasks[idx](ctx)
				results[idx] = Result[T]{Value: value, Err: err}
				if err != nil && opts.CancelOnError {
					cancel()
				}
			}
		}()
	}

	for i := range tasks {
		if ctx.Err() != nil {
			for j := i; j < len(tasks); j++ {
				results[j].Err = ctx.Err()
			}
			break
		}
		idxCh <- i
	}
	close(idxCh)
	wg.Wait()

	return results
}

func runSequential[T any](ctx context.Context, tasks []Task[T], opts Options) []Result[T] {
	results := make([]Result[T], len(tasks))
	cancel := func() {}
	if opts.CancelOnError {
		ctx, cancel = context.WithCancel(ctx)
	}
	defer cancel()

	for i, task := range tasks {
		if ctx.Err() != nil {
			results[i].Err = ctx.Err()
			continue
		}
		value, err := task(ctx)
		results[i] = Result[T]{Value: value, Err: err}
		if err != nil && opts.CancelOnError {
			cancel()
			for j := i + 1; j < len(tasks); j++ {
				results[j].Err = ctx.Err()
			}
			break
		}
	}
	return results
}
