package syntax

import (
	"context"
	"strings"
	"testing"

	"github.com/miekg/dns"

	"codeberg.org/pawal/gonemaster/engine/dnsname"
	"codeberg.org/pawal/gonemaster/engine/logger"
	"codeberg.org/pawal/gonemaster/engine/nameserver"
	"codeberg.org/pawal/gonemaster/engine/packet"
	"codeberg.org/pawal/gonemaster/engine/profile"
	"codeberg.org/pawal/gonemaster/engine/recursor"
	"codeberg.org/pawal/gonemaster/engine/util"
	"codeberg.org/pawal/gonemaster/engine/zone"
)

func TestSyntax01AllowedChars(t *testing.T) {
	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Syntax01(context.Background(), &z)
	if err != nil {
		t.Fatalf("syntax01: %v", err)
	}
	if !hasEntryTag(entries, "ONLY_ALLOWED_CHARS") {
		t.Fatalf("expected ONLY_ALLOWED_CHARS")
	}
}

func TestSyntax01NonAllowedChars(t *testing.T) {
	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	z := zone.Zone{Name: dnsname.New("bad_.example")}
	entries, err := Syntax01(context.Background(), &z)
	if err != nil {
		t.Fatalf("syntax01: %v", err)
	}
	if !hasEntryTag(entries, "NON_ALLOWED_CHARS") {
		t.Fatalf("expected NON_ALLOWED_CHARS")
	}
}

func TestSyntax02HyphenTags(t *testing.T) {
	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	z := zone.Zone{Name: dnsname.New("-bad-.example")}
	entries, err := Syntax02(context.Background(), &z)
	if err != nil {
		t.Fatalf("syntax02: %v", err)
	}
	if !hasEntryTag(entries, "INITIAL_HYPHEN") {
		t.Fatalf("expected INITIAL_HYPHEN")
	}
	if !hasEntryTag(entries, "TERMINAL_HYPHEN") {
		t.Fatalf("expected TERMINAL_HYPHEN")
	}
}

func TestSyntax02NoEndingHyphens(t *testing.T) {
	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	z := zone.Zone{Name: dnsname.New("good.example")}
	entries, err := Syntax02(context.Background(), &z)
	if err != nil {
		t.Fatalf("syntax02: %v", err)
	}
	if !hasEntryTag(entries, "NO_ENDING_HYPHENS") {
		t.Fatalf("expected NO_ENDING_HYPHENS")
	}
}

func TestSyntax03DoubleDash(t *testing.T) {
	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	z := zone.Zone{Name: dnsname.New("ab--cd.example")}
	entries, err := Syntax03(context.Background(), &z)
	if err != nil {
		t.Fatalf("syntax03: %v", err)
	}
	if !hasEntryTag(entries, "DISCOURAGED_DOUBLE_DASH") {
		t.Fatalf("expected DISCOURAGED_DOUBLE_DASH")
	}
}

func TestSyntax03NoDoubleDash(t *testing.T) {
	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	z := zone.Zone{Name: dnsname.New("good.example")}
	entries, err := Syntax03(context.Background(), &z)
	if err != nil {
		t.Fatalf("syntax03: %v", err)
	}
	if !hasEntryTag(entries, "NO_DOUBLE_DASH") {
		t.Fatalf("expected NO_DOUBLE_DASH")
	}
}

func TestSyntax04NameserverSyntaxOK(t *testing.T) {
	z := newRootZoneWithHook(t, func(qname string, qtype string) packet.Packet {
		if strings.EqualFold(qname, ".") && strings.EqualFold(qtype, "NS") {
			return nsPacket(".", "ns1.example.")
		}
		return packet.Packet{}
	})

	entries, err := Syntax04(context.Background(), z)
	if err != nil {
		t.Fatalf("syntax04: %v", err)
	}
	if !hasEntryTag(entries, "NAMESERVER_SYNTAX_OK") {
		t.Fatalf("expected NAMESERVER_SYNTAX_OK")
	}
}

func TestSyntax05MisusedAtSign(t *testing.T) {
	z := newRootZoneWithHook(t, func(qname string, qtype string) packet.Packet {
		if strings.EqualFold(qname, ".") && strings.EqualFold(qtype, "SOA") {
			return soaPacket(".", "a.root.", "user@example.")
		}
		return packet.Packet{}
	})

	entries, err := Syntax05(context.Background(), z)
	if err != nil {
		t.Fatalf("syntax05: %v", err)
	}
	if !hasEntryTag(entries, "RNAME_MISUSED_AT_SIGN") {
		t.Fatalf("expected RNAME_MISUSED_AT_SIGN")
	}
}

func TestSyntax05NoResponseSOAQuery(t *testing.T) {
	z := newRootZoneWithHook(t, func(_ string, _ string) packet.Packet {
		return packet.Packet{}
	})

	entries, err := Syntax05(context.Background(), z)
	if err != nil {
		t.Fatalf("syntax05: %v", err)
	}
	if !hasEntryTag(entries, "NO_RESPONSE_SOA_QUERY") {
		t.Fatalf("expected NO_RESPONSE_SOA_QUERY")
	}
}

func TestSyntax07MNameSyntaxOK(t *testing.T) {
	z := newRootZoneWithHook(t, func(qname string, qtype string) packet.Packet {
		if strings.EqualFold(qname, ".") && strings.EqualFold(qtype, "SOA") {
			return soaPacket(".", "ns1.example.", "hostmaster.example.")
		}
		return packet.Packet{}
	})

	entries, err := Syntax07(context.Background(), z)
	if err != nil {
		t.Fatalf("syntax07: %v", err)
	}
	if !hasEntryTag(entries, "MNAME_SYNTAX_OK") {
		t.Fatalf("expected MNAME_SYNTAX_OK")
	}
}

func TestSyntax08MxSyntaxOK(t *testing.T) {
	z := newRootZoneWithHook(t, func(qname string, qtype string) packet.Packet {
		if strings.EqualFold(qname, ".") && strings.EqualFold(qtype, "MX") {
			return mxPacket(".", "mail.example.")
		}
		return packet.Packet{}
	})

	entries, err := Syntax08(context.Background(), z)
	if err != nil {
		t.Fatalf("syntax08: %v", err)
	}
	if !hasEntryTag(entries, "MX_SYNTAX_OK") {
		t.Fatalf("expected MX_SYNTAX_OK")
	}
}

func TestSyntax08NoResponseMXQuery(t *testing.T) {
	z := newRootZoneWithHook(t, func(_ string, _ string) packet.Packet {
		return packet.Packet{}
	})

	entries, err := Syntax08(context.Background(), z)
	if err != nil {
		t.Fatalf("syntax08: %v", err)
	}
	if !hasEntryTag(entries, "NO_RESPONSE_MX_QUERY") {
		t.Fatalf("expected NO_RESPONSE_MX_QUERY")
	}
}

func TestCheckNameSyntaxNumericTLD(t *testing.T) {
	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	entries, err := checkNameSyntax("NAMESERVER", dnsname.New("ns1.123"), "Syntax04")
	if err != nil {
		t.Fatalf("check name syntax: %v", err)
	}
	if !hasEntryTag(entries, "NAMESERVER_NUMERIC_TLD") {
		t.Fatalf("expected NAMESERVER_NUMERIC_TLD")
	}
}

func TestRnameToEmailEscapedDots(t *testing.T) {
	got := rnameToEmail("host\\.master.example.")
	if got != "host.master@example" {
		t.Fatalf("expected host.master@example, got %q", got)
	}
}

func TestLabelNotACEHasDoubleHyphen(t *testing.T) {
	if !labelNotACEHasDoubleHyphen("ab--cd") {
		t.Fatalf("expected double hyphen to be discouraged")
	}
	if labelNotACEHasDoubleHyphen("xn--id") {
		t.Fatalf("expected ACE prefix to be ignored")
	}
}

func newRootZoneWithHook(t *testing.T, handler func(qname string, qtype string) packet.Packet) *zone.Zone {
	t.Helper()
	nameserver.EmptyCache()
	t.Cleanup(nameserver.EmptyCache)
	t.Cleanup(profile.ResetEffective)

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	r := &recursor.Recursor{}
	if err := r.AddFakeAddresses(".", map[string][]string{"a.root": {"192.0.2.1"}}); err != nil {
		t.Fatalf("add root hints: %v", err)
	}

	ns, err := nameserver.New("a.root", "192.0.2.1", r.Client())
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}
	ns.SetQueryHook(func(_ context.Context, qname string, qtype string, _ string, _ *nameserver.QueryOptions) (packet.Packet, error) {
		return handler(qname, qtype), nil
	})

	z, err := zone.NewWithRecursor(".", r)
	if err != nil {
		t.Fatalf("new zone: %v", err)
	}
	return &z
}

func nsPacket(zoneName string, nsName string) packet.Packet {
	msg := new(dns.Msg)
	msg.Rcode = dns.RcodeSuccess
	msg.Answer = []dns.RR{
		&dns.NS{
			Hdr: dns.RR_Header{
				Name:   dns.Fqdn(zoneName),
				Rrtype: dns.TypeNS,
				Class:  dns.ClassINET,
				Ttl:    60,
			},
			Ns: dns.Fqdn(nsName),
		},
	}
	return packet.Packet{Msg: msg}
}

func soaPacket(zoneName string, mname string, rname string) packet.Packet {
	msg := new(dns.Msg)
	msg.Rcode = dns.RcodeSuccess
	msg.Authoritative = true
	msg.Answer = []dns.RR{
		&dns.SOA{
			Hdr: dns.RR_Header{
				Name:   dns.Fqdn(zoneName),
				Rrtype: dns.TypeSOA,
				Class:  dns.ClassINET,
				Ttl:    60,
			},
			Ns:      dns.Fqdn(mname),
			Mbox:    dns.Fqdn(rname),
			Serial:  1,
			Refresh: 3600,
			Retry:   600,
			Expire:  86400,
			Minttl:  60,
		},
	}
	return packet.Packet{Msg: msg}
}

func mxPacket(zoneName string, exchange string) packet.Packet {
	msg := new(dns.Msg)
	msg.Rcode = dns.RcodeSuccess
	msg.Answer = []dns.RR{
		&dns.MX{
			Hdr: dns.RR_Header{
				Name:   dns.Fqdn(zoneName),
				Rrtype: dns.TypeMX,
				Class:  dns.ClassINET,
				Ttl:    60,
			},
			Preference: 10,
			Mx:         dns.Fqdn(exchange),
		},
	}
	return packet.Packet{Msg: msg}
}

// hasEntryTag reports whether a tag is present in the entries.
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
