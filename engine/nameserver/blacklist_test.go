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
