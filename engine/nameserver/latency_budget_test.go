package nameserver

import (
	"context"
	"fmt"
	"testing"
	"time"

	"codeberg.org/pawal/gonemaster/engine/packet"
	"codeberg.org/pawal/gonemaster/engine/querytrace"
)

// TestLatencyTrackerBudget exercises the tracker in isolation: it must stay open
// while the cumulative elapsed is under budget, report exactly one transition on
// the call that crosses it, then stay blocked without reporting further
// transitions. The "exactly one transition" contract is what lets the caller
// emit a single DecisionLatencyBudgetBlocked event per address.
func TestLatencyTrackerBudget(t *testing.T) {
	var tr latencyTracker
	budget := 100 * time.Millisecond

	if tr.observe(40*time.Millisecond, budget) {
		t.Fatal("must not block below budget")
	}
	if tr.shouldSkip(budget) {
		t.Fatal("must not skip below budget")
	}
	if !tr.observe(70*time.Millisecond, budget) { // cumulative 110ms >= 100ms
		t.Fatal("expected a block transition when cumulative crosses budget")
	}
	if !tr.shouldSkip(budget) {
		t.Fatal("expected skip after the budget was crossed")
	}
	if tr.observe(50*time.Millisecond, budget) {
		t.Fatal("a second observe after blocking must not report another transition")
	}
}

// TestLatencyTrackerDisabledWhenBudgetZero confirms a zero/negative budget fully
// disables the tracker, so the feature is off-by-default and free.
func TestLatencyTrackerDisabledWhenBudgetZero(t *testing.T) {
	var tr latencyTracker
	if tr.observe(time.Hour, 0) {
		t.Fatal("budget 0 must disable the tracker")
	}
	if tr.shouldSkip(0) {
		t.Fatal("budget 0 must never skip")
	}
}

// TestQuerySkipsAddressOverLatencyBudget is the step-10 verification end to end:
// once the cumulative time spent on one nameserver address crosses the budget,
// further queries to it are skipped for the rest of the run, with a single
// DecisionLatencyBudgetBlocked and a DecisionSkippedLatencyBudget on the
// suppressed query. Fast-fail and the error cache are disabled so only the
// latency budget can fire - and the hook returns errors, so this also proves the
// budget catches a server that never produces a usable answer.
func TestQuerySkipsAddressOverLatencyBudget(t *testing.T) {
	ctx, prof := testContext(t)
	// 100ms budget against 60ms sleeps leaves 40ms of headroom on the first
	// query. The earlier 30ms/20ms pairing left only 10ms, which is not enough
	// under the race detector on a loaded CI runner: one slow first query would
	// cross the budget on its own and suppress the second.
	prof.Resolver.Defaults.NameserverMaxTotalMS = 100
	prof.Resolver.Defaults.FastFailTimeoutCount = 0
	prof.Resolver.Defaults.ErrorCacheTTL = 0
	opts := &QueryOptions{BlacklistingDisabled: true}

	rec := &recordingTrace{}
	ctx = querytrace.WithContext(ctx, rec)

	ns, err := NewWithContext(ctx, "ns.example", "192.0.2.250", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}

	var calls int
	ns.SetQueryHook(func(_ context.Context, _ string, _ string, _ string, _ *QueryOptions) (packet.Packet, error) {
		calls++
		time.Sleep(60 * time.Millisecond) // ~60ms each; two queries (~120ms) cross the 100ms budget
		return packet.Packet{}, fmt.Errorf("read timeout")
	})

	// Distinct qnames so the per-query cache does not short-circuit. The first
	// two queries run; their ~120ms total crosses 100ms, so the third is skipped.
	for _, qname := range []string{"a.example", "b.example", "c.example"} {
		_, _ = ns.QueryWithOptions(ctx, qname, "A", opts)
	}

	if calls != 2 {
		t.Fatalf("expected the latency budget to suppress the 3rd network call, got %d hook calls", calls)
	}
	if got := rec.decisionsOfKind(querytrace.DecisionLatencyBudgetBlocked); len(got) != 1 {
		t.Fatalf("expected exactly 1 latency-budget block decision, got %d: %+v", len(got), got)
	}
	if got := rec.decisionsOfKind(querytrace.DecisionSkippedLatencyBudget); len(got) == 0 {
		t.Fatalf("expected a skipped-latency-budget decision on the suppressed query, got none")
	}
}
