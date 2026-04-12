package engine

import (
	"context"
	"testing"

	"codeberg.org/pawal/gonemaster/engine/logger"
	"codeberg.org/pawal/gonemaster/engine/nameserver"
	"codeberg.org/pawal/gonemaster/engine/profile"
	"codeberg.org/pawal/gonemaster/engine/transport"
)

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
