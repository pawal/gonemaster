package engine

import (
	"context"
	"testing"

	"codeberg.org/pawal/gonemaster/engine/logger"
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

func TestRunnerFromContextNil(t *testing.T) {
	if got := RunnerFromContext(nil); got != nil {
		t.Fatalf("expected nil runner, got %v", got)
	}
}

func TestRunnerFromContextOrDefaultUsesContextValues(t *testing.T) {
	log := logger.New()
	prof := profile.Effective()
	limiter := transport.NewLimiter(1)
	ctx := logger.WithContext(context.Background(), log)
	ctx = profile.WithContext(ctx, prof)
	ctx = transport.WithLimiter(ctx, limiter)

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
}

func TestMustRunnerFromContextPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatalf("expected panic")
		}
	}()
	_ = MustRunnerFromContext(context.Background())
}
