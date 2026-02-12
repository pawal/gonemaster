package nameserver

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	"codeberg.org/pawal/gonemaster/engine/packet"
)

func TestAdaptiveTimeoutTrackerReductionAfterRepeatedTimeouts(t *testing.T) {
	tracker := &adaptiveTimeoutTracker{}
	base := 4 * time.Second

	if got := tracker.timeoutFor(base, false); got != base {
		t.Fatalf("initial timeout = %v, want %v", got, base)
	}

	tracker.observeResult(false, true)
	if got := tracker.timeoutFor(base, false); got != base {
		t.Fatalf("timeout after first failure = %v, want %v", got, base)
	}

	tracker.observeResult(false, true)
	if got := tracker.timeoutFor(base, false); got != 3*time.Second {
		t.Fatalf("timeout after second failure = %v, want 3s", got)
	}

	tracker.observeResult(false, true)
	tracker.observeResult(false, true)
	if got := tracker.timeoutFor(base, false); got != 2*time.Second {
		t.Fatalf("timeout after fourth failure = %v, want 2s", got)
	}
}

func TestAdaptiveTimeoutTrackerResetConsecutiveOnNonTimeout(t *testing.T) {
	tracker := &adaptiveTimeoutTracker{}
	base := 4 * time.Second

	tracker.observeResult(false, true)
	tracker.observeResult(false, false)
	tracker.observeResult(false, true)
	if got := tracker.timeoutFor(base, false); got != base {
		t.Fatalf("timeout before threshold = %v, want %v", got, base)
	}

	tracker.observeResult(false, true)
	if got := tracker.timeoutFor(base, false); got != 3*time.Second {
		t.Fatalf("timeout after reset + threshold = %v, want 3s", got)
	}
}

func TestAdaptiveTimeoutTrackerProtocolIsolation(t *testing.T) {
	tracker := &adaptiveTimeoutTracker{}
	base := 4 * time.Second

	tracker.observeResult(false, true)
	tracker.observeResult(false, true)

	if got := tracker.timeoutFor(base, false); got != 3*time.Second {
		t.Fatalf("udp timeout = %v, want 3s", got)
	}
	if got := tracker.timeoutFor(base, true); got != base {
		t.Fatalf("tcp timeout = %v, want %v", got, base)
	}
}

func TestReducedTimeoutMinAndStepCap(t *testing.T) {
	base := 600 * time.Millisecond
	if got := reducedTimeout(base, adaptiveTimeoutMaxStep+10); got != adaptiveTimeoutMin {
		t.Fatalf("reduced timeout floor = %v, want %v", got, adaptiveTimeoutMin)
	}
}

func TestIsTimeoutPatternError(t *testing.T) {
	timeoutErr := &net.DNSError{Err: "i/o timeout", IsTimeout: true}
	if !isTimeoutPatternError(timeoutErr) {
		t.Fatalf("expected DNS timeout to match timeout pattern")
	}
	if isTimeoutPatternError(context.Canceled) {
		t.Fatalf("expected context cancellation not to match timeout pattern")
	}
	if isTimeoutPatternError(fmt.Errorf("connection refused")) {
		t.Fatalf("expected non-timeout error not to match timeout pattern")
	}
}

func TestQueryWithOptionsAdaptiveTimeoutDisabledByDefault(t *testing.T) {
	ns, err := New("ns.example", "192.0.2.60", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}
	ctx, prof := testContext(t)
	if prof.Resolver.Defaults.AdaptiveTimeout {
		t.Fatalf("expected adaptive timeout disabled by default profile")
	}

	var sawTimeoutOverride bool
	ns.SetQueryHook(func(_ context.Context, _ string, _ string, _ string, opts *QueryOptions) (packet.Packet, error) {
		if opts != nil && opts.Timeout != nil {
			sawTimeoutOverride = true
		}
		return packet.Packet{}, &net.DNSError{Err: "i/o timeout", IsTimeout: true}
	})

	for i := 0; i < 4; i++ {
		_, _ = ns.QueryWithOptions(ctx, "example.com", "A", nil)
	}
	if sawTimeoutOverride {
		t.Fatalf("expected no timeout override when adaptive timeout is disabled")
	}
}

func TestQueryWithOptionsAdaptiveTimeoutAppliesAfterRepeatedTimeouts(t *testing.T) {
	ns, err := New("ns.example", "192.0.2.61", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}
	ctx, prof := testContext(t)
	prof.Resolver.Defaults.AdaptiveTimeout = true
	prof.Resolver.Defaults.Timeout = 4

	var observed []time.Duration
	ns.SetQueryHook(func(_ context.Context, _ string, _ string, _ string, opts *QueryOptions) (packet.Packet, error) {
		timeout := time.Duration(0)
		if opts != nil && opts.Timeout != nil {
			timeout = *opts.Timeout
		}
		observed = append(observed, timeout)
		return packet.Packet{}, &net.DNSError{Err: "i/o timeout", IsTimeout: true}
	})

	for i := 0; i < 5; i++ {
		_, _ = ns.QueryWithOptions(ctx, "example.com", "A", nil)
	}
	if len(observed) != 5 {
		t.Fatalf("expected 5 calls, got %d", len(observed))
	}
	if observed[0] != 0 || observed[1] != 0 {
		t.Fatalf("expected no timeout override before threshold, got %v", observed[:2])
	}
	if observed[2] != 3*time.Second || observed[3] != 3*time.Second {
		t.Fatalf("expected 3s timeout override after first reduction, got %v", observed[2:4])
	}
	if observed[4] != 2*time.Second {
		t.Fatalf("expected second reduction to 2s, got %v", observed[4])
	}
}

func TestQueryWithOptionsAdaptiveTimeoutIgnoresExplicitTimeout(t *testing.T) {
	ns, err := New("ns.example", "192.0.2.62", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}
	ctx, prof := testContext(t)
	prof.Resolver.Defaults.AdaptiveTimeout = true
	prof.Resolver.Defaults.Timeout = 4

	explicit := 1500 * time.Millisecond
	var observed []time.Duration
	ns.SetQueryHook(func(_ context.Context, _ string, _ string, _ string, opts *QueryOptions) (packet.Packet, error) {
		if opts == nil || opts.Timeout == nil {
			observed = append(observed, 0)
		} else {
			observed = append(observed, *opts.Timeout)
		}
		return packet.Packet{}, &net.DNSError{Err: "i/o timeout", IsTimeout: true}
	})

	for i := 0; i < 4; i++ {
		_, _ = ns.QueryWithOptions(ctx, "example.com", "A", &QueryOptions{Timeout: &explicit})
	}
	_, _ = ns.QueryWithOptions(ctx, "example.com", "A", nil)

	if len(observed) != 5 {
		t.Fatalf("expected 5 calls, got %d", len(observed))
	}
	for i := 0; i < 4; i++ {
		if observed[i] != explicit {
			t.Fatalf("explicit timeout call %d = %v, want %v", i, observed[i], explicit)
		}
	}
	if observed[4] != 0 {
		t.Fatalf("expected no adaptive reduction after explicit-timeout calls, got %v", observed[4])
	}
}

func TestQueryWithOptionsAdaptiveTimeoutProtocolSpecific(t *testing.T) {
	ns, err := New("ns.example", "192.0.2.63", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}
	ctx, prof := testContext(t)
	prof.Resolver.Defaults.AdaptiveTimeout = true
	prof.Resolver.Defaults.Timeout = 4

	type call struct {
		useTCP     bool
		timeout    time.Duration
		hasTimeout bool
	}
	var calls []call
	ns.SetQueryHook(func(_ context.Context, _ string, _ string, _ string, opts *QueryOptions) (packet.Packet, error) {
		c := call{useTCP: resolveUseVC(opts)}
		if opts != nil && opts.Timeout != nil {
			c.hasTimeout = true
			c.timeout = *opts.Timeout
		}
		calls = append(calls, c)
		return packet.Packet{}, &net.DNSError{Err: "i/o timeout", IsTimeout: true}
	})

	_, _ = ns.QueryWithOptions(ctx, "example.com", "A", nil)
	_, _ = ns.QueryWithOptions(ctx, "example.com", "A", nil)

	useTCP := true
	_, _ = ns.QueryWithOptions(ctx, "example.com", "A", &QueryOptions{UseVC: &useTCP})
	_, _ = ns.QueryWithOptions(ctx, "example.com", "A", nil)

	if len(calls) != 4 {
		t.Fatalf("expected 4 calls, got %d", len(calls))
	}
	if calls[2].useTCP != true || calls[2].hasTimeout {
		t.Fatalf("expected TCP path to remain unadapted, got %+v", calls[2])
	}
	if calls[3].useTCP != false || !calls[3].hasTimeout || calls[3].timeout != 3*time.Second {
		t.Fatalf("expected UDP path to be adapted, got %+v", calls[3])
	}
}

func TestQueryWithOptionsAdaptiveTimeoutOnlyCountsTimeoutPattern(t *testing.T) {
	ns, err := New("ns.example", "192.0.2.64", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}
	ctx, prof := testContext(t)
	prof.Resolver.Defaults.AdaptiveTimeout = true
	prof.Resolver.Defaults.Timeout = 4

	var observed []time.Duration
	ns.SetQueryHook(func(_ context.Context, _ string, _ string, _ string, opts *QueryOptions) (packet.Packet, error) {
		timeout := time.Duration(0)
		if opts != nil && opts.Timeout != nil {
			timeout = *opts.Timeout
		}
		observed = append(observed, timeout)
		return packet.Packet{}, fmt.Errorf("connection refused")
	})

	for i := 0; i < 4; i++ {
		_, _ = ns.QueryWithOptions(ctx, "example.com", "A", nil)
	}
	for i, timeout := range observed {
		if timeout != 0 {
			t.Fatalf("call %d timeout override = %v, want none", i, timeout)
		}
	}
}
