package nameserver

import (
	"context"
	"errors"
	"net"
	"strings"
	"sync"
)

// isTimeoutPatternError reports whether err looks like a DNS query timeout.
// Context cancellation and deadline exceeded are excluded - those are job-level
// signals, not nameserver-level failure indicators.
func isTimeoutPatternError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
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

func (t *fastFailTracker) observeResult(useTCP bool, timeoutPattern bool, threshold int) {
	if t == nil || threshold <= 0 {
		return
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	state := t.stateForProtocol(useTCP)
	if !timeoutPattern {
		state.consecutiveTimeouts = 0
		state.blocked = false
		return
	}

	if state.blocked {
		return
	}
	state.consecutiveTimeouts++
	if state.consecutiveTimeouts >= threshold {
		state.consecutiveTimeouts = 0
		state.blocked = true
	}
}
