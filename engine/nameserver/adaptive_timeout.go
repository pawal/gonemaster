package nameserver

import (
	"context"
	"errors"
	"net"
	"strings"
	"sync"
	"time"
)

const (
	adaptiveTimeoutFailureThreshold = 2
	adaptiveTimeoutReductionPct     = 25
	adaptiveTimeoutMaxStep          = 3
	adaptiveTimeoutMin              = 500 * time.Millisecond
)

type adaptiveTimeoutTracker struct {
	mu  sync.Mutex
	udp adaptiveTimeoutState
	tcp adaptiveTimeoutState
}

type adaptiveTimeoutState struct {
	consecutiveTimeouts int
	reductionStep       int
}

func (t *adaptiveTimeoutTracker) timeoutFor(base time.Duration, useTCP bool) time.Duration {
	if t == nil || base <= 0 {
		return base
	}
	t.mu.Lock()
	step := t.stateForProtocol(useTCP).reductionStep
	t.mu.Unlock()
	return reducedTimeout(base, step)
}

func (t *adaptiveTimeoutTracker) observeResult(useTCP bool, timeoutPattern bool) {
	if t == nil {
		return
	}
	t.mu.Lock()
	state := t.stateForProtocol(useTCP)
	if !timeoutPattern {
		state.consecutiveTimeouts = 0
		t.mu.Unlock()
		return
	}
	state.consecutiveTimeouts++
	if state.consecutiveTimeouts >= adaptiveTimeoutFailureThreshold {
		state.consecutiveTimeouts = 0
		if state.reductionStep < adaptiveTimeoutMaxStep {
			state.reductionStep++
		}
	}
	t.mu.Unlock()
}

func (t *adaptiveTimeoutTracker) stateForProtocol(useTCP bool) *adaptiveTimeoutState {
	if useTCP {
		return &t.tcp
	}
	return &t.udp
}

func reducedTimeout(base time.Duration, reductionStep int) time.Duration {
	if base <= 0 || reductionStep <= 0 {
		return base
	}
	if reductionStep > adaptiveTimeoutMaxStep {
		reductionStep = adaptiveTimeoutMaxStep
	}
	pct := 100 - (reductionStep * adaptiveTimeoutReductionPct)
	if pct < 1 {
		pct = 1
	}
	reduced := (base * time.Duration(pct)) / 100
	if reduced < adaptiveTimeoutMin {
		return adaptiveTimeoutMin
	}
	return reduced
}

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
