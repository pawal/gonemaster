package nameserver

import (
	"context"
	"fmt"
	"testing"
	"time"

	"codeberg.org/pawal/gonemaster/engine/packet"
)

func TestTransportAdaptationAdaptiveTimeoutStabilityAndRecovery(t *testing.T) {
	ns, err := New("ns.example", "192.0.2.206", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}

	ctx, prof := testContext(t)
	prof.Resolver.Defaults.AdaptiveTimeout = true
	prof.Resolver.Defaults.Timeout = 4

	var observed []time.Duration
	call := 0
	ns.SetQueryHook(func(_ context.Context, _ string, _ string, _ string, opts *QueryOptions) (packet.Packet, error) {
		call++
		timeout := time.Duration(0)
		if opts != nil && opts.Timeout != nil {
			timeout = *opts.Timeout
		}
		observed = append(observed, timeout)

		if call <= 4 {
			return packet.Packet{}, fmt.Errorf("timeout")
		}
		return packet.Packet{Msg: nil}, nil
	})

	for i := 0; i < 11; i++ {
		_, _ = ns.QueryWithOptions(ctx, fmt.Sprintf("seq-%d.example.com", i), "A", nil)
	}

	want := []time.Duration{
		0, 0,
		3 * time.Second, 3 * time.Second,
		2 * time.Second, 2 * time.Second, 2 * time.Second,
		3 * time.Second, 3 * time.Second, 3 * time.Second,
		0,
	}
	if len(observed) != len(want) {
		t.Fatalf("observed calls = %d, want %d", len(observed), len(want))
	}
	for i := range want {
		if observed[i] != want[i] {
			t.Fatalf("call %d timeout = %v, want %v", i+1, observed[i], want[i])
		}
	}
}

func TestTransportAdaptationBlacklistBackoffRecoversAfterSuccess(t *testing.T) {
	ns, err := New("ns.example", "192.0.2.207", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}

	ctx, prof := testContext(t)
	prof.Resolver.Defaults.Timeout = 1
	if ns.state == nil {
		t.Fatalf("expected nameserver state")
	}
	ns.state.blacklist.jitterFn = func() float64 { return 0.5 }

	mode := "timeout"
	var calls int
	ns.SetQueryHook(func(_ context.Context, _ string, _ string, _ string, _ *QueryOptions) (packet.Packet, error) {
		calls++
		if mode == "success" {
			return packet.Packet{Msg: nil}, nil
		}
		return packet.Packet{}, fmt.Errorf("timeout")
	})

	runSOABurst := func(prefix string) {
		t.Helper()
		for i := 0; i < blacklistTimeoutFailureThreshold; i++ {
			_, _ = ns.QueryWithOptions(ctx, fmt.Sprintf("%s-%d.example.com", prefix, i), "SOA", nil)
		}
	}
	state := ns.state.blacklist.stateForProtocol(false)

	runSOABurst("burst1")
	if state.backoffStep != 1 {
		t.Fatalf("backoff step after first burst = %d, want 1", state.backoffStep)
	}

	state.blockedUntil = time.Now().Add(-time.Millisecond)
	runSOABurst("burst2")
	if state.backoffStep != 2 {
		t.Fatalf("backoff step after second burst = %d, want 2", state.backoffStep)
	}

	state.blockedUntil = time.Now().Add(-time.Millisecond)
	mode = "success"
	_, err = ns.QueryWithOptions(ctx, "recovery.example.com", "A", nil)
	if err != nil {
		t.Fatalf("recovery query: %v", err)
	}
	if state.backoffStep != 1 {
		t.Fatalf("backoff step after success recovery = %d, want 1", state.backoffStep)
	}

	mode = "timeout"
	state.blockedUntil = time.Now().Add(-time.Millisecond)
	runSOABurst("burst3")
	if state.backoffStep != 2 {
		t.Fatalf("backoff step after recovery + new burst = %d, want 2", state.backoffStep)
	}
	if calls < 7 {
		t.Fatalf("expected network hook calls across bursts and recovery, got %d", calls)
	}
}
