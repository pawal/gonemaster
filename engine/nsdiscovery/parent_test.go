package nsdiscovery

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"codeberg.org/pawal/gonemaster/engine/internal/dnstest"
	"codeberg.org/pawal/gonemaster/engine/internal/testhelpers"
	"codeberg.org/pawal/gonemaster/engine/nameserver"
	"codeberg.org/pawal/gonemaster/engine/packet"
	"codeberg.org/pawal/gonemaster/engine/recursor"
	"codeberg.org/pawal/gonemaster/engine/zone"
)

// Tests for ParentNameservers and the per-context Cache.

func TestParentCacheStoresSnapshotData(t *testing.T) {
	cache := NewCache()
	ctx, _, _ := testhelpers.Context(t)
	ctx = WithCache(ctx, cache)

	ns, err := nameserver.NewWithContext(ctx, "ns1.example", "192.0.2.53", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}

	cache.store("example", []nameserver.Nameserver{ns}, true)

	entry, ok := cache.lookup("example")
	if !ok {
		t.Fatalf("expected parent cache entry")
	}
	if !entry.defined {
		t.Fatalf("expected defined parent cache entry")
	}
	if len(entry.servers) != 1 {
		t.Fatalf("expected 1 cached parent server, got %d", len(entry.servers))
	}
	if entry.servers[0].Name != "ns1.example" {
		t.Fatalf("unexpected cached name %q", entry.servers[0].Name)
	}
	if entry.servers[0].Address != "192.0.2.53" {
		t.Fatalf("unexpected cached address %q", entry.servers[0].Address)
	}

	ctx2, _, _ := testhelpers.Context(t)
	servers := materializeParentServers(ctx2, nil, append(entry.servers, parentCacheServer{}))
	if len(servers) != 1 {
		t.Fatalf("expected malformed cached rows to be skipped, got %d materialized servers", len(servers))
	}
	if servers[0].Name.String() != "ns1.example" {
		t.Fatalf("unexpected materialized name %q", servers[0].Name.String())
	}
	if servers[0].Address.String() != "192.0.2.53" {
		t.Fatalf("unexpected materialized address %q", servers[0].Address.String())
	}
}

func TestParentNameserversUndelegated(t *testing.T) {
	ctx, _, _ := testhelpers.Context(t)
	ctx = WithCache(ctx, NewCache())

	r := &recursor.Recursor{}
	if err := r.AddFakeAddresses("example", map[string][]string{
		"ns1.example": {"192.0.2.1"},
	}); err != nil {
		t.Fatalf("add fake addresses: %v", err)
	}
	z, err := zone.NewWithRecursor("example", r)
	if err != nil {
		t.Fatalf("new zone: %v", err)
	}

	parent, err := ParentNameservers(ctx, &z)
	if err != nil {
		t.Fatalf("ParentNameservers: %v", err)
	}
	if parent == nil || len(parent) != 0 {
		t.Fatalf("expected empty parent list, got %#v", parent)
	}
}

func TestParentNameserversSkipsOnIntermediateNoResponse(t *testing.T) {
	ctx, prof, _ := testhelpers.Context(t)
	ctx = WithCache(ctx, NewCache())
	prof.Net.IPv4 = true
	prof.Net.IPv6 = true

	r := &recursor.Recursor{}
	if err := r.AddFakeAddresses(".", map[string][]string{
		"ns1.root": {"192.0.2.1"},
		"ns2.root": {"192.0.2.2"},
	}); err != nil {
		t.Fatalf("add fake root addresses: %v", err)
	}

	rootSOA := func(name string) packet.Packet {
		soa := dnstest.SOARR(name, dnstest.MName("ns.example."), dnstest.RName("hostmaster.example."))
		return dnstest.Response(dnstest.Answers(dnstest.TTL(0, soa)...))
	}

	rootNS := func() packet.Packet {
		return dnstest.Response(
			dnstest.Answers(dnstest.TTL(0, dnstest.NSRRs(".", "ns1.root.", "ns2.root.")...)...),
			dnstest.Additional(dnstest.TTL(0,
				dnstest.ARR("ns1.root.", "192.0.2.1"),
				dnstest.ARR("ns2.root.", "192.0.2.2"))...))
	}

	ns1, err := nameserver.NewWithContext(ctx, "ns1.root", "192.0.2.1", r.Client())
	if err != nil {
		t.Fatalf("new ns1: %v", err)
	}
	ns1.SetQueryHook(func(_ context.Context, name string, qtype string, _ string, _ *nameserver.QueryOptions) (packet.Packet, error) {
		switch {
		case name == "." && qtype == "SOA":
			return rootSOA("."), nil
		case name == "." && qtype == "NS":
			return rootNS(), nil
		case name == "example" && qtype == "SOA":
			return packet.Packet{}, nil
		default:
			return packet.Packet{}, nil
		}
	})

	ns2, err := nameserver.NewWithContext(ctx, "ns2.root", "192.0.2.2", r.Client())
	if err != nil {
		t.Fatalf("new ns2: %v", err)
	}
	ns2.SetQueryHook(func(_ context.Context, name string, qtype string, _ string, _ *nameserver.QueryOptions) (packet.Packet, error) {
		switch {
		case name == "." && qtype == "SOA":
			return rootSOA("."), nil
		case name == "." && qtype == "NS":
			return rootNS(), nil
		case name == "example" && qtype == "SOA":
			return rootSOA("example"), nil
		default:
			return packet.Packet{}, nil
		}
	})

	z, err := zone.NewWithRecursor("example", r)
	if err != nil {
		t.Fatalf("new zone: %v", err)
	}

	parent, err := ParentNameservers(ctx, &z)
	if err != nil {
		t.Fatalf("ParentNameservers: %v", err)
	}
	if len(parent) != 1 {
		t.Fatalf("expected 1 parent nameserver, got %d", len(parent))
	}
	if parent[0].String() != "ns2.root/192.0.2.2" {
		t.Fatalf("unexpected parent nameserver %q", parent[0].String())
	}
}

// TestParentNameserversAcceptsRFC8020ContradictionAtIntermediate covers the
// case where a parent NS returns NXDOMAIN+AA at an empty non-terminal above
// the queried zone but still returns a referral at the zone itself. The
// walker probes the child name and accepts the NS as the parent so
// downstream tests see a populated delegation view.
func TestParentNameserversAcceptsRFC8020ContradictionAtIntermediate(t *testing.T) {
	ctx, prof, _ := testhelpers.Context(t)
	ctx = WithCache(ctx, NewCache())
	prof.Net.IPv4 = true
	prof.Net.IPv6 = true

	r := &recursor.Recursor{}
	if err := r.AddFakeAddresses(".", map[string][]string{
		"ns.root": {"192.0.2.1"},
	}); err != nil {
		t.Fatalf("add fake root addresses: %v", err)
	}

	rootSOA := func() packet.Packet {
		soa := dnstest.SOARR(".", dnstest.MName("ns.root."), dnstest.RName("hostmaster."))
		return dnstest.Response(dnstest.Answers(dnstest.TTL(0, soa)...))
	}
	rootNS := func() packet.Packet {
		return dnstest.Response(
			dnstest.Answers(dnstest.TTL(0, dnstest.NSRR(".", "ns.root."))...),
			dnstest.Additional(dnstest.TTL(0, dnstest.ARR("ns.root.", "192.0.2.1"))...))
	}
	rootReferralExample := func() packet.Packet {
		return dnstest.Response(dnstest.NotAuthoritative(),
			dnstest.Authority(dnstest.TTL(0, dnstest.NSRR("example.", "ns.example."))...),
			dnstest.Additional(dnstest.TTL(0, dnstest.ARR("ns.example.", "192.0.2.2"))...))
	}

	exampleSOA := func() packet.Packet {
		soa := dnstest.SOARR("example.", dnstest.MName("ns.example."), dnstest.RName("hostmaster.example."))
		return dnstest.Response(dnstest.Answers(dnstest.TTL(0, soa)...))
	}
	exampleNS := func() packet.Packet {
		return dnstest.Response(
			dnstest.Answers(dnstest.TTL(0, dnstest.NSRR("example.", "ns.example."))...),
			dnstest.Additional(dnstest.TTL(0, dnstest.ARR("ns.example.", "192.0.2.2"))...))
	}
	nxdomainAA := func() packet.Packet {
		return dnstest.Response(dnstest.NXDOMAIN())
	}
	childReferral := func() packet.Packet {
		return dnstest.Response(dnstest.NotAuthoritative(),
			dnstest.Authority(dnstest.TTL(0, dnstest.NSRR("c.b.example.", "ns.child.example."))...))
	}

	nsRoot, err := nameserver.NewWithContext(ctx, "ns.root", "192.0.2.1", r.Client())
	if err != nil {
		t.Fatalf("new ns.root: %v", err)
	}
	nsRoot.SetQueryHook(func(_ context.Context, name string, qtype string, _ string, _ *nameserver.QueryOptions) (packet.Packet, error) {
		switch {
		case name == "." && qtype == "SOA":
			return rootSOA(), nil
		case name == "." && qtype == "NS":
			return rootNS(), nil
		case name == "example" && qtype == "SOA":
			return rootReferralExample(), nil
		}
		return packet.Packet{}, nil
	})

	nsExample, err := nameserver.NewWithContext(ctx, "ns.example", "192.0.2.2", r.Client())
	if err != nil {
		t.Fatalf("new ns.example: %v", err)
	}
	nsExample.SetQueryHook(func(_ context.Context, name string, qtype string, _ string, _ *nameserver.QueryOptions) (packet.Packet, error) {
		switch {
		case name == "example" && qtype == "SOA":
			return exampleSOA(), nil
		case name == "example" && qtype == "NS":
			return exampleNS(), nil
		case name == "b.example" && qtype == "SOA":
			return nxdomainAA(), nil
		case name == "c.b.example" && qtype == "SOA":
			return childReferral(), nil
		}
		return packet.Packet{}, nil
	})

	z, err := zone.NewWithRecursor("c.b.example", r)
	if err != nil {
		t.Fatalf("new zone: %v", err)
	}

	parent, err := ParentNameservers(ctx, &z)
	if err != nil {
		t.Fatalf("ParentNameservers: %v", err)
	}
	if len(parent) != 1 {
		t.Fatalf("expected 1 parent nameserver, got %d: %#v", len(parent), parent)
	}
	if parent[0].String() != "ns.example/192.0.2.2" {
		t.Fatalf("unexpected parent nameserver %q", parent[0].String())
	}
}

// TestParentNameserversUsesCacheOnSecondCall verifies that a pre-seeded cache
// entry is returned without traversing the delegation chain.
func TestParentNameserversUsesCacheOnSecondCall(t *testing.T) {
	cache := NewCache()
	ctx, _, _ := testhelpers.Context(t)
	ctx = WithCache(ctx, cache)

	r := &recursor.Recursor{}
	if err := r.AddFakeAddresses(".", map[string][]string{
		"a.root": {"192.0.2.1"},
	}); err != nil {
		t.Fatalf("add root: %v", err)
	}

	z, err := zone.NewWithRecursor("example.com", r)
	if err != nil {
		t.Fatalf("new zone: %v", err)
	}

	key := z.Name.String()
	seedParentCache(ctx, t, r, cache, key, "sentinel.ns.example", "203.0.113.99")

	out, err := ParentNameservers(ctx, &z)
	if err != nil {
		t.Fatalf("ParentNameservers: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("expected 1 cached server materialized, got %d: %#v", len(out), out)
	}
	if out[0].Name.String() != "sentinel.ns.example" || out[0].Address.String() != "203.0.113.99" {
		t.Fatalf("expected sentinel.ns.example/203.0.113.99, got %s/%s",
			out[0].Name.String(), out[0].Address.String())
	}
}

// TestCacheClearRemovesAllEntries verifies that Cache.Clear empties the cache.
func TestCacheClearRemovesAllEntries(t *testing.T) {
	cache := NewCache()
	ctx, _, _ := testhelpers.Context(t)
	ctx = WithCache(ctx, cache)

	r := &recursor.Recursor{}
	if err := r.AddFakeAddresses(".", map[string][]string{"a.root": {"192.0.2.1"}}); err != nil {
		t.Fatalf("add root: %v", err)
	}

	seedParentCache(ctx, t, r, cache, "example.com.", "ns.example", "203.0.113.1")
	seedParentCache(ctx, t, r, cache, "example.org.", "ns.example", "203.0.113.2")

	if before := cache.Len(); before != 2 {
		t.Fatalf("expected 2 cache entries before clear, got %d", before)
	}

	cache.Clear()

	if after := cache.Len(); after != 0 {
		t.Fatalf("expected empty cache after Clear, got %d entries", after)
	}
}

// TestParentNameserversCachesPerInstance verifies that two separate caches
// hold distinct state - the per-run isolation that replaces the old
// package-global cache.
func TestParentNameserversCachesPerInstance(t *testing.T) {
	cacheA := NewCache()
	cacheB := NewCache()
	ctx, _, _ := testhelpers.Context(t)

	r := &recursor.Recursor{}
	if err := r.AddFakeAddresses(".", map[string][]string{"a.root": {"192.0.2.1"}}); err != nil {
		t.Fatalf("add root: %v", err)
	}

	zA, err := zone.NewWithRecursor("alpha.test", r)
	if err != nil {
		t.Fatalf("new zone alpha: %v", err)
	}
	zB, err := zone.NewWithRecursor("beta.test", r)
	if err != nil {
		t.Fatalf("new zone beta: %v", err)
	}

	seedParentCache(ctx, t, r, cacheA, zA.Name.String(), "ns.alpha", "203.0.113.10")
	seedParentCache(ctx, t, r, cacheB, zB.Name.String(), "ns.beta", "203.0.113.20")

	ctxA := WithCache(ctx, cacheA)
	ctxB := WithCache(ctx, cacheB)

	outA, err := ParentNameservers(ctxA, &zA)
	if err != nil {
		t.Fatalf("parent alpha: %v", err)
	}
	outB, err := ParentNameservers(ctxB, &zB)
	if err != nil {
		t.Fatalf("parent beta: %v", err)
	}

	if len(outA) != 1 || outA[0].Name.String() != "ns.alpha" {
		t.Fatalf("alpha: expected ns.alpha, got %#v", outA)
	}
	if len(outB) != 1 || outB[0].Name.String() != "ns.beta" {
		t.Fatalf("beta: expected ns.beta, got %#v", outB)
	}

	// cacheA must not see beta's entry, and vice versa.
	if _, ok := cacheA.lookup(zB.Name.String()); ok {
		t.Fatalf("cacheA leaked beta zone entry")
	}
	if _, ok := cacheB.lookup(zA.Name.String()); ok {
		t.Fatalf("cacheB leaked alpha zone entry")
	}
}

// TestParentNameserversNoCacheStillWorks verifies that calling
// ParentNameservers without WithCache succeeds (no caching, but no panic).
func TestParentNameserversNoCacheStillWorks(t *testing.T) {
	ctx, _, _ := testhelpers.Context(t)

	r := &recursor.Recursor{}
	if err := r.AddFakeAddresses("example", map[string][]string{
		"ns1.example": {"192.0.2.1"},
	}); err != nil {
		t.Fatalf("add fake addresses: %v", err)
	}
	z, err := zone.NewWithRecursor("example", r)
	if err != nil {
		t.Fatalf("new zone: %v", err)
	}

	parent, err := ParentNameservers(ctx, &z)
	if err != nil {
		t.Fatalf("ParentNameservers: %v", err)
	}
	if parent == nil || len(parent) != 0 {
		t.Fatalf("expected empty parent list, got %#v", parent)
	}
}

// TestParentNameserversConcurrentCallsSameZone launches 50 goroutines on the
// same zone with a shared cache. Run under -race to detect missing mutex
// protection.
func TestParentNameserversConcurrentCallsSameZone(t *testing.T) {
	cache := NewCache()
	ctx, _, _ := testhelpers.Context(t)
	ctx = WithCache(ctx, cache)

	r := &recursor.Recursor{}
	if err := r.AddFakeAddresses(".", map[string][]string{"a.root": {"192.0.2.1"}}); err != nil {
		t.Fatalf("add root: %v", err)
	}

	z, err := zone.NewWithRecursor("example.com", r)
	if err != nil {
		t.Fatalf("new zone: %v", err)
	}

	seedParentCache(ctx, t, r, cache, z.Name.String(), "shared.ns", "203.0.113.42")

	const N = 50
	var wg sync.WaitGroup
	wg.Add(N)
	for i := 0; i < N; i++ {
		go func() {
			defer wg.Done()
			out, err := ParentNameservers(ctx, &z)
			if err != nil {
				t.Errorf("goroutine got error: %v", err)
				return
			}
			if len(out) != 1 || out[0].Name.String() != "shared.ns" {
				t.Errorf("goroutine got unexpected result: %#v", out)
			}
		}()
	}
	wg.Wait()
}

// TestParentNameserversConcurrentCallsDifferentZones launches goroutines
// spread across 10 distinct zones with their own pre-seeded cache entries.
func TestParentNameserversConcurrentCallsDifferentZones(t *testing.T) {
	cache := NewCache()
	ctx, _, _ := testhelpers.Context(t)
	ctx = WithCache(ctx, cache)

	r := &recursor.Recursor{}
	if err := r.AddFakeAddresses(".", map[string][]string{"a.root": {"192.0.2.1"}}); err != nil {
		t.Fatalf("add root: %v", err)
	}

	const NZones = 10
	zones := make([]zone.Zone, NZones)
	expectedNS := make([]string, NZones)
	for i := 0; i < NZones; i++ {
		name := fmt.Sprintf("zone%d.test", i)
		z, err := zone.NewWithRecursor(name, r)
		if err != nil {
			t.Fatalf("new zone %s: %v", name, err)
		}
		zones[i] = z
		nsName := fmt.Sprintf("ns.zone%d", i)
		expectedNS[i] = nsName
		seedParentCache(ctx, t, r, cache, z.Name.String(), nsName, fmt.Sprintf("203.0.113.%d", 10+i))
	}

	const PerZone = 5
	var wg sync.WaitGroup
	wg.Add(NZones * PerZone)
	for i := 0; i < NZones; i++ {
		for j := 0; j < PerZone; j++ {
			i := i
			go func() {
				defer wg.Done()
				out, err := ParentNameservers(ctx, &zones[i])
				if err != nil {
					t.Errorf("zone %d: %v", i, err)
					return
				}
				if len(out) != 1 || out[0].Name.String() != expectedNS[i] {
					t.Errorf("zone %d: expected %s, got %#v", i, expectedNS[i], out)
				}
			}()
		}
	}
	wg.Wait()

	if count := cache.Len(); count != NZones {
		t.Fatalf("expected %d cache entries, got %d", NZones, count)
	}
}

// TestCacheClearConcurrentWithParentNameservers exercises Cache.Clear under
// contention with concurrent ParentNameservers callers.
func TestCacheClearConcurrentWithParentNameservers(t *testing.T) {
	cache := NewCache()
	ctx, _, _ := testhelpers.Context(t)
	ctx = WithCache(ctx, cache)

	r := &recursor.Recursor{}
	if err := r.AddFakeAddresses(".", map[string][]string{"a.root": {"192.0.2.1"}}); err != nil {
		t.Fatalf("add root: %v", err)
	}

	z, err := zone.NewWithRecursor("example.com", r)
	if err != nil {
		t.Fatalf("new zone: %v", err)
	}

	seedParentCache(ctx, t, r, cache, z.Name.String(), "ns.example", "203.0.113.50")

	const Iterations = 200
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < Iterations; i++ {
			cache.Clear()
			seedParentCache(ctx, t, r, cache, z.Name.String(), "ns.example", "203.0.113.50")
		}
	}()

	const NReaders = 20
	wg.Add(NReaders)
	for i := 0; i < NReaders; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < Iterations; j++ {
				_, _ = ParentNameservers(ctx, &z)
			}
		}()
	}

	wg.Wait()
}
