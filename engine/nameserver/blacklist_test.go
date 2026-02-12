package nameserver

import (
	"testing"
	"time"
)

func TestBlacklistTrackerTimeoutThreshold(t *testing.T) {
	tracker := &blacklistTracker{}
	now := time.Unix(100, 0)

	tracker.observeFailure(false, true, time.Second, now)
	if tracker.isBlocked(false, now) {
		t.Fatalf("expected first timeout failure not to trigger blacklist")
	}

	tracker.observeFailure(false, true, time.Second, now)
	if !tracker.isBlocked(false, now) {
		t.Fatalf("expected second timeout failure to trigger blacklist")
	}
}

func TestBlacklistTrackerSuccessResetsTimeoutBurst(t *testing.T) {
	tracker := &blacklistTracker{}
	now := time.Unix(200, 0)

	tracker.observeFailure(false, true, time.Second, now)
	tracker.observeSuccess(false)
	tracker.observeFailure(false, true, time.Second, now)

	if tracker.isBlocked(false, now) {
		t.Fatalf("expected timeout burst to reset after success")
	}
}

func TestBlacklistTrackerExpiry(t *testing.T) {
	tracker := &blacklistTracker{}
	now := time.Unix(300, 0)
	tracker.observeFailure(false, false, 400*time.Millisecond, now)

	if !tracker.isBlocked(false, now.Add(100*time.Millisecond)) {
		t.Fatalf("expected blacklist active before expiry")
	}
	if tracker.isBlocked(false, now.Add(500*time.Millisecond)) {
		t.Fatalf("expected blacklist inactive after expiry")
	}
}

func TestClampBlacklistTTLBounds(t *testing.T) {
	if got := clampBlacklistTTL(0); got != blacklistDefaultTTL {
		t.Fatalf("zero ttl clamp = %v, want %v", got, blacklistDefaultTTL)
	}
	if got := clampBlacklistTTL(10 * time.Millisecond); got != blacklistMinTTL {
		t.Fatalf("low ttl clamp = %v, want %v", got, blacklistMinTTL)
	}
	if got := clampBlacklistTTL(2 * time.Minute); got != blacklistMaxTTL {
		t.Fatalf("high ttl clamp = %v, want %v", got, blacklistMaxTTL)
	}
}

func TestBlacklistTrackerBackoffGrowth(t *testing.T) {
	tracker := &blacklistTracker{jitterFn: func() float64 { return 0.5 }}
	base := 500 * time.Millisecond
	now := time.Unix(400, 0)

	// First timeout burst triggers initial block window.
	tracker.observeFailure(false, true, base, now)
	tracker.observeFailure(false, true, base, now)
	state := tracker.stateForProtocol(false)
	if got := state.blockedUntil.Sub(now); got != 500*time.Millisecond {
		t.Fatalf("first block window = %v, want 500ms", got)
	}

	// Simulate a later burst after window expiry; backoff should grow.
	later := now.Add(2 * time.Second)
	tracker.observeFailure(false, true, base, later)
	tracker.observeFailure(false, true, base, later)
	if got := state.blockedUntil.Sub(later); got != time.Second {
		t.Fatalf("second block window = %v, want 1s", got)
	}
}

func TestBlacklistTTLJitterBounds(t *testing.T) {
	low := (&blacklistTracker{jitterFn: func() float64 { return 0 }}).blacklistTTL(time.Second, 0)
	if low != 800*time.Millisecond {
		t.Fatalf("low jitter ttl = %v, want 800ms", low)
	}

	high := (&blacklistTracker{jitterFn: func() float64 { return 1 }}).blacklistTTL(time.Second, 0)
	if high != 1200*time.Millisecond {
		t.Fatalf("high jitter ttl = %v, want 1200ms", high)
	}
}

func TestBlacklistTTLBackoffCap(t *testing.T) {
	tracker := &blacklistTracker{jitterFn: func() float64 { return 0.5 }}
	if got := tracker.blacklistTTL(2*time.Second, blacklistMaxBackoffStep+3); got != blacklistMaxTTL {
		t.Fatalf("backoff capped ttl = %v, want %v", got, blacklistMaxTTL)
	}
}

func TestBlacklistTrackerSuccessDecaysBackoff(t *testing.T) {
	tracker := &blacklistTracker{}
	state := tracker.stateForProtocol(false)
	state.backoffStep = 3

	tracker.observeSuccess(false)
	if state.backoffStep != 2 {
		t.Fatalf("backoff step after success = %d, want 2", state.backoffStep)
	}
}
