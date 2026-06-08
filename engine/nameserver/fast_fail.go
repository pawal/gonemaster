package nameserver

import (
	"errors"
	"net"
	"strings"
	"sync"
)

// isTimeoutPatternError reports whether err looks like a network timeout.
// Caller is expected to gate on the outer context's Err() to distinguish a
// job-level cancellation from a real nameserver-side timeout - a fired dial
// deadline wraps context.DeadlineExceeded internally even when the outer
// context is fine.
func isTimeoutPatternError(err error) bool {
	if err == nil {
		return false
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "timeout")
}

type protocolFastFailState struct {
	consecutiveTimeouts int
	blocked             bool
}

type fastFailTracker struct {
	mu  sync.Mutex
	udp protocolFastFailState
	tcp protocolFastFailState
}

func (t *fastFailTracker) stateForProtocol(useTCP bool) *protocolFastFailState {
	if useTCP {
		return &t.tcp
	}
	return &t.udp
}

func (t *fastFailTracker) shouldSkip(useTCP bool, threshold int) bool {
	if t == nil || threshold <= 0 {
		return false
	}
	t.mu.Lock()
	blocked := t.stateForProtocol(useTCP).blocked
	t.mu.Unlock()
	return blocked
}

// sawConsecutiveTimeouts reports whether at least two consecutive timeouts
// have been observed on this protocol, or fast-fail has already engaged.
// Used to debounce error-cache writes so a single transient drop is not
// enough to blackout the nameserver.
func (t *fastFailTracker) sawConsecutiveTimeouts(useTCP bool) bool {
	if t == nil {
		return false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	state := t.stateForProtocol(useTCP)
	return state.blocked || state.consecutiveTimeouts >= 2
}

// observeResult records an attempt outcome and reports whether this call just
// transitioned the nameserver/protocol into the blocked state.
func (t *fastFailTracker) observeResult(useTCP bool, timeoutPattern bool, threshold int) bool {
	if t == nil || threshold <= 0 {
		return false
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	state := t.stateForProtocol(useTCP)
	if !timeoutPattern {
		state.consecutiveTimeouts = 0
		state.blocked = false
		return false
	}

	if state.blocked {
		return false
	}
	state.consecutiveTimeouts++
	if state.consecutiveTimeouts >= threshold {
		state.consecutiveTimeouts = 0
		state.blocked = true
		return true
	}
	return false
}
