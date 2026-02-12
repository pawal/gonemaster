package nameserver

import "time"

const (
	blacklistTimeoutFailureThreshold = 2
	blacklistMinTTL                  = 200 * time.Millisecond
	blacklistMaxTTL                  = 10 * time.Second
	blacklistDefaultTTL              = 2 * time.Second
)

type protocolBlacklistState struct {
	blockedUntil        time.Time
	consecutiveTimeouts int
}

type blacklistTracker struct {
	udp protocolBlacklistState
	tcp protocolBlacklistState
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
	state.blockedUntil = now.Add(clampBlacklistTTL(baseTTL))
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
