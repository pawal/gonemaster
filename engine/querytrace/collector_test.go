package querytrace

import (
	"testing"
	"time"
)

// TestCollectorAggregatesPerNameserver feeds a mix of attempt outcomes and a
// decision for two nameservers and checks that per-address counts, the timeout
// tally, accumulated elapsed time, and the run-wide Totals all line up. This is
// the data the step-6 summary table is built from, so the aggregation contract
// is pinned here.
func TestCollectorAggregatesPerNameserver(t *testing.T) {
	c := NewCollector()

	// nsA: two attempts, one OK one timeout.
	c.AttemptDone(AttemptEvent{NSAddr: "192.0.2.1", QType: "SOA", Protocol: "udp", Attempt: 1, Elapsed: 10 * time.Millisecond, Outcome: OutcomeOK})
	c.AttemptDone(AttemptEvent{NSAddr: "192.0.2.1", QType: "A", Protocol: "udp", Attempt: 1, Elapsed: 3 * time.Second, Outcome: OutcomeTimeout})
	c.Decision(DecisionEvent{Kind: DecisionFastFailBlocked, NSAddr: "192.0.2.1", NSName: "ns.example"})

	// nsB: one error.
	c.AttemptDone(AttemptEvent{NSAddr: "192.0.2.2", QType: "A", Protocol: "tcp", Attempt: 1, Elapsed: 5 * time.Millisecond, Outcome: OutcomeError})

	attempts, timeouts, nsCount := c.Totals()
	if attempts != 3 {
		t.Errorf("Totals attempts = %d, want 3", attempts)
	}
	if timeouts != 1 {
		t.Errorf("Totals timeouts = %d, want 1", timeouts)
	}
	if nsCount != 2 {
		t.Errorf("Totals nameservers = %d, want 2", nsCount)
	}

	a := c.ns["192.0.2.1"]
	if a == nil {
		t.Fatal("missing stats for 192.0.2.1")
	}
	if a.Attempts != 2 || a.Timeouts != 1 || a.Errors != 0 {
		t.Errorf("nsA stats = {attempts:%d timeouts:%d errors:%d}, want {2 1 0}", a.Attempts, a.Timeouts, a.Errors)
	}
	if want := 10*time.Millisecond + 3*time.Second; a.TotalElapsed != want {
		t.Errorf("nsA TotalElapsed = %v, want %v", a.TotalElapsed, want)
	}
	if a.Name != "ns.example" {
		t.Errorf("nsA Name = %q, want it backfilled from the decision event", a.Name)
	}
	if a.Decisions[DecisionFastFailBlocked] != 1 {
		t.Errorf("nsA fast-fail decisions = %d, want 1", a.Decisions[DecisionFastFailBlocked])
	}

	b := c.ns["192.0.2.2"]
	if b == nil || b.Errors != 1 || b.Timeouts != 0 {
		t.Errorf("nsB stats = %+v, want 1 error and 0 timeouts", b)
	}
}

// TestCollectorStatsSortedByElapsed verifies Stats returns nameservers ordered
// by attributable time, slowest first - the ordering the CLI table relies on to
// surface the slow servers at the top.
func TestCollectorStatsSortedByElapsed(t *testing.T) {
	c := NewCollector()
	c.AttemptDone(AttemptEvent{NSAddr: "fast", Elapsed: 5 * time.Millisecond, Outcome: OutcomeOK})
	c.AttemptDone(AttemptEvent{NSAddr: "slow", Elapsed: 12 * time.Second, Outcome: OutcomeTimeout})
	c.AttemptDone(AttemptEvent{NSAddr: "mid", Elapsed: 300 * time.Millisecond, Outcome: OutcomeOK})

	stats := c.Stats()
	if len(stats) != 3 {
		t.Fatalf("Stats len = %d, want 3", len(stats))
	}
	if stats[0].Addr != "slow" || stats[1].Addr != "mid" || stats[2].Addr != "fast" {
		t.Fatalf("Stats order = [%s %s %s], want [slow mid fast]", stats[0].Addr, stats[1].Addr, stats[2].Addr)
	}
}
