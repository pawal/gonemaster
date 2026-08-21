package dnstest

import (
	"sync"

	"codeberg.org/pawal/gonemaster/engine/querytrace"
)

// RecordingTrace is a QueryTrace test double capturing every attempt and
// decision event. It locks because a real run fires these from several
// goroutines, even where a test drives it serially.
type RecordingTrace struct {
	mu        sync.Mutex
	attempts  []querytrace.AttemptEvent
	decisions []querytrace.DecisionEvent
}

// AttemptDone records one attempt event.
func (r *RecordingTrace) AttemptDone(ev querytrace.AttemptEvent) {
	r.mu.Lock()
	r.attempts = append(r.attempts, ev)
	r.mu.Unlock()
}

// Decision records one decision event.
func (r *RecordingTrace) Decision(ev querytrace.DecisionEvent) {
	r.mu.Lock()
	r.decisions = append(r.decisions, ev)
	r.mu.Unlock()
}

// Attempts returns a copy of the captured attempt events.
func (r *RecordingTrace) Attempts() []querytrace.AttemptEvent {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]querytrace.AttemptEvent(nil), r.attempts...)
}

// Decisions returns a copy of the captured decision events.
func (r *RecordingTrace) Decisions() []querytrace.DecisionEvent {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]querytrace.DecisionEvent(nil), r.decisions...)
}

// DecisionsOfKind returns the captured decisions matching kind.
func (r *RecordingTrace) DecisionsOfKind(kind querytrace.DecisionKind) []querytrace.DecisionEvent {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []querytrace.DecisionEvent
	for _, d := range r.decisions {
		if d.Kind == kind {
			out = append(out, d)
		}
	}
	return out
}
