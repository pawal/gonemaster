package dnssec

import (
	"context"
	"maps"
	"slices"
	"strings"
	"testing"

	dns "codeberg.org/miekg/dns"
	"codeberg.org/miekg/dns/dnsutil"

	"codeberg.org/pawal/gonemaster/engine/dnsname"
	"codeberg.org/pawal/gonemaster/engine/logger"
	"codeberg.org/pawal/gonemaster/engine/nsdiscovery"
	"codeberg.org/pawal/gonemaster/engine/packet"
	"codeberg.org/pawal/gonemaster/engine/profile"
	"codeberg.org/pawal/gonemaster/engine/test/internal/tctest"
	"codeberg.org/pawal/gonemaster/engine/zone"
)

const ds23Probe = "xn--gonemaster-dnssec23.example"

// ds23ErrorTags is every DS23 tag that suppresses the OK tag.
var ds23ErrorTags = []string{"DS23_NO_DENIAL_PROOF", "DS23_NSEC3_CHAIN_NOT_PUBLISHED", "DS23_NSEC3_DUPLICATE_NEXT",
	"DS23_NSEC3_MIXED_PARAMETERS", "DS23_NSEC3_RANGES_OVERLAP", "DS23_NSEC_RANGES_OVERLAP"}

// ds23Fixture is the zone "example" answering from a table.
type ds23Fixture struct {
	answers ds22Answers
	counts  map[string]int
}

func newDS23Fixture(t *testing.T) *ds23Fixture {
	t.Helper()
	return &ds23Fixture{answers: ds22Answers{}, counts: map[string]int{}}
}

// signed publishes an apex DNSKEY RRset.
func (f *ds23Fixture) signed() {
	f.answers[ds22Key("example", "DNSKEY")] = dnskeyPacket("example", tctest.DNSKEYRR("example", 13, tctest.PublicKey("AwEAAc==")))
}

// denial answers the probe with NXDOMAIN and the given authority records beside the SOA.
func (f *ds23Fixture) denial(rrs ...dns.RR) {
	f.answers[ds22Key(ds23Probe, "A")] = tctest.Response(tctest.Question(ds23Probe, dns.TypeA), tctest.Secure(),
		tctest.NXDOMAIN(), tctest.Authority(append([]dns.RR{tctest.SOARR("example")}, rrs...)...))
}

// params publishes the apex NSEC3PARAM RRset; none is a NODATA answer.
func (f *ds23Fixture) params(records ...*dns.NSEC3PARAM) {
	opts := []tctest.MsgOpt{tctest.Question("example", dns.TypeNSEC3PARAM), tctest.Secure()}
	if len(records) == 0 {
		opts = append(opts, tctest.Authority(tctest.SOARR("example")))
	}
	for _, rr := range records {
		opts = append(opts, tctest.Answers(rr))
	}
	f.answers[ds22Key("example", "NSEC3PARAM")] = tctest.Response(opts...)
}

// run runs DNSSEC23 on zone "example" delegated to the "name/address" specs.
func (f *ds23Fixture) run(t *testing.T, ctx context.Context, specs ...string) []*logger.Entry {
	t.Helper()
	return ds23Run(t, ctx, "example", specs...)
}

// ds23Zone stubs the delegation of zoneName to the "name/address" specs and returns the zone.
func ds23Zone(t *testing.T, zoneName string, specs ...string) *zone.Zone {
	t.Helper()
	items := tctest.NSItems(specs...)
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
	return &z
}

// ds23Run runs DNSSEC23 on zoneName delegated to the "name/address" specs.
func ds23Run(t *testing.T, ctx context.Context, zoneName string, specs ...string) []*logger.Entry {
	t.Helper()
	entries, err := DNSSEC23(ctx, ds23Zone(t, zoneName, specs...))
	if err != nil {
		t.Fatalf("DNSSEC23: %v", err)
	}
	return entries
}

// ds23NSEC3 builds an NSEC3 owned by hash below zoneName, with no salt and no iterations.
func ds23NSEC3(hash string, zoneName string, next string) *dns.NSEC3 {
	rr := nsec3At(hash+"."+zoneName, "", 0, dns.TypeRRSIG)
	rr.HashLength = uint8(len(next))
	rr.NextDomain = next
	return rr
}

// ds23Salted sets the salt of an NSEC3 record.
func ds23Salted(rr *dns.NSEC3, salt string) *dns.NSEC3 {
	rr.Salt = salt
	rr.SaltLength = uint8(len(salt) / 2)
	return rr
}

// ds23NSEC builds an NSEC from owner to next.
func ds23NSEC(owner string, next string) *dns.NSEC {
	rr := nsecRecord(owner, dns.TypeRRSIG, dns.TypeNSEC)
	rr.NextDomain = dnsutil.Fqdn(next)
	return rr
}

func ds23Param(salt string) *dns.NSEC3PARAM {
	rr := &dns.NSEC3PARAM{Hdr: dns.Header{Name: dnsutil.Fqdn("example"), Class: dns.ClassINET, TTL: 60}}
	rr.Hash = dns.SHA1
	rr.Salt = salt
	rr.SaltLength = uint8(len(salt) / 2)
	return rr
}

// observedProof is the live case: two records share a Next Hashed Owner Name.
func observedProof() []dns.RR {
	return []dns.RR{
		ds23NSEC3("thoko93k5mkqlce0n2o6nhvd09pnauk6", "example", "VIU5GCJLL1UM20BO5HN6N3TIO5JV0O9D"),
		ds23NSEC3("tjtj5216ai7cv24b18s4flf9vlmf94rv", "example", "VIU5GCJLL1UM20BO5HN6N3TIO5JV0O9D"),
		ds23NSEC3("lgokkdr1u4d30m69qkge1pri7ollnjhm", "example", "M2DCGNG5014HMBJK0CJONU50NSFLE3EP"),
	}
}

// correctProof is the proof example.com served on 2026-10-03, with its wrap record.
func correctProof() []dns.RR {
	return []dns.RR{
		ds23NSEC3("onib9mgub9h0rml3cdf5bgrj59dkjhvk", "example", "ORDCO76QIHM48OHKS3EGM4PQQ8TMH34D"),
		ds23NSEC3("asrl1c0m0tut0mdoju88no598du3o3o9", "example", "DPUPKHEHTK6L0TUJCSQ61UEDLG9HU8I8"),
		ds23NSEC3("sbus2lgnol7oo15jmvv398a481v5pphb", "example", "9RHU5RBRS0UJ718GM6FIT3L5QT2SN9AM"),
	}
}

func TestDNSSEC23ChainErrors(t *testing.T) {
	cases := []struct {
		name    string
		denial  []dns.RR
		params  []*dns.NSEC3PARAM
		tag     string
		owner   string
		covered string
		next    string
		absent  []string
		counts  map[string]int
	}{
		{name: "NSEC3 duplicate next", denial: observedProof(), params: []*dns.NSEC3PARAM{ds23Param("")},
			tag: "DS23_NSEC3_DUPLICATE_NEXT", owner: "thoko93k5mkqlce0n2o6nhvd09pnauk6.example",
			covered: "tjtj5216ai7cv24b18s4flf9vlmf94rv.example", next: "VIU5GCJLL1UM20BO5HN6N3TIO5JV0O9D",
			absent: []string{"DS23_NSEC3_RANGES_OVERLAP", "DS23_DENIAL_PROOF_CONSISTENT"},
			counts: map[string]int{"example/DNSKEY": 1, ds23Probe + "/A": 1, "example/NSEC3PARAM": 1}},
		{name: "NSEC3 ranges overlap", denial: []dns.RR{ds23NSEC3("aaaa", "example", "CCCC"),
			ds23NSEC3("bbbb", "example", "DDDD"), ds23NSEC3("dddd", "example", "AAAA")},
			params: []*dns.NSEC3PARAM{ds23Param("")}, tag: "DS23_NSEC3_RANGES_OVERLAP",
			owner: "aaaa.example", covered: "bbbb.example", next: "CCCC",
			absent: []string{"DS23_NSEC3_DUPLICATE_NEXT", "DS23_DENIAL_PROOF_CONSISTENT"}},
		{name: "NSEC ranges overlap", denial: []dns.RR{ds23NSEC("example", "m.example"), ds23NSEC("b.example", "z.example")},
			tag: "DS23_NSEC_RANGES_OVERLAP", owner: "example", covered: "b.example", next: "m.example",
			absent: []string{"DS23_DENIAL_PROOF_CONSISTENT"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := tctest.Context(t)
			f := newDS23Fixture(t)
			f.signed()
			f.denial(tc.denial...)
			if tc.params != nil {
				f.params(tc.params...)
			}
			ds22Server(t, ctx, "ns1.example", "192.0.2.10", f.answers, f.counts)

			entries := f.run(t, ctx, "ns1.example/192.0.2.10")
			entry := tctest.RequireTag(t, entries, tc.tag)
			tctest.RequireArg(t, entry, "owner", tc.owner)
			tctest.RequireArg(t, entry, "covered", tc.covered)
			tctest.RequireArg(t, entry, "next", tc.next)
			tctest.RequireCount(t, entries, tc.tag, 1)
			tctest.RequireNoTag(t, entries, tc.absent...)
			for want, n := range tc.counts {
				if got := f.counts[want]; got != n {
					t.Fatalf("%s questions = %d, want %d", want, got, n)
				}
			}
		})
	}
}

func TestDNSSEC23ConsistentNSEC3(t *testing.T) {
	cases := []struct {
		name  string
		proof []dns.RR
	}{
		{"three records with a wrap record", correctProof()},
		{"opt-out chain", func() []dns.RR {
			proof := correctProof()
			for _, rr := range proof {
				rr.(*dns.NSEC3).Flags = 1
			}
			return proof
		}()},
		{"one record", correctProof()[:1]},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := tctest.Context(t)
			f := newDS23Fixture(t)
			f.signed()
			f.denial(tc.proof...)
			f.params(ds23Param(""))
			ds22Server(t, ctx, "ns1.example", "192.0.2.10", f.answers, f.counts)

			entries := f.run(t, ctx, "ns1.example/192.0.2.10")
			entry := tctest.RequireTag(t, entries, "DS23_DENIAL_PROOF_CONSISTENT")
			if got := tctest.ServerEndpoints(t, entry.Args); !slices.Equal(got, []string{"ns1.example/192.0.2.10"}) {
				t.Fatalf("servers = %v, want the one nameserver", got)
			}
			tctest.RequireNoTag(t, entries, append(ds23ErrorTags, "DS23_MULTIPLE_NSEC3PARAM")...)
		})
	}
}

func TestDNSSEC23ConsistentNSEC(t *testing.T) {
	ctx := tctest.Context(t)
	f := newDS23Fixture(t)
	f.signed()
	// The iis.se proof of 2026-10-03, plus the last record of the chain.
	f.denial(ds23NSEC("example", "_dmarc.example"), ds23NSEC("xmpp01.example", "xwin.example"),
		ds23NSEC("xwin.example", "example"))
	ds22Server(t, ctx, "ns1.example", "192.0.2.10", f.answers, f.counts)

	entries := f.run(t, ctx, "ns1.example/192.0.2.10")
	tctest.RequireTags(t, entries, "DS23_DENIAL_PROOF_CONSISTENT")
	tctest.RequireNoTag(t, entries, ds23ErrorTags...)
	if got := f.counts["example/NSEC3PARAM"]; got != 0 {
		t.Fatalf("NSEC3PARAM questions = %d, want 0", got)
	}
}

func TestDNSSEC23MixedParameters(t *testing.T) {
	ctx := tctest.Context(t)
	f := newDS23Fixture(t)
	f.signed()
	// The duplicate next field is not reported across two chains.
	f.denial(
		ds23Salted(ds23NSEC3("aaaa", "example", "CCCC"), "AABB"),
		ds23NSEC3("bbbb", "example", "CCCC"))
	f.params(ds23Param("0011"), ds23Param("2233"))
	ds22Server(t, ctx, "ns1.example", "192.0.2.10", f.answers, f.counts)

	entries := f.run(t, ctx, "ns1.example/192.0.2.10")
	tctest.RequireTags(t, entries, "DS23_NSEC3_MIXED_PARAMETERS", "DS23_MULTIPLE_NSEC3PARAM")
	tctest.RequireNoTag(t, entries, "DS23_NSEC3_DUPLICATE_NEXT", "DS23_NSEC3_RANGES_OVERLAP",
		"DS23_NSEC3_CHAIN_NOT_PUBLISHED", "DS23_DENIAL_PROOF_CONSISTENT")
}

func TestDNSSEC23ChainNotPublished(t *testing.T) {
	cases := []struct {
		name   string
		params func(f *ds23Fixture)
		want   string
		absent []string
	}{
		{"other salt", func(f *ds23Fixture) { f.params(ds23Param("AABB")) },
			"DS23_NSEC3_CHAIN_NOT_PUBLISHED", []string{"DS23_DENIAL_PROOF_CONSISTENT"}},
		{"no NSEC3PARAM", func(f *ds23Fixture) { f.params() },
			"DS23_NSEC3_CHAIN_NOT_PUBLISHED", []string{"DS23_DENIAL_PROOF_CONSISTENT"}},
		{"NSEC3PARAM unanswered", func(*ds23Fixture) {},
			"DS23_DENIAL_PROOF_CONSISTENT", []string{"DS23_NSEC3_CHAIN_NOT_PUBLISHED", "DS23_MULTIPLE_NSEC3PARAM"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := tctest.Context(t)
			f := newDS23Fixture(t)
			f.signed()
			f.denial(correctProof()...)
			tc.params(f)
			ds22Server(t, ctx, "ns1.example", "192.0.2.10", f.answers, f.counts)

			entries := f.run(t, ctx, "ns1.example/192.0.2.10")
			tctest.RequireTags(t, entries, tc.want)
			tctest.RequireNoTag(t, entries, tc.absent...)
		})
	}
}

func TestDNSSEC23MultipleNSEC3PARAM(t *testing.T) {
	ctx := tctest.Context(t)
	f := newDS23Fixture(t)
	f.signed()
	f.denial(correctProof()...)
	f.params(ds23Param(""), ds23Param("AABB"))
	ds22Server(t, ctx, "ns1.example", "192.0.2.10", f.answers, f.counts)

	entries := f.run(t, ctx, "ns1.example/192.0.2.10")
	tctest.RequireTags(t, entries, "DS23_MULTIPLE_NSEC3PARAM", "DS23_DENIAL_PROOF_CONSISTENT")
	tctest.RequireNoTag(t, entries, ds23ErrorTags...)
}

func TestDNSSEC23CompactDenial(t *testing.T) {
	ctx := tctest.Context(t)
	f := newDS23Fixture(t)
	f.signed()
	nsec := ds23NSEC(ds23Probe, "\\000."+ds23Probe)
	nsec.TypeBitMap = []uint16{dns.TypeRRSIG, dns.TypeNSEC, 128}
	f.answers[ds22Key(ds23Probe, "A")] = tctest.Response(tctest.Question(ds23Probe, dns.TypeA), tctest.Secure(),
		tctest.Authority(tctest.SOARR("example"), nsec))
	ds22Server(t, ctx, "ns1.example", "192.0.2.10", f.answers, f.counts)

	entries := f.run(t, ctx, "ns1.example/192.0.2.10")
	tctest.RequireTags(t, entries, "DS23_DENIAL_PROOF_CONSISTENT")
	tctest.RequireNoTag(t, entries, ds23ErrorTags...)
}

// A wildcard expansion carries the proof in its authority section and is read.
func TestDNSSEC23WildcardExpansion(t *testing.T) {
	cases := []struct {
		name   string
		proof  []dns.RR
		want   string
		absent []string
	}{
		{"consistent", correctProof()[:1], "DS23_DENIAL_PROOF_CONSISTENT", ds23ErrorTags},
		{"duplicate next", observedProof(), "DS23_NSEC3_DUPLICATE_NEXT", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := tctest.Context(t)
			f := newDS23Fixture(t)
			f.signed()
			f.answers[ds22Key(ds23Probe, "A")] = tctest.Response(tctest.Question(ds23Probe, dns.TypeA), tctest.Secure(),
				tctest.Answers(tctest.ARR(ds23Probe, "192.0.2.1")), tctest.Authority(tc.proof...))
			f.params(ds23Param(""))
			ds22Server(t, ctx, "ns1.example", "192.0.2.10", f.answers, f.counts)

			entries := f.run(t, ctx, "ns1.example/192.0.2.10")
			tctest.RequireTags(t, entries, tc.want)
			tctest.RequireNoTag(t, entries, tc.absent...)
		})
	}
}

func TestDNSSEC23UnsignedZoneIsSilent(t *testing.T) {
	ctx := tctest.Context(t)
	f := newDS23Fixture(t)
	f.denial(observedProof()...)
	ds22Server(t, ctx, "ns1.example", "192.0.2.10", f.answers, f.counts)

	entries := f.run(t, ctx, "ns1.example/192.0.2.10")
	if tags := tctest.TagsWithPrefix(entries, "DS23_"); len(tags) != 0 {
		t.Fatalf("tags = %v, want none", tags)
	}
	tctest.RequireTags(t, entries, "TEST_CASE_START", "TEST_CASE_END")
	if got := f.counts[ds23Probe+"/A"]; got != 0 {
		t.Fatalf("probe questions = %d, want 0", got)
	}
}

func TestDNSSEC23NoDenialProof(t *testing.T) {
	cases := []struct {
		name      string
		authority []dns.RR
	}{
		{"SOA only", nil},
		{"NSEC3 of another zone", []dns.RR{ds23NSEC3("aaaa", "other", "CCCC")}},
		{"NSEC of another zone", []dns.RR{ds23NSEC("other", "a.other")}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := tctest.Context(t)
			f := newDS23Fixture(t)
			f.signed()
			f.denial(tc.authority...)
			ds22Server(t, ctx, "ns1.example", "192.0.2.10", f.answers, f.counts)

			entries := f.run(t, ctx, "ns1.example/192.0.2.10")
			entry := tctest.RequireTag(t, entries, "DS23_NO_DENIAL_PROOF")
			if got := tctest.ServerEndpoints(t, entry.Args); !slices.Equal(got, []string{"ns1.example/192.0.2.10"}) {
				t.Fatalf("servers = %v, want the one nameserver", got)
			}
			tctest.RequireNoTag(t, entries, "DS23_DENIAL_PROOF_CONSISTENT")
		})
	}
}

// A response that is no proof yields nothing; other testcases report the nameserver.
func TestDNSSEC23UnusableResponse(t *testing.T) {
	truncated := func(msg *dns.Msg) { msg.Truncated = true }
	cases := []struct {
		name   string
		answer packet.Packet
	}{
		{"no response", packet.Packet{}},
		{"not authoritative", tctest.Response(tctest.Secure(), tctest.NotAuthoritative(), tctest.NXDOMAIN(),
			tctest.Authority(observedProof()...))},
		{"SERVFAIL", tctest.Response(tctest.Secure(), tctest.Rcode(dns.RcodeServerFailure))},
		{"REFUSED", tctest.Response(tctest.Secure(), tctest.Rcode(dns.RcodeRefused))},
		{"truncated", tctest.Response(tctest.Secure(), tctest.NXDOMAIN(), truncated,
			tctest.Authority(tctest.SOARR("example")))},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := tctest.Context(t)
			f := newDS23Fixture(t)
			f.signed()
			f.answers[ds22Key(ds23Probe, "A")] = tc.answer
			ds22Server(t, ctx, "ns1.example", "192.0.2.10", f.answers, f.counts)

			entries := f.run(t, ctx, "ns1.example/192.0.2.10")
			if tags := tctest.TagsWithPrefix(entries, "DS23_"); len(tags) != 0 {
				t.Fatalf("tags = %v, want none", tags)
			}
		})
	}
}

func TestDNSSEC23PerServerDisagreement(t *testing.T) {
	ctx := tctest.Context(t)
	f := newDS23Fixture(t)
	f.signed()
	f.denial(observedProof()...)
	f.params(ds23Param(""))
	ds22Server(t, ctx, "ns1.example", "192.0.2.10", f.answers, f.counts)

	healthy := &ds23Fixture{answers: maps.Clone(f.answers)}
	healthy.denial(correctProof()...)
	ds22Server(t, ctx, "ns2.example", "192.0.2.11", healthy.answers, nil)

	entries := f.run(t, ctx, "ns1.example/192.0.2.10", "ns2.example/192.0.2.11")
	entry := tctest.RequireTag(t, entries, "DS23_NSEC3_DUPLICATE_NEXT")
	if got := tctest.ServerEndpoints(t, entry.Args); !slices.Equal(got, []string{"ns1.example/192.0.2.10"}) {
		t.Fatalf("servers = %v, want the faulty nameserver alone", got)
	}
	tctest.RequireNoTag(t, entries, "DS23_DENIAL_PROOF_CONSISTENT")
}

// Two nameserver names on one address are probed once and reported together.
func TestDNSSEC23SharedAddress(t *testing.T) {
	ctx := tctest.Context(t)
	f := newDS23Fixture(t)
	f.signed()
	f.denial(correctProof()...)
	f.params(ds23Param(""))
	ds22Server(t, ctx, "ns1.example", "192.0.2.10", f.answers, f.counts)

	entries := f.run(t, ctx, "ns1.example/192.0.2.10", "ns2.example/192.0.2.10")
	entry := tctest.RequireTag(t, entries, "DS23_DENIAL_PROOF_CONSISTENT")
	if got := tctest.ServerEndpoints(t, entry.Args); !slices.Equal(got, []string{"ns1.example/192.0.2.10", "ns2.example/192.0.2.10"}) {
		t.Fatalf("servers = %v, want both names", got)
	}
	if got := f.counts[ds23Probe+"/A"]; got != 1 {
		t.Fatalf("probe questions = %d, want 1", got)
	}
}

func TestDNSSEC23TransportDisabled(t *testing.T) {
	ctx := tctest.Context(t)
	f := newDS23Fixture(t)
	if err := profile.Effective().Set("net.ipv6", false); err != nil {
		t.Fatalf("set net.ipv6: %v", err)
	}
	f.signed()
	f.denial(correctProof()...)
	f.params(ds23Param(""))
	ds22Server(t, ctx, "ns1.example", "192.0.2.10", f.answers, f.counts)
	ds22Server(t, ctx, "ns2.example", "2001:db8::1", f.answers, nil)

	entries := f.run(t, ctx, "ns1.example/192.0.2.10", "ns2.example/2001:db8::1")
	entry := tctest.RequireTag(t, entries, "IPV6_DISABLED")
	tctest.RequireArgShape(t, entry, tctest.ArgShape{NS: "ns2.example", Address: "2001:db8::1"})
	tctest.RequireArg(t, entry, "query_type", "A")
	entry = tctest.RequireTag(t, entries, "DS23_DENIAL_PROOF_CONSISTENT")
	if got := tctest.ServerEndpoints(t, entry.Args); !slices.Equal(got, []string{"ns1.example/192.0.2.10"}) {
		t.Fatalf("servers = %v, want the IPv4 nameserver alone", got)
	}
}

func TestDNSSEC23RootZone(t *testing.T) {
	ctx := tctest.Context(t)
	f := newDS23Fixture(t)
	f.answers[ds22Key(".", "DNSKEY")] = dnskeyPacket(".", tctest.DNSKEYRR(".", 13, tctest.PublicKey("AwEAAc==")))
	// The root proof of 2026-10-03.
	f.answers[ds22Key("xn--gonemaster-dnssec23.", "A")] = tctest.Response(tctest.Secure(), tctest.NXDOMAIN(),
		tctest.Authority(ds23NSEC(".", "aaa."), ds23NSEC("xn--gk3at1e.", "xn--h2breg3eve.")))
	ds22Server(t, ctx, "a.root-servers.net", "192.0.2.10", f.answers, f.counts)

	entries := ds23Run(t, ctx, ".", "a.root-servers.net/192.0.2.10")
	tctest.RequireTags(t, entries, "DS23_DENIAL_PROOF_CONSISTENT")
	tctest.RequireNoTag(t, entries, ds23ErrorTags...)
	if got := f.counts["xn--gonemaster-dnssec23/A"]; got != 1 {
		t.Fatalf("probe questions = %d, want 1", got)
	}
}

func TestDNSSEC23Covers(t *testing.T) {
	cases := []struct {
		name  string
		owner string
		next  string
		key   string
		want  bool
	}{
		{"inside", "b", "d", "c", true},
		{"at the owner", "b", "d", "b", false},
		{"at the next", "b", "d", "d", false},
		{"below", "b", "d", "a", false},
		{"above", "b", "d", "e", false},
		{"wrap above the owner", "d", "b", "e", true},
		{"wrap below the next", "d", "b", "a", true},
		{"wrap between", "d", "b", "c", false},
		{"single record chain", "b", "b", "z", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rr := dnssec23Record{key: tc.owner, nextKey: tc.next}
			if got := rr.covers(tc.key, strings.Compare); got != tc.want {
				t.Fatalf("covers = %v, want %v", got, tc.want)
			}
		})
	}
}

// NSEC intervals follow canonical name order, not string order.
func TestDNSSEC23NSECIntervalsOrder(t *testing.T) {
	records := dnssec23NSECIntervals([]*dns.NSEC{
		ds23NSEC("c.example", "example"), ds23NSEC("a.b.example", "c.example"),
		ds23NSEC("example", "b.example"), ds23NSEC("B.example", "a.b.example"),
	})
	var got []string
	for _, rr := range records {
		got = append(got, rr.owner)
	}
	want := []string{"example", "b.example", "a.b.example", "c.example"}
	if !slices.Equal(got, want) {
		t.Fatalf("order = %v, want %v", got, want)
	}
	if records[1].next != "a.b.example" {
		t.Fatalf("next = %q, want a.b.example", records[1].next)
	}
}

func TestDNSSEC23NSEC3IntervalsLowercaseKeys(t *testing.T) {
	records := dnssec23NSEC3Intervals([]*dns.NSEC3{
		ds23NSEC3("BBBB", "example", "CCCC"), ds23NSEC3("aaaa", "EXAMPLE", "bbbb"),
	})
	if records[0].owner != "aaaa.example" || records[0].key != "aaaa" || records[0].nextKey != "bbbb" {
		t.Fatalf("first record = %+v", records[0])
	}
	if records[1].owner != "bbbb.example" || records[1].next != "CCCC" || records[1].nextKey != "cccc" {
		t.Fatalf("second record = %+v", records[1])
	}
}

func TestDNSSEC23ProbeName(t *testing.T) {
	if got := dnsname.New("example").Prepend(dnssec23ProbeLabel).String(); got != ds23Probe {
		t.Fatalf("probe = %q, want %q", got, ds23Probe)
	}
}

// In a full run the DNSKEY and NSEC3PARAM answers come from the cache DNSSEC10 filled.
func TestDNSSEC23ReusesDNSSEC10Queries(t *testing.T) {
	ctx := tctest.Context(t)
	f := newDS23Fixture(t)
	f.signed()
	f.denial(correctProof()...)
	f.params(ds23Param(""))
	f.answers[ds22Key("example", "NSEC")] = tctest.Response(tctest.Question("example", dns.TypeNSEC), tctest.Secure(),
		tctest.Authority(tctest.SOARR("example"), correctProof()[0]))
	ds22Server(t, ctx, "ns1.example", "192.0.2.10", f.answers, f.counts)
	z := ds23Zone(t, "example", "ns1.example/192.0.2.10")
	if _, err := DNSSEC10(ctx, z); err != nil {
		t.Fatalf("DNSSEC10: %v", err)
	}
	entries, err := DNSSEC23(ctx, z)
	if err != nil {
		t.Fatalf("DNSSEC23: %v", err)
	}
	tctest.RequireTags(t, entries, "DS23_DENIAL_PROOF_CONSISTENT")
	for _, want := range []string{"example/DNSKEY", "example/NSEC3PARAM", ds23Probe + "/A"} {
		if got := f.counts[want]; got != 1 {
			t.Fatalf("%s questions = %d, want 1", want, got)
		}
	}
}
