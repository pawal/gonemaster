// Coverage target for engine/methodsv2: aim for >=90% line coverage
// (currently ~69%). The remaining gap is in CNAME-following code paths
// (cnameTargetFromQuestion, followCNAME, collectResolvedAddrs) which need
// dedicated CNAME fixtures. Future contributors should add CNAME-path
// tests if those flows are exercised by new testcases.

package methodsv2

import (
	"context"
	"fmt"
	"net/netip"
	"sort"
	"sync"
	"sync/atomic"
	"testing"

	dns "codeberg.org/miekg/dns"
	"codeberg.org/miekg/dns/dnsutil"

	"codeberg.org/pawal/gonemaster/engine/dnsname"
	"codeberg.org/pawal/gonemaster/engine/internal/testhelpers"
	"codeberg.org/pawal/gonemaster/engine/nameserver"
	"codeberg.org/pawal/gonemaster/engine/packet"
	"codeberg.org/pawal/gonemaster/engine/recursor"
	"codeberg.org/pawal/gonemaster/engine/zone"
)

func authoritativeNSPacket(zoneName string, nsNames ...string) packet.Packet {
	msg := new(dns.Msg)
	msg.Rcode = dns.RcodeSuccess
	msg.Authoritative = true
	for _, nsName := range nsNames {
		nsRR := &dns.NS{}
		nsRR.Hdr = dns.Header{Name: dnsutil.Fqdn(zoneName), Class: dns.ClassINET, TTL: 60}
		nsRR.Ns = dnsutil.Fqdn(nsName)
		msg.Answer = append(msg.Answer, nsRR)
	}
	return packet.Packet{Msg: msg}
}

func newAuthoritativeNameserver(ctx context.Context, t *testing.T, r *recursor.Recursor, name string, addr string, zoneName string, nsNames ...string) nameserver.Nameserver {
	t.Helper()
	ns, err := nameserver.NewWithContext(ctx, name, addr, r.Client())
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}
	ns.SetQueryHook(func(_ context.Context, qname string, qtype string, _ string, _ *nameserver.QueryOptions) (packet.Packet, error) {
		if qname != zoneName || qtype != "NS" {
			return packet.Packet{}, nil
		}
		return authoritativeNSPacket(zoneName, nsNames...), nil
	})
	return ns
}

func TestParentCacheStoresSnapshotData(t *testing.T) {
	ClearCache()
	defer ClearCache()

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

func TestGetParentNSNamesAndIPsUndelegated(t *testing.T) {
	ClearCache()
	defer ClearCache()
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

	parent, err := GetParentNSNamesAndIPs(ctx, &z)
	if err != nil {
		t.Fatalf("get parent: %v", err)
	}
	if parent == nil || len(parent) != 0 {
		t.Fatalf("expected empty parent list, got %#v", parent)
	}
}

func TestGetDelNSNamesAndIPsUndelegated(t *testing.T) {
	ClearCache()
	defer ClearCache()
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

	items, err := GetDelNSNamesAndIPs(ctx, &z)
	if err != nil {
		t.Fatalf("get delegation: %v", err)
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

func TestGetZoneNSNamesUndelegatedIgnoresAuthoritativeApexSet(t *testing.T) {
	ClearCache()
	defer ClearCache()
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

	names, err := GetZoneNSNames(ctx, &z)
	if err != nil {
		t.Fatalf("get zone NS names: %v", err)
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

func TestGetZoneNSNamesAndIPsOutOfBailiwick(t *testing.T) {
	ClearCache()
	defer ClearCache()
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

	items, err := GetZoneNSNamesAndIPs(ctx, &z)
	if err != nil {
		t.Fatalf("get zone NS names and IPs: %v", err)
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

func TestGetDelNSNamesAndIPsUndelegatedLookupWhenNoIP(t *testing.T) {
	ClearCache()
	defer ClearCache()
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

	items, err := GetDelNSNamesAndIPs(ctx, &z)
	if err != nil {
		t.Fatalf("get delegation: %v", err)
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

func TestGetDelNSNamesAndIPsUndelegatedKeepsInBailiwickNameWithoutIP(t *testing.T) {
	ClearCache()
	defer ClearCache()
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

	items, err := GetDelNSNamesAndIPs(ctx, &z)
	if err != nil {
		t.Fatalf("get delegation: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if items[0].Name.String() != "ns1.example" || items[0].HasAddress {
		t.Fatalf("unexpected item: %#v", items[0])
	}
}

func TestGetZoneNSNamesUndelegatedUsesDelegationNames(t *testing.T) {
	ClearCache()
	defer ClearCache()
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

	names, err := GetZoneNSNames(ctx, &z)
	if err != nil {
		t.Fatalf("get zone ns names: %v", err)
	}
	if len(names) != 2 {
		t.Fatalf("expected 2 names, got %d", len(names))
	}
	if names[0].String() != "ns1.example" || names[1].String() != "ns2.example.net" {
		t.Fatalf("unexpected names: %q, %q", names[0].String(), names[1].String())
	}
}

func TestGetZoneNSNamesAndIPsUndelegatedInBailiwickUsesProvidedGlue(t *testing.T) {
	ClearCache()
	defer ClearCache()
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

	items, err := GetZoneNSNamesAndIPs(ctx, &z)
	if err != nil {
		t.Fatalf("get zone ns names and ips: %v", err)
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

func TestGetParentNSNamesAndIPsSkipsOnIntermediateNoResponse(t *testing.T) {
	ClearCache()
	defer ClearCache()
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

	parent, err := GetParentNSNamesAndIPs(ctx, &z)
	if err != nil {
		t.Fatalf("get parent: %v", err)
	}
	if len(parent) != 1 {
		t.Fatalf("expected 1 parent nameserver, got %d", len(parent))
	}
	if parent[0].String() != "ns2.root/192.0.2.2" {
		t.Fatalf("unexpected parent nameserver %q", parent[0].String())
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

// ibTestRootHook returns a query hook for a root server that delegates zoneName
// to the specified nameservers with glue.
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

// TestGetIBAddrInZoneSkipsDeadDelegationServer exercises the non-undelegated
// in-bailiwick resolution path where one delegation server is unreachable.
// Zone "example" is delegated from root to three in-bailiwick servers, one
// dead. The dead server should be tried at most once then skipped.
func TestGetIBAddrInZoneSkipsDeadDelegationServer(t *testing.T) {
	ClearCache()
	defer ClearCache()
	ctx, prof, _ := testhelpers.Context(t)
	prof.Net.IPv4 = true
	prof.Net.IPv6 = false

	r := &recursor.Recursor{}
	if err := r.AddFakeAddresses(".", map[string][]string{
		"ns.root": {"192.0.2.9"},
	}); err != nil {
		t.Fatal(err)
	}

	// Root delegates "example" to three in-bailiwick servers with glue.
	rootNS, err := nameserver.NewWithContext(ctx, "ns.root", "192.0.2.9", r.Client())
	if err != nil {
		t.Fatal(err)
	}
	rootNS.SetQueryHook(ibTestRootHook("example", map[string]string{
		"ns1.example":  "192.0.2.11",
		"ns2.example":  "192.0.2.12",
		"dead.example": "192.0.2.99",
	}))

	// Healthy delegation server - responds authoritatively for example.
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

	// Dead delegation server - returns error for all queries.
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

	items, err := GetZoneNSNamesAndIPs(ctx, &z)
	if err != nil {
		t.Fatalf("GetZoneNSNamesAndIPs: %v", err)
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

	// The dead server should be tried at most a few times total. Without
	// dead-server tracking it would be queried once per IB name × qtype
	// (3 × 1 = 3 from getIBAddrInZone alone, plus GetZoneNSNames queries).
	// With the fix, expect ≤ 5 total across both phases.
	dq := int(deadQueryCount.Load())
	if dq > 5 {
		t.Errorf("dead server queried %d times (expected ≤ 5 with dead-server skip)", dq)
	}
}

// TestGetIBAddrInZoneBreaksEarlyOnSuccess verifies that once a delegation
// server provides addresses for an in-bailiwick NS name, remaining servers
// are not tried for that name.
func TestGetIBAddrInZoneBreaksEarlyOnSuccess(t *testing.T) {
	ClearCache()
	defer ClearCache()
	ctx, prof, _ := testhelpers.Context(t)
	prof.Net.IPv4 = true
	prof.Net.IPv6 = false

	r := &recursor.Recursor{}
	if err := r.AddFakeAddresses(".", map[string][]string{
		"ns.root": {"192.0.2.9"},
	}); err != nil {
		t.Fatal(err)
	}

	// Root delegates "example" to two in-bailiwick servers with glue.
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

	// Reset counters after zone creation (GetZoneNSNames queries both servers).
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

// seedParentCache stores a sentinel entry for the given zone key using
// cacheParent. Returns a freshly built nameserver matching the entry so
// tests can compare materialized output.
func seedParentCache(ctx context.Context, t *testing.T, r *recursor.Recursor, zoneKey string, nsName string, nsAddr string) nameserver.Nameserver {
	t.Helper()
	ns, err := nameserver.NewWithContext(ctx, nsName, nsAddr, r.Client())
	if err != nil {
		t.Fatalf("seed nameserver: %v", err)
	}
	cacheParent(zoneKey, []nameserver.Nameserver{ns}, true)
	return ns
}

// TestGetParentNSNamesAndIPsUsesCacheOnSecondCall verifies that a pre-seeded
// cache entry is returned without traversing the delegation chain. The test
// uses a recursor with no fake addresses and no hooked servers, so any
// cache miss would either fail or return a different (empty) result.
func TestGetParentNSNamesAndIPsUsesCacheOnSecondCall(t *testing.T) {
	ClearCache()
	defer ClearCache()
	ctx, _, _ := testhelpers.Context(t)

	r := &recursor.Recursor{}
	// Add root hints so r.Recursor isn't unusable, but example.com is not
	// undelegated (no fake addresses for it).
	if err := r.AddFakeAddresses(".", map[string][]string{
		"a.root": {"192.0.2.1"},
	}); err != nil {
		t.Fatalf("add root: %v", err)
	}

	z, err := zone.NewWithRecursor("example.com", r)
	if err != nil {
		t.Fatalf("new zone: %v", err)
	}

	// Seed cache with a sentinel entry under the same key
	// GetParentNSNamesAndIPs would derive from z.Name.
	key := z.Name.String()
	seedParentCache(ctx, t, r, key, "sentinel.ns.example", "203.0.113.99")

	out, err := GetParentNSNamesAndIPs(ctx, &z)
	if err != nil {
		t.Fatalf("get parent: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("expected 1 cached server materialized, got %d: %#v", len(out), out)
	}
	if out[0].Name.String() != "sentinel.ns.example" || out[0].Address.String() != "203.0.113.99" {
		t.Fatalf("expected sentinel.ns.example/203.0.113.99, got %s/%s",
			out[0].Name.String(), out[0].Address.String())
	}
}

// TestClearCacheRemovesAllEntries verifies that ClearCache empties the
// global parent cache map, so subsequent calls miss and re-walk the chain.
func TestClearCacheRemovesAllEntries(t *testing.T) {
	ClearCache()
	defer ClearCache()
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

	ClearCache()

	parentCache.mu.Lock()
	after := len(parentCache.items)
	parentCache.mu.Unlock()
	if after != 0 {
		t.Fatalf("expected empty cache after ClearCache, got %d entries", after)
	}
}

// TestGetParentNSNamesAndIPsCacheIsolatedPerZone verifies that cache entries
// for two distinct zones do not bleed into each other.
func TestGetParentNSNamesAndIPsCacheIsolatedPerZone(t *testing.T) {
	ClearCache()
	defer ClearCache()
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

	outA, err := GetParentNSNamesAndIPs(ctx, &zA)
	if err != nil {
		t.Fatalf("get parent alpha: %v", err)
	}
	outB, err := GetParentNSNamesAndIPs(ctx, &zB)
	if err != nil {
		t.Fatalf("get parent beta: %v", err)
	}

	if len(outA) != 1 || outA[0].Name.String() != "ns.alpha" {
		t.Fatalf("alpha: expected ns.alpha, got %#v", outA)
	}
	if len(outB) != 1 || outB[0].Name.String() != "ns.beta" {
		t.Fatalf("beta: expected ns.beta, got %#v", outB)
	}
}

// TestGetParentNSNamesAndIPsCacheSurvivesAcrossContexts verifies that the
// cache lookup is keyed by zone name only, not by context. Two distinct
// contexts querying the same zone must both hit the cache.
func TestGetParentNSNamesAndIPsCacheSurvivesAcrossContexts(t *testing.T) {
	ClearCache()
	defer ClearCache()
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

	out1, err := GetParentNSNamesAndIPs(ctx1, &z)
	if err != nil {
		t.Fatalf("ctx1: %v", err)
	}
	out2, err := GetParentNSNamesAndIPs(ctx2, &z)
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

// TestGetParentNSNamesAndIPsConcurrentCallsSameZone launches 50 goroutines
// calling GetParentNSNamesAndIPs on the same zone with a pre-seeded cache.
// Run under -race to detect missing mutex protection on parentCache.
func TestGetParentNSNamesAndIPsConcurrentCallsSameZone(t *testing.T) {
	ClearCache()
	defer ClearCache()
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
			out, err := GetParentNSNamesAndIPs(ctx, &z)
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

// TestGetParentNSNamesAndIPsConcurrentCallsDifferentZones launches 50
// goroutines spread across 10 distinct zones, each with its own pre-seeded
// cache entry. Verifies the cache map's per-key isolation under contention.
func TestGetParentNSNamesAndIPsConcurrentCallsDifferentZones(t *testing.T) {
	ClearCache()
	defer ClearCache()
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

	const PerZone = 5 // 50 total goroutines
	var wg sync.WaitGroup
	wg.Add(NZones * PerZone)
	for i := 0; i < NZones; i++ {
		for j := 0; j < PerZone; j++ {
			i := i
			go func() {
				defer wg.Done()
				out, err := GetParentNSNamesAndIPs(ctx, &zones[i])
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

// TestClearCacheConcurrentWithGetParent runs ClearCache repeatedly in one
// goroutine while many readers call GetParentNSNamesAndIPs. The mutex must
// serialize them; the test passes as long as no race is reported and no
// goroutine panics. Reader results may be either cached or empty depending
// on timing; both are acceptable.
func TestClearCacheConcurrentWithGetParent(t *testing.T) {
	ClearCache()
	defer ClearCache()
	ctx, _, _ := testhelpers.Context(t)

	r := &recursor.Recursor{}
	if err := r.AddFakeAddresses(".", map[string][]string{"a.root": {"192.0.2.1"}}); err != nil {
		t.Fatalf("add root: %v", err)
	}

	z, err := zone.NewWithRecursor("example.com", r)
	if err != nil {
		t.Fatalf("new zone: %v", err)
	}

	// Initial seed so readers have something to find on cache hits.
	seedParentCache(ctx, t, r, z.Name.String(), "ns.example", "203.0.113.50")

	const Iterations = 200
	var wg sync.WaitGroup

	// Clearer goroutine: clears cache repeatedly, re-seeding between rounds
	// so readers can hit the cache during some iterations.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < Iterations; i++ {
			ClearCache()
			seedParentCache(ctx, t, r, z.Name.String(), "ns.example", "203.0.113.50")
		}
	}()

	// Reader goroutines.
	const NReaders = 20
	wg.Add(NReaders)
	for i := 0; i < NReaders; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < Iterations; j++ {
				// Result may be empty (cache cleared, no real chain to walk
				// since recursor has no servers for example.com) or contain
				// the seeded sentinel. Both are acceptable; we just must
				// not race or panic.
				_, _ = GetParentNSNamesAndIPs(ctx, &z)
			}
		}()
	}

	wg.Wait()
}

// TestNSItemStringStableForSort verifies that NSItem.String() is a
// deterministic, total order key suitable for sort.SliceStable: items with
// the same String() produce equal output across repeated shuffled sorts,
// and items with different String() sort in lexicographic order.
func TestNSItemStringStableForSort(t *testing.T) {
	items := []NSItem{
		{Name: dnsname.New("b.example"), Address: netip.MustParseAddr("192.0.2.2"), HasAddress: true},
		{Name: dnsname.New("a.example"), Address: netip.MustParseAddr("192.0.2.1"), HasAddress: true},
		{Name: dnsname.New("c.example"), HasAddress: false},
		{Name: dnsname.New("a.example"), Address: netip.MustParseAddr("192.0.2.10"), HasAddress: true},
	}

	sortByString := func(in []NSItem) []NSItem {
		out := append([]NSItem(nil), in...)
		sort.SliceStable(out, func(i, j int) bool { return out[i].String() < out[j].String() })
		return out
	}

	first := sortByString(items)
	// Shuffle and re-sort multiple times; result must be identical.
	for trial := 0; trial < 20; trial++ {
		shuffled := append([]NSItem(nil), items...)
		// Rotate by trial to vary order.
		shuffled = append(shuffled[trial%len(shuffled):], shuffled[:trial%len(shuffled)]...)
		got := sortByString(shuffled)
		if len(got) != len(first) {
			t.Fatalf("trial %d: length mismatch", trial)
		}
		for i := range got {
			if got[i].String() != first[i].String() {
				t.Fatalf("trial %d index %d: expected %q, got %q",
					trial, i, first[i].String(), got[i].String())
			}
		}
	}
}
