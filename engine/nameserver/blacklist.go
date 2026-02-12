package nameserver

import (
	"math"
	"math/rand"
	"time"
)

const (
	blacklistTimeoutFailureThreshold = 2
	blacklistMinTTL                  = 200 * time.Millisecond
	blacklistMaxTTL                  = 10 * time.Second
	blacklistDefaultTTL              = 2 * time.Second
	blacklistJitterPct               = 0.2
	blacklistMaxBackoffStep          = 5
)

type protocolBlacklistState struct {
	blockedUntil        time.Time
	consecutiveTimeouts int
	backoffStep         int
}

type blacklistTracker struct {
	udp      protocolBlacklistState
	tcp      protocolBlacklistState
	jitterFn func() float64
}

func (t *blacklistTracker) stateForProtocol(useTCP bool) *protocolBlacklistState {
	if useTCP {
		return &t.tcp
	}
	return &t.udp
}

func (t *blacklistTracker) isBlocked(useTCP bool, now time.Time) bool {
	if t == nil {
		return false
	}
	if now.IsZero() {
		now = time.Now()
	}
	state := t.stateForProtocol(useTCP)
	if state.blockedUntil.IsZero() {
		return false
	}
	if now.Before(state.blockedUntil) {
		return true
	}
	state.blockedUntil = time.Time{}
	return false
}

func (t *blacklistTracker) observeSuccess(useTCP bool) {
	if t == nil {
		return
	}
	state := t.stateForProtocol(useTCP)
	state.consecutiveTimeouts = 0
	if state.backoffStep > 0 {
		state.backoffStep--
	}
}

func (t *blacklistTracker) observeFailure(useTCP bool, timeoutPattern bool, baseTTL time.Duration, now time.Time) {
	if t == nil {
		return
	}
	if now.IsZero() {
		now = time.Now()
	}
	state := t.stateForProtocol(useTCP)
	if timeoutPattern {
		state.consecutiveTimeouts++
		if state.consecutiveTimeouts < blacklistTimeoutFailureThreshold {
			return
		}
	} else {
		state.consecutiveTimeouts = 0
	}

	state.consecutiveTimeouts = 0
	state.blockedUntil = now.Add(t.blacklistTTL(baseTTL, state.backoffStep))
	if state.backoffStep < blacklistMaxBackoffStep {
		state.backoffStep++
	}
}

func clampBlacklistTTL(ttl time.Duration) time.Duration {
	if ttl <= 0 {
		return blacklistDefaultTTL
	}
	if ttl < blacklistMinTTL {
		return blacklistMinTTL
	}
	if ttl > blacklistMaxTTL {
		return blacklistMaxTTL
	}
	return ttl
}

func (t *blacklistTracker) blacklistTTL(baseTTL time.Duration, backoffStep int) time.Duration {
	ttl := clampBlacklistTTL(baseTTL)
	if backoffStep > 0 {
		if backoffStep > blacklistMaxBackoffStep {
			backoffStep = blacklistMaxBackoffStep
		}
		multiplier := math.Pow(2, float64(backoffStep))
		ttl = time.Duration(float64(ttl) * multiplier)
	}
	ttl = time.Duration(float64(ttl) * t.jitterMultiplier())
	return clampBlacklistTTL(ttl)
}

func (t *blacklistTracker) jitterMultiplier() float64 {
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
	low := 1 - blacklistJitterPct
	high := 1 + blacklistJitterPct
	return low + ((high - low) * raw)
}
