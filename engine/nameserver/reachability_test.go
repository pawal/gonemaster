package nameserver

import (
	"fmt"
	"net/netip"
	"testing"
	"time"
)

// The reachability cache is a two-strike circuit breaker: one hard network
// error records a pending strike, a second one within the same TTL window
// promotes the address to a blackout. These tests exercise the type directly,
// without going through QueryWithOptions, so they pin the bookkeeping the
// integration tests rely on.

func TestReachabilityCacheMarkDebounce(t *testing.T) {
	c := newReachabilityCache()
	addr := "192.0.2.251"

	c.mark(addr, time.Minute)
	if skip, _ := c.shouldSkip(addr); skip {
		t.Fatalf("first mark must not engage skip")
	}

	c.mark(addr, time.Minute)
	if skip, remaining := c.shouldSkip(addr); !skip || remaining <= 0 {
		t.Fatalf("second mark must engage skip with positive remaining; skip=%v remaining=%v", skip, remaining)
	}

	// A later success must not lift a blackout that is already engaged: it
	// only clears pending strikes.
	c.observeSuccess(addr)
	if skip, _ := c.shouldSkip(addr); !skip {
		t.Fatalf("observeSuccess must not clear an already-engaged blackout entry")
	}

	c.clear()
	if skip, _ := c.shouldSkip(addr); skip {
		t.Fatalf("clear must remove the blackout entry")
	}

	// After clear, two marks are again required to re-engage.
	c.mark(addr, time.Minute)
	if skip, _ := c.shouldSkip(addr); skip {
		t.Fatalf("first mark after clear must not engage skip")
	}
	c.mark(addr, time.Minute)
	if skip, _ := c.shouldSkip(addr); !skip {
		t.Fatalf("second mark after clear must engage skip")
	}
}

// Two hard errors farther apart than the TTL window must not promote to a
// blackout. The first failure is forgotten; the second is treated as fresh.
func TestReachabilityCachePendingExpires(t *testing.T) {
	c := newReachabilityCache()
	addr := "192.0.2.250"

	c.mark(addr, 10*time.Millisecond)
	if skip, _ := c.shouldSkip(addr); skip {
		t.Fatalf("first mark must not engage skip")
	}

	// Sleep past the window. A second mark now is "fresh first," not "second."
	time.Sleep(20 * time.Millisecond)

	c.mark(addr, 10*time.Millisecond)
	if skip, _ := c.shouldSkip(addr); skip {
		t.Fatalf("second mark after window expiry must not engage skip")
	}

	// A second mark within the window of the just-recorded pending DOES engage.
	c.mark(addr, 10*time.Millisecond)
	if skip, _ := c.shouldSkip(addr); !skip {
		t.Fatalf("two consecutive marks within window must engage skip")
	}
}

// A success observed between two failures clears the pending strike, so the
// second failure counts as a fresh first strike and the cache stays open.
func TestReachabilityCacheObserveSuccessClearsPending(t *testing.T) {
	c := newReachabilityCache()
	addr := "192.0.2.252"

	c.mark(addr, time.Minute)
	c.observeSuccess(addr)
	c.mark(addr, time.Minute)

	if skip, _ := c.shouldSkip(addr); skip {
		t.Fatalf("success between two marks must reset pending so the cache stays open")
	}
}

// The cache must not grow without bound. When the entry cap is reached, the
// blackout closest to expiring is dropped to make room for the new one.
func TestReachabilityCacheBoundsEntries(t *testing.T) {
	const limit = 4
	c := newReachabilityCacheWithLimit(limit)

	// Six addresses with staggered TTLs, so expiry order matches index order.
	// Two marks each, because promotion needs a second strike.
	addrs := make([]string, 6)
	for i := range addrs {
		addrs[i] = fmt.Sprintf("192.0.2.%d", 10+i)
		ttl := time.Duration(i+1) * time.Minute
		c.mark(addrs[i], ttl)
		c.mark(addrs[i], ttl)
	}

	blocked, _ := c.len()
	if blocked > limit {
		t.Fatalf("blackout entries must stay within the cap; got %d want <= %d", blocked, limit)
	}

	// The two earliest-expiring addresses were evicted to make room.
	for _, addr := range addrs[:2] {
		if skip, _ := c.shouldSkip(addr); skip {
			t.Fatalf("earliest-expiring address %s should have been evicted", addr)
		}
	}
	// The most recent, longest-lived blackout survived.
	if skip, _ := c.shouldSkip(addrs[5]); !skip {
		t.Fatalf("most recently promoted address must still be blacked out")
	}
	if got := c.metrics().Evictions; got < 2 {
		t.Fatalf("evictions must count the dropped entries; got %d want >= 2", got)
	}
}

// clear resets the counters as well as the maps. This is the assertion that
// pins the atomic reset: a whole-struct assignment would race with snapshot.
func TestReachabilityCacheClearResetsMetrics(t *testing.T) {
	c := newReachabilityCache()
	addr := "192.0.2.253"

	c.shouldSkip(addr) // miss
	c.mark(addr, time.Minute)
	c.mark(addr, time.Minute)
	c.shouldSkip(addr) // hit

	if got := c.metrics(); got.Hits == 0 || got.Misses == 0 {
		t.Fatalf("expected non-zero counters before clear; got %+v", got)
	}

	c.clear()
	if got := c.metrics(); got != (CacheMetrics{}) {
		t.Fatalf("clear must zero the counters; got %+v", got)
	}
}

// The reachability rung is the first in the skip ladder and was the only one
// without a nil-state guard. A zero-value Nameserver must skip the mechanism
// rather than panic.
func TestZeroValueNameserverSkipsReachability(t *testing.T) {
	ctx, prof := testContext(t)
	prof.Net.AllowNonGlobalTargets = true

	var ns Nameserver
	ns.Address = netip.MustParseAddr("192.0.2.233")

	// No store, so no state: the call must not panic before it fails.
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("zero-value nameserver must not panic on the reachability rung: %v", r)
		}
	}()
	_, _ = ns.QueryWithOptions(ctx, "example", "A", nil)
}

// Ownership: the cache belongs to the CacheStore that created it. A snapshot
// taken for a run must get its own instance, because the server only ever
// runs on SnapshotForRun-derived stores. Every method on the cache is
// nil-receiver-safe, so a missed initialisation here would not panic - it
// would silently disable backoff on the server path.
func TestSnapshotForRunHasWorkingReachabilityCache(t *testing.T) {
	base := NewCacheStore()
	snap := base.SnapshotForRun()

	backoff := snap.reachabilityBackoff()
	if backoff == nil {
		t.Fatalf("SnapshotForRun must give the run its own reachability cache")
	}

	addr := "192.0.2.230"
	backoff.mark(addr, time.Minute)
	backoff.mark(addr, time.Minute)
	if skip, _ := backoff.shouldSkip(addr); !skip {
		t.Fatalf("the snapshot's cache must engage after two hard errors")
	}
	if got := snap.ReachabilityMetrics(); got.Hits != 1 {
		t.Fatalf("store metrics must report the suppressed query; got %+v", got)
	}
}

// A blackout learned by one store must be invisible to every other store,
// including the base a snapshot was taken from. This is the isolation the
// package global made impossible to express.
func TestCacheStoresGetDistinctReachabilityCaches(t *testing.T) {
	base := NewCacheStore()
	other := NewCacheStore()
	snap := base.SnapshotForRun()

	addr := "192.0.2.231"
	snap.reachabilityBackoff().mark(addr, time.Minute)
	snap.reachabilityBackoff().mark(addr, time.Minute)
	if skip, _ := snap.reachabilityBackoff().shouldSkip(addr); !skip {
		t.Fatalf("the snapshot must have engaged its own blackout")
	}

	for name, store := range map[string]*CacheStore{"base": base, "unrelated": other} {
		if skip, _ := store.reachabilityBackoff().shouldSkip(addr); skip {
			t.Fatalf("%s store must not see a blackout learned by another store", name)
		}
	}

	// A second snapshot of the same base starts clean too.
	if skip, _ := base.SnapshotForRun().reachabilityBackoff().shouldSkip(addr); skip {
		t.Fatalf("a later run must not inherit the previous run's blackout")
	}
}

// Empty flushes cached responses. A blackout is not response data, so it
// survives - and clearing it here would deadlock, since Empty holds the
// store lock for its whole body.
func TestEmptyKeepsReachabilityBlackout(t *testing.T) {
	store := NewCacheStore()
	addr := "192.0.2.232"
	store.reachabilityBackoff().mark(addr, time.Minute)
	store.reachabilityBackoff().mark(addr, time.Minute)

	store.Empty()

	if skip, _ := store.reachabilityBackoff().shouldSkip(addr); !skip {
		t.Fatalf("Empty must not clear the reachability blackout")
	}
}

// A pending strike for an address that is never marked again would otherwise
// sit in the map for the lifetime of the cache. mark sweeps stale entries.
func TestReachabilityCacheMarkSweepsStalePending(t *testing.T) {
	c := newReachabilityCache()

	c.mark("192.0.2.240", 10*time.Millisecond)
	if _, pending := c.len(); pending != 1 {
		t.Fatalf("first mark must record one pending strike; got %d", pending)
	}

	time.Sleep(20 * time.Millisecond)

	// Marking an unrelated address sweeps the stale pending entry.
	c.mark("192.0.2.241", 10*time.Millisecond)
	if _, pending := c.len(); pending != 1 {
		t.Fatalf("stale pending strike must be swept, leaving only the new one; got %d", pending)
	}
	if got := c.metrics().Evictions; got != 1 {
		t.Fatalf("sweep must count one eviction; got %d", got)
	}
}
