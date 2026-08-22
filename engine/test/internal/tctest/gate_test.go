package tctest

import (
	"codeberg.org/pawal/gonemaster/internal/tbtest"
	"testing"
	"testing/synctest"
)

func TestGateHoldsArrivalsUntilRelease(t *testing.T) {
	// The bubble makes "both goroutines are parked in the gate" observable:
	// synctest.Wait returns only once every other goroutine blocks durably.
	synctest.Test(t, func(t *testing.T) {
		g := NewGate()
		passed := map[string]chan struct{}{}
		for _, id := range []string{"ns1", "ns2"} {
			through := make(chan struct{})
			passed[id] = through
			go func() {
				g.Arrive(id)
				close(through)
			}()
		}

		synctest.Wait()
		g.RequireInFlight(t, "ns1", "ns2")
		for id, through := range passed {
			select {
			case <-through:
				t.Fatalf("expected %s to stay blocked before Release", id)
			default:
			}
		}

		g.Release()
		synctest.Wait()
		for id, through := range passed {
			select {
			case <-through:
			default:
				t.Fatalf("expected %s to be released", id)
			}
		}
	})
}

func TestGateArriveAfterReleaseDoesNotBlock(t *testing.T) {
	g := NewGate()
	g.Release()
	g.Arrive("late")
	if got := g.InFlight(); len(got) != 1 || got[0] != "late" {
		t.Fatalf("expected the late arrival to be recorded, got %v", got)
	}
}

func TestGateReleaseIsIdempotent(t *testing.T) {
	g := NewGate()
	g.Release()
	g.Release()
}

func TestGateRecordDoesNotBlock(t *testing.T) {
	g := NewGate()
	g.Record("a")
	g.Record("b")
	g.Record("a")
	if got := g.InFlight(); len(got) != 3 {
		t.Fatalf("expected three recorded arrivals in order, got %v", got)
	}
	g.RequireInFlight(t, "a", "b")
}

func TestGateInFlightIsACopy(t *testing.T) {
	g := NewGate()
	g.Record("a")
	got := g.InFlight()
	got[0] = "mutated"
	if again := g.InFlight(); again[0] != "a" {
		t.Fatalf("expected InFlight to return a copy, got %v", again)
	}
}

func TestGateConcurrentRecordIsRaceFree(t *testing.T) {
	// Runs under -race to cover the mutex around the arrival slice.
	g := NewGate()
	done := make(chan struct{})
	for i := 0; i < 8; i++ {
		go func() {
			g.Record("id")
			g.InFlight()
			close(done)
		}()
		<-done
		done = make(chan struct{})
	}
	if got := g.InFlight(); len(got) != 8 {
		t.Fatalf("expected 8 recorded arrivals, got %v", got)
	}
}

func TestGateRequireInFlightReportsMismatch(t *testing.T) {
	g := NewGate()
	g.Record("ns1")
	tbtest.MustFail(t, "expected parallel queries from [ns1 ns2], got [ns1]", func(tb *tbtest.TB) {
		g.RequireInFlight(tb, "ns1", "ns2")
	})
}
