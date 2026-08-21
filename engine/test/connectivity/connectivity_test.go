package connectivity

import (
	"context"
	"net/netip"
	"strings"
	"testing"
	"testing/synctest"

	"codeberg.org/pawal/gonemaster/engine/asnlookup"
	"codeberg.org/pawal/gonemaster/engine/dnsname"
	"codeberg.org/pawal/gonemaster/engine/logger"
	"codeberg.org/pawal/gonemaster/engine/nameserver"
	"codeberg.org/pawal/gonemaster/engine/nsdiscovery"
	"codeberg.org/pawal/gonemaster/engine/packet"
	"codeberg.org/pawal/gonemaster/engine/profile"
	"codeberg.org/pawal/gonemaster/engine/recursor"
	"codeberg.org/pawal/gonemaster/engine/test/internal/tctest"
	"codeberg.org/pawal/gonemaster/engine/zone"
)

func TestConnectivity01IPv6Disabled(t *testing.T) {
	ctx := tctest.Context(t)

	ns4 := tctest.NS(t, ctx, "ns1.example", "192.0.2.1", nil)
	ns6 := tctest.NS(t, ctx, "ns2.example", "2001:db8::1", nil)
	tctest.Stub(t, &authoritativeNS, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{ns4, ns6}, nil
	})

	profile.Effective().Net.IPv6 = false

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Connectivity01(ctx, &z)
	if err != nil {
		t.Fatalf("connectivity01: %v", err)
	}
	entry := tctest.RequireTag(t, entries, "CN01_IPV6_DISABLED")
	if _, ok := entry.Args["ns_list"]; ok {
		t.Fatalf("did not expect legacy ns_list key in args")
	}
	names := tctest.ServerNames(t, entry.Args)
	if len(names) != 1 || names[0] != "ns2.example" {
		t.Fatalf("expected typed servers with ns2.example, got %v", names)
	}
}

// TestConnectivity01EmitsCNAMETagForUnresolvableNS verifies that when zone
// NS discovery returns an address-less NSItem carrying a *recursor.CNAMEError,
// Connectivity01 emits the matching CNAME_* tag. authoritativeNS is stubbed to
// return a resolvable nameserver so the rest of the testcase proceeds normally.
func TestConnectivity01EmitsCNAMETagForUnresolvableNS(t *testing.T) {
	ctx := tctest.Context(t)

	ns := tctest.NS(t, ctx, "ns1.example", "192.0.2.1", nil)
	tctest.Stub(t, &authoritativeNS, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{ns}, nil
	})
	cnameErr := &recursor.CNAMEError{
		Reason: recursor.CNAMEUnresolved,
		Name:   "ns.outside.test",
		Target: "loop.outside.test",
		Detail: "loop",
	}
	tctest.Stub(t, &zoneNameservers, func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
		return []nsdiscovery.NSItem{
			{Name: dnsname.New("ns1.example"), Address: netip.MustParseAddr("192.0.2.1"), HasAddress: true},
			{Name: dnsname.New("ns.outside.test"), Err: cnameErr},
		}, nil
	})

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Connectivity01(ctx, &z)
	if err != nil {
		t.Fatalf("connectivity01: %v", err)
	}

	entry := tctest.RequireTag(t, entries, "CNAME_TARGET_UNRESOLVED")
	if got := entry.Args["query_name"]; got != "ns.outside.test" {
		t.Fatalf("query_name: got %#v, want ns.outside.test", got)
	}
}

func TestConnectivityLoopNoResponse(t *testing.T) {
	ctx := tctest.Context(t)

	ns := tctest.NS(t, ctx, "ns1.example", "192.0.2.1", nil)
	results := []*logger.Entry{}

	if err := connectivityLoop(ctx, "Connectivity01", dnsname.New("example"), []nameserver.Nameserver{ns}, &results); err != nil {
		t.Fatalf("connectivity loop: %v", err)
	}
	tctest.RequireTags(t, results, "CN01_NO_RESPONSE_UDP")
}

func TestConnectivityLoopWrongSOAOwner(t *testing.T) {
	ctx := tctest.Context(t)

	ns := tctest.NS(t, ctx, "ns1.example", "192.0.2.1", func(q tctest.Query) packet.Packet {
		switch strings.ToUpper(q.Type) {
		case "SOA":
			return soaPacket("wrong.example", "ns1.example", "hostmaster.example")
		case "NS":
			return nsPacket("example", "ns1.example")
		default:
			return packet.Packet{}
		}
	})

	results := []*logger.Entry{}
	if err := connectivityLoop(ctx, "Connectivity01", dnsname.New("example"), []nameserver.Nameserver{ns}, &results); err != nil {
		t.Fatalf("connectivity loop: %v", err)
	}
	tctest.RequireTags(t, results, "CN01_WRONG_SOA_RECORD_UDP")
}

func TestConnectivity03SameASNSet(t *testing.T) {
	ctx := tctest.Context(t)

	ns1 := tctest.NS(t, ctx, "ns1.example", "192.0.2.1", nil)
	ns2 := tctest.NS(t, ctx, "ns2.example", "192.0.2.2", nil)
	tctest.Stub(t, &authoritativeNS, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{ns1, ns2}, nil
	})

	prefix, err := netip.ParsePrefix("192.0.2.0/24")
	if err != nil {
		t.Fatalf("parse prefix: %v", err)
	}
	tctest.Stub(t, &lookupASN, func(_ context.Context, _ asnlookup.Resolver, _ netip.Addr) (asnlookup.Result, error) {
		return asnlookup.Result{
			ASNs:   []int{64500, 64501},
			Prefix: &prefix,
			Raw:    "64500 64501 | 192.0.2.0/24 | test",
			Code:   asnlookup.CodeFound,
		}, nil
	})

	z, err := zone.New("example")
	if err != nil {
		t.Fatalf("new zone: %v", err)
	}
	entries, err := Connectivity03(ctx, &z)
	if err != nil {
		t.Fatalf("connectivity03: %v", err)
	}
	sameASN := tctest.RequireTag(t, entries, "IPV4_SAME_ASN")
	if _, ok := sameASN.Args["asn_list"]; ok {
		t.Fatalf("did not expect legacy asn_list key in args")
	}
	asns := tctest.Ints(t, sameASN.Args, "asns")
	if len(asns) != 2 || asns[0] != 64500 || asns[1] != 64501 {
		t.Fatalf("expected asns [64500 64501], got %v", asns)
	}
	announce := tctest.RequireTag(t, entries, "ASN_INFOS_ANNOUNCE_BY")
	if _, ok := announce.Args["asn"]; ok {
		t.Fatalf("did not expect legacy asn key in ASN_INFOS_ANNOUNCE_BY args")
	}
	announceASNs := tctest.Ints(t, announce.Args, "asns")
	if len(announceASNs) != 2 || announceASNs[0] != 64500 || announceASNs[1] != 64501 {
		t.Fatalf("expected announce asns [64500 64501], got %v", announceASNs)
	}
	announceIn := tctest.RequireTag(t, entries, "ASN_INFOS_ANNOUNCE_IN")
	if _, ok := announceIn.Args["prefix"]; ok {
		t.Fatalf("did not expect legacy prefix key in ASN_INFOS_ANNOUNCE_IN args")
	}
	prefixes := tctest.Strings(t, announceIn.Args, "prefixes")
	if len(prefixes) != 1 || prefixes[0] != "192.0.2.0/24" {
		t.Fatalf("expected prefixes [192.0.2.0/24], got %v", prefixes)
	}
}

func TestConnectivityLoopParallelQueries(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := tctest.Context(t)

		profile.Effective().Resolver.Defaults.Parallel = 2

		gate := tctest.NewGate()
		hook := func(q tctest.Query) packet.Packet {
			if strings.EqualFold(q.Type, "SOA") {
				gate.Arrive(q.Name)
			}
			return packet.Packet{}
		}

		ns1 := tctest.NS(t, ctx, "ns1.example", "192.0.2.1", hook)
		ns2 := tctest.NS(t, ctx, "ns2.example", "192.0.2.2", hook)

		done := make(chan struct{})
		var results []*logger.Entry
		var loopErr error
		go func() {
			loopErr = connectivityLoop(ctx, "Connectivity01", dnsname.New("example"), []nameserver.Nameserver{ns1, ns2}, &results)
			close(done)
		}()

		// Both servers are asked for the same name, so count arrivals.
		synctest.Wait()
		if got := gate.InFlight(); len(got) != 2 {
			t.Fatalf("expected parallel SOA queries to start, got %v", got)
		}
		gate.Release()

		<-done
		if loopErr != nil {
			t.Fatalf("connectivity loop: %v", loopErr)
		}

		var order []string
		var addresses []string
		for _, entry := range tctest.All(results, "CN01_NO_RESPONSE_UDP") {
			tctest.RequireArgShape(t, entry, tctest.ArgShape{})
			if ns, ok := entry.Args["ns"].(string); ok {
				order = append(order, ns)
			}
			if address, ok := entry.Args["address"].(string); ok {
				addresses = append(addresses, address)
			}
		}
		if len(order) != 2 {
			t.Fatalf("expected 2 no-response entries, got %v", order)
		}
		if order[0] != "ns1.example" || order[1] != "ns2.example" {
			t.Fatalf("expected deterministic nameserver order, got %v", order)
		}
		if len(addresses) != 2 {
			t.Fatalf("expected 2 no-response addresses, got %v", addresses)
		}
		if addresses[0] != "192.0.2.1" || addresses[1] != "192.0.2.2" {
			t.Fatalf("expected deterministic address order, got %v", addresses)
		}
	})
}

func TestConnectivity03ParallelASNLookups(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := tctest.Context(t)

		profile.Effective().Resolver.Defaults.Parallel = 2

		ns1 := tctest.NS(t, ctx, "ns1.example", "192.0.2.1", nil)
		ns2 := tctest.NS(t, ctx, "ns2.example", "192.0.2.2", nil)
		tctest.Stub(t, &authoritativeNS, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
			return []nameserver.Nameserver{ns1, ns2}, nil
		})

		gate := tctest.NewGate()
		prefix, err := netip.ParsePrefix("192.0.2.0/24")
		if err != nil {
			t.Fatalf("parse prefix: %v", err)
		}
		tctest.Stub(t, &lookupASN, func(_ context.Context, _ asnlookup.Resolver, ip netip.Addr) (asnlookup.Result, error) {
			gate.Arrive(ip.String())
			return asnlookup.Result{
				ASNs:   []int{64500},
				Prefix: &prefix,
				Code:   asnlookup.CodeFound,
			}, nil
		})

		z, err := zone.New("example")
		if err != nil {
			t.Fatalf("new zone: %v", err)
		}

		done := make(chan struct{})
		var entries []*logger.Entry
		var connErr error
		go func() {
			entries, connErr = Connectivity03(ctx, &z)
			close(done)
		}()

		synctest.Wait()
		gate.RequireInFlight(t, "192.0.2.1", "192.0.2.2")
		gate.Release()

		<-done
		if connErr != nil {
			t.Fatalf("connectivity03: %v", connErr)
		}

		var order []string
		for _, entry := range entries {
			if entry == nil || entry.Tag != "ASN_INFOS_ANNOUNCE_BY" {
				continue
			}
			if ip, ok := entry.Args["address"].(string); ok {
				order = append(order, ip)
			}
		}
		if len(order) != 2 {
			t.Fatalf("expected 2 announce-by entries, got %v", order)
		}
		if order[0] != "192.0.2.1" || order[1] != "192.0.2.2" {
			t.Fatalf("expected deterministic log order, got %v", order)
		}
	})
}

func TestConnectivity04SinglePrefix(t *testing.T) {
	ctx := tctest.Context(t)

	items := []nsdiscovery.NSItem{
		{
			Name:       dnsname.New("ns1.example"),
			Address:    netip.MustParseAddr("192.0.2.1"),
			HasAddress: true,
		},
		{
			Name:       dnsname.New("ns2.example"),
			Address:    netip.MustParseAddr("192.0.2.2"),
			HasAddress: true,
		},
	}
	tctest.Stub(t, &delegationNameservers, func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
		return items, nil
	})
	tctest.Stub(t, &zoneNameservers, func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
		return []nsdiscovery.NSItem{}, nil
	})

	prefix, err := netip.ParsePrefix("192.0.2.0/24")
	if err != nil {
		t.Fatalf("parse prefix: %v", err)
	}
	tctest.Stub(t, &lookupASN, func(_ context.Context, _ asnlookup.Resolver, _ netip.Addr) (asnlookup.Result, error) {
		return asnlookup.Result{
			Prefix: &prefix,
			Raw:    "64500 | 192.0.2.0/24 | test",
			Code:   asnlookup.CodeFound,
		}, nil
	})

	z, err := zone.New("example")
	if err != nil {
		t.Fatalf("new zone: %v", err)
	}
	entries, err := Connectivity04(ctx, &z)
	if err != nil {
		t.Fatalf("connectivity04: %v", err)
	}
	samePrefix := tctest.RequireTag(t, entries, "CN04_IPV4_SAME_PREFIX")
	if _, ok := samePrefix.Args["ns_list"]; ok {
		t.Fatalf("did not expect legacy ns_list key in args")
	}
	if _, ok := samePrefix.Args["ip_prefix"]; ok {
		t.Fatalf("did not expect legacy ip_prefix key in args")
	}
	names := tctest.ServerNames(t, samePrefix.Args)
	if len(names) != 2 || names[0] != "ns1.example" || names[1] != "ns2.example" {
		t.Fatalf("expected typed servers [ns1.example ns2.example], got %v", names)
	}
	prefixes := tctest.Strings(t, samePrefix.Args, "prefixes")
	if len(prefixes) != 1 || prefixes[0] != "192.0.2.0/24" {
		t.Fatalf("expected prefixes [192.0.2.0/24], got %v", prefixes)
	}
	announceIn := tctest.RequireTag(t, entries, "CN04_ASN_INFOS_ANNOUNCE_IN")
	if _, ok := announceIn.Args["prefix"]; ok {
		t.Fatalf("did not expect legacy prefix key in CN04_ASN_INFOS_ANNOUNCE_IN args")
	}
	announcePrefixes := tctest.Strings(t, announceIn.Args, "prefixes")
	if len(announcePrefixes) != 1 || announcePrefixes[0] != "192.0.2.0/24" {
		t.Fatalf("expected announce prefixes [192.0.2.0/24], got %v", announcePrefixes)
	}
	tctest.RequireTags(t, entries, "CN04_IPV4_SINGLE_PREFIX")
}

func soaPacket(owner string, mname string, rname string) packet.Packet {
	return tctest.Response(tctest.Answers(tctest.SOARR(owner,
		tctest.MName(mname), tctest.RName(rname), tctest.Serial(2024010101))))
}

func nsPacket(owner string, nsname string) packet.Packet {
	return tctest.Response(tctest.Answers(tctest.NSRR(owner, nsname)))
}
