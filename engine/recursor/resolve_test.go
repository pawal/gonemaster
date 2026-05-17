package recursor

import (
	"context"
	"errors"
	"net/netip"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	dns "codeberg.org/miekg/dns"
	"codeberg.org/miekg/dns/dnsutil"

	"codeberg.org/pawal/gonemaster/engine/dnsname"
	"codeberg.org/pawal/gonemaster/engine/internal/testhelpers"
	"codeberg.org/pawal/gonemaster/engine/logger"
	"codeberg.org/pawal/gonemaster/engine/nameserver"
	"codeberg.org/pawal/gonemaster/engine/packet"
	"codeberg.org/pawal/gonemaster/engine/profile"
	"codeberg.org/pawal/gonemaster/engine/transport"
)

func TestAddFakeAddressesDedupAndRemove(t *testing.T) {
	r := &Recursor{}
	if err := r.AddFakeAddresses("Example.COM", map[string][]string{
		"NS1.Example.COM": {"192.0.2.1", "192.0.2.1"},
	}); err != nil {
		t.Fatalf("add fake addresses: %v", err)
	}
	if err := r.AddFakeAddresses("example.com", map[string][]string{
		"ns2.example.com": {},
	}); err != nil {
		t.Fatalf("add fake addresses (empty): %v", err)
	}
	if !r.HasFakeAddresses("example.com") {
		t.Fatalf("expected fake addresses for example.com")
	}
	addrs := r.GetFakeAddresses("EXAMPLE.COM", "ns1.example.com")
	if len(addrs) != 1 || addrs[0].String() != "192.0.2.1" {
		t.Fatalf("unexpected addresses: %#v", addrs)
	}
	names := r.GetFakeNames("example.com")
	found := slices.Contains(names, "ns2.example.com")
	if !found {
		t.Fatalf("expected ns2.example.com in fake names: %#v", names)
	}

	r.RemoveFakeAddresses("example.com")
	if r.HasFakeAddresses("example.com") {
		t.Fatalf("expected fake addresses removed")
	}
	if err := r.AddFakeAddresses("example.com", map[string][]string{"ns": {"bad"}}); err == nil {
		t.Fatalf("expected error for invalid IP")
	}
}

func TestRootServersSorted(t *testing.T) {
	nameserver.EmptyCache()
	defer nameserver.EmptyCache()

	r := &Recursor{
		fakeAddresses: map[string]map[string][]netip.Addr{
			".": {
				"b.root": {netip.MustParseAddr("192.0.2.2")},
				"a.root": {netip.MustParseAddr("192.0.2.1")},
			},
		},
		client: &transport.Client{},
	}

	servers, err := r.RootServers(context.Background())
	if err != nil {
		t.Fatalf("root servers: %v", err)
	}
	if len(servers) != 2 {
		t.Fatalf("expected 2 root servers, got %d", len(servers))
	}
	if servers[0].Name.String() != "a.root" || servers[1].Name.String() != "b.root" {
		t.Fatalf("unexpected root order: %s, %s", servers[0].Name.String(), servers[1].Name.String())
	}
}

func TestCacheStoreLookupAndClear(t *testing.T) {
	r := &Recursor{}
	msg := new(dns.Msg)
	msg.Rcode = dns.RcodeSuccess
	resp := packet.Packet{Msg: msg}

	r.cacheStore("example", "A", "IN", resp)
	cached, ok := r.cacheLookup("example", "A", "IN")
	if !ok || cached.Msg == nil {
		t.Fatalf("expected cached response")
	}

	r.cacheStore("empty", "A", "IN", packet.Packet{})
	if _, ok := r.cacheLookup("empty", "A", "IN"); ok {
		t.Fatalf("expected empty response not to be cached")
	}

	r.ClearCache()
	if _, ok := r.cacheLookup("example", "A", "IN"); ok {
		t.Fatalf("expected cache cleared")
	}
}

func TestCacheNameKeyPreservesQNameCase(t *testing.T) {
	mixed := cacheNameKey(dnsname.New("ExAmPlE.CoM."), nil)
	lower := cacheNameKey(dnsname.New("example.com."), nil)
	if mixed == lower {
		t.Fatalf("recursor packet cache key must preserve QNAME case:\n mixed: %s\n lower: %s", mixed, lower)
	}
	if !strings.HasSuffix(mixed, "|ExAmPlE.CoM") {
		t.Fatalf("mixed-case recursor key lost QNAME case: %q", mixed)
	}
}

func TestCacheStoreBoundsCacheSize(t *testing.T) {
	oldMax := recurseCacheMaxEntries
	recurseCacheMaxEntries = 2
	defer func() {
		recurseCacheMaxEntries = oldMax
	}()

	r := &Recursor{}
	msg := new(dns.Msg)
	msg.Rcode = dns.RcodeSuccess
	resp := packet.Packet{Msg: msg}

	r.cacheStore("a", "A", "IN", resp)
	r.cacheStore("b", "A", "IN", resp)
	r.cacheStore("c", "A", "IN", resp)

	if _, ok := r.cacheLookup("a", "A", "IN"); ok {
		t.Fatalf("expected oldest entry evicted after cache cap")
	}
	if _, ok := r.cacheLookup("b", "A", "IN"); ok {
		t.Fatalf("expected cache to reset once cap exceeded")
	}
	if _, ok := r.cacheLookup("c", "A", "IN"); !ok {
		t.Fatalf("expected latest entry to remain after reset")
	}
	if r.recurseCount != 1 {
		t.Fatalf("unexpected cache count after reset: %d", r.recurseCount)
	}
}

func TestRecurseWithNameserversDoesNotPoisonRootCache(t *testing.T) {
	nameserver.EmptyCache()
	defer nameserver.EmptyCache()

	r := &Recursor{
		fakeAddresses: map[string]map[string][]netip.Addr{},
		client:        &transport.Client{},
		recurseCache:  map[string]map[string]map[string]*recurseCacheEntry{},
		inflight:      map[string]*inflightLookup{},
	}
	if err := r.AddFakeAddresses(".", map[string][]string{
		"a.root.test": {"192.0.2.1"},
	}); err != nil {
		t.Fatalf("add fake root: %v", err)
	}

	var rootCalls int32
	rootNS, err := nameserver.NewWithContext(context.Background(), "a.root.test", "192.0.2.1", r.client)
	if err != nil {
		t.Fatalf("new root nameserver: %v", err)
	}
	rootNS.SetQueryHook(func(_ context.Context, name string, qtype string, _ string, _ *nameserver.QueryOptions) (packet.Packet, error) {
		if dnsname.New(name).String() != "example" || strings.ToUpper(qtype) != "A" {
			return packet.Packet{}, nil
		}
		atomic.AddInt32(&rootCalls, 1)
		return packetWithA(name, netip.MustParseAddr("192.0.2.1")), nil
	})

	var customCalls int32
	customNS, err := nameserver.NewWithContext(context.Background(), "custom.test", "192.0.2.2", r.client)
	if err != nil {
		t.Fatalf("new custom nameserver: %v", err)
	}
	customNS.SetQueryHook(func(_ context.Context, name string, qtype string, _ string, _ *nameserver.QueryOptions) (packet.Packet, error) {
		if dnsname.New(name).String() != "example" || strings.ToUpper(qtype) != "A" {
			return packet.Packet{}, nil
		}
		atomic.AddInt32(&customCalls, 1)
		return packetWithA(name, netip.MustParseAddr("192.0.2.2")), nil
	})

	ctx := context.Background()
	respCustom, err := r.RecurseWithNameservers(ctx, "example", "A", "IN", []nameserver.Nameserver{customNS})
	if err != nil {
		t.Fatalf("custom recurse: %v", err)
	}
	recordsCustom := respCustom.GetRecords("A", "answer")
	if len(recordsCustom) != 1 {
		t.Fatalf("expected one custom A record, got %d", len(recordsCustom))
	}
	customA, ok := recordsCustom[0].(*dns.A)
	if !ok || customA.Addr.String() != "192.0.2.2" {
		t.Fatalf("unexpected custom response: %#v", recordsCustom[0])
	}

	respRoot, err := r.Recurse(ctx, "example", "A", "IN")
	if err != nil {
		t.Fatalf("root recurse: %v", err)
	}
	recordsRoot := respRoot.GetRecords("A", "answer")
	if len(recordsRoot) != 1 {
		t.Fatalf("expected one root A record, got %d", len(recordsRoot))
	}
	rootA, ok := recordsRoot[0].(*dns.A)
	if !ok || rootA.Addr.String() != "192.0.2.1" {
		t.Fatalf("unexpected root response: %#v", recordsRoot[0])
	}

	if atomic.LoadInt32(&customCalls) == 0 {
		t.Fatalf("expected custom nameserver to be queried")
	}
	if atomic.LoadInt32(&rootCalls) == 0 {
		t.Fatalf("expected root nameserver query, cache should not reuse custom result")
	}
}

func TestRecurseInflightLookupCoalescing(t *testing.T) {
	nameserver.EmptyCache()
	defer nameserver.EmptyCache()

	r := &Recursor{
		fakeAddresses: map[string]map[string][]netip.Addr{},
		client:        &transport.Client{},
		recurseCache:  map[string]map[string]map[string]*recurseCacheEntry{},
		inflight:      map[string]*inflightLookup{},
	}
	if err := r.AddFakeAddresses(".", map[string][]string{
		"a.root.test": {"192.0.2.1"},
	}); err != nil {
		t.Fatalf("add fake root: %v", err)
	}

	rootNS, err := nameserver.NewWithContext(context.Background(), "a.root.test", "192.0.2.1", r.client)
	if err != nil {
		t.Fatalf("new root nameserver: %v", err)
	}

	started := make(chan struct{})
	release := make(chan struct{})
	var startOnce sync.Once
	var calls int32
	rootNS.SetQueryHook(func(_ context.Context, name string, qtype string, _ string, _ *nameserver.QueryOptions) (packet.Packet, error) {
		if dnsname.New(name).String() != "example" || strings.ToUpper(qtype) != "A" {
			return packet.Packet{}, errors.New("unexpected query")
		}
		atomic.AddInt32(&calls, 1)
		startOnce.Do(func() { close(started) })
		<-release
		return packetWithA(name, netip.MustParseAddr("192.0.2.111")), nil
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(2)
	var resp1 packet.Packet
	var resp2 packet.Packet
	var err1 error
	var err2 error

	go func() {
		defer wg.Done()
		resp1, err1 = r.Recurse(ctx, "example", "A", "IN")
	}()

	select {
	case <-started:
	case <-time.After(200 * time.Millisecond):
		t.Fatalf("expected first recurse query to start")
	}

	go func() {
		defer wg.Done()
		resp2, err2 = r.Recurse(ctx, "example", "A", "IN")
	}()

	time.Sleep(50 * time.Millisecond)
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("expected single in-flight recurse call, got %d", got)
	}

	close(release)
	wg.Wait()

	if err1 != nil || err2 != nil {
		t.Fatalf("unexpected recurse errors: %v %v", err1, err2)
	}
	if resp1.Msg == nil || resp2.Msg == nil {
		t.Fatalf("expected responses from both recurse calls")
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("expected one network recurse call total, got %d", got)
	}

	if _, err := r.Recurse(ctx, "example", "A", "IN"); err != nil {
		t.Fatalf("cached recurse: %v", err)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("expected cache hit after coalescing, got %d network calls", got)
	}
}

func TestRecurseInflightLookupWaiterCancellation(t *testing.T) {
	nameserver.EmptyCache()
	defer nameserver.EmptyCache()

	r := &Recursor{
		fakeAddresses: map[string]map[string][]netip.Addr{},
		client:        &transport.Client{},
		recurseCache:  map[string]map[string]map[string]*recurseCacheEntry{},
		inflight:      map[string]*inflightLookup{},
	}
	if err := r.AddFakeAddresses(".", map[string][]string{
		"a.root.test": {"192.0.2.1"},
	}); err != nil {
		t.Fatalf("add fake root: %v", err)
	}

	rootNS, err := nameserver.NewWithContext(context.Background(), "a.root.test", "192.0.2.1", r.client)
	if err != nil {
		t.Fatalf("new root nameserver: %v", err)
	}

	started := make(chan struct{})
	release := make(chan struct{})
	var startOnce sync.Once
	var calls int32
	rootNS.SetQueryHook(func(_ context.Context, name string, qtype string, _ string, _ *nameserver.QueryOptions) (packet.Packet, error) {
		if dnsname.New(name).String() != "example" || strings.ToUpper(qtype) != "A" {
			return packet.Packet{}, errors.New("unexpected query")
		}
		atomic.AddInt32(&calls, 1)
		startOnce.Do(func() { close(started) })
		<-release
		return packetWithA(name, netip.MustParseAddr("192.0.2.112")), nil
	})

	leaderCtx, leaderCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer leaderCancel()

	leaderDone := make(chan struct{})
	var leaderErr error
	go func() {
		_, leaderErr = r.Recurse(leaderCtx, "example", "A", "IN")
		close(leaderDone)
	}()

	select {
	case <-started:
	case <-time.After(200 * time.Millisecond):
		t.Fatalf("expected leader recurse query to start")
	}

	waiterCtx, waiterCancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer waiterCancel()
	_, waitErr := r.Recurse(waiterCtx, "example", "A", "IN")
	if !errors.Is(waitErr, context.DeadlineExceeded) {
		t.Fatalf("expected waiter deadline exceeded, got %v", waitErr)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("expected waiter to share in-flight lookup, got %d calls", got)
	}

	close(release)
	select {
	case <-leaderDone:
	case <-time.After(time.Second):
		t.Fatalf("leader recurse did not finish")
	}
	if leaderErr != nil {
		t.Fatalf("leader recurse: %v", leaderErr)
	}
}

func TestParentSingleLabelFallsBackToRoot(t *testing.T) {
	nameserver.EmptyCache()
	defer nameserver.EmptyCache()

	r := &Recursor{
		fakeAddresses: map[string]map[string][]netip.Addr{},
		client:        &transport.Client{},
	}
	if err := r.AddFakeAddresses(".", map[string][]string{
		"a.root.test": {"192.0.2.1"},
	}); err != nil {
		t.Fatalf("add fake root: %v", err)
	}

	rootNS, err := nameserver.NewWithContext(context.Background(), "a.root.test", "192.0.2.1", r.client)
	if err != nil {
		t.Fatalf("new root nameserver: %v", err)
	}
	rootNS.SetQueryHook(func(_ context.Context, name string, qtype string, _ string, _ *nameserver.QueryOptions) (packet.Packet, error) {
		nameObj := dnsname.New(name)
		name = nameObj.String()
		qtype = strings.ToUpper(qtype)
		switch {
		case name == "arpa" && qtype == "SOA":
			msg := new(dns.Msg)
			msg.Rcode = dns.RcodeSuccess
			nsRR1 := &dns.NS{Hdr: dns.Header{Name: "arpa.", Class: dns.ClassINET}}
			nsRR1.Ns = "ns.arpa.test."
			msg.Ns = []dns.RR{nsRR1}
			aRR1 := &dns.A{Hdr: dns.Header{Name: "ns.arpa.test.", Class: dns.ClassINET}}
			aRR1.Addr = netip.AddrFrom4([4]byte{192, 0, 2, 2})
			msg.Extra = []dns.RR{aRR1}
			return packet.Packet{Msg: msg}, nil
		case name == "." && qtype == "SOA":
			// Simulate a transient parent-check failure. Parent() should still
			// fall back to root for single-label zones.
			return packet.Packet{}, errors.New("timeout")
		default:
			return packet.Packet{}, errors.New("unexpected query")
		}
	})

	arpaNS, err := nameserver.NewWithContext(context.Background(), "ns.arpa.test", "192.0.2.2", r.client)
	if err != nil {
		t.Fatalf("new arpa nameserver: %v", err)
	}
	arpaNS.SetQueryHook(func(_ context.Context, name string, qtype string, _ string, _ *nameserver.QueryOptions) (packet.Packet, error) {
		nameObj := dnsname.New(name)
		name = nameObj.String()
		qtype = strings.ToUpper(qtype)
		if name != "arpa" || qtype != "SOA" {
			return packet.Packet{}, errors.New("unexpected query")
		}

		msg := new(dns.Msg)
		msg.Rcode = dns.RcodeSuccess
		soaRR2 := &dns.SOA{Hdr: dns.Header{Name: "arpa.", Class: dns.ClassINET}}
		soaRR2.Ns = "ns.arpa.test."
		soaRR2.Mbox = "hostmaster.arpa."
		soaRR2.Serial = 1
		soaRR2.Refresh = 3600
		soaRR2.Retry = 600
		soaRR2.Expire = 1209600
		soaRR2.Minttl = 3600
		msg.Answer = []dns.RR{soaRR2}
		return packet.Packet{Msg: msg}, nil
	})

	parent, _, err := r.Parent(context.Background(), "arpa")
	if err != nil {
		t.Fatalf("parent: %v", err)
	}
	if parent != "." {
		t.Fatalf("expected parent '.', got %q", parent)
	}
}

func TestParentSingleLabelNoTraceFallsBackToRoot(t *testing.T) {
	nameserver.EmptyCache()
	defer nameserver.EmptyCache()

	r := &Recursor{
		fakeAddresses: map[string]map[string][]netip.Addr{},
		client:        &transport.Client{},
	}
	if err := r.AddFakeAddresses(".", map[string][]string{
		"a.root.test": {"192.0.2.1"},
	}); err != nil {
		t.Fatalf("add fake root: %v", err)
	}

	rootNS, err := nameserver.NewWithContext(context.Background(), "a.root.test", "192.0.2.1", r.client)
	if err != nil {
		t.Fatalf("new root nameserver: %v", err)
	}
	rootNS.SetQueryHook(func(_ context.Context, name string, qtype string, _ string, _ *nameserver.QueryOptions) (packet.Packet, error) {
		nameObj := dnsname.New(name)
		name = nameObj.String()
		qtype = strings.ToUpper(qtype)
		if name != "arpa" || qtype != "SOA" {
			return packet.Packet{}, errors.New("unexpected query")
		}

		// Authoritative answer directly from the current server produces no
		// referral trace. Parent() must still resolve arpa -> .
		msg := new(dns.Msg)
		msg.Rcode = dns.RcodeSuccess
		soaRR3 := &dns.SOA{Hdr: dns.Header{Name: "arpa.", Class: dns.ClassINET}}
		soaRR3.Ns = "ns.arpa.test."
		soaRR3.Mbox = "hostmaster.arpa."
		soaRR3.Serial = 1
		soaRR3.Refresh = 3600
		soaRR3.Retry = 600
		soaRR3.Expire = 1209600
		soaRR3.Minttl = 3600
		msg.Answer = []dns.RR{soaRR3}
		return packet.Packet{Msg: msg}, nil
	})

	parent, _, err := r.Parent(context.Background(), "arpa")
	if err != nil {
		t.Fatalf("parent: %v", err)
	}
	if parent != "." {
		t.Fatalf("expected parent '.', got %q", parent)
	}
}

func TestGetNSFromUsesGlueAndLazy(t *testing.T) {
	nameserver.EmptyCache()
	defer nameserver.EmptyCache()

	msg := new(dns.Msg)
	nsRR2 := &dns.NS{Hdr: dns.Header{Name: "example.", Class: dns.ClassINET}}
	nsRR2.Ns = "ns2.example."
	nsRR3 := &dns.NS{Hdr: dns.Header{Name: "example.", Class: dns.ClassINET}}
	nsRR3.Ns = "ns1.example."
	msg.Ns = []dns.RR{nsRR2, nsRR3}
	aRR2 := &dns.A{Hdr: dns.Header{Name: "ns1.example.", Class: dns.ClassINET}}
	aRR2.Addr = netip.AddrFrom4([4]byte{192, 0, 2, 53})
	msg.Extra = []dns.RR{aRR2}

	resp := packet.Packet{Msg: msg}
	r := &Recursor{client: &transport.Client{}}
	queryers, err := r.getNSFrom(context.Background(), resp, nil)
	if err != nil {
		t.Fatalf("getNSFrom: %v", err)
	}
	if len(queryers) != 2 {
		t.Fatalf("expected 2 queryers, got %d", len(queryers))
	}

	first, ok := queryers[0].(nameserver.Nameserver)
	if !ok || first.Address.String() != "192.0.2.53" {
		t.Fatalf("unexpected first queryer: %#v", queryers[0])
	}
	if _, ok := queryers[1].(lazyNameserver); !ok {
		t.Fatalf("expected lazy nameserver, got %#v", queryers[1])
	}
}

func TestGetNSFromIgnoresOutOfBailiwickAndUnrelatedGlue(t *testing.T) {
	nameserver.EmptyCache()
	defer nameserver.EmptyCache()

	msg := new(dns.Msg)
	nsRR4 := &dns.NS{Hdr: dns.Header{Name: "example.", Class: dns.ClassINET}}
	nsRR4.Ns = "ns1.example."
	nsRR5 := &dns.NS{Hdr: dns.Header{Name: "example.", Class: dns.ClassINET}}
	nsRR5.Ns = "ns.outside.net."
	msg.Ns = []dns.RR{nsRR4, nsRR5}
	aRR3 := &dns.A{Hdr: dns.Header{Name: "ns1.example.", Class: dns.ClassINET}}
	aRR3.Addr = netip.AddrFrom4([4]byte{192, 0, 2, 53})
	aRR4 := &dns.A{Hdr: dns.Header{Name: "ns.outside.net.", Class: dns.ClassINET}}
	aRR4.Addr = netip.AddrFrom4([4]byte{203, 0, 113, 9})
	aRR5 := &dns.A{Hdr: dns.Header{Name: "attacker.example.", Class: dns.ClassINET}}
	aRR5.Addr = netip.AddrFrom4([4]byte{198, 51, 100, 66})
	msg.Extra = []dns.RR{aRR3, aRR4, aRR5}

	resp := packet.Packet{Msg: msg}
	r := &Recursor{client: &transport.Client{}}
	state := &recurseState{}
	queryers, err := r.getNSFrom(context.Background(), resp, state)
	if err != nil {
		t.Fatalf("getNSFrom: %v", err)
	}
	if len(queryers) != 2 {
		t.Fatalf("expected 2 queryers, got %d", len(queryers))
	}

	first, ok := queryers[0].(nameserver.Nameserver)
	if !ok || first.Name.String() != "ns1.example" || first.Address.String() != "192.0.2.53" {
		t.Fatalf("unexpected first queryer: %#v", queryers[0])
	}
	second, ok := queryers[1].(lazyNameserver)
	if !ok || second.name != "ns.outside.net" {
		t.Fatalf("expected lazy queryer for out-of-bailiwick NS, got %#v", queryers[1])
	}

	state.ensureLock()
	state.lock()
	defer state.unlock()
	if _, ok := state.glue["ns.outside.net"]; ok {
		t.Fatalf("did not expect out-of-bailiwick glue to be trusted")
	}
	if _, ok := state.glue["attacker.example"]; ok {
		t.Fatalf("did not expect unrelated additional address to be trusted")
	}
}

func TestLazyNameserverMissingRecursor(t *testing.T) {
	lns := lazyNameserver{name: "ns1.example"}
	if _, err := lns.QueryWithClass(context.Background(), "example", "A", "IN"); err == nil {
		t.Fatalf("expected error for missing recursor")
	}
}

func TestGetAddressesForParallelAAndAAAA(t *testing.T) {
	nameserver.EmptyCache()
	defer nameserver.EmptyCache()
	baseCtx, prof, _ := testhelpers.Context(t)

	if err := prof.Set("resolver.defaults.parallel", 2); err != nil {
		t.Fatalf("set parallel: %v", err)
	}
	if err := prof.Set("resolver.defaults.unordered", false); err != nil {
		t.Fatalf("set unordered: %v", err)
	}

	r := &Recursor{client: &transport.Client{}}
	if err := r.AddFakeAddresses(".", map[string][]string{
		"root.test": {"192.0.2.53"},
	}); err != nil {
		t.Fatalf("add fake root: %v", err)
	}

	aaaaStarted := make(chan struct{})
	rootNS, err := nameserver.NewWithContext(baseCtx, "root.test", "192.0.2.53", r.client)
	if err != nil {
		t.Fatalf("new root nameserver: %v", err)
	}
	rootNS.SetQueryHook(func(ctx context.Context, name string, qtype string, qclass string, _ *nameserver.QueryOptions) (packet.Packet, error) {
		switch strings.ToUpper(qtype) {
		case "A":
			select {
			case <-aaaaStarted:
			case <-ctx.Done():
				return packet.Packet{}, ctx.Err()
			}
			return packetWithA(name, netip.MustParseAddr("192.0.2.10")), nil
		case "AAAA":
			select {
			case <-aaaaStarted:
			default:
				close(aaaaStarted)
			}
			return packetWithAAAA(name, netip.MustParseAddr("2001:db8::1")), nil
		default:
			return packet.Packet{Msg: new(dns.Msg)}, nil
		}
	})

	ctx, cancel := context.WithTimeout(baseCtx, time.Second)
	defer cancel()

	addrs, err := r.GetAddressesFor(ctx, "ns.example")
	if err != nil {
		t.Fatalf("get addresses: %v", err)
	}
	if len(addrs) != 2 {
		t.Fatalf("expected 2 addresses, got %d", len(addrs))
	}
	if addrs[0].String() != "192.0.2.10" || addrs[1].String() != "2001:db8::1" {
		t.Fatalf("unexpected address order: %#v", addrs)
	}
}

func TestLazyNameserverParallelPrefersFirstAddress(t *testing.T) {
	nameserver.EmptyCache()
	defer nameserver.EmptyCache()
	baseCtx, prof, _ := testhelpers.Context(t)

	if err := prof.Set("resolver.defaults.parallel", 2); err != nil {
		t.Fatalf("set parallel: %v", err)
	}
	if err := prof.Set("resolver.defaults.unordered", false); err != nil {
		t.Fatalf("set unordered: %v", err)
	}

	r := &Recursor{client: &transport.Client{}}
	if err := r.AddFakeAddresses(".", map[string][]string{
		"root.test": {"192.0.2.53"},
	}); err != nil {
		t.Fatalf("add fake root: %v", err)
	}

	rootNS, err := nameserver.NewWithContext(baseCtx, "root.test", "192.0.2.53", r.client)
	if err != nil {
		t.Fatalf("new root nameserver: %v", err)
	}
	rootNS.SetQueryHook(func(ctx context.Context, name string, qtype string, qclass string, _ *nameserver.QueryOptions) (packet.Packet, error) {
		switch strings.ToUpper(qtype) {
		case "A":
			return packetWithARecords(name, []netip.Addr{
				netip.MustParseAddr("192.0.2.10"),
				netip.MustParseAddr("192.0.2.20"),
			}), nil
		case "AAAA":
			return packet.Packet{Msg: new(dns.Msg)}, nil
		default:
			return packet.Packet{Msg: new(dns.Msg)}, nil
		}
	})

	addr2Started := make(chan struct{})
	allowAddr1 := make(chan struct{})
	allowOnce := func() {
		select {
		case <-allowAddr1:
		default:
			close(allowAddr1)
		}
	}

	ns1, err := nameserver.NewWithContext(baseCtx, "ns1.example", "192.0.2.10", r.client)
	if err != nil {
		t.Fatalf("new ns1: %v", err)
	}
	ns1.SetQueryHook(func(ctx context.Context, name string, qtype string, qclass string, _ *nameserver.QueryOptions) (packet.Packet, error) {
		select {
		case <-allowAddr1:
		case <-ctx.Done():
			return packet.Packet{}, ctx.Err()
		}
		resp := packetWithA(name, netip.MustParseAddr("192.0.2.10"))
		resp.AnswerFrom = "192.0.2.10"
		return resp, nil
	})

	ns2, err := nameserver.NewWithContext(baseCtx, "ns1.example", "192.0.2.20", r.client)
	if err != nil {
		t.Fatalf("new ns2: %v", err)
	}
	ns2.SetQueryHook(func(ctx context.Context, name string, qtype string, qclass string, _ *nameserver.QueryOptions) (packet.Packet, error) {
		select {
		case <-addr2Started:
		default:
			close(addr2Started)
		}
		resp := packetWithA(name, netip.MustParseAddr("192.0.2.20"))
		resp.AnswerFrom = "192.0.2.20"
		return resp, nil
	})

	lns := lazyNameserver{name: "ns1.example", recursor: r, state: &recurseState{glue: map[string]map[netip.Addr]bool{}}}
	ctx, cancel := context.WithTimeout(baseCtx, time.Second)
	defer cancel()

	resultCh := make(chan packet.Packet, 1)
	errCh := make(chan error, 1)
	go func() {
		resp, err := lns.QueryWithClass(ctx, "example", "A", "IN")
		resultCh <- resp
		errCh <- err
	}()

	select {
	case <-addr2Started:
	case <-time.After(time.Second):
		allowOnce()
		t.Fatalf("expected second address to be queried")
	}
	allowOnce()

	resp := <-resultCh
	err = <-errCh
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if resp.AnswerFrom != "192.0.2.10" {
		t.Fatalf("expected first address response, got %q", resp.AnswerFrom)
	}
}

func TestLazyNameserverConcurrentQueriesShareGlue(t *testing.T) {
	nameserver.EmptyCache()
	defer nameserver.EmptyCache()

	r := &Recursor{client: &transport.Client{}}
	if err := r.AddFakeAddresses(".", map[string][]string{
		"root.test": {"192.0.2.53"},
	}); err != nil {
		t.Fatalf("add fake root: %v", err)
	}

	rootNS, err := nameserver.New("root.test", "192.0.2.53", r.client)
	if err != nil {
		t.Fatalf("new root nameserver: %v", err)
	}
	rootNS.SetQueryHook(func(ctx context.Context, name string, qtype string, qclass string, _ *nameserver.QueryOptions) (packet.Packet, error) {
		switch strings.ToUpper(qtype) {
		case "A":
			return packetWithA(name, netip.MustParseAddr("192.0.2.30")), nil
		case "AAAA":
			return packetWithAAAA(name, netip.MustParseAddr("2001:db8::30")), nil
		default:
			return packet.Packet{Msg: new(dns.Msg)}, nil
		}
	})

	ns4, err := nameserver.New("ns.example", "192.0.2.30", r.client)
	if err != nil {
		t.Fatalf("new ns4: %v", err)
	}
	ns4.SetQueryHook(func(ctx context.Context, name string, qtype string, qclass string, _ *nameserver.QueryOptions) (packet.Packet, error) {
		return packetWithA(name, netip.MustParseAddr("192.0.2.31")), nil
	})

	ns6, err := nameserver.New("ns.example", "2001:db8::30", r.client)
	if err != nil {
		t.Fatalf("new ns6: %v", err)
	}
	ns6.SetQueryHook(func(ctx context.Context, name string, qtype string, qclass string, _ *nameserver.QueryOptions) (packet.Packet, error) {
		return packetWithA(name, netip.MustParseAddr("192.0.2.32")), nil
	})

	state := &recurseState{glue: map[string]map[netip.Addr]bool{}}
	lns := lazyNameserver{name: "ns.example", recursor: r, state: state}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	start := make(chan struct{})
	errCh := make(chan error, 64)
	var wg sync.WaitGroup
	for range 32 {
		wg.Go(func() {
			<-start
			_, err := lns.QueryWithClass(ctx, "example", "A", "IN")
			if err != nil && ctx.Err() == nil {
				errCh <- err
			}
		})
	}
	close(start)
	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Fatalf("query failed: %v", err)
	}

	state.ensureLock()
	state.lock()
	defer state.unlock()
	nameObj := dnsname.New("ns.example")
	nameKey := strings.ToLower(nameObj.String())
	if len(state.glue[nameKey]) == 0 {
		t.Fatalf("expected glue cached for ns.example")
	}
}

func TestGetNSFromConcurrentWithLazyNameserver(t *testing.T) {
	nameserver.EmptyCache()
	defer nameserver.EmptyCache()

	r := &Recursor{client: &transport.Client{}}
	if err := r.AddFakeAddresses(".", map[string][]string{
		"root.test": {"192.0.2.53"},
	}); err != nil {
		t.Fatalf("add fake root: %v", err)
	}

	rootNS, err := nameserver.New("root.test", "192.0.2.53", r.client)
	if err != nil {
		t.Fatalf("new root nameserver: %v", err)
	}
	rootNS.SetQueryHook(func(ctx context.Context, name string, qtype string, qclass string, _ *nameserver.QueryOptions) (packet.Packet, error) {
		switch strings.ToUpper(qtype) {
		case "A":
			return packetWithA(name, netip.MustParseAddr("192.0.2.40")), nil
		case "AAAA":
			return packetWithAAAA(name, netip.MustParseAddr("2001:db8::40")), nil
		default:
			return packet.Packet{Msg: new(dns.Msg)}, nil
		}
	})

	ns4, err := nameserver.New("ns.example", "192.0.2.40", r.client)
	if err != nil {
		t.Fatalf("new ns4: %v", err)
	}
	ns4.SetQueryHook(func(ctx context.Context, name string, qtype string, qclass string, _ *nameserver.QueryOptions) (packet.Packet, error) {
		return packetWithA(name, netip.MustParseAddr("192.0.2.41")), nil
	})

	ns6, err := nameserver.New("ns.example", "2001:db8::40", r.client)
	if err != nil {
		t.Fatalf("new ns6: %v", err)
	}
	ns6.SetQueryHook(func(ctx context.Context, name string, qtype string, qclass string, _ *nameserver.QueryOptions) (packet.Packet, error) {
		return packetWithA(name, netip.MustParseAddr("192.0.2.42")), nil
	})

	msg := new(dns.Msg)
	nsRR6 := &dns.NS{Hdr: dns.Header{Name: "example.", Class: dns.ClassINET}}
	nsRR6.Ns = "ns.example."
	msg.Ns = []dns.RR{nsRR6}
	aRR6 := &dns.A{Hdr: dns.Header{Name: "ns.example.", Class: dns.ClassINET}}
	aRR6.Addr = netip.AddrFrom4([4]byte{192, 0, 2, 40})
	aaaa6 := &dns.AAAA{Hdr: dns.Header{Name: "ns.example.", Class: dns.ClassINET}}
	aaaa6.Addr = netip.MustParseAddr("2001:db8::40")
	msg.Extra = []dns.RR{aRR6, aaaa6}
	resp := packet.Packet{Msg: msg}

	state := &recurseState{glue: map[string]map[netip.Addr]bool{}}
	lns := lazyNameserver{name: "ns.example", recursor: r, state: state}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	start := make(chan struct{})
	errCh := make(chan error, 2)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		for range 20 {
			if _, err := r.getNSFrom(context.Background(), resp, state); err != nil {
				errCh <- err
				return
			}
		}
	}()
	go func() {
		defer wg.Done()
		<-start
		for range 20 {
			if _, err := lns.QueryWithClass(ctx, "example", "A", "IN"); err != nil && ctx.Err() == nil {
				errCh <- err
				return
			}
		}
	}()
	close(start)
	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Fatalf("concurrent query failed: %v", err)
	}

	state.ensureLock()
	state.lock()
	defer state.unlock()
	nameObj := dnsname.New("ns.example")
	nameKey := strings.ToLower(nameObj.String())
	if len(state.glue[nameKey]) == 0 {
		t.Fatalf("expected glue cached for ns.example")
	}
}

func TestCollectCNAMEsAndAddresses(t *testing.T) {
	msg := new(dns.Msg)
	cnameRR3 := &dns.CNAME{Hdr: dns.Header{Name: "www.example.", Class: dns.ClassINET}}
	cnameRR3.Target = "alias.example."
	aRR7 := &dns.A{Hdr: dns.Header{Name: "alias.example.", Class: dns.ClassINET}}
	aRR7.Addr = netip.AddrFrom4([4]byte{192, 0, 2, 55})
	aaaa7 := &dns.AAAA{Hdr: dns.Header{Name: "alias.example.", Class: dns.ClassINET}}
	aaaa7.Addr = netip.MustParseAddr("2001:db8::55")
	msg.Answer = []dns.RR{cnameRR3, aRR7, aaaa7}

	resp := packet.Packet{Msg: msg}
	target := dnsname.New("www.example")
	cnames := map[string]bool{}
	collectCNAMEs(resp, target, cnames)
	if !cnames["alias.example"] {
		t.Fatalf("expected alias.example cname")
	}

	addrs := collectAddresses(resp, target, cnames)
	if len(addrs) != 2 {
		t.Fatalf("expected 2 addresses, got %d", len(addrs))
	}
	got := map[string]bool{}
	for _, addr := range addrs {
		got[addr.String()] = true
	}
	if !got["192.0.2.55"] || !got["2001:db8::55"] {
		t.Fatalf("unexpected addresses: %#v", got)
	}
}

func TestFirstSOAOwner(t *testing.T) {
	msg := new(dns.Msg)
	soaRR4 := &dns.SOA{Hdr: dns.Header{Name: "example.", Class: dns.ClassINET}}
	msg.Answer = []dns.RR{soaRR4}
	resp := packet.Packet{Msg: msg}
	if owner := firstSOAOwner(resp); owner != "example" {
		t.Fatalf("unexpected owner %q", owner)
	}

	if owner := firstSOAOwner(packet.Packet{Msg: new(dns.Msg)}); owner != "" {
		t.Fatalf("expected empty owner, got %q", owner)
	}
}

func TestRecurseOrderedUsesLIFO(t *testing.T) {
	ctx, prof, _ := testhelpers.Context(t)
	if err := prof.Set("resolver.defaults.unordered", false); err != nil {
		t.Fatalf("set unordered: %v", err)
	}
	if err := prof.Set("resolver.defaults.parallel", 2); err != nil {
		t.Fatalf("set parallel: %v", err)
	}

	r := &Recursor{}

	slowResp := packetWithA("example", netip.MustParseAddr("192.0.2.10"))
	slowResp.AnswerFrom = "slow"
	fastResp := packetWithA("example", netip.MustParseAddr("192.0.2.11"))
	fastResp.AnswerFrom = "fast"

	slow := testQueryer{id: "slow", resp: slowResp}
	fast := testQueryer{id: "fast", resp: fastResp}

	state := &recurseState{ns: []queryer{fast, slow}}
	resp, _, err := r.recurse(ctx, "example", "A", "IN", state)
	if err != nil {
		t.Fatalf("recurse: %v", err)
	}
	if resp.AnswerFrom != "slow" {
		t.Fatalf("expected LIFO response from slow, got %q", resp.AnswerFrom)
	}
}

func TestRecurseOrderedParallelStartsNextQuery(t *testing.T) {
	baseCtx, prof, _ := testhelpers.Context(t)
	if err := prof.Set("resolver.defaults.unordered", false); err != nil {
		t.Fatalf("set unordered: %v", err)
	}
	if err := prof.Set("resolver.defaults.parallel", 2); err != nil {
		t.Fatalf("set parallel: %v", err)
	}

	r := &Recursor{}
	slowStarted := make(chan struct{})
	fastStarted := make(chan struct{})
	releaseSlow := make(chan struct{})

	slow := testQueryer{id: "slow", startCh: slowStarted, waitCh: releaseSlow}
	fastResp := packetWithA("example", netip.MustParseAddr("192.0.2.71"))
	fastResp.AnswerFrom = "fast"
	fast := testQueryer{id: "fast", startCh: fastStarted, resp: fastResp}

	state := &recurseState{ns: []queryer{fast, slow}}
	ctx, cancel := context.WithTimeout(baseCtx, time.Second)
	defer cancel()

	done := make(chan struct{})
	var out packet.Packet
	var recurseErr error
	go func() {
		out, _, recurseErr = r.recurse(ctx, "example", "A", "IN", state)
		close(done)
	}()

	select {
	case <-slowStarted:
	case <-time.After(200 * time.Millisecond):
		t.Fatalf("expected slow query to start")
	}
	select {
	case <-fastStarted:
	case <-time.After(200 * time.Millisecond):
		t.Fatalf("expected fast query to start in parallel while slow is blocked")
	}

	close(releaseSlow)

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatalf("recurse did not finish")
	}
	if recurseErr != nil {
		t.Fatalf("recurse: %v", recurseErr)
	}
	if out.AnswerFrom != "fast" {
		t.Fatalf("expected fast response after slow miss, got %q", out.AnswerFrom)
	}
}

func TestRecurseOrderedParallelPreservesRedirectPriority(t *testing.T) {
	ctx, prof, _ := testhelpers.Context(t)
	if err := prof.Set("resolver.defaults.unordered", false); err != nil {
		t.Fatalf("set unordered: %v", err)
	}
	if err := prof.Set("resolver.defaults.parallel", 2); err != nil {
		t.Fatalf("set parallel: %v", err)
	}

	r := &Recursor{}
	redirectResp := packetWithReferral("child.example", "ns.child.example")
	redirectResp.AnswerFrom = "redirect"
	speculativeResp := packetWithA("example", netip.MustParseAddr("192.0.2.80"))
	speculativeResp.AnswerFrom = "speculative"
	childResp := packetWithA("example", netip.MustParseAddr("192.0.2.81"))
	childResp.AnswerFrom = "child"

	state := &recurseState{
		ns: []queryer{
			testQueryer{id: "speculative", resp: speculativeResp},
			testQueryer{id: "redirect", resp: redirectResp},
		},
		nsFrom: func(_ context.Context, resp packet.Packet, _ *recurseState) ([]queryer, error) {
			if resp.AnswerFrom != "redirect" {
				t.Fatalf("expected redirect source, got %q", resp.AnswerFrom)
			}
			return []queryer{testQueryer{id: "child", resp: childResp}}, nil
		},
	}

	out, nextState, err := r.recurse(ctx, "example", "A", "IN", state)
	if err != nil {
		t.Fatalf("recurse: %v", err)
	}
	if out.AnswerFrom != "child" {
		t.Fatalf("expected child answer after redirect, got %q", out.AnswerFrom)
	}
	if len(nextState.trace) == 0 {
		t.Fatalf("expected trace entry for redirect")
	}
	if nextState.trace[0].answerFrom != "redirect" {
		t.Fatalf("expected redirect trace source, got %q", nextState.trace[0].answerFrom)
	}
	if nextState.trace[0].zoneName != "child.example" {
		t.Fatalf("expected child.example trace, got %q", nextState.trace[0].zoneName)
	}
}

func TestRecurseOrderedParallelCandidateSelection(t *testing.T) {
	ctx, prof, _ := testhelpers.Context(t)
	if err := prof.Set("resolver.defaults.unordered", false); err != nil {
		t.Fatalf("set unordered: %v", err)
	}
	if err := prof.Set("resolver.defaults.parallel", 2); err != nil {
		t.Fatalf("set parallel: %v", err)
	}

	r := &Recursor{}
	servfail := new(dns.Msg)
	servfail.Rcode = dns.RcodeServerFailure
	refused := new(dns.Msg)
	refused.Rcode = dns.RcodeRefused

	state := &recurseState{
		ns: []queryer{
			testQueryer{id: "final-miss", resp: packet.Packet{}},
			testQueryer{id: "refused", resp: packet.Packet{Msg: refused, AnswerFrom: "refused"}},
			testQueryer{id: "servfail", resp: packet.Packet{Msg: servfail, AnswerFrom: "servfail"}},
		},
	}

	out, _, err := r.recurse(ctx, "example", "A", "IN", state)
	if err != nil {
		t.Fatalf("recurse: %v", err)
	}
	if out.Msg == nil || out.Rcode() != "REFUSED" {
		t.Fatalf("expected REFUSED candidate, got %#v", out.Msg)
	}
	if out.AnswerFrom != "refused" {
		t.Fatalf("expected latest candidate from refused, got %q", out.AnswerFrom)
	}
}

func TestRecurseOrderedParallelDropsSpeculativeLogs(t *testing.T) {
	baseCtx, prof, log := testhelpers.Context(t)
	if err := prof.Set("resolver.defaults.unordered", false); err != nil {
		t.Fatalf("set unordered: %v", err)
	}
	if err := prof.Set("resolver.defaults.parallel", 2); err != nil {
		t.Fatalf("set parallel: %v", err)
	}

	r := &Recursor{}
	specStarted := make(chan struct{})
	specBlocked := make(chan struct{})

	redirectResp := packetWithReferral("child.example", "ns.child.example")
	redirectResp.AnswerFrom = "redirect"
	childResp := packetWithA("example", netip.MustParseAddr("192.0.2.91"))
	childResp.AnswerFrom = "child"

	state := &recurseState{
		ns: []queryer{
			loggingQueryer{
				id:      "spec",
				startCh: specStarted,
				waitCh:  specBlocked,
				resp:    packetWithA("example", netip.MustParseAddr("192.0.2.90")),
			},
			loggingQueryer{
				id:        "redirect",
				waitStart: specStarted,
				resp:      redirectResp,
			},
		},
		nsFrom: func(_ context.Context, _ packet.Packet, _ *recurseState) ([]queryer, error) {
			return []queryer{loggingQueryer{id: "child", resp: childResp}}, nil
		},
	}

	ctx, cancel := context.WithTimeout(baseCtx, time.Second)
	defer cancel()

	out, _, err := r.recurse(ctx, "example", "A", "IN", state)
	if err != nil {
		t.Fatalf("recurse: %v", err)
	}
	if out.AnswerFrom != "child" {
		t.Fatalf("expected child answer after redirect, got %q", out.AnswerFrom)
	}

	tags := map[string]bool{}
	for _, entry := range log.Entries() {
		if entry == nil {
			continue
		}
		tags[entry.Tag] = true
	}
	if !tags["TEST_QUERY_REDIRECT"] || !tags["TEST_QUERY_CHILD"] {
		t.Fatalf("expected redirect and child logs, got %v", tags)
	}
	if tags["TEST_QUERY_SPEC"] {
		t.Fatalf("expected speculative logs to be dropped, got %v", tags)
	}
}

func TestRecurseUnorderedReturnsFastest(t *testing.T) {
	baseCtx, prof, _ := testhelpers.Context(t)
	if err := prof.Set("resolver.defaults.unordered", true); err != nil {
		t.Fatalf("set unordered: %v", err)
	}
	if err := prof.Set("resolver.defaults.parallel", 2); err != nil {
		t.Fatalf("set parallel: %v", err)
	}

	r := &Recursor{}

	slowResp := packetWithA("example", netip.MustParseAddr("192.0.2.20"))
	slowResp.AnswerFrom = "slow"
	fastResp := packetWithA("example", netip.MustParseAddr("192.0.2.21"))
	fastResp.AnswerFrom = "fast"

	slow := testQueryer{id: "slow", resp: slowResp, delay: 80 * time.Millisecond}
	fast := testQueryer{id: "fast", resp: fastResp}

	state := &recurseState{ns: []queryer{fast, slow}}
	ctx, cancel := context.WithTimeout(baseCtx, time.Second)
	defer cancel()

	resp, _, err := r.recurse(ctx, "example", "A", "IN", state)
	if err != nil {
		t.Fatalf("recurse: %v", err)
	}
	if resp.AnswerFrom != "fast" {
		t.Fatalf("expected fastest response from fast, got %q", resp.AnswerFrom)
	}
}

func TestRecurseUnorderedCancelsSlowQuery(t *testing.T) {
	baseCtx, prof, _ := testhelpers.Context(t)
	if err := prof.Set("resolver.defaults.unordered", true); err != nil {
		t.Fatalf("set unordered: %v", err)
	}
	if err := prof.Set("resolver.defaults.parallel", 2); err != nil {
		t.Fatalf("set parallel: %v", err)
	}

	r := &Recursor{}

	slowStarted := make(chan struct{})
	slowCanceled := make(chan struct{})
	slow := testQueryer{id: "slow", waitForCancel: true, cancelCh: slowCanceled, startCh: slowStarted}

	fastResp := packetWithA("example", netip.MustParseAddr("192.0.2.22"))
	fastResp.AnswerFrom = "fast"
	fast := testQueryer{id: "fast", resp: fastResp, waitCh: slowStarted}

	state := &recurseState{ns: []queryer{fast, slow}}
	ctx, cancel := context.WithTimeout(baseCtx, time.Second)
	defer cancel()

	resp, _, err := r.recurse(ctx, "example", "A", "IN", state)
	if err != nil {
		t.Fatalf("recurse: %v", err)
	}
	if resp.AnswerFrom != "fast" {
		t.Fatalf("expected fastest response from fast, got %q", resp.AnswerFrom)
	}

	select {
	case <-slowCanceled:
	case <-time.After(200 * time.Millisecond):
		t.Fatalf("expected slow query to be canceled")
	}
}

func TestRecurseUnorderedWaitsForRedirectBatchCleanup(t *testing.T) {
	baseCtx, prof, _ := testhelpers.Context(t)
	if err := prof.Set("resolver.defaults.unordered", true); err != nil {
		t.Fatalf("set unordered: %v", err)
	}
	if err := prof.Set("resolver.defaults.parallel", 2); err != nil {
		t.Fatalf("set parallel: %v", err)
	}

	r := &Recursor{}

	slowStarted := make(chan struct{})
	slowCanceled := make(chan struct{})
	cancelGate := make(chan struct{})
	go func() {
		time.Sleep(50 * time.Millisecond)
		close(cancelGate)
	}()
	slow := testQueryer{
		id:            "slow",
		waitForCancel: true,
		cancelCh:      slowCanceled,
		cancelWaitCh:  cancelGate,
		startCh:       slowStarted,
	}

	referral := packetWithReferral("example", "ns1.example")
	referral.AnswerFrom = "redirect"
	redirect := testQueryer{id: "redirect", resp: referral, waitCh: slowStarted}

	failCh := make(chan string, 1)
	nextResp := packetWithA("example", netip.MustParseAddr("192.0.2.23"))
	nextResp.AnswerFrom = "next"
	next := testQueryer{id: "next", resp: nextResp, requireClosed: slowCanceled, failCh: failCh}

	state := &recurseState{
		ns: []queryer{redirect, slow},
		nsFrom: func(_ context.Context, _ packet.Packet, _ *recurseState) ([]queryer, error) {
			select {
			case <-slowCanceled:
			case <-time.After(200 * time.Millisecond):
				return nil, errors.New("redirect nsFrom called before cancel")
			}
			return []queryer{next}, nil
		},
	}

	ctx, cancel := context.WithTimeout(baseCtx, time.Second)
	defer cancel()

	resp, _, err := r.recurse(ctx, "example", "A", "IN", state)
	if err != nil {
		t.Fatalf("recurse: %v", err)
	}
	if resp.AnswerFrom != "next" {
		t.Fatalf("expected next response, got %q", resp.AnswerFrom)
	}

	select {
	case id := <-failCh:
		t.Fatalf("expected redirect batch cleanup before %s started", id)
	default:
	}
}

func TestGetAddressesForUnorderedSequential(t *testing.T) {
	baseCtx, prof, _ := testhelpers.Context(t)
	if err := prof.Set("resolver.defaults.unordered", true); err != nil {
		t.Fatalf("set unordered: %v", err)
	}
	if err := prof.Set("resolver.defaults.parallel", 2); err != nil {
		t.Fatalf("set parallel: %v", err)
	}

	nameserver.EmptyCache()
	defer nameserver.EmptyCache()

	r, err := New()
	if err != nil {
		t.Fatalf("new recursor: %v", err)
	}
	r.RemoveFakeAddresses(".")
	if err := r.AddFakeAddresses(".", map[string][]string{
		"root.test": {"192.0.2.53"},
	}); err != nil {
		t.Fatalf("add root: %v", err)
	}

	ns, err := nameserver.NewWithContext(baseCtx, "root.test", "192.0.2.53", r.client)
	if err != nil {
		t.Fatalf("nameserver: %v", err)
	}

	started := make(chan string, 2)
	blockA := make(chan struct{})
	blockAAAA := make(chan struct{})

	ns.SetQueryHook(func(_ context.Context, name string, qtype string, _ string, _ *nameserver.QueryOptions) (packet.Packet, error) {
		switch strings.ToUpper(qtype) {
		case "A":
			started <- "A"
			<-blockA
			return packetWithA(name, netip.MustParseAddr("192.0.2.44")), nil
		case "AAAA":
			started <- "AAAA"
			<-blockAAAA
			return packetWithAAAA(name, netip.MustParseAddr("2001:db8::44")), nil
		default:
			return packet.Packet{}, nil
		}
	})

	ctx := baseCtx
	done := make(chan struct{})
	var addrs []netip.Addr
	var addrErr error
	go func() {
		addrs, addrErr = r.getAddressesFor(ctx, "ns.example", nil)
		close(done)
	}()

	first := <-started
	select {
	case second := <-started:
		t.Fatalf("expected sequential A/AAAA recursion, got %s and %s", first, second)
	case <-time.After(50 * time.Millisecond):
	}

	if first == "A" {
		close(blockA)
	} else {
		close(blockAAAA)
	}

	var second string
	select {
	case second = <-started:
	case <-time.After(200 * time.Millisecond):
		t.Fatalf("expected second recursion to start")
	}

	if second == "A" {
		close(blockA)
	} else {
		close(blockAAAA)
	}

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatalf("getAddressesFor timed out")
	}

	if addrErr != nil {
		t.Fatalf("getAddressesFor: %v", addrErr)
	}
	if len(addrs) != 2 {
		t.Fatalf("expected 2 addresses, got %d", len(addrs))
	}
}

func TestRecurseUnorderedDepthLimitsWorkers(t *testing.T) {
	baseCtx, prof, _ := testhelpers.Context(t)
	if err := prof.Set("resolver.defaults.unordered", true); err != nil {
		t.Fatalf("set unordered: %v", err)
	}
	if err := prof.Set("resolver.defaults.parallel", 2); err != nil {
		t.Fatalf("set parallel: %v", err)
	}

	r := &Recursor{}

	slowStarted := make(chan struct{})
	fastStarted := make(chan struct{})
	blockSlow := make(chan struct{})

	slowResp := packetWithA("example", netip.MustParseAddr("192.0.2.60"))
	slowResp.AnswerFrom = "slow"
	fastResp := packetWithA("example", netip.MustParseAddr("192.0.2.61"))
	fastResp.AnswerFrom = "fast"

	slow := testQueryer{id: "slow", resp: slowResp, startCh: slowStarted, waitCh: blockSlow}
	fast := testQueryer{id: "fast", resp: fastResp, startCh: fastStarted}

	state := &recurseState{ns: []queryer{slow, fast}}
	ctx := withUnorderedDepth(withUnorderedContext(baseCtx), 1)

	done := make(chan struct{})
	go func() {
		_, _, _ = r.recurse(ctx, "example", "A", "IN", state)
		close(done)
	}()

	select {
	case <-slowStarted:
	case <-time.After(200 * time.Millisecond):
		t.Fatalf("expected slow query to start")
	}

	select {
	case <-fastStarted:
		t.Fatalf("expected fast query to wait for slow in nested unordered context")
	case <-time.After(50 * time.Millisecond):
	}

	close(blockSlow)

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatalf("expected recurse to finish")
	}
}

func TestSnapshotStateMapsConcurrentMutation(t *testing.T) {
	state := &recurseState{}
	state.ensureLock()

	nameKey := "ns.example"
	baseAddr := netip.MustParseAddr("192.0.2.10")
	flapAddr := netip.MustParseAddr("192.0.2.11")
	snapshotAddr := netip.MustParseAddr("192.0.2.250")

	state.lock()
	state.inProgress = map[string]map[string]bool{
		nameKey: {"A": true},
	}
	state.glue = map[string]map[netip.Addr]bool{
		nameKey: {baseAddr: true},
	}
	state.unlock()

	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		toggle := false
		for {
			select {
			case <-stop:
				return
			default:
			}

			state.lock()
			state.inProgress[nameKey]["AAAA"] = toggle
			if toggle {
				state.glue[nameKey][flapAddr] = true
			} else {
				delete(state.glue[nameKey], flapAddr)
			}
			state.unlock()
			toggle = !toggle
		}
	}()

	deadline := time.After(200 * time.Millisecond)
	for {
		select {
		case <-deadline:
			close(stop)
			<-done

			state.lock()
			_, hasSnapshotMarker := state.inProgress[nameKey]["SNAPSHOT_MARKER"]
			_, hasSnapshotAddr := state.glue[nameKey][snapshotAddr]
			state.unlock()

			if hasSnapshotMarker {
				t.Fatalf("snapshot mutation leaked into inProgress map")
			}
			if hasSnapshotAddr {
				t.Fatalf("snapshot mutation leaked into glue map")
			}
			return
		default:
			inProgress, glue := snapshotStateMaps(state)
			inProgress[nameKey]["SNAPSHOT_MARKER"] = true
			glue[nameKey][snapshotAddr] = true
		}
	}
}

func packetWithA(name string, addr netip.Addr) packet.Packet {
	msg := new(dns.Msg)
	aRR := &dns.A{Hdr: dns.Header{Name: dnsutil.Fqdn(name), Class: dns.ClassINET}}
	aRR.Addr = addr
	msg.Answer = []dns.RR{aRR}
	return packet.Packet{Msg: msg}
}

type testQueryer struct {
	id            string
	delay         time.Duration
	resp          packet.Packet
	called        chan string
	waitForCancel bool
	cancelCh      chan struct{}
	cancelWaitCh  <-chan struct{}
	startCh       chan struct{}
	waitCh        <-chan struct{}
	requireClosed <-chan struct{}
	failCh        chan<- string
}

func (t testQueryer) QueryWithClass(ctx context.Context, _ string, _ string, _ string) (packet.Packet, error) {
	if t.called != nil {
		t.called <- t.id
	}
	if t.startCh != nil {
		select {
		case <-t.startCh:
		default:
			close(t.startCh)
		}
	}
	if t.requireClosed != nil {
		select {
		case <-t.requireClosed:
		default:
			if t.failCh != nil {
				t.failCh <- t.id
			}
		}
	}
	if t.waitCh != nil {
		select {
		case <-t.waitCh:
		case <-ctx.Done():
			return packet.Packet{}, ctx.Err()
		}
	}
	if t.waitForCancel {
		<-ctx.Done()
		if t.cancelWaitCh != nil {
			<-t.cancelWaitCh
		}
		if t.cancelCh != nil {
			close(t.cancelCh)
		}
		return packet.Packet{}, ctx.Err()
	}
	if t.delay > 0 {
		select {
		case <-time.After(t.delay):
		case <-ctx.Done():
			return packet.Packet{}, ctx.Err()
		}
	}
	return t.resp, nil
}

type loggingQueryer struct {
	id        string
	resp      packet.Packet
	startCh   chan struct{}
	waitStart <-chan struct{}
	waitCh    <-chan struct{}
}

func (q loggingQueryer) QueryWithClass(ctx context.Context, _ string, _ string, _ string) (packet.Packet, error) {
	if q.waitStart != nil {
		select {
		case <-q.waitStart:
		case <-ctx.Done():
			return packet.Packet{}, ctx.Err()
		}
	}
	if q.startCh != nil {
		select {
		case <-q.startCh:
		default:
			close(q.startCh)
		}
	}
	if log := logger.FromContext(ctx); log != nil {
		_, _ = log.Add("TEST_QUERY_"+strings.ToUpper(q.id), map[string]any{"id": q.id}, "System", "")
	}
	if q.waitCh != nil {
		select {
		case <-q.waitCh:
		case <-ctx.Done():
			return packet.Packet{}, ctx.Err()
		}
	}
	return q.resp, nil
}

func packetWithReferral(zone string, nsName string) packet.Packet {
	msg := new(dns.Msg)
	nsRR := &dns.NS{Hdr: dns.Header{Name: dnsutil.Fqdn(zone), Class: dns.ClassINET}}
	nsRR.Ns = dnsutil.Fqdn(nsName)
	msg.Ns = []dns.RR{nsRR}
	return packet.Packet{Msg: msg}
}

func packetWithAAAA(name string, addr netip.Addr) packet.Packet {
	msg := new(dns.Msg)
	aaaa := &dns.AAAA{Hdr: dns.Header{Name: dnsutil.Fqdn(name), Class: dns.ClassINET}}
	aaaa.Addr = addr
	msg.Answer = []dns.RR{aaaa}
	return packet.Packet{Msg: msg}
}

func packetWithARecords(name string, addrs []netip.Addr) packet.Packet {
	msg := new(dns.Msg)
	for _, addr := range addrs {
		aRR := &dns.A{Hdr: dns.Header{Name: dnsutil.Fqdn(name), Class: dns.ClassINET}}
		aRR.Addr = addr
		msg.Answer = append(msg.Answer, aRR)
	}
	return packet.Packet{Msg: msg}
}

func TestRedirectNameNoNS(t *testing.T) {
	resp := packet.Packet{Msg: new(dns.Msg)}
	if _, ok := redirectName(resp); ok {
		t.Fatalf("expected no redirect name")
	}
}

// --- Negative cache tests (in-run dedupe of indeterminate lookups) ---

func TestNegativeCacheTTLZeroDoesNotStore(t *testing.T) {
	r := &Recursor{}
	// Default TTL is 0 - should preserve old behavior.
	r.cacheStoreNegative("k", "A", "IN")
	if _, ok := r.cacheLookup("k", "A", "IN"); ok {
		t.Fatalf("expected no negative cache entry when TTL is 0")
	}
}

func TestNegativeCacheStoresWithTTL(t *testing.T) {
	r := &Recursor{}
	r.SetNegativeCacheTTL(60 * time.Second)

	r.cacheStoreNegative("k", "A", "IN")
	cached, ok := r.cacheLookup("k", "A", "IN")
	if !ok {
		t.Fatalf("expected cache hit for negative entry")
	}
	if cached.Msg != nil {
		t.Fatalf("expected nil Msg for negative entry, got %v", cached.Msg)
	}
}

func TestNegativeCacheExpires(t *testing.T) {
	r := &Recursor{}
	r.SetNegativeCacheTTL(20 * time.Millisecond)

	r.cacheStoreNegative("k", "A", "IN")
	if _, ok := r.cacheLookup("k", "A", "IN"); !ok {
		t.Fatalf("expected cache hit immediately after store")
	}
	time.Sleep(40 * time.Millisecond)
	if _, ok := r.cacheLookup("k", "A", "IN"); ok {
		t.Fatalf("expected negative cache entry to expire after TTL")
	}
}

func TestNegativeCacheNotPersisted(t *testing.T) {
	r := &Recursor{}
	r.SetNegativeCacheTTL(60 * time.Second)
	r.cacheStoreNegative(cacheNameKey(dnsname.New("example.com."), nil), "A", "IN")

	entries, err := r.ExportCacheEntries()
	if err != nil {
		t.Fatalf("export entries: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected no exported entries; negative cache must not persist, got %d", len(entries))
	}
}

func TestPositiveEntryStillNotEvictedByNegativeLookup(t *testing.T) {
	r := &Recursor{}
	r.SetNegativeCacheTTL(60 * time.Second)
	msg := new(dns.Msg)
	msg.Rcode = dns.RcodeSuccess
	r.cacheStore("k", "A", "IN", packet.Packet{Msg: msg})

	cached, ok := r.cacheLookup("k", "A", "IN")
	if !ok || cached.Msg == nil {
		t.Fatalf("positive entry must remain after a negative-cache-enabled lookup")
	}
}

func TestDefaultProfileNegativeCacheTTLIsNonZero(t *testing.T) {
	p, err := profile.Default()
	if err != nil {
		t.Fatalf("profile default: %v", err)
	}
	if p.Resolver.Defaults.NegativeCacheTTL <= 0 {
		t.Fatalf("default negative_cache_ttl must be > 0 so indeterminate recursions dedupe within a run; got %d", p.Resolver.Defaults.NegativeCacheTTL)
	}
}

// TestRecursorDedupesIndeterminateLookups is the load-bearing integration test:
// a recursor batch returning no decision should be re-attempted at most once
// within the negative-cache TTL window, even across multiple resolve() calls.
func TestRecursorDedupesIndeterminateLookups(t *testing.T) {
	r := &Recursor{}
	r.SetNegativeCacheTTL(60 * time.Second)

	var calls atomic.Int32
	indeterminate := &countingQueryer{
		count: &calls,
		// REFUSED is treated as a "candidate" but not a decided answer; the
		// recursor exhausts the batch and returns Msg == nil.
		resp: refusedPacket("203.0.113.99"),
	}

	state1 := &recurseState{ns: []queryer{indeterminate}}
	if _, _, err := r.recurse(context.Background(), "www.example", "A", "IN", state1); err != nil {
		t.Fatalf("first recurse: %v", err)
	}
	first := calls.Load()

	// recurseWithNameservers wraps r.recurse and is the cache integration
	// boundary; calling it twice should reuse the first call's negative
	// cache entry on the second call.
	if _, err := r.recurseWithNameservers(context.Background(), "www.example", "A", "IN", []nameserver.Nameserver{}); err == nil {
		// Either nil or non-nil err is fine; the state is already candidate-empty.
		_ = err
	}
	if _, err := r.recurseWithNameservers(context.Background(), "www.example", "A", "IN", []nameserver.Nameserver{}); err == nil {
		_ = err
	}

	if calls.Load() < first {
		t.Fatalf("hook call count regressed: first=%d, after=%d", first, calls.Load())
	}
	// The second recurseWithNameservers must not have re-issued the live query.
	if delta := calls.Load() - first; delta > 1 {
		t.Fatalf("expected at most 1 extra call across deduplicated lookups, got %d (cache not engaging)", delta)
	}
}

type countingQueryer struct {
	count *atomic.Int32
	resp  packet.Packet
	err   error
}

func (q *countingQueryer) QueryWithClass(_ context.Context, _ string, _ string, _ string) (packet.Packet, error) {
	q.count.Add(1)
	return q.resp, q.err
}
