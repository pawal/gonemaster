package engine

import (
	"context"
	"time"

	"codeberg.org/pawal/gonemaster/engine/logger"
	"codeberg.org/pawal/gonemaster/engine/profile"
	"codeberg.org/pawal/gonemaster/engine/transport"
)

// Runner holds per-run state. Minimal contract used by modules/tests:
// - Profile: effective profile for the run
// - Logger: per-run logger instance
// - Limiter: effective query limiter for DNS calls
// - StartedAt: run start time
type Runner struct {
	Profile   *profile.Profile
	Logger    *logger.Logger
	Limiter   *transport.Limiter
	StartedAt time.Time
}

type runnerKey struct{}

// WithRunner stores r in ctx for downstream access.
func WithRunner(ctx context.Context, r *Runner) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, runnerKey{}, r)
}

// RunnerFromContext returns the runner stored in ctx, or nil.
func RunnerFromContext(ctx context.Context) *Runner {
	if ctx == nil {
		return nil
	}
	runner, _ := ctx.Value(runnerKey{}).(*Runner)
	return runner
}

// MustRunnerFromContext returns the runner stored in ctx or panics.
func MustRunnerFromContext(ctx context.Context) *Runner {
	runner := RunnerFromContext(ctx)
	if runner == nil {
		panic("engine: missing Runner in context")
	}
	return runner
}
