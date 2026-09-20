package nameserver

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"testing/synctest"
	"time"

	dns "codeberg.org/miekg/dns"
	"codeberg.org/miekg/dns/dnsutil"

	"codeberg.org/pawal/gonemaster/engine/dnsname"
	"codeberg.org/pawal/gonemaster/engine/internal/dnstest"
	"codeberg.org/pawal/gonemaster/engine/logger"
	"codeberg.org/pawal/gonemaster/engine/packet"
	"codeberg.org/pawal/gonemaster/engine/profile"
	"codeberg.org/pawal/gonemaster/engine/transport"
)

func TestFakeDSResponse(t *testing.T) {
	ns := newCacheNS(t, NewCacheStore(), "ns.example", "192.0.2.1")

	err := ns.AddFakeDS("example", []DSData{{
		KeyTag:     1234,
		Algorithm:  8,
		DigestType: 2,
		Digest:     "ABCD",
	}})
	if err != nil {
		t.Fatalf("add fake DS: %v", err)
	}

	resp, err := ns.QueryWithOptions(context.Background(), "example", "DS", nil)
	if err != nil {
		t.Fatalf("query fake DS: %v", err)
	}
	if resp.Msg == nil || len(resp.Msg.Answer) != 1 {
		t.Fatalf("expected DS answer, got %#v", resp.Msg)
	}
	if _, ok := resp.Msg.Answer[0].(*dns.DS); !ok {
		t.Fatalf("expected DS record, got %T", resp.Msg.Answer[0])
	}
	if !resp.Msg.Authoritative {
		t.Fatalf("expected authoritative response")
	}
}

func TestFakeDelegationNS(t *testing.T) {
	ns := newCacheNS(t, NewCacheStore(), "ns.example", "192.0.2.1")

	err := ns.AddFakeDelegation("example", map[string][]string{
		"ns1.example.": {"192.0.2.2"},
	})
	if err != nil {
		t.Fatalf("add fake delegation: %v", err)
	}

	resp, err := ns.QueryWithOptions(context.Background(), "example", "NS", nil)
	if err != nil {
		t.Fatalf("query fake delegation: %v", err)
	}
	if resp.Msg == nil || len(resp.Msg.Answer) == 0 {
		t.Fatalf("expected delegation answer, got %#v", resp.Msg)
	}
	if _, ok := resp.Msg.Answer[0].(*dns.NS); !ok {
		t.Fatalf("expected NS record, got %T", resp.Msg.Answer[0])
	}
	if len(resp.Msg.Extra) == 0 {
		t.Fatalf("expected glue in additional section")
	}
}

func TestQueryCacheHit(t *testing.T) {
	cache := NewCacheStore()
	ns := newCacheNS(t, cache, "ns.example", "192.0.2.10")

	var calls int
	ns.SetQueryHook(func(_ context.Context, _ string, _ string, _ string, _ *QueryOptions) (packet.Packet, error) {
		calls++
		msg := new(dns.Msg)
		msg.Rcode = dns.RcodeSuccess
		aRR := &dns.A{Hdr: dns.Header{Name: "example.", Class: dns.ClassINET, TTL: 60}}
		aRR.Addr = netip.AddrFrom4([4]byte{192, 0, 2, 5})
		msg.Answer = []dns.RR{aRR}
		return packet.Packet{Msg: msg}, nil
	})

	_, err := ns.QueryWithOptions(context.Background(), "example", "A", nil)
	if err != nil {
		t.Fatalf("query 1: %v", err)
	}
	_, err = ns.QueryWithOptions(context.Background(), "example", "A", nil)
	if err != nil {
		t.Fatalf("query 2: %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected 1 call, got %d", calls)
	}

	metrics := cache.QueryMetrics()
	if metrics.Hits != 1 || metrics.Misses != 1 || metrics.Evictions != 0 {
		t.Fatalf("unexpected query cache metrics: %+v", metrics)
	}
}

func TestInflightQueryCoalescing(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		cache := NewCacheStore()
		ns := newCacheNS(t, cache, "ns.example", "192.0.2.51")

		ctx, _ := testContext(t)

		started := make(chan struct{})
		release := make(chan struct{})
		var startOnce sync.Once
		var calls int32
		ns.SetQueryHook(func(_ context.Context, _ string, _ string, _ string, _ *QueryOptions) (packet.Packet, error) {
			atomic.AddInt32(&calls, 1)
			startOnce.Do(func() { close(started) })
			<-release
			msg := new(dns.Msg)
			msg.Rcode = dns.RcodeSuccess
			return packet.Packet{Msg: msg}, nil
		})

		var wg sync.WaitGroup
		wg.Add(2)
		var err1, err2 error
		go func() {
			defer wg.Done()
			_, err1 = ns.QueryWithOptions(ctx, "example", "A", nil)
		}()

		select {
		case <-started:
		case <-time.After(200 * time.Millisecond):
			t.Fatalf("expected first query to start")
		}

		go func() {
			defer wg.Done()
			_, err2 = ns.QueryWithOptions(ctx, "example", "A", nil)
		}()

		synctest.Wait()
		if got := atomic.LoadInt32(&calls); got != 1 {
			t.Fatalf("expected single inflight call, got %d", got)
		}

		close(release)
		wg.Wait()

		if err1 != nil || err2 != nil {
			t.Fatalf("unexpected errors: %v %v", err1, err2)
		}
		if got := atomic.LoadInt32(&calls); got != 1 {
			t.Fatalf("expected one query call total, got %d", got)
		}
	})
}

func TestCacheStoreIsolation(t *testing.T) {
	cacheA := NewCacheStore()
	cacheB := NewCacheStore()

	nsA := newCacheNS(t, cacheA, "ns.example", "192.0.2.30")
	nsB := newCacheNS(t, cacheB, "ns.example", "192.0.2.30")

	var callsA int
	nsA.SetQueryHook(func(_ context.Context, _ string, _ string, _ string, _ *QueryOptions) (packet.Packet, error) {
		callsA++
		msg := new(dns.Msg)
		msg.Rcode = dns.RcodeSuccess
		return packet.Packet{Msg: msg}, nil
	})

	var callsB int
	nsB.SetQueryHook(func(_ context.Context, _ string, _ string, _ string, _ *QueryOptions) (packet.Packet, error) {
		callsB++
		msg := new(dns.Msg)
		msg.Rcode = dns.RcodeSuccess
		return packet.Packet{Msg: msg}, nil
	})

	_, _ = nsA.QueryWithOptions(context.Background(), "example", "A", nil)
	_, _ = nsA.QueryWithOptions(context.Background(), "example", "A", nil)
	if callsA != 1 {
		t.Fatalf("expected cache hit in store A, got %d calls", callsA)
	}

	_, _ = nsB.QueryWithOptions(context.Background(), "example", "A", nil)
	if callsB != 1 {
		t.Fatalf("expected isolated cache store, got %d calls", callsB)
	}
}

// TestTwoRunsDoNotShareCache pins run isolation for the error cache: a
// forced error recorded on one store must not blackout the same address
// on an independent store. The answer-cache half of this invariant is
// pinned by TestCacheStoreIsolation above.
func TestTwoRunsDoNotShareCache(t *testing.T) {
	cacheA := NewCacheStore()
	cacheB := NewCacheStore()
	addr := "192.0.2.32"

	nsA := newCacheNS(t, cacheA, "ns.example", addr)
	nsB := newCacheNS(t, cacheB, "ns.example", addr)
	if nsA.state.errorCache == nsB.state.errorCache {
		t.Fatalf("independent stores must not share error cache pointers")
	}

	ctx, prof := testContext(t)
	prof.Resolver.Defaults.ErrorCacheTTL = 60

	nsA.SetQueryHook(func(_ context.Context, _ string, _ string, _ string, _ *QueryOptions) (packet.Packet, error) {
		return packet.Packet{}, fmt.Errorf("network error")
	})
	opts := &QueryOptions{BlacklistingDisabled: true}

	if _, err := nsA.QueryWithOptions(ctx, "example", "A", opts); err == nil {
		t.Fatalf("expected error on store A's query")
	}

	key, _, _, err := buildCacheKey("example", "A", "IN", opts)
	if err != nil {
		t.Fatalf("build cache key: %v", err)
	}
	if skip, _ := nsA.state.errorCache.shouldSkip(key); !skip {
		t.Fatalf("expected store A's error cache to engage")
	}
	if skip, _ := nsB.state.errorCache.shouldSkip(key); skip {
		t.Fatalf("store A's error must not blackout the address on store B")
	}

	var callsB int
	nsB.SetQueryHook(func(_ context.Context, _ string, _ string, _ string, _ *QueryOptions) (packet.Packet, error) {
		callsB++
		msg := new(dns.Msg)
		msg.Rcode = dns.RcodeSuccess
		return packet.Packet{Msg: msg}, nil
	})
	if _, err := nsB.QueryWithOptions(ctx, "example", "A", opts); err != nil {
		t.Fatalf("store B's query must run unaffected: %v", err)
	}
	if callsB != 1 {
		t.Fatalf("expected store B's query to reach the network, got %d calls", callsB)
	}
}

// TestIntraRunSharesPerAddressAcrossObjects pins the perf guarantee that
// the answer cache is keyed per (store, address), not per Nameserver
// object: two objects with different names on the same IP share one
// query cache, so a repeat query is a hit even though the object cache
// missed. A per-instance fallback store would break this.
func TestIntraRunSharesPerAddressAcrossObjects(t *testing.T) {
	cache := NewCacheStore()
	addr := "192.0.2.33"

	ns1 := newCacheNS(t, cache, "ns1.example", addr)
	ns2 := newCacheNS(t, cache, "ns2.example", addr)
	if ns1.state.cache != ns2.state.cache {
		t.Fatalf("expected both objects to share the per-address query cache")
	}

	var calls int
	hook := func(_ context.Context, _ string, _ string, _ string, _ *QueryOptions) (packet.Packet, error) {
		calls++
		msg := new(dns.Msg)
		msg.Rcode = dns.RcodeSuccess
		return packet.Packet{Msg: msg}, nil
	}
	ns1.SetQueryHook(hook)
	ns2.SetQueryHook(hook)

	if _, err := ns1.QueryWithOptions(context.Background(), "example", "A", nil); err != nil {
		t.Fatalf("query on object 1: %v", err)
	}
	if _, err := ns2.QueryWithOptions(context.Background(), "example", "A", nil); err != nil {
		t.Fatalf("query on object 2: %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected object 2's repeat query to hit the shared cache, got %d network calls", calls)
	}

	metrics := cache.QueryMetrics()
	if metrics.Hits != 1 || metrics.Misses != 1 {
		t.Fatalf("unexpected query cache metrics: %+v", metrics)
	}
}

// TestNilCacheConstructionIsLoudError pins the contract that constructing
// a Nameserver without a cache store fails loudly instead of silently
// binding to a process-global store.
func TestNilCacheConstructionIsLoudError(t *testing.T) {
	if _, err := NewWithCache(nil, "ns.example", "192.0.2.34", nil); err == nil {
		t.Fatalf("expected error from NewWithCache with nil store")
	}
	if _, err := NewWithContext(context.Background(), "ns.example", "192.0.2.34", nil); err == nil {
		t.Fatalf("expected error from NewWithContext with a cache-less ctx")
	}
}

// TestEnsureStateOnZeroValueNameserverPanics guards the invariant that a
// zero-value Nameserver is never queried; a query must panic rather than
// bind to a global store.
func TestEnsureStateOnZeroValueNameserverPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatalf("expected panic from querying a zero-value Nameserver")
		}
	}()
	ns := Nameserver{Name: dnsname.New("ns.example"), Address: netip.MustParseAddr("192.0.2.35")}
	ns.SetQueryHook(func(_ context.Context, _ string, _ string, _ string, _ *QueryOptions) (packet.Packet, error) {
		return packet.Packet{}, nil
	})
}

func TestDefaultProfileErrorCacheTTLIsNonZero(t *testing.T) {
	prof := dnstest.DefaultProfile(t)
	if prof.Resolver.Defaults.ErrorCacheTTL <= 0 {
		t.Fatalf("default error_cache_ttl must be > 0 so a transport timeout suppresses repeat queries to the same NS within a run; got %d", prof.Resolver.Defaults.ErrorCacheTTL)
	}
}

func TestErrorCacheEngagesByDefault(t *testing.T) {
	ctx, _ := testContext(t)
	// Intentionally do not override ErrorCacheTTL: this asserts the default
	// profile suppresses repeat live queries on the same transport.
	ns := newNS(t, ctx, "ns.example", "192.0.2.16")

	var calls int
	ns.SetQueryHook(func(_ context.Context, _ string, _ string, _ string, _ *QueryOptions) (packet.Packet, error) {
		calls++
		return packet.Packet{}, fmt.Errorf("network error")
	})
	opts := &QueryOptions{BlacklistingDisabled: true}

	// Same qname/qtype on the second query: the error cache key now
	// includes the full query identity, so a different qname would miss
	// the cache. The query cache stores nothing on a network error, so
	// the second call has to be suppressed by the error cache itself.
	if _, err := ns.QueryWithOptions(ctx, "first.example", "A", opts); err == nil {
		t.Fatalf("expected error on first query")
	}
	if _, err := ns.QueryWithOptions(ctx, "first.example", "A", opts); err != nil {
		t.Fatalf("expected error cache to suppress repeat query, got error: %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected default error cache to suppress repeat query; got %d hook calls", calls)
	}
}

// TestErrorCacheSkipsQueries asserts the user-visible contract that a
// repeat of an already-failed query is suppressed within the run. The
// suppression is jointly provided by the query cache (memoizes nil) and
// the error cache (TTL-bound skip). Different queries to the same NS
// are NOT suppressed - that is what fix #3 exists to ensure and is
// pinned by TestErrorCacheKeyIsolatesQueries below.
func TestErrorCacheSkipsQueries(t *testing.T) {
	cache := NewCacheStore()
	ns := newCacheNS(t, cache, "ns.example", "192.0.2.15")

	ctx, prof := testContext(t)
	prof.Resolver.Defaults.ErrorCacheTTL = 60

	var calls int
	ns.SetQueryHook(func(_ context.Context, _ string, _ string, _ string, _ *QueryOptions) (packet.Packet, error) {
		calls++
		return packet.Packet{}, fmt.Errorf("network error")
	})

	_, err := ns.QueryWithOptions(ctx, "example", "A", nil)
	if err == nil {
		t.Fatalf("expected error on first query")
	}
	_, err = ns.QueryWithOptions(ctx, "example", "A", nil)
	if err != nil {
		t.Fatalf("expected repeat of failed query to be suppressed, got %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected 1 live call, got %d", calls)
	}
}

// TestErrorCacheDebouncesSingleTimeout pins that a single timed-out
// exchange does not write the error cache. With the per-query cache key,
// this matters mostly as a hedge: if the query cache later evicts the
// nil entry (FIFO at QueryCacheMaxEntries), the error cache TTL must
// not have engaged off of one transient drop. We assert directly on the
// error cache's state because the query cache shadows it for repeats.
func TestErrorCacheDebouncesSingleTimeout(t *testing.T) {
	ctx, prof := testContext(t)
	prof.Resolver.Defaults.ErrorCacheTTL = 60
	prof.Resolver.Defaults.FastFailTimeoutCount = 5

	ns := newNS(t, ctx, "ns.example", "192.0.2.17")

	var calls int
	ns.SetQueryHook(func(_ context.Context, _ string, _ string, _ string, _ *QueryOptions) (packet.Packet, error) {
		calls++
		return packet.Packet{}, fmt.Errorf("read udp: i/o timeout")
	})
	opts := &QueryOptions{BlacklistingDisabled: true}

	keyFor := func(qname string) string {
		t.Helper()
		k, _, _, err := buildCacheKey(qname, "A", "IN", opts)
		if err != nil {
			t.Fatalf("build cache key: %v", err)
		}
		return k
	}

	// First timeout: debounce holds, error cache stays empty.
	if _, err := ns.QueryWithOptions(ctx, "first.example", "A", opts); err == nil {
		t.Fatalf("expected timeout error on first query")
	}
	if skip, _ := ns.state.errorCache.shouldSkip(keyFor("first.example")); skip {
		t.Fatalf("error cache must not engage after a single timeout")
	}

	// Second timeout on a different qname so the query cache misses and
	// the network hook fires again. fastFail's consecutive count rises
	// to 2 and now the second key is committed to the error cache.
	if _, err := ns.QueryWithOptions(ctx, "second.example", "A", opts); err == nil {
		t.Fatalf("expected timeout error on second query")
	}
	if calls != 2 {
		t.Fatalf("expected 2 live calls (cross-qname misses query cache); got %d", calls)
	}
	if skip, _ := ns.state.errorCache.shouldSkip(keyFor("second.example")); !skip {
		t.Fatalf("error cache must engage on the second consecutive timeout")
	}
	// The first query's slot stays open: the debounce held when it failed.
	if skip, _ := ns.state.errorCache.shouldSkip(keyFor("first.example")); skip {
		t.Fatalf("first qname's error cache slot must still be empty - debounce held when it failed")
	}
}

// TestErrorCacheTTLRespectsTimeoutBudget pins the contract that the
// error cache TTL is bounded by the per-query retry budget so a stale
// blackout does not outlive the time it would have taken to retry the
// query live. Asserted directly on errorCache because the query cache
// otherwise shadows the same key.
func TestErrorCacheTTLRespectsTimeoutBudget(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ns := newCacheNS(t, NewCacheStore(), "ns.example", "192.0.2.31")

		ctx, prof := testContext(t)
		prof.Resolver.Defaults.ErrorCacheTTL = 60

		ns.SetQueryHook(func(_ context.Context, _ string, _ string, _ string, _ *QueryOptions) (packet.Packet, error) {
			return packet.Packet{}, fmt.Errorf("network error")
		})

		timeout := 20 * time.Millisecond
		retry := 1
		opts := &QueryOptions{Timeout: &timeout, Retry: &retry}

		if _, err := ns.QueryWithOptions(ctx, "example", "A", opts); err == nil {
			t.Fatalf("expected error on first query")
		}

		key, _, _, err := buildCacheKey("example", "A", "IN", opts)
		if err != nil {
			t.Fatalf("build cache key: %v", err)
		}
		if skip, _ := ns.state.errorCache.shouldSkip(key); !skip {
			t.Fatalf("expected error cache to engage on this non-timeout error")
		}

		time.Sleep(60 * time.Millisecond)

		if skip, _ := ns.state.errorCache.shouldSkip(key); skip {
			t.Fatalf("expected error cache TTL to expire after retry-budget window")
		}
	})
}

// TestErrorCacheNotSharedAcrossSnapshots pins the contract that error caches
// are run-local. Without this, a single transient failure in one job (e.g.
// a recursor probe) cascades into spurious B02_NO_WORKING_NS verdicts on
// concurrent jobs that share the same NS address via cross-job hot cache.
func TestErrorCacheNotSharedAcrossSnapshots(t *testing.T) {
	root := NewCacheStore()
	addr := "192.0.2.221"

	// Snapshot A populates its own error cache by getting it once and writing.
	snapA := root.SnapshotForRun()
	cacheA := snapA.errorCacheForAddress(addr)
	cacheA.set("udp", time.Minute)
	if skip, _ := cacheA.shouldSkip("udp"); !skip {
		t.Fatalf("snapshot A's error cache must reflect its own write")
	}

	// Snapshot B (sibling of A under the same root) must see a fresh
	// error cache for the same address - no skip.
	snapB := root.SnapshotForRun()
	cacheB := snapB.errorCacheForAddress(addr)
	if cacheA == cacheB {
		t.Fatalf("snapshots must not share error cache pointers for the same address")
	}
	if skip, _ := cacheB.shouldSkip("udp"); skip {
		t.Fatalf("snapshot B must not inherit snapshot A's error cache")
	}
}

// TestErrorCacheDoesNotMergeBackToParent pins that even after a run's
// MergeWarmDataFrom call, error-cache state stays out of the parent. A
// subsequent run leasing the same parent must start with no error cache.
func TestErrorCacheDoesNotMergeBackToParent(t *testing.T) {
	root := NewCacheStore()
	addr := "192.0.2.222"

	run1 := root.SnapshotForRun()
	cache1 := run1.errorCacheForAddress(addr)
	cache1.set("udp", time.Minute)
	root.MergeWarmDataFrom(run1)

	if got := root.ErrorCacheCount(); got != 0 {
		t.Fatalf("parent must have no error caches after merge; got count %d", got)
	}

	run2 := root.SnapshotForRun()
	cache2 := run2.errorCacheForAddress(addr)
	if skip, _ := cache2.shouldSkip("udp"); skip {
		t.Fatalf("a fresh run must start with an empty error cache for this address")
	}
}

// TestQueryCacheStillMergesBackToParent guards against an over-broad fix:
// the positive query cache must still propagate across runs (that is the
// whole point of cross-job hot cache - warmed parent-zone responses).
func TestQueryCacheStillMergesBackToParent(t *testing.T) {
	root := NewCacheStore()
	addr := "192.0.2.223"

	run1 := root.SnapshotForRun()
	newCacheNS(t, run1, "ns.example", addr)
	root.MergeWarmDataFrom(run1)

	if got := root.AddressCacheCount(); got != 1 {
		t.Fatalf("parent must inherit query cache after merge; got %d", got)
	}
}

// TestErrorCacheKeyIsolatesQueries pins the central contract of fix #3:
// a cached failure for one (qname, qtype) must not blackout queries with
// a different identity. This is the standalone-trace pathology where one
// failed CDS query during DNSSEC15 used to ghost-skip every later test's
// SOA, NS, MX, etc. queries to the same nameserver.
func TestErrorCacheKeyIsolatesQueries(t *testing.T) {
	ctx, prof := testContext(t)
	prof.Resolver.Defaults.ErrorCacheTTL = 60

	ns := newNS(t, ctx, "ns.example", "192.0.2.230")

	// Pre-populate the error cache for a CDS query (mimicking DNSSEC15).
	cdsKey, _, _, err := buildCacheKey("kristianstad.se", "CDS", "IN", nil)
	if err != nil {
		t.Fatalf("build cache key: %v", err)
	}
	ns.state.errorCache.set(cdsKey, time.Minute)

	var calls int
	ns.SetQueryHook(func(_ context.Context, _ string, qtype string, _ string, _ *QueryOptions) (packet.Packet, error) {
		calls++
		msg := new(dns.Msg)
		msg.Rcode = dns.RcodeSuccess
		return packet.Packet{Msg: msg}, nil
	})

	// A CDS query for the same name must skip - identical key.
	if pkt, err := ns.QueryWithOptions(ctx, "kristianstad.se", "CDS", nil); err != nil || pkt.Msg != nil {
		t.Fatalf("expected CDS to be skipped by error cache, got msg=%v err=%v", pkt.Msg, err)
	}
	if calls != 0 {
		t.Fatalf("CDS query should not have reached the live path; got %d calls", calls)
	}

	// SOA query for the same name must NOT be poisoned - different key.
	if pkt, err := ns.QueryWithOptions(ctx, "kristianstad.se", "SOA", nil); err != nil || pkt.Msg == nil {
		t.Fatalf("SOA query must reach the live path despite CDS being cached; got msg=%v err=%v", pkt.Msg, err)
	}
	// MX, NS, A, CDNSKEY, etc. all need to live-fire too.
	for _, qtype := range []string{"MX", "NS", "A", "CDNSKEY"} {
		if pkt, err := ns.QueryWithOptions(ctx, "kristianstad.se", qtype, nil); err != nil || pkt.Msg == nil {
			t.Fatalf("%s query must reach live path; got msg=%v err=%v", qtype, pkt.Msg, err)
		}
	}
	if calls != 5 {
		t.Fatalf("expected 5 live calls (SOA, MX, NS, A, CDNSKEY) past the cached CDS; got %d", calls)
	}
}

// TestErrorCacheKeyIsolatesEDNSVariants pins that EDNS-version probes
// (NS02, NS10, NS11, etc.) failing on a server that mishandles EDNS do
// not poison the plain-EDNS0 SOA queries used by Basic02 and downstream
// testcases. Without distinguishing EDNS state in the cache key, a
// failed EDNS-1 query would blackout the address for all UDP queries.
func TestErrorCacheKeyIsolatesEDNSVariants(t *testing.T) {
	ctx, prof := testContext(t)
	prof.Resolver.Defaults.ErrorCacheTTL = 60

	ns := newNS(t, ctx, "ns.example", "192.0.2.231")

	// Cache a failure for an EDNS-version-1 SOA probe.
	ver1 := uint8(1)
	ednsOpts := &QueryOptions{EDNSDetails: &transport.EDNSDetails{Version: &ver1}}
	ednsKey, _, _, err := buildCacheKey("example.test", "SOA", "IN", ednsOpts)
	if err != nil {
		t.Fatalf("build cache key: %v", err)
	}
	ns.state.errorCache.set(ednsKey, time.Minute)

	var calls int
	ns.SetQueryHook(func(_ context.Context, _ string, _ string, _ string, _ *QueryOptions) (packet.Packet, error) {
		calls++
		msg := new(dns.Msg)
		msg.Rcode = dns.RcodeSuccess
		return packet.Packet{Msg: msg}, nil
	})

	// A plain SOA query (no EDNS overrides) must live-fire even though
	// the EDNS-1 variant is cached - the EDNS state is part of the key.
	if pkt, err := ns.QueryWithOptions(ctx, "example.test", "SOA", nil); err != nil || pkt.Msg == nil {
		t.Fatalf("plain SOA must not inherit cached EDNS-1 failure; got msg=%v err=%v", pkt.Msg, err)
	}
	if calls != 1 {
		t.Fatalf("expected the plain SOA to live-fire; got %d calls", calls)
	}
}

// TestErrorCacheSetEvictsExpiredEntries pins the eviction sweep added
// in this commit: a long-lived cache must not accumulate stale entries
// that nothing reads back. The wider per-query key allows many entries
// per address, so opportunistic eviction during set keeps the map size
// bounded by live workload.
func TestErrorCacheSetEvictsExpiredEntries(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c := &errorCache{}
		c.set("a", 10*time.Millisecond)
		c.set("b", 10*time.Millisecond)
		c.set("c", time.Hour)

		if got := len(c.data); got != 3 {
			t.Fatalf("expected 3 entries before sweep; got %d", got)
		}

		time.Sleep(20 * time.Millisecond)

		// Inserting a fresh entry must sweep "a" and "b" but keep "c".
		c.set("d", time.Hour)

		c.mu.Lock()
		defer c.mu.Unlock()
		if _, ok := c.data["a"]; ok {
			t.Fatalf("expired entry a must have been evicted")
		}
		if _, ok := c.data["b"]; ok {
			t.Fatalf("expired entry b must have been evicted")
		}
		if _, ok := c.data["c"]; !ok {
			t.Fatalf("live entry c must remain")
		}
		if _, ok := c.data["d"]; !ok {
			t.Fatalf("just-inserted entry d must be present")
		}
		if len(c.data) != 2 {
			t.Fatalf("expected 2 entries after sweep + insert; got %d", len(c.data))
		}
	})
}

func TestQueryCacheStoresTimeoutsAsNoMessage(t *testing.T) {
	ctx, prof := testContext(t)
	prof.Resolver.Defaults.ErrorCacheTTL = 0

	ns := newNS(t, ctx, "ns.example", "192.0.2.250")

	var calls int
	ns.SetQueryHook(func(_ context.Context, _ string, _ string, _ string, _ *QueryOptions) (packet.Packet, error) {
		calls++
		return packet.Packet{}, fmt.Errorf("timeout")
	})

	opts := &QueryOptions{BlacklistingDisabled: true}
	pkt, err := ns.QueryWithOptions(ctx, "example", "SOA", opts)
	if err == nil {
		t.Fatalf("expected error on first query")
	}
	if pkt.Msg != nil {
		t.Fatalf("expected nil msg on first timeout, got %v", pkt.Msg)
	}
	pkt, err = ns.QueryWithOptions(ctx, "example", "SOA", opts)
	if err != nil {
		t.Fatalf("expected cached no-response on second query, got error: %v", err)
	}
	if pkt.Msg != nil {
		t.Fatalf("expected cached no-response on second query, got msg: %v", pkt.Msg)
	}
	if calls != 1 {
		t.Fatalf("expected 1 network call after caching the timeout, got %d", calls)
	}
}

func TestQueryCacheDoesNotStoreContextCancelErrors(t *testing.T) {
	ctx, _ := testContext(t)
	cctx, cancel := context.WithCancel(ctx)

	ns := newNS(t, cctx, "ns.example", "192.0.2.251")

	var calls int
	ns.SetQueryHook(func(_ context.Context, _ string, _ string, _ string, _ *QueryOptions) (packet.Packet, error) {
		calls++
		cancel()
		return packet.Packet{}, context.Canceled
	})

	opts := &QueryOptions{BlacklistingDisabled: true}
	_, _ = ns.QueryWithOptions(cctx, "example", "SOA", opts)
	// New context for the second call so it isn't short-circuited by ctx.Err().
	cctx2, cancel2 := context.WithCancel(ctx)
	defer cancel2()
	ns.SetQueryHook(func(_ context.Context, _ string, _ string, _ string, _ *QueryOptions) (packet.Packet, error) {
		calls++
		return packet.Packet{}, fmt.Errorf("timeout")
	})
	_, err := ns.QueryWithOptions(cctx2, "example", "SOA", opts)
	if err == nil {
		t.Fatalf("expected error on second query (cancellation must not have cached nil)")
	}
	if calls != 2 {
		t.Fatalf("expected 2 network calls, got %d (cancellation should not cache)", calls)
	}
}

// TestQueryCacheDoesNotStoreCauseCancelledQueries documents that a query
// whose ctx was cancelled (with any cause - the recursor uses ErrRaceLost
// for parallel race-losers) must not produce a cached no_message entry:
// that ctx is the recursor's batch ctx, and a future identical query under
// a fresh ctx should still be attempted.
func TestQueryCacheDoesNotStoreCauseCancelledQueries(t *testing.T) {
	ctx, prof := testContext(t)
	prof.Resolver.Defaults.ErrorCacheTTL = 0

	store := NewCacheStore()
	ns := newCacheNS(t, store, "ns.example", "192.0.2.253")

	// Local sentinel mirroring recursor.ErrRaceLost; importing recursor here
	// would be a cycle. The contract under test is "ctx cancelled with any
	// cause -> no cache entry", regardless of the cause's identity.
	raceLost := fmt.Errorf("test: parallel race lost")
	cctx, cancelCause := context.WithCancelCause(ctx)
	cancelCause(raceLost)
	ns.SetQueryHook(func(_ context.Context, _ string, _ string, _ string, _ *QueryOptions) (packet.Packet, error) {
		return packet.Packet{}, cctx.Err()
	})

	if _, err := ns.QueryWithOptions(cctx, "example", "SOA", &QueryOptions{BlacklistingDisabled: true}); err == nil {
		t.Fatalf("expected error from race-cancelled query")
	}

	entries, err := store.ExportEntries()
	if err != nil {
		t.Fatalf("export entries: %v", err)
	}
	for _, e := range entries {
		if e.Address == "192.0.2.253" {
			t.Fatalf("race-lost query must not produce a cache entry: %+v", e)
		}
	}
}

func TestContextCanceledDoesNotBlacklist(t *testing.T) {
	ns := newCacheNS(t, NewCacheStore(), "ns.example", "192.0.2.200")

	ns.SetQueryHook(func(_ context.Context, _ string, _ string, _ string, _ *QueryOptions) (packet.Packet, error) {
		return packet.Packet{}, context.Canceled
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, _ = ns.QueryWithOptions(ctx, "example", "SOA", nil)

	if ns.state == nil {
		t.Fatalf("expected state to be initialized")
	}
	if ns.state.blacklisted[false] {
		t.Fatalf("expected UDP not to be blacklisted on context cancellation")
	}
}

// Within one run, a blackout learned through one nameserver object suppresses
// queries through every other object on the same address.
//
// Names and qnames must all differ: NewWithCache returns the cached instance
// for a repeated name and address, and the error and query caches are keyed by
// address, so a reused qname trips ERROR_CACHE_SKIP and measures the wrong
// mechanism.
func TestReachabilityCacheSkipsWithinStore(t *testing.T) {
	store := NewCacheStore()

	nsA := newCacheNS(t, store, "ns-a.example", "192.0.2.44")
	nsB := newCacheNS(t, store, "ns-b.example", "192.0.2.44")
	nsC := newCacheNS(t, store, "ns-c.example", "192.0.2.44")

	ctx, prof := testContext(t)
	prof.Resolver.Defaults.NegativeCacheTTL = 60

	hardErr := func(_ context.Context, _ string, _ string, _ string, _ *QueryOptions) (packet.Packet, error) {
		return packet.Packet{}, &net.OpError{Op: "dial", Net: "udp", Err: syscall.EHOSTUNREACH}
	}
	var callsA, callsB, callsC int
	nsA.SetQueryHook(func(c context.Context, n string, t1 string, t2 string, o *QueryOptions) (packet.Packet, error) {
		callsA++
		return hardErr(c, n, t1, t2, o)
	})
	nsB.SetQueryHook(func(c context.Context, n string, t1 string, t2 string, o *QueryOptions) (packet.Packet, error) {
		callsB++
		return hardErr(c, n, t1, t2, o)
	})
	nsC.SetQueryHook(func(_ context.Context, _ string, _ string, _ string, _ *QueryOptions) (packet.Packet, error) {
		callsC++
		return packet.Packet{}, nil
	})

	// Two consecutive hard errors promote the store's reachability cache from
	// pending to blocked.
	if _, err := nsA.QueryWithOptions(ctx, "first.example", "A", nil); err == nil {
		t.Fatalf("expected error on first hard-error query")
	}
	if _, err := nsB.QueryWithOptions(ctx, "second.example", "A", nil); err == nil {
		t.Fatalf("expected error on second hard-error query")
	}

	_, err := nsC.QueryWithOptions(ctx, "third.example", "A", nil)
	if err != nil {
		t.Fatalf("expected reachability cache to skip error after 2 hard errors, got %v", err)
	}
	if callsA != 1 || callsB != 1 {
		t.Fatalf("expected both hard-error calls to run live; got A=%d B=%d", callsA, callsB)
	}
	if callsC != 0 {
		t.Fatalf("expected third call to be skipped by reachability cache, got %d", callsC)
	}

	metrics := store.ReachabilityMetrics()
	if metrics.Hits != 1 || metrics.Misses != 2 || metrics.Evictions != 0 {
		t.Fatalf("unexpected reachability metrics: %+v", metrics)
	}

	// A different run must not inherit the blackout.
	other := NewCacheStore()
	nsOther := newCacheNS(t, other, "ns-a.example", "192.0.2.44")
	var callsOther int
	nsOther.SetQueryHook(func(c context.Context, n string, t1 string, t2 string, o *QueryOptions) (packet.Packet, error) {
		callsOther++
		return hardErr(c, n, t1, t2, o)
	})
	if _, err = nsOther.QueryWithOptions(ctx, "first.example", "A", nil); err == nil {
		t.Fatalf("expected error on the unrelated store's query")
	}
	if callsOther != 1 {
		t.Fatalf("a separate store must issue live; got %d calls", callsOther)
	}
}

// The blackout TTL is bounded by the retry budget, so it expires and the live
// path runs again.
//
// The pre-sleep assertion keeps this test honest: the final call count alone is
// satisfied both by "the blackout expired" and by "it never engaged". The 100ms
// budget is both the debounce window and the blackout length, so it needs
// headroom under the race detector.
func TestReachabilityCacheExpiresByBudget(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		store := NewCacheStore()

		names := []string{"ns-a.example", "ns-b.example", "ns-c.example", "ns-d.example"}
		servers := make([]Nameserver, 0, len(names))
		for _, name := range names {
			ns := newCacheNS(t, store, name, "192.0.2.55")
			servers = append(servers, ns)
		}

		ctx, prof := testContext(t)
		prof.Resolver.Defaults.NegativeCacheTTL = 60

		timeout := 100 * time.Millisecond
		retry := 0
		opts := &QueryOptions{Timeout: &timeout, Retry: &retry}

		var calls int
		for _, ns := range servers {
			ns.SetQueryHook(func(_ context.Context, _ string, _ string, _ string, _ *QueryOptions) (packet.Packet, error) {
				calls++
				return packet.Packet{}, &net.OpError{Op: "dial", Net: "udp", Err: syscall.EHOSTUNREACH}
			})
		}

		if _, err := servers[0].QueryWithOptions(ctx, "first.example", "A", opts); err == nil {
			t.Fatalf("expected error on first query")
		}
		if _, err := servers[1].QueryWithOptions(ctx, "second.example", "A", opts); err == nil {
			t.Fatalf("expected error on second query")
		}

		// Anti-vacuity: the blackout must be live right now.
		if _, err := servers[2].QueryWithOptions(ctx, "third.example", "A", opts); err != nil {
			t.Fatalf("expected the blackout to suppress the third query, got %v", err)
		}
		if calls != 2 {
			t.Fatalf("expected the third query to be suppressed; got %d live calls", calls)
		}

		time.Sleep(150 * time.Millisecond)

		// Past the budget-bounded TTL the live path runs again.
		if _, err := servers[3].QueryWithOptions(ctx, "fourth.example", "A", opts); err == nil {
			t.Fatalf("expected error after reachability cache expiry")
		}
		if calls != 3 {
			t.Fatalf("expected reachability cache to expire and re-query, got %d calls", calls)
		}
	})
}

// A single transient EHOSTUNREACH (one IPv6 routing flap, one stray ICMP
// destination-unreachable) must not blackout the address: without the debounce
// one bad packet under batch load blackholes the NS for negative_cache_ttl
// seconds and cascades into spurious B02_NO_WORKING_NS verdicts.
//
// Distinct nameserver names and qnames on one store keep the address-keyed
// error and query caches from short-circuiting before the reachability rung.
func TestReachabilityCacheDebouncesSingleHardError(t *testing.T) {
	ctx, prof := testContext(t)
	prof.Resolver.Defaults.NegativeCacheTTL = 60

	hardErr := func(_ context.Context, _ string, _ string, _ string, _ *QueryOptions) (packet.Packet, error) {
		return packet.Packet{}, &net.OpError{Op: "dial", Net: "udp", Err: syscall.EHOSTUNREACH}
	}
	addr := "192.0.2.56"
	store := NewCacheStore()
	freshNS := freshNSSeq(t, store, addr)

	var calls int
	hook := func(c context.Context, n string, t1 string, t2 string, o *QueryOptions) (packet.Packet, error) {
		calls++
		return hardErr(c, n, t1, t2, o)
	}

	ns1 := freshNS(hook)
	if _, err := ns1.QueryWithOptions(ctx, "first.example", "A", nil); err == nil {
		t.Fatalf("expected error on first hard-error query")
	}

	ns2 := freshNS(hook)
	if _, err := ns2.QueryWithOptions(ctx, "second.example", "A", nil); err == nil {
		t.Fatalf("expected error on second hard-error query")
	}
	if calls != 2 {
		t.Fatalf("expected 2 live calls (single hard error must not blackout); got %d", calls)
	}

	// After two consecutive hard errors the cache engages; a third query
	// from yet another fresh nameserver is suppressed without invoking the
	// hook.
	ns3 := freshNS(hook)
	if _, err := ns3.QueryWithOptions(ctx, "third.example", "A", nil); err != nil {
		t.Fatalf("expected reachability cache to suppress 3rd query, got: %v", err)
	}
	if calls != 2 {
		t.Fatalf("expected cache to engage after 2 hard errors (2 hook calls total); got %d", calls)
	}
}

// TestReachabilityCacheResetsOnSuccess checks that an intervening successful
// query clears the pending hard-error count so a third fresh failure starts
// the debounce over rather than promoting straight to a blackout. As with
// TestReachabilityCacheDebouncesSingleHardError, each phase uses a distinct
// nameserver name and qname on one store.
func TestReachabilityCacheResetsOnSuccess(t *testing.T) {
	ctx, prof := testContext(t)
	prof.Resolver.Defaults.NegativeCacheTTL = 60

	addr := "192.0.2.57"
	store := NewCacheStore()
	freshNS := freshNSSeq(t, store, addr)

	var phase int
	hook := func(_ context.Context, _ string, _ string, _ string, _ *QueryOptions) (packet.Packet, error) {
		phase++
		if phase == 2 {
			return packet.Packet{Msg: &dns.Msg{}}, nil
		}
		return packet.Packet{}, &net.OpError{Op: "dial", Net: "udp", Err: syscall.EHOSTUNREACH}
	}

	ns1 := freshNS(hook)
	if _, err := ns1.QueryWithOptions(ctx, "a.example", "A", nil); err == nil {
		t.Fatalf("expected error on first hard-error query")
	}

	ns2 := freshNS(hook)
	if pkt, err := ns2.QueryWithOptions(ctx, "b.example", "A", nil); err != nil || pkt.Msg == nil {
		t.Fatalf("expected successful 2nd query, got pkt.Msg=%v err=%v", pkt.Msg, err)
	}

	ns3 := freshNS(hook)
	if _, err := ns3.QueryWithOptions(ctx, "c.example", "A", nil); err == nil {
		t.Fatalf("expected error on third hard-error query")
	}

	if phase != 3 {
		t.Fatalf("expected 3 hook calls (success must reset pending so cache stays open); got %d", phase)
	}

	// A 4th query from yet another fresh nameserver must still issue live:
	// the success between phases 1 and 3 reset the consecutive count, so
	// phase 3 is treated as a fresh first failure.
	ns4 := freshNS(hook)
	if _, err := ns4.QueryWithOptions(ctx, "d.example", "A", nil); err == nil {
		t.Fatalf("expected error on 4th query (cache must not have engaged)")
	}
	if phase != 4 {
		t.Fatalf("expected 4 hook calls; cache must not have engaged after a success reset; got %d", phase)
	}
}

func TestQueryIPv4Disabled(t *testing.T) {
	ns := newCacheNS(t, NewCacheStore(), "ns.example", "192.0.2.11")

	ctx, prof := testContext(t)
	prof.Net.IPv4 = false

	var calls int
	ns.SetQueryHook(func(_ context.Context, _ string, _ string, _ string, _ *QueryOptions) (packet.Packet, error) {
		calls++
		return packet.Packet{}, nil
	})

	resp, err := ns.QueryWithOptions(ctx, "example", "A", nil)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if resp.Msg != nil {
		t.Fatalf("expected no response when IPv4 disabled")
	}
	if calls != 0 {
		t.Fatalf("expected hook not called, got %d", calls)
	}
}

func TestClientForOptionsDefaults(t *testing.T) {
	ns := newCacheNS(t, NewCacheStore(), "ns.example", "192.0.2.12")

	ctx, _ := testContext(t)

	client, err := ns.clientForOptions(ctx, nil)
	if err != nil {
		t.Fatalf("client for nil opts: %v", err)
	}
	if client.RecursionDesired {
		t.Fatalf("expected recursion disabled by default")
	}
	if client.UseTCP {
		t.Fatalf("expected UDP by default")
	}

	on := true
	client, err = ns.clientForOptions(ctx, &QueryOptions{Recurse: &on, UseVC: &on})
	if err != nil {
		t.Fatalf("client for explicit opts: %v", err)
	}
	if !client.RecursionDesired {
		t.Fatalf("expected recursion enabled when requested")
	}
	if !client.UseTCP {
		t.Fatalf("expected TCP when requested")
	}
}

func TestClientForOptionsAppliesProfileSourceAddressByFamily(t *testing.T) {
	ctx, prof := testContext(t)
	prof.Resolver.Source4 = "192.0.2.88"
	prof.Resolver.Source6 = "2001:db8::88"

	ns4 := newCacheNS(t, NewCacheStore(), "ns4.example", "192.0.2.30")
	client4, err := ns4.clientForOptions(ctx, nil)
	if err != nil {
		t.Fatalf("client4: %v", err)
	}
	if client4.SourceIP != "192.0.2.88" {
		t.Fatalf("client4.SourceIP = %q, want 192.0.2.88", client4.SourceIP)
	}

	ns6 := newCacheNS(t, NewCacheStore(), "ns6.example", "2001:db8::30")
	client6, err := ns6.clientForOptions(ctx, nil)
	if err != nil {
		t.Fatalf("client6: %v", err)
	}
	if client6.SourceIP != "2001:db8::88" {
		t.Fatalf("client6.SourceIP = %q, want 2001:db8::88", client6.SourceIP)
	}

	explicit := &transport.Client{SourceIP: "192.0.2.199"}
	nsExplicit, err := NewWithCache(NewCacheStore(), "ns-explicit.example", "192.0.2.31", explicit)
	if err != nil {
		t.Fatalf("new explicit nameserver: %v", err)
	}
	clientExplicit, err := nsExplicit.clientForOptions(ctx, nil)
	if err != nil {
		t.Fatalf("clientExplicit: %v", err)
	}
	if clientExplicit.SourceIP != "192.0.2.199" {
		t.Fatalf("expected explicit source ip to be preserved, got %q", clientExplicit.SourceIP)
	}
}

func TestAXFRHook(t *testing.T) {
	ns := newCacheNS(t, NewCacheStore(), "ns.example", "192.0.2.22")

	rr1, err := dns.New("example. 60 IN SOA ns.example. hostmaster.example. 1 3600 600 86400 60")
	if err != nil {
		t.Fatalf("soa rr: %v", err)
	}
	rr2, err := dns.New("example. 60 IN A 192.0.2.10")
	if err != nil {
		t.Fatalf("a rr: %v", err)
	}

	ns.SetAXFRHook(func(_ context.Context, domain string, callback func(dns.RR) bool, class string) error {
		if domain != "example" {
			return fmt.Errorf("unexpected domain %q", domain)
		}
		if class != "CH" {
			return fmt.Errorf("unexpected class %q", class)
		}
		callback(rr1)
		callback(rr2)
		return nil
	})

	var seen int
	err = ns.AXFR(context.Background(), "example", func(_ dns.RR) bool {
		seen++
		return true
	}, "CH")
	if err != nil {
		t.Fatalf("axfr hook: %v", err)
	}
	if seen != 2 {
		t.Fatalf("expected 2 records, got %d", seen)
	}
}

func TestAXFRNoNetwork(t *testing.T) {
	ns := newCacheNS(t, NewCacheStore(), "ns.example", "192.0.2.23")

	ctx, prof := testContext(t)
	prof.NoNetwork = true

	err := ns.AXFR(ctx, "example", nil, "")
	if err == nil {
		t.Fatalf("expected error when no_network is set")
	}
	if !strings.Contains(err.Error(), "external AXFR query") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAXFRIPv4Disabled(t *testing.T) {
	ns := newCacheNS(t, NewCacheStore(), "ns.example", "192.0.2.24")

	ctx, prof := testContext(t)
	prof.Net.IPv4 = false

	var called int
	ns.SetAXFRHook(func(_ context.Context, _ string, _ func(dns.RR) bool, _ string) error {
		called++
		return nil
	})

	err := ns.AXFR(ctx, "example", nil, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if called != 0 {
		t.Fatalf("expected hook not called, got %d", called)
	}
}

func TestCacheStoreEmpty(t *testing.T) {
	cache := NewCacheStore()
	ns := newCacheNS(t, cache, "ns.example", "192.0.2.25")
	if ns.state == nil || ns.state.cache == nil {
		t.Fatalf("expected nameserver cache state")
	}

	cacheKey := "example-cache"
	ns.state.cache.set(cacheKey, &packet.Packet{Msg: new(dns.Msg)})
	if _, ok := ns.state.cache.get(cacheKey); !ok {
		t.Fatalf("expected cached packet")
	}

	nameKey := strings.ToLower(ns.Name.String())
	addrKey := ns.Address.String()
	if cached := cache.cachedNameserver(nameKey, addrKey); cached == nil {
		t.Fatalf("expected nameserver cached")
	}

	cache.Empty()

	if cached := cache.cachedNameserver(nameKey, addrKey); cached != nil {
		t.Fatalf("expected nameserver cache cleared")
	}
	if _, ok := ns.state.cache.get(cacheKey); ok {
		t.Fatalf("expected query cache cleared")
	}
}

func TestQueryLogging(t *testing.T) {
	ctx, prof := testContext(t)
	// Exercise the real query path against a loopback stand-in address.
	prof.Net.AllowNonGlobalTargets = true
	log := logger.FromContext(ctx)

	ns := newNS(t, ctx, "ns.example", "127.0.0.1")

	// Use a very short timeout since we expect network failure
	timeout := 10 * time.Millisecond
	opts := &QueryOptions{Timeout: &timeout}

	// This query will likely fail due to no server at 127.0.0.1:53, but logging happens before exchange
	_, _ = ns.QueryWithOptions(ctx, "example.com", "SOA", opts)

	loggedArgs := dnstest.RequireEntryByTag(t, log.Entries(), "EXTERNAL_QUERY").Args
	if name, ok := loggedArgs["query_name"]; !ok || name != "example.com" {
		t.Errorf("expected query_name=example.com, got %v", name)
	}
	if qtype, ok := loggedArgs["query_type"]; !ok || qtype != "SOA" {
		t.Errorf("expected query_type=SOA, got %v", qtype)
	}
	if qclass, ok := loggedArgs["query_class"]; !ok || qclass != "IN" {
		t.Errorf("expected query_class=IN, got %v", qclass)
	}
	if address, ok := loggedArgs["address"]; !ok || address != "127.0.0.1" {
		t.Errorf("expected address=127.0.0.1, got %v", address)
	}
	if _, ok := loggedArgs["ip"]; ok {
		t.Errorf("legacy key ip should not be present, got %v", loggedArgs["ip"])
	}
	if flags, ok := loggedArgs["flags"]; !ok || flags != "{\"class\":\"IN\"}" {
		t.Errorf("expected flags={\"class\":\"IN\"}, got %v", flags)
	}
}

func TestConstructorEmitsCreationLogs(t *testing.T) {
	ctx, _ := testContext(t)
	log := logger.FromContext(ctx)

	newNS(t, ctx, "ns1.example", "192.0.2.77")
	newNS(t, ctx, "ns2.example", "192.0.2.77")

	var cacheCreated, cacheFetched, nsCreated int
	seenNS := map[string]bool{}
	for _, entry := range log.Entries() {
		if entry == nil {
			continue
		}
		switch entry.Tag {
		case "CACHE_CREATED":
			cacheCreated++
		case "CACHE_FETCHED":
			cacheFetched++
		case "NS_CREATED":
			nsCreated++
			ns, _ := entry.Args["ns"].(string)
			address, _ := entry.Args["address"].(string)
			if ns == "" || address == "" {
				t.Fatalf("expected typed ns/address args in NS_CREATED, got %#v", entry.Args)
			}
			seenNS[ns] = true
			if _, ok := entry.Args["name"]; ok {
				t.Fatalf("legacy key name should not be present: %#v", entry.Args)
			}
		}
	}
	if cacheCreated != 1 {
		t.Fatalf("expected 1 CACHE_CREATED, got %d", cacheCreated)
	}
	if cacheFetched != 1 {
		t.Fatalf("expected 1 CACHE_FETCHED, got %d", cacheFetched)
	}
	if nsCreated != 2 {
		t.Fatalf("expected 2 NS_CREATED, got %d", nsCreated)
	}
	if !seenNS["ns1.example"] || !seenNS["ns2.example"] {
		t.Fatalf("expected NS_CREATED entries for ns1/ns2.example, got %#v", seenNS)
	}
}

func TestQueryEmitsQueryAndCachedReturn(t *testing.T) {
	ctx, _ := testContext(t)
	log := logger.FromContext(ctx)

	ns := newNS(t, ctx, "ns.example", "192.0.2.80")

	var calls int
	ns.SetQueryHook(func(_ context.Context, qname string, qtype string, qclass string, _ *QueryOptions) (packet.Packet, error) {
		calls++
		msg := new(dns.Msg)
		dnsutil.SetQuestion(msg, dnsutil.Fqdn(qname), dns.StringToType[qtype])
		msg.Rcode = dns.RcodeSuccess
		return packet.Packet{Msg: msg}, nil
	})

	if _, err := ns.QueryWithOptions(ctx, "example", "A", nil); err != nil {
		t.Fatalf("query 1: %v", err)
	}
	if _, err := ns.QueryWithOptions(ctx, "example", "A", nil); err != nil {
		t.Fatalf("query 2: %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected 1 network call due to cache hit, got %d", calls)
	}

	var queryCount, cachedReturnCount int
	for _, entry := range log.Entries() {
		if entry == nil {
			continue
		}
		switch entry.Tag {
		case "QUERY":
			queryCount++
		case "CACHED_RETURN":
			cachedReturnCount++
		}
	}
	if queryCount != 2 {
		t.Fatalf("expected 2 QUERY logs, got %d", queryCount)
	}
	if cachedReturnCount != 2 {
		t.Fatalf("expected 2 CACHED_RETURN logs, got %d", cachedReturnCount)
	}
}

func TestQueryCacheKeyPreservesQNameCase(t *testing.T) {
	ctx, _ := testContext(t)

	ns := newNS(t, ctx, "ns.example", "192.0.2.81")

	var calls int
	ns.SetQueryHook(func(_ context.Context, qname string, qtype string, _ string, _ *QueryOptions) (packet.Packet, error) {
		calls++
		msg := new(dns.Msg)
		dnsutil.SetQuestion(msg, dnsutil.Fqdn(qname), dns.StringToType[qtype])
		return packet.Packet{Msg: msg}, nil
	})

	mixed, err := ns.QueryWithOptions(ctx, "ExAmPlE.com", "A", nil)
	if err != nil {
		t.Fatalf("mixed query: %v", err)
	}
	lower, err := ns.QueryWithOptions(ctx, "example.com", "A", nil)
	if err != nil {
		t.Fatalf("lower query: %v", err)
	}
	if calls != 2 {
		t.Fatalf("expected differently cased QNAMEs to use distinct cache entries, got %d network calls", calls)
	}
	if got := mixed.Question()[0].Header().Name; got != "ExAmPlE.com." {
		t.Fatalf("mixed response question = %q, want ExAmPlE.com.", got)
	}
	if got := lower.Question()[0].Header().Name; got != "example.com." {
		t.Fatalf("lower response question = %q, want example.com.", got)
	}

	if _, err := ns.QueryWithOptions(ctx, "ExAmPlE.com", "A", nil); err != nil {
		t.Fatalf("mixed query cache hit: %v", err)
	}
	if calls != 2 {
		t.Fatalf("expected exact-case repeat to hit cache, got %d network calls", calls)
	}
}

func TestQueryLogsIPBlocked(t *testing.T) {
	ctx, prof := testContext(t)
	log := logger.FromContext(ctx)
	prof.Net.IPv4 = false

	ns := newNS(t, ctx, "ns.example", "192.0.2.81")
	if _, err := ns.QueryWithOptions(ctx, "example", "A", nil); err != nil {
		t.Fatalf("query with ipv4 disabled: %v", err)
	}

	if !dnstest.HasTag(log.Entries(), "IPV4_BLOCKED") {
		t.Fatalf("expected IPV4_BLOCKED log entry")
	}
}

// TestSkipShortCircuitPriorityOrder pins the relative priority of the five
// "give up on this server" mechanisms: reachability beats error-cache beats
// blacklisting beats fast-fail beats the latency budget. Refactors of
// QueryWithOptions's early-return ladder must not change which tag wins when
// multiple short-circuits would fire on the same query.
func TestSkipShortCircuitPriorityOrder(t *testing.T) {
	t.Run("blacklist beats fast-fail", func(t *testing.T) {
		ctx, prof := testContext(t)
		// Isolate the skip ladder from the non-global address guard; these
		// subtests use non-global stand-in addresses the guard would block.
		prof.Net.AllowNonGlobalTargets = true
		prof.Resolver.Defaults.ErrorCacheTTL = 0
		prof.Resolver.Defaults.FastFailTimeoutCount = 1
		log := logger.FromContext(ctx)

		ns := newNS(t, ctx, "ns.example", "192.0.2.180")
		// Trip both fast-fail and blacklisting up front.
		ns.state.fastFail.observeResult(false, true, 1)
		ns.state.blacklisted[false] = true

		_, _ = ns.QueryWithOptions(ctx, "example", "A", &QueryOptions{BlacklistingDisabled: false})
		dnstest.AssertOnlyTag(t, log.Entries(), "IS_BLACKLISTED", []string{"FAST_FAIL_SKIP", "REACHABILITY_CACHE_SKIP", "ERROR_CACHE_SKIP", "LATENCY_BUDGET_SKIP"})
	})

	t.Run("error-cache beats blacklist", func(t *testing.T) {
		ctx, prof := testContext(t)
		prof.Net.AllowNonGlobalTargets = true
		prof.Resolver.Defaults.ErrorCacheTTL = 60
		prof.Resolver.Defaults.FastFailTimeoutCount = 0
		log := logger.FromContext(ctx)

		ns := newNS(t, ctx, "ns.example", "192.0.2.181")
		key, _, _, err := buildCacheKey("example", "A", "IN", nil)
		if err != nil {
			t.Fatalf("build cache key: %v", err)
		}
		ns.state.errorCache.set(key, 60*time.Second)
		ns.state.blacklisted[false] = true

		_, _ = ns.QueryWithOptions(ctx, "example", "A", nil)
		dnstest.AssertOnlyTag(t, log.Entries(), "ERROR_CACHE_SKIP", []string{"IS_BLACKLISTED", "FAST_FAIL_SKIP", "REACHABILITY_CACHE_SKIP", "LATENCY_BUDGET_SKIP"})
	})

	t.Run("reachability beats error-cache", func(t *testing.T) {
		ctx, prof := testContext(t)
		prof.Net.AllowNonGlobalTargets = true
		prof.Resolver.Defaults.NegativeCacheTTL = 60
		prof.Resolver.Defaults.ErrorCacheTTL = 60
		log := logger.FromContext(ctx)

		ns := newNS(t, ctx, "ns.example", "192.0.2.182")
		// Two marks promote pending to blocked, matching production semantics.
		ns.state.reachability.mark(ns.Address.String(), 60*time.Second)
		ns.state.reachability.mark(ns.Address.String(), 60*time.Second)
		key, _, _, err := buildCacheKey("example", "A", "IN", nil)
		if err != nil {
			t.Fatalf("build cache key: %v", err)
		}
		ns.state.errorCache.set(key, 60*time.Second)

		_, _ = ns.QueryWithOptions(ctx, "example", "A", nil)
		dnstest.AssertOnlyTag(t, log.Entries(), "REACHABILITY_CACHE_SKIP", []string{"ERROR_CACHE_SKIP", "IS_BLACKLISTED", "FAST_FAIL_SKIP", "LATENCY_BUDGET_SKIP"})
	})

	t.Run("fast-fail beats latency-budget", func(t *testing.T) {
		ctx, prof := testContext(t)
		prof.Net.AllowNonGlobalTargets = true
		prof.Resolver.Defaults.ErrorCacheTTL = 0
		prof.Resolver.Defaults.FastFailTimeoutCount = 1
		prof.Resolver.Defaults.NameserverMaxTotalMS = 1
		log := logger.FromContext(ctx)

		ns := newNS(t, ctx, "ns.example", "192.0.2.183")
		// Trip both fast-fail and the latency budget up front; fast-fail is
		// checked first, so its tag must win.
		ns.state.fastFail.observeResult(false, true, 1)
		ns.state.latency.observe(time.Second, time.Millisecond)

		_, _ = ns.QueryWithOptions(ctx, "example", "A", &QueryOptions{BlacklistingDisabled: true})
		dnstest.AssertOnlyTag(t, log.Entries(), "FAST_FAIL_SKIP", []string{"LATENCY_BUDGET_SKIP", "IS_BLACKLISTED", "REACHABILITY_CACHE_SKIP", "ERROR_CACHE_SKIP"})
	})
}

func TestBlacklistingEmitsTags(t *testing.T) {
	ctx, prof := testContext(t)
	prof.Resolver.Defaults.ErrorCacheTTL = 0
	log := logger.FromContext(ctx)

	ns := newNS(t, ctx, "ns.example", "192.0.2.90")
	ns.SetQueryHook(func(_ context.Context, _ string, _ string, _ string, _ *QueryOptions) (packet.Packet, error) {
		return packet.Packet{}, fmt.Errorf("timeout")
	})

	// First SOA query triggers blacklisting.
	_, _ = ns.QueryWithOptions(ctx, "example", "SOA", nil)
	// Different qname so the second query bypasses the per-query timeout cache
	// and reaches the IS_BLACKLISTED short-circuit.
	_, _ = ns.QueryWithOptions(ctx, "other.example", "SOA", nil)

	var hasBlacklisting, hasIsBlacklisted bool
	for _, entry := range log.Entries() {
		if entry == nil {
			continue
		}
		switch entry.Tag {
		case "BLACKLISTING":
			hasBlacklisting = true
		case "IS_BLACKLISTED":
			hasIsBlacklisted = true
		}
	}
	if !hasBlacklisting {
		t.Fatalf("expected BLACKLISTING tag after failed SOA query")
	}
	if !hasIsBlacklisted {
		t.Fatalf("expected IS_BLACKLISTED tag on second query to blacklisted NS")
	}
}

func TestPacketBigEmitted(t *testing.T) {
	ctx, _ := testContext(t)
	log := logger.FromContext(ctx)

	ns := newNS(t, ctx, "ns.example", "192.0.2.91")
	ns.SetQueryHook(func(_ context.Context, _ string, _ string, _ string, _ *QueryOptions) (packet.Packet, error) {
		msg := new(dns.Msg)
		msg.Rcode = dns.RcodeSuccess
		// Add enough records to exceed 4096 bytes.
		for i := range 250 {
			rr := &dns.A{Hdr: dns.Header{Name: fmt.Sprintf("host%d.example.", i), Class: dns.ClassINET, TTL: 300}}
			rr.Addr = netip.AddrFrom4([4]byte{192, 0, 2, byte(i % 256)})
			msg.Answer = append(msg.Answer, rr)
		}
		return packet.Packet{Msg: msg}, nil
	})

	_, err := ns.QueryWithOptions(ctx, "example", "A", nil)
	if err != nil {
		t.Fatalf("query: %v", err)
	}

	for _, entry := range log.Entries() {
		if entry != nil && entry.Tag == "PACKET_BIG" {
			if size, ok := entry.Args["size"]; ok {
				if s, ok := size.(int); ok && s > 4096 {
					return
				}
			}
		}
	}
	t.Fatalf("expected PACKET_BIG tag for large response")
}

// testContext is testhelpers.Context for this package, which cannot import it:
// testhelpers imports nameserver.
func testContext(t *testing.T) (context.Context, *profile.Profile) {
	t.Helper()
	ctx, prof, _ := dnstest.Context(t)
	return WithCache(ctx, NewCacheStore()), prof
}

// queryHook is the SetQueryHook signature.
type queryHook = func(ctx context.Context, name string, qtype string, qclass string, opts *QueryOptions) (packet.Packet, error)

// newNS builds a nameserver on the cache store carried by ctx.
func newNS(t *testing.T, ctx context.Context, name string, addr string) Nameserver {
	t.Helper()
	ns, err := NewWithContext(ctx, name, addr, nil)
	if err != nil {
		t.Fatalf("new nameserver %s/%s: %v", name, addr, err)
	}
	return ns
}

// newCacheNS builds a nameserver on store.
func newCacheNS(t *testing.T, store *CacheStore, name string, addr string) Nameserver {
	t.Helper()
	ns, err := NewWithCache(store, name, addr, nil)
	if err != nil {
		t.Fatalf("new nameserver %s/%s: %v", name, addr, err)
	}
	return ns
}

// hookedNS builds a nameserver on store answering through hook.
func hookedNS(t *testing.T, store *CacheStore, name string, addr string, hook queryHook) Nameserver {
	t.Helper()
	ns := newCacheNS(t, store, name, addr)
	ns.SetQueryHook(hook)
	return ns
}

// freshNSSeq returns a factory handing out distinct nameserver names on one
// store and address, so the address-keyed caches are not short-circuited.
func freshNSSeq(t *testing.T, store *CacheStore, addr string) func(queryHook) Nameserver {
	t.Helper()
	seq := 0
	return func(hook queryHook) Nameserver {
		seq++
		return hookedNS(t, store, fmt.Sprintf("ns%d.example", seq), addr, hook)
	}
}

func TestNonGlobalQueryGuard(t *testing.T) {
	const blockTag = "NON_GLOBAL_QUERY_BLOCKED"
	timeout := 10 * time.Millisecond
	opts := &QueryOptions{Timeout: &timeout}

	// Guard on (default): a discovered non-global address is not dialed. The
	// guard logs NON_GLOBAL_QUERY_BLOCKED and never emits EXTERNAL_QUERY. The
	// v4-mapped form locks the Unmap step.
	for _, addr := range []string{"192.168.0.1", "10.0.0.1", "::ffff:127.0.0.1"} {
		ctx, _ := testContext(t)
		log := logger.FromContext(ctx)
		ns := newNS(t, ctx, "ns.example", addr)
		resp, err := ns.QueryWithOptions(ctx, "example.com", "SOA", opts)
		if err != nil {
			t.Errorf("%s: expected nil error from blocked query, got %v", addr, err)
		}
		if resp.Msg != nil {
			t.Errorf("%s: expected empty packet from blocked query", addr)
		}
		if !dnstest.HasTag(log.Entries(), blockTag) {
			t.Errorf("%s: expected %s to be logged", addr, blockTag)
		}
		if dnstest.HasTag(log.Entries(), "EXTERNAL_QUERY") {
			t.Errorf("%s: blocked query must not emit EXTERNAL_QUERY", addr)
		}
	}

	// Guard disabled (net.allow_non_global_targets=true): the same non-global
	// addresses are no longer blocked; the query is attempted (EXTERNAL_QUERY
	// logged before the dial) and the block tag is never emitted.
	for _, addr := range []string{"127.0.0.1", "10.0.0.1", "::ffff:192.168.0.1"} {
		ctx, prof := testContext(t)
		prof.Net.AllowNonGlobalTargets = true
		log := logger.FromContext(ctx)
		ns := newNS(t, ctx, "ns.example", addr)
		_, _ = ns.QueryWithOptions(ctx, "example.com", "SOA", opts)
		if dnstest.HasTag(log.Entries(), blockTag) {
			t.Errorf("%s: guard disabled, did not expect %s", addr, blockTag)
		}
		if !dnstest.HasTag(log.Entries(), "EXTERNAL_QUERY") {
			t.Errorf("%s: guard disabled, expected the query to be attempted", addr)
		}
	}

	// Operator allow-set (e.g. a pinned undelegated address) is exempt even
	// while the guard is on.
	{
		ctx, _ := testContext(t)
		ctx = profile.WithAllowedTargets(ctx, map[netip.Addr]struct{}{
			netip.MustParseAddr("127.0.0.1"): {},
		})
		log := logger.FromContext(ctx)
		ns := newNS(t, ctx, "ns.example", "127.0.0.1")
		_, _ = ns.QueryWithOptions(ctx, "example.com", "SOA", opts)
		if dnstest.HasTag(log.Entries(), blockTag) {
			t.Errorf("allow-set: did not expect %s", blockTag)
		}
		if !dnstest.HasTag(log.Entries(), "EXTERNAL_QUERY") {
			t.Errorf("allow-set: expected the query to be attempted")
		}
	}
}

// Msg.Z is an OPT trigger in Pack, so a residual value would emit a second OPT
// alongside the explicit one and make the message unparseable.
func TestSetMessageEDNSZClearsResidualZ(t *testing.T) {
	msg := new(dns.Msg)
	msg.UDPSize = 512
	msg.Z = 3

	setMessageEDNSZ(msg, 1)
	if msg.Z != 0 {
		t.Fatalf("Msg.Z = %d after setMessageEDNSZ, want 0", msg.Z)
	}

	if err := msg.Pack(); err != nil {
		t.Fatalf("pack: %v", err)
	}
	wire := new(dns.Msg)
	wire.Data = msg.Data
	if err := wire.Unpack(); err != nil {
		t.Fatalf("unpack: %v; the message carried more than one OPT", err)
	}
	if wire.Z != 1 {
		t.Fatalf("Z = %d over the wire, want 1", wire.Z)
	}
}
