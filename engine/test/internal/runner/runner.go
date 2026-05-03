package runner

import (
	"context"

	"codeberg.org/pawal/gonemaster/engine/internal/parallel"
	"codeberg.org/pawal/gonemaster/engine/logger"
)

// Task is a unit of work that records log entries via the provided logger.
type Task func(context.Context, *logger.Logger) error

// Options controls execution behavior for Run.
type Options struct {
	// Parallel bounds the number of concurrent task workers.
	Parallel int
	// CancelOnError stops scheduling more work after the first error.
	CancelOnError bool
}

// Run executes tasks with bounded concurrency and merges log entries deterministically.
//
// When CancelOnError is true, logs for tasks after the first error are dropped to
// match sequential behavior.
func Run(ctx context.Context, tasks []Task, opts Options) ([]*logger.Entry, error) {
	if len(tasks) == 0 {
		return nil, nil
	}
	limit := max(opts.Parallel, 1)
	parent := logger.FromContext(ctx)

	wrapped := make([]parallel.Task[[]*logger.Entry], len(tasks))
	for i, task := range tasks {
		wrapped[i] = func(ctx context.Context) ([]*logger.Entry, error) {
			buf := logger.New()
			if parent != nil {
				buf.CopyConfigFrom(parent)
				buf.CopyStartTimeFrom(parent)
			}
			if err := task(ctx, buf); err != nil {
				return buf.Entries(), err
			}
			return buf.Entries(), nil
		}
	}

	results := parallel.RunOrdered(ctx, wrapped, parallel.Options{Limit: limit, CancelOnError: opts.CancelOnError})

	var out []*logger.Entry
	var firstErr error
	for _, res := range results {
		if res.Err != nil && firstErr == nil {
			firstErr = res.Err
		}
		out = append(out, res.Value...)
		if firstErr != nil && opts.CancelOnError {
			break
		}
	}
	return out, firstErr
}
