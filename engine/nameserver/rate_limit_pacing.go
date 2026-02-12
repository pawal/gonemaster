package nameserver

import (
	"math"
	"math/rand"
	"sync"
	"time"
)

const (
	rateLimitPacingWindowSize     = 16
	rateLimitPacingBaseDelay      = 200 * time.Millisecond
	rateLimitPacingMaxDelay       = 10 * time.Second
	rateLimitPacingJitterPct      = 0.2
	rateLimitPacingMaxBackoffStep = 6
)

type rateLimitPacingTracker struct {
	mu       sync.Mutex
	udp      rateLimitPacingState
	tcp      rateLimitPacingState
	jitterFn func() float64
}

type rateLimitPacingState struct {
	window                [rateLimitPacingWindowSize]rateLimitSignal
	windowLen             int
	windowPos             int
	windowSuccesses       int
	windowServfailRefused int
	consecutiveTimeouts   int
	backoffStep           int
	backoffDelay          time.Duration
	nextAllowed           time.Time
}

type rateLimitPacingSnapshot struct {
	WindowTotal           int
	WindowSuccesses       int
	WindowErrors          int
	WindowServfailRefused int
	ConsecutiveTimeouts   int
	BackoffStep           int
	BackoffDelay          time.Duration
	NextAllowed           time.Time
}

func (t *rateLimitPacingTracker) observeResult(useTCP bool, signal rateLimitSignal, now time.Time) {
	if t == nil {
		return
	}
	if now.IsZero() {
		now = time.Now()
	}

	t.mu.Lock()
	state := t.stateForProtocol(useTCP)
	state.observeSignal(signal)

	likelyRateLimit := false
	switch signal {
	case rateLimitSignalTimeoutPattern:
		if isTimeoutBurstSignal(state.consecutiveTimeouts) {
			likelyRateLimit = true
		}
	case rateLimitSignalConnectionError:
		likelyRateLimit = true
	case rateLimitSignalServfailOrRefused:
		if isServfailRefusedRatioSpike(state.windowServfailRefused, state.windowLen) {
			likelyRateLimit = true
		}
	}

	if signal == rateLimitSignalNone {
		t.decayOnSuccess(state, now)
		t.mu.Unlock()
		return
	}
	if likelyRateLimit {
		t.applyBackoff(state, now)
	}
	t.mu.Unlock()
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
	return rateLimitPacingSnapshot{
		WindowTotal:           state.windowLen,
		WindowSuccesses:       state.windowSuccesses,
		WindowErrors:          state.windowLen - state.windowSuccesses,
		WindowServfailRefused: state.windowServfailRefused,
		ConsecutiveTimeouts:   state.consecutiveTimeouts,
		BackoffStep:           state.backoffStep,
		BackoffDelay:          state.backoffDelay,
		NextAllowed:           state.nextAllowed,
	}
}

func (t *rateLimitPacingTracker) stateForProtocol(useTCP bool) *rateLimitPacingState {
	if useTCP {
		return &t.tcp
	}
	return &t.udp
}

func (t *rateLimitPacingTracker) applyBackoff(state *rateLimitPacingState, now time.Time) {
	if state == nil {
		return
	}
	if state.backoffStep < rateLimitPacingMaxBackoffStep {
		state.backoffStep++
	}
	delay := t.backoffDelayForStep(state.backoffStep)
	state.backoffDelay = delay
	next := now.Add(delay)
	if next.After(state.nextAllowed) {
		state.nextAllowed = next
	}
}

func (t *rateLimitPacingTracker) decayOnSuccess(state *rateLimitPacingState, now time.Time) {
	if state == nil {
		return
	}
	state.consecutiveTimeouts = 0
	if state.backoffStep == 0 {
		state.backoffDelay = 0
		state.nextAllowed = time.Time{}
		return
	}

	state.backoffStep--
	if state.backoffStep == 0 {
		state.backoffDelay = 0
		state.nextAllowed = time.Time{}
		return
	}

	nextDelay := t.backoffDelayForStep(state.backoffStep)
	state.backoffDelay = nextDelay
	if state.nextAllowed.After(now) {
		remaining := state.nextAllowed.Sub(now)
		if remaining > nextDelay {
			state.nextAllowed = now.Add(nextDelay)
		}
	} else {
		state.nextAllowed = time.Time{}
	}
}

func (t *rateLimitPacingTracker) backoffDelayForStep(step int) time.Duration {
	if step <= 0 {
		return 0
	}
	if step > rateLimitPacingMaxBackoffStep {
		step = rateLimitPacingMaxBackoffStep
	}
	delay := float64(rateLimitPacingBaseDelay) * math.Pow(2, float64(step-1))
	delay *= t.jitterMultiplier()
	if delay > float64(rateLimitPacingMaxDelay) {
		return rateLimitPacingMaxDelay
	}
	if delay < float64(time.Millisecond) {
		return time.Millisecond
	}
	return time.Duration(delay)
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
		return
	}
	s.consecutiveTimeouts = 0
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
