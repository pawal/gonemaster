package syntax

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/miekg/dns"

	"codeberg.org/pawal/gonemaster/engine/dnsname"
	"codeberg.org/pawal/gonemaster/engine/internal/testhelpers"
	"codeberg.org/pawal/gonemaster/engine/logger"
	"codeberg.org/pawal/gonemaster/engine/nameserver"
	"codeberg.org/pawal/gonemaster/engine/packet"
	"codeberg.org/pawal/gonemaster/engine/recursor"
	"codeberg.org/pawal/gonemaster/engine/zone"
)

func TestSyntax01AllowedChars(t *testing.T) {
	ctx := testContext(t)

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Syntax01(ctx, &z)
	if err != nil {
		t.Fatalf("syntax01: %v", err)
	}
	if !hasEntryTag(entries, "ONLY_ALLOWED_CHARS") {
		t.Fatalf("expected ONLY_ALLOWED_CHARS")
	}
}

func TestSyntax01NonAllowedChars(t *testing.T) {
	ctx := testContext(t)

	z := zone.Zone{Name: dnsname.New("bad_.example")}
	entries, err := Syntax01(ctx, &z)
	if err != nil {
		t.Fatalf("syntax01: %v", err)
	}
	if !hasEntryTag(entries, "NON_ALLOWED_CHARS") {
		t.Fatalf("expected NON_ALLOWED_CHARS")
	}
}

func TestSyntax02HyphenTags(t *testing.T) {
	ctx := testContext(t)

	z := zone.Zone{Name: dnsname.New("-bad-.example")}
	entries, err := Syntax02(ctx, &z)
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
	ctx := testContext(t)

	z := zone.Zone{Name: dnsname.New("good.example")}
	entries, err := Syntax02(ctx, &z)
	if err != nil {
		t.Fatalf("syntax02: %v", err)
	}
	if !hasEntryTag(entries, "NO_ENDING_HYPHENS") {
		t.Fatalf("expected NO_ENDING_HYPHENS")
	}
}

func TestSyntax03DoubleDash(t *testing.T) {
	ctx := testContext(t)

	z := zone.Zone{Name: dnsname.New("ab--cd.example")}
	entries, err := Syntax03(ctx, &z)
	if err != nil {
		t.Fatalf("syntax03: %v", err)
	}
	if !hasEntryTag(entries, "DISCOURAGED_DOUBLE_DASH") {
		t.Fatalf("expected DISCOURAGED_DOUBLE_DASH")
	}
}

func TestSyntax03NoDoubleDash(t *testing.T) {
	ctx := testContext(t)

	z := zone.Zone{Name: dnsname.New("good.example")}
	entries, err := Syntax03(ctx, &z)
	if err != nil {
		t.Fatalf("syntax03: %v", err)
	}
	if !hasEntryTag(entries, "NO_DOUBLE_DASH") {
		t.Fatalf("expected NO_DOUBLE_DASH")
	}
}

func TestSyntax04NameserverSyntaxOK(t *testing.T) {
	ctx := testContext(t)
	z := newRootZoneWithHook(ctx, t, func(qname string, qtype string) packet.Packet {
		if strings.EqualFold(qname, ".") && strings.EqualFold(qtype, "NS") {
			return nsPacket(".", "ns1.example.")
		}
		return packet.Packet{}
	})

	entries, err := Syntax04(ctx, z)
	if err != nil {
		t.Fatalf("syntax04: %v", err)
	}
	if !hasEntryTag(entries, "NAMESERVER_SYNTAX_OK") {
		t.Fatalf("expected NAMESERVER_SYNTAX_OK")
	}
}

func TestSyntax05MisusedAtSign(t *testing.T) {
	ctx := testContext(t)
	z := newRootZoneWithHook(ctx, t, func(qname string, qtype string) packet.Packet {
		if strings.EqualFold(qname, ".") && strings.EqualFold(qtype, "SOA") {
			return soaPacket(".", "a.root.", "user@example.")
		}
		return packet.Packet{}
	})

	entries, err := Syntax05(ctx, z)
	if err != nil {
		t.Fatalf("syntax05: %v", err)
	}
	if !hasEntryTag(entries, "RNAME_MISUSED_AT_SIGN") {
		t.Fatalf("expected RNAME_MISUSED_AT_SIGN")
	}
}

func TestSyntax05NoResponseSOAQuery(t *testing.T) {
	ctx := testContext(t)
	z := newRootZoneWithHook(ctx, t, func(_ string, _ string) packet.Packet {
		return packet.Packet{}
	})

	entries, err := Syntax05(ctx, z)
	if err != nil {
		t.Fatalf("syntax05: %v", err)
	}
	if !hasEntryTag(entries, "NO_RESPONSE_SOA_QUERY") {
		t.Fatalf("expected NO_RESPONSE_SOA_QUERY")
	}
}

func TestSyntax06ParallelMailServers(t *testing.T) {
	nameserver.EmptyCache()
	defer nameserver.EmptyCache()
	baseCtx, prof, _ := testhelpers.Context(t)
	prof.Resolver.Defaults.Parallel = 2

	r := &recursor.Recursor{}
	if err := r.AddFakeAddresses(".", map[string][]string{
		"a.root": {"192.0.2.1"},
	}); err != nil {
		t.Fatalf("add root hints: %v", err)
	}

	started := make(chan string, 2)
	release := make(chan struct{})

	root, err := nameserver.NewWithContext(baseCtx, "a.root", "192.0.2.1", r.Client())
	if err != nil {
		t.Fatalf("new root nameserver: %v", err)
	}
	root.SetQueryHook(func(ctx context.Context, qname string, qtype string, _ string, _ *nameserver.QueryOptions) (packet.Packet, error) {
		name := strings.ToLower(qname)
		kind := strings.ToUpper(qtype)
		switch {
		case name == "." && kind == "NS":
			return nsPacket(".", "a.root"), nil
		case name == "." && kind == "SOA":
			return soaPacket(".", "a.root", "hostmaster.example."), nil
		case name == "example" && kind == "MX":
			return mxPacketMulti("example", "mail1.example.", "mail2.example."), nil
		case name == "mail1.example" && kind == "A":
			select {
			case started <- "mail1":
			default:
			}
			select {
			case <-release:
			case <-ctx.Done():
				return packet.Packet{}, ctx.Err()
			}
			return aPacket("mail1.example", net.IPv4(192, 0, 2, 10)), nil
		case name == "mail2.example" && kind == "A":
			select {
			case started <- "mail2":
			default:
			}
			select {
			case <-release:
			case <-ctx.Done():
				return packet.Packet{}, ctx.Err()
			}
			return aPacket("mail2.example", net.IPv4(192, 0, 2, 11)), nil
		case name == "mail1.example" && kind == "AAAA":
			return packet.Packet{}, nil
		case name == "mail2.example" && kind == "AAAA":
			return packet.Packet{}, nil
		default:
			return packet.Packet{}, nil
		}
	})

	z, err := zone.NewWithRecursor(".", r)
	if err != nil {
		t.Fatalf("new zone: %v", err)
	}

	ctx, cancel := context.WithTimeout(baseCtx, 2*time.Second)
	defer cancel()

	done := make(chan struct{})
	var entries []*logger.Entry
	var syntaxErr error
	go func() {
		entries, syntaxErr = Syntax06(ctx, &z)
		close(done)
	}()

	got := map[string]bool{}
	deadline := time.After(1 * time.Second)
	for len(got) < 2 {
		select {
		case id := <-started:
			got[id] = true
		case <-deadline:
			t.Fatalf("expected parallel mail queries to start, got %v", got)
		}
	}

	close(release)

	select {
	case <-done:
		if syntaxErr != nil {
			t.Fatalf("syntax06: %v", syntaxErr)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("syntax06 did not finish")
	}

	if !hasEntryTag(entries, "RNAME_RFC822_VALID") {
		t.Fatalf("expected RNAME_RFC822_VALID")
	}
}

func TestSyntax07MNameSyntaxOK(t *testing.T) {
	ctx := testContext(t)
	z := newRootZoneWithHook(ctx, t, func(qname string, qtype string) packet.Packet {
		if strings.EqualFold(qname, ".") && strings.EqualFold(qtype, "SOA") {
			return soaPacket(".", "ns1.example.", "hostmaster.example.")
		}
		return packet.Packet{}
	})

	entries, err := Syntax07(ctx, z)
	if err != nil {
		t.Fatalf("syntax07: %v", err)
	}
	if !hasEntryTag(entries, "MNAME_SYNTAX_OK") {
		t.Fatalf("expected MNAME_SYNTAX_OK")
	}
}

func TestSyntax08MxSyntaxOK(t *testing.T) {
	ctx := testContext(t)
	z := newRootZoneWithHook(ctx, t, func(qname string, qtype string) packet.Packet {
		if strings.EqualFold(qname, ".") && strings.EqualFold(qtype, "MX") {
			return mxPacket(".", "mail.example.")
		}
		return packet.Packet{}
	})

	entries, err := Syntax08(ctx, z)
	if err != nil {
		t.Fatalf("syntax08: %v", err)
	}
	if !hasEntryTag(entries, "MX_SYNTAX_OK") {
		t.Fatalf("expected MX_SYNTAX_OK")
	}
}

func TestSyntax08NoResponseMXQuery(t *testing.T) {
	ctx := testContext(t)
	z := newRootZoneWithHook(ctx, t, func(_ string, _ string) packet.Packet {
		return packet.Packet{}
	})

	entries, err := Syntax08(ctx, z)
	if err != nil {
		t.Fatalf("syntax08: %v", err)
	}
	if !hasEntryTag(entries, "NO_RESPONSE_MX_QUERY") {
		t.Fatalf("expected NO_RESPONSE_MX_QUERY")
	}
}

func TestCheckNameSyntaxNumericTLD(t *testing.T) {
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

func testContext(t *testing.T) context.Context {
	t.Helper()
	ctx, _, _ := testhelpers.Context(t)
	return ctx
}

func newRootZoneWithHook(ctx context.Context, t *testing.T, handler func(qname string, qtype string) packet.Packet) *zone.Zone {
	t.Helper()
	nameserver.EmptyCache()
	t.Cleanup(nameserver.EmptyCache)

	r := &recursor.Recursor{}
	if err := r.AddFakeAddresses(".", map[string][]string{"a.root": {"192.0.2.1"}}); err != nil {
		t.Fatalf("add root hints: %v", err)
	}

	ns, err := nameserver.NewWithContext(ctx, "a.root", "192.0.2.1", r.Client())
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

func mxPacketMulti(zoneName string, exchanges ...string) packet.Packet {
	msg := new(dns.Msg)
	msg.Rcode = dns.RcodeSuccess
	msg.Authoritative = true
	for i, exchange := range exchanges {
		msg.Answer = append(msg.Answer, &dns.MX{
			Hdr: dns.RR_Header{
				Name:   dns.Fqdn(zoneName),
				Rrtype: dns.TypeMX,
				Class:  dns.ClassINET,
				Ttl:    60,
			},
			Preference: uint16(10 + i),
			Mx:         dns.Fqdn(exchange),
		})
	}
	return packet.Packet{Msg: msg}
}

func aPacket(owner string, addr net.IP) packet.Packet {
	msg := new(dns.Msg)
	msg.Rcode = dns.RcodeSuccess
	msg.Authoritative = true
	msg.Answer = []dns.RR{
		&dns.A{
			Hdr: dns.RR_Header{
				Name:   dns.Fqdn(owner),
				Rrtype: dns.TypeA,
				Class:  dns.ClassINET,
				Ttl:    60,
			},
			A: addr,
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
