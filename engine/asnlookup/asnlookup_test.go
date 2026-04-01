package asnlookup

import (
	"context"
	"errors"
	"net/netip"
	"strings"
	"testing"

	dns "codeberg.org/miekg/dns"

	"codeberg.org/pawal/gonemaster/engine/internal/testhelpers"
	"codeberg.org/pawal/gonemaster/engine/packet"
)

type fakeResolver struct {
	handler func(ctx context.Context, name string, qtype string, qclass string) (packet.Packet, error)
}

func (f fakeResolver) Recurse(ctx context.Context, name string, qtype string, qclass string) (packet.Packet, error) {
	if f.handler == nil {
		return packet.Packet{}, nil
	}
	return f.handler(ctx, name, qtype, qclass)
}

func packetFor(rcode int, answer []dns.RR, authority []dns.RR) packet.Packet {
	msg := new(dns.Msg)
	msg.Rcode = uint16(rcode)
	msg.Answer = answer
	msg.Ns = authority
	return packet.New(msg)
}

func txtRR(name string, txt string) *dns.TXT {
	rr := &dns.TXT{Hdr: dns.Header{Name: name, Class: dns.ClassINET, TTL: 3600}}
	rr.Txt = []string{txt}
	return rr
}

func soaRR(name string, mname string, rname string) *dns.SOA {
	rr := &dns.SOA{Hdr: dns.Header{Name: name, Class: dns.ClassINET, TTL: 3600}}
	rr.Ns = mname
	rr.Mbox = rname
	rr.Serial = 1
	rr.Refresh = 2
	rr.Retry = 3
	rr.Expire = 4
	rr.Minttl = 5
	return rr
}

func TestGetWithPrefixValidationErrors(t *testing.T) {
	ctx, prof, _ := testhelpers.Context(t)
	ip := netip.MustParseAddr("192.0.2.1")

	if _, err := GetWithPrefix(ctx, nil, ip); err == nil || !strings.Contains(err.Error(), "missing resolver") {
		t.Fatalf("expected missing resolver error, got %v", err)
	}

	resolver := fakeResolver{}
	var zero netip.Addr
	if _, err := GetWithPrefix(ctx, resolver, zero); err == nil || !strings.Contains(err.Error(), "invalid IP address") {
		t.Fatalf("expected invalid IP address error, got %v", err)
	}

	prof.ASNDB.Style = ""
	prof.ASNDB.Sources = map[string][]string{"cymru": {"asnlookup.zonemaster.net"}}
	if _, err := GetWithPrefix(ctx, resolver, ip); err == nil || !strings.Contains(err.Error(), "asn database style undefined") {
		t.Fatalf("expected undefined style error, got %v", err)
	}

	prof.ASNDB.Style = "cymru"
	prof.ASNDB.Sources = map[string][]string{}
	if _, err := GetWithPrefix(ctx, resolver, ip); err == nil || !strings.Contains(err.Error(), "asn database sources undefined") {
		t.Fatalf("expected undefined sources error, got %v", err)
	}

	prof.ASNDB.Style = "bogus"
	prof.ASNDB.Sources = map[string][]string{"bogus": {"example.com"}}
	if _, err := GetWithPrefix(ctx, resolver, ip); err == nil || !strings.Contains(err.Error(), "asn database style value") {
		t.Fatalf("expected illegal style error, got %v", err)
	}
}

func TestGetWithPrefixTryNextToCodeError(t *testing.T) {
	ctx, prof, _ := testhelpers.Context(t)
	prof.ASNDB.Style = "cymru"
	prof.ASNDB.Sources = map[string][]string{"cymru": {"asnlookup.zonemaster.net"}}

	resolver := fakeResolver{
		handler: func(ctx context.Context, name, qtype, qclass string) (packet.Packet, error) {
			return packet.Packet{}, errors.New("no response")
		},
	}

	ip := netip.MustParseAddr("192.0.2.1")
	result, err := GetWithPrefix(ctx, resolver, ip)
	if err != nil {
		t.Fatalf("GetWithPrefix error: %v", err)
	}
	if result.Code != CodeError {
		t.Fatalf("expected %s, got %s", CodeError, result.Code)
	}
}

func TestLookupCymruNXDomainSOAEmpty(t *testing.T) {
	ctx, _, _ := testhelpers.Context(t)

	ip := netip.MustParseAddr("192.0.2.1")
	source := "asnlookup.zonemaster.net"
	resp := packetFor(dns.RcodeNameError, nil, []dns.RR{
		soaRR(source+".", "ns."+source+".", "hostmaster."+source+"."),
	})

	resolver := fakeResolver{
		handler: func(ctx context.Context, name, qtype, qclass string) (packet.Packet, error) {
			return resp, nil
		},
	}

	result, err := lookupCymru(ctx, resolver, ip, source)
	if err != nil {
		t.Fatalf("lookupCymru error: %v", err)
	}
	if result.Code != CodeEmpty {
		t.Fatalf("expected %s, got %s", CodeEmpty, result.Code)
	}
}

func TestLookupCymruSelectsMostSpecificPrefix(t *testing.T) {
	ctx, _, log := testhelpers.Context(t)

	ip := netip.MustParseAddr("192.0.2.1")
	source := "asnlookup.zonemaster.net"
	answers := []dns.RR{
		txtRR("txt."+source+".", "64500 | 192.0.0.0/16 | NA | NA | NA"),
		txtRR("txt."+source+".", "64500 64501 | 192.0.2.0/24 | NA | NA | NA"),
	}
	resp := packetFor(dns.RcodeSuccess, answers, nil)

	resolver := fakeResolver{
		handler: func(ctx context.Context, name, qtype, qclass string) (packet.Packet, error) {
			return resp, nil
		},
	}

	result, err := lookupCymru(ctx, resolver, ip, source)
	if err != nil {
		t.Fatalf("lookupCymru error: %v", err)
	}
	if result.Code != CodeFound {
		t.Fatalf("expected %s, got %s", CodeFound, result.Code)
	}
	if result.Prefix == nil || result.Prefix.Bits() != 24 {
		t.Fatalf("expected /24 prefix, got %v", result.Prefix)
	}
	if len(result.ASNs) != 2 || result.ASNs[0] != 64500 || result.ASNs[1] != 64501 {
		t.Fatalf("unexpected ASN list: %v", result.ASNs)
	}
	var found bool
	for _, entry := range log.Entries() {
		if entry == nil || entry.Tag != "ASN_LOOKUP_SOURCE" {
			continue
		}
		found = true
		if sourceArg, ok := entry.Args["source"].(string); !ok || sourceArg != source {
			t.Fatalf("expected source=%q in ASN_LOOKUP_SOURCE args, got %#v", source, entry.Args)
		}
		if _, ok := entry.Args["name"]; ok {
			t.Fatalf("legacy key name should not be present: %#v", entry.Args)
		}
	}
	if !found {
		t.Fatalf("expected ASN_LOOKUP_SOURCE log entry")
	}
}

func TestGetReturnsNilOnEmpty(t *testing.T) {
	ctx, prof, _ := testhelpers.Context(t)
	prof.ASNDB.Style = "cymru"
	prof.ASNDB.Sources = map[string][]string{"cymru": {"asnlookup.zonemaster.net"}}

	resolver := fakeResolver{
		handler: func(ctx context.Context, name, qtype, qclass string) (packet.Packet, error) {
			return packetFor(dns.RcodeSuccess, nil, nil), nil
		},
	}

	ip := netip.MustParseAddr("192.0.2.1")
	asns, err := Get(ctx, resolver, ip)
	if err != nil {
		t.Fatalf("Get error: %v", err)
	}
	if asns != nil {
		t.Fatalf("expected nil ASN list, got %v", asns)
	}
}

func TestParseASNList(t *testing.T) {
	asns, err := parseASNList("64500 64501")
	if err != nil {
		t.Fatalf("parseASNList error: %v", err)
	}
	if len(asns) != 2 || asns[0] != 64500 || asns[1] != 64501 {
		t.Fatalf("unexpected ASN list: %v", asns)
	}

	if _, err := parseASNList("64500 foo"); err == nil || !strings.Contains(err.Error(), "isn't numeric") {
		t.Fatalf("expected numeric error, got %v", err)
	}
}
