package basic

import (
	"net"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	dns "codeberg.org/miekg/dns"

	"codeberg.org/pawal/gonemaster/engine/internal/testhelpers"
	"codeberg.org/pawal/gonemaster/engine/logger"
	"codeberg.org/pawal/gonemaster/engine/packet"
	"codeberg.org/pawal/gonemaster/engine/test/internal/tctest"
)

var onePathRootNames = []string{"ns1.root", "ns2.root", "ns3.root"}

// onePathHealthyRoot serves the root zone and refers "example" to ns.example.
func onePathHealthyRoot(q tctest.Query) packet.Packet {
	switch {
	case q.Name == "." && q.Type == "SOA":
		return soaPacket(".", "ns1.root", "hostmaster.root")
	case q.Name == "." && q.Type == "NS":
		return nsPacketMulti(".", onePathRootNames...)
	case strings.EqualFold(q.Name, "example") && q.Type == "SOA":
		return referralPacket("example", "ns.example", net.IPv4(192, 0, 2, 10))
	}
	return packet.Packet{}
}

// runBasic01OnePath runs Basic01 with broken overriding roots and child answering child.example.
func runBasic01OnePath(t *testing.T, zoneName string, broken map[string]tctest.Handler, child tctest.Handler, ipv6Root bool) ([]*logger.Entry, map[string]int32) {
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
		tctest.NSOn(t, ctx, r, name, addr, func(q tctest.Query) packet.Packet {
			counter.Add(1)
			return hook(q)
		})
	}
	tctest.NSOn(t, ctx, r, "ns.example", "192.0.2.10", func(q tctest.Query) packet.Packet {
		switch {
		case strings.EqualFold(q.Name, "example") && q.Type == "SOA":
			return soaPacket("example", "ns.example", "hostmaster.example")
		case strings.EqualFold(q.Name, "example") && q.Type == "NS":
			return tctest.Response(tctest.Answers(tctest.NSRR("example", "ns.example")),
				tctest.Additional(tctest.ARR("ns.example", "192.0.2.10")))
		case strings.EqualFold(q.Name, "child.example"):
			return child(q)
		}
		return packet.Packet{}
	})
	tctest.NSOn(t, ctx, r, "ns.stale.example", "192.0.2.30", nil)

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

func delegatedChild(tctest.Query) packet.Packet {
	return referralPacket("child.example", "ns.child.example", net.IPv4(192, 0, 2, 20))
}

// requireParentExample fails unless ns.example is reported as the parent of the child.
func requireParentExample(t *testing.T, entries []*logger.Entry) {
	t.Helper()
	for _, entry := range entries {
		if entry == nil || entry.Tag != "B01_PARENT_FOUND" || entry.Args["domain"] != "example" {
			continue
		}
		if got := tctest.ServerEndpoints(t, entry.Args); !slices.Equal(got, []string{"ns.example/192.0.2.10"}) {
			t.Fatalf("B01_PARENT_FOUND for example = %v, want ns.example/192.0.2.10", got)
		}
		return
	}
	t.Fatalf("expected B01_PARENT_FOUND for example, got %v", tctest.Tags(entries))
}

// A zone below a TLD is walked through the first working root only; child and parent are found.
func TestBasic01FollowsOneRootPath(t *testing.T) {
	silent := func(tctest.Query) packet.Packet { return packet.Packet{} }
	refused := func(tctest.Query) packet.Packet { return rcodePacket(dns.RcodeRefused) }
	nxdomainTLD := func(q tctest.Query) packet.Packet {
		if q.Name == "." {
			return onePathHealthyRoot(q)
		}
		return nxdomainAAPacket()
	}
	badSOA := func(q tctest.Query) packet.Packet {
		if q.Name == "." && q.Type == "SOA" {
			return emptyAnswerPacket()
		}
		return onePathHealthyRoot(q)
	}
	staleReferral := func(q tctest.Query) packet.Packet {
		if strings.EqualFold(q.Name, "example") {
			return referralPacket("example", "ns.stale.example", net.IPv4(192, 0, 2, 30))
		}
		return onePathHealthyRoot(q)
	}

	tests := []struct {
		name        string
		broken      map[string]tctest.Handler
		ipv6Root    bool
		wantAsked   []string
		wantSkipped []string
		zoneErrors  []string
	}{
		{name: "first root works", wantAsked: []string{"ns1.root"}, wantSkipped: []string{"ns2.root", "ns3.root"}},
		{name: "first root silent", broken: map[string]tctest.Handler{"ns1.root": silent},
			wantAsked: []string{"ns1.root", "ns2.root"}, wantSkipped: []string{"ns3.root"}, zoneErrors: []string{"ns1.root"}},
		{name: "first root refuses", broken: map[string]tctest.Handler{"ns1.root": refused},
			wantAsked: []string{"ns1.root", "ns2.root"}, wantSkipped: []string{"ns3.root"}, zoneErrors: []string{"ns1.root"}},
		{name: "first root bad SOA", broken: map[string]tctest.Handler{"ns1.root": badSOA},
			wantAsked: []string{"ns1.root", "ns2.root"}, wantSkipped: []string{"ns3.root"}, zoneErrors: []string{"ns1.root"}},
		// An NXDOMAIN root is a parent without the child, and the walk falls back to every root.
		{name: "first root NXDOMAIN for the TLD", broken: map[string]tctest.Handler{"ns1.root": nxdomainTLD},
			wantAsked: []string{"ns1.root", "ns2.root", "ns3.root"}},
		{name: "first root refers to dead servers", broken: map[string]tctest.Handler{"ns1.root": staleReferral},
			wantAsked: []string{"ns1.root", "ns2.root", "ns3.root"}, zoneErrors: []string{"ns.stale.example"}},
		{name: "all but the last root broken", broken: map[string]tctest.Handler{"ns1.root": refused, "ns2.root": silent},
			wantAsked: []string{"ns1.root", "ns2.root", "ns3.root"}, zoneErrors: []string{"ns1.root", "ns2.root"}},
		{name: "IPv6 root first with IPv6 disabled", ipv6Root: true, wantAsked: []string{"ns2.root"}, wantSkipped: []string{"ns1.root", "ns3.root"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entries, counts := runBasic01OnePath(t, "child.example", tt.broken, delegatedChild, tt.ipv6Root)

			tctest.RequireTag(t, entries, "B01_CHILD_FOUND")
			tctest.RequireNoTag(t, entries, "B01_NO_CHILD", "B01_PARENT_NOT_FOUND")
			requireParentExample(t, entries)
			zoneErrors := tctest.ArgValues(entries, "B01_SERVER_ZONE_ERROR", "ns")
			for _, root := range tt.zoneErrors {
				if !slices.Contains(zoneErrors, root) {
					t.Fatalf("expected B01_SERVER_ZONE_ERROR for %s, got %v", root, tctest.Tags(entries))
				}
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
	missing := func(tctest.Query) packet.Packet { return nxdomainAAPacket() }
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
