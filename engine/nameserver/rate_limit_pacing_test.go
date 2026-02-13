package nameserver

import (
	"fmt"
	"testing"
	"time"

	"codeberg.org/pawal/gonemaster/engine/profile"
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

func TestRateLimitPacingMetricsCounters(t *testing.T) {
	t.Parallel()

	tracker := &rateLimitPacingTracker{jitterFn: func() float64 { return 0.5 }}
	now := time.Unix(1700000850, 0)

	tracker.observeResult(false, rateLimitSignalTimeoutPattern, now)
	tracker.observeResult(false, rateLimitSignalTimeoutPattern, now)
	tracker.observeResult(false, rateLimitSignalConnectionError, now)
	for i := 0; i < 3; i++ {
		tracker.observeResult(false, rateLimitSignalServfailOrRefused, now)
	}
	for i := 0; i < 3; i++ {
		tracker.observeResult(false, rateLimitSignalNone, now)
	}

	tracker.recordPacingDelay(false)
	tracker.recordPacingSkip(false)

	snap := tracker.snapshot(false)
	if snap.DetectionTimeoutBurst != 1 {
		t.Fatalf("timeout burst detections = %d, want 1", snap.DetectionTimeoutBurst)
	}
	if snap.DetectionConnError != 1 {
		t.Fatalf("connection error detections = %d, want 1", snap.DetectionConnError)
	}
	if snap.DetectionServfail != 1 {
		t.Fatalf("servfail/refused detections = %d, want 1", snap.DetectionServfail)
	}
	if snap.PacingDelayCount != 1 {
		t.Fatalf("pacing delay count = %d, want 1", snap.PacingDelayCount)
	}
	if snap.PacingSkipCount != 1 {
		t.Fatalf("pacing skip count = %d, want 1", snap.PacingSkipCount)
	}
}

func TestRateLimitPacingSyntheticConvergence(t *testing.T) {
	t.Parallel()

	tracker := &rateLimitPacingTracker{jitterFn: func() float64 { return 0.5 }}
	tracker.setPolicy(rateLimitPacingPolicyConfig{
		Enabled:   true,
		MinDelay:  20 * time.Millisecond,
		MaxDelay:  2 * time.Second,
		EWMAAlpha: 0.4,
		Headroom:  0.85,
	})

	now := time.Unix(1700001200, 0)
	for i := 0; i < 4; i++ {
		tracker.observeResult(false, rateLimitSignalConnectionError, now.Add(time.Duration(i)*10*time.Millisecond))
	}

	start := now.Add(100 * time.Millisecond)
	for i := 0; i < 30; i++ {
		tracker.observeResult(false, rateLimitSignalNone, start.Add(time.Duration(i)*100*time.Millisecond))
	}

	snap := tracker.snapshot(false)
	if snap.BackoffStep != 0 {
		t.Fatalf("expected backoff to decay to zero, got %d", snap.BackoffStep)
	}
	if snap.EstimatedInterval < 90*time.Millisecond || snap.EstimatedInterval > 120*time.Millisecond {
		t.Fatalf("estimated interval not converged near 100ms: %v", snap.EstimatedInterval)
	}
	if snap.AdaptiveDelay < 100*time.Millisecond || snap.AdaptiveDelay > 140*time.Millisecond {
		t.Fatalf("adaptive delay not converged to bounded target, got %v", snap.AdaptiveDelay)
	}
}

func TestRateLimitPacingSyntheticNoStarvation(t *testing.T) {
	t.Parallel()

	tracker := &rateLimitPacingTracker{jitterFn: func() float64 { return 0.5 }}
	tracker.setPolicy(rateLimitPacingPolicyConfig{
		Enabled:   true,
		MinDelay:  150 * time.Millisecond,
		MaxDelay:  150 * time.Millisecond,
		EWMAAlpha: 0.5,
		Headroom:  0.9,
	})

	start := time.Unix(1700001300, 0)
	tracker.observeResult(false, rateLimitSignalConnectionError, start)

	allowed := 0
	for i := 1; i <= 12; i++ {
		now := start.Add(time.Duration(i) * 50 * time.Millisecond)
		shouldPace, _ := tracker.shouldPace(false, now)
		if shouldPace {
			continue
		}
		allowed++
		tracker.observeResult(false, rateLimitSignalNone, now)
	}

	if allowed == 0 {
		t.Fatalf("expected at least one non-paced slot over synthetic schedule")
	}
	if allowed >= 12 {
		t.Fatalf("expected some paced intervals in synthetic schedule, got all allowed")
	}
}

func TestRateLimitPacingTrackerAdaptivePolicyEWMA(t *testing.T) {
	t.Parallel()

	tracker := &rateLimitPacingTracker{jitterFn: func() float64 { return 0.5 }}
	tracker.setPolicy(rateLimitPacingPolicyConfig{
		Enabled:   true,
		MinDelay:  50 * time.Millisecond,
		MaxDelay:  5 * time.Second,
		EWMAAlpha: 0.5,
		Headroom:  0.8,
	})

	now := time.Unix(1700000900, 0)
	tracker.observeResult(false, rateLimitSignalNone, now)
	tracker.observeResult(false, rateLimitSignalNone, now.Add(200*time.Millisecond))

	snap := tracker.snapshot(false)
	if snap.EstimatedInterval != 200*time.Millisecond {
		t.Fatalf("estimated interval = %v, want %v", snap.EstimatedInterval, 200*time.Millisecond)
	}
	if snap.AdaptiveDelay != 250*time.Millisecond {
		t.Fatalf("adaptive delay = %v, want %v", snap.AdaptiveDelay, 250*time.Millisecond)
	}
	if snap.EstimatedQPS != 5 {
		t.Fatalf("estimated qps = %v, want 5", snap.EstimatedQPS)
	}

	shouldPace, remaining := tracker.shouldPace(false, now.Add(200*time.Millisecond))
	if !shouldPace {
		t.Fatalf("expected pacing after success-based adaptive update")
	}
	if remaining != 250*time.Millisecond {
		t.Fatalf("remaining adaptive delay = %v, want %v", remaining, 250*time.Millisecond)
	}

	tracker.observeResult(false, rateLimitSignalNone, now.Add(800*time.Millisecond))
	snap = tracker.snapshot(false)
	if snap.EstimatedInterval != 400*time.Millisecond {
		t.Fatalf("estimated interval after EWMA update = %v, want %v", snap.EstimatedInterval, 400*time.Millisecond)
	}
	if snap.AdaptiveDelay != 500*time.Millisecond {
		t.Fatalf("adaptive delay after EWMA update = %v, want %v", snap.AdaptiveDelay, 500*time.Millisecond)
	}
}

func TestRateLimitPacingTrackerAdaptivePolicyDisabledByDefault(t *testing.T) {
	t.Parallel()

	tracker := &rateLimitPacingTracker{}
	now := time.Unix(1700001000, 0)
	tracker.observeResult(false, rateLimitSignalNone, now)
	tracker.observeResult(false, rateLimitSignalNone, now.Add(100*time.Millisecond))

	snap := tracker.snapshot(false)
	if snap.AdaptiveDelay != 0 {
		t.Fatalf("adaptive delay with disabled policy = %v, want 0", snap.AdaptiveDelay)
	}
	if snap.EstimatedInterval != 0 {
		t.Fatalf("estimated interval with disabled policy = %v, want 0", snap.EstimatedInterval)
	}
	if shouldPace, _ := tracker.shouldPace(false, now.Add(100*time.Millisecond)); shouldPace {
		t.Fatalf("disabled policy should not create pacing on successes")
	}
}

func TestRateLimitPacingTrackerAdaptivePolicyBounds(t *testing.T) {
	t.Parallel()

	tracker := &rateLimitPacingTracker{jitterFn: func() float64 { return 0.5 }}
	tracker.setPolicy(rateLimitPacingPolicyConfig{
		Enabled:   true,
		MinDelay:  300 * time.Millisecond,
		MaxDelay:  600 * time.Millisecond,
		EWMAAlpha: 1,
		Headroom:  0.5,
	})

	now := time.Unix(1700001100, 0)
	tracker.observeResult(false, rateLimitSignalNone, now)

	tracker.observeResult(false, rateLimitSignalNone, now.Add(50*time.Millisecond))
	if snap := tracker.snapshot(false); snap.AdaptiveDelay != 300*time.Millisecond {
		t.Fatalf("adaptive delay lower bound = %v, want %v", snap.AdaptiveDelay, 300*time.Millisecond)
	}

	tracker.observeResult(false, rateLimitSignalNone, now.Add(5*time.Second))
	if snap := tracker.snapshot(false); snap.AdaptiveDelay != 600*time.Millisecond {
		t.Fatalf("adaptive delay upper bound = %v, want %v", snap.AdaptiveDelay, 600*time.Millisecond)
	}
}

func TestResolveRateLimitPacingPolicyConfig(t *testing.T) {
	t.Parallel()

	prof := profile.New()
	prof.Resolver.Defaults.RateLimitPacingEnabled = true
	prof.Resolver.Defaults.RateLimitPacingMinMS = 150
	prof.Resolver.Defaults.RateLimitPacingMaxMS = 2500
	prof.Resolver.Defaults.RateLimitPacingEWMAAlphaPct = 40
	prof.Resolver.Defaults.RateLimitPacingHeadroomPct = 85

	cfg := resolveRateLimitPacingPolicyConfig(prof)
	if !cfg.Enabled {
		t.Fatalf("expected resolved policy to be enabled")
	}
	if cfg.MinDelay != 150*time.Millisecond {
		t.Fatalf("min delay = %v, want %v", cfg.MinDelay, 150*time.Millisecond)
	}
	if cfg.MaxDelay != 2500*time.Millisecond {
		t.Fatalf("max delay = %v, want %v", cfg.MaxDelay, 2500*time.Millisecond)
	}
	if cfg.EWMAAlpha != 0.4 {
		t.Fatalf("ewma alpha = %v, want 0.4", cfg.EWMAAlpha)
	}
	if cfg.Headroom != 0.85 {
		t.Fatalf("headroom = %v, want 0.85", cfg.Headroom)
	}
}

func TestSanitizeRateLimitPacingPolicyConfig(t *testing.T) {
	t.Parallel()

	cfg := sanitizeRateLimitPacingPolicyConfig(rateLimitPacingPolicyConfig{
		Enabled:   true,
		MinDelay:  2 * time.Second,
		MaxDelay:  100 * time.Millisecond,
		EWMAAlpha: 0,
		Headroom:  2,
	})

	if cfg.MinDelay != 2*time.Second {
		t.Fatalf("min delay = %v, want %v", cfg.MinDelay, 2*time.Second)
	}
	if cfg.MaxDelay != 2*time.Second {
		t.Fatalf("max delay should clamp to min delay, got %v", cfg.MaxDelay)
	}
	if cfg.EWMAAlpha != rateLimitPacingDefaultEWMAAlpha {
		t.Fatalf("ewma alpha fallback = %v, want %v", cfg.EWMAAlpha, rateLimitPacingDefaultEWMAAlpha)
	}
	if cfg.Headroom != rateLimitPacingDefaultHeadroom {
		t.Fatalf("headroom fallback = %v, want %v", cfg.Headroom, rateLimitPacingDefaultHeadroom)
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
