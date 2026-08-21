package syntax

import (
	"context"
	"net"
	"net/netip"
	"strings"
	"testing"
	"time"

	dns "codeberg.org/miekg/dns"
	"codeberg.org/miekg/dns/dnsutil"

	"codeberg.org/pawal/gonemaster/engine/dnsname"
	"codeberg.org/pawal/gonemaster/engine/internal/testhelpers"
	"codeberg.org/pawal/gonemaster/engine/logger"
	"codeberg.org/pawal/gonemaster/engine/nameserver"
	"codeberg.org/pawal/gonemaster/engine/packet"
	"codeberg.org/pawal/gonemaster/engine/recursor"
	"codeberg.org/pawal/gonemaster/engine/test/internal/tctest"
	"codeberg.org/pawal/gonemaster/engine/zone"
)

func TestSyntax01AllowedChars(t *testing.T) {
	ctx := testContext(t)

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := Syntax01(ctx, &z)
	if err != nil {
		t.Fatalf("syntax01: %v", err)
	}
	tctest.RequireTags(t, entries, "ONLY_ALLOWED_CHARS")
}

func TestSyntax01NonAllowedChars(t *testing.T) {
	ctx := testContext(t)

	z := zone.Zone{Name: dnsname.New("bad_.example")}
	entries, err := Syntax01(ctx, &z)
	if err != nil {
		t.Fatalf("syntax01: %v", err)
	}
	tctest.RequireTags(t, entries, "NON_ALLOWED_CHARS")
}

func TestSyntax02HyphenTags(t *testing.T) {
	ctx := testContext(t)

	z := zone.Zone{Name: dnsname.New("-bad-.example")}
	entries, err := Syntax02(ctx, &z)
	if err != nil {
		t.Fatalf("syntax02: %v", err)
	}
	tctest.RequireTags(t, entries, "INITIAL_HYPHEN", "TERMINAL_HYPHEN")
}

func TestSyntax02NoEndingHyphens(t *testing.T) {
	ctx := testContext(t)

	z := zone.Zone{Name: dnsname.New("good.example")}
	entries, err := Syntax02(ctx, &z)
	if err != nil {
		t.Fatalf("syntax02: %v", err)
	}
	tctest.RequireTags(t, entries, "NO_ENDING_HYPHENS")
}

func TestSyntax03DoubleDash(t *testing.T) {
	ctx := testContext(t)

	z := zone.Zone{Name: dnsname.New("ab--cd.example")}
	entries, err := Syntax03(ctx, &z)
	if err != nil {
		t.Fatalf("syntax03: %v", err)
	}
	tctest.RequireTags(t, entries, "DISCOURAGED_DOUBLE_DASH")
}

func TestSyntax03NoDoubleDash(t *testing.T) {
	ctx := testContext(t)

	z := zone.Zone{Name: dnsname.New("good.example")}
	entries, err := Syntax03(ctx, &z)
	if err != nil {
		t.Fatalf("syntax03: %v", err)
	}
	tctest.RequireTags(t, entries, "NO_DOUBLE_DASH")
}

func TestSyntax04NameserverSyntaxOK(t *testing.T) {
	ctx := testContext(t)
	z := tctest.RootZone(t, ctx, func(q tctest.Query) packet.Packet {
		if strings.EqualFold(q.Name, ".") && strings.EqualFold(q.Type, "NS") {
			return nsPacket(".", "ns1.example.")
		}
		return packet.Packet{}
	})

	entries, err := Syntax04(ctx, z)
	if err != nil {
		t.Fatalf("syntax04: %v", err)
	}
	tctest.RequireTags(t, entries, "NAMESERVER_SYNTAX_OK")
}

func TestSyntax05MisusedAtSign(t *testing.T) {
	ctx := testContext(t)
	z := tctest.RootZone(t, ctx, func(q tctest.Query) packet.Packet {
		if strings.EqualFold(q.Name, ".") && strings.EqualFold(q.Type, "SOA") {
			return soaPacket(".", "a.root.", "user@example.")
		}
		return packet.Packet{}
	})

	entries, err := Syntax05(ctx, z)
	if err != nil {
		t.Fatalf("syntax05: %v", err)
	}
	tctest.RequireTags(t, entries, "RNAME_MISUSED_AT_SIGN")
}

func TestSyntax05NoResponseSOAQuery(t *testing.T) {
	ctx := testContext(t)
	z := tctest.RootZone(t, ctx, func(q tctest.Query) packet.Packet {
		return packet.Packet{}
	})

	entries, err := Syntax05(ctx, z)
	if err != nil {
		t.Fatalf("syntax05: %v", err)
	}
	tctest.RequireTags(t, entries, "NO_RESPONSE_SOA_QUERY")
}

func TestSyntax06ParallelMailServers(t *testing.T) {
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
			return soaPacket(".", "a.root", "hostmaster.example.com."), nil
		case name == "example.com" && kind == "MX":
			return mxPacketMulti("example.com", "mail1.example.com.", "mail2.example.com."), nil
		case name == "mail1.example.com" && kind == "A":
			select {
			case started <- "mail1":
			default:
			}
			select {
			case <-release:
			case <-ctx.Done():
				return packet.Packet{}, ctx.Err()
			}
			return aPacket("mail1.example.com", net.IPv4(192, 0, 2, 10)), nil
		case name == "mail2.example.com" && kind == "A":
			select {
			case started <- "mail2":
			default:
			}
			select {
			case <-release:
			case <-ctx.Done():
				return packet.Packet{}, ctx.Err()
			}
			return aPacket("mail2.example.com", net.IPv4(192, 0, 2, 11)), nil
		case name == "mail1.example.com" && kind == "AAAA":
			return packet.Packet{}, nil
		case name == "mail2.example.com" && kind == "AAAA":
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

	tctest.RequireTags(t, entries, "RNAME_RFC822_VALID")
}

func TestSyntax06MailDomainInvalidUsesProfileLevel(t *testing.T) {
	ctx, prof, log := testhelpers.Context(t)
	log.SetProfile(prof)

	z := tctest.RootZone(t, ctx, func(q tctest.Query) packet.Packet {
		switch {
		case strings.EqualFold(q.Name, ".") && strings.EqualFold(q.Type, "NS"):
			return nsPacket(".", "a.root.")
		case strings.EqualFold(q.Name, ".") && strings.EqualFold(q.Type, "SOA"):
			return soaPacket(".", "a.root.", "hostmaster.example.com.")
		case strings.EqualFold(q.Name, "example.com") && strings.EqualFold(q.Type, "MX"):
			return mxPacket("example.com", "mail.example.com.")
		default:
			return packet.Packet{}
		}
	})

	entries, err := Syntax06(ctx, z)
	if err != nil {
		t.Fatalf("syntax06: %v", err)
	}

	entry := tctest.RequireTag(t, entries, "RNAME_MAIL_DOMAIN_INVALID")

	wantLevel := "NOTICE"
	if moduleLevels := prof.TestLevels["SYNTAX"]; moduleLevels != nil {
		if configured, ok := moduleLevels["RNAME_MAIL_DOMAIN_INVALID"]; ok {
			wantLevel = strings.ToUpper(configured)
		}
	}
	if entry.Level() != wantLevel {
		t.Fatalf("RNAME_MAIL_DOMAIN_INVALID level=%s, want %s", entry.Level(), wantLevel)
	}
}

func TestSyntax06RnameSingleLabelDomainInvalid(t *testing.T) {
	ctx := testContext(t)
	z := tctest.RootZone(t, ctx, func(q tctest.Query) packet.Packet {
		if strings.EqualFold(q.Name, ".") && strings.EqualFold(q.Type, "NS") {
			return nsPacket(".", "a.root.")
		}
		if strings.EqualFold(q.Name, ".") && strings.EqualFold(q.Type, "SOA") {
			return soaPacket(".", "a.root.", "dnsadmin.mo.")
		}
		return packet.Packet{}
	})

	entries, err := Syntax06(ctx, z)
	if err != nil {
		t.Fatalf("syntax06: %v", err)
	}
	tctest.RequireTags(t, entries, "RNAME_RFC822_INVALID")
	tctest.RequireNoTag(t, entries, "RNAME_RFC822_VALID")
}

func TestSyntax06NoResponseArgsSplit(t *testing.T) {
	ctx := testContext(t)
	z := tctest.RootZone(t, ctx, func(q tctest.Query) packet.Packet {
		if strings.EqualFold(q.Name, ".") && strings.EqualFold(q.Type, "NS") {
			return nsPacket(".", "a.root.")
		}
		return packet.Packet{}
	})

	entries, err := Syntax06(ctx, z)
	if err != nil {
		t.Fatalf("syntax06: %v", err)
	}
	entry := tctest.RequireTag(t, entries, "NO_RESPONSE")
	tctest.RequireArgShape(t, entry, tctest.ArgShape{NS: "a.root", Address: "192.0.2.1"})
}

func TestSyntax06IPv4DisabledArgsSplit(t *testing.T) {
	ctx, prof, _ := testhelpers.Context(t)
	prof.Net.IPv4 = false

	z := tctest.RootZone(t, ctx, func(q tctest.Query) packet.Packet {
		if strings.EqualFold(q.Name, ".") && strings.EqualFold(q.Type, "NS") {
			return nsPacket(".", "a.root.")
		}
		return packet.Packet{}
	})

	entries, err := Syntax06(ctx, z)
	if err != nil {
		t.Fatalf("syntax06: %v", err)
	}
	entry := tctest.RequireTag(t, entries, "IPV4_DISABLED")
	tctest.RequireArgShape(t, entry, tctest.ArgShape{NS: "a.root", Address: "192.0.2.1"})
	if _, ok := entry.Args["rrtype"]; ok {
		t.Fatalf("did not expect legacy rrtype in args: %#v", entry.Args["rrtype"])
	}
	if qtype, ok := entry.Args["query_type"].(string); !ok || qtype != "SOA" {
		t.Fatalf("expected query_type=SOA, got %#v", entry.Args["query_type"])
	}
}

func TestSyntax07MNameSyntaxOK(t *testing.T) {
	ctx := testContext(t)
	z := tctest.RootZone(t, ctx, func(q tctest.Query) packet.Packet {
		if strings.EqualFold(q.Name, ".") && strings.EqualFold(q.Type, "SOA") {
			return soaPacket(".", "ns1.example.", "hostmaster.example.")
		}
		return packet.Packet{}
	})

	entries, err := Syntax07(ctx, z)
	if err != nil {
		t.Fatalf("syntax07: %v", err)
	}
	tctest.RequireTags(t, entries, "MNAME_SYNTAX_OK")
}

func TestSyntax08MxSyntaxOK(t *testing.T) {
	ctx := testContext(t)
	z := tctest.RootZone(t, ctx, func(q tctest.Query) packet.Packet {
		if strings.EqualFold(q.Name, ".") && strings.EqualFold(q.Type, "MX") {
			return mxPacket(".", "mail.example.")
		}
		return packet.Packet{}
	})

	entries, err := Syntax08(ctx, z)
	if err != nil {
		t.Fatalf("syntax08: %v", err)
	}
	tctest.RequireTags(t, entries, "MX_SYNTAX_OK")
}

func TestSyntax08NoResponseMXQuery(t *testing.T) {
	ctx := testContext(t)
	z := tctest.RootZone(t, ctx, func(q tctest.Query) packet.Packet {
		return packet.Packet{}
	})

	entries, err := Syntax08(ctx, z)
	if err != nil {
		t.Fatalf("syntax08: %v", err)
	}
	tctest.RequireTags(t, entries, "NO_RESPONSE_MX_QUERY")
}

func TestCheckNameSyntaxNumericTLD(t *testing.T) {
	entries, err := checkNameSyntax("NAMESERVER", dnsname.New("ns1.123"), "Syntax04")
	if err != nil {
		t.Fatalf("check name syntax: %v", err)
	}
	tctest.RequireTags(t, entries, "NAMESERVER_NUMERIC_TLD")
}

func TestRnameToEmailEscapedDots(t *testing.T) {
	got := rnameToEmail("host\\.master.example.")
	if got != "host.master@example" {
		t.Fatalf("expected host.master@example, got %q", got)
	}
}

func TestValidEmailAddress(t *testing.T) {
	if !validEmailAddress("hostmaster@example.com") {
		t.Fatalf("expected hostmaster@example.com to be valid")
	}
	if validEmailAddress("dnsadmin@mo") {
		t.Fatalf("expected dnsadmin@mo to be invalid")
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

func nsPacket(zoneName string, nsName string) packet.Packet {
	msg := new(dns.Msg)
	msg.Rcode = dns.RcodeSuccess
	nsRR := &dns.NS{Hdr: dns.Header{Name: dnsutil.Fqdn(zoneName), Class: dns.ClassINET, TTL: 60}}
	nsRR.Ns = dnsutil.Fqdn(nsName)
	msg.Answer = []dns.RR{nsRR}
	return packet.Packet{Msg: msg}
}

func soaPacket(zoneName string, mname string, rname string) packet.Packet {
	msg := new(dns.Msg)
	msg.Rcode = dns.RcodeSuccess
	msg.Authoritative = true
	soaRR := &dns.SOA{Hdr: dns.Header{Name: dnsutil.Fqdn(zoneName), Class: dns.ClassINET, TTL: 60}}
	soaRR.Ns = dnsutil.Fqdn(mname)
	soaRR.Mbox = dnsutil.Fqdn(rname)
	soaRR.Serial = 1
	soaRR.Refresh = 3600
	soaRR.Retry = 600
	soaRR.Expire = 86400
	soaRR.Minttl = 60
	msg.Answer = []dns.RR{soaRR}
	return packet.Packet{Msg: msg}
}

func mxPacket(zoneName string, exchange string) packet.Packet {
	msg := new(dns.Msg)
	msg.Rcode = dns.RcodeSuccess
	mxRR := &dns.MX{Hdr: dns.Header{Name: dnsutil.Fqdn(zoneName), Class: dns.ClassINET, TTL: 60}}
	mxRR.Preference = 10
	mxRR.Mx = dnsutil.Fqdn(exchange)
	msg.Answer = []dns.RR{mxRR}
	return packet.Packet{Msg: msg}
}

func mxPacketMulti(zoneName string, exchanges ...string) packet.Packet {
	msg := new(dns.Msg)
	msg.Rcode = dns.RcodeSuccess
	msg.Authoritative = true
	for i, exchange := range exchanges {
		mxRR := &dns.MX{Hdr: dns.Header{Name: dnsutil.Fqdn(zoneName), Class: dns.ClassINET, TTL: 60}}
		mxRR.Preference = uint16(10 + i)
		mxRR.Mx = dnsutil.Fqdn(exchange)
		msg.Answer = append(msg.Answer, mxRR)
	}
	return packet.Packet{Msg: msg}
}

func aPacket(owner string, addr net.IP) packet.Packet {
	msg := new(dns.Msg)
	msg.Rcode = dns.RcodeSuccess
	msg.Authoritative = true
	aRR := &dns.A{Hdr: dns.Header{Name: dnsutil.Fqdn(owner), Class: dns.ClassINET, TTL: 60}}
	if ip4 := addr.To4(); ip4 != nil {
		aRR.Addr = netip.AddrFrom4([4]byte(ip4))
	}
	msg.Answer = []dns.RR{aRR}
	return packet.Packet{Msg: msg}
}
