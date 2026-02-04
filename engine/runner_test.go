package engine

import (
	"context"
	"testing"
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

func TestMustRunnerFromContextPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatalf("expected panic")
		}
	}()
	_ = MustRunnerFromContext(context.Background())
}
