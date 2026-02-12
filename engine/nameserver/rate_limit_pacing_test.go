package nameserver

import (
	"fmt"
	"testing"
	"time"
)

func TestRateLimitPacingTrackerTimeoutBurstBackoff(t *testing.T) {
	t.Parallel()

	tracker := &rateLimitPacingTracker{jitterFn: func() float64 { return 0.5 }}
	now := time.Unix(1700000000, 0)

	tracker.observeResult(false, rateLimitSignalTimeoutPattern, now)
	snap := tracker.snapshot(false)
	if snap.ConsecutiveTimeouts != 1 {
		t.Fatalf("consecutive timeouts after first timeout = %d, want 1", snap.ConsecutiveTimeouts)
	}
	if snap.BackoffStep != 0 {
		t.Fatalf("backoff step after first timeout = %d, want 0", snap.BackoffStep)
	}

	tracker.observeResult(false, rateLimitSignalTimeoutPattern, now)
	snap = tracker.snapshot(false)
	if snap.ConsecutiveTimeouts != 2 {
		t.Fatalf("consecutive timeouts after burst = %d, want 2", snap.ConsecutiveTimeouts)
	}
	if snap.BackoffStep != 1 {
		t.Fatalf("backoff step after timeout burst = %d, want 1", snap.BackoffStep)
	}
	if snap.BackoffDelay != rateLimitPacingBaseDelay {
		t.Fatalf("backoff delay after timeout burst = %v, want %v", snap.BackoffDelay, rateLimitPacingBaseDelay)
	}

	shouldPace, remaining := tracker.shouldPace(false, now)
	if !shouldPace {
		t.Fatalf("expected tracker to pace immediately after timeout burst")
	}
	if remaining != rateLimitPacingBaseDelay {
		t.Fatalf("remaining delay = %v, want %v", remaining, rateLimitPacingBaseDelay)
	}

	shouldPace, _ = tracker.shouldPace(false, now.Add(rateLimitPacingBaseDelay))
	if shouldPace {
		t.Fatalf("expected pacing window to expire at next_allowed timestamp")
	}
}

func TestRateLimitPacingTrackerServfailRatioSpike(t *testing.T) {
	t.Parallel()

	tracker := &rateLimitPacingTracker{jitterFn: func() float64 { return 0.5 }}
	now := time.Unix(1700000100, 0)

	signals := []rateLimitSignal{
		rateLimitSignalNone,
		rateLimitSignalNone,
		rateLimitSignalServfailOrRefused,
		rateLimitSignalNone,
		rateLimitSignalServfailOrRefused,
	}
	for _, signal := range signals {
		tracker.observeResult(false, signal, now)
	}
	if snap := tracker.snapshot(false); snap.BackoffStep != 0 {
		t.Fatalf("backoff should not trigger before min sample threshold, got step %d", snap.BackoffStep)
	}

	tracker.observeResult(false, rateLimitSignalServfailOrRefused, now)
	snap := tracker.snapshot(false)
	if snap.WindowTotal != 6 {
		t.Fatalf("window total = %d, want 6", snap.WindowTotal)
	}
	if snap.WindowServfailRefused != 3 {
		t.Fatalf("window servfail/refused count = %d, want 3", snap.WindowServfailRefused)
	}
	if snap.WindowSuccesses != 3 {
		t.Fatalf("window success count = %d, want 3", snap.WindowSuccesses)
	}
	if snap.BackoffStep != 1 {
		t.Fatalf("backoff step after ratio spike = %d, want 1", snap.BackoffStep)
	}
}

func TestRateLimitPacingTrackerConnectionErrorsAndHardUnreachable(t *testing.T) {
	t.Parallel()

	tracker := &rateLimitPacingTracker{jitterFn: func() float64 { return 0.5 }}
	now := time.Unix(1700000200, 0)

	tracker.observeResult(false, rateLimitSignalHardUnreachable, now)
	if snap := tracker.snapshot(false); snap.BackoffStep != 0 {
		t.Fatalf("hard unreachable should not trigger pacing backoff, got step %d", snap.BackoffStep)
	}

	tracker.observeResult(false, rateLimitSignalConnectionError, now)
	snap := tracker.snapshot(false)
	if snap.BackoffStep != 1 {
		t.Fatalf("connection error should trigger pacing backoff, got step %d", snap.BackoffStep)
	}
}

func TestRateLimitPacingTrackerSuccessDecay(t *testing.T) {
	t.Parallel()

	tracker := &rateLimitPacingTracker{jitterFn: func() float64 { return 0.5 }}
	now := time.Unix(1700000300, 0)

	for i := 0; i < 3; i++ {
		tracker.observeResult(false, rateLimitSignalConnectionError, now.Add(time.Duration(i)*100*time.Millisecond))
	}
	snap := tracker.snapshot(false)
	if snap.BackoffStep != 3 {
		t.Fatalf("backoff step after 3 connection errors = %d, want 3", snap.BackoffStep)
	}
	if snap.BackoffDelay != 4*rateLimitPacingBaseDelay {
		t.Fatalf("backoff delay at step 3 = %v, want %v", snap.BackoffDelay, 4*rateLimitPacingBaseDelay)
	}

	successNow := now.Add(250 * time.Millisecond)
	tracker.observeResult(false, rateLimitSignalNone, successNow)
	snap = tracker.snapshot(false)
	if snap.BackoffStep != 2 {
		t.Fatalf("backoff step after first success decay = %d, want 2", snap.BackoffStep)
	}
	if snap.BackoffDelay != 2*rateLimitPacingBaseDelay {
		t.Fatalf("backoff delay after first success decay = %v, want %v", snap.BackoffDelay, 2*rateLimitPacingBaseDelay)
	}

	tracker.observeResult(false, rateLimitSignalNone, successNow)
	snap = tracker.snapshot(false)
	if snap.BackoffStep != 1 {
		t.Fatalf("backoff step after second success decay = %d, want 1", snap.BackoffStep)
	}
	if snap.BackoffDelay != rateLimitPacingBaseDelay {
		t.Fatalf("backoff delay after second success decay = %v, want %v", snap.BackoffDelay, rateLimitPacingBaseDelay)
	}

	tracker.observeResult(false, rateLimitSignalNone, successNow)
	snap = tracker.snapshot(false)
	if snap.BackoffStep != 0 {
		t.Fatalf("backoff step after third success decay = %d, want 0", snap.BackoffStep)
	}
	if snap.BackoffDelay != 0 {
		t.Fatalf("backoff delay after full success decay = %v, want 0", snap.BackoffDelay)
	}
	if !snap.NextAllowed.IsZero() {
		t.Fatalf("next_allowed should be cleared after full success decay, got %v", snap.NextAllowed)
	}
}

func TestRateLimitPacingTrackerProtocolIsolation(t *testing.T) {
	t.Parallel()

	tracker := &rateLimitPacingTracker{jitterFn: func() float64 { return 0.5 }}
	now := time.Unix(1700000400, 0)

	tracker.observeResult(false, rateLimitSignalConnectionError, now)
	udp := tracker.snapshot(false)
	tcp := tracker.snapshot(true)

	if udp.BackoffStep != 1 {
		t.Fatalf("udp backoff step = %d, want 1", udp.BackoffStep)
	}
	if tcp.BackoffStep != 0 {
		t.Fatalf("tcp backoff step = %d, want 0", tcp.BackoffStep)
	}
	if shouldPace, _ := tracker.shouldPace(true, now); shouldPace {
		t.Fatalf("tcp should not be paced when only udp observed throttling")
	}
}

func TestRateLimitPacingTrackerRollingWindowEviction(t *testing.T) {
	t.Parallel()

	tracker := &rateLimitPacingTracker{jitterFn: func() float64 { return 0.5 }}
	now := time.Unix(1700000500, 0)

	for i := 0; i < rateLimitPacingWindowSize; i++ {
		tracker.observeResult(false, rateLimitSignalServfailOrRefused, now)
	}
	for i := 0; i < 4; i++ {
		tracker.observeResult(false, rateLimitSignalNone, now)
	}

	snap := tracker.snapshot(false)
	if snap.WindowTotal != rateLimitPacingWindowSize {
		t.Fatalf("window total = %d, want %d", snap.WindowTotal, rateLimitPacingWindowSize)
	}
	if snap.WindowSuccesses != 4 {
		t.Fatalf("window successes after eviction = %d, want 4", snap.WindowSuccesses)
	}
	if snap.WindowServfailRefused != rateLimitPacingWindowSize-4 {
		t.Fatalf("window servfail/refused after eviction = %d, want %d", snap.WindowServfailRefused, rateLimitPacingWindowSize-4)
	}
}

func TestRateLimitPacingTrackerConsecutiveTimeoutReset(t *testing.T) {
	t.Parallel()

	tracker := &rateLimitPacingTracker{}
	now := time.Unix(1700000600, 0)

	tracker.observeResult(false, rateLimitSignalTimeoutPattern, now)
	tracker.observeResult(false, rateLimitSignalNone, now)
	if snap := tracker.snapshot(false); snap.ConsecutiveTimeouts != 0 {
		t.Fatalf("consecutive timeout count should reset after non-timeout signal, got %d", snap.ConsecutiveTimeouts)
	}
}

func TestRateLimitPacingTrackerJitterAndDelayBounds(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		jitter   func() float64
		step     int
		expected time.Duration
	}{
		{name: "jitter low clamp", jitter: func() float64 { return -2 }, step: 1, expected: 160 * time.Millisecond},
		{name: "jitter high clamp", jitter: func() float64 { return 2 }, step: 1, expected: 240 * time.Millisecond},
		{name: "step clamp", jitter: func() float64 { return 1 }, step: rateLimitPacingMaxBackoffStep + 10, expected: 7680 * time.Millisecond},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tracker := &rateLimitPacingTracker{jitterFn: tc.jitter}
			if got := tracker.backoffDelayForStep(tc.step); got != tc.expected {
				t.Fatalf("backoffDelayForStep(%d) = %v, want %v", tc.step, got, tc.expected)
			}
		})
	}
}

func TestRateLimitPacingTrackerShouldPaceNil(t *testing.T) {
	t.Parallel()

	var tracker *rateLimitPacingTracker
	shouldPace, remaining := tracker.shouldPace(false, time.Now())
	if shouldPace || remaining != 0 {
		t.Fatalf("nil tracker should not pace; got should=%v remaining=%v", shouldPace, remaining)
	}
}

func TestRateLimitPacingSnapshotWindowErrors(t *testing.T) {
	t.Parallel()

	tracker := &rateLimitPacingTracker{}
	now := time.Unix(1700000700, 0)
	signals := []rateLimitSignal{
		rateLimitSignalNone,
		rateLimitSignalConnectionError,
		rateLimitSignalServfailOrRefused,
		rateLimitSignalNone,
	}
	for _, signal := range signals {
		tracker.observeResult(false, signal, now)
	}

	snap := tracker.snapshot(false)
	if snap.WindowTotal != 4 {
		t.Fatalf("window total = %d, want 4", snap.WindowTotal)
	}
	if snap.WindowSuccesses != 2 {
		t.Fatalf("window successes = %d, want 2", snap.WindowSuccesses)
	}
	if snap.WindowErrors != 2 {
		t.Fatalf("window errors = %d, want 2", snap.WindowErrors)
	}
	if got, want := snap.WindowSuccesses+snap.WindowErrors, snap.WindowTotal; got != want {
		t.Fatalf("window accounting mismatch: successes+errors=%d total=%d", got, want)
	}
}

func BenchmarkRateLimitPacingTrackerObserveResult(b *testing.B) {
	tracker := &rateLimitPacingTracker{jitterFn: func() float64 { return 0.5 }}
	now := time.Unix(1700000800, 0)
	signals := []rateLimitSignal{
		rateLimitSignalNone,
		rateLimitSignalTimeoutPattern,
		rateLimitSignalServfailOrRefused,
		rateLimitSignalConnectionError,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tracker.observeResult(false, signals[i%len(signals)], now)
	}

	snap := tracker.snapshot(false)
	if snap.WindowTotal == 0 {
		b.Fatalf("unexpected empty window after benchmark loop")
	}
	b.ReportMetric(float64(snap.WindowErrors), fmt.Sprintf("window_errors/%d", rateLimitPacingWindowSize))
}
