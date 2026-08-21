package dnstest

import (
	"sync"
	"testing"

	"codeberg.org/pawal/gonemaster/engine/querytrace"
)

func TestRecordingTraceCapturesEvents(t *testing.T) {
	var tr RecordingTrace
	tr.AttemptDone(querytrace.AttemptEvent{Protocol: "udp"})
	tr.Decision(querytrace.DecisionEvent{Kind: querytrace.DecisionFastFailBlocked})
	tr.Decision(querytrace.DecisionEvent{Kind: querytrace.DecisionSkippedFastFail})
	tr.Decision(querytrace.DecisionEvent{Kind: querytrace.DecisionFastFailBlocked})

	if got := tr.Attempts(); len(got) != 1 || got[0].Protocol != "udp" {
		t.Fatalf("unexpected attempts: %#v", got)
	}
	if got := tr.Decisions(); len(got) != 3 {
		t.Fatalf("expected 3 decisions, got %d", len(got))
	}
	if got := tr.DecisionsOfKind(querytrace.DecisionFastFailBlocked); len(got) != 2 {
		t.Fatalf("expected 2 blocked decisions, got %d", len(got))
	}
	if got := tr.DecisionsOfKind(querytrace.DecisionSkippedFastFail); len(got) != 1 {
		t.Fatalf("expected 1 skipped decision, got %d", len(got))
	}
}

func TestRecordingTraceReturnsCopies(t *testing.T) {
	var tr RecordingTrace
	tr.AttemptDone(querytrace.AttemptEvent{Protocol: "udp"})

	got := tr.Attempts()
	got[0].Protocol = "mutated"
	if again := tr.Attempts(); again[0].Protocol != "udp" {
		t.Fatalf("expected Attempts to return a copy, got %q", again[0].Protocol)
	}
}

func TestRecordingTraceIsConcurrencySafe(t *testing.T) {
	// A real run fires these from several goroutines, so this runs under -race.
	var tr RecordingTrace
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tr.AttemptDone(querytrace.AttemptEvent{Protocol: "udp"})
			tr.Decision(querytrace.DecisionEvent{Kind: querytrace.DecisionFastFailBlocked})
			tr.Attempts()
			tr.DecisionsOfKind(querytrace.DecisionFastFailBlocked)
		}()
	}
	wg.Wait()

	if got := tr.Attempts(); len(got) != 8 {
		t.Fatalf("expected 8 attempts, got %d", len(got))
	}
}
