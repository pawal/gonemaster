package transport

import (
	"context"
	"testing"
	"time"
)

func TestLimiterBlocksUntilRelease(t *testing.T) {
	limiter := NewLimiter(1)
	ctx := WithLimiter(context.Background(), limiter)

	if err := acquireQuerySlot(ctx); err != nil {
		t.Fatalf("acquire: %v", err)
	}

	done := make(chan struct{})
	go func() {
		if err := acquireQuerySlot(ctx); err == nil {
			releaseQuerySlot(ctx)
		}
		close(done)
	}()

	select {
	case <-done:
		t.Fatalf("expected acquire to block before release")
	case <-time.After(50 * time.Millisecond):
	}

	releaseQuerySlot(ctx)

	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		t.Fatalf("expected acquire to succeed after release")
	}
}

func TestLimiterHonorsContext(t *testing.T) {
	limiter := NewLimiter(1)
	ctx := WithLimiter(context.Background(), limiter)

	if err := acquireQuerySlot(ctx); err != nil {
		t.Fatalf("acquire: %v", err)
	}
	defer releaseQuerySlot(ctx)

	ctx, cancel := context.WithTimeout(ctx, 20*time.Millisecond)
	defer cancel()

	if err := acquireQuerySlot(ctx); err == nil {
		t.Fatalf("expected context deadline error")
	}
}

func TestLimiterIndependentLimits(t *testing.T) {
	first := NewLimiter(1)
	second := NewLimiter(2)

	ctx1 := WithLimiter(context.Background(), first)
	ctx2 := WithLimiter(context.Background(), second)

	if err := acquireQuerySlot(ctx1); err != nil {
		t.Fatalf("acquire ctx1: %v", err)
	}
	defer releaseQuerySlot(ctx1)

	if cap(second.tokens) != 2 || len(second.tokens) != 2 {
		t.Fatalf("expected second limiter capacity 2, got cap=%d len=%d", cap(second.tokens), len(second.tokens))
	}

	if err := acquireQuerySlot(ctx2); err != nil {
		t.Fatalf("acquire ctx2: %v", err)
	}
	releaseQuerySlot(ctx2)
}
