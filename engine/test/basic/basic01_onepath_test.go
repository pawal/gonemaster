package basic

import (
	"context"
	"net"
	"strings"
	"sync/atomic"
	"testing"

	dns "codeberg.org/miekg/dns"

	"codeberg.org/pawal/gonemaster/engine/internal/testhelpers"
	"codeberg.org/pawal/gonemaster/engine/logger"
	"codeberg.org/pawal/gonemaster/engine/nameserver"
	"codeberg.org/pawal/gonemaster/engine/packet"
	"codeberg.org/pawal/gonemaster/engine/test/internal/tctest"
)

var onePathRootNames = []string{"ns1.root", "ns2.root", "ns3.root"}

// onePathHealthyRoot serves the root zone and refers "example" to ns.example.
func onePathHealthyRoot(_ context.Context, name string, qtype string, _ string, _ *nameserver.QueryOptions) (packet.Packet, error) {
	switch {
	case name == "." && qtype == "SOA":
		return soaPacket(".", "ns1.root", "hostmaster.root"), nil
	case name == "." && qtype == "NS":
		return nsPacketMulti(".", onePathRootNames...), nil
	case strings.EqualFold(name, "example") && qtype == "SOA":
		return referralPacket("example", "ns.example", net.IPv4(192, 0, 2, 10)), nil
	}
	return packet.Packet{}, nil
}

// runBasic01OnePath runs Basic01 for zoneName against three roots, ns.example
// (answering child.example SOA through child) and a silent ns.stale.example.
// broken replaces a root's answers; the query count per root is returned.
func runBasic01OnePath(t *testing.T, zoneName string, broken map[string]tctest.RawHandler, child tctest.RawHandler, ipv6Root bool) ([]*logger.Entry, map[string]int32) {
	t.Helper()
	ctx, prof, _ := testhelpers.Context(t)
	prof.Net.IPv6 = !ipv6Root

	addrs := map[string]string{"ns1.root": "192.0.2.1", "ns2.root": "192.0.2.2", "ns3.root": "192.0.2.3"}
	if ipv6Root {
		addrs["ns1.root"] = "2001:db8::1"
	}
	hints := map[string][]string{}
	for name, addr := range addrs {
		hints[name] = []string{addr}
	}
	r := tctest.Recursor(t, map[string]map[string][]string{".": hints})

	counts := map[string]*atomic.Int32{}
	for name, addr := range addrs {
		hook, ok := broken[name]
		if !ok {
			hook = onePathHealthyRoot
		}
		counter := &atomic.Int32{}
		counts[name] = counter
		tctest.NSRaw(t, ctx, r, name, addr, func(ctx context.Context, qname string, qtype string, qclass string, opts *nameserver.QueryOptions) (packet.Packet, error) {
			counter.Add(1)
			return hook(ctx, qname, qtype, qclass, opts)
		})
	}
	tctest.NSRaw(t, ctx, r, "ns.example", "192.0.2.10", func(ctx context.Context, name string, qtype string, qclass string, opts *nameserver.QueryOptions) (packet.Packet, error) {
		switch {
		case strings.EqualFold(name, "example") && qtype == "SOA":
			return soaPacket("example", "ns.example", "hostmaster.example"), nil
		case strings.EqualFold(name, "example") && qtype == "NS":
			return tctest.Response(tctest.Answers(tctest.NSRR("example", "ns.example")),
				tctest.Additional(tctest.ARR("ns.example", "192.0.2.10"))), nil
		case strings.EqualFold(name, "child.example"):
			return child(ctx, name, qtype, qclass, opts)
		}
		return packet.Packet{}, nil
	})
	tctest.NSRaw(t, ctx, r, "ns.stale.example", "192.0.2.30", func(context.Context, string, string, string, *nameserver.QueryOptions) (packet.Packet, error) {
		return packet.Packet{}, nil
	})

	entries, err := Basic01(ctx, tctest.Zone(t, zoneName, r))
	if err != nil {
		t.Fatalf("basic01: %v", err)
	}
	out := map[string]int32{}
	for name, counter := range counts {
		out[name] = counter.Load()
	}
	return entries, out
}

func delegatedChild(context.Context, string, string, string, *nameserver.QueryOptions) (packet.Packet, error) {
	return referralPacket("child.example", "ns.child.example", net.IPv4(192, 0, 2, 20)), nil
}

// requireParentExample fails unless ns.example is reported as the parent of the child.
func requireParentExample(t *testing.T, entries []*logger.Entry) {
	t.Helper()
	for _, entry := range entries {
		if entry == nil || entry.Tag != "B01_PARENT_FOUND" || entry.Args["domain"] != "example" {
			continue
		}
		servers := tctest.Servers(t, entry.Args["servers"])
		if len(servers) == 1 && servers[0]["ns"] == "ns.example" && servers[0]["address"] == "192.0.2.10" {
			return
		}
		t.Fatalf("B01_PARENT_FOUND for example = %v, want ns.example/192.0.2.10", servers)
	}
	t.Fatalf("expected B01_PARENT_FOUND for example, got %v", tctest.Tags(entries))
}

// requireZoneError fails unless root reported B01_SERVER_ZONE_ERROR.
func requireZoneError(t *testing.T, entries []*logger.Entry, root string) {
	t.Helper()
	for _, entry := range entries {
		if entry != nil && entry.Tag == "B01_SERVER_ZONE_ERROR" && entry.Args["ns"] == root {
			return
		}
	}
	t.Fatalf("expected B01_SERVER_ZONE_ERROR for %s, got %v", root, tctest.Tags(entries))
}

// TestBasic01FollowsOneRootPath checks that a zone below a TLD is walked
// through the first working root only, and that Basic01 still proves the child
// exists, with the same parent, whichever way the first roots fail.
func TestBasic01FollowsOneRootPath(t *testing.T) {
	silent := func(context.Context, string, string, string, *nameserver.QueryOptions) (packet.Packet, error) {
		return packet.Packet{}, nil
	}
	refused := func(context.Context, string, string, string, *nameserver.QueryOptions) (packet.Packet, error) {
		return rcodePacket(dns.RcodeRefused), nil
	}
	nxdomainTLD := func(ctx context.Context, name string, qtype string, qclass string, opts *nameserver.QueryOptions) (packet.Packet, error) {
		if name == "." {
			return onePathHealthyRoot(ctx, name, qtype, qclass, opts)
		}
		return nxdomainAAPacket(), nil
	}
	badSOA := func(ctx context.Context, name string, qtype string, qclass string, opts *nameserver.QueryOptions) (packet.Packet, error) {
		if name == "." && qtype == "SOA" {
			return emptyAnswerPacket(), nil
		}
		return onePathHealthyRoot(ctx, name, qtype, qclass, opts)
	}
	staleReferral := func(ctx context.Context, name string, qtype string, qclass string, opts *nameserver.QueryOptions) (packet.Packet, error) {
		if strings.EqualFold(name, "example") {
			return referralPacket("example", "ns.stale.example", net.IPv4(192, 0, 2, 30)), nil
		}
		return onePathHealthyRoot(ctx, name, qtype, qclass, opts)
	}

	tests := []struct {
		name        string
		broken      map[string]tctest.RawHandler
		ipv6Root    bool
		wantAsked   []string
		wantSkipped []string
		zoneErrors  []string
	}{
		{name: "first root works", wantAsked: []string{"ns1.root"}, wantSkipped: []string{"ns2.root", "ns3.root"}},
		{name: "first root silent", broken: map[string]tctest.RawHandler{"ns1.root": silent},
			wantAsked: []string{"ns1.root", "ns2.root"}, wantSkipped: []string{"ns3.root"}, zoneErrors: []string{"ns1.root"}},
		{name: "first root refuses", broken: map[string]tctest.RawHandler{"ns1.root": refused},
			wantAsked: []string{"ns1.root", "ns2.root"}, wantSkipped: []string{"ns3.root"}, zoneErrors: []string{"ns1.root"}},
		{name: "first root bad SOA", broken: map[string]tctest.RawHandler{"ns1.root": badSOA},
			wantAsked: []string{"ns1.root", "ns2.root"}, wantSkipped: []string{"ns3.root"}, zoneErrors: []string{"ns1.root"}},
		// An NXDOMAIN from a root makes it a parent without the child, so the
		// walk falls back to every root, as the full walk would.
		{name: "first root NXDOMAIN for the TLD", broken: map[string]tctest.RawHandler{"ns1.root": nxdomainTLD},
			wantAsked: []string{"ns1.root", "ns2.root", "ns3.root"}},
		{name: "first root refers to dead servers", broken: map[string]tctest.RawHandler{"ns1.root": staleReferral},
			wantAsked: []string{"ns1.root", "ns2.root", "ns3.root"}, zoneErrors: []string{"ns.stale.example"}},
		{name: "all but the last root broken", broken: map[string]tctest.RawHandler{"ns1.root": refused, "ns2.root": silent},
			wantAsked: []string{"ns1.root", "ns2.root", "ns3.root"}, zoneErrors: []string{"ns1.root", "ns2.root"}},
		{name: "IPv6 root first with IPv6 disabled", ipv6Root: true, wantAsked: []string{"ns2.root"}, wantSkipped: []string{"ns1.root", "ns3.root"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entries, counts := runBasic01OnePath(t, "child.example", tt.broken, delegatedChild, tt.ipv6Root)

			tctest.RequireTag(t, entries, "B01_CHILD_FOUND")
			tctest.RequireNoTag(t, entries, "B01_NO_CHILD", "B01_PARENT_NOT_FOUND")
			requireParentExample(t, entries)
			for _, root := range tt.zoneErrors {
				requireZoneError(t, entries, root)
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

// A child that does not exist still gets B01_NO_CHILD, after every root was tried.
func TestBasic01OneRootPathStillReportsNoChild(t *testing.T) {
	missing := func(context.Context, string, string, string, *nameserver.QueryOptions) (packet.Packet, error) {
		return nxdomainAAPacket(), nil
	}
	entries, counts := runBasic01OnePath(t, "child.example", nil, missing, false)

	tctest.RequireTag(t, entries, "B01_NO_CHILD")
	for _, name := range onePathRootNames {
		if counts[name] == 0 {
			t.Errorf("%s was never asked", name)
		}
	}
}

// For a TLD the roots are the parent, so every root is probed.
func TestBasic01ProbesEveryRootForTLD(t *testing.T) {
	entries, counts := runBasic01OnePath(t, "example", nil, delegatedChild, false)

	tctest.RequireTag(t, entries, "B01_CHILD_FOUND")
	for _, name := range onePathRootNames {
		if counts[name] == 0 {
			t.Errorf("%s was never asked", name)
		}
	}
}
