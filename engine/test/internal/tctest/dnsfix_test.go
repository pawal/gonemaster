package tctest

import (
	"reflect"
	"testing"
	"time"

	dns "codeberg.org/miekg/dns"
)

func TestResponseDefaultsAndSections(t *testing.T) {
	p := Response(
		Question("example", dns.TypeSOA),
		Answers(SOARR("example")),
		Authority(NSRR("example", "ns1.example")),
		Additional(ARR("ns1.example", "192.0.2.1")),
	)

	if p.Msg == nil {
		t.Fatalf("expected a message")
	}
	if p.Msg.Rcode != dns.RcodeSuccess || !p.Msg.Authoritative {
		t.Fatalf("expected an authoritative NOERROR response, got %#v", p.Msg)
	}
	if len(p.Msg.Question) != 1 || p.Msg.Question[0].Header().Name != "example." {
		t.Fatalf("unexpected question section: %#v", p.Msg.Question)
	}
	if len(p.Msg.Answer) != 1 || len(p.Msg.Ns) != 1 || len(p.Msg.Extra) != 1 {
		t.Fatalf("unexpected sections: %#v", p.Msg)
	}
}

func TestResponseOptions(t *testing.T) {
	if p := Response(Rcode(dns.RcodeRefused)); p.Msg.Rcode != dns.RcodeRefused {
		t.Fatalf("expected REFUSED, got %d", p.Msg.Rcode)
	}
	if p := Response(NXDOMAIN()); p.Msg.Rcode != dns.RcodeNameError {
		t.Fatalf("expected NXDOMAIN, got %d", p.Msg.Rcode)
	}
	if p := Response(NotAuthoritative()); p.Msg.Authoritative {
		t.Fatalf("expected the AA bit to be clear")
	}
	secure := Response(Secure())
	if !secure.Msg.Response || !secure.Msg.Security || secure.Msg.UDPSize != 1232 {
		t.Fatalf("expected a DNSSEC-aware reply, got %#v", secure.Msg)
	}
}

func TestAnswersAppendInOrder(t *testing.T) {
	p := Response(Answers(NSRR("example", "ns2.example")), Answers(NSRRs("example", "ns1.example", "ns3.example")...))

	got := make([]string, 0, len(p.Msg.Answer))
	for _, rr := range p.Msg.Answer {
		got = append(got, rr.(*dns.NS).Ns)
	}
	want := []string{"ns2.example.", "ns1.example.", "ns3.example."}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
}

func TestSOARRDefaultsAndOverrides(t *testing.T) {
	soa := SOARR("example")
	if soa.Hdr.Name != "example." || soa.Hdr.TTL != 60 || soa.Hdr.Class != dns.ClassINET {
		t.Fatalf("unexpected header: %#v", soa.Hdr)
	}
	if soa.Ns != "ns1.example." || soa.Mbox != "hostmaster.example." {
		t.Fatalf("unexpected mname/rname: %s %s", soa.Ns, soa.Mbox)
	}
	if soa.Serial != 1 || soa.Refresh != 3600 || soa.Retry != 600 || soa.Expire != 86400 || soa.Minttl != 60 {
		t.Fatalf("unexpected timers: %#v", soa)
	}

	// A zero override must stick, which rules out zero-value defaulting.
	soa = SOARR(".", MName("a.root"), RName("hostmaster.root"), Serial(0),
		Refresh(1), Retry(2), Expire(3), Minttl(0))
	if soa.Hdr.Name != "." || soa.Ns != "a.root." || soa.Mbox != "hostmaster.root." {
		t.Fatalf("unexpected root SOA: %#v", soa)
	}
	if soa.Serial != 0 || soa.Refresh != 1 || soa.Retry != 2 || soa.Expire != 3 || soa.Minttl != 0 {
		t.Fatalf("expected the overrides to stick, got %#v", soa)
	}
}

func TestSOARRRootDefaultNames(t *testing.T) {
	if soa := SOARR("."); soa.Ns != "ns1.root." || soa.Mbox != "hostmaster.root." {
		t.Fatalf("unexpected root defaults: %s %s", soa.Ns, soa.Mbox)
	}
}

func TestRecordBuilders(t *testing.T) {
	if rr := NSRR("example", "ns1.example"); rr.Ns != "ns1.example." {
		t.Fatalf("unexpected NS: %#v", rr)
	}
	if rr := ARR("ns1.example", "192.0.2.1"); rr.Addr.String() != "192.0.2.1" {
		t.Fatalf("unexpected A: %#v", rr)
	}
	if rr := AAAARR("ns1.example", "2001:db8::1"); rr.Addr.String() != "2001:db8::1" {
		t.Fatalf("unexpected AAAA: %#v", rr)
	}
	if _, ok := AddrRR("ns1.example", "192.0.2.1").(*dns.A); !ok {
		t.Fatalf("expected an A record for an IPv4 address")
	}
	if _, ok := AddrRR("ns1.example", "2001:db8::1").(*dns.AAAA); !ok {
		t.Fatalf("expected an AAAA record for an IPv6 address")
	}
	if rr := TXTRR("example", "v=spf1 -all"); !reflect.DeepEqual(rr.Txt, []string{"v=spf1 -all"}) {
		t.Fatalf("unexpected TXT: %#v", rr)
	}
	if rr := MXRR("example", 10, "mail.example"); rr.Preference != 10 || rr.Mx != "mail.example." {
		t.Fatalf("unexpected MX: %#v", rr)
	}
	if rr := CNAMERR("www.example", "example"); rr.Target != "example." {
		t.Fatalf("unexpected CNAME: %#v", rr)
	}
	if rr := DNAMERR("sub.example", "example"); rr.Target != "example." {
		t.Fatalf("unexpected DNAME: %#v", rr)
	}
	if rr := PTRRR("1.2.0.192.in-addr.arpa", "ns1.example"); rr.Ptr != "ns1.example." {
		t.Fatalf("unexpected PTR: %#v", rr)
	}
}

func TestTTLRewritesHeaders(t *testing.T) {
	rrs := TTL(300, NSRR("example", "ns1.example"), ARR("ns1.example", "192.0.2.1"))

	for _, rr := range rrs {
		if rr.Header().TTL != 300 {
			t.Fatalf("expected TTL 300, got %d", rr.Header().TTL)
		}
	}
}

func TestDNSKEYAndDS(t *testing.T) {
	key := DNSKEYRR("example", dns.RSASHA256, PublicKey("AwEAAc=="))
	if key.Flags != dns.FlagZONE || key.Protocol != 3 || key.Algorithm != dns.RSASHA256 {
		t.Fatalf("unexpected DNSKEY: %#v", key)
	}
	if key.PublicKey != "AwEAAc==" || key.Hdr.TTL != 60 {
		t.Fatalf("unexpected DNSKEY body: %#v", key)
	}

	sep := DNSKEYRR("example", dns.RSASHA256, SEP(), KeyTTL(3600))
	if sep.Flags != dns.FlagZONE|dns.FlagSEP || sep.Hdr.TTL != 3600 {
		t.Fatalf("unexpected SEP key: %#v", sep)
	}
	if flagged := DNSKEYRR("example", dns.RSASHA256, Flags(dns.FlagSEP)); flagged.Flags != dns.FlagSEP {
		t.Fatalf("expected the flags override to stick, got %d", flagged.Flags)
	}

	ds := DSRR("example", 12345, 8, 2, "DEADBEEF")
	if ds.KeyTag != 12345 || ds.Algorithm != 8 || ds.DigestType != 2 || ds.Digest != "DEADBEEF" {
		t.Fatalf("unexpected DS: %#v", ds)
	}
}

func TestRRSIGRRDefaultsAreCurrentlyValid(t *testing.T) {
	sig := RRSIGRR("example", dns.TypeSOA)

	if sig.TypeCovered != dns.TypeSOA || sig.SignerName != "example." {
		t.Fatalf("unexpected RRSIG: %#v", sig)
	}
	if !sig.ValidPeriod(time.Now()) {
		t.Fatalf("expected the default validity window to cover now")
	}

	now := time.Now().UTC()
	expired := RRSIGRR("example", dns.TypeSOA,
		Inception(now.Add(-48*time.Hour)), Expiration(now.Add(-24*time.Hour)), KeyTag(4242))
	if expired.ValidPeriod(now) {
		t.Fatalf("expected the overridden window to be expired")
	}
	if expired.KeyTag != 4242 {
		t.Fatalf("expected the key tag override, got %d", expired.KeyTag)
	}
}

func TestSignedKeyProducesVerifiableSignature(t *testing.T) {
	key, signer := SignedKey(t, "example", dns.ECDSAP256SHA256, SEP(), KeyTTL(3600))
	if key.Flags != dns.FlagZONE|dns.FlagSEP || key.PublicKey == "" {
		t.Fatalf("expected a generated SEP key, got %#v", key)
	}

	rrset := []dns.RR{key}
	sig := Sign(t, key, signer, dns.TypeDNSKEY, rrset)
	if sig.KeyTag != key.KeyTag() || sig.Algorithm != key.Algorithm {
		t.Fatalf("unexpected signature metadata: %#v", sig)
	}
	if err := sig.Verify(key, rrset, &dns.SignOption{}); err != nil {
		t.Fatalf("expected the signature to verify: %v", err)
	}
}

func TestSignRejectsEmptyRRset(t *testing.T) {
	key, signer := SignedKey(t, "example", dns.ECDSAP256SHA256)

	mustFail(t, "empty rrset", func(tb TB) { Sign(tb, key, signer, dns.TypeDNSKEY, nil) })
}

func TestNSItems(t *testing.T) {
	item := NSItem("ns1.example", "192.0.2.1")
	if item.Name.String() != "ns1.example" || !item.HasAddress || item.Address.String() != "192.0.2.1" {
		t.Fatalf("unexpected item: %#v", item)
	}
	if unresolved := NSItem("ns2.example", ""); unresolved.HasAddress || unresolved.Address.IsValid() {
		t.Fatalf("expected an unresolved item, got %#v", unresolved)
	}

	items := NSItems("ns1.example/192.0.2.1", "ns2.example")
	if len(items) != 2 {
		t.Fatalf("expected two items, got %d", len(items))
	}
	if !items[0].HasAddress || items[0].Address.String() != "192.0.2.1" {
		t.Fatalf("unexpected first item: %#v", items[0])
	}
	if items[1].Name.String() != "ns2.example" || items[1].HasAddress {
		t.Fatalf("unexpected second item: %#v", items[1])
	}
}
