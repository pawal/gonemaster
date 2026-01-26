package asnlookup

import (
	"context"
	"errors"
	"net/netip"
	"strings"
	"testing"

	"github.com/miekg/dns"

	"github.com/pawal/gonemaster/engine/logger"
	"github.com/pawal/gonemaster/engine/packet"
	"github.com/pawal/gonemaster/engine/profile"
	"github.com/pawal/gonemaster/engine/util"
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
	msg.Rcode = rcode
	msg.Answer = answer
	msg.Ns = authority
	return packet.New(msg)
}

func txtRR(name string, txt string) *dns.TXT {
	return &dns.TXT{
		Hdr: dns.RR_Header{Name: name, Rrtype: dns.TypeTXT, Class: dns.ClassINET, Ttl: 3600},
		Txt: []string{txt},
	}
}

func soaRR(name string, mname string, rname string) *dns.SOA {
	return &dns.SOA{
		Hdr:     dns.RR_Header{Name: name, Rrtype: dns.TypeSOA, Class: dns.ClassINET, Ttl: 3600},
		Ns:      mname,
		Mbox:    rname,
		Serial:  1,
		Refresh: 2,
		Retry:   3,
		Expire:  4,
		Minttl:  5,
	}
}

func TestGetWithPrefixValidationErrors(t *testing.T) {
	ctx := context.Background()
	ip := netip.MustParseAddr("192.0.2.1")

	if _, err := GetWithPrefix(ctx, nil, ip); err == nil || !strings.Contains(err.Error(), "missing resolver") {
		t.Fatalf("expected missing resolver error, got %v", err)
	}

	resolver := fakeResolver{}
	var zero netip.Addr
	if _, err := GetWithPrefix(ctx, resolver, zero); err == nil || !strings.Contains(err.Error(), "invalid IP address") {
		t.Fatalf("expected invalid IP address error, got %v", err)
	}

	profile.ResetEffective()
	t.Cleanup(profile.ResetEffective)

	profile.Effective().ASNDB.Style = ""
	profile.Effective().ASNDB.Sources = map[string][]string{"cymru": {"asnlookup.zonemaster.net"}}
	if _, err := GetWithPrefix(ctx, resolver, ip); err == nil || !strings.Contains(err.Error(), "asn database style undefined") {
		t.Fatalf("expected undefined style error, got %v", err)
	}

	profile.Effective().ASNDB.Style = "cymru"
	profile.Effective().ASNDB.Sources = map[string][]string{}
	if _, err := GetWithPrefix(ctx, resolver, ip); err == nil || !strings.Contains(err.Error(), "asn database sources undefined") {
		t.Fatalf("expected undefined sources error, got %v", err)
	}

	profile.Effective().ASNDB.Style = "bogus"
	profile.Effective().ASNDB.Sources = map[string][]string{"bogus": {"example.com"}}
	if _, err := GetWithPrefix(ctx, resolver, ip); err == nil || !strings.Contains(err.Error(), "asn database style value") {
		t.Fatalf("expected illegal style error, got %v", err)
	}
}

func TestGetWithPrefixTryNextToCodeError(t *testing.T) {
	profile.ResetEffective()
	t.Cleanup(profile.ResetEffective)

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	profile.Effective().ASNDB.Style = "cymru"
	profile.Effective().ASNDB.Sources = map[string][]string{"cymru": {"asnlookup.zonemaster.net"}}

	resolver := fakeResolver{
		handler: func(ctx context.Context, name, qtype, qclass string) (packet.Packet, error) {
			return packet.Packet{}, errors.New("no response")
		},
	}

	ip := netip.MustParseAddr("192.0.2.1")
	result, err := GetWithPrefix(context.Background(), resolver, ip)
	if err != nil {
		t.Fatalf("GetWithPrefix error: %v", err)
	}
	if result.Code != CodeError {
		t.Fatalf("expected %s, got %s", CodeError, result.Code)
	}
}

func TestLookupCymruNXDomainSOAEmpty(t *testing.T) {
	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

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

	result, err := lookupCymru(context.Background(), resolver, ip, source)
	if err != nil {
		t.Fatalf("lookupCymru error: %v", err)
	}
	if result.Code != CodeEmpty {
		t.Fatalf("expected %s, got %s", CodeEmpty, result.Code)
	}
}

func TestLookupCymruSelectsMostSpecificPrefix(t *testing.T) {
	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

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

	result, err := lookupCymru(context.Background(), resolver, ip, source)
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
}

func TestGetReturnsNilOnEmpty(t *testing.T) {
	profile.ResetEffective()
	t.Cleanup(profile.ResetEffective)

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	profile.Effective().ASNDB.Style = "cymru"
	profile.Effective().ASNDB.Sources = map[string][]string{"cymru": {"asnlookup.zonemaster.net"}}

	resolver := fakeResolver{
		handler: func(ctx context.Context, name, qtype, qclass string) (packet.Packet, error) {
			return packetFor(dns.RcodeSuccess, nil, nil), nil
		},
	}

	ip := netip.MustParseAddr("192.0.2.1")
	asns, err := Get(context.Background(), resolver, ip)
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
