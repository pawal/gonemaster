package connectivity

import (
	"context"
	"net/netip"
	"strings"
	"testing"

	"github.com/miekg/dns"

	"github.com/pawal/gonemaster/engine/asnlookup"
	"github.com/pawal/gonemaster/engine/dnsname"
	"github.com/pawal/gonemaster/engine/logger"
	"github.com/pawal/gonemaster/engine/methodsv2"
	"github.com/pawal/gonemaster/engine/nameserver"
	"github.com/pawal/gonemaster/engine/packet"
	"github.com/pawal/gonemaster/engine/profile"
	"github.com/pawal/gonemaster/engine/util"
	"github.com/pawal/gonemaster/engine/zone"
)

func TestConnectivity01IPv6Disabled(t *testing.T) {
	nameserver.EmptyCache()
	t.Cleanup(nameserver.EmptyCache)
	t.Cleanup(profile.ResetEffective)

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	origMethod := method4and5
	t.Cleanup(func() { method4and5 = origMethod })

	ns4 := newNameserver(t, "ns1.example", "192.0.2.1", nil)
	ns6 := newNameserver(t, "ns2.example", "2001:db8::1", nil)
	method4and5 = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{ns4, ns6}, nil
	}

	profile.Effective().Net.IPv6 = false

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Connectivity01(context.Background(), &z)
	if err != nil {
		t.Fatalf("connectivity01: %v", err)
	}
	if !hasEntryTag(entries, "CN01_IPV6_DISABLED") {
		t.Fatalf("expected CN01_IPV6_DISABLED")
	}
}

func TestConnectivityLoopNoResponse(t *testing.T) {
	nameserver.EmptyCache()
	t.Cleanup(nameserver.EmptyCache)
	t.Cleanup(profile.ResetEffective)

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	ns := newNameserver(t, "ns1.example", "192.0.2.1", nil)
	results := []*logger.Entry{}

	if err := connectivityLoop(context.Background(), "Connectivity01", dnsname.New("example"), []nameserver.Nameserver{ns}, &results); err != nil {
		t.Fatalf("connectivity loop: %v", err)
	}
	if !hasEntryTag(results, "CN01_NO_RESPONSE_UDP") {
		t.Fatalf("expected CN01_NO_RESPONSE_UDP")
	}
}

func TestConnectivityLoopWrongSOAOwner(t *testing.T) {
	nameserver.EmptyCache()
	t.Cleanup(nameserver.EmptyCache)
	t.Cleanup(profile.ResetEffective)

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	ns := newNameserver(t, "ns1.example", "192.0.2.1", func(qname string, qtype string) packet.Packet {
		switch strings.ToUpper(qtype) {
		case "SOA":
			return soaPacket("wrong.example", "ns1.example", "hostmaster.example")
		case "NS":
			return nsPacket("example", "ns1.example")
		default:
			return packet.Packet{}
		}
	})

	results := []*logger.Entry{}
	if err := connectivityLoop(context.Background(), "Connectivity01", dnsname.New("example"), []nameserver.Nameserver{ns}, &results); err != nil {
		t.Fatalf("connectivity loop: %v", err)
	}
	if !hasEntryTag(results, "CN01_WRONG_SOA_RECORD_UDP") {
		t.Fatalf("expected CN01_WRONG_SOA_RECORD_UDP")
	}
}

func TestConnectivity03SameASNSet(t *testing.T) {
	nameserver.EmptyCache()
	t.Cleanup(nameserver.EmptyCache)
	t.Cleanup(profile.ResetEffective)

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	origMethod := method4and5
	origLookup := lookupASN
	t.Cleanup(func() {
		method4and5 = origMethod
		lookupASN = origLookup
	})

	ns1 := newNameserver(t, "ns1.example", "192.0.2.1", nil)
	ns2 := newNameserver(t, "ns2.example", "192.0.2.2", nil)
	method4and5 = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{ns1, ns2}, nil
	}

	prefix, err := netip.ParsePrefix("192.0.2.0/24")
	if err != nil {
		t.Fatalf("parse prefix: %v", err)
	}
	lookupASN = func(_ context.Context, _ asnlookup.Resolver, _ netip.Addr) (asnlookup.Result, error) {
		return asnlookup.Result{
			ASNs:   []int{64500, 64501},
			Prefix: &prefix,
			Raw:    "64500 64501 | 192.0.2.0/24 | test",
			Code:   asnlookup.CodeFound,
		}, nil
	}

	z, err := zone.New("example")
	if err != nil {
		t.Fatalf("new zone: %v", err)
	}
	entries, err := Connectivity03(context.Background(), &z)
	if err != nil {
		t.Fatalf("connectivity03: %v", err)
	}
	if !hasEntryTag(entries, "IPV4_SAME_ASN") {
		t.Fatalf("expected IPV4_SAME_ASN")
	}
}

func TestConnectivity04SinglePrefix(t *testing.T) {
	nameserver.EmptyCache()
	t.Cleanup(nameserver.EmptyCache)
	t.Cleanup(profile.ResetEffective)

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	origDel := getDelNSNamesAndIPs
	origZone := getZoneNSNamesAndIPs
	origLookup := lookupASN
	t.Cleanup(func() {
		getDelNSNamesAndIPs = origDel
		getZoneNSNamesAndIPs = origZone
		lookupASN = origLookup
	})

	items := []methodsv2.NSItem{
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
	getDelNSNamesAndIPs = func(_ context.Context, _ *zone.Zone) ([]methodsv2.NSItem, error) {
		return items, nil
	}
	getZoneNSNamesAndIPs = func(_ context.Context, _ *zone.Zone) ([]methodsv2.NSItem, error) {
		return []methodsv2.NSItem{}, nil
	}

	prefix, err := netip.ParsePrefix("192.0.2.0/24")
	if err != nil {
		t.Fatalf("parse prefix: %v", err)
	}
	lookupASN = func(_ context.Context, _ asnlookup.Resolver, _ netip.Addr) (asnlookup.Result, error) {
		return asnlookup.Result{
			Prefix: &prefix,
			Raw:    "64500 | 192.0.2.0/24 | test",
			Code:   asnlookup.CodeFound,
		}, nil
	}

	z, err := zone.New("example")
	if err != nil {
		t.Fatalf("new zone: %v", err)
	}
	entries, err := Connectivity04(context.Background(), &z)
	if err != nil {
		t.Fatalf("connectivity04: %v", err)
	}
	if !hasEntryTag(entries, "CN04_IPV4_SAME_PREFIX") {
		t.Fatalf("expected CN04_IPV4_SAME_PREFIX")
	}
	if !hasEntryTag(entries, "CN04_IPV4_SINGLE_PREFIX") {
		t.Fatalf("expected CN04_IPV4_SINGLE_PREFIX")
	}
}

func newNameserver(t *testing.T, name string, ip string, handler func(qname string, qtype string) packet.Packet) nameserver.Nameserver {
	t.Helper()

	ns, err := nameserver.New(name, ip, nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}
	if handler == nil {
		handler = func(_ string, _ string) packet.Packet {
			return packet.Packet{}
		}
	}
	ns.SetQueryHook(func(_ context.Context, qname string, qtype string, _ string, _ *nameserver.QueryOptions) (packet.Packet, error) {
		return handler(qname, qtype), nil
	})
	return ns
}

func hasEntryTag(entries []*logger.Entry, tag string) bool {
	for _, entry := range entries {
		if entry == nil {
			continue
		}
		if entry.Tag == tag {
			return true
		}
	}
	return false
}

func soaPacket(owner string, mname string, rname string) packet.Packet {
	msg := new(dns.Msg)
	msg.Rcode = dns.RcodeSuccess
	msg.Authoritative = true
	msg.Answer = []dns.RR{
		&dns.SOA{
			Hdr: dns.RR_Header{
				Name:   dns.Fqdn(owner),
				Rrtype: dns.TypeSOA,
				Class:  dns.ClassINET,
				Ttl:    60,
			},
			Ns:      dns.Fqdn(mname),
			Mbox:    dns.Fqdn(rname),
			Serial:  2024010101,
			Refresh: 3600,
			Retry:   600,
			Expire:  86400,
			Minttl:  60,
		},
	}
	return packet.Packet{Msg: msg}
}

func nsPacket(owner string, nsname string) packet.Packet {
	msg := new(dns.Msg)
	msg.Rcode = dns.RcodeSuccess
	msg.Authoritative = true
	msg.Answer = []dns.RR{
		&dns.NS{
			Hdr: dns.RR_Header{
				Name:   dns.Fqdn(owner),
				Rrtype: dns.TypeNS,
				Class:  dns.ClassINET,
				Ttl:    60,
			},
			Ns: dns.Fqdn(nsname),
		},
	}
	return packet.Packet{Msg: msg}
}
