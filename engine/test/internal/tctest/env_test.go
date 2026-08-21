package tctest

import (
	"context"
	"testing"

	dns "codeberg.org/miekg/dns"
	"codeberg.org/miekg/dns/dnsutil"

	"codeberg.org/pawal/gonemaster/engine/nameserver"
	"codeberg.org/pawal/gonemaster/engine/packet"
	"codeberg.org/pawal/gonemaster/engine/util"
)

func TestContextCarriesCacheAndLogger(t *testing.T) {
	var inner context.Context
	var testLogger any

	t.Run("inner", func(t *testing.T) {
		inner = Context(t)
		if nameserver.CacheFromContext(inner) == nil {
			t.Fatalf("expected a nameserver cache in the context")
		}
		testLogger = util.Logger()
		if testLogger == nil {
			t.Fatalf("expected a global logger for the test")
		}
	})

	// The cleanup registered by Context clears the global logger, so the next
	// caller gets a fresh one instead of the finished test's.
	if util.Logger() == testLogger {
		t.Fatalf("expected the global logger to be cleared after the test")
	}
}

func TestSetupClearsGlobalLoggerAfterTest(t *testing.T) {
	var testLogger any

	t.Run("inner", func(t *testing.T) {
		Setup(t)
		testLogger = util.Logger()
	})

	if util.Logger() == testLogger {
		t.Fatalf("expected the global logger to be cleared after the test")
	}
}

func TestNSAnswersThroughHandler(t *testing.T) {
	ctx := Context(t)
	var seen Query
	ns := NS(t, ctx, "ns1.example", "192.0.2.1", func(q Query) packet.Packet {
		seen = q
		return soaAnswer("example")
	})

	if got := ns.Name.String(); got != "ns1.example" {
		t.Fatalf("expected ns1.example, got %s", got)
	}
	p, err := ns.QueryWithClass(ctx, "example", "SOA", "IN")
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if p.Msg == nil || len(p.Msg.Answer) != 1 {
		t.Fatalf("expected the handler answer, got %#v", p.Msg)
	}
	if seen.Name != "example" || seen.Type != "SOA" || seen.Class != "IN" {
		t.Fatalf("unexpected query passed to the handler: %#v", seen)
	}
}

func TestNSWithoutHandlerAnswersEmpty(t *testing.T) {
	ctx := Context(t)
	ns := NS(t, ctx, "ns1.example", "192.0.2.1", nil)

	p, err := ns.Query(ctx, "example", "SOA")
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if p.Msg != nil {
		t.Fatalf("expected an empty packet, got %#v", p.Msg)
	}
}

func TestNSRejectsBadAddress(t *testing.T) {
	ctx := Context(t)
	mustFail(t, "new nameserver", func(tb TB) { NS(tb, ctx, "ns1.example", "not-an-ip", nil) })
}

func TestRootZoneServesHandler(t *testing.T) {
	ctx := Context(t)
	z := RootZone(t, ctx, func(q Query) packet.Packet {
		if q.Type == "NS" {
			return nsAnswer(".", "a.root")
		}
		return packet.Packet{}
	})

	if got := z.Name.String(); got != "." {
		t.Fatalf("expected the root zone, got %s", got)
	}
	names, err := z.ApexNSNames(ctx)
	if err != nil {
		t.Fatalf("apex ns names: %v", err)
	}
	if len(names) != 1 || names[0].String() != "a.root" {
		t.Fatalf("expected a.root from the handler, got %v", names)
	}
}

func TestZoneWithAddrsResolvesFakeAddresses(t *testing.T) {
	z := ZoneWithAddrs(t, "example", map[string][]string{"ns1.example": {"192.0.2.1"}})

	if got := z.Name.String(); got != "example" {
		t.Fatalf("expected the example zone, got %s", got)
	}
	if !z.Recursor().HasFakeAddresses("example") {
		t.Fatalf("expected the zone to be undelegated")
	}
	addrs := z.Recursor().GetFakeAddresses("example", "ns1.example")
	if len(addrs) != 1 || addrs[0].String() != "192.0.2.1" {
		t.Fatalf("expected 192.0.2.1, got %v", addrs)
	}
}

// seam stands in for a package-level function variable a testcase overrides.
var seam = func() string { return "real" }

func TestStubRestoresOnCleanup(t *testing.T) {
	t.Run("stubbed", func(t *testing.T) {
		Stub(t, &seam, func() string { return "stub" })
		if got := seam(); got != "stub" {
			t.Fatalf("expected the stub, got %s", got)
		}
	})

	if got := seam(); got != "real" {
		t.Fatalf("expected the original seam, got %s", got)
	}
}

func TestStubNestsAndUnwinds(t *testing.T) {
	t.Run("outer", func(t *testing.T) {
		Stub(t, &seam, func() string { return "outer" })
		t.Run("inner", func(t *testing.T) {
			Stub(t, &seam, func() string { return "inner" })
			if got := seam(); got != "inner" {
				t.Fatalf("expected the inner stub, got %s", got)
			}
		})
		if got := seam(); got != "outer" {
			t.Fatalf("expected the outer stub after the inner test, got %s", got)
		}
	})
	if got := seam(); got != "real" {
		t.Fatalf("expected the original seam, got %s", got)
	}
}

func soaAnswer(owner string) packet.Packet {
	msg := new(dns.Msg)
	msg.Rcode = dns.RcodeSuccess
	msg.Authoritative = true
	soa := &dns.SOA{Hdr: dns.Header{Name: dnsutil.Fqdn(owner), Class: dns.ClassINET, TTL: 60}}
	soa.Ns = dnsutil.Fqdn("ns1." + owner)
	soa.Mbox = dnsutil.Fqdn("hostmaster." + owner)
	soa.Serial = 1
	msg.Answer = []dns.RR{soa}
	return packet.Packet{Msg: msg}
}

func nsAnswer(owner string, nsName string) packet.Packet {
	msg := new(dns.Msg)
	msg.Rcode = dns.RcodeSuccess
	msg.Authoritative = true
	ns := &dns.NS{Hdr: dns.Header{Name: dnsutil.Fqdn(owner), Class: dns.ClassINET, TTL: 60}}
	ns.Ns = dnsutil.Fqdn(nsName)
	msg.Answer = []dns.RR{ns}
	return packet.Packet{Msg: msg}
}
