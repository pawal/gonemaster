package dnstest

import (
	"testing"

	dns "codeberg.org/miekg/dns"
)

func TestResponseDefaultsAndOptions(t *testing.T) {
	p := Response()
	if p.Msg.Rcode != dns.RcodeSuccess {
		t.Fatalf("expected NOERROR by default, got %d", p.Msg.Rcode)
	}
	if !p.Msg.Authoritative {
		t.Fatal("expected the AA bit set by default, as tctest.Response does")
	}

	p = Response(NotAuthoritative(), NXDOMAIN(), AnswerFrom("192.0.2.1"), Reply(), Secure())
	if p.Msg.Authoritative {
		t.Fatal("expected NotAuthoritative to clear the AA bit")
	}
	if p.Msg.Rcode != dns.RcodeNameError {
		t.Fatalf("expected NXDOMAIN, got %d", p.Msg.Rcode)
	}
	if p.AnswerFrom != "192.0.2.1" {
		t.Fatalf("expected AnswerFrom to be recorded, got %q", p.AnswerFrom)
	}
	if !p.Msg.Response || !p.Msg.Security || p.Msg.UDPSize != 1232 {
		t.Fatalf("expected Reply and Secure to apply, got %#v", p.Msg)
	}
}

func TestResponseSections(t *testing.T) {
	p := Response(
		Question("example.test", dns.TypeA),
		Answers(ARR("example.test", "192.0.2.1")),
		Authority(NSRR("example.test", "ns1.example.test")),
		Additional(AAAARR("ns1.example.test", "2001:db8::1")),
	)
	if len(p.Msg.Question) != 1 || p.Msg.Question[0].Header().Name != "example.test." {
		t.Fatalf("unexpected question section: %#v", p.Msg.Question)
	}
	if len(p.Msg.Answer) != 1 || len(p.Msg.Ns) != 1 || len(p.Msg.Extra) != 1 {
		t.Fatalf("unexpected section sizes: %d/%d/%d", len(p.Msg.Answer), len(p.Msg.Ns), len(p.Msg.Extra))
	}
}

func TestFromDecoratesAnExistingPacket(t *testing.T) {
	p := From(Response(), AnswerFrom("192.0.2.9"), Rcode(dns.RcodeServerFailure))
	if p.AnswerFrom != "192.0.2.9" || p.Msg.Rcode != dns.RcodeServerFailure {
		t.Fatalf("expected From to apply both options, got %#v", p)
	}
}

func TestReferralAndNoData(t *testing.T) {
	ref := Referral("example.test", "ns1.example.test", "ns2.example.test")
	if ref.Msg.Authoritative {
		t.Fatal("a referral comes from a server that is not authoritative for the child")
	}
	if len(ref.Msg.Ns) != 2 || len(ref.Msg.Answer) != 0 {
		t.Fatalf("expected two authority NS records and no answer, got %#v", ref.Msg)
	}

	nodata := NoData("example.test")
	if len(nodata.Msg.Answer) != 0 || len(nodata.Msg.Ns) != 1 {
		t.Fatalf("expected an empty answer with the SOA in authority, got %#v", nodata.Msg)
	}
	if _, ok := nodata.Msg.Ns[0].(*dns.SOA); !ok {
		t.Fatalf("expected an SOA in the authority section, got %T", nodata.Msg.Ns[0])
	}
}

func TestMixedApexRecords(t *testing.T) {
	p := MixedApexRecords("example.test", []string{"ns1.example.test", "ns2.example.test"})
	if p.Msg.Authoritative {
		t.Fatal("the apex mix models a non-authoritative answer")
	}
	// SOA first, then one NS per name, then the decoy A.
	if len(p.Msg.Answer) != 4 {
		t.Fatalf("expected 4 answer records, got %#v", p.Msg.Answer)
	}
	soa, ok := p.Msg.Answer[0].(*dns.SOA)
	if !ok || soa.Hdr.TTL != 3600 || soa.Ns != "ns1.example.test." {
		t.Fatalf("unexpected SOA: %#v", p.Msg.Answer[0])
	}
	for i, want := range []string{"ns1.example.test.", "ns2.example.test."} {
		ns, ok := p.Msg.Answer[1+i].(*dns.NS)
		if !ok || ns.Ns != want {
			t.Fatalf("expected NS %s at %d, got %#v", want, 1+i, p.Msg.Answer[1+i])
		}
	}
	decoy, ok := p.Msg.Answer[3].(*dns.A)
	if !ok || decoy.Hdr.Name != "decoy.example.test." {
		t.Fatalf("unexpected decoy record: %#v", p.Msg.Answer[3])
	}
}

func TestRRBuildersFqdnAndDefaults(t *testing.T) {
	if got := ARR("example.test", "192.0.2.1"); got.Hdr.Name != "example.test." || got.Hdr.TTL != defaultTTL {
		t.Fatalf("unexpected A header: %#v", got.Hdr)
	}
	if got := NSRR("example.test", "ns1.example.test"); got.Ns != "ns1.example.test." {
		t.Fatalf("expected the NS target to be fully qualified, got %q", got.Ns)
	}
	if got := CNAMERR("a.test", "b.test"); got.Target != "b.test." {
		t.Fatalf("expected the CNAME target to be fully qualified, got %q", got.Target)
	}
	if got := PTRRR("1.2.0.192.in-addr.arpa", "host.test"); got.Ptr != "host.test." {
		t.Fatalf("expected the PTR target to be fully qualified, got %q", got.Ptr)
	}
	if got := TXTRR("example.test", "a", "b"); len(got.Txt) != 2 {
		t.Fatalf("expected two TXT strings, got %#v", got.Txt)
	}
	if got := NSRRs("example.test", "ns1.test", "ns2.test"); len(got) != 2 {
		t.Fatalf("expected two NS records, got %d", len(got))
	}
}

func TestAddrRRPicksTheRecordType(t *testing.T) {
	if _, ok := AddrRR("example.test", "192.0.2.1").(*dns.A); !ok {
		t.Fatal("expected an A record for an IPv4 address")
	}
	if _, ok := AddrRR("example.test", "2001:db8::1").(*dns.AAAA); !ok {
		t.Fatal("expected an AAAA record for an IPv6 address")
	}
}

func TestSOARRDefaultsAndOverrides(t *testing.T) {
	soa := SOARR("example.test")
	if soa.Ns != "ns.example.test." || soa.Mbox != "hostmaster.example.test." {
		t.Fatalf("unexpected SOA defaults: %s / %s", soa.Ns, soa.Mbox)
	}

	// The root has no label to build on, so it gets a stand-in.
	if got := SOARR("."); got.Ns != "ns.root." {
		t.Fatalf("unexpected root SOA mname: %q", got.Ns)
	}

	soa = SOARR("example.test", MName("a.test"), RName("hm.test"), Serial(7), SOATimers(1, 2, 3, 4))
	if soa.Ns != "a.test." || soa.Mbox != "hm.test." || soa.Serial != 7 {
		t.Fatalf("unexpected SOA overrides: %#v", soa)
	}
	if soa.Refresh != 1 || soa.Retry != 2 || soa.Expire != 3 || soa.Minttl != 4 {
		t.Fatalf("unexpected SOA timers: %#v", soa)
	}
}

func TestTTLOverridesRecords(t *testing.T) {
	rrs := TTL(300, ARR("example.test", "192.0.2.1"), NSRR("example.test", "ns1.test"))
	for _, rr := range rrs {
		if rr.Header().TTL != 300 {
			t.Fatalf("expected TTL 300, got %d", rr.Header().TTL)
		}
	}
}
