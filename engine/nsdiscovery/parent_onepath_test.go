package nsdiscovery

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"

	dns "codeberg.org/miekg/dns"

	"codeberg.org/pawal/gonemaster/engine/internal/dnstest"
	"codeberg.org/pawal/gonemaster/engine/internal/nstest"
	"codeberg.org/pawal/gonemaster/engine/internal/testhelpers"
	"codeberg.org/pawal/gonemaster/engine/nameserver"
	"codeberg.org/pawal/gonemaster/engine/packet"
)

// onePathRoot is a root server in the one-path fixture; broken, when set,
// replaces the healthy answers.
type onePathRoot struct {
	name   string
	addr   string
	broken nstest.Hook
}

// healthyRoot serves the root zone and refers "example" to ns.example.
func healthyRoot(names []string) nstest.Hook {
	return func(_ context.Context, name string, qtype string, _ string, _ *nameserver.QueryOptions) (packet.Packet, error) {
		switch {
		case name == "." && qtype == "SOA":
			return dnstest.Response(dnstest.Answers(dnstest.SOARR("."))), nil
		case name == "." && qtype == "NS":
			return dnstest.Response(dnstest.Answers(dnstest.NSRRs(".", names...)...)), nil
		case strings.EqualFold(name, "example") && qtype == "SOA":
			return referralTo("example", "ns.example", "192.0.2.10"), nil
		}
		return packet.Packet{}, nil
	}
}

func referralTo(zoneName string, nsName string, addr string) packet.Packet {
	return dnstest.From(dnstest.Referral(zoneName, nsName+"."), dnstest.Additional(dnstest.ARR(nsName, addr)))
}

// runOnePath builds the roots, ns.example (which delegates child.example) and a
// silent ns.stale.example, then walks zoneName. It returns the parent set and
// the number of queries each root received.
func runOnePath(t *testing.T, zoneName string, roots []onePathRoot, ipv6 bool) ([]nameserver.Nameserver, map[string]int32) {
	t.Helper()
	ctx, prof, _ := testhelpers.Context(t)
	ctx = WithCache(ctx, NewCache())
	prof.Net.IPv6 = ipv6

	hints := map[string][]string{}
	var names []string
	for _, root := range roots {
		hints[root.name] = []string{root.addr}
		names = append(names, root.name+".")
	}
	r := nstest.Recursor(t, map[string]map[string][]string{".": hints})

	calls := map[string]*atomic.Int32{}
	for _, root := range roots {
		hook := root.broken
		if hook == nil {
			hook = healthyRoot(names)
		}
		counter := &atomic.Int32{}
		calls[root.name] = counter
		nstest.HookedNS(t, ctx, r, root.name, root.addr, func(ctx context.Context, name string, qtype string, qclass string, opts *nameserver.QueryOptions) (packet.Packet, error) {
			counter.Add(1)
			return hook(ctx, name, qtype, qclass, opts)
		})
	}
	nstest.HookedNS(t, ctx, r, "ns.example", "192.0.2.10", func(_ context.Context, name string, qtype string, _ string, _ *nameserver.QueryOptions) (packet.Packet, error) {
		switch {
		case strings.EqualFold(name, "example") && qtype == "SOA":
			return dnstest.Response(dnstest.Answers(dnstest.SOARR("example"))), nil
		case strings.EqualFold(name, "example") && qtype == "NS":
			return dnstest.Response(dnstest.Answers(dnstest.NSRRs("example", "ns.example.")...),
				dnstest.Additional(dnstest.ARR("ns.example", "192.0.2.10"))), nil
		case strings.EqualFold(name, "child.example") && qtype == "SOA":
			return referralTo("child.example", "ns.child.example", "192.0.2.20"), nil
		}
		return packet.Packet{}, nil
	})
	nstest.HookedNS(t, ctx, r, "ns.stale.example", "192.0.2.30", nstest.PacketHook(packet.Packet{}))

	z := newZone(t, zoneName, r)
	parent, err := ParentNameservers(ctx, &z)
	if err != nil {
		t.Fatalf("ParentNameservers: %v", err)
	}
	counts := map[string]int32{}
	for name, counter := range calls {
		counts[name] = counter.Load()
	}
	return parent, counts
}

// TestParentNameserversFollowsOneRootPath checks that a zone below a TLD is
// walked through the first working root only, and that every way the first
// roots can fail still ends with the parent set the full walk finds.
func TestParentNameserversFollowsOneRootPath(t *testing.T) {
	silent := nstest.PacketHook(packet.Packet{})
	refused := nstest.PacketHook(dnstest.Response(dnstest.Rcode(dns.RcodeRefused)))
	names := []string{"ns1.root.", "ns2.root.", "ns3.root."}
	nxdomainTLD := func(ctx context.Context, name string, qtype string, qclass string, opts *nameserver.QueryOptions) (packet.Packet, error) {
		if name == "." {
			return healthyRoot(names)(ctx, name, qtype, qclass, opts)
		}
		return dnstest.Response(dnstest.NXDOMAIN()), nil
	}
	badSOA := func(ctx context.Context, name string, qtype string, qclass string, opts *nameserver.QueryOptions) (packet.Packet, error) {
		if name == "." && qtype == "SOA" {
			return dnstest.NoData("."), nil
		}
		return healthyRoot(names)(ctx, name, qtype, qclass, opts)
	}
	staleReferral := func(ctx context.Context, name string, qtype string, qclass string, opts *nameserver.QueryOptions) (packet.Packet, error) {
		if strings.EqualFold(name, "example") {
			return referralTo("example", "ns.stale.example", "192.0.2.30"), nil
		}
		return healthyRoot(names)(ctx, name, qtype, qclass, opts)
	}

	tests := []struct {
		name        string
		first       nstest.Hook
		second      nstest.Hook
		ipv6Root    bool
		wantAsked   []string
		wantSkipped []string
	}{
		{name: "first root works", wantAsked: []string{"ns1.root"}, wantSkipped: []string{"ns2.root", "ns3.root"}},
		{name: "first root silent", first: silent, wantAsked: []string{"ns1.root", "ns2.root"}, wantSkipped: []string{"ns3.root"}},
		{name: "first root refuses", first: refused, wantAsked: []string{"ns1.root", "ns2.root"}, wantSkipped: []string{"ns3.root"}},
		{name: "first root NXDOMAIN for the TLD", first: nxdomainTLD, wantAsked: []string{"ns1.root", "ns2.root"}, wantSkipped: []string{"ns3.root"}},
		{name: "first root bad SOA", first: badSOA, wantAsked: []string{"ns1.root", "ns2.root"}, wantSkipped: []string{"ns3.root"}},
		// The stale referral leads nowhere, so the walk falls back to every root.
		{name: "first root refers to dead servers", first: staleReferral, wantAsked: []string{"ns1.root", "ns2.root", "ns3.root"}},
		{name: "all but the last root broken", first: refused, second: silent, wantAsked: []string{"ns1.root", "ns2.root", "ns3.root"}},
		{name: "IPv6 root first with IPv6 disabled", ipv6Root: true, wantAsked: []string{"ns2.root"}, wantSkipped: []string{"ns1.root", "ns3.root"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			roots := []onePathRoot{
				{name: "ns1.root", addr: "192.0.2.1", broken: tt.first},
				{name: "ns2.root", addr: "192.0.2.2", broken: tt.second},
				{name: "ns3.root", addr: "192.0.2.3"},
			}
			if tt.ipv6Root {
				roots[0].addr = "2001:db8::1"
			}
			parent, counts := runOnePath(t, "child.example", roots, !tt.ipv6Root)
			if len(parent) != 1 || parent[0].String() != "ns.example/192.0.2.10" {
				t.Fatalf("parent set = %v, want [ns.example/192.0.2.10]", parent)
			}
			for _, name := range tt.wantAsked {
				if counts[name] == 0 {
					t.Errorf("%s was never asked", name)
				}
			}
			for _, name := range tt.wantSkipped {
				if counts[name] != 0 {
					t.Errorf("%s was asked %d times, want 0", name, counts[name])
				}
			}
		})
	}
}

// For a TLD the roots are the parent, so every root is probed.
func TestParentNameserversProbesEveryRootForTLD(t *testing.T) {
	roots := []onePathRoot{
		{name: "ns1.root", addr: "192.0.2.1"},
		{name: "ns2.root", addr: "192.0.2.2"},
		{name: "ns3.root", addr: "192.0.2.3"},
	}
	parent, counts := runOnePath(t, "example", roots, true)
	if len(parent) != 3 {
		t.Fatalf("parent set = %v, want all three roots", parent)
	}
	for name, n := range counts {
		if n == 0 {
			t.Errorf("%s was never asked", name)
		}
	}
}
