package methodsv2

import (
	"context"
	"net"
	"testing"

	"github.com/miekg/dns"

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
		msg.Answer = append(msg.Answer, &dns.NS{
			Hdr: dns.RR_Header{
				Name:   dns.Fqdn(zoneName),
				Rrtype: dns.TypeNS,
				Class:  dns.ClassINET,
				Ttl:    60,
			},
			Ns: dns.Fqdn(nsName),
		})
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
		msg.Answer = []dns.RR{
			&dns.A{
				Hdr: dns.RR_Header{
					Name:   "ns1.example.net.",
					Rrtype: dns.TypeA,
					Class:  dns.ClassINET,
				},
				A: net.IPv4(192, 0, 2, 99),
			},
		}
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
		msg.Answer = []dns.RR{
			&dns.SOA{
				Hdr: dns.RR_Header{
					Name:   dns.Fqdn(name),
					Rrtype: dns.TypeSOA,
					Class:  dns.ClassINET,
				},
				Ns:     "ns.example.",
				Mbox:   "hostmaster.example.",
				Serial: 1,
			},
		}
		return packet.Packet{Msg: msg}
	}

	rootNS := func() packet.Packet {
		msg := new(dns.Msg)
		msg.Rcode = dns.RcodeSuccess
		msg.Authoritative = true
		msg.Answer = []dns.RR{
			&dns.NS{
				Hdr: dns.RR_Header{
					Name:   ".",
					Rrtype: dns.TypeNS,
					Class:  dns.ClassINET,
				},
				Ns: "ns1.root.",
			},
			&dns.NS{
				Hdr: dns.RR_Header{
					Name:   ".",
					Rrtype: dns.TypeNS,
					Class:  dns.ClassINET,
				},
				Ns: "ns2.root.",
			},
		}
		msg.Extra = []dns.RR{
			&dns.A{
				Hdr: dns.RR_Header{
					Name:   "ns1.root.",
					Rrtype: dns.TypeA,
					Class:  dns.ClassINET,
				},
				A: net.IPv4(192, 0, 2, 1),
			},
			&dns.A{
				Hdr: dns.RR_Header{
					Name:   "ns2.root.",
					Rrtype: dns.TypeA,
					Class:  dns.ClassINET,
				},
				A: net.IPv4(192, 0, 2, 2),
			},
		}
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
