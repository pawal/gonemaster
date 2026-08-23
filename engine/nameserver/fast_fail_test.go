package nameserver

import (
	"context"
	"errors"
	"fmt"
	"net"
	"syscall"
	"testing"

	"codeberg.org/pawal/gonemaster/engine/internal/dnstest"
	"codeberg.org/pawal/gonemaster/engine/packet"
	"codeberg.org/pawal/gonemaster/engine/querytrace"
)

func TestIsTimeoutPatternErrorClassifies(t *testing.T) {
	dialDeadlineExceeded := &net.OpError{
		Op:  "dial",
		Net: "tcp",
		Err: context.DeadlineExceeded,
	}
	if !errors.Is(dialDeadlineExceeded, context.DeadlineExceeded) {
		t.Fatalf("test fixture invariant: dialDeadlineExceeded must wrap context.DeadlineExceeded")
	}
	if !dialDeadlineExceeded.Timeout() {
		t.Fatalf("test fixture invariant: dialDeadlineExceeded must report Timeout() == true")
	}

	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"plain timeout string", fmt.Errorf("read timeout"), true},
		{"net.OpError wrapping deadline-exceeded (real dial timeout)", dialDeadlineExceeded, true},
		{"raw context.DeadlineExceeded", context.DeadlineExceeded, true},
		{"raw context.Canceled", context.Canceled, false},
		{"net.OpError with non-timeout error", &net.OpError{Op: "read", Net: "udp", Err: syscall.ECONNREFUSED}, false},
		{"net.OpError EHOSTUNREACH", &net.OpError{Op: "dial", Net: "udp", Err: syscall.EHOSTUNREACH}, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := isTimeoutPatternError(tc.err)
			if got != tc.want {
				t.Fatalf("isTimeoutPatternError(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

func TestIsHardNetworkErrorClassifies(t *testing.T) {
	dialDeadlineExceeded := &net.OpError{
		Op:  "dial",
		Net: "tcp",
		Err: context.DeadlineExceeded,
	}

	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"context.Canceled", context.Canceled, false},
		{"raw context.DeadlineExceeded", context.DeadlineExceeded, false},
		{"dial-timeout (wraps DeadlineExceeded)", dialDeadlineExceeded, false},
		{"EHOSTUNREACH wrapped in OpError", &net.OpError{Op: "dial", Net: "udp", Err: syscall.EHOSTUNREACH}, true},
		{"ENETUNREACH wrapped in OpError", &net.OpError{Op: "dial", Net: "udp", Err: syscall.ENETUNREACH}, true},
		{"plain ECONNREFUSED is not hard", syscall.ECONNREFUSED, false},
		{"no route to host string", fmt.Errorf("dial udp 192.0.2.1: no route to host"), true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := isHardNetworkError(tc.err)
			if got != tc.want {
				t.Fatalf("isHardNetworkError(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

func TestFastFailTrackerThreshold(t *testing.T) {
	tracker := &fastFailTracker{}
	threshold := 3

	tracker.observeResult(false, true, threshold)
	tracker.observeResult(false, true, threshold)
	if tracker.shouldSkip(false, threshold) {
		t.Fatalf("expected no fast-fail skip before threshold")
	}

	tracker.observeResult(false, true, threshold)
	if !tracker.shouldSkip(false, threshold) {
		t.Fatalf("expected fast-fail skip after threshold")
	}
}

func TestFastFailTrackerDisabledWhenThresholdZero(t *testing.T) {
	tracker := &fastFailTracker{}
	tracker.observeResult(false, true, 0)
	if tracker.shouldSkip(false, 0) {
		t.Fatalf("expected fast-fail disabled when threshold is zero")
	}
}

func TestFastFailTrackerResetsOnSuccess(t *testing.T) {
	tracker := &fastFailTracker{}
	threshold := 2

	tracker.observeResult(false, true, threshold)
	tracker.observeResult(false, true, threshold)
	if !tracker.shouldSkip(false, threshold) {
		t.Fatalf("expected fast-fail skip after threshold")
	}

	tracker.observeResult(false, false, threshold)
	if tracker.shouldSkip(false, threshold) {
		t.Fatalf("expected success to clear fast-fail skip state")
	}
}

func TestFastFailTrackerProtocolIsolation(t *testing.T) {
	tracker := &fastFailTracker{}
	threshold := 2

	tracker.observeResult(false, true, threshold)
	tracker.observeResult(false, true, threshold)
	if !tracker.shouldSkip(false, threshold) {
		t.Fatalf("expected UDP skip after threshold")
	}
	if tracker.shouldSkip(true, threshold) {
		t.Fatalf("expected TCP skip state unaffected by UDP failures")
	}
}

// TestFastFailEngagesOnDialTimeouts is a regression test for the bug where
// dial timeouts wrapping context.DeadlineExceeded were misclassified as
// "not a timeout" and the fast-fail counter never advanced, so the engine
// would re-pay the full dial timeout for every test case against a dead host.
func TestFastFailEngagesOnDialTimeouts(t *testing.T) {
	ctx, prof := testContext(t)
	prof.Resolver.Defaults.FastFailTimeoutCount = 3
	// Disable error cache so this test isolates the fast-fail mechanism;
	// otherwise the ERROR_CACHE_SKIP path short-circuits before fast-fail
	// can observe enough timeouts to engage.
	prof.Resolver.Defaults.ErrorCacheTTL = 0
	// Use BlacklistingDisabled to isolate fast-fail from the SOA-blacklist path.
	opts := &QueryOptions{BlacklistingDisabled: true}

	ns := newNS(t, ctx, "ns.example", "192.0.2.240")

	var calls int
	ns.SetQueryHook(func(_ context.Context, _ string, _ string, _ string, _ *QueryOptions) (packet.Packet, error) {
		calls++
		return packet.Packet{}, &net.OpError{Op: "dial", Net: "udp", Err: context.DeadlineExceeded}
	})

	// Three failing queries (different qnames so the per-query cache does
	// not short-circuit on the second-and-later calls) should trip fast-fail.
	for i, qname := range []string{"a.example", "b.example", "c.example"} {
		_, _ = ns.QueryWithOptions(ctx, qname, "A", opts)
		if got, want := calls, i+1; got != want {
			t.Fatalf("after query %d: hook calls = %d, want %d", i+1, got, want)
		}
	}

	// The fourth query is to a fresh qname (cache miss). Fast-fail should now
	// short-circuit and the hook must NOT be invoked.
	_, _ = ns.QueryWithOptions(ctx, "d.example", "A", opts)
	if calls != 3 {
		t.Fatalf("expected fast-fail to suppress the 4th network call, got %d hook calls", calls)
	}
}

// TestFastFailIgnoresOuterContextCancellation ensures we do not blame the
// nameserver when the outer context is the thing that got cancelled.
func TestFastFailIgnoresOuterContextCancellation(t *testing.T) {
	ctx, prof := testContext(t)
	prof.Resolver.Defaults.FastFailTimeoutCount = 2
	prof.Resolver.Defaults.ErrorCacheTTL = 0
	opts := &QueryOptions{BlacklistingDisabled: true}

	ns := newNS(t, ctx, "ns.example", "192.0.2.241")

	ns.SetQueryHook(func(_ context.Context, _ string, _ string, _ string, _ *QueryOptions) (packet.Packet, error) {
		return packet.Packet{}, context.DeadlineExceeded
	})

	// Run two queries with an already-cancelled outer context. Even though
	// the err is a "timeout pattern", fast-fail must not engage.
	for _, qname := range []string{"a.example", "b.example"} {
		cctx, cancel := context.WithCancel(ctx)
		cancel()
		_, _ = ns.QueryWithOptions(cctx, qname, "A", opts)
	}

	if ns.state.fastFail.shouldSkip(false, 2) {
		t.Fatalf("fast-fail must not engage on outer-context cancellation")
	}
}

// TestQueryEmitsFastFailDecision is the step-4 verification. With fast-fail set
// to engage after 3 consecutive timeouts, the run's QueryTrace must see exactly
// one DecisionFastFailBlocked event - emitted on the threshold-tripping query -
// and the next query (a fresh cache miss) must be suppressed by fast-fail,
// emitting a DecisionSkippedFastFail rather than issuing a network attempt. The
// error cache and blacklist paths are disabled so only fast-fail can fire,
// matching the isolation in TestFastFailEngagesOnDialTimeouts above.
func TestQueryEmitsFastFailDecision(t *testing.T) {
	ctx, prof := testContext(t)
	prof.Resolver.Defaults.FastFailTimeoutCount = 3
	prof.Resolver.Defaults.ErrorCacheTTL = 0
	opts := &QueryOptions{BlacklistingDisabled: true}

	rec := &dnstest.RecordingTrace{}
	ctx = querytrace.WithContext(ctx, rec)

	ns := newNS(t, ctx, "ns.example", "192.0.2.242")

	var calls int
	ns.SetQueryHook(func(_ context.Context, _ string, _ string, _ string, _ *QueryOptions) (packet.Packet, error) {
		calls++
		return packet.Packet{}, &net.OpError{Op: "dial", Net: "udp", Err: context.DeadlineExceeded}
	})

	// Three failing queries with distinct qnames (so the per-query cache does
	// not short-circuit) trip fast-fail on the third.
	for _, qname := range []string{"a.example", "b.example", "c.example"} {
		_, _ = ns.QueryWithOptions(ctx, qname, "A", opts)
	}
	if got := rec.DecisionsOfKind(querytrace.DecisionFastFailBlocked); len(got) != 1 {
		t.Fatalf("expected exactly 1 fast-fail block decision after 3 timeouts, got %d: %+v", len(got), got)
	}
	if calls != 3 {
		t.Fatalf("expected 3 hook calls before fast-fail engages, got %d", calls)
	}

	// The fourth query is a fresh cache miss; fast-fail must suppress it - no
	// new hook call, and a skipped-fast-fail decision instead.
	_, _ = ns.QueryWithOptions(ctx, "d.example", "A", opts)
	if calls != 3 {
		t.Fatalf("expected fast-fail to suppress the 4th network call, got %d hook calls", calls)
	}
	if got := rec.DecisionsOfKind(querytrace.DecisionSkippedFastFail); len(got) == 0 {
		t.Fatalf("expected a skipped-fast-fail decision on the suppressed query, got none")
	}
}
