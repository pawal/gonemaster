package recursor

import (
	"context"
	"net"
	"net/netip"
	"testing"

	"github.com/miekg/dns"

	"codeberg.org/pawal/gonemaster/engine/dnsname"
	"codeberg.org/pawal/gonemaster/engine/nameserver"
	"codeberg.org/pawal/gonemaster/engine/packet"
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
	found := false
	for _, name := range names {
		if name == "ns2.example.com" {
			found = true
			break
		}
	}
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

	servers, err := r.RootServers()
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
	if _, ok := r.cacheLookup("empty", "A", "IN"); !ok {
		t.Fatalf("expected cached nil response")
	}

	r.ClearCache()
	if _, ok := r.cacheLookup("example", "A", "IN"); ok {
		t.Fatalf("expected cache cleared")
	}
}

func TestGetNSFromUsesGlueAndLazy(t *testing.T) {
	nameserver.EmptyCache()
	defer nameserver.EmptyCache()

	msg := new(dns.Msg)
	msg.Ns = []dns.RR{
		&dns.NS{
			Hdr: dns.RR_Header{
				Name:   "example.",
				Rrtype: dns.TypeNS,
				Class:  dns.ClassINET,
			},
			Ns: "ns2.example.",
		},
		&dns.NS{
			Hdr: dns.RR_Header{
				Name:   "example.",
				Rrtype: dns.TypeNS,
				Class:  dns.ClassINET,
			},
			Ns: "ns1.example.",
		},
	}
	msg.Extra = []dns.RR{
		&dns.A{
			Hdr: dns.RR_Header{
				Name:   "ns1.example.",
				Rrtype: dns.TypeA,
				Class:  dns.ClassINET,
			},
			A: net.IPv4(192, 0, 2, 53),
		},
	}

	resp := packet.Packet{Msg: msg}
	r := &Recursor{client: &transport.Client{}}
	queryers, err := r.getNSFrom(resp, nil)
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

func TestLazyNameserverMissingRecursor(t *testing.T) {
	lns := lazyNameserver{name: "ns1.example"}
	if _, err := lns.QueryWithClass(context.Background(), "example", "A", "IN"); err == nil {
		t.Fatalf("expected error for missing recursor")
	}
}

func TestCollectCNAMEsAndAddresses(t *testing.T) {
	msg := new(dns.Msg)
	msg.Answer = []dns.RR{
		&dns.CNAME{
			Hdr: dns.RR_Header{
				Name:   "www.example.",
				Rrtype: dns.TypeCNAME,
				Class:  dns.ClassINET,
			},
			Target: "alias.example.",
		},
		&dns.A{
			Hdr: dns.RR_Header{
				Name:   "alias.example.",
				Rrtype: dns.TypeA,
				Class:  dns.ClassINET,
			},
			A: net.IPv4(192, 0, 2, 55),
		},
		&dns.AAAA{
			Hdr: dns.RR_Header{
				Name:   "alias.example.",
				Rrtype: dns.TypeAAAA,
				Class:  dns.ClassINET,
			},
			AAAA: net.ParseIP("2001:db8::55"),
		},
	}

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
	msg.Answer = []dns.RR{
		&dns.SOA{
			Hdr: dns.RR_Header{
				Name:   "example.",
				Rrtype: dns.TypeSOA,
				Class:  dns.ClassINET,
			},
		},
	}
	resp := packet.Packet{Msg: msg}
	if owner := firstSOAOwner(resp); owner != "example" {
		t.Fatalf("unexpected owner %q", owner)
	}

	if owner := firstSOAOwner(packet.Packet{Msg: new(dns.Msg)}); owner != "" {
		t.Fatalf("expected empty owner, got %q", owner)
	}
}

func TestRedirectNameNoNS(t *testing.T) {
	resp := packet.Packet{Msg: new(dns.Msg)}
	if _, ok := redirectName(resp); ok {
		t.Fatalf("expected no redirect name")
	}
}
