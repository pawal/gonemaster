package transport

import (
	"context"
	"testing"
	"testing/synctest"
	"time"
)

func TestLimiterBlocksUntilRelease(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
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

		// The second acquire is parked on the single token, not merely slow.
		synctest.Wait()
		select {
		case <-done:
			t.Fatalf("expected acquire to block before release")
		default:
		}

		releaseQuerySlot(ctx)
		<-done
	})
}

func TestLimiterHonorsContext(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
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
	})
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
