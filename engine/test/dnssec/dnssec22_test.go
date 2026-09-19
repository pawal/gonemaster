package dnssec

import (
	"context"
	"crypto"
	"maps"
	"slices"
	"strings"
	"sync"
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

// ds22CountMu guards counts, which several query goroutines share.
var ds22CountMu sync.Mutex

// ds22Server answers from answers, counts questions, and strips RRSIGs when DO is unset.
func ds22Server(t *testing.T, ctx context.Context, name string, ip string, answers ds22Answers, counts map[string]int) nameserver.Nameserver {
	t.Helper()
	return tctest.NS(t, ctx, name, ip, func(q tctest.Query) packet.Packet {
		key := ds22Key(q.Name, q.Type)
		if counts != nil {
			ds22CountMu.Lock()
			counts[key]++
			ds22CountMu.Unlock()
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
	// Copy shares the section slices, so each is cloned before deletion.
	msg := resp.Msg.Copy()
	isSig := func(rr dns.RR) bool { _, ok := rr.(*dns.RRSIG); return ok }
	msg.Answer = slices.DeleteFunc(slices.Clone(msg.Answer), isSig)
	msg.Ns = slices.DeleteFunc(slices.Clone(msg.Ns), isSig)
	msg.Extra = slices.DeleteFunc(slices.Clone(msg.Extra), isSig)
	return packet.Packet{Msg: msg}
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
	// The delegated zone of the referral cases: its key and its own server.
	childKey     *dns.DNSKEY
	childSigner  crypto.Signer
	childAnswers ds22Answers
	childCounts  map[string]int
}

// newDS22Fixture generates the zone and cut keys and an empty answer table.
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
	nsec := nsecRecord("ns.example", dns.TypeNS, dns.TypeRRSIG, dns.TypeNSEC)
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
			tctest.Authority(nsecRecord("ns.example", dns.TypeA, dns.TypeRRSIG, dns.TypeNSEC)))},
		{"NSEC3 without the NS bit", tctest.Response(tctest.Secure(),
			tctest.Authority(nsec3Record("ns.example", "example", dns.TypeA, dns.TypeRRSIG)))},
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
			tctest.Authority(nsecRecord("ns.example", dns.TypeNS, dns.TypeDS, dns.TypeRRSIG)))},
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
		tctest.Authority(nsec3Record("a.ns.example", "ns.example", dns.TypeA, dns.TypeRRSIG)))
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
		tctest.Authority(nsecRecord("ns1.example", dns.TypeA, dns.TypeRRSIG, dns.TypeNSEC)))
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
	if !slices.Equal(got, want) {
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
	if !slices.Equal(got, want) {
		t.Fatalf("names = %v, want %v", got, want)
	}

	// Every name is subordinate to the root, so the root zone has none.
	if rootNames := dnssec22InDomainNames(dnsname.New("."), tctest.NSItems("a.root-servers.net/192.0.2.1")); rootNames != nil {
		t.Fatalf("root name set = %v, want none", rootNames)
	}
}

// run runs DNSSEC22 on zone "example"; specs are "name" or "name/address", address 192.0.2.10 by default.
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

// secureZone publishes the signed apex DNSKEY RRset and its DS at the parent.
func (f *ds22Fixture) secureZone(t *testing.T, ctx context.Context) {
	t.Helper()
	f.apexDNSKEY(t)
	f.parentDS(t, ctx, true)
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
	f.secureZone(t, ctx)
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

// A name served as a zone apex with no delegation proven for it is an orphan.
func TestDNSSEC22OrphanZone(t *testing.T) {
	const owner = "a.ns.example"
	cases := []struct {
		name   string
		denial packet.Packet
	}{
		{"NSEC3 without the NS bit", tctest.Response(tctest.Question(owner, dns.TypeDS), tctest.Secure(),
			tctest.Authority(nsec3Record(owner, "ns.example", dns.TypeA, dns.TypeRRSIG)))},
		{"NSEC without the NS bit", tctest.Response(tctest.Question(owner, dns.TypeDS), tctest.Secure(),
			tctest.Authority(nsecRecord(owner, dns.TypeA, dns.TypeRRSIG, dns.TypeNSEC)))},
		{"NXDOMAIN", tctest.Response(tctest.Question(owner, dns.TypeDS), tctest.Secure(), tctest.NXDOMAIN())},
		{"NSEC3 beside NS records", tctest.Response(tctest.Question(owner, dns.TypeDS), tctest.Secure(),
			tctest.Authority(tctest.SOARR("ns.example"), tctest.NSRR(owner, "ns1.other"),
				nsec3Record(owner, "ns.example", dns.TypeA, dns.TypeRRSIG)))},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := tctest.Context(t)
			f := newDS22Fixture(t)
			f.secureZone(t, ctx)
			f.secureCut(t)
			f.orphan(t, owner)
			f.answers[ds22Key(owner, "DS")] = tc.denial
			ds22Server(t, ctx, owner, "192.0.2.10", f.answers, f.counts)

			entries := f.run(t, ctx, owner)
			entry := tctest.RequireTag(t, entries, "DS22_NS_ADDRESS_ORPHAN_ZONE")
			tctest.RequireArg(t, entry, "signer", owner)
			tctest.RequireArg(t, entry, "ns", owner)
			tctest.RequireNoTag(t, entries, "DS22_NS_ADDRESS_VALIDATES")
			// One leaf query, the two zone cut probes of the walk, and one DNSKEY.
			for _, want := range []string{"a.ns.example/A", "a.ns.example/DS", "ns.example/DS", "ns.example/DNSKEY"} {
				if got := f.counts[want]; got != 1 {
					t.Fatalf("%s questions = %d, want 1", want, got)
				}
			}
			if got := f.counts[ds22Key(owner, "AAAA")]; got != 0 {
				t.Fatalf("AAAA questions = %d, want 0", got)
			}
		})
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

// into returns a copy of the fixture writing into another answer table.
func (f *ds22Fixture) into(answers ds22Answers) *ds22Fixture {
	clone := *f
	clone.answers = answers
	return &clone
}

// noDataOnA publishes a signed NODATA for the A question of name.
func (f *ds22Fixture) noDataOnA(t *testing.T, name string) {
	t.Helper()
	nsec := nsecRecord(name, dns.TypeAAAA, dns.TypeRRSIG, dns.TypeNSEC)
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

// orphan publishes an A RRset for name signed by a key of name itself, and a DS denial for it.
func (f *ds22Fixture) orphan(t *testing.T, name string, denial ...dns.RR) {
	t.Helper()
	key, signer := tctest.SignedKey(t, name, dns.ECDSAP256SHA256, tctest.SEP())
	f.signedAddress(t, name, key, signer)
	f.denyDS(name, denial...)
}

// A nameserver name below a secure zone cut is signed by that zone cut.
func TestDNSSEC22SecureZoneCutValidates(t *testing.T) {
	ctx := tctest.Context(t)
	f := newDS22Fixture(t)
	f.secureZone(t, ctx)
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
	f.secureZone(t, ctx)
	f.unsignedAddress(t, "ns1.sub.example")
	f.denyDS("ns1.sub.example", tctest.SOARR("sub.example"))
	f.denyDS("sub.example", nsecRecord("sub.example", dns.TypeNS, dns.TypeRRSIG, dns.TypeNSEC))
	ds22Server(t, ctx, "ns1.sub.example", "192.0.2.10", f.answers, f.counts)

	entries := f.run(t, ctx, "ns1.sub.example")
	entry := tctest.RequireTag(t, entries, "DS22_NS_ADDRESS_INSECURE")
	tctest.RequireArg(t, entry, "ns", "ns1.sub.example")
	tctest.RequireNoTag(t, entries, "DS22_NS_ADDRESS_UNSIGNED", "DS22_NS_ADDRESS_VALIDATES")
}

// Unsigned address records in the signed zone itself are bogus.
func TestDNSSEC22Unsigned(t *testing.T) {
	ctx := tctest.Context(t)
	f := newDS22Fixture(t)
	f.secureZone(t, ctx)
	f.unsignedAddress(t, "ns1.example")
	f.denyDS("ns1.example", nsecRecord("ns1.example", dns.TypeA, dns.TypeRRSIG, dns.TypeNSEC))
	ds22Server(t, ctx, "ns1.example", "192.0.2.10", f.answers, f.counts)

	entries := f.run(t, ctx, "ns1.example")
	entry := tctest.RequireTag(t, entries, "DS22_NS_ADDRESS_UNSIGNED")
	tctest.RequireArg(t, entry, "ns", "ns1.example")
	// A settles a name at fault; AAAA is never asked for it.
	if got := f.counts[ds22Key("ns1.example", "AAAA")]; got != 0 {
		t.Fatalf("AAAA questions = %d, want 0", got)
	}
}

func TestDNSSEC22LeafSignatureFaults(t *testing.T) {
	cases := []struct {
		name string
		// sign publishes the A RRset of ns1.example and returns the keytag the finding names.
		sign     func(t *testing.T, f *ds22Fixture) uint16
		wantTag  string
		wantArgs map[string]string
	}{
		{
			name: "expired",
			sign: func(t *testing.T, f *ds22Fixture) uint16 {
				f.signedAddress(t, "ns1.example", f.zoneKey, f.zoneSigner,
					tctest.Inception(time.Now().Add(-48*time.Hour)), tctest.Expiration(time.Now().Add(-time.Hour)))
				return keyTag(f.zoneKey)
			},
			wantTag:  "DS22_NS_ADDRESS_RRSIG_EXPIRED",
			wantArgs: map[string]string{"ns": "ns1.example"},
		},
		{
			name: "no DNSKEY with the keytag",
			sign: func(t *testing.T, f *ds22Fixture) uint16 {
				// A key of the apex that the DNSKEY RRset does not publish.
				other, otherSigner := tctest.SignedKey(t, "example", dns.ECDSAP256SHA256, tctest.SEP())
				f.signedAddress(t, "ns1.example", other, otherSigner)
				return keyTag(other)
			},
			wantTag:  "DS22_NS_ADDRESS_RRSIG_NOT_VALID_BY_DNSKEY",
			wantArgs: map[string]string{"ns": "ns1.example", "signer": "example"},
		},
		{
			name: "signature does not verify",
			sign: func(t *testing.T, f *ds22Fixture) uint16 {
				// Another key signs under the keytag of the published one.
				other, otherSigner := tctest.SignedKey(t, "example", dns.ECDSAP256SHA256, tctest.SEP())
				f.signedAddress(t, "ns1.example", other, otherSigner, tctest.KeyTag(keyTag(f.zoneKey)))
				return keyTag(f.zoneKey)
			},
			wantTag:  "DS22_NS_ADDRESS_RRSIG_NOT_VALID_BY_DNSKEY",
			wantArgs: map[string]string{"ns": "ns1.example"},
		},
		{
			name: "signer outside the zone",
			sign: func(t *testing.T, f *ds22Fixture) uint16 {
				foreign, foreignSigner := tctest.SignedKey(t, "other", dns.ECDSAP256SHA256, tctest.SEP())
				f.signedAddress(t, "ns1.example", foreign, foreignSigner)
				f.denyDS("ns1.example", nsecRecord("ns1.example", dns.TypeA, dns.TypeRRSIG, dns.TypeNSEC))
				return keyTag(foreign)
			},
			wantTag:  "DS22_NS_ADDRESS_RRSIG_NOT_VALID_BY_DNSKEY",
			wantArgs: map[string]string{"ns": "ns1.example", "signer": "other"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := tctest.Context(t)
			f := newDS22Fixture(t)
			f.secureZone(t, ctx)
			wantKeytag := tc.sign(t, f)
			ds22Server(t, ctx, "ns1.example", "192.0.2.10", f.answers, f.counts)

			entries := f.run(t, ctx, "ns1.example")
			entry := tctest.RequireTag(t, entries, tc.wantTag)
			for key, want := range tc.wantArgs {
				tctest.RequireArg(t, entry, key, want)
			}
			if entry.Args["keytag"] != wantKeytag {
				t.Fatalf("keytag = %#v, want %d", entry.Args["keytag"], wantKeytag)
			}
			tctest.RequireNoTag(t, entries, "DS22_NS_ADDRESS_VALIDATES")
		})
	}
}

// A zone cut whose own chain is broken carries every name below it with it.
func TestDNSSEC22ChainBroken(t *testing.T) {
	ctx := tctest.Context(t)
	f := newDS22Fixture(t)
	f.secureZone(t, ctx)
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
	tctest.RequireArg(t, entry, "ns", "a.ns.example")
	tctest.RequireArg(t, entry, "signer", "ns.example")
	tctest.RequireNoTag(t, entries, "DS22_NS_ADDRESS_VALIDATES")
}

// Each nameserver is evaluated on its own; the finding names only its servers.
func TestDNSSEC22PerServerDisagreement(t *testing.T) {
	ctx := tctest.Context(t)
	f := newDS22Fixture(t)
	f.secureZone(t, ctx)
	f.orphan(t, "ns1.example", nsecRecord("ns1.example", dns.TypeA, dns.TypeRRSIG, dns.TypeNSEC))
	ds22Server(t, ctx, "ns1.example", "192.0.2.10", f.answers, f.counts)

	healthy := f.copyAnswers()
	f.into(healthy).signedAddress(t, "ns1.example", f.zoneKey, f.zoneSigner)
	ds22Server(t, ctx, "ns1.example", "192.0.2.11", healthy, nil)

	entries := f.run(t, ctx, "ns1.example/192.0.2.10", "ns1.example/192.0.2.11")
	entry := tctest.RequireTag(t, entries, "DS22_NS_ADDRESS_ORPHAN_ZONE")
	if got := tctest.ServerEndpoints(t, entry.Args); !slices.Equal(got, []string{"ns1.example/192.0.2.10"}) {
		t.Fatalf("servers = %v, want the orphaned nameserver alone", got)
	}
	tctest.RequireNoTag(t, entries, "DS22_NS_ADDRESS_VALIDATES")
}

// A signature the local verifier cannot process is indeterminate.
func TestDNSSEC22UnsupportedAlgorithm(t *testing.T) {
	ctx := tctest.Context(t)
	f := newDS22Fixture(t)
	f.secureZone(t, ctx)
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
		f.secureZone(t, ctx)
		f.signedAddress(t, "ns1.example", f.zoneKey, f.zoneSigner)
		f.signedAddress(t, "ns2.example", f.zoneKey, f.zoneSigner)
		ds22Server(t, ctx, "ns1.example", "192.0.2.10", f.answers, f.counts)
		ds22Server(t, ctx, "ns2.example", "2001:db8::1", f.answers, nil)

		entries := f.run(t, ctx, "ns1.example/192.0.2.10", "ns2.example/2001:db8::1")
		entry := tctest.RequireTag(t, entries, "IPV6_DISABLED")
		tctest.RequireArgShape(t, entry, tctest.ArgShape{NS: "ns2.example", Address: "2001:db8::1"})
		tctest.RequireArg(t, entry, "query_type", "A")
	})

	t.Run("IPv4 disabled", func(t *testing.T) {
		ctx := tctest.Context(t)
		f := newDS22Fixture(t)
		f.parentIP = "2001:db8::99"
		if err := profile.Effective().Set("net.ipv4", false); err != nil {
			t.Fatalf("set net.ipv4: %v", err)
		}
		f.secureZone(t, ctx)
		f.signedAddress(t, "ns1.example", f.zoneKey, f.zoneSigner)
		f.signedAddress(t, "ns2.example", f.zoneKey, f.zoneSigner)
		ds22Server(t, ctx, "ns1.example", "2001:db8::1", f.answers, f.counts)
		ds22Server(t, ctx, "ns2.example", "192.0.2.20", f.answers, nil)

		entries := f.run(t, ctx, "ns1.example/2001:db8::1", "ns2.example/192.0.2.20")
		entry := tctest.RequireTag(t, entries, "IPV4_DISABLED")
		tctest.RequireArgShape(t, entry, tctest.ArgShape{NS: "ns2.example", Address: "192.0.2.20"})
		tctest.RequireArg(t, entry, "query_type", "A")
	})
}

// A name with no A RRset is settled by AAAA, or by the denial of both.
func TestDNSSEC22NoDataOnA(t *testing.T) {
	t.Run("AAAA RRset", func(t *testing.T) {
		ctx := tctest.Context(t)
		f := newDS22Fixture(t)
		f.secureZone(t, ctx)
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
		f.secureZone(t, ctx)
		f.noDataOnA(t, "ns1.example")
		ds22Server(t, ctx, "ns1.example", "192.0.2.10", f.answers, f.counts)

		// The signer comes from the denial proof of the A question.
		entries := f.run(t, ctx, "ns1.example")
		tctest.RequireTags(t, entries, "DS22_NS_ADDRESS_VALIDATES")
	})
}

// A referral costs one query and takes the whole subtree with it.
func TestDNSSEC22ReferralShortCircuit(t *testing.T) {
	ctx := tctest.Context(t)
	f := newDS22Fixture(t)
	f.secureZone(t, ctx)
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
	f.secureZone(t, ctx)
	f.orphan(t, "ns1.example", nsecRecord("ns1.example", dns.TypeA, dns.TypeRRSIG, dns.TypeNSEC))
	f.answers[ds22Key("ns1.sub.example", "A")] = tctest.Response(
		tctest.Question("ns1.sub.example", dns.TypeA), tctest.NotAuthoritative(),
		tctest.Authority(tctest.NSRR("sub.example", "ns1.other")))
	ds22Server(t, ctx, "ns1.example", "192.0.2.10", f.answers, f.counts)

	entries := f.run(t, ctx, "ns1.example", "ns1.sub.example")
	entry := tctest.RequireTag(t, entries, "DS22_NS_ADDRESS_ORPHAN_ZONE")
	tctest.RequireArg(t, entry, "ns", "ns1.example")
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
	f.secureZone(t, ctx)
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
	want := []string{"a.ns.example/192.0.2.11", "b.ns.example/192.0.2.11"}
	if got := tctest.ServerEndpoints(t, entry.Args); !slices.Equal(got, want) {
		t.Fatalf("servers = %v, want %v", got, want)
	}
	tctest.RequireNoTag(t, entries, "DS22_NS_ADDRESS_ORPHAN_ZONE",
		"DS22_NS_ADDRESS_RRSIG_NOT_VALID_BY_DNSKEY", "DS22_NS_ADDRESS_CHAIN_BROKEN")
	// The memo asks the question once for both names below the zone cut.
	if got := f.counts[ds22Key("ns.example", "DS")]; got != 1 {
		t.Fatalf("DS questions = %d, want 1", got)
	}
}

// collected returns the chain-document entry for name, or fails.
func collected(t *testing.T, ctx context.Context, name string) dnssecchain.NSName {
	t.Helper()
	names := dnssecchain.NSNamesFromContext(ctx)
	i := slices.IndexFunc(names, func(n dnssecchain.NSName) bool { return n.Name == name })
	if i < 0 {
		t.Fatalf("no collected entry for %q in %+v", name, names)
	}
	return names[i]
}

// The chain document learns the orphan the tag reports, with the same signer.
func TestDNSSEC22CollectsOrphanZone(t *testing.T) {
	ctx := dnssecchain.WithNSNames(tctest.Context(t))
	f := newDS22Fixture(t)
	f.secureZone(t, ctx)
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
	if !slices.Equal(got.Servers, []string{"192.0.2.10"}) {
		t.Errorf("servers = %v, want the one nameserver", got.Servers)
	}
}

// A validating name is collected too, with the zone that signs it.
func TestDNSSEC22CollectsValidatingName(t *testing.T) {
	ctx := dnssecchain.WithNSNames(tctest.Context(t))
	f := newDS22Fixture(t)
	f.secureZone(t, ctx)
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

// A name below a secure zone cut names the cut as its signer.
func TestDNSSEC22CollectsSignerOfASecureCut(t *testing.T) {
	ctx := dnssecchain.WithNSNames(tctest.Context(t))
	f := newDS22Fixture(t)
	f.secureZone(t, ctx)
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
	f.secureZone(t, ctx)
	f.secureCut(t)
	f.orphan(t, "a.ns.example")
	ds22Server(t, ctx, "a.ns.example", "192.0.2.10", f.answers, f.counts)

	entries := f.run(t, ctx, "a.ns.example")
	tctest.RequireTag(t, entries, "DS22_NS_ADDRESS_ORPHAN_ZONE")
	if got := dnssecchain.NSNamesFromContext(ctx); got != nil {
		t.Errorf("want nothing collected, got %+v", got)
	}
}

// Every per-name verdict tag maps onto a chain-document status.
func TestDNSSEC22ChainStatusCoversEveryVerdictTag(t *testing.T) {
	want := map[string]string{
		"DS22_NS_ADDRESS_CHAIN_BROKEN":              dnssecchain.NSNameChainBroken,
		"DS22_NS_ADDRESS_INSECURE":                  dnssecchain.NSNameInsecure,
		"DS22_NS_ADDRESS_ORPHAN_ZONE":               dnssecchain.NSNameOrphan,
		"DS22_NS_ADDRESS_RRSIG_EXPIRED":             dnssecchain.NSNameRRSIGExpired,
		"DS22_NS_ADDRESS_RRSIG_NOT_VALID_BY_DNSKEY": dnssecchain.NSNameRRSIGInvalid,
		"DS22_NS_ADDRESS_UNSIGNED":                  dnssecchain.NSNameUnsigned,
	}
	if !maps.Equal(dnssec22ChainStatus, want) {
		t.Errorf("chain status table = %v, want %v", dnssec22ChainStatus, want)
	}
	for tag := range dnssec22ErrorTags {
		if dnssec22ChainStatus[tag] == "" {
			t.Errorf("tag %s has no chain-document status", tag)
		}
	}
}

// ds22ChildIP is the glue address the referrals below carry.
const ds22ChildIP = "192.0.2.20"

// delegatedZone builds the key of a delegated zone and its own answer table.
func (f *ds22Fixture) delegatedZone(t *testing.T, cut string) {
	t.Helper()
	key, signer := tctest.SignedKey(t, cut, dns.ECDSAP256SHA256, tctest.SEP())
	f.childKey, f.childSigner = key, signer
	f.childAnswers = ds22Answers{}
	f.childCounts = map[string]int{}
	sig := tctest.Sign(t, key, signer, dns.TypeDNSKEY, []dns.RR{key})
	f.childAnswers[ds22Key(cut, "DNSKEY")] = tctest.Response(
		tctest.Question(cut, dns.TypeDNSKEY), tctest.Secure(), tctest.Answers(key, sig))
}

// secureDelegation publishes a DS for cut matching the delegated zone's key, signed by the apex key.
func (f *ds22Fixture) secureDelegation(t *testing.T, cut string) {
	t.Helper()
	ds := f.childKey.ToDS(dns.SHA256)
	if ds == nil {
		t.Fatal("ToDS returned nil")
	}
	ds.Hdr = dns.Header{Name: dnsutil.Fqdn(cut), Class: dns.ClassINET, TTL: 60}
	sig := tctest.Sign(t, f.zoneKey, f.zoneSigner, dns.TypeDS, []dns.RR{ds})
	f.answers[ds22Key(cut, "DS")] = tctest.Response(
		tctest.Question(cut, dns.TypeDS), tctest.Secure(), tctest.Answers(ds, sig))
}

// referTo publishes a referral for name to cut via child; an empty childIP omits the glue.
func (f *ds22Fixture) referTo(name string, cut string, child string, childIP string) {
	opts := []tctest.MsgOpt{
		tctest.Question(name, dns.TypeA),
		tctest.NotAuthoritative(),
		tctest.Authority(tctest.NSRR(cut, child)),
	}
	if childIP != "" {
		opts = append(opts, tctest.Additional(tctest.ARR(child, childIP)))
	}
	f.answers[ds22Key(name, "A")] = tctest.Response(opts...)
}

// childAddress publishes a signed A RRset for name on the delegated zone.
func (f *ds22Fixture) childAddress(t *testing.T, name string, key *dns.DNSKEY, signer crypto.Signer, opts ...tctest.SigOpt) {
	t.Helper()
	addr := tctest.ARR(name, "192.0.2.40")
	sig := tctest.Sign(t, key, signer, dns.TypeA, []dns.RR{addr}, opts...)
	f.childAnswers[ds22Key(name, "A")] = tctest.Response(
		tctest.Question(name, dns.TypeA), tctest.Secure(), tctest.Answers(addr, sig))
}

// secureReferral sets up a signed zone referring names to a securely delegated child with one nameserver.
func (f *ds22Fixture) secureReferral(t *testing.T, ctx context.Context, names ...string) {
	t.Helper()
	f.secureZone(t, ctx)
	f.delegatedZone(t, "sub.example")
	f.secureDelegation(t, "sub.example")
	for _, name := range names {
		f.referTo(name, "sub.example", "bow.sub.example", ds22ChildIP)
	}
	ds22Server(t, ctx, "ns1.sub.example", "192.0.2.10", f.answers, f.counts)
	ds22Server(t, ctx, "bow.sub.example", ds22ChildIP, f.childAnswers, f.childCounts)
}

// An insecure delegation is settled by the zone's own nameservers.
func TestDNSSEC22ReferredInsecureDelegation(t *testing.T) {
	ctx := dnssecchain.WithNSNames(tctest.Context(t))
	f := newDS22Fixture(t)
	f.secureZone(t, ctx)
	f.referTo("ns1.sub.example", "sub.example", "bow.sub.example", ds22ChildIP)
	f.denyDS("sub.example", nsecRecord("sub.example", dns.TypeNS, dns.TypeRRSIG, dns.TypeNSEC))
	ds22Server(t, ctx, "ns1.sub.example", "192.0.2.10", f.answers, f.counts)
	child := map[string]int{}
	ds22Server(t, ctx, "bow.sub.example", ds22ChildIP, ds22Answers{}, child)

	entries := f.run(t, ctx, "ns1.sub.example")
	entry := tctest.RequireTag(t, entries, "DS22_NS_ADDRESS_INSECURE")
	tctest.RequireArg(t, entry, "ns", "ns1.sub.example")
	if got := f.counts[ds22Key("sub.example", "DNSKEY")]; got != 0 {
		t.Errorf("DNSKEY questions on the zone server = %d, want 0", got)
	}
	if len(child) != 0 {
		t.Errorf("questions to the delegated zone = %v, want none", child)
	}
	if got := collected(t, ctx, "ns1.sub.example").Status; got != dnssecchain.NSNameInsecure {
		t.Errorf("status = %q, want %q", got, dnssecchain.NSNameInsecure)
	}
}

// A DS the parent cannot vouch for is the fault; the child is not asked.
func TestDNSSEC22ReferredBrokenDelegation(t *testing.T) {
	ctx := tctest.Context(t)
	f := newDS22Fixture(t)
	f.secureZone(t, ctx)
	f.delegatedZone(t, "sub.example")
	f.secureDelegation(t, "sub.example")
	f.answers[ds22Key("sub.example", "DS")] = ds22WithoutRRSIG(f.answers[ds22Key("sub.example", "DS")])
	f.referTo("ns1.sub.example", "sub.example", "bow.sub.example", ds22ChildIP)
	ds22Server(t, ctx, "ns1.sub.example", "192.0.2.10", f.answers, f.counts)
	ds22Server(t, ctx, "bow.sub.example", ds22ChildIP, f.childAnswers, f.childCounts)

	entries := f.run(t, ctx, "ns1.sub.example")
	entry := tctest.RequireTag(t, entries, "DS22_NS_ADDRESS_CHAIN_BROKEN")
	tctest.RequireArg(t, entry, "ns", "ns1.sub.example")
	tctest.RequireArg(t, entry, "signer", "sub.example")
	if len(f.childCounts) != 0 {
		t.Errorf("questions to the delegated zone = %v, want none", f.childCounts)
	}
}

// A cut the nameserver says nothing about stays without a verdict.
func TestDNSSEC22ReferredCutUnanswered(t *testing.T) {
	ctx := dnssecchain.WithNSNames(tctest.Context(t))
	f := newDS22Fixture(t)
	f.secureZone(t, ctx)
	f.referTo("ns1.sub.example", "sub.example", "bow.sub.example", ds22ChildIP)
	ds22Server(t, ctx, "ns1.sub.example", "192.0.2.10", f.answers, f.counts)

	entries := f.run(t, ctx, "ns1.sub.example")
	if tags := tctest.TagsWithPrefix(entries, "DS22_"); len(tags) != 0 {
		t.Fatalf("tags = %v, want none", tags)
	}
	if got := collected(t, ctx, "ns1.sub.example").Status; got != dnssecchain.NSNameIndeterminate {
		t.Errorf("status = %q, want %q", got, dnssecchain.NSNameIndeterminate)
	}
}

// A secure delegation is followed onto the delegated zone's nameservers, which validate the records.
func TestDNSSEC22ReferredSecureDelegationValidates(t *testing.T) {
	ctx := dnssecchain.WithNSNames(tctest.Context(t))
	f := newDS22Fixture(t)
	f.secureReferral(t, ctx, "ns1.sub.example")
	f.childAddress(t, "ns1.sub.example", f.childKey, f.childSigner)

	entries := f.run(t, ctx, "ns1.sub.example")
	entry := tctest.RequireTag(t, entries, "DS22_NS_ADDRESS_VALIDATES")
	if got := tctest.ServerEndpoints(t, entry.Args); !slices.Equal(got, []string{"bow.sub.example/" + ds22ChildIP}) {
		t.Errorf("servers = %v, want the delegated zone's nameserver", got)
	}
	tctest.RequireNoTag(t, entries, "DS22_NS_ADDRESS_REFERRED")
	if got := f.counts[ds22Key("sub.example", "DNSKEY")]; got != 0 {
		t.Errorf("DNSKEY questions on the zone server = %d, want 0", got)
	}
	if got := f.childCounts[ds22Key("sub.example", "DNSKEY")]; got != 1 {
		t.Errorf("DNSKEY questions on the delegated zone = %d, want 1", got)
	}
	got := collected(t, ctx, "ns1.sub.example")
	if got.Status != dnssecchain.NSNameValidates {
		t.Errorf("status = %q, want %q", got.Status, dnssecchain.NSNameValidates)
	}
	if !slices.Equal(got.Servers, []string{ds22ChildIP}) {
		t.Errorf("servers = %v, want the delegated zone's nameserver", got.Servers)
	}
}

// A fault in the delegated zone is reported against the zone under test, naming the child's nameserver.
func TestDNSSEC22ReferredSecureDelegationLeafFaults(t *testing.T) {
	t.Run("unsigned", func(t *testing.T) {
		ctx := tctest.Context(t)
		f := newDS22Fixture(t)
		f.secureReferral(t, ctx, "ns1.sub.example")
		f.childAddress(t, "ns1.sub.example", f.childKey, f.childSigner)
		f.childAnswers[ds22Key("ns1.sub.example", "A")] = ds22WithoutRRSIG(f.childAnswers[ds22Key("ns1.sub.example", "A")])

		entries := f.run(t, ctx, "ns1.sub.example")
		entry := tctest.RequireTag(t, entries, "DS22_NS_ADDRESS_UNSIGNED")
		tctest.RequireArg(t, entry, "ns", "ns1.sub.example")
		if got := tctest.ServerEndpoints(t, entry.Args); !slices.Equal(got, []string{"bow.sub.example/" + ds22ChildIP}) {
			t.Errorf("servers = %v, want the delegated zone's nameserver", got)
		}
		tctest.RequireNoTag(t, entries, "DS22_NS_ADDRESS_VALIDATES")
		tctest.RequireNoTag(t, entries, "DS22_NS_ADDRESS_REFERRED")
	})

	t.Run("signed by a foreign key", func(t *testing.T) {
		ctx := tctest.Context(t)
		f := newDS22Fixture(t)
		f.secureReferral(t, ctx, "ns1.sub.example")
		foreign, foreignSigner := tctest.SignedKey(t, "sub.example", dns.ECDSAP256SHA256, tctest.SEP())
		f.childAddress(t, "ns1.sub.example", foreign, foreignSigner)

		entries := f.run(t, ctx, "ns1.sub.example")
		entry := tctest.RequireTag(t, entries, "DS22_NS_ADDRESS_RRSIG_NOT_VALID_BY_DNSKEY")
		tctest.RequireArg(t, entry, "signer", "sub.example")
		tctest.RequireNoTag(t, entries, "DS22_NS_ADDRESS_VALIDATES")
	})
}

// A delegated zone whose keys do not match the DS breaks the chain.
func TestDNSSEC22ReferredSecureDelegationChildKeysBroken(t *testing.T) {
	ctx := tctest.Context(t)
	f := newDS22Fixture(t)
	f.secureReferral(t, ctx, "ns1.sub.example")
	other, otherSigner := tctest.SignedKey(t, "sub.example", dns.ECDSAP256SHA256, tctest.SEP())
	sig := tctest.Sign(t, other, otherSigner, dns.TypeDNSKEY, []dns.RR{other})
	f.childAnswers[ds22Key("sub.example", "DNSKEY")] = tctest.Response(
		tctest.Question("sub.example", dns.TypeDNSKEY), tctest.Secure(), tctest.Answers(other, sig))

	entries := f.run(t, ctx, "ns1.sub.example")
	entry := tctest.RequireTag(t, entries, "DS22_NS_ADDRESS_CHAIN_BROKEN")
	tctest.RequireArg(t, entry, "ns", "ns1.sub.example")
	tctest.RequireArg(t, entry, "signer", "sub.example")
	if got := tctest.ServerEndpoints(t, entry.Args); !slices.Equal(got, []string{"bow.sub.example/" + ds22ChildIP}) {
		t.Errorf("servers = %v, want the delegated zone's nameserver", got)
	}
}

// A delegated zone whose nameserver does not answer leaves the name unchecked and reported.
func TestDNSSEC22ReferredSecureDelegationChildSilent(t *testing.T) {
	ctx := dnssecchain.WithNSNames(tctest.Context(t))
	f := newDS22Fixture(t)
	f.secureReferral(t, ctx, "ns1.sub.example")
	delete(f.childAnswers, ds22Key("sub.example", "DNSKEY"))

	entries := f.run(t, ctx, "ns1.sub.example")
	entry := tctest.RequireTag(t, entries, "DS22_NS_ADDRESS_REFERRED")
	tctest.RequireArg(t, entry, "ns", "ns1.sub.example")
	tctest.RequireArg(t, entry, "zone", "sub.example")
	if got := tctest.ServerEndpoints(t, entry.Args); !slices.Equal(got, []string{"ns1.sub.example/192.0.2.10"}) {
		t.Errorf("servers = %v, want the referring nameserver", got)
	}
	if got := collected(t, ctx, "ns1.sub.example").Status; got != dnssecchain.NSNameIndeterminate {
		t.Errorf("status = %q, want %q", got, dnssecchain.NSNameIndeterminate)
	}
}

// A referral without glue names no nameserver to ask, so nothing is queried.
func TestDNSSEC22ReferredSecureDelegationWithoutGlue(t *testing.T) {
	ctx := tctest.Context(t)
	f := newDS22Fixture(t)
	f.secureZone(t, ctx)
	f.delegatedZone(t, "sub.example")
	f.secureDelegation(t, "sub.example")
	f.referTo("ns1.sub.example", "sub.example", "bow.sub.example", "")
	ds22Server(t, ctx, "ns1.sub.example", "192.0.2.10", f.answers, f.counts)
	ds22Server(t, ctx, "bow.sub.example", ds22ChildIP, f.childAnswers, f.childCounts)

	entries := f.run(t, ctx, "ns1.sub.example")
	entry := tctest.RequireTag(t, entries, "DS22_NS_ADDRESS_REFERRED")
	tctest.RequireArg(t, entry, "zone", "sub.example")
	if len(f.childCounts) != 0 {
		t.Errorf("questions to the delegated zone = %v, want none", f.childCounts)
	}
}

// The follow is one level deep; a referral from the delegated zone is not followed again.
func TestDNSSEC22ReferredSecureDelegationDepthOne(t *testing.T) {
	ctx := tctest.Context(t)
	f := newDS22Fixture(t)
	f.secureReferral(t, ctx, "ns1.deep.sub.example")
	ds22Server(t, ctx, "ns1.deep.sub.example", "192.0.2.10", f.answers, f.counts)

	deepKey, deepSigner := tctest.SignedKey(t, "deep.sub.example", dns.ECDSAP256SHA256, tctest.SEP())
	ds := deepKey.ToDS(dns.SHA256)
	ds.Hdr = dns.Header{Name: dnsutil.Fqdn("deep.sub.example"), Class: dns.ClassINET, TTL: 60}
	dsSig := tctest.Sign(t, f.childKey, f.childSigner, dns.TypeDS, []dns.RR{ds})
	f.childAnswers[ds22Key("deep.sub.example", "DS")] = tctest.Response(
		tctest.Question("deep.sub.example", dns.TypeDS), tctest.Secure(), tctest.Answers(ds, dsSig))
	f.childAnswers[ds22Key("ns1.deep.sub.example", "A")] = tctest.Response(
		tctest.Question("ns1.deep.sub.example", dns.TypeA), tctest.NotAuthoritative(),
		tctest.Authority(tctest.NSRR("deep.sub.example", "deep.ns.example")),
		tctest.Additional(tctest.ARR("deep.ns.example", "192.0.2.50")))
	deepCounts := map[string]int{}
	deepAnswers := ds22Answers{}
	deepSig := tctest.Sign(t, deepKey, deepSigner, dns.TypeDNSKEY, []dns.RR{deepKey})
	deepAnswers[ds22Key("deep.sub.example", "DNSKEY")] = tctest.Response(
		tctest.Question("deep.sub.example", dns.TypeDNSKEY), tctest.Secure(), tctest.Answers(deepKey, deepSig))
	ds22Server(t, ctx, "deep.ns.example", "192.0.2.50", deepAnswers, deepCounts)

	entries := f.run(t, ctx, "ns1.deep.sub.example")
	entry := tctest.RequireTag(t, entries, "DS22_NS_ADDRESS_REFERRED")
	tctest.RequireArg(t, entry, "zone", "deep.sub.example")
	if got := tctest.ServerEndpoints(t, entry.Args); !slices.Equal(got, []string{"bow.sub.example/" + ds22ChildIP}) {
		t.Errorf("servers = %v, want the delegated zone's nameserver", got)
	}
	if len(deepCounts) != 0 {
		t.Errorf("questions two levels down = %v, want none", deepCounts)
	}
}

// Every name below a referred cut is evaluated at one DS question per zone nameserver and one DNSKEY per child.
func TestDNSSEC22ReferralCoversEveryNameInTheSubtree(t *testing.T) {
	ctx := dnssecchain.WithNSNames(tctest.Context(t))
	names := []string{"ns1.sub.example", "ns2.sub.example", "ns3.sub.example"}
	f := newDS22Fixture(t)
	f.secureReferral(t, ctx, names...)
	ds22Server(t, ctx, "ns2.sub.example", "192.0.2.11", f.answers, f.counts)
	for _, name := range names {
		f.childAddress(t, name, f.childKey, f.childSigner)
	}

	entries := f.run(t, ctx, "ns1.sub.example/192.0.2.10", "ns2.sub.example/192.0.2.11", "ns3.sub.example/192.0.2.10")
	tctest.RequireTag(t, entries, "DS22_NS_ADDRESS_VALIDATES")
	if got := f.counts[ds22Key("sub.example", "DS")]; got != 2 {
		t.Errorf("DS questions = %d, want one per nameserver of the zone", got)
	}
	if got := f.childCounts[ds22Key("sub.example", "DNSKEY")]; got != 1 {
		t.Errorf("DNSKEY questions = %d, want one for the run", got)
	}
	for _, name := range names {
		if got := f.childCounts[ds22Key(name, "A")]; got != 1 {
			t.Errorf("%s A questions = %d, want 1", name, got)
		}
		if got := collected(t, ctx, name).Status; got != dnssecchain.NSNameValidates {
			t.Errorf("%s status = %q, want %q", name, got, dnssecchain.NSNameValidates)
		}
	}
}

// A delegated zone nameserver on a disabled transport is reported and skipped.
func TestDNSSEC22ReferredChildTransportDisabled(t *testing.T) {
	ctx := tctest.Context(t)
	f := newDS22Fixture(t)
	if err := profile.Effective().Set("net.ipv6", false); err != nil {
		t.Fatalf("set net.ipv6: %v", err)
	}
	f.secureZone(t, ctx)
	f.delegatedZone(t, "sub.example")
	f.secureDelegation(t, "sub.example")
	f.answers[ds22Key("ns1.sub.example", "A")] = tctest.Response(
		tctest.Question("ns1.sub.example", dns.TypeA), tctest.NotAuthoritative(),
		tctest.Authority(tctest.NSRR("sub.example", "bow.sub.example")),
		tctest.Additional(tctest.AAAARR("bow.sub.example", "2001:db8::2")))
	ds22Server(t, ctx, "ns1.sub.example", "192.0.2.10", f.answers, f.counts)
	ds22Server(t, ctx, "bow.sub.example", "2001:db8::2", f.childAnswers, f.childCounts)

	entries := f.run(t, ctx, "ns1.sub.example")
	entry := tctest.RequireTag(t, entries, "IPV6_DISABLED")
	tctest.RequireArgShape(t, entry, tctest.ArgShape{NS: "bow.sub.example", Address: "2001:db8::2"})
	tctest.RequireTag(t, entries, "DS22_NS_ADDRESS_REFERRED")
	if len(f.childCounts) != 0 {
		t.Errorf("questions to the delegated zone = %v, want none", f.childCounts)
	}
}

// The zone cut memo holds one entry per name whichever path asked first.
func TestDNSSEC22ReferredCutIsNotContendedWithASignerCut(t *testing.T) {
	ctx := tctest.Context(t)
	f := newDS22Fixture(t)
	f.apexDNSKEY(t)
	f.delegatedZone(t, "ns.example")
	f.secureDelegation(t, "ns.example")
	walker := f.walker(t, ctx)

	referral := tctest.Response(tctest.NotAuthoritative(), tctest.Authority(tctest.NSRR("ns.example", "bow.ns.example")))
	walker.noteReferral(referral)
	if got := walker.referredCut(ctx, dnsname.New("ns.example")).status; got != dnssec22SecureDelegation {
		t.Fatalf("referred status = %v, want a secure delegation", got)
	}
	if got := walker.cut(ctx, dnsname.New("ns.example")).status; got != dnssec22SecureDelegation {
		t.Errorf("memoized status = %v, want the referred one", got)
	}
	if got := len(walker.cuts); got != 1 {
		t.Errorf("memo entries = %d, want 1", got)
	}
	if got := f.counts[ds22Key("ns.example", "DNSKEY")]; got != 0 {
		t.Errorf("DNSKEY questions = %d, want 0", got)
	}
}
