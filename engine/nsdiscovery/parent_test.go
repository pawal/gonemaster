package nsdiscovery

import (
	"context"
	"fmt"
	"net/netip"
	"sync"
	"testing"

	dns "codeberg.org/miekg/dns"
	"codeberg.org/miekg/dns/dnsutil"

	"codeberg.org/pawal/gonemaster/engine/internal/testhelpers"
	"codeberg.org/pawal/gonemaster/engine/nameserver"
	"codeberg.org/pawal/gonemaster/engine/packet"
	"codeberg.org/pawal/gonemaster/engine/recursor"
	"codeberg.org/pawal/gonemaster/engine/zone"
)

// Tests for ParentNameservers and the parent NS cache.

func TestParentCacheStoresSnapshotData(t *testing.T) {
	ClearParentNSCache()
	defer ClearParentNSCache()

	ctx, _, _ := testhelpers.Context(t)
	ns, err := nameserver.NewWithContext(ctx, "ns1.example", "192.0.2.53", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}

	cacheParent("example", []nameserver.Nameserver{ns}, true)

	parentCache.mu.Lock()
	entry, ok := parentCache.items["example"]
	parentCache.mu.Unlock()
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
	ClearParentNSCache()
	defer ClearParentNSCache()
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

func TestParentNameserversSkipsOnIntermediateNoResponse(t *testing.T) {
	ClearParentNSCache()
	defer ClearParentNSCache()
	ctx, prof, _ := testhelpers.Context(t)
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
		msg := new(dns.Msg)
		msg.Rcode = dns.RcodeSuccess
		msg.Authoritative = true
		soaRR := &dns.SOA{Hdr: dns.Header{Name: dnsutil.Fqdn(name), Class: dns.ClassINET}}
		soaRR.Ns = "ns.example."
		soaRR.Mbox = "hostmaster.example."
		soaRR.Serial = 1
		msg.Answer = []dns.RR{soaRR}
		return packet.Packet{Msg: msg}
	}

	rootNS := func() packet.Packet {
		msg := new(dns.Msg)
		msg.Rcode = dns.RcodeSuccess
		msg.Authoritative = true
		ns1RR := &dns.NS{}
		ns1RR.Hdr = dns.Header{Name: ".", Class: dns.ClassINET}
		ns1RR.Ns = "ns1.root."
		ns2RR := &dns.NS{}
		ns2RR.Hdr = dns.Header{Name: ".", Class: dns.ClassINET}
		ns2RR.Ns = "ns2.root."
		msg.Answer = []dns.RR{ns1RR, ns2RR}
		a1RR := &dns.A{}
		a1RR.Hdr = dns.Header{Name: "ns1.root.", Class: dns.ClassINET}
		a1RR.Addr = netip.AddrFrom4([4]byte{192, 0, 2, 1})
		a2RR := &dns.A{}
		a2RR.Hdr = dns.Header{Name: "ns2.root.", Class: dns.ClassINET}
		a2RR.Addr = netip.AddrFrom4([4]byte{192, 0, 2, 2})
		msg.Extra = []dns.RR{a1RR, a2RR}
		return packet.Packet{Msg: msg}
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

// TestParentNameserversUsesCacheOnSecondCall verifies that a pre-seeded cache
// entry is returned without traversing the delegation chain.
func TestParentNameserversUsesCacheOnSecondCall(t *testing.T) {
	ClearParentNSCache()
	defer ClearParentNSCache()
	ctx, _, _ := testhelpers.Context(t)

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
	seedParentCache(ctx, t, r, key, "sentinel.ns.example", "203.0.113.99")

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

// TestClearParentNSCacheRemovesAllEntries verifies that ClearParentNSCache
// empties the global parent cache map.
func TestClearParentNSCacheRemovesAllEntries(t *testing.T) {
	ClearParentNSCache()
	defer ClearParentNSCache()
	ctx, _, _ := testhelpers.Context(t)

	r := &recursor.Recursor{}
	if err := r.AddFakeAddresses(".", map[string][]string{"a.root": {"192.0.2.1"}}); err != nil {
		t.Fatalf("add root: %v", err)
	}

	seedParentCache(ctx, t, r, "example.com.", "ns.example", "203.0.113.1")
	seedParentCache(ctx, t, r, "example.org.", "ns.example", "203.0.113.2")

	parentCache.mu.Lock()
	before := len(parentCache.items)
	parentCache.mu.Unlock()
	if before != 2 {
		t.Fatalf("expected 2 cache entries before clear, got %d", before)
	}

	ClearParentNSCache()

	parentCache.mu.Lock()
	after := len(parentCache.items)
	parentCache.mu.Unlock()
	if after != 0 {
		t.Fatalf("expected empty cache after ClearParentNSCache, got %d entries", after)
	}
}

// TestParentNameserversCacheIsolatedPerZone verifies that cache entries for
// two distinct zones do not bleed into each other.
func TestParentNameserversCacheIsolatedPerZone(t *testing.T) {
	ClearParentNSCache()
	defer ClearParentNSCache()
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

	seedParentCache(ctx, t, r, zA.Name.String(), "ns.alpha", "203.0.113.10")
	seedParentCache(ctx, t, r, zB.Name.String(), "ns.beta", "203.0.113.20")

	outA, err := ParentNameservers(ctx, &zA)
	if err != nil {
		t.Fatalf("parent alpha: %v", err)
	}
	outB, err := ParentNameservers(ctx, &zB)
	if err != nil {
		t.Fatalf("parent beta: %v", err)
	}

	if len(outA) != 1 || outA[0].Name.String() != "ns.alpha" {
		t.Fatalf("alpha: expected ns.alpha, got %#v", outA)
	}
	if len(outB) != 1 || outB[0].Name.String() != "ns.beta" {
		t.Fatalf("beta: expected ns.beta, got %#v", outB)
	}
}

// TestParentNameserversCacheSurvivesAcrossContexts verifies the cache lookup
// is keyed by zone name only.
func TestParentNameserversCacheSurvivesAcrossContexts(t *testing.T) {
	ClearParentNSCache()
	defer ClearParentNSCache()
	ctx1, _, _ := testhelpers.Context(t)
	ctx2, _, _ := testhelpers.Context(t)

	r := &recursor.Recursor{}
	if err := r.AddFakeAddresses(".", map[string][]string{"a.root": {"192.0.2.1"}}); err != nil {
		t.Fatalf("add root: %v", err)
	}

	z, err := zone.NewWithRecursor("example.com", r)
	if err != nil {
		t.Fatalf("new zone: %v", err)
	}

	seedParentCache(ctx1, t, r, z.Name.String(), "sentinel.ns", "203.0.113.7")

	out1, err := ParentNameservers(ctx1, &z)
	if err != nil {
		t.Fatalf("ctx1: %v", err)
	}
	out2, err := ParentNameservers(ctx2, &z)
	if err != nil {
		t.Fatalf("ctx2: %v", err)
	}
	if len(out1) != 1 || len(out2) != 1 {
		t.Fatalf("expected both contexts to hit cache; got %d and %d", len(out1), len(out2))
	}
	if out1[0].Name.String() != "sentinel.ns" || out2[0].Name.String() != "sentinel.ns" {
		t.Fatalf("expected sentinel.ns from both contexts, got %s and %s",
			out1[0].Name.String(), out2[0].Name.String())
	}
}

// TestParentNameserversConcurrentCallsSameZone launches 50 goroutines on the
// same zone. Run under -race to detect missing mutex protection.
func TestParentNameserversConcurrentCallsSameZone(t *testing.T) {
	ClearParentNSCache()
	defer ClearParentNSCache()
	ctx, _, _ := testhelpers.Context(t)

	r := &recursor.Recursor{}
	if err := r.AddFakeAddresses(".", map[string][]string{"a.root": {"192.0.2.1"}}); err != nil {
		t.Fatalf("add root: %v", err)
	}

	z, err := zone.NewWithRecursor("example.com", r)
	if err != nil {
		t.Fatalf("new zone: %v", err)
	}

	seedParentCache(ctx, t, r, z.Name.String(), "shared.ns", "203.0.113.42")

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
	ClearParentNSCache()
	defer ClearParentNSCache()
	ctx, _, _ := testhelpers.Context(t)

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
		seedParentCache(ctx, t, r, z.Name.String(), nsName, fmt.Sprintf("203.0.113.%d", 10+i))
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

	parentCache.mu.Lock()
	count := len(parentCache.items)
	parentCache.mu.Unlock()
	if count != NZones {
		t.Fatalf("expected %d cache entries, got %d", NZones, count)
	}
}

// TestClearParentNSCacheConcurrentWithParentNameservers runs ClearParentNSCache
// repeatedly while many readers call ParentNameservers.
func TestClearParentNSCacheConcurrentWithParentNameservers(t *testing.T) {
	ClearParentNSCache()
	defer ClearParentNSCache()
	ctx, _, _ := testhelpers.Context(t)

	r := &recursor.Recursor{}
	if err := r.AddFakeAddresses(".", map[string][]string{"a.root": {"192.0.2.1"}}); err != nil {
		t.Fatalf("add root: %v", err)
	}

	z, err := zone.NewWithRecursor("example.com", r)
	if err != nil {
		t.Fatalf("new zone: %v", err)
	}

	seedParentCache(ctx, t, r, z.Name.String(), "ns.example", "203.0.113.50")

	const Iterations = 200
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < Iterations; i++ {
			ClearParentNSCache()
			seedParentCache(ctx, t, r, z.Name.String(), "ns.example", "203.0.113.50")
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
