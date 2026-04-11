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

