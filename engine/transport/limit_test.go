package transport

import (
	"context"
	"testing"
	"time"
)

func TestGlobalQueryLimiterBlocksUntilRelease(t *testing.T) {
	resetGlobalQueryLimit()
	SetGlobalQueryLimit(1)
	t.Cleanup(resetGlobalQueryLimit)

	if err := acquireQuerySlot(context.Background()); err != nil {
		t.Fatalf("acquire: %v", err)
	}

	done := make(chan struct{})
	go func() {
		if err := acquireQuerySlot(context.Background()); err == nil {
			releaseQuerySlot()
		}
		close(done)
	}()

	select {
	case <-done:
		t.Fatalf("expected acquire to block before release")
	case <-time.After(50 * time.Millisecond):
	}

	releaseQuerySlot()

	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		t.Fatalf("expected acquire to succeed after release")
	}
}

func TestGlobalQueryLimiterHonorsContext(t *testing.T) {
	resetGlobalQueryLimit()
	SetGlobalQueryLimit(1)
	t.Cleanup(resetGlobalQueryLimit)

	if err := acquireQuerySlot(context.Background()); err != nil {
		t.Fatalf("acquire: %v", err)
	}
	defer releaseQuerySlot()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	if err := acquireQuerySlot(ctx); err == nil {
		t.Fatalf("expected context deadline error")
	}
}
