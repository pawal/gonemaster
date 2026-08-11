package nameserver

import (
	"context"
	"fmt"
	"net"
	"sync"
	"syscall"
	"testing"
	"time"

	dns "codeberg.org/miekg/dns"

	"codeberg.org/pawal/gonemaster/engine/packet"
)

// These tests drive the reachability cache from many goroutines at once. They
// assert order-independent invariants only: which goroutine wins the race to
// promote the blackout is not defined, so an exact call count would flake.
// Their main job is to give the race detector something to inspect.

func TestReachabilityCacheConcurrentWithinStore(t *testing.T) {
	const goroutines = 8
	const perGoroutine = 10

	store := NewCacheStore()
	addr := "192.0.2.90"
	ctx, prof := testContext(t)
	prof.Resolver.Defaults.NegativeCacheTTL = 60

	var mu sync.Mutex
	liveCalls := 0

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for g := range goroutines {
		go func() {
			defer wg.Done()
			ns, err := NewWithCache(store, fmt.Sprintf("ns%d.example", g), addr, nil)
			if err != nil {
				t.Errorf("new nameserver: %v", err)
				return
			}
			ns.SetQueryHook(func(_ context.Context, _ string, _ string, _ string, _ *QueryOptions) (packet.Packet, error) {
				mu.Lock()
				liveCalls++
				mu.Unlock()
				return packet.Packet{}, &net.OpError{Op: "dial", Net: "udp", Err: syscall.EHOSTUNREACH}
			})
			for q := range perGoroutine {
				// Globally unique qnames, so nothing is short-circuited by the
				// address-keyed query or error caches before the skip ladder.
				_, _ = ns.QueryWithOptions(ctx, fmt.Sprintf("q%d-%d.example", g, q), "A", nil)
			}
		}()
	}
	wg.Wait()

	if skip, _ := store.reachabilityBackoff().shouldSkip(addr); !skip {
		t.Fatalf("the address must end up blacked out after %d hard errors", goroutines*perGoroutine)
	}

	mu.Lock()
	got := liveCalls
	mu.Unlock()
	if got >= goroutines*perGoroutine {
		t.Fatalf("suppression never engaged: %d live calls out of %d queries", got, goroutines*perGoroutine)
	}

	metrics := store.ReachabilityMetrics()
	if metrics.Hits == 0 {
		t.Fatalf("expected suppressed queries to count as hits; got %+v", metrics)
	}
}

// Successes and failures interleaving on the same address exercise the
// mark/observeSuccess/shouldSkip paths against each other. The assertion is
// only that the bookkeeping stays internally consistent.
func TestReachabilityCacheConcurrentMixedResults(t *testing.T) {
	const goroutines = 8

	store := NewCacheStore()
	ctx, prof := testContext(t)
	prof.Resolver.Defaults.NegativeCacheTTL = 60

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for g := range goroutines {
		go func() {
			defer wg.Done()
			ns, err := NewWithCache(store, fmt.Sprintf("ns%d.example", g), "192.0.2.91", nil)
			if err != nil {
				t.Errorf("new nameserver: %v", err)
				return
			}
			failing := g%2 == 0
			ns.SetQueryHook(func(_ context.Context, _ string, _ string, _ string, _ *QueryOptions) (packet.Packet, error) {
				if failing {
					return packet.Packet{}, &net.OpError{Op: "dial", Net: "udp", Err: syscall.EHOSTUNREACH}
				}
				return packet.Packet{Msg: &dns.Msg{}}, nil
			})
			for q := range 10 {
				_, _ = ns.QueryWithOptions(ctx, fmt.Sprintf("m%d-%d.example", g, q), "A", nil)
			}
		}()
	}
	wg.Wait()

	blocked, pending := store.reachabilityBackoff().len()
	if blocked > 1 || pending > 1 {
		t.Fatalf("one address can hold at most one blackout and one pending strike; got blocked=%d pending=%d", blocked, pending)
	}
}

// Clearing the cache while queries are in flight is the interleaving that the
// atomic metrics reset defends: a whole-struct store would race with the
// counter reads happening on the query path.
func TestReachabilityCacheClearDuringQueries(t *testing.T) {
	store := NewCacheStore()
	ctx, prof := testContext(t)
	prof.Resolver.Defaults.NegativeCacheTTL = 60

	backoff := store.reachabilityBackoff()
	stop := make(chan struct{})

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
				backoff.clear()
				_ = backoff.metrics()
				time.Sleep(time.Millisecond)
			}
		}
	}()

	for g := range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ns, err := NewWithCache(store, fmt.Sprintf("ns%d.example", g), "192.0.2.92", nil)
			if err != nil {
				t.Errorf("new nameserver: %v", err)
				return
			}
			ns.SetQueryHook(func(_ context.Context, _ string, _ string, _ string, _ *QueryOptions) (packet.Packet, error) {
				return packet.Packet{}, &net.OpError{Op: "dial", Net: "udp", Err: syscall.EHOSTUNREACH}
			})
			for q := range 20 {
				_, _ = ns.QueryWithOptions(ctx, fmt.Sprintf("c%d-%d.example", g, q), "A", nil)
			}
		}()
	}

	// Let the query goroutines finish, then stop the clearing loop.
	go func() {
		time.Sleep(50 * time.Millisecond)
		close(stop)
	}()
	wg.Wait()
}
