package dnssec

import (
	"context"
	"crypto"
	"net/netip"
	"strings"
	"testing"

	dns "codeberg.org/miekg/dns"
	"codeberg.org/miekg/dns/dnsutil"

	"codeberg.org/pawal/gonemaster/engine/dnsname"
	"codeberg.org/pawal/gonemaster/engine/logger"
	"codeberg.org/pawal/gonemaster/engine/nameserver"
	"codeberg.org/pawal/gonemaster/engine/nsdiscovery"
	"codeberg.org/pawal/gonemaster/engine/packet"
	"codeberg.org/pawal/gonemaster/engine/test/internal/tctest"
	"codeberg.org/pawal/gonemaster/engine/zone"
)

// ds22Answers maps "name/TYPE" to the response a test nameserver returns.
type ds22Answers map[string]packet.Packet

// ds22Key is the lookup key for one question.
func ds22Key(name string, qtype string) string {
	return strings.ToLower(strings.TrimSuffix(name, ".")) + "/" + qtype
}

// ds22Server answers from answers and counts the questions it was asked.
// A query without the DO bit loses its RRSIG records, so a testcase that
// forgets DO sees unsigned data.
func ds22Server(t *testing.T, ctx context.Context, name string, ip string, answers ds22Answers, counts map[string]int) nameserver.Nameserver {
	t.Helper()
	return tctest.NS(t, ctx, name, ip, func(q tctest.Query) packet.Packet {
		key := ds22Key(q.Name, q.Type)
		if counts != nil {
			counts[key]++
		}
		resp, ok := answers[key]
		if !ok {
			return packet.Packet{}
		}
		if q.Opts == nil || q.Opts.DNSSEC == nil || !*q.Opts.DNSSEC {
			return ds22WithoutRRSIG(resp)
		}
		return resp
	})
}

// ds22WithoutRRSIG returns resp with every RRSIG removed.
func ds22WithoutRRSIG(resp packet.Packet) packet.Packet {
	if resp.Msg == nil {
		return resp
	}
	msg := resp.Msg.Copy()
	strip := func(rrs []dns.RR) []dns.RR {
		out := make([]dns.RR, 0, len(rrs))
		for _, rr := range rrs {
			if _, ok := rr.(*dns.RRSIG); !ok {
				out = append(out, rr)
			}
		}
		return out
	}
	msg.Answer = strip(msg.Answer)
	msg.Ns = strip(msg.Ns)
	msg.Extra = strip(msg.Extra)
	return packet.Packet{Msg: msg}
}

// ds22NSEC builds an NSEC record carrying the given type bitmap.
func ds22NSEC(owner string, types ...uint16) *dns.NSEC {
	rr := &dns.NSEC{Hdr: dns.Header{Name: dnsutil.Fqdn(owner), Class: dns.ClassINET, TTL: 60}}
	rr.NextDomain = "\\000." + dnsutil.Fqdn(owner)
	rr.TypeBitMap = types
	return rr
}

// ds22NSEC3 builds the NSEC3 owned by the unsalted hash of name in zone.
func ds22NSEC3(name string, zoneName string, types ...uint16) *dns.NSEC3 {
	hash := dnsutil.NSEC3Name(dnsutil.Fqdn(name), "", 0)
	rr := &dns.NSEC3{Hdr: dns.Header{Name: hash + "." + dnsutil.Fqdn(zoneName), Class: dns.ClassINET, TTL: 60}}
	rr.Hash = dns.SHA1
	rr.Iterations = 0
	rr.Salt = ""
	rr.TypeBitMap = types
	return rr
}

// ds22Fixture is a signed zone "example" and the key of a zone cut below it.
type ds22Fixture struct {
	zone       dnsname.Name
	zoneKey    *dns.DNSKEY
	zoneSigner crypto.Signer
	cutKey     *dns.DNSKEY
	cutSigner  crypto.Signer
	answers    ds22Answers
	counts     map[string]int
}

func newDS22Fixture(t *testing.T) *ds22Fixture {
	t.Helper()
	zoneKey, zoneSigner := tctest.SignedKey(t, "example", dns.ECDSAP256SHA256, tctest.SEP())
	cutKey, cutSigner := tctest.SignedKey(t, "ns.example", dns.ECDSAP256SHA256, tctest.SEP())
	return &ds22Fixture{
		zone:       dnsname.New("example"),
		zoneKey:    zoneKey,
		zoneSigner: zoneSigner,
		cutKey:     cutKey,
		cutSigner:  cutSigner,
		answers:    ds22Answers{},
		counts:     map[string]int{},
	}
}

// secureCut publishes a valid DS and DNSKEY RRset for "ns.example".
func (f *ds22Fixture) secureCut(t *testing.T) {
	t.Helper()
	ds := f.cutKey.ToDS(dns.SHA256)
	if ds == nil {
		t.Fatal("ToDS returned nil")
	}
	ds.Hdr = dns.Header{Name: dnsutil.Fqdn("ns.example"), Class: dns.ClassINET, TTL: 60}
	dsSig := tctest.Sign(t, f.zoneKey, f.zoneSigner, dns.TypeDS, []dns.RR{ds})
	f.answers[ds22Key("ns.example", "DS")] = tctest.Response(
		tctest.Question("ns.example", dns.TypeDS), tctest.Secure(), tctest.Answers(ds, dsSig))

	keySig := tctest.Sign(t, f.cutKey, f.cutSigner, dns.TypeDNSKEY, []dns.RR{f.cutKey})
	f.answers[ds22Key("ns.example", "DNSKEY")] = tctest.Response(
		tctest.Question("ns.example", dns.TypeDNSKEY), tctest.Secure(), tctest.Answers(f.cutKey, keySig))
}

// walker returns a walker for a nameserver answering the fixture's table.
func (f *ds22Fixture) walker(t *testing.T, ctx context.Context) *dnssec22Walker {
	t.Helper()
	ns := ds22Server(t, ctx, "ns1.example", "192.0.2.10", f.answers, f.counts)
	return newDNSSEC22Walker(ns, f.zone, []*dns.DNSKEY{f.zoneKey})
}

// --- zone cut walker ---

func TestDNSSEC22CutSecure(t *testing.T) {
	ctx := tctest.Context(t)
	f := newDS22Fixture(t)
	f.secureCut(t)

	cut := f.walker(t, ctx).cut(ctx, dnsname.New("ns.example"))
	if cut.status != dnssec22SecureCut {
		t.Fatalf("status = %d, want secure", cut.status)
	}
	if len(cut.keys) != 1 || keyTag(cut.keys[0]) != keyTag(f.cutKey) {
		t.Fatal("the secure cut did not retain the DNSKEY RRset")
	}
}

func TestDNSSEC22CutInsecure(t *testing.T) {
	ctx := tctest.Context(t)
	f := newDS22Fixture(t)
	nsec := ds22NSEC("ns.example", dns.TypeNS, dns.TypeRRSIG, dns.TypeNSEC)
	f.answers[ds22Key("ns.example", "DS")] = tctest.Response(
		tctest.Question("ns.example", dns.TypeDS), tctest.Secure(), tctest.Authority(nsec))

	cut := f.walker(t, ctx).cut(ctx, dnsname.New("ns.example"))
	if cut.status != dnssec22InsecureCut {
		t.Fatalf("status = %d, want insecure", cut.status)
	}
}

func TestDNSSEC22CutNotACut(t *testing.T) {
	cases := []struct {
		name   string
		answer packet.Packet
	}{
		{"NSEC without the NS bit", tctest.Response(tctest.Secure(),
			tctest.Authority(ds22NSEC("ns.example", dns.TypeA, dns.TypeRRSIG, dns.TypeNSEC)))},
		{"NSEC3 without the NS bit", tctest.Response(tctest.Secure(),
			tctest.Authority(ds22NSEC3("ns.example", "example", dns.TypeA, dns.TypeRRSIG)))},
		{"NXDOMAIN", tctest.Response(tctest.Secure(), tctest.NXDOMAIN())},
		{"unsigned enclosing zone", tctest.Response(tctest.Secure(),
			tctest.Authority(tctest.SOARR("example")))},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := tctest.Context(t)
			f := newDS22Fixture(t)
			f.answers[ds22Key("ns.example", "DS")] = tc.answer

			cut := f.walker(t, ctx).cut(ctx, dnsname.New("ns.example"))
			if cut.status != dnssec22NotACut {
				t.Fatalf("status = %d, want not a cut", cut.status)
			}
		})
	}
}

func TestDNSSEC22CutBroken(t *testing.T) {
	t.Run("DS matches no DNSKEY", func(t *testing.T) {
		ctx := tctest.Context(t)
		f := newDS22Fixture(t)
		f.secureCut(t)
		// Republish the DNSKEY RRset with a key the DS does not name.
		other, otherSigner := tctest.SignedKey(t, "ns.example", dns.ECDSAP256SHA256, tctest.SEP())
		otherSig := tctest.Sign(t, other, otherSigner, dns.TypeDNSKEY, []dns.RR{other})
		f.answers[ds22Key("ns.example", "DNSKEY")] = tctest.Response(
			tctest.Question("ns.example", dns.TypeDNSKEY), tctest.Secure(), tctest.Answers(other, otherSig))

		cut := f.walker(t, ctx).cut(ctx, dnsname.New("ns.example"))
		if cut.status != dnssec22BrokenCut {
			t.Fatalf("status = %d, want broken", cut.status)
		}
	})

	t.Run("DS RRset unsigned", func(t *testing.T) {
		ctx := tctest.Context(t)
		f := newDS22Fixture(t)
		f.secureCut(t)
		ds := f.cutKey.ToDS(dns.SHA256)
		ds.Hdr = dns.Header{Name: dnsutil.Fqdn("ns.example"), Class: dns.ClassINET, TTL: 60}
		f.answers[ds22Key("ns.example", "DS")] = tctest.Response(
			tctest.Question("ns.example", dns.TypeDS), tctest.Secure(), tctest.Answers(ds))

		cut := f.walker(t, ctx).cut(ctx, dnsname.New("ns.example"))
		if cut.status != dnssec22BrokenCut {
			t.Fatalf("status = %d, want broken", cut.status)
		}
	})

	t.Run("DS signed outside the zone", func(t *testing.T) {
		ctx := tctest.Context(t)
		f := newDS22Fixture(t)
		f.secureCut(t)
		ds := f.cutKey.ToDS(dns.SHA256)
		ds.Hdr = dns.Header{Name: dnsutil.Fqdn("ns.example"), Class: dns.ClassINET, TTL: 60}
		foreign, foreignSigner := tctest.SignedKey(t, "other", dns.ECDSAP256SHA256, tctest.SEP())
		sig := tctest.Sign(t, foreign, foreignSigner, dns.TypeDS, []dns.RR{ds})
		f.answers[ds22Key("ns.example", "DS")] = tctest.Response(
			tctest.Question("ns.example", dns.TypeDS), tctest.Secure(), tctest.Answers(ds, sig))

		cut := f.walker(t, ctx).cut(ctx, dnsname.New("ns.example"))
		if cut.status != dnssec22BrokenCut {
			t.Fatalf("status = %d, want broken", cut.status)
		}
	})
}

func TestDNSSEC22CutIndeterminate(t *testing.T) {
	cases := []struct {
		name   string
		answer packet.Packet
	}{
		{"no response", packet.Packet{}},
		{"referral", tctest.Response(tctest.NotAuthoritative(),
			tctest.Authority(tctest.NSRR("ns.example", "a.other")))},
		{"bitmap with both NS and DS", tctest.Response(tctest.Secure(),
			tctest.Authority(ds22NSEC("ns.example", dns.TypeNS, dns.TypeDS, dns.TypeRRSIG)))},
		{"SERVFAIL", tctest.Response(tctest.Secure(), tctest.Rcode(dns.RcodeServerFailure))},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := tctest.Context(t)
			f := newDS22Fixture(t)
			f.answers[ds22Key("ns.example", "DS")] = tc.answer

			cut := f.walker(t, ctx).cut(ctx, dnsname.New("ns.example"))
			if cut.status != dnssec22Indeterminate {
				t.Fatalf("status = %d, want indeterminate", cut.status)
			}
		})
	}
}

// The zone under test is secure by the gate and is never probed as a cut.
func TestDNSSEC22CutOfTheApexIsSecureWithoutAQuery(t *testing.T) {
	ctx := tctest.Context(t)
	f := newDS22Fixture(t)

	cut := f.walker(t, ctx).cut(ctx, f.zone)
	if cut.status != dnssec22SecureCut {
		t.Fatalf("status = %d, want secure", cut.status)
	}
	if f.counts[ds22Key("example", "DS")] != 0 {
		t.Fatal("the apex was probed as a zone cut")
	}
}

// An indeterminate status is memoized like every other status.
func TestDNSSEC22CutMemoizesEveryStatus(t *testing.T) {
	ctx := tctest.Context(t)
	f := newDS22Fixture(t)
	walker := f.walker(t, ctx)

	first := walker.cut(ctx, dnsname.New("ns.example"))
	second := walker.cut(ctx, dnsname.New("ns.example"))
	if first.status != dnssec22Indeterminate || second.status != first.status {
		t.Fatalf("statuses = %d and %d, want indeterminate twice", first.status, second.status)
	}
	if got := f.counts[ds22Key("ns.example", "DS")]; got != 1 {
		t.Fatalf("DS questions = %d, want 1", got)
	}
}

func TestDNSSEC22ExpectedSignerTakesTheLowestCut(t *testing.T) {
	ctx := tctest.Context(t)
	f := newDS22Fixture(t)
	f.secureCut(t)
	f.answers[ds22Key("a.ns.example", "DS")] = tctest.Response(tctest.Secure(),
		tctest.Authority(ds22NSEC3("a.ns.example", "ns.example", dns.TypeA, dns.TypeRRSIG)))
	walker := f.walker(t, ctx)

	order := dnssec22WalkOrder(dnsname.New("a.ns.example"), f.zone)
	idx, cut := walker.expectedSigner(ctx, order, 0)
	if order[idx].String() != "ns.example" {
		t.Fatalf("expected signer = %q, want ns.example", order[idx].String())
	}
	if cut.status != dnssec22SecureCut {
		t.Fatalf("status = %d, want secure", cut.status)
	}
}

// With no zone cut below it, the expected signer is the zone apex.
func TestDNSSEC22ExpectedSignerFallsBackToTheApex(t *testing.T) {
	ctx := tctest.Context(t)
	f := newDS22Fixture(t)
	f.answers[ds22Key("ns1.example", "DS")] = tctest.Response(tctest.Secure(),
		tctest.Authority(ds22NSEC("ns1.example", dns.TypeA, dns.TypeRRSIG, dns.TypeNSEC)))
	walker := f.walker(t, ctx)

	order := dnssec22WalkOrder(dnsname.New("ns1.example"), f.zone)
	idx, cut := walker.expectedSigner(ctx, order, 0)
	if order[idx].Compare(f.zone) != 0 {
		t.Fatalf("expected signer = %q, want the apex", order[idx].String())
	}
	if cut.status != dnssec22SecureCut || len(cut.keys) != 1 {
		t.Fatal("the apex cut did not carry the zone keys")
	}
}

func TestDNSSEC22WalkOrder(t *testing.T) {
	order := dnssec22WalkOrder(dnsname.New("a.ns.example"), dnsname.New("example"))
	var got []string
	for _, name := range order {
		got = append(got, name.String())
	}
	want := []string{"a.ns.example", "ns.example", "example"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("walk order = %v, want %v", got, want)
	}
	if idx := dnssec22IndexOf(order, dnsname.New("NS.EXAMPLE")); idx != 1 {
		t.Fatalf("index = %d, want 1", idx)
	}
	if idx := dnssec22IndexOf(order, dnsname.New("other")); idx != -1 {
		t.Fatalf("index = %d, want -1", idx)
	}

	apexOnly := dnssec22WalkOrder(dnsname.New("example"), dnsname.New("example"))
	if len(apexOnly) != 1 || apexOnly[0].String() != "example" {
		t.Fatalf("apex walk order = %v, want [example]", apexOnly)
	}
}

func TestDNSSEC22Referrals(t *testing.T) {
	ctx := tctest.Context(t)
	f := newDS22Fixture(t)
	walker := f.walker(t, ctx)

	referral := tctest.Response(tctest.NotAuthoritative(),
		tctest.Authority(tctest.NSRR("sub.example", "a.other")))
	walker.noteReferral(referral)

	if !walker.referred(dnsname.New("ns1.sub.example")) {
		t.Error("a name below the referred cut was not skipped")
	}
	if !walker.referred(dnsname.New("sub.example")) {
		t.Error("the referred cut itself was not skipped")
	}
	if walker.referred(dnsname.New("ns1.example")) {
		t.Error("a name outside the referred subtree was skipped")
	}
}

func TestDNSSEC22InDomainNames(t *testing.T) {
	names := dnssec22InDomainNames(dnsname.New("example"),
		tctest.NSItems("ns1.example/192.0.2.1", "NS1.EXAMPLE/192.0.2.2", "ns.other/192.0.2.3"),
		tctest.NSItems("example/192.0.2.4", "ns2.example/192.0.2.5"))

	var got []string
	for _, name := range names {
		got = append(got, name.String())
	}
	want := []string{"example", "ns1.example", "ns2.example"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("names = %v, want %v", got, want)
	}

	// Every name is subordinate to the root, so the root zone has none.
	if rootNames := dnssec22InDomainNames(dnsname.New("."), tctest.NSItems("a.root-servers.net/192.0.2.1")); rootNames != nil {
		t.Fatalf("root name set = %v, want none", rootNames)
	}
}

// --- testcase ---

// ds22Run wires the fixture into DNSSEC22 and runs it.
func (f *ds22Fixture) run(t *testing.T, ctx context.Context, nsNames ...string) []*logger.Entry {
	t.Helper()
	items := make([]nsdiscovery.NSItem, 0, len(nsNames))
	for _, name := range nsNames {
		items = append(items, nsdiscovery.NSItem{
			Name: dnsname.New(name), Address: netip.MustParseAddr("192.0.2.10"), HasAddress: true,
		})
	}
	tctest.Stub(t, &delegationNameservers, func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
		return items, nil
	})
	tctest.Stub(t, &zoneNameservers, func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
		return nil, nil
	})

	z, err := zone.New("example")
	if err != nil {
		t.Fatalf("zone new: %v", err)
	}
	entries, err := DNSSEC22(ctx, &z)
	if err != nil {
		t.Fatalf("DNSSEC22: %v", err)
	}
	return entries
}

// apexDNSKEY publishes the signed apex DNSKEY RRset the gate reads.
func (f *ds22Fixture) apexDNSKEY(t *testing.T) {
	t.Helper()
	sig := tctest.Sign(t, f.zoneKey, f.zoneSigner, dns.TypeDNSKEY, []dns.RR{f.zoneKey})
	f.answers[ds22Key("example", "DNSKEY")] = tctest.Response(
		tctest.Question("example", dns.TypeDNSKEY), tctest.Secure(), tctest.Answers(f.zoneKey, sig))
}

// parentDS publishes a DS for the zone on a stubbed parent nameserver.
func (f *ds22Fixture) parentDS(t *testing.T, ctx context.Context, present bool) {
	t.Helper()
	answers := ds22Answers{}
	if present {
		ds := f.zoneKey.ToDS(dns.SHA256)
		ds.Hdr = dns.Header{Name: dnsutil.Fqdn("example"), Class: dns.ClassINET, TTL: 60}
		answers[ds22Key("example", "DS")] = tctest.Response(
			tctest.Question("example", dns.TypeDS), tctest.Secure(), tctest.Answers(ds))
	}
	parent := ds22Server(t, ctx, "ns1.parent", "192.0.2.99", answers, nil)
	tctest.Stub(t, &parentNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{parent}, nil
	})
}

// signedAddress publishes a signed A RRset for name.
func (f *ds22Fixture) signedAddress(t *testing.T, name string, key *dns.DNSKEY, signer crypto.Signer) {
	t.Helper()
	addr := tctest.ARR(name, "192.0.2.10")
	sig := tctest.Sign(t, key, signer, dns.TypeA, []dns.RR{addr})
	f.answers[ds22Key(name, "A")] = tctest.Response(
		tctest.Question(name, dns.TypeA), tctest.Secure(), tctest.Answers(addr, sig))
}

func TestDNSSEC22Validates(t *testing.T) {
	ctx := tctest.Context(t)
	f := newDS22Fixture(t)
	f.apexDNSKEY(t)
	f.parentDS(t, ctx, true)
	f.signedAddress(t, "ns1.example", f.zoneKey, f.zoneSigner)
	ds22Server(t, ctx, "ns1.example", "192.0.2.10", f.answers, f.counts)

	entries := f.run(t, ctx, "ns1.example")
	tctest.RequireTags(t, entries, "DS22_NS_ADDRESS_VALIDATES")
	tctest.RequireNoTag(t, entries, "DS22_NS_ADDRESS_ORPHAN_ZONE", "DS22_NS_ADDRESS_UNSIGNED",
		"DS22_ZONE_NOT_SECURE", "DS22_NO_IN_DOMAIN_NS")
	// The signer is the apex, so no zone cut is probed and A settles the name.
	if got := f.counts[ds22Key("ns1.example", "A")]; got != 1 {
		t.Fatalf("A questions = %d, want 1", got)
	}
	if got := f.counts[ds22Key("ns1.example", "AAAA")]; got != 0 {
		t.Fatalf("AAAA questions = %d, want 0", got)
	}
}

// The address records of a name the nameserver serves as a zone apex, with no
// delegation proven for it, are bogus for every validator.
func TestDNSSEC22OrphanZone(t *testing.T) {
	ctx := tctest.Context(t)
	f := newDS22Fixture(t)
	f.apexDNSKEY(t)
	f.parentDS(t, ctx, true)
	f.secureCut(t)

	orphanKey, orphanSigner := tctest.SignedKey(t, "a.ns.example", dns.ECDSAP256SHA256, tctest.SEP())
	f.signedAddress(t, "a.ns.example", orphanKey, orphanSigner)
	f.answers[ds22Key("a.ns.example", "DS")] = tctest.Response(
		tctest.Question("a.ns.example", dns.TypeDS), tctest.Secure(),
		tctest.Authority(ds22NSEC3("a.ns.example", "ns.example", dns.TypeA, dns.TypeRRSIG)))
	ds22Server(t, ctx, "a.ns.example", "192.0.2.10", f.answers, f.counts)

	entries := f.run(t, ctx, "a.ns.example")
	entry := tctest.RequireTag(t, entries, "DS22_NS_ADDRESS_ORPHAN_ZONE")
	if signer, _ := entry.Args["signer"].(string); signer != "a.ns.example" {
		t.Fatalf("signer = %q, want a.ns.example", signer)
	}
	if ns, _ := entry.Args["ns"].(string); ns != "a.ns.example" {
		t.Fatalf("ns = %q, want a.ns.example", ns)
	}
	tctest.RequireNoTag(t, entries, "DS22_NS_ADDRESS_VALIDATES")
	// One leaf query, the two zone cut probes of the walk, and one DNSKEY.
	for _, want := range []string{"a.ns.example/A", "a.ns.example/DS", "ns.example/DS", "ns.example/DNSKEY"} {
		if got := f.counts[want]; got != 1 {
			t.Fatalf("%s questions = %d, want 1", want, got)
		}
	}
	if got := f.counts[ds22Key("a.ns.example", "AAAA")]; got != 0 {
		t.Fatalf("AAAA questions = %d, want 0", got)
	}
}

func TestDNSSEC22ZoneNotSecure(t *testing.T) {
	t.Run("no apex DNSKEY", func(t *testing.T) {
		ctx := tctest.Context(t)
		f := newDS22Fixture(t)
		f.parentDS(t, ctx, true)
		ds22Server(t, ctx, "ns1.example", "192.0.2.10", f.answers, f.counts)

		entries := f.run(t, ctx, "ns1.example")
		tctest.RequireTags(t, entries, "DS22_ZONE_NOT_SECURE")
		if got := f.counts[ds22Key("ns1.example", "A")]; got != 0 {
			t.Fatalf("A questions = %d, want 0", got)
		}
	})

	t.Run("no parent DS", func(t *testing.T) {
		ctx := tctest.Context(t)
		f := newDS22Fixture(t)
		f.apexDNSKEY(t)
		f.parentDS(t, ctx, false)
		ds22Server(t, ctx, "ns1.example", "192.0.2.10", f.answers, f.counts)

		entries := f.run(t, ctx, "ns1.example")
		tctest.RequireTags(t, entries, "DS22_ZONE_NOT_SECURE")
	})
}

func TestDNSSEC22NoInDomainNS(t *testing.T) {
	ctx := tctest.Context(t)
	f := newDS22Fixture(t)
	f.apexDNSKEY(t)
	ds22Server(t, ctx, "ns1.other", "192.0.2.10", f.answers, f.counts)

	entries := f.run(t, ctx, "ns1.other")
	tctest.RequireTags(t, entries, "DS22_NO_IN_DOMAIN_NS")
	tctest.RequireNoTag(t, entries, "DS22_ZONE_NOT_SECURE")
	if got := f.counts[ds22Key("example", "DNSKEY")]; got != 0 {
		t.Fatalf("DNSKEY questions = %d, want 0", got)
	}
}
