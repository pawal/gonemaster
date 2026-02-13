package nameserver

import (
	"math"
	"math/rand"
	"sync"
	"time"

	"codeberg.org/pawal/gonemaster/engine/profile"
)

const (
	rateLimitPacingWindowSize        = 16
	rateLimitPacingBaseDelay         = 200 * time.Millisecond
	rateLimitPacingDefaultMinDelay   = 50 * time.Millisecond
	rateLimitPacingDefaultMaxDelay   = 10 * time.Second
	rateLimitPacingDefaultEWMAAlpha  = 0.25
	rateLimitPacingDefaultHeadroom   = 0.9
	rateLimitPacingDefaultEnabled    = false
	rateLimitPacingJitterPct         = 0.2
	rateLimitPacingMaxBackoffStep    = 6
	rateLimitPacingMinSampleInterval = time.Millisecond
)

type rateLimitPacingPolicyConfig struct {
	Enabled   bool
	MinDelay  time.Duration
	MaxDelay  time.Duration
	EWMAAlpha float64
	Headroom  float64
}

type rateLimitPacingTracker struct {
	mu       sync.Mutex
	udp      rateLimitPacingState
	tcp      rateLimitPacingState
	jitterFn func() float64
	policy   rateLimitPacingPolicyConfig
}

type rateLimitPacingState struct {
	window                [rateLimitPacingWindowSize]rateLimitSignal
	windowLen             int
	windowPos             int
	windowSuccesses       int
	windowServfailRefused int
	consecutiveTimeouts   int
	timeoutBurstCount     int
	backoffStep           int
	backoffDelay          time.Duration
	nextAllowed           time.Time
	lastSuccessAt         time.Time
	ewmaInterval          time.Duration
	adaptiveDelay         time.Duration
	detectionTimeoutBurst int
	detectionServfail     int
	detectionConnError    int
	pacingDelayCount      int
	pacingSkipCount       int
}

type rateLimitPacingSnapshot struct {
	WindowTotal           int
	WindowSuccesses       int
	WindowErrors          int
	WindowServfailRefused int
	ConsecutiveTimeouts   int
	BackoffStep           int
	BackoffDelay          time.Duration
	AdaptiveDelay         time.Duration
	EstimatedInterval     time.Duration
	EstimatedQPS          float64
	NextAllowed           time.Time
	DetectionTimeoutBurst int
	DetectionServfail     int
	DetectionConnError    int
	PacingDelayCount      int
	PacingSkipCount       int
}

type rateLimitPacingObservation struct {
	Detected bool
	Reason   string
	Signal   rateLimitSignal
	Snapshot rateLimitPacingSnapshot
}

type rateLimitPacingDecision struct {
	Action    string
	Remaining time.Duration
	Budget    time.Duration
	Snapshot  rateLimitPacingSnapshot
}

func defaultRateLimitPacingPolicyConfig() rateLimitPacingPolicyConfig {
	return rateLimitPacingPolicyConfig{
		Enabled:   rateLimitPacingDefaultEnabled,
		MinDelay:  rateLimitPacingDefaultMinDelay,
		MaxDelay:  rateLimitPacingDefaultMaxDelay,
		EWMAAlpha: rateLimitPacingDefaultEWMAAlpha,
		Headroom:  rateLimitPacingDefaultHeadroom,
	}
}

func sanitizeRateLimitPacingPolicyConfig(cfg rateLimitPacingPolicyConfig) rateLimitPacingPolicyConfig {
	def := defaultRateLimitPacingPolicyConfig()
	if cfg.MinDelay <= 0 {
		cfg.MinDelay = def.MinDelay
	}
	if cfg.MaxDelay <= 0 {
		cfg.MaxDelay = def.MaxDelay
	}
	if cfg.MaxDelay < cfg.MinDelay {
		cfg.MaxDelay = cfg.MinDelay
	}
	if cfg.EWMAAlpha <= 0 || cfg.EWMAAlpha > 1 {
		cfg.EWMAAlpha = def.EWMAAlpha
	}
	if cfg.Headroom <= 0 || cfg.Headroom > 1 {
		cfg.Headroom = def.Headroom
	}
	return cfg
}

func resolveRateLimitPacingPolicyConfig(prof *profile.Profile) rateLimitPacingPolicyConfig {
	cfg := defaultRateLimitPacingPolicyConfig()
	if prof == nil {
		prof = profile.Effective()
	}
	if prof == nil {
		return cfg
	}

	def := prof.Resolver.Defaults
	cfg.Enabled = def.RateLimitPacingEnabled
	if def.RateLimitPacingMinMS > 0 {
		cfg.MinDelay = time.Duration(def.RateLimitPacingMinMS) * time.Millisecond
	}
	if def.RateLimitPacingMaxMS > 0 {
		cfg.MaxDelay = time.Duration(def.RateLimitPacingMaxMS) * time.Millisecond
	}
	if def.RateLimitPacingEWMAAlphaPct > 0 {
		cfg.EWMAAlpha = float64(def.RateLimitPacingEWMAAlphaPct) / 100
	}
	if def.RateLimitPacingHeadroomPct > 0 {
		cfg.Headroom = float64(def.RateLimitPacingHeadroomPct) / 100
	}

	return sanitizeRateLimitPacingPolicyConfig(cfg)
}

func (t *rateLimitPacingTracker) setPolicy(cfg rateLimitPacingPolicyConfig) {
	if t == nil {
		return
	}
	t.mu.Lock()
	t.policy = sanitizeRateLimitPacingPolicyConfig(cfg)
	t.mu.Unlock()
}

func (t *rateLimitPacingTracker) policyConfig() rateLimitPacingPolicyConfig {
	if t == nil {
		return defaultRateLimitPacingPolicyConfig()
	}
	t.mu.Lock()
	cfg := t.policy
	t.mu.Unlock()
	return sanitizeRateLimitPacingPolicyConfig(cfg)
}

func (t *rateLimitPacingTracker) observeResult(useTCP bool, signal rateLimitSignal, now time.Time) {
	_ = t.observeResultWithObservation(useTCP, signal, now)
}

func (t *rateLimitPacingTracker) observeResultWithObservation(useTCP bool, signal rateLimitSignal, now time.Time) rateLimitPacingObservation {
	if t == nil {
		return rateLimitPacingObservation{}
	}
	if now.IsZero() {
		now = time.Now()
	}
	cfg := t.policyConfig()

	t.mu.Lock()
	defer t.mu.Unlock()
	state := t.stateForProtocol(useTCP)
	state.observeSignal(signal)

	likelyRateLimit := false
	reason := ""
	switch signal {
	case rateLimitSignalTimeoutPattern:
		if isTimeoutBurstSignal(state.timeoutBurstCount) {
			likelyRateLimit = true
			reason = "timeout_burst"
			state.detectionTimeoutBurst++
		}
	case rateLimitSignalConnectionError:
		likelyRateLimit = true
		reason = "connection_error"
		state.detectionConnError++
	case rateLimitSignalServfailOrRefused:
		if isServfailRefusedRatioSpike(state.windowServfailRefused, state.windowLen) {
			likelyRateLimit = true
			reason = "servfail_refused_spike"
			state.detectionServfail++
		}
	}

	if signal == rateLimitSignalNone {
		state.observeSuccess(now, cfg)
		t.decayOnSuccess(state, now, cfg)
		return rateLimitPacingObservation{
			Detected: false,
			Reason:   "",
			Signal:   signal,
			Snapshot: snapshotFromPacingState(state),
		}
	}
	if likelyRateLimit {
		t.applyBackoff(state, now, cfg)
	}
	return rateLimitPacingObservation{
		Detected: likelyRateLimit,
		Reason:   reason,
		Signal:   signal,
		Snapshot: snapshotFromPacingState(state),
	}
}

func (t *rateLimitPacingTracker) shouldPace(useTCP bool, now time.Time) (bool, time.Duration) {
	if t == nil {
		return false, 0
	}
	if now.IsZero() {
		now = time.Now()
	}
	t.mu.Lock()
	nextAllowed := t.stateForProtocol(useTCP).nextAllowed
	t.mu.Unlock()
	if nextAllowed.IsZero() || !nextAllowed.After(now) {
		return false, 0
	}
	return true, nextAllowed.Sub(now)
}

func (t *rateLimitPacingTracker) snapshot(useTCP bool) rateLimitPacingSnapshot {
	if t == nil {
		return rateLimitPacingSnapshot{}
	}

	t.mu.Lock()
	state := *t.stateForProtocol(useTCP)
	t.mu.Unlock()
	return snapshotFromPacingState(&state)
}

func (t *rateLimitPacingTracker) stateForProtocol(useTCP bool) *rateLimitPacingState {
	if useTCP {
		return &t.tcp
	}
	return &t.udp
}

func (t *rateLimitPacingTracker) recordPacingDelay(useTCP bool) {
	if t == nil {
		return
	}
	t.mu.Lock()
	t.stateForProtocol(useTCP).pacingDelayCount++
	t.mu.Unlock()
}

func (t *rateLimitPacingTracker) recordPacingSkip(useTCP bool) {
	if t == nil {
		return
	}
	t.mu.Lock()
	t.stateForProtocol(useTCP).pacingSkipCount++
	t.mu.Unlock()
}

func (t *rateLimitPacingTracker) applyBackoff(state *rateLimitPacingState, now time.Time, cfg rateLimitPacingPolicyConfig) {
	if state == nil {
		return
	}
	if state.backoffStep < rateLimitPacingMaxBackoffStep {
		state.backoffStep++
	}
	state.backoffDelay = t.backoffDelayForStepWithConfig(state.backoffStep, cfg)
	delay := state.effectiveDelay(cfg)
	if delay <= 0 {
		state.nextAllowed = time.Time{}
		return
	}
	next := now.Add(delay)
	if next.After(state.nextAllowed) {
		state.nextAllowed = next
	}
}

func (t *rateLimitPacingTracker) decayOnSuccess(state *rateLimitPacingState, now time.Time, cfg rateLimitPacingPolicyConfig) {
	if state == nil {
		return
	}
	state.consecutiveTimeouts = 0
	if state.backoffStep > 0 {
		state.backoffStep--
	}
	if state.backoffStep > 0 {
		state.backoffDelay = t.backoffDelayForStepWithConfig(state.backoffStep, cfg)
	} else {
		state.backoffDelay = 0
	}

	delay := state.effectiveDelay(cfg)
	if delay <= 0 {
		state.nextAllowed = time.Time{}
		return
	}
	if state.nextAllowed.After(now) {
		remaining := state.nextAllowed.Sub(now)
		if remaining > delay {
			state.nextAllowed = now.Add(delay)
		}
		return
	}
	state.nextAllowed = now.Add(delay)
}

func (t *rateLimitPacingTracker) backoffDelayForStep(step int) time.Duration {
	return t.backoffDelayForStepWithConfig(step, t.policyConfig())
}

func (t *rateLimitPacingTracker) backoffDelayForStepWithConfig(step int, cfg rateLimitPacingPolicyConfig) time.Duration {
	cfg = sanitizeRateLimitPacingPolicyConfig(cfg)
	if step <= 0 {
		return 0
	}
	if step > rateLimitPacingMaxBackoffStep {
		step = rateLimitPacingMaxBackoffStep
	}
	delay := float64(rateLimitPacingBaseDelay) * math.Pow(2, float64(step-1))
	delay *= t.jitterMultiplier()
	bounded := time.Duration(delay)
	if bounded < cfg.MinDelay {
		bounded = cfg.MinDelay
	}
	if bounded > cfg.MaxDelay {
		bounded = cfg.MaxDelay
	}
	return bounded
}

func (t *rateLimitPacingTracker) jitterMultiplier() float64 {
	jitterFn := rand.Float64
	if t != nil && t.jitterFn != nil {
		jitterFn = t.jitterFn
	}
	raw := jitterFn()
	if raw < 0 {
		raw = 0
	} else if raw > 1 {
		raw = 1
	}
	low := 1 - rateLimitPacingJitterPct
	high := 1 + rateLimitPacingJitterPct
	return low + ((high - low) * raw)
}

func (s *rateLimitPacingState) observeSignal(signal rateLimitSignal) {
	if s == nil {
		return
	}
	s.pushWindowSignal(signal)
	if signal == rateLimitSignalTimeoutPattern {
		s.consecutiveTimeouts++
		s.timeoutBurstCount++
		return
	}
	s.consecutiveTimeouts = 0
	s.timeoutBurstCount = 0
}

func (s *rateLimitPacingState) pushWindowSignal(signal rateLimitSignal) {
	if s == nil {
		return
	}
	if s.windowLen == rateLimitPacingWindowSize {
		evicted := s.window[s.windowPos]
		if evicted == rateLimitSignalNone {
			s.windowSuccesses--
		}
		if evicted == rateLimitSignalServfailOrRefused {
			s.windowServfailRefused--
		}
	} else {
		s.windowLen++
	}

	s.window[s.windowPos] = signal
	s.windowPos = (s.windowPos + 1) % rateLimitPacingWindowSize

	if signal == rateLimitSignalNone {
		s.windowSuccesses++
	}
	if signal == rateLimitSignalServfailOrRefused {
		s.windowServfailRefused++
	}
}

func (s *rateLimitPacingState) observeSuccess(now time.Time, cfg rateLimitPacingPolicyConfig) {
	if s == nil || !cfg.Enabled {
		return
	}
	if !s.lastSuccessAt.IsZero() && now.After(s.lastSuccessAt) {
		sample := now.Sub(s.lastSuccessAt)
		if sample < rateLimitPacingMinSampleInterval {
			sample = rateLimitPacingMinSampleInterval
		}
		if s.ewmaInterval <= 0 {
			s.ewmaInterval = sample
		} else {
			next := (cfg.EWMAAlpha * float64(sample)) + ((1 - cfg.EWMAAlpha) * float64(s.ewmaInterval))
			s.ewmaInterval = time.Duration(next)
			if s.ewmaInterval < rateLimitPacingMinSampleInterval {
				s.ewmaInterval = rateLimitPacingMinSampleInterval
			}
		}
		target := time.Duration(float64(s.ewmaInterval) / cfg.Headroom)
		s.adaptiveDelay = clampDelay(target, cfg.MinDelay, cfg.MaxDelay)
	}
	s.lastSuccessAt = now
}

func (s *rateLimitPacingState) estimatedQPS() float64 {
	if s == nil || s.ewmaInterval <= 0 {
		return 0
	}
	return 1 / s.ewmaInterval.Seconds()
}

func (s *rateLimitPacingState) effectiveDelay(cfg rateLimitPacingPolicyConfig) time.Duration {
	if s == nil {
		return 0
	}
	delay := s.backoffDelay
	if s.adaptiveDelay > delay {
		delay = s.adaptiveDelay
	}
	if delay <= 0 {
		return 0
	}
	cfg = sanitizeRateLimitPacingPolicyConfig(cfg)
	return clampDelay(delay, cfg.MinDelay, cfg.MaxDelay)
}

func snapshotFromPacingState(state *rateLimitPacingState) rateLimitPacingSnapshot {
	if state == nil {
		return rateLimitPacingSnapshot{}
	}
	return rateLimitPacingSnapshot{
		WindowTotal:           state.windowLen,
		WindowSuccesses:       state.windowSuccesses,
		WindowErrors:          state.windowLen - state.windowSuccesses,
		WindowServfailRefused: state.windowServfailRefused,
		ConsecutiveTimeouts:   state.consecutiveTimeouts,
		BackoffStep:           state.backoffStep,
		BackoffDelay:          state.backoffDelay,
		AdaptiveDelay:         state.adaptiveDelay,
		EstimatedInterval:     state.ewmaInterval,
		EstimatedQPS:          state.estimatedQPS(),
		NextAllowed:           state.nextAllowed,
		DetectionTimeoutBurst: state.detectionTimeoutBurst,
		DetectionServfail:     state.detectionServfail,
		DetectionConnError:    state.detectionConnError,
		PacingDelayCount:      state.pacingDelayCount,
		PacingSkipCount:       state.pacingSkipCount,
	}
}

func clampDelay(delay time.Duration, minDelay time.Duration, maxDelay time.Duration) time.Duration {
	if delay <= 0 {
		return 0
	}
	if minDelay > 0 && delay < minDelay {
		return minDelay
	}
	if maxDelay > 0 && delay > maxDelay {
		return maxDelay
	}
	return delay
}
