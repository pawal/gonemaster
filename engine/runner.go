package engine

import (
	"context"
	"time"

	"codeberg.org/pawal/gonemaster/engine/logger"
	"codeberg.org/pawal/gonemaster/engine/nameserver"
	"codeberg.org/pawal/gonemaster/engine/profile"
	"codeberg.org/pawal/gonemaster/engine/transport"
)

// Runner holds per-run state. Minimal contract used by modules/tests:
// - Profile: effective profile for the run
// - Logger: per-run logger instance
// - Limiter: effective query limiter for DNS calls
// - StartedAt: run start time
type Runner struct {
	// Profile is the effective profile for the run.
	Profile *profile.Profile
	// Logger collects all per-run log entries.
	Logger *logger.Logger
	// Limiter bounds concurrent DNS query work for the run.
	Limiter *transport.Limiter
	// StartedAt is the run start timestamp.
	StartedAt time.Time
	// NameserverCache holds per-run nameserver caches.
	NameserverCache *nameserver.CacheStore
	// AutoIPv6Disabled records whether IPv6 was disabled by the auto-detect heuristic.
	AutoIPv6Disabled bool
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

// RunnerFromContextOrDefault returns the runner stored in ctx, or a default runner
// built from other per-run context values.
func RunnerFromContextOrDefault(ctx context.Context) *Runner {
	if runner := RunnerFromContext(ctx); runner != nil {
		return runner
	}
	prof := profile.FromContext(ctx)
	log := logger.FromContext(ctx)
	if log == nil {
		log = logger.New()
		log.SetProfile(prof)
	}
	limiter := transport.LimiterFromContext(ctx)
	cache := nameserver.CacheFromContext(ctx)
	if cache == nil {
		cache = nameserver.NewCacheStore()
	}
	return &Runner{
		Profile:         prof,
		Logger:          log,
		Limiter:         limiter,
		NameserverCache: cache,
		StartedAt:       time.Now(),
	}
}

// MustRunnerFromContext returns the runner stored in ctx or panics.
func MustRunnerFromContext(ctx context.Context) *Runner {
	runner := RunnerFromContext(ctx)
	if runner == nil {
		panic("engine: missing Runner in context")
	}
	return runner
}
