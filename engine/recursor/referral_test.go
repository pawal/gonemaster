package recursor

import (
	"context"
	"fmt"
	"net/netip"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	dns "codeberg.org/miekg/dns"

	"codeberg.org/pawal/gonemaster/engine/dnsname"
	"codeberg.org/pawal/gonemaster/engine/internal/dnstest"
	"codeberg.org/pawal/gonemaster/engine/internal/testhelpers"
	"codeberg.org/pawal/gonemaster/engine/nameserver"
	"codeberg.org/pawal/gonemaster/engine/packet"
)

// rootFixture is a root (a.root.test) referring names to the deepest cut in
// delegate, and servers answering A by default; queries are counted per server.
type rootFixture struct {
	r          *Recursor
	ttl        uint32
	delegate   map[string]string
	addrs      map[string]string
	answers    map[string]func(name string, qtype string) packet.Packet
	rootAnswer func(name string) (packet.Packet, bool)
	rootGate   chan struct{}
	calls      map[string]*atomic.Int32
}

func newRootFixture(t *testing.T, ctx context.Context, ttl uint32) *rootFixture {
	t.Helper()
	f := &rootFixture{
		r:   fakeRootRecursor(t, "a.root.test", "192.0.2.1"),
		ttl: ttl,
		delegate: map[string]string{
			"example":      "ns1.example",
			"other":        "ns1.other",
			"in-addr.arpa": "ns1.in-addr.arpa",
		},
		addrs: map[string]string{
			"ns1.example":      "192.0.2.53",
			"ns2.example":      "192.0.2.55",
			"ns1.other":        "192.0.2.54",
			"ns1.in-addr.arpa": "192.0.2.56",
		},
		answers: map[string]func(string, string) packet.Packet{},
		calls:   map[string]*atomic.Int32{".": {}},
	}
	hookedNS(t, ctx, f.r, "a.root.test", "192.0.2.1", func(_ context.Context, name string, _ string, _ string, _ *nameserver.QueryOptions) (packet.Packet, error) {
		f.calls["."].Add(1)
		if f.rootGate != nil {
			<-f.rootGate
		}
		if f.rootAnswer != nil {
			if resp, ok := f.rootAnswer(name); ok {
				return resp, nil
			}
		}
		labels := dnsname.New(name).Labels()
		for i := range len(labels) {
			cut := strings.Join(labels[i:], ".")
			server, ok := f.delegate[cut]
			if !ok {
				continue
			}
			if i == 0 {
				return dnstest.NoData(cut), nil
			}
			return dnstest.From(dnstest.Referral(cut, server+"."),
				dnstest.Additional(dnstest.ARR(server, f.addrs[server])),
				withNSTTL(f.ttl)), nil
		}
		return nxdomainPacket("192.0.2.1"), nil
	})
	for server, addr := range f.addrs {
		f.calls[server] = &atomic.Int32{}
		hookedNS(t, ctx, f.r, server, addr, func(_ context.Context, name string, qtype string, _ string, _ *nameserver.QueryOptions) (packet.Packet, error) {
			f.calls[server].Add(1)
			if answer, ok := f.answers[server]; ok {
				return answer(name, qtype), nil
			}
			return packetWithA(name, netip.MustParseAddr("198.51.100.1")), nil
		})
	}
	return f
}

// withNSTTL sets the TTL of the NS records in the authority section.
func withNSTTL(ttl uint32) dnstest.MsgOpt {
	return func(p *packet.Packet) {
		for _, rr := range p.Msg.Ns {
			rr.Header().TTL = ttl
		}
	}
}

func (f *rootFixture) count(server string) int32 {
	return f.calls[server].Load()
}

// lookup resolves name A from the root; safe to call from any goroutine.
func (f *rootFixture) lookup(ctx context.Context, name string) (packet.Packet, error) {
	resp, err := f.r.Recurse(ctx, name, "A", "IN")
	if err == nil && resp.Msg == nil {
		err = fmt.Errorf("recurse %s: no response", name)
	}
	return resp, err
}

func (f *rootFixture) mustRecurse(t *testing.T, ctx context.Context, name string) packet.Packet {
	t.Helper()
	resp, err := f.lookup(ctx, name)
	if err != nil {
		t.Fatalf("recurse %s: %v", name, err)
	}
	return resp
}

// cachedReferral returns the root referral cached for cut, if any.
func cachedReferral(r *Recursor, cut string) (packet.Packet, bool) {
	r.cacheMu.Lock()
	defer r.cacheMu.Unlock()
	entry, ok := r.referrals[cut]
	return entry.resp, ok
}

// A second lookup under the same TLD starts at the cached TLD server, in both resolver modes.
func TestRecurseStartsAtCachedTLDReferral(t *testing.T) {
	for _, unordered := range []bool{true, false} {
		t.Run(map[bool]string{true: "unordered", false: "ordered"}[unordered], func(t *testing.T) {
			ctx, prof, _ := testhelpers.Context(t)
			prof.Resolver.Defaults.Unordered = unordered
			f := newRootFixture(t, ctx, 3600)

			f.mustRecurse(t, ctx, "a.example")
			f.mustRecurse(t, ctx, "b.example")

			if got := f.count("."); got != 1 {
				t.Fatalf("root queried %d times, want 1", got)
			}
			if got := f.count("ns1.example"); got != 2 {
				t.Fatalf("TLD server queried %d times, want 2", got)
			}
		})
	}
}

// Address lookups and out-of-bailiwick CNAME restarts also start at the cache;
// the root is only asked for the "other" referral.
func TestRootStartedLookupsUseCachedReferral(t *testing.T) {
	ctx, _, _ := testhelpers.Context(t)
	f := newRootFixture(t, ctx, 3600)
	f.answers["ns1.other"] = func(name string, _ string) packet.Packet {
		return cnamePacket(name, "t.example", "192.0.2.54")
	}
	f.mustRecurse(t, ctx, "a.example")

	if _, err := f.r.GetAddressesFor(ctx, "ns9.example"); err != nil {
		t.Fatalf("addresses: %v", err)
	}
	if got := f.count("."); got != 1 {
		t.Fatalf("root queried %d times after address lookup, want 1", got)
	}

	resp := f.mustRecurse(t, ctx, "alias.other")
	if len(resp.GetRecordsForName("A", dnsname.New("t.example"), "answer")) == 0 {
		t.Fatalf("expected the CNAME target's A record, got %v", resp.Msg)
	}
	if got := f.count("."); got != 2 {
		t.Fatalf("root queried %d times after CNAME restart, want 2", got)
	}
}

// A lookup for the TLD name itself goes to the root, which holds data such as its DS.
func TestRecurseForTLDNameStartsAtRoot(t *testing.T) {
	ctx, _, _ := testhelpers.Context(t)
	f := newRootFixture(t, ctx, 3600)
	f.mustRecurse(t, ctx, "a.example")

	if _, err := f.r.Recurse(ctx, "example", "DS", "IN"); err != nil {
		t.Fatalf("recurse DS: %v", err)
	}
	if got := f.count("."); got != 2 {
		t.Fatalf("root queried %d times, want 2", got)
	}
	if got := f.count("ns1.example"); got != 1 {
		t.Fatalf("TLD server queried %d times, want 1", got)
	}
}

// TTL 0 is never cached and an entry expires with its NS TTL (synctest moves the clock).
func TestReferralCacheHonoursTTL(t *testing.T) {
	t.Run("zero TTL is not cached", func(t *testing.T) {
		ctx, _, _ := testhelpers.Context(t)
		f := newRootFixture(t, ctx, 0)
		f.mustRecurse(t, ctx, "a.example")
		f.mustRecurse(t, ctx, "b.example")
		if got := f.count("."); got != 2 {
			t.Fatalf("root queried %d times, want 2", got)
		}
	})
	t.Run("entry expires with the NS TTL", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			ctx, _, _ := testhelpers.Context(t)
			f := newRootFixture(t, ctx, 60)
			f.mustRecurse(t, ctx, "a.example")
			f.mustRecurse(t, ctx, "b.example")
			if got := f.count("."); got != 1 {
				t.Fatalf("root queried %d times within the TTL, want 1", got)
			}
			time.Sleep(61 * time.Second)
			f.mustRecurse(t, ctx, "c.example")
			if got := f.count("."); got != 2 {
				t.Fatalf("root queried %d times after the TTL, want 2", got)
			}
		})
	})
}

// A cached TLD server that goes silent sends the lookup back to the root,
// which now refers to ns2.example; the cache must pick up the new referral.
func TestRecurseFallsBackToRootWhenCachedServersFail(t *testing.T) {
	ctx, _, _ := testhelpers.Context(t)
	f := newRootFixture(t, ctx, 3600)
	f.mustRecurse(t, ctx, "a.example")

	f.answers["ns1.example"] = func(string, string) packet.Packet { return packet.Packet{} }
	f.delegate["example"] = "ns2.example"

	f.mustRecurse(t, ctx, "b.example")
	if got := f.count("."); got != 2 {
		t.Fatalf("root queried %d times, want 2", got)
	}
	if got := f.count("ns2.example"); got != 1 {
		t.Fatalf("new TLD server queried %d times, want 1", got)
	}
	cached, ok := cachedReferral(f.r, "example")
	if !ok {
		t.Fatalf("expected a refreshed referral")
	}
	if ns, isNS := cached.GetRecords("NS")[0].(*dns.NS); !isNS || ns.Ns != "ns2.example." {
		t.Fatalf("cached referral = %v, want ns2.example", cached.Msg.Ns)
	}
	f.mustRecurse(t, ctx, "c.example")
	if got := f.count("ns2.example"); got != 2 {
		t.Fatalf("refreshed referral not used: new TLD server queried %d times, want 2", got)
	}
}

// A referral learned from the old root set must not survive a change of the root set.
func TestReferralCacheClearedWhenRootChanges(t *testing.T) {
	root := map[string][]string{"a.root.test": {"192.0.2.1"}}
	tests := []struct {
		name  string
		apply func(r *Recursor) error
	}{
		{"ClearCache", func(r *Recursor) error { r.ClearCache(); return nil }},
		{"SetUndelegatedRoot", func(r *Recursor) error { return r.SetUndelegatedRoot(root) }},
		{"AddFakeAddresses", func(r *Recursor) error { return r.AddFakeAddresses(".", root) }},
		{"RemoveFakeAddresses", func(r *Recursor) error { r.RemoveFakeAddresses("."); return nil }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, _, _ := testhelpers.Context(t)
			f := newRootFixture(t, ctx, 3600)
			f.mustRecurse(t, ctx, "a.example")
			if _, ok := cachedReferral(f.r, "example"); !ok {
				t.Fatalf("expected a cached referral before %s", tt.name)
			}
			if err := tt.apply(f.r); err != nil {
				t.Fatalf("%s: %v", tt.name, err)
			}
			if _, ok := cachedReferral(f.r, "example"); ok {
				t.Fatalf("referral survived %s", tt.name)
			}
		})
	}
}

// Parent reads the parent zone from the referrals it walks, so it always starts
// at the root; an explicit server set is asked directly.
func TestParentAndExplicitNameserversBypassReferralCache(t *testing.T) {
	ctx, _, _ := testhelpers.Context(t)
	f := newRootFixture(t, ctx, 3600)
	f.mustRecurse(t, ctx, "a.example")

	pname, _, err := f.r.Parent(ctx, "b.example")
	if err != nil {
		t.Fatalf("parent: %v", err)
	}
	if pname != "example" {
		t.Fatalf("parent = %q, want example", pname)
	}
	if got := f.count("."); got != 2 {
		t.Fatalf("root queried %d times, want 2: Parent must walk from the root", got)
	}

	custom := hookedNS(t, ctx, f.r, "custom.test", "192.0.2.99", func(_ context.Context, name string, _ string, _ string, _ *nameserver.QueryOptions) (packet.Packet, error) {
		return packetWithA(name, netip.MustParseAddr("198.51.100.2")), nil
	})
	before := f.count("ns1.example")
	if _, err := f.r.RecurseWithNameservers(ctx, "c.example", "A", "IN", []nameserver.Nameserver{custom}); err != nil {
		t.Fatalf("recurse with nameservers: %v", err)
	}
	if got := f.count("ns1.example"); got != before {
		t.Fatalf("cached TLD server queried for an explicit server set")
	}
}

// The root refers straight to in-addr.arpa; siblings reuse that cut, ip6.arpa does not.
func TestRecurseCachesRootReferralBelowTLD(t *testing.T) {
	ctx, _, _ := testhelpers.Context(t)
	f := newRootFixture(t, ctx, 3600)

	for _, name := range []string{"1.2.0.192.in-addr.arpa", "2.2.0.192.in-addr.arpa"} {
		if _, err := f.r.Recurse(ctx, name, "PTR", "IN"); err != nil {
			t.Fatalf("recurse %s: %v", name, err)
		}
	}
	if got := f.count("."); got != 1 {
		t.Fatalf("root queried %d times for two in-addr.arpa names, want 1", got)
	}
	if _, err := f.r.Recurse(ctx, "1.0.ip6.arpa", "PTR", "IN"); err != nil {
		t.Fatalf("recurse ip6.arpa: %v", err)
	}
	if got := f.count("."); got != 2 {
		t.Fatalf("root queried %d times after an ip6.arpa name, want 2", got)
	}
}

// Cold lookups under one TLD started together ask the root once; the others
// wait for that referral while the root is held.
func TestConcurrentLookupsShareOneRootReferral(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, _, _ := testhelpers.Context(t)
		f := newRootFixture(t, ctx, 3600)
		f.rootGate = make(chan struct{})

		names := []string{"a.example", "b.example", "c.example", "d.example"}
		errs := make([]error, len(names))
		var wg sync.WaitGroup
		for i, name := range names {
			wg.Go(func() { _, errs[i] = f.lookup(ctx, name) })
		}
		synctest.Wait()
		if got := f.count("."); got != 1 {
			t.Fatalf("root queried %d times while held, want 1", got)
		}
		close(f.rootGate)
		wg.Wait()

		for i, err := range errs {
			if err != nil {
				t.Fatalf("lookup %s: %v", names[i], err)
			}
		}
		if got := f.count("."); got != 1 {
			t.Fatalf("root queried %d times, want 1", got)
		}
		if got := f.count("ns1.example"); got != int32(len(names)) {
			t.Fatalf("TLD server queried %d times, want %d", got, len(names))
		}
	})
}

// A root CNAME into the same TLD must not wait on the walk's own claim; synctest
// reports that wait as a deadlock. Both modes reach resolveCNAME differently.
func TestRootCNAMEDoesNotWaitOnOwnClaim(t *testing.T) {
	for _, unordered := range []bool{true, false} {
		t.Run(map[bool]string{true: "unordered", false: "ordered"}[unordered], func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				ctx, prof, _ := testhelpers.Context(t)
				prof.Resolver.Defaults.Unordered = unordered
				f := newRootFixture(t, ctx, 3600)
				f.rootAnswer = func(name string) (packet.Packet, bool) {
					if dnsname.New(name).String() != "a.example" {
						return packet.Packet{}, false
					}
					return cnamePacket(name, "b.example", "192.0.2.1"), true
				}

				resp := f.mustRecurse(t, ctx, "a.example")
				if len(resp.GetRecordsForName("A", dnsname.New("b.example"), "answer")) == 0 {
					t.Fatalf("expected the CNAME target's A record, got %v", resp.Msg)
				}
			})
		})
	}
}

// A waiter goes to the root itself once referralWaitLimit passes, and a claim
// that ends without a referral lets all waiters go to the root together.
func TestReferralWaitIsBoundedAndOneShot(t *testing.T) {
	t.Run("bounded", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			ctx, _, _ := testhelpers.Context(t)
			f := newRootFixture(t, ctx, 3600)
			f.rootGate = make(chan struct{})

			var wg sync.WaitGroup
			wg.Go(func() { _, _ = f.lookup(ctx, "a.example") })
			synctest.Wait()
			wg.Go(func() { _, _ = f.lookup(ctx, "b.example") })
			synctest.Wait()
			if got := f.count("."); got != 1 {
				t.Fatalf("root queried %d times before the limit, want 1", got)
			}
			time.Sleep(referralWaitLimit)
			synctest.Wait()
			if got := f.count("."); got != 2 {
				t.Fatalf("root queried %d times after the limit, want 2", got)
			}
			close(f.rootGate)
			wg.Wait()
		})
	})
	t.Run("one-shot", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			ctx, _, _ := testhelpers.Context(t)
			f := newRootFixture(t, ctx, 3600)
			first, rest := make(chan struct{}), make(chan struct{})
			// The root answers directly, so the first walk stores no referral.
			f.rootAnswer = func(name string) (packet.Packet, bool) {
				if dnsname.New(name).String() == "a.example" {
					<-first
				} else {
					<-rest
				}
				return packetWithA(name, netip.MustParseAddr("198.51.100.3")), true
			}

			var wg sync.WaitGroup
			wg.Go(func() { _, _ = f.lookup(ctx, "a.example") })
			synctest.Wait()
			for _, name := range []string{"b.example", "c.example"} {
				wg.Go(func() { _, _ = f.lookup(ctx, name) })
			}
			synctest.Wait()
			close(first)
			synctest.Wait()
			if got := f.count("."); got != 3 {
				t.Fatalf("root queried %d times, want 3: waiters must not queue behind each other", got)
			}
			close(rest)
			wg.Wait()
		})
	})
}
