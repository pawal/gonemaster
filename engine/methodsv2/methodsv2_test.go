package methodsv2

import (
	"context"
	"net"
	"testing"

	"github.com/miekg/dns"

	"github.com/pawal/gonemaster/engine/nameserver"
	"github.com/pawal/gonemaster/engine/packet"
	"github.com/pawal/gonemaster/engine/profile"
	"github.com/pawal/gonemaster/engine/recursor"
	"github.com/pawal/gonemaster/engine/zone"
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

func newAuthoritativeNameserver(t *testing.T, r *recursor.Recursor, name string, addr string, zoneName string, nsNames ...string) nameserver.Nameserver {
	t.Helper()
	ns, err := nameserver.New(name, addr, r.Client())
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

func TestGetParentNSNamesAndIPsUndelegated(t *testing.T) {
	nameserver.EmptyCache()
	defer nameserver.EmptyCache()
	ClearCache()
	defer ClearCache()

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

	parent, err := GetParentNSNamesAndIPs(context.Background(), &z)
	if err != nil {
		t.Fatalf("get parent: %v", err)
	}
	if parent == nil || len(parent) != 0 {
		t.Fatalf("expected empty parent list, got %#v", parent)
	}
}

func TestGetDelNSNamesAndIPsUndelegated(t *testing.T) {
	nameserver.EmptyCache()
	defer nameserver.EmptyCache()
	ClearCache()
	defer ClearCache()

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

	items, err := GetDelNSNamesAndIPs(context.Background(), &z)
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

func TestGetZoneNSNamesFromAuthoritative(t *testing.T) {
	nameserver.EmptyCache()
	defer nameserver.EmptyCache()
	ClearCache()
	defer ClearCache()
	defer profile.ResetEffective()
	profile.Effective().Net.IPv4 = true
	profile.Effective().Net.IPv6 = true

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

	_ = newAuthoritativeNameserver(t, r, "ns1.example.net", "192.0.2.53", "example", "NS1.EXAMPLE.NET", "ns2.example.net")

	names, err := GetZoneNSNames(context.Background(), &z)
	if err != nil {
		t.Fatalf("get zone NS names: %v", err)
	}
	want := []string{"ns1.example.net", "ns2.example.net"}
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
	nameserver.EmptyCache()
	defer nameserver.EmptyCache()
	ClearCache()
	defer ClearCache()
	defer profile.ResetEffective()
	profile.Effective().Net.IPv4 = true
	profile.Effective().Net.IPv6 = true

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

	_ = newAuthoritativeNameserver(t, r, "ns1.example.net", "192.0.2.53", "example", "ns1.example.net", "ns2.example.net")
	_ = newAuthoritativeNameserver(t, r, "ns2.example.net", "192.0.2.54", "example", "ns1.example.net", "ns2.example.net")

	items, err := GetZoneNSNamesAndIPs(context.Background(), &z)
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
	nameserver.EmptyCache()
	defer nameserver.EmptyCache()
	ClearCache()
	defer ClearCache()
	defer profile.ResetEffective()
	profile.Effective().Net.IPv4 = true
	profile.Effective().Net.IPv6 = true

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

	rootNS, err := nameserver.New("root.test", "192.0.2.9", r.Client())
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

	items, err := GetDelNSNamesAndIPs(context.Background(), &z)
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

func TestGetParentNSNamesAndIPsSkipsOnIntermediateNoResponse(t *testing.T) {
	nameserver.EmptyCache()
	defer nameserver.EmptyCache()
	ClearCache()
	defer ClearCache()
	defer profile.ResetEffective()
	profile.Effective().Net.IPv4 = true
	profile.Effective().Net.IPv6 = true

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

	ns1, err := nameserver.New("ns1.root", "192.0.2.1", r.Client())
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

	ns2, err := nameserver.New("ns2.root", "192.0.2.2", r.Client())
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

	parent, err := GetParentNSNamesAndIPs(context.Background(), &z)
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
