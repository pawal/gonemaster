package nameserver

import (
	"context"
	"fmt"
	"net"
	"syscall"
	"testing"
	"testing/synctest"
	"time"

	"codeberg.org/pawal/gonemaster/engine/packet"
)

// A diagnostic query is one that is expected to fail sometimes: a probe that
// deliberately asks for something the path may not deliver. Its failure says
// nothing about the server, so it must leave every piece of server-health
// bookkeeping untouched. Each test below runs the identical scenario twice,
// with Diagnostic set and unset, so the assertion is that the flag - and
// nothing else - is what makes the difference.

// timeoutHookErr is what a UDP probe that never gets an answer looks like.
func timeoutHookErr() error {
	return &net.OpError{Op: "read", Net: "udp", Err: context.DeadlineExceeded}
}

// hardHookErr is a hard network error, the kind that marks reachability.
func hardHookErr() error {
	return &net.OpError{Op: "dial", Net: "udp", Err: syscall.EHOSTUNREACH}
}

func TestDiagnosticQuerySuppressesTimeoutBookkeeping(t *testing.T) {
	for _, tc := range []struct {
		name       string
		diagnostic bool
		// want* describe the state after one timed-out query.
		wantTimeouts    int
		wantFastFail    bool
		wantErrorCached bool
	}{
		{"normal query records the timeout", false, 1, true, true},
		{"diagnostic query records nothing", true, 0, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, prof := testContext(t)
			// Threshold 1 so a single timeout is enough to engage fast-fail,
			// which in turn is what lets the error cache write through.
			prof.Resolver.Defaults.FastFailTimeoutCount = 1
			prof.Resolver.Defaults.ErrorCacheTTL = 60
			store := CacheFromContext(ctx)

			ns := newNS(t, ctx, "ns.example", "192.0.2.60")
			var calls int
			ns.SetQueryHook(func(_ context.Context, _ string, _ string, _ string, _ *QueryOptions) (packet.Packet, error) {
				calls++
				return packet.Packet{}, timeoutHookErr()
			})

			opts := &QueryOptions{BlacklistingDisabled: true, Diagnostic: tc.diagnostic}
			if _, err := ns.QueryWithOptions(ctx, "a.example", "A", opts); err == nil {
				t.Fatalf("expected the hook error to surface")
			}
			if calls != 1 {
				t.Fatalf("hook calls = %d, want 1", calls)
			}

			timeoutKey := ns.NameString() + "/" + ns.AddressString()
			if got := store.QueryTimeouts()[timeoutKey]; got != tc.wantTimeouts {
				t.Fatalf("timeout count = %d, want %d", got, tc.wantTimeouts)
			}
			if got := ns.state.fastFail.shouldSkip(false, 1); got != tc.wantFastFail {
				t.Fatalf("fast-fail engaged = %v, want %v", got, tc.wantFastFail)
			}

			cacheKey, _, _, err := buildCacheKey("a.example", "A", "IN", opts)
			if err != nil {
				t.Fatalf("buildCacheKey: %v", err)
			}
			got, _ := ns.state.errorCache.shouldSkip(cacheKey)
			if got != tc.wantErrorCached {
				t.Fatalf("error cached = %v, want %v", got, tc.wantErrorCached)
			}
		})
	}
}

func TestDiagnosticQuerySuppressesReachabilityMarking(t *testing.T) {
	for _, tc := range []struct {
		name        string
		diagnostic  bool
		wantBlocked bool
	}{
		{"normal query marks the address", false, true},
		{"diagnostic query leaves the address alone", true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, prof := testContext(t)
			prof.Resolver.Defaults.FastFailTimeoutCount = 0
			prof.Resolver.Defaults.ErrorCacheTTL = 0
			prof.Resolver.Defaults.NegativeCacheTTL = 60

			addr := "192.0.2.61"
			ns := newNS(t, ctx, "ns.example", addr)
			ns.SetQueryHook(func(_ context.Context, _ string, _ string, _ string, _ *QueryOptions) (packet.Packet, error) {
				return packet.Packet{}, hardHookErr()
			})

			// Reachability is a two-strike breaker, so two hard errors are
			// needed. Distinct qnames keep the per-query cache out of the way.
			opts := &QueryOptions{BlacklistingDisabled: true, Diagnostic: tc.diagnostic}
			for _, qname := range []string{"a.example", "b.example"} {
				_, _ = ns.QueryWithOptions(ctx, qname, "A", opts)
			}

			blocked, _ := ns.state.reachability.shouldSkip(addr)
			if blocked != tc.wantBlocked {
				t.Fatalf("reachability blackout = %v, want %v", blocked, tc.wantBlocked)
			}
		})
	}
}

func TestDiagnosticQuerySuppressesLatencyBudget(t *testing.T) {
	for _, tc := range []struct {
		name       string
		diagnostic bool
		wantCalls  int
	}{
		// Each hook call costs 60ms of the 100ms budget, so a normal query
		// crosses it on the second call and the third is skipped.
		{"normal query spends the budget", false, 2},
		{"diagnostic query spends nothing", true, 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				ctx, prof := testContext(t)
				prof.Resolver.Defaults.NameserverMaxTotalMS = 100
				prof.Resolver.Defaults.FastFailTimeoutCount = 0
				prof.Resolver.Defaults.ErrorCacheTTL = 0

				ns := newNS(t, ctx, "ns.example", "192.0.2.62")
				var calls int
				ns.SetQueryHook(func(_ context.Context, _ string, _ string, _ string, _ *QueryOptions) (packet.Packet, error) {
					calls++
					time.Sleep(60 * time.Millisecond)
					return packet.Packet{}, fmt.Errorf("read timeout")
				})

				opts := &QueryOptions{BlacklistingDisabled: true, Diagnostic: tc.diagnostic}
				for _, qname := range []string{"a.example", "b.example", "c.example"} {
					_, _ = ns.QueryWithOptions(ctx, qname, "A", opts)
				}
				if calls != tc.wantCalls {
					t.Fatalf("hook calls = %d, want %d", calls, tc.wantCalls)
				}
			})
		})
	}
}

// The negative cache marker is deliberately not suppressed: a --save/--restore
// replay must reproduce the probe's silence instead of re-issuing it live.
func TestDiagnosticQueryStillWritesNegativeCacheMarker(t *testing.T) {
	ctx, prof := testContext(t)
	prof.Resolver.Defaults.FastFailTimeoutCount = 0
	prof.Resolver.Defaults.ErrorCacheTTL = 0

	ns := newNS(t, ctx, "ns.example", "192.0.2.63")
	var calls int
	ns.SetQueryHook(func(_ context.Context, _ string, _ string, _ string, _ *QueryOptions) (packet.Packet, error) {
		calls++
		return packet.Packet{}, timeoutHookErr()
	})

	opts := &QueryOptions{BlacklistingDisabled: true, Diagnostic: true}
	_, _ = ns.QueryWithOptions(ctx, "a.example", "A", opts)

	cacheKey, _, _, err := buildCacheKey("a.example", "A", "IN", opts)
	if err != nil {
		t.Fatalf("buildCacheKey: %v", err)
	}
	cached, ok := ns.state.cache.get(cacheKey)
	if !ok {
		t.Fatalf("expected a cache entry for the failed diagnostic query")
	}
	if cached != nil {
		t.Fatalf("expected a no-response marker, got %+v", cached)
	}

	// The replay must come from the marker, not from a second network attempt.
	if _, err := ns.QueryWithOptions(ctx, "a.example", "A", opts); err != nil {
		t.Fatalf("replayed marker returned an error: %v", err)
	}
	if calls != 1 {
		t.Fatalf("hook calls = %d, want 1", calls)
	}
}

// Diagnostic must stay out of the cache key: the probe is kept apart from the
// functional queries by its transport overrides, not by the flag.
func TestDiagnosticIsNotPartOfCacheKey(t *testing.T) {
	t.Parallel()

	plain, _, _, err := buildCacheKey("example.com", "DNSKEY", "IN", &QueryOptions{BlacklistingDisabled: true})
	if err != nil {
		t.Fatalf("buildCacheKey: %v", err)
	}
	diag, _, _, err := buildCacheKey("example.com", "DNSKEY", "IN", &QueryOptions{BlacklistingDisabled: true, Diagnostic: true})
	if err != nil {
		t.Fatalf("buildCacheKey: %v", err)
	}
	if plain != diag {
		t.Fatalf("Diagnostic changed the cache key:\n plain: %s\n  diag: %s", plain, diag)
	}
}
