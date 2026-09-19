package engine

import (
	"context"
	"testing"

	"codeberg.org/pawal/gonemaster/engine/internal/testhelpers"
	"codeberg.org/pawal/gonemaster/engine/logger"
	"codeberg.org/pawal/gonemaster/engine/nameserver"
	"codeberg.org/pawal/gonemaster/engine/profile"
	"codeberg.org/pawal/gonemaster/engine/transport"
)

// runnerOpt overrides a field of the runner newTestRunner builds.
type runnerOpt func(*Runner)

// withProfile uses the caller's profile instead of a fresh default one.
func withProfile(p *profile.Profile) runnerOpt {
	return func(r *Runner) { r.Profile = p }
}

// withRunLimits adds the limiter and per-run nameserver cache a real run needs;
// the error paths that fail before any query do not.
func withRunLimits(parallel int) runnerOpt {
	return func(r *Runner) {
		r.Limiter = transport.NewLimiter(parallel)
		r.NameserverCache = nameserver.NewCacheStore()
	}
}

// withCache uses the caller's nameserver cache.
func withCache(cache *nameserver.CacheStore) runnerOpt {
	return func(r *Runner) { r.NameserverCache = cache }
}

// withTestcases narrows the profile to the named testcases. RunWithRunner
// takes the caller's profile as given, so only the module is selected without
// this and every testcase in it queries the network.
func withTestcases(names ...string) runnerOpt {
	return func(r *Runner) { _ = r.Profile.Set("test_cases", toAnySlice(names)) }
}

// newTestRunner returns a runner with a default profile and a fresh logger.
// Callers read back runner.Profile and runner.Logger to set up and assert.
func newTestRunner(t *testing.T, opts ...runnerOpt) *Runner {
	t.Helper()
	runner := &Runner{Profile: testhelpers.DefaultProfile(t), Logger: logger.New()}
	for _, opt := range opts {
		opt(runner)
	}
	return runner
}

func TestRunnerContextRoundTrip(t *testing.T) {
	runner := &Runner{}
	ctx := WithRunner(context.Background(), runner)
	if got := RunnerFromContext(ctx); got != runner {
		t.Fatalf("expected runner %p, got %p", runner, got)
	}
}

func TestRunnerFromContextMissing(t *testing.T) {
	if got := RunnerFromContext(context.Background()); got != nil {
		t.Fatalf("expected nil runner, got %v", got)
	}
}

func TestRunnerFromContextOrDefaultUsesContextValues(t *testing.T) {
	log := logger.New()
	prof := profile.Effective()
	limiter := transport.NewLimiter(1)
	cache := nameserver.NewCacheStore()
	ctx := logger.WithContext(context.Background(), log)
	ctx = profile.WithContext(ctx, prof)
	ctx = transport.WithLimiter(ctx, limiter)
	ctx = nameserver.WithCache(ctx, cache)

	got := RunnerFromContextOrDefault(ctx)
	if got == nil {
		t.Fatalf("expected runner")
	}
	if got.Logger != log {
		t.Fatalf("expected logger from context")
	}
	if got.Profile != prof {
		t.Fatalf("expected profile from context")
	}
	if got.Limiter != limiter {
		t.Fatalf("expected limiter from context")
	}
	if got.NameserverCache != cache {
		t.Fatalf("expected nameserver cache from context")
	}
}

func TestRunnerFromContextOrDefaultCreatesLogger(t *testing.T) {
	ctx := context.Background()
	got := RunnerFromContextOrDefault(ctx)
	if got == nil || got.Logger == nil {
		t.Fatalf("expected default logger")
	}
	if got.Profile == nil {
		t.Fatalf("expected default profile")
	}
	if got.NameserverCache == nil {
		t.Fatalf("expected default nameserver cache")
	}
}

func TestMustRunnerFromContextPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatalf("expected panic")
		}
	}()
	_ = MustRunnerFromContext(context.Background())
}
