package dnssec

import (
	"context"
	"crypto"
	"strings"
	"testing"
	"time"

	dns "codeberg.org/miekg/dns"
	"codeberg.org/miekg/dns/dnsutil"

	"codeberg.org/pawal/gonemaster/engine/dnsname"
	"codeberg.org/pawal/gonemaster/engine/dnssecchain"
	"codeberg.org/pawal/gonemaster/engine/logger"
	"codeberg.org/pawal/gonemaster/engine/nameserver"
	"codeberg.org/pawal/gonemaster/engine/nsdiscovery"
	"codeberg.org/pawal/gonemaster/engine/packet"
	"codeberg.org/pawal/gonemaster/engine/profile"
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
	parentIP   string
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
		parentIP:   "192.0.2.99",
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

// run wires the fixture into DNSSEC22 and runs it on zone "example". Each spec
// is "name" or "name/address"; the address defaults to 192.0.2.10.
func (f *ds22Fixture) run(t *testing.T, ctx context.Context, specs ...string) []*logger.Entry {
	t.Helper()
	return f.runZone(t, ctx, "example", specs...)
}

// runZone runs DNSSEC22 on another zone than the fixture's.
func (f *ds22Fixture) runZone(t *testing.T, ctx context.Context, zoneName string, specs ...string) []*logger.Entry {
	t.Helper()
	items := make([]nsdiscovery.NSItem, 0, len(specs))
	for _, spec := range specs {
		name, address, found := strings.Cut(spec, "/")
		if !found {
			address = "192.0.2.10"
		}
		items = append(items, tctest.NSItem(name, address))
	}
	tctest.Stub(t, &delegationNameservers, func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
		return items, nil
	})
	tctest.Stub(t, &zoneNameservers, func(_ context.Context, _ *zone.Zone) ([]nsdiscovery.NSItem, error) {
		return nil, nil
	})

	z, err := zone.New(zoneName)
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
	parent := ds22Server(t, ctx, "ns1.parent", f.parentIP, answers, nil)
	tctest.Stub(t, &parentNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{parent}, nil
	})
}

// signedAddress publishes an A RRset for name, signed with the given options.
func (f *ds22Fixture) signedAddress(t *testing.T, name string, key *dns.DNSKEY, signer crypto.Signer, opts ...tctest.SigOpt) {
	t.Helper()
	addr := tctest.ARR(name, "192.0.2.10")
	sig := tctest.Sign(t, key, signer, dns.TypeA, []dns.RR{addr}, opts...)
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

// copyAnswers returns a copy of the answer table.
func (f *ds22Fixture) copyAnswers() ds22Answers {
	out := ds22Answers{}
	for key, resp := range f.answers {
		out[key] = resp
	}
	return out
}

// into returns the fixture writing into another table, for a second nameserver
// that answers differently.
func (f *ds22Fixture) into(answers ds22Answers) *ds22Fixture {
	clone := *f
	clone.answers = answers
	return &clone
}

// noDataOnA publishes an authoritative NODATA for the A question of name, with
// a signed NSEC denial.
func (f *ds22Fixture) noDataOnA(t *testing.T, name string) {
	t.Helper()
	nsec := ds22NSEC(name, dns.TypeAAAA, dns.TypeRRSIG, dns.TypeNSEC)
	sig := tctest.Sign(t, f.zoneKey, f.zoneSigner, dns.TypeNSEC, []dns.RR{nsec})
	f.answers[ds22Key(name, "A")] = tctest.Response(
		tctest.Question(name, dns.TypeA), tctest.Secure(),
		tctest.Authority(tctest.SOARR("example"), nsec, sig))
}

// unsignedAddress publishes an A RRset with no RRSIG.
func (f *ds22Fixture) unsignedAddress(t *testing.T, name string) {
	t.Helper()
	f.answers[ds22Key(name, "A")] = tctest.Response(
		tctest.Question(name, dns.TypeA), tctest.Secure(), tctest.Answers(tctest.ARR(name, "192.0.2.10")))
}

// denyDS publishes an authoritative DS NODATA carrying the given authority records.
func (f *ds22Fixture) denyDS(name string, authority ...dns.RR) {
	f.answers[ds22Key(name, "DS")] = tctest.Response(
		tctest.Question(name, dns.TypeDS), tctest.Secure(), tctest.Authority(authority...))
}

// orphan publishes an A RRset for name signed by a key of name itself, with the
// enclosing zone proving no delegation there.
func (f *ds22Fixture) orphan(t *testing.T, name string, denial ...dns.RR) {
	t.Helper()
	key, signer := tctest.SignedKey(t, name, dns.ECDSAP256SHA256, tctest.SEP())
	f.signedAddress(t, name, key, signer)
	f.denyDS(name, denial...)
}

// requireArg fails unless the entry carries the expected string argument.
func requireArg(t *testing.T, entry *logger.Entry, key string, want string) {
	t.Helper()
	if got, _ := entry.Args[key].(string); got != want {
		t.Fatalf("%s = %q, want %q", key, got, want)
	}
}

// The enclosing zone denies the delegation with an NSEC record.
func TestDNSSEC22OrphanZoneNSECParent(t *testing.T) {
	ctx := tctest.Context(t)
	f := newDS22Fixture(t)
	f.apexDNSKEY(t)
	f.parentDS(t, ctx, true)
	f.secureCut(t)
	f.orphan(t, "a.ns.example", ds22NSEC("a.ns.example", dns.TypeA, dns.TypeRRSIG, dns.TypeNSEC))
	ds22Server(t, ctx, "a.ns.example", "192.0.2.10", f.answers, f.counts)

	entries := f.run(t, ctx, "a.ns.example")
	entry := tctest.RequireTag(t, entries, "DS22_NS_ADDRESS_ORPHAN_ZONE")
	requireArg(t, entry, "signer", "a.ns.example")
}

// An NXDOMAIN for the DS question proves no delegation either.
func TestDNSSEC22OrphanZoneNXDOMAINParent(t *testing.T) {
	ctx := tctest.Context(t)
	f := newDS22Fixture(t)
	f.apexDNSKEY(t)
	f.parentDS(t, ctx, true)
	f.secureCut(t)
	f.orphan(t, "a.ns.example")
	f.answers[ds22Key("a.ns.example", "DS")] = tctest.Response(
		tctest.Question("a.ns.example", dns.TypeDS), tctest.Secure(), tctest.NXDOMAIN())
	ds22Server(t, ctx, "a.ns.example", "192.0.2.10", f.answers, f.counts)

	entries := f.run(t, ctx, "a.ns.example")
	entry := tctest.RequireTag(t, entries, "DS22_NS_ADDRESS_ORPHAN_ZONE")
	requireArg(t, entry, "signer", "a.ns.example")
}

// A nameserver name below a secure zone cut is signed by that zone cut.
func TestDNSSEC22SecureZoneCutValidates(t *testing.T) {
	ctx := tctest.Context(t)
	f := newDS22Fixture(t)
	f.apexDNSKEY(t)
	f.parentDS(t, ctx, true)
	f.secureCut(t)
	f.signedAddress(t, "b.ns.example", f.cutKey, f.cutSigner)
	ds22Server(t, ctx, "b.ns.example", "192.0.2.10", f.answers, f.counts)

	entries := f.run(t, ctx, "b.ns.example")
	tctest.RequireTags(t, entries, "DS22_NS_ADDRESS_VALIDATES")
	tctest.RequireNoTag(t, entries, "DS22_NS_ADDRESS_ORPHAN_ZONE", "DS22_NS_ADDRESS_CHAIN_BROKEN")
	// The signer names the zone cut, so the name itself is never probed as one.
	if got := f.counts[ds22Key("b.ns.example", "DS")]; got != 0 {
		t.Fatalf("DS questions for the name = %d, want 0", got)
	}
}

// Below an insecure delegation the address records are unsigned by design.
func TestDNSSEC22InsecureDelegation(t *testing.T) {
	ctx := tctest.Context(t)
	f := newDS22Fixture(t)
	f.apexDNSKEY(t)
	f.parentDS(t, ctx, true)
	f.unsignedAddress(t, "ns1.sub.example")
	f.denyDS("ns1.sub.example", tctest.SOARR("sub.example"))
	f.denyDS("sub.example", ds22NSEC("sub.example", dns.TypeNS, dns.TypeRRSIG, dns.TypeNSEC))
	ds22Server(t, ctx, "ns1.sub.example", "192.0.2.10", f.answers, f.counts)

	entries := f.run(t, ctx, "ns1.sub.example")
	entry := tctest.RequireTag(t, entries, "DS22_NS_ADDRESS_INSECURE")
	requireArg(t, entry, "ns", "ns1.sub.example")
	tctest.RequireNoTag(t, entries, "DS22_NS_ADDRESS_UNSIGNED", "DS22_NS_ADDRESS_VALIDATES")
}

// Unsigned address records in the signed zone itself are bogus.
func TestDNSSEC22Unsigned(t *testing.T) {
	ctx := tctest.Context(t)
	f := newDS22Fixture(t)
	f.apexDNSKEY(t)
	f.parentDS(t, ctx, true)
	f.unsignedAddress(t, "ns1.example")
	f.denyDS("ns1.example", ds22NSEC("ns1.example", dns.TypeA, dns.TypeRRSIG, dns.TypeNSEC))
	ds22Server(t, ctx, "ns1.example", "192.0.2.10", f.answers, f.counts)

	entries := f.run(t, ctx, "ns1.example")
	entry := tctest.RequireTag(t, entries, "DS22_NS_ADDRESS_UNSIGNED")
	requireArg(t, entry, "ns", "ns1.example")
	// A settles a name at fault; AAAA is never asked for it.
	if got := f.counts[ds22Key("ns1.example", "AAAA")]; got != 0 {
		t.Fatalf("AAAA questions = %d, want 0", got)
	}
}

func TestDNSSEC22LeafSignatureFaults(t *testing.T) {
	t.Run("expired", func(t *testing.T) {
		ctx := tctest.Context(t)
		f := newDS22Fixture(t)
		f.apexDNSKEY(t)
		f.parentDS(t, ctx, true)
		f.signedAddress(t, "ns1.example", f.zoneKey, f.zoneSigner,
			tctest.Inception(time.Now().Add(-48*time.Hour)), tctest.Expiration(time.Now().Add(-time.Hour)))
		ds22Server(t, ctx, "ns1.example", "192.0.2.10", f.answers, f.counts)

		entries := f.run(t, ctx, "ns1.example")
		entry := tctest.RequireTag(t, entries, "DS22_NS_ADDRESS_RRSIG_EXPIRED")
		requireArg(t, entry, "ns", "ns1.example")
		if entry.Args["keytag"] != keyTag(f.zoneKey) {
			t.Fatalf("keytag = %#v, want %d", entry.Args["keytag"], keyTag(f.zoneKey))
		}
		tctest.RequireNoTag(t, entries, "DS22_NS_ADDRESS_VALIDATES")
	})

	t.Run("no DNSKEY with the keytag", func(t *testing.T) {
		ctx := tctest.Context(t)
		f := newDS22Fixture(t)
		f.apexDNSKEY(t)
		f.parentDS(t, ctx, true)
		// A key of the apex that the DNSKEY RRset does not publish.
		other, otherSigner := tctest.SignedKey(t, "example", dns.ECDSAP256SHA256, tctest.SEP())
		f.signedAddress(t, "ns1.example", other, otherSigner)
		ds22Server(t, ctx, "ns1.example", "192.0.2.10", f.answers, f.counts)

		entries := f.run(t, ctx, "ns1.example")
		entry := tctest.RequireTag(t, entries, "DS22_NS_ADDRESS_RRSIG_NOT_VALID_BY_DNSKEY")
		requireArg(t, entry, "signer", "example")
		if entry.Args["keytag"] != keyTag(other) {
			t.Fatalf("keytag = %#v, want %d", entry.Args["keytag"], keyTag(other))
		}
	})

	t.Run("signature does not verify", func(t *testing.T) {
		ctx := tctest.Context(t)
		f := newDS22Fixture(t)
		f.apexDNSKEY(t)
		f.parentDS(t, ctx, true)
		// Another key signs under the keytag of the published one.
		other, otherSigner := tctest.SignedKey(t, "example", dns.ECDSAP256SHA256, tctest.SEP())
		f.signedAddress(t, "ns1.example", other, otherSigner, tctest.KeyTag(keyTag(f.zoneKey)))
		ds22Server(t, ctx, "ns1.example", "192.0.2.10", f.answers, f.counts)

		entries := f.run(t, ctx, "ns1.example")
		entry := tctest.RequireTag(t, entries, "DS22_NS_ADDRESS_RRSIG_NOT_VALID_BY_DNSKEY")
		if entry.Args["keytag"] != keyTag(f.zoneKey) {
			t.Fatalf("keytag = %#v, want %d", entry.Args["keytag"], keyTag(f.zoneKey))
		}
	})

	t.Run("signer outside the zone", func(t *testing.T) {
		ctx := tctest.Context(t)
		f := newDS22Fixture(t)
		f.apexDNSKEY(t)
		f.parentDS(t, ctx, true)
		foreign, foreignSigner := tctest.SignedKey(t, "other", dns.ECDSAP256SHA256, tctest.SEP())
		f.signedAddress(t, "ns1.example", foreign, foreignSigner)
		f.denyDS("ns1.example", ds22NSEC("ns1.example", dns.TypeA, dns.TypeRRSIG, dns.TypeNSEC))
		ds22Server(t, ctx, "ns1.example", "192.0.2.10", f.answers, f.counts)

		entries := f.run(t, ctx, "ns1.example")
		entry := tctest.RequireTag(t, entries, "DS22_NS_ADDRESS_RRSIG_NOT_VALID_BY_DNSKEY")
		requireArg(t, entry, "signer", "other")
	})
}

// A zone cut whose own chain is broken carries every name below it with it.
func TestDNSSEC22ChainBroken(t *testing.T) {
	ctx := tctest.Context(t)
	f := newDS22Fixture(t)
	f.apexDNSKEY(t)
	f.parentDS(t, ctx, true)
	f.secureCut(t)
	// Republish the DNSKEY RRset of the zone cut with a key its DS does not name.
	other, otherSigner := tctest.SignedKey(t, "ns.example", dns.ECDSAP256SHA256, tctest.SEP())
	otherSig := tctest.Sign(t, other, otherSigner, dns.TypeDNSKEY, []dns.RR{other})
	f.answers[ds22Key("ns.example", "DNSKEY")] = tctest.Response(
		tctest.Question("ns.example", dns.TypeDNSKEY), tctest.Secure(), tctest.Answers(other, otherSig))
	f.signedAddress(t, "a.ns.example", f.cutKey, f.cutSigner)
	ds22Server(t, ctx, "a.ns.example", "192.0.2.10", f.answers, f.counts)

	entries := f.run(t, ctx, "a.ns.example")
	entry := tctest.RequireTag(t, entries, "DS22_NS_ADDRESS_CHAIN_BROKEN")
	requireArg(t, entry, "ns", "a.ns.example")
	requireArg(t, entry, "signer", "ns.example")
	tctest.RequireNoTag(t, entries, "DS22_NS_ADDRESS_VALIDATES")
}

// Each nameserver is evaluated on its own; the finding names only its servers.
func TestDNSSEC22PerServerDisagreement(t *testing.T) {
	ctx := tctest.Context(t)
	f := newDS22Fixture(t)
	f.apexDNSKEY(t)
	f.parentDS(t, ctx, true)
	f.orphan(t, "ns1.example", ds22NSEC("ns1.example", dns.TypeA, dns.TypeRRSIG, dns.TypeNSEC))
	ds22Server(t, ctx, "ns1.example", "192.0.2.10", f.answers, f.counts)

	healthy := f.copyAnswers()
	f.into(healthy).signedAddress(t, "ns1.example", f.zoneKey, f.zoneSigner)
	ds22Server(t, ctx, "ns1.example", "192.0.2.11", healthy, nil)

	entries := f.run(t, ctx, "ns1.example/192.0.2.10", "ns1.example/192.0.2.11")
	entry := tctest.RequireTag(t, entries, "DS22_NS_ADDRESS_ORPHAN_ZONE")
	if got := tctest.ServerEndpoints(t, entry.Args); len(got) != 1 || got[0] != "ns1.example/192.0.2.10" {
		t.Fatalf("servers = %v, want the orphaned nameserver alone", got)
	}
	tctest.RequireNoTag(t, entries, "DS22_NS_ADDRESS_VALIDATES")
}

// A signature the local verifier cannot process is indeterminate.
func TestDNSSEC22UnsupportedAlgorithm(t *testing.T) {
	ctx := tctest.Context(t)
	f := newDS22Fixture(t)
	f.apexDNSKEY(t)
	f.parentDS(t, ctx, true)
	addr := tctest.ARR("ns1.example", "192.0.2.10")
	sig := tctest.RRSIGRR("ns1.example", dns.TypeA, tctest.SigAlgo(dns.DSA),
		tctest.Signer("example"), tctest.KeyTag(keyTag(f.zoneKey)))
	f.answers[ds22Key("ns1.example", "A")] = tctest.Response(
		tctest.Question("ns1.example", dns.TypeA), tctest.Secure(), tctest.Answers(addr, sig))
	ds22Server(t, ctx, "ns1.example", "192.0.2.10", f.answers, f.counts)

	entries := f.run(t, ctx, "ns1.example")
	if tags := tctest.TagsWithPrefix(entries, "DS22_"); len(tags) != 0 {
		t.Fatalf("tags = %v, want none", tags)
	}
}

func TestDNSSEC22TransportDisabled(t *testing.T) {
	t.Run("IPv6 disabled", func(t *testing.T) {
		ctx := tctest.Context(t)
		f := newDS22Fixture(t)
		if err := profile.Effective().Set("net.ipv6", false); err != nil {
			t.Fatalf("set net.ipv6: %v", err)
		}
		f.apexDNSKEY(t)
		f.parentDS(t, ctx, true)
		f.signedAddress(t, "ns1.example", f.zoneKey, f.zoneSigner)
		f.signedAddress(t, "ns2.example", f.zoneKey, f.zoneSigner)
		ds22Server(t, ctx, "ns1.example", "192.0.2.10", f.answers, f.counts)
		ds22Server(t, ctx, "ns2.example", "2001:db8::1", f.answers, nil)

		entries := f.run(t, ctx, "ns1.example/192.0.2.10", "ns2.example/2001:db8::1")
		entry := tctest.RequireTag(t, entries, "IPV6_DISABLED")
		tctest.RequireArgShape(t, entry, tctest.ArgShape{NS: "ns2.example", Address: "2001:db8::1"})
		requireArg(t, entry, "query_type", "A")
	})

	t.Run("IPv4 disabled", func(t *testing.T) {
		ctx := tctest.Context(t)
		f := newDS22Fixture(t)
		f.parentIP = "2001:db8::99"
		if err := profile.Effective().Set("net.ipv4", false); err != nil {
			t.Fatalf("set net.ipv4: %v", err)
		}
		f.apexDNSKEY(t)
		f.parentDS(t, ctx, true)
		f.signedAddress(t, "ns1.example", f.zoneKey, f.zoneSigner)
		f.signedAddress(t, "ns2.example", f.zoneKey, f.zoneSigner)
		ds22Server(t, ctx, "ns1.example", "2001:db8::1", f.answers, f.counts)
		ds22Server(t, ctx, "ns2.example", "192.0.2.20", f.answers, nil)

		entries := f.run(t, ctx, "ns1.example/2001:db8::1", "ns2.example/192.0.2.20")
		entry := tctest.RequireTag(t, entries, "IPV4_DISABLED")
		tctest.RequireArgShape(t, entry, tctest.ArgShape{NS: "ns2.example", Address: "192.0.2.20"})
		requireArg(t, entry, "query_type", "A")
	})
}

// A name with no A RRset is settled by AAAA, or by the denial of both.
func TestDNSSEC22NoDataOnA(t *testing.T) {
	t.Run("AAAA RRset", func(t *testing.T) {
		ctx := tctest.Context(t)
		f := newDS22Fixture(t)
		f.apexDNSKEY(t)
		f.parentDS(t, ctx, true)
		f.noDataOnA(t, "ns1.example")
		addr := tctest.AAAARR("ns1.example", "2001:db8::1")
		sig := tctest.Sign(t, f.zoneKey, f.zoneSigner, dns.TypeAAAA, []dns.RR{addr})
		f.answers[ds22Key("ns1.example", "AAAA")] = tctest.Response(
			tctest.Question("ns1.example", dns.TypeAAAA), tctest.Secure(), tctest.Answers(addr, sig))
		ds22Server(t, ctx, "ns1.example", "192.0.2.10", f.answers, f.counts)

		entries := f.run(t, ctx, "ns1.example")
		tctest.RequireTags(t, entries, "DS22_NS_ADDRESS_VALIDATES")
		if got := f.counts[ds22Key("ns1.example", "AAAA")]; got != 1 {
			t.Fatalf("AAAA questions = %d, want 1", got)
		}
	})

	t.Run("neither type", func(t *testing.T) {
		ctx := tctest.Context(t)
		f := newDS22Fixture(t)
		f.apexDNSKEY(t)
		f.parentDS(t, ctx, true)
		f.noDataOnA(t, "ns1.example")
		ds22Server(t, ctx, "ns1.example", "192.0.2.10", f.answers, f.counts)

		// The signer comes from the denial proof of the A question.
		entries := f.run(t, ctx, "ns1.example")
		tctest.RequireTags(t, entries, "DS22_NS_ADDRESS_VALIDATES")
	})
}

// A delegated name whose NSEC3 omits the NS bit is an orphan from this side.
func TestDNSSEC22DelegatedNameWithoutTheNSBit(t *testing.T) {
	ctx := tctest.Context(t)
	f := newDS22Fixture(t)
	f.apexDNSKEY(t)
	f.parentDS(t, ctx, true)
	f.secureCut(t)
	key, signer := tctest.SignedKey(t, "a.ns.example", dns.ECDSAP256SHA256, tctest.SEP())
	f.signedAddress(t, "a.ns.example", key, signer)
	f.denyDS("a.ns.example", tctest.SOARR("ns.example"), tctest.NSRR("a.ns.example", "ns1.other"),
		ds22NSEC3("a.ns.example", "ns.example", dns.TypeA, dns.TypeRRSIG))
	ds22Server(t, ctx, "a.ns.example", "192.0.2.10", f.answers, f.counts)

	entries := f.run(t, ctx, "a.ns.example")
	entry := tctest.RequireTag(t, entries, "DS22_NS_ADDRESS_ORPHAN_ZONE")
	requireArg(t, entry, "signer", "a.ns.example")
}

// A referral costs one query and takes the whole subtree with it.
func TestDNSSEC22ReferralShortCircuit(t *testing.T) {
	ctx := tctest.Context(t)
	f := newDS22Fixture(t)
	f.apexDNSKEY(t)
	f.parentDS(t, ctx, true)
	referral := tctest.Response(tctest.Question("ns1.sub.example", dns.TypeA), tctest.NotAuthoritative(),
		tctest.Authority(tctest.NSRR("sub.example", "ns1.other")))
	f.answers[ds22Key("ns1.sub.example", "A")] = referral
	ds22Server(t, ctx, "ns1.sub.example", "192.0.2.10", f.answers, f.counts)

	entries := f.run(t, ctx, "ns1.sub.example", "ns2.sub.example", "ns3.sub.example")
	if tags := tctest.TagsWithPrefix(entries, "DS22_"); len(tags) != 0 {
		t.Fatalf("tags = %v, want none", tags)
	}
	if got := f.counts[ds22Key("ns1.sub.example", "A")]; got != 1 {
		t.Fatalf("A questions = %d, want 1", got)
	}
	for _, name := range []string{"ns2.sub.example", "ns3.sub.example"} {
		if got := f.counts[ds22Key(name, "A")]; got != 0 {
			t.Fatalf("%s A questions = %d, want 0", name, got)
		}
	}
}

// A referral for one subtree leaves the names outside it evaluated.
func TestDNSSEC22ReferralKeepsOtherNames(t *testing.T) {
	ctx := tctest.Context(t)
	f := newDS22Fixture(t)
	f.apexDNSKEY(t)
	f.parentDS(t, ctx, true)
	f.orphan(t, "ns1.example", ds22NSEC("ns1.example", dns.TypeA, dns.TypeRRSIG, dns.TypeNSEC))
	f.answers[ds22Key("ns1.sub.example", "A")] = tctest.Response(
		tctest.Question("ns1.sub.example", dns.TypeA), tctest.NotAuthoritative(),
		tctest.Authority(tctest.NSRR("sub.example", "ns1.other")))
	ds22Server(t, ctx, "ns1.example", "192.0.2.10", f.answers, f.counts)

	entries := f.run(t, ctx, "ns1.example", "ns1.sub.example")
	entry := tctest.RequireTag(t, entries, "DS22_NS_ADDRESS_ORPHAN_ZONE")
	requireArg(t, entry, "ns", "ns1.example")
}

// Every name is subordinate to the root, so the root zone has no name set.
func TestDNSSEC22RootZone(t *testing.T) {
	ctx := tctest.Context(t)
	f := newDS22Fixture(t)
	ds22Server(t, ctx, "a.root-servers.net", "192.0.2.10", f.answers, f.counts)

	entries := f.runZone(t, ctx, ".", "a.root-servers.net/192.0.2.10")
	tctest.RequireTags(t, entries, "DS22_NO_IN_DOMAIN_NS")
	if len(f.counts) != 0 {
		t.Fatalf("questions = %v, want none", f.counts)
	}
}

// A zone cut the nameserver says nothing about yields no finding.
func TestDNSSEC22IndeterminateCut(t *testing.T) {
	ctx := tctest.Context(t)
	f := newDS22Fixture(t)
	f.apexDNSKEY(t)
	f.parentDS(t, ctx, true)
	f.signedAddress(t, "a.ns.example", f.cutKey, f.cutSigner)
	f.signedAddress(t, "b.ns.example", f.cutKey, f.cutSigner)

	// The second nameserver answers the zone cut question, the first does not.
	sound := f.copyAnswers()
	f.into(sound).secureCut(t)
	ds22Server(t, ctx, "a.ns.example", "192.0.2.10", f.answers, f.counts)
	ds22Server(t, ctx, "a.ns.example", "192.0.2.11", sound, nil)

	entries := f.run(t, ctx, "a.ns.example/192.0.2.10", "b.ns.example/192.0.2.10",
		"a.ns.example/192.0.2.11", "b.ns.example/192.0.2.11")
	entry := tctest.RequireTag(t, entries, "DS22_NS_ADDRESS_VALIDATES")
	if got := tctest.ServerEndpoints(t, entry.Args); len(got) != 2 || got[0] != "a.ns.example/192.0.2.11" {
		t.Fatalf("servers = %v, want the answering nameserver alone", got)
	}
	tctest.RequireNoTag(t, entries, "DS22_NS_ADDRESS_ORPHAN_ZONE",
		"DS22_NS_ADDRESS_RRSIG_NOT_VALID_BY_DNSKEY", "DS22_NS_ADDRESS_CHAIN_BROKEN")
	// The memo asks the question once for both names below the zone cut.
	if got := f.counts[ds22Key("ns.example", "DS")]; got != 1 {
		t.Fatalf("DS questions = %d, want 1", got)
	}
}

// --- chain document ---

// collected returns the chain-document entry for name, or fails.
func collected(t *testing.T, ctx context.Context, name string) dnssecchain.NSName {
	t.Helper()
	for _, entry := range dnssecchain.NSNamesFromContext(ctx) {
		if entry.Name == name {
			return entry
		}
	}
	t.Fatalf("no collected entry for %q in %+v", name, dnssecchain.NSNamesFromContext(ctx))
	return dnssecchain.NSName{}
}

// The chain document learns the orphan the tag reports, with the same signer.
func TestDNSSEC22CollectsOrphanZone(t *testing.T) {
	ctx := dnssecchain.WithNSNames(tctest.Context(t))
	f := newDS22Fixture(t)
	f.apexDNSKEY(t)
	f.parentDS(t, ctx, true)
	f.secureCut(t)
	f.orphan(t, "a.ns.example")
	ds22Server(t, ctx, "a.ns.example", "192.0.2.10", f.answers, f.counts)

	entries := f.run(t, ctx, "a.ns.example")
	tctest.RequireTag(t, entries, "DS22_NS_ADDRESS_ORPHAN_ZONE")

	got := collected(t, ctx, "a.ns.example")
	if got.Status != dnssecchain.NSNameOrphan {
		t.Errorf("status = %q, want %q", got.Status, dnssecchain.NSNameOrphan)
	}
	if got.Signer != "a.ns.example" {
		t.Errorf("signer = %q, want a.ns.example", got.Signer)
	}
	if len(got.Servers) != 1 || got.Servers[0] != "192.0.2.10" {
		t.Errorf("servers = %v, want the one nameserver", got.Servers)
	}
}

// A validating name is collected too, with the zone that signs it.
func TestDNSSEC22CollectsValidatingName(t *testing.T) {
	ctx := dnssecchain.WithNSNames(tctest.Context(t))
	f := newDS22Fixture(t)
	f.apexDNSKEY(t)
	f.parentDS(t, ctx, true)
	f.signedAddress(t, "ns1.example", f.zoneKey, f.zoneSigner)
	ds22Server(t, ctx, "ns1.example", "192.0.2.10", f.answers, f.counts)

	entries := f.run(t, ctx, "ns1.example")
	tctest.RequireTags(t, entries, "DS22_NS_ADDRESS_VALIDATES")

	got := collected(t, ctx, "ns1.example")
	if got.Status != dnssecchain.NSNameValidates {
		t.Errorf("status = %q, want %q", got.Status, dnssecchain.NSNameValidates)
	}
	if got.Signer != "example" {
		t.Errorf("signer = %q, want the zone apex", got.Signer)
	}
}

// A name below a secure zone cut names the cut as its signer, which is what
// the chain document draws the edge to.
func TestDNSSEC22CollectsSignerOfASecureCut(t *testing.T) {
	ctx := dnssecchain.WithNSNames(tctest.Context(t))
	f := newDS22Fixture(t)
	f.apexDNSKEY(t)
	f.parentDS(t, ctx, true)
	f.secureCut(t)
	f.signedAddress(t, "a.ns.example", f.cutKey, f.cutSigner)
	ds22Server(t, ctx, "a.ns.example", "192.0.2.10", f.answers, f.counts)

	f.run(t, ctx, "a.ns.example")

	got := collected(t, ctx, "a.ns.example")
	if got.Status != dnssecchain.NSNameValidates {
		t.Errorf("status = %q, want %q", got.Status, dnssecchain.NSNameValidates)
	}
	if got.Signer != "ns.example" {
		t.Errorf("signer = %q, want the zone cut", got.Signer)
	}
}

// A run with no collector installed reaches the same verdict and stores nothing.
func TestDNSSEC22WithoutCollectorStillReports(t *testing.T) {
	ctx := tctest.Context(t)
	f := newDS22Fixture(t)
	f.apexDNSKEY(t)
	f.parentDS(t, ctx, true)
	f.secureCut(t)
	f.orphan(t, "a.ns.example")
	ds22Server(t, ctx, "a.ns.example", "192.0.2.10", f.answers, f.counts)

	entries := f.run(t, ctx, "a.ns.example")
	tctest.RequireTag(t, entries, "DS22_NS_ADDRESS_ORPHAN_ZONE")
	if got := dnssecchain.NSNamesFromContext(ctx); got != nil {
		t.Errorf("want nothing collected, got %+v", got)
	}
}

// Every tag that is a per-name verdict maps onto a chain-document status.
func TestDNSSEC22ChainStatusCoversEveryVerdictTag(t *testing.T) {
	for tag := range dnssec22ErrorTags {
		if dnssec22ChainStatus[tag] == "" {
			t.Errorf("tag %s has no chain-document status", tag)
		}
	}
	if dnssec22ChainStatus["DS22_NS_ADDRESS_INSECURE"] != dnssecchain.NSNameInsecure {
		t.Error("DS22_NS_ADDRESS_INSECURE must map to the insecure status")
	}
	if len(dnssec22ChainStatus) != len(dnssec22ErrorTags)+1 {
		t.Errorf("chain status table has %d rows, want the %d verdict tags", len(dnssec22ChainStatus), len(dnssec22ErrorTags)+1)
	}
}
