package nsdiscovery

import (
	"context"
	"fmt"
	"net/netip"
	"sync/atomic"
	"testing"

	dns "codeberg.org/miekg/dns"
	"codeberg.org/miekg/dns/dnsutil"

	"codeberg.org/pawal/gonemaster/engine/internal/testhelpers"
	"codeberg.org/pawal/gonemaster/engine/nameserver"
	"codeberg.org/pawal/gonemaster/engine/packet"
	"codeberg.org/pawal/gonemaster/engine/recursor"
	"codeberg.org/pawal/gonemaster/engine/zone"
)

func TestDelegationNameserversUndelegated(t *testing.T) {
	ClearParentNSCache()
	defer ClearParentNSCache()
	ctx, _, _ := testhelpers.Context(t)

	r := &recursor.Recursor{}
	if err := r.AddFakeAddresses("example", map[string][]string{
		"ns1.example":     {"192.0.2.1"},
		"ns2.example.net": {"192.0.2.2"},
	}); err != nil {
		t.Fatalf("add fake addresses: %v", err)
	}
	z, err := zone.NewWithRecursor("example", r)
	if err != nil {
		t.Fatalf("new zone: %v", err)
	}

	items, err := DelegationNameservers(ctx, &z)
	if err != nil {
		t.Fatalf("DelegationNameservers: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	got := map[string]string{}
	for _, item := range items {
		if !item.HasAddress {
			t.Fatalf("expected address for %s", item.Name.String())
		}
		got[item.Name.String()] = item.Address.String()
	}
	if got["ns1.example"] != "192.0.2.1" {
		t.Fatalf("unexpected ns1.example address %q", got["ns1.example"])
	}
	if got["ns2.example.net"] != "192.0.2.2" {
		t.Fatalf("unexpected ns2.example.net address %q", got["ns2.example.net"])
	}
}

func TestZoneNSNamesUndelegatedIgnoresAuthoritativeApexSet(t *testing.T) {
	ClearParentNSCache()
	defer ClearParentNSCache()
	ctx, prof, _ := testhelpers.Context(t)
	prof.Net.IPv4 = true
	prof.Net.IPv6 = true

	r := &recursor.Recursor{}
	if err := r.AddFakeAddresses("example", map[string][]string{
		"ns1.example.net": {"192.0.2.53"},
	}); err != nil {
		t.Fatalf("add fake addresses: %v", err)
	}
	z, err := zone.NewWithRecursor("example", r)
	if err != nil {
		t.Fatalf("new zone: %v", err)
	}

	_ = newAuthoritativeNameserver(ctx, t, r, "ns1.example.net", "192.0.2.53", "example", "NS1.EXAMPLE.NET", "ns2.example.net")

	names, err := zoneNSNames(ctx, &z)
	if err != nil {
		t.Fatalf("zoneNSNames: %v", err)
	}
	want := []string{"ns1.example.net"}
	if len(names) != len(want) {
		t.Fatalf("expected %d names, got %d", len(want), len(names))
	}
	for i, name := range names {
		if name.String() != want[i] {
			t.Fatalf("expected %q at %d, got %q", want[i], i, name.String())
		}
	}
}

func TestZoneNameserversOutOfBailiwick(t *testing.T) {
	ClearParentNSCache()
	defer ClearParentNSCache()
	ctx, prof, _ := testhelpers.Context(t)
	prof.Net.IPv4 = true
	prof.Net.IPv6 = true

	r := &recursor.Recursor{}
	if err := r.AddFakeAddresses("example", map[string][]string{
		"ns1.example.net": {"192.0.2.53"},
		"ns2.example.net": {"192.0.2.54"},
	}); err != nil {
		t.Fatalf("add fake addresses: %v", err)
	}
	z, err := zone.NewWithRecursor("example", r)
	if err != nil {
		t.Fatalf("new zone: %v", err)
	}

	_ = newAuthoritativeNameserver(ctx, t, r, "ns1.example.net", "192.0.2.53", "example", "ns1.example.net", "ns2.example.net")
	_ = newAuthoritativeNameserver(ctx, t, r, "ns2.example.net", "192.0.2.54", "example", "ns1.example.net", "ns2.example.net")

	items, err := ZoneNameservers(ctx, &z)
	if err != nil {
		t.Fatalf("ZoneNameservers: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	got := map[string]string{}
	for _, item := range items {
		if !item.HasAddress {
			t.Fatalf("expected address for %s", item.Name.String())
		}
		got[item.Name.String()] = item.Address.String()
	}
	if got["ns1.example.net"] != "192.0.2.53" {
		t.Fatalf("unexpected ns1.example.net address %q", got["ns1.example.net"])
	}
	if got["ns2.example.net"] != "192.0.2.54" {
		t.Fatalf("unexpected ns2.example.net address %q", got["ns2.example.net"])
	}
}

func TestDelegationNameserversUndelegatedLookupWhenNoIP(t *testing.T) {
	ClearParentNSCache()
	defer ClearParentNSCache()
	ctx, prof, _ := testhelpers.Context(t)
	prof.Net.IPv4 = true
	prof.Net.IPv6 = true

	r := &recursor.Recursor{}
	if err := r.AddFakeAddresses(".", map[string][]string{
		"root.test": {"192.0.2.9"},
	}); err != nil {
		t.Fatalf("add fake root addresses: %v", err)
	}
	if err := r.AddFakeAddresses("example", map[string][]string{
		"ns1.example.net": {},
	}); err != nil {
		t.Fatalf("add fake addresses: %v", err)
	}

	rootNS, err := nameserver.NewWithContext(ctx, "root.test", "192.0.2.9", r.Client())
	if err != nil {
		t.Fatalf("new root nameserver: %v", err)
	}
	rootNS.SetQueryHook(func(_ context.Context, name string, qtype string, _ string, _ *nameserver.QueryOptions) (packet.Packet, error) {
		if name != "ns1.example.net" || qtype != "A" {
			return packet.Packet{}, nil
		}
		msg := new(dns.Msg)
		msg.Rcode = dns.RcodeSuccess
		aRR := &dns.A{}
		aRR.Hdr = dns.Header{Name: "ns1.example.net.", Class: dns.ClassINET}
		aRR.Addr = netip.AddrFrom4([4]byte{192, 0, 2, 99})
		msg.Answer = []dns.RR{aRR}
		return packet.Packet{Msg: msg}, nil
	})

	z, err := zone.NewWithRecursor("example", r)
	if err != nil {
		t.Fatalf("new zone: %v", err)
	}

	items, err := DelegationNameservers(ctx, &z)
	if err != nil {
		t.Fatalf("DelegationNameservers: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if !items[0].HasAddress {
		t.Fatalf("expected resolved address")
	}
	if items[0].Name.String() != "ns1.example.net" || items[0].Address.String() != "192.0.2.99" {
		t.Fatalf("unexpected item: %#v", items[0])
	}
}

func TestDelegationNameserversUndelegatedKeepsInBailiwickNameWithoutIP(t *testing.T) {
	ClearParentNSCache()
	defer ClearParentNSCache()
	ctx, _, _ := testhelpers.Context(t)

	r := &recursor.Recursor{}
	if err := r.AddFakeAddresses("example", map[string][]string{
		"ns1.example": {},
	}); err != nil {
		t.Fatalf("add fake addresses: %v", err)
	}
	z, err := zone.NewWithRecursor("example", r)
	if err != nil {
		t.Fatalf("new zone: %v", err)
	}

	items, err := DelegationNameservers(ctx, &z)
	if err != nil {
		t.Fatalf("DelegationNameservers: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if items[0].Name.String() != "ns1.example" || items[0].HasAddress {
		t.Fatalf("unexpected item: %#v", items[0])
	}
}

func TestZoneNSNamesUndelegatedUsesDelegationNames(t *testing.T) {
	ClearParentNSCache()
	defer ClearParentNSCache()
	ctx, _, _ := testhelpers.Context(t)

	r := &recursor.Recursor{}
	if err := r.AddFakeAddresses("example", map[string][]string{
		"ns2.example.net": {"192.0.2.54"},
		"ns1.example":     {"192.0.2.53"},
	}); err != nil {
		t.Fatalf("add fake addresses: %v", err)
	}
	z, err := zone.NewWithRecursor("example", r)
	if err != nil {
		t.Fatalf("new zone: %v", err)
	}

	names, err := zoneNSNames(ctx, &z)
	if err != nil {
		t.Fatalf("zoneNSNames: %v", err)
	}
	if len(names) != 2 {
		t.Fatalf("expected 2 names, got %d", len(names))
	}
	if names[0].String() != "ns1.example" || names[1].String() != "ns2.example.net" {
		t.Fatalf("unexpected names: %q, %q", names[0].String(), names[1].String())
	}
}

func TestZoneNameserversUndelegatedInBailiwickUsesProvidedGlue(t *testing.T) {
	ClearParentNSCache()
	defer ClearParentNSCache()
	ctx, _, _ := testhelpers.Context(t)

	r := &recursor.Recursor{}
	if err := r.AddFakeAddresses("example", map[string][]string{
		"ns1.example": {"192.0.2.53"},
	}); err != nil {
		t.Fatalf("add fake addresses: %v", err)
	}
	z, err := zone.NewWithRecursor("example", r)
	if err != nil {
		t.Fatalf("new zone: %v", err)
	}

	ns, err := nameserver.NewWithContext(ctx, "ns1.example", "192.0.2.53", r.Client())
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}
	queryCalls := 0
	ns.SetQueryHook(func(_ context.Context, _ string, _ string, _ string, _ *nameserver.QueryOptions) (packet.Packet, error) {
		queryCalls++
		return packet.Packet{}, nil
	})

	items, err := ZoneNameservers(ctx, &z)
	if err != nil {
		t.Fatalf("ZoneNameservers: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if !items[0].HasAddress || items[0].Name.String() != "ns1.example" || items[0].Address.String() != "192.0.2.53" {
		t.Fatalf("unexpected item: %#v", items[0])
	}
	if queryCalls != 0 {
		t.Fatalf("expected no A/AAAA lookup queries for provided glue, got %d", queryCalls)
	}
}

// delegationPacket builds a referral response for zoneName with NS records
// pointing to the given nsNames and optional A glue for in-bailiwick names.
func delegationPacket(zoneName string, nsGlue map[string]string) packet.Packet {
	msg := new(dns.Msg)
	msg.Rcode = dns.RcodeSuccess
	msg.Authoritative = false
	for nsName := range nsGlue {
		nsRR := &dns.NS{}
		nsRR.Hdr = dns.Header{Name: dnsutil.Fqdn(zoneName), Class: dns.ClassINET, TTL: 60}
		nsRR.Ns = dnsutil.Fqdn(nsName)
		msg.Ns = append(msg.Ns, nsRR)
	}
	for nsName, addr := range nsGlue {
		if addr == "" {
			continue
		}
		ip, err := netip.ParseAddr(addr)
		if err != nil {
			continue
		}
		if ip.Is4() {
			aRR := &dns.A{}
			aRR.Hdr = dns.Header{Name: dnsutil.Fqdn(nsName), Class: dns.ClassINET, TTL: 60}
			aRR.Addr = ip
			msg.Extra = append(msg.Extra, aRR)
		}
	}
	return packet.Packet{Msg: msg}
}

// authoritativeAPacket builds an authoritative A response.
func authoritativeAPacket(name string, addr string) packet.Packet {
	msg := new(dns.Msg)
	msg.Rcode = dns.RcodeSuccess
	msg.Authoritative = true
	ip, _ := netip.ParseAddr(addr)
	aRR := &dns.A{}
	aRR.Hdr = dns.Header{Name: dnsutil.Fqdn(name), Class: dns.ClassINET, TTL: 60}
	aRR.Addr = ip
	msg.Answer = append(msg.Answer, aRR)
	return packet.Packet{Msg: msg}
}

// ibTestRootHook returns a query hook for a root server that delegates
// zoneName to the specified nameservers with glue.
func ibTestRootHook(zoneName string, nsGlue map[string]string) func(context.Context, string, string, string, *nameserver.QueryOptions) (packet.Packet, error) {
	return func(_ context.Context, name string, qtype string, _ string, _ *nameserver.QueryOptions) (packet.Packet, error) {
		if name == "." && qtype == "SOA" {
			msg := new(dns.Msg)
			msg.Rcode = dns.RcodeSuccess
			msg.Authoritative = true
			soaRR := &dns.SOA{Hdr: dns.Header{Name: ".", Class: dns.ClassINET}}
			soaRR.Ns = "ns.root."
			soaRR.Mbox = "admin.root."
			soaRR.Serial = 1
			msg.Answer = []dns.RR{soaRR}
			return packet.Packet{Msg: msg}, nil
		}
		if name == "." && qtype == "NS" {
			msg := new(dns.Msg)
			msg.Rcode = dns.RcodeSuccess
			msg.Authoritative = true
			nsRR := &dns.NS{}
			nsRR.Hdr = dns.Header{Name: ".", Class: dns.ClassINET}
			nsRR.Ns = "ns.root."
			msg.Answer = []dns.RR{nsRR}
			aRR := &dns.A{}
			aRR.Hdr = dns.Header{Name: "ns.root.", Class: dns.ClassINET}
			aRR.Addr = netip.AddrFrom4([4]byte{192, 0, 2, 9})
			msg.Extra = []dns.RR{aRR}
			return packet.Packet{Msg: msg}, nil
		}
		if name == zoneName {
			return delegationPacket(zoneName, nsGlue), nil
		}
		return packet.Packet{}, nil
	}
}

// TestGetIBAddrInZoneSkipsDeadDelegationServer exercises in-bailiwick
// resolution where one delegation server is unreachable.
func TestGetIBAddrInZoneSkipsDeadDelegationServer(t *testing.T) {
	ClearParentNSCache()
	defer ClearParentNSCache()
	ctx, prof, _ := testhelpers.Context(t)
	prof.Net.IPv4 = true
	prof.Net.IPv6 = false

	r := &recursor.Recursor{}
	if err := r.AddFakeAddresses(".", map[string][]string{
		"ns.root": {"192.0.2.9"},
	}); err != nil {
		t.Fatal(err)
	}

	rootNS, err := nameserver.NewWithContext(ctx, "ns.root", "192.0.2.9", r.Client())
	if err != nil {
		t.Fatal(err)
	}
	rootNS.SetQueryHook(ibTestRootHook("example", map[string]string{
		"ns1.example":  "192.0.2.11",
		"ns2.example":  "192.0.2.12",
		"dead.example": "192.0.2.99",
	}))

	ibHook := func(name string, qtype string) (packet.Packet, error) {
		if name == "example" && qtype == "NS" {
			return authoritativeNSPacket("example", "ns1.example", "ns2.example", "dead.example"), nil
		}
		if qtype == "A" {
			switch name {
			case "ns1.example":
				return authoritativeAPacket("ns1.example", "192.0.2.11"), nil
			case "ns2.example":
				return authoritativeAPacket("ns2.example", "192.0.2.12"), nil
			case "dead.example":
				return authoritativeAPacket("dead.example", "192.0.2.99"), nil
			}
		}
		if qtype == "AAAA" {
			msg := new(dns.Msg)
			msg.Rcode = dns.RcodeSuccess
			msg.Authoritative = true
			return packet.Packet{Msg: msg}, nil
		}
		return packet.Packet{}, nil
	}

	ns1, err := nameserver.NewWithContext(ctx, "ns1.example", "192.0.2.11", r.Client())
	if err != nil {
		t.Fatal(err)
	}
	ns1.SetQueryHook(func(_ context.Context, name string, qtype string, _ string, _ *nameserver.QueryOptions) (packet.Packet, error) {
		return ibHook(name, qtype)
	})

	ns2, err := nameserver.NewWithContext(ctx, "ns2.example", "192.0.2.12", r.Client())
	if err != nil {
		t.Fatal(err)
	}
	ns2.SetQueryHook(func(_ context.Context, name string, qtype string, _ string, _ *nameserver.QueryOptions) (packet.Packet, error) {
		return ibHook(name, qtype)
	})

	var deadQueryCount atomic.Int32
	deadNS, err := nameserver.NewWithContext(ctx, "dead.example", "192.0.2.99", r.Client())
	if err != nil {
		t.Fatal(err)
	}
	deadNS.SetQueryHook(func(_ context.Context, _ string, _ string, _ string, _ *nameserver.QueryOptions) (packet.Packet, error) {
		deadQueryCount.Add(1)
		return packet.Packet{}, fmt.Errorf("connection timed out")
	})

	z, err := zone.NewWithRecursor("example", r)
	if err != nil {
		t.Fatal(err)
	}

	items, err := ZoneNameservers(ctx, &z)
	if err != nil {
		t.Fatalf("ZoneNameservers: %v", err)
	}

	got := map[string]string{}
	for _, item := range items {
		if item.HasAddress {
			got[item.Name.String()] = item.Address.String()
		}
	}
	if got["ns1.example"] != "192.0.2.11" {
		t.Errorf("ns1.example: want 192.0.2.11, got %q", got["ns1.example"])
	}
	if got["ns2.example"] != "192.0.2.12" {
		t.Errorf("ns2.example: want 192.0.2.12, got %q", got["ns2.example"])
	}

	dq := int(deadQueryCount.Load())
	if dq > 5 {
		t.Errorf("dead server queried %d times (expected <= 5 with dead-server skip)", dq)
	}
}

// TestGetIBAddrInZoneBreaksEarlyOnSuccess verifies that once a delegation
// server provides addresses for an in-bailiwick NS name, remaining servers
// are not tried for that name.
func TestGetIBAddrInZoneBreaksEarlyOnSuccess(t *testing.T) {
	ClearParentNSCache()
	defer ClearParentNSCache()
	ctx, prof, _ := testhelpers.Context(t)
	prof.Net.IPv4 = true
	prof.Net.IPv6 = false

	r := &recursor.Recursor{}
	if err := r.AddFakeAddresses(".", map[string][]string{
		"ns.root": {"192.0.2.9"},
	}); err != nil {
		t.Fatal(err)
	}

	rootNS, err := nameserver.NewWithContext(ctx, "ns.root", "192.0.2.9", r.Client())
	if err != nil {
		t.Fatal(err)
	}
	rootNS.SetQueryHook(ibTestRootHook("example", map[string]string{
		"ns1.example": "192.0.2.21",
		"ns2.example": "192.0.2.22",
	}))

	ibHook := func(counter *atomic.Int32) func(context.Context, string, string, string, *nameserver.QueryOptions) (packet.Packet, error) {
		return func(_ context.Context, name string, qtype string, _ string, _ *nameserver.QueryOptions) (packet.Packet, error) {
			counter.Add(1)
			if name == "example" && qtype == "NS" {
				return authoritativeNSPacket("example", "ns1.example", "ns2.example"), nil
			}
			if qtype == "A" {
				switch name {
				case "ns1.example":
					return authoritativeAPacket("ns1.example", "192.0.2.21"), nil
				case "ns2.example":
					return authoritativeAPacket("ns2.example", "192.0.2.22"), nil
				}
			}
			if qtype == "AAAA" {
				msg := new(dns.Msg)
				msg.Rcode = dns.RcodeSuccess
				msg.Authoritative = true
				return packet.Packet{Msg: msg}, nil
			}
			return packet.Packet{}, nil
		}
	}

	var ns1Count, ns2Count atomic.Int32
	ns1, err := nameserver.NewWithContext(ctx, "ns1.example", "192.0.2.21", r.Client())
	if err != nil {
		t.Fatal(err)
	}
	ns1.SetQueryHook(ibHook(&ns1Count))

	ns2, err := nameserver.NewWithContext(ctx, "ns2.example", "192.0.2.22", r.Client())
	if err != nil {
		t.Fatal(err)
	}
	ns2.SetQueryHook(ibHook(&ns2Count))

	z, err := zone.NewWithRecursor("example", r)
	if err != nil {
		t.Fatal(err)
	}

	// Reset counters after zone creation (zoneNSNames queries both servers).
	ns1Count.Store(0)
	ns2Count.Store(0)

	result, err := getIBAddrInZone(ctx, &z)
	if err != nil {
		t.Fatalf("getIBAddrInZone: %v", err)
	}

	got := map[string]string{}
	for _, ns := range result {
		got[ns.Name.String()] = ns.Address.String()
	}
	if got["ns1.example"] != "192.0.2.21" {
		t.Errorf("ns1.example: want 192.0.2.21, got %q", got["ns1.example"])
	}
	if got["ns2.example"] != "192.0.2.22" {
		t.Errorf("ns2.example: want 192.0.2.22, got %q", got["ns2.example"])
	}

	c1 := int(ns1Count.Load())
	c2 := int(ns2Count.Load())
	t.Logf("ns1 queries (getIBAddrInZone phase): %d, ns2 queries: %d", c1, c2)
	if c2 > c1 {
		t.Errorf("ns2 queried more than ns1 (%d > %d); early break may not be working", c2, c1)
	}
}
