package nameserver

type protocolFastFailState struct {
	consecutiveTimeouts int
	blocked             bool
}

type fastFailTracker struct {
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
	return t.stateForProtocol(useTCP).blocked
}

func (t *fastFailTracker) observeResult(useTCP bool, timeoutPattern bool, threshold int) {
	if t == nil || threshold <= 0 {
		return
	}

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
