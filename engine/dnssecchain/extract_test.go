package dnssecchain

import (
	"context"
	"encoding/json"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	dns "codeberg.org/miekg/dns"
	"codeberg.org/miekg/dns/dnsutil"

	"codeberg.org/pawal/gonemaster/engine/dnsname"
	"codeberg.org/pawal/gonemaster/engine/internal/dnstest"
	"codeberg.org/pawal/gonemaster/engine/internal/testhelpers"
	"codeberg.org/pawal/gonemaster/engine/nameserver"
	"codeberg.org/pawal/gonemaster/engine/packet"
)

const (
	testZone   = "example.test"
	testParent = "test"
)

var fixedAt = time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)

// genKey generates an ECDSA P-256 zone key for owner.
func genKey(t *testing.T, owner string, sep bool) dnstest.Keypair {
	t.Helper()
	return dnstest.GenKey(t, owner, dns.ECDSAP256SHA256, sep)
}

func dnssecAnswer(owner string, qtype uint16, answers ...dns.RR) packet.Packet {
	return dnstest.Response(dnstest.Question(owner, qtype), dnstest.Reply(),
		dnstest.Answers(answers...), dnstest.Secure())
}

type hookFn func(qname, qtype string, opts *nameserver.QueryOptions) packet.Packet

func hookedNS(t *testing.T, ctx context.Context, name, ip string, hook hookFn) nameserver.Nameserver {
	t.Helper()
	ns, err := nameserver.NewWithContext(ctx, name, ip, nil)
	if err != nil {
		t.Fatalf("new nameserver %s: %v", ip, err)
	}
	ns.SetQueryHook(func(_ context.Context, qname, qtype, _ string, opts *nameserver.QueryOptions) (packet.Packet, error) {
		return hook(qname, qtype, opts), nil
	})
	return ns
}

// warm populates the cache for (name,qtype) as a testcase would, so the
// extractor's cache-only read hits the same key.
func warm(t *testing.T, ctx context.Context, ns nameserver.Nameserver, name, qtype string) {
	t.Helper()
	on := true
	if _, err := ns.QueryWithOptions(ctx, name, qtype, &nameserver.QueryOptions{DNSSEC: &on}); err != nil {
		t.Fatalf("warm %s %s: %v", name, qtype, err)
	}
}

func findLink(s *Summary, dsKeyTag uint16) (Link, bool) {
	for _, l := range s.Links {
		if l.DSKeyTag == dsKeyTag {
			return l, true
		}
	}
	return Link{}, false
}

func findDNSKEYSig(s *Summary, keytag uint16) (RRSIG, bool) {
	for _, sig := range s.Child.DNSKEYRRSIG {
		if sig.KeyTag == keytag {
			return sig, true
		}
	}
	return RRSIG{}, false
}

// secureFixture wires a fully signed delegation. When mutate is non-nil it can
// alter records or the DS before packets are built.
type fixtureOpts struct {
	badDigest        bool // corrupt the DS digest -> digest_mismatch
	wrongDSAlgo      bool // DS algorithm field disagrees with the key -> algorithm_mismatch
	extraWrongAlgoDS bool // add a second DS with a wrong algorithm field next to the good one
	expiredKeySig    bool // child DNSKEY RRSIG expired
	noDS             bool // parent serves no DS (island)
	noDNSKEY         bool // child serves no DNSKEY
}

func buildInput(t *testing.T, ctx context.Context, opts fixtureOpts) Input {
	t.Helper()

	childKSK := genKey(t, testZone, true)
	childZSK := genKey(t, testZone, false)
	parentKSK := genKey(t, testParent, true)

	dnskeyRRset := []dns.RR{childKSK.Key, childZSK.Key}
	keyInception := fixedAt.Add(-24 * time.Hour)
	keyExpiration := fixedAt.Add(24 * time.Hour)
	if opts.expiredKeySig {
		keyInception = fixedAt.Add(-48 * time.Hour)
		keyExpiration = fixedAt.Add(-24 * time.Hour)
	}
	dnskeySig := dnstest.SignRRset(t, childKSK, dnskeyRRset, testZone, testZone, keyInception, keyExpiration)

	ds := childKSK.Key.ToDS(dns.SHA256)
	if ds == nil {
		t.Fatalf("child KSK ToDS returned nil")
	}
	if opts.badDigest {
		ds.Digest = strings.Repeat("00", len(ds.Digest)/2)
	}
	if opts.wrongDSAlgo {
		ds.Algorithm = 253
	}
	dsRRset := []dns.RR{ds}
	if opts.extraWrongAlgoDS {
		wrong := *ds
		wrong.Algorithm = 253
		dsRRset = append(dsRRset, &wrong)
	}
	dsSig := dnstest.SignRRset(t, parentKSK, dsRRset, testZone, testParent, fixedAt.Add(-24*time.Hour), fixedAt.Add(24*time.Hour))

	parentKeyRRset := []dns.RR{parentKSK.Key}
	parentKeySig := dnstest.SignRRset(t, parentKSK, parentKeyRRset, testParent, testParent, fixedAt.Add(-24*time.Hour), fixedAt.Add(24*time.Hour))

	childHook := func(qname, qtype string, _ *nameserver.QueryOptions) packet.Packet {
		switch qtype {
		case "DNSKEY":
			if opts.noDNSKEY {
				return dnssecAnswer(testZone, dns.TypeDNSKEY)
			}
			return dnssecAnswer(testZone, dns.TypeDNSKEY, childKSK.Key, childZSK.Key, dnskeySig)
		}
		return packet.Packet{}
	}
	parentHook := func(qname, qtype string, _ *nameserver.QueryOptions) packet.Packet {
		switch qtype {
		case "DS":
			if opts.noDS {
				return dnssecAnswer(testZone, dns.TypeDS)
			}
			answers := append(append([]dns.RR{}, dsRRset...), dsSig)
			return dnssecAnswer(testZone, dns.TypeDS, answers...)
		case "DNSKEY":
			if qname == dnsutil.Fqdn(testParent) || qname == testParent {
				return dnssecAnswer(testParent, dns.TypeDNSKEY, parentKSK.Key, parentKeySig)
			}
		}
		return packet.Packet{}
	}

	childNS := hookedNS(t, ctx, "ns1."+testZone, "203.0.113.1", childHook)
	parentNS := hookedNS(t, ctx, "ns1."+testParent, "192.0.2.1", parentHook)

	warm(t, ctx, childNS, testZone, "DNSKEY")
	warm(t, ctx, parentNS, testZone, "DS")
	warm(t, ctx, parentNS, testParent, "DNSKEY")

	return Input{
		Zone:       dnsname.New(testZone),
		ParentZone: dnsname.New(testParent),
		ChildNS:    []nameserver.Nameserver{childNS},
		ParentNS:   []nameserver.Nameserver{parentNS},
		At:         fixedAt,
	}
}

func TestExtractSecure(t *testing.T) {
	ctx, _, _ := testhelpers.Context(t)
	in := buildInput(t, ctx, fixtureOpts{})

	got := Extract(ctx, in)
	if got == nil {
		t.Fatal("expected a summary, got nil")
	}
	if got.Status != StatusSecure {
		t.Errorf("status = %q, want %q", got.Status, StatusSecure)
	}
	if got.Version != Version {
		t.Errorf("version = %d, want %d", got.Version, Version)
	}
	if got.Delegation != DelegationNormal {
		t.Errorf("delegation = %q, want normal", got.Delegation)
	}
	if got.Parent.DSSource != DSSourceParent {
		t.Errorf("ds_source = %q, want parent", got.Parent.DSSource)
	}
	if len(got.Parent.DS) != 1 {
		t.Fatalf("want 1 DS, got %d", len(got.Parent.DS))
	}
	// The fixture records use TTL 3600; DS and DNSKEY carry the RRset TTL.
	if got.Parent.DS[0].TTL != 3600 {
		t.Errorf("DS TTL = %d, want 3600", got.Parent.DS[0].TTL)
	}
	if len(got.Child.DNSKEYs) != 2 {
		t.Fatalf("want 2 DNSKEYs, got %d", len(got.Child.DNSKEYs))
	}
	if got.Child.DNSKEYs[0].TTL != 3600 {
		t.Errorf("DNSKEY TTL = %d, want 3600", got.Child.DNSKEYs[0].TTL)
	}
	if !got.Child.DNSKEYs[0].SEP {
		t.Errorf("expected SEP (KSK) key ordered first")
	}
	ksk := got.Child.DNSKEYs[0]
	link, ok := findLink(got, ksk.KeyTag)
	if !ok || link.Status != LinkMatch {
		t.Errorf("expected matching link for KSK %d, got %+v", ksk.KeyTag, link)
	}
	sig, ok := findDNSKEYSig(got, ksk.KeyTag)
	if !ok || sig.State != SigValid {
		t.Errorf("expected valid DNSKEY RRSIG for KSK, got %+v", sig)
	}
	if !ksk.Anchored {
		t.Error("expected the DS-matched KSK to be anchored")
	}
	if len(got.Parent.DSRRSIG) != 1 || got.Parent.DSRRSIG[0].State != SigValid {
		t.Errorf("expected 1 valid DS RRSIG, got %+v", got.Parent.DSRRSIG)
	}
	// The parent key that signs the DS RRset is captured, matching the DS RRSIG.
	if len(got.Parent.DNSKEYs) != 1 {
		t.Fatalf("want 1 parent DNSKEY (the DS signer), got %d", len(got.Parent.DNSKEYs))
	}
	if got.Parent.DNSKEYs[0].KeyTag != got.Parent.DSRRSIG[0].KeyTag {
		t.Errorf("parent DNSKEY keytag %d != DS RRSIG signer %d", got.Parent.DNSKEYs[0].KeyTag, got.Parent.DSRRSIG[0].KeyTag)
	}
}

func TestExtractSignedRRsets(t *testing.T) {
	ctx, _, _ := testhelpers.Context(t)

	childKSK := genKey(t, testZone, true)
	childZSK := genKey(t, testZone, false)
	dnskeyRRset := []dns.RR{childKSK.Key, childZSK.Key}
	win := func(rr []dns.RR, signer dnstest.Keypair) *dns.RRSIG {
		return dnstest.SignRRset(t, signer, rr, testZone, testZone, fixedAt.Add(-24*time.Hour), fixedAt.Add(24*time.Hour))
	}
	dnskeySig := win(dnskeyRRset, childKSK)

	// SOA signed by the ZSK, CDS (from the KSK) signed by the KSK.
	soa := &dns.SOA{Hdr: dns.Header{Name: dnsutil.Fqdn(testZone), Class: dns.ClassINET, TTL: 3600}}
	soa.Ns = dnsutil.Fqdn("ns1." + testZone)
	soa.Mbox = dnsutil.Fqdn("hostmaster." + testZone)
	soa.Serial = 1
	soa.Refresh = 3600
	soa.Retry = 600
	soa.Expire = 604800
	soa.Minttl = 3600
	soaSig := win([]dns.RR{soa}, childZSK)

	ds := childKSK.Key.ToDS(dns.SHA256)
	cds := &dns.CDS{DS: *ds}
	cdsSig := win([]dns.RR{cds}, childKSK)

	childHook := func(_, qtype string, _ *nameserver.QueryOptions) packet.Packet {
		switch qtype {
		case "DNSKEY":
			return dnssecAnswer(testZone, dns.TypeDNSKEY, childKSK.Key, childZSK.Key, dnskeySig)
		case "SOA":
			return dnssecAnswer(testZone, dns.TypeSOA, soa, soaSig)
		case "CDS":
			return dnssecAnswer(testZone, dns.TypeCDS, cds, cdsSig)
		}
		return packet.Packet{}
	}
	parentHook := func(_, qtype string, _ *nameserver.QueryOptions) packet.Packet {
		if qtype == "DS" {
			return dnssecAnswer(testZone, dns.TypeDS, ds, win([]dns.RR{ds}, childKSK))
		}
		return packet.Packet{}
	}
	childNS := hookedNS(t, ctx, "ns1."+testZone, "203.0.113.1", childHook)
	parentNS := hookedNS(t, ctx, "ns1."+testParent, "192.0.2.1", parentHook)

	warm(t, ctx, childNS, testZone, "DNSKEY")
	warm(t, ctx, childNS, testZone, "SOA")
	warm(t, ctx, childNS, testZone, "CDS")
	warm(t, ctx, parentNS, testZone, "DS")

	in := Input{
		Zone:       dnsname.New(testZone),
		ParentZone: dnsname.New(testParent),
		ChildNS:    []nameserver.Nameserver{childNS},
		ParentNS:   []nameserver.Nameserver{parentNS},
		At:         fixedAt,
	}
	got := Extract(ctx, in)
	if got == nil {
		t.Fatal("expected a summary")
	}
	if len(got.Child.Signed) != 2 {
		t.Fatalf("expected 2 signed RRsets, got %d: %+v", len(got.Child.Signed), got.Child.Signed)
	}
	// SOA comes before CDS, matching the fixed display order.
	if got.Child.Signed[0].Type != "SOA" || got.Child.Signed[1].Type != "CDS" {
		t.Fatalf("unexpected signed order: %s, %s", got.Child.Signed[0].Type, got.Child.Signed[1].Type)
	}
	if got.Child.Signed[0].TTL != 3600 {
		t.Errorf("SOA RRset TTL = %d, want 3600", got.Child.Signed[0].TTL)
	}
	for _, s := range got.Child.Signed {
		if len(s.RRSIG) != 1 || s.RRSIG[0].State != SigValid {
			t.Errorf("%s: expected 1 valid RRSIG, got %+v", s.Type, s.RRSIG)
		}
	}
	// The CDS record names the KSK by tag; SOA carries no ref.
	cdsEntry := got.Child.Signed[1]
	if len(cdsEntry.Refs) != 1 || cdsEntry.Refs[0] != childKSK.Key.KeyTag() {
		t.Errorf("CDS refs = %v, want [%d]", cdsEntry.Refs, childKSK.Key.KeyTag())
	}
	// CDS names the same key the parent DS anchors: a steady-state match.
	if cdsEntry.DSMatch != CDSMatchExact || len(cdsEntry.NewKeys) != 0 {
		t.Errorf("CDS ds_match = %q new_keys = %v, want match/none", cdsEntry.DSMatch, cdsEntry.NewKeys)
	}
	if len(got.Child.Signed[0].Refs) != 0 {
		t.Errorf("SOA should carry no refs, got %v", got.Child.Signed[0].Refs)
	}
}

func TestExtractCDSRolloverSignaled(t *testing.T) {
	// A KSK rollover in progress: the parent DS anchors only the old KSK, but
	// CDS/CDNSKEY name the old and a new incoming KSK. The extractor must flag
	// the new key tag as signaled-but-unanchored, mirroring DNSSEC18.
	ctx, _, _ := testhelpers.Context(t)

	oldKSK := genKey(t, testZone, true)
	newKSK := genKey(t, testZone, true)
	zsk := genKey(t, testZone, false)
	dnskeyRRset := []dns.RR{oldKSK.Key, newKSK.Key, zsk.Key}
	win := func(rr []dns.RR, signer dnstest.Keypair) *dns.RRSIG {
		return dnstest.SignRRset(t, signer, rr, testZone, testZone, fixedAt.Add(-24*time.Hour), fixedAt.Add(24*time.Hour))
	}
	dnskeySig := win(dnskeyRRset, oldKSK)

	// Parent publishes DS only for the old KSK.
	ds := oldKSK.Key.ToDS(dns.SHA256)

	cdsOld := &dns.CDS{DS: *oldKSK.Key.ToDS(dns.SHA256)}
	cdsNew := &dns.CDS{DS: *newKSK.Key.ToDS(dns.SHA256)}
	cdsSig := win([]dns.RR{cdsOld, cdsNew}, oldKSK)
	cdnskeyOld := &dns.CDNSKEY{DNSKEY: *oldKSK.Key}
	cdnskeyNew := &dns.CDNSKEY{DNSKEY: *newKSK.Key}
	cdnskeySig := win([]dns.RR{cdnskeyOld, cdnskeyNew}, oldKSK)

	childHook := func(_, qtype string, _ *nameserver.QueryOptions) packet.Packet {
		switch qtype {
		case "DNSKEY":
			return dnssecAnswer(testZone, dns.TypeDNSKEY, oldKSK.Key, newKSK.Key, zsk.Key, dnskeySig)
		case "CDS":
			return dnssecAnswer(testZone, dns.TypeCDS, cdsOld, cdsNew, cdsSig)
		case "CDNSKEY":
			return dnssecAnswer(testZone, dns.TypeCDNSKEY, cdnskeyOld, cdnskeyNew, cdnskeySig)
		}
		return packet.Packet{}
	}
	parentHook := func(_, qtype string, _ *nameserver.QueryOptions) packet.Packet {
		if qtype == "DS" {
			return dnssecAnswer(testZone, dns.TypeDS, ds, win([]dns.RR{ds}, oldKSK))
		}
		return packet.Packet{}
	}
	childNS := hookedNS(t, ctx, "ns1."+testZone, "203.0.113.1", childHook)
	parentNS := hookedNS(t, ctx, "ns1."+testParent, "192.0.2.1", parentHook)

	warm(t, ctx, childNS, testZone, "DNSKEY")
	warm(t, ctx, childNS, testZone, "CDS")
	warm(t, ctx, childNS, testZone, "CDNSKEY")
	warm(t, ctx, parentNS, testZone, "DS")

	got := Extract(ctx, Input{
		Zone:       dnsname.New(testZone),
		ParentZone: dnsname.New(testParent),
		ChildNS:    []nameserver.Nameserver{childNS},
		ParentNS:   []nameserver.Nameserver{parentNS},
		At:         fixedAt,
	})
	if got == nil {
		t.Fatal("expected a summary")
	}
	for _, s := range got.Child.Signed {
		if s.Type != "CDS" && s.Type != "CDNSKEY" {
			continue
		}
		if s.DSMatch != CDSMatchRollover {
			t.Errorf("%s ds_match = %q, want rollover", s.Type, s.DSMatch)
		}
		if len(s.NewKeys) != 1 || s.NewKeys[0] != newKSK.Key.KeyTag() {
			t.Errorf("%s new_keys = %v, want [%d]", s.Type, s.NewKeys, newKSK.Key.KeyTag())
		}
	}
}

func TestExtractAnchoredMarksOnlyDSMatchedKSK(t *testing.T) {
	// A double-signature KSK rollover: two KSKs both sign the DNSKEY RRset, but
	// the parent DS anchors only one. Only that KSK must be flagged anchored;
	// the incoming KSK is left unanchored.
	ctx, _, _ := testhelpers.Context(t)

	anchoredKSK := genKey(t, testZone, true)
	incomingKSK := genKey(t, testZone, true)
	zsk := genKey(t, testZone, false)
	dnskeyRRset := []dns.RR{anchoredKSK.Key, incomingKSK.Key, zsk.Key}
	win := func(rr []dns.RR, signer dnstest.Keypair) *dns.RRSIG {
		return dnstest.SignRRset(t, signer, rr, testZone, testZone, fixedAt.Add(-24*time.Hour), fixedAt.Add(24*time.Hour))
	}
	// Both KSKs sign the DNSKEY RRset (double-signature phase).
	sigAnchored := win(dnskeyRRset, anchoredKSK)
	sigIncoming := win(dnskeyRRset, incomingKSK)

	ds := anchoredKSK.Key.ToDS(dns.SHA256)
	childHook := func(_, qtype string, _ *nameserver.QueryOptions) packet.Packet {
		if qtype == "DNSKEY" {
			return dnssecAnswer(testZone, dns.TypeDNSKEY, anchoredKSK.Key, incomingKSK.Key, zsk.Key, sigAnchored, sigIncoming)
		}
		return packet.Packet{}
	}
	parentHook := func(_, qtype string, _ *nameserver.QueryOptions) packet.Packet {
		if qtype == "DS" {
			return dnssecAnswer(testZone, dns.TypeDS, ds, win([]dns.RR{ds}, anchoredKSK))
		}
		return packet.Packet{}
	}
	childNS := hookedNS(t, ctx, "ns1."+testZone, "203.0.113.1", childHook)
	parentNS := hookedNS(t, ctx, "ns1."+testParent, "192.0.2.1", parentHook)

	warm(t, ctx, childNS, testZone, "DNSKEY")
	warm(t, ctx, parentNS, testZone, "DS")

	got := Extract(ctx, Input{
		Zone:       dnsname.New(testZone),
		ParentZone: dnsname.New(testParent),
		ChildNS:    []nameserver.Nameserver{childNS},
		ParentNS:   []nameserver.Nameserver{parentNS},
		At:         fixedAt,
	})
	if got == nil {
		t.Fatal("expected a summary")
	}
	for _, k := range got.Child.DNSKEYs {
		want := k.KeyTag == anchoredKSK.Key.KeyTag()
		if k.Anchored != want {
			t.Errorf("key %d anchored = %v, want %v", k.KeyTag, k.Anchored, want)
		}
	}
	// Both KSK signatures over the DNSKEY RRset are recorded and valid.
	if len(got.Child.DNSKEYRRSIG) != 2 {
		t.Fatalf("expected 2 DNSKEY RRSIGs, got %d", len(got.Child.DNSKEYRRSIG))
	}
}

func TestExtractCDSNoParentDSLeavesMatchEmpty(t *testing.T) {
	// An island (keys and CDS but no DS at the parent) has nothing to compare
	// against, so ds_match stays empty rather than claiming a rollover.
	ctx, _, _ := testhelpers.Context(t)

	ksk := genKey(t, testZone, true)
	win := func(rr []dns.RR, signer dnstest.Keypair) *dns.RRSIG {
		return dnstest.SignRRset(t, signer, rr, testZone, testZone, fixedAt.Add(-24*time.Hour), fixedAt.Add(24*time.Hour))
	}
	dnskeySig := win([]dns.RR{ksk.Key}, ksk)
	cds := &dns.CDS{DS: *ksk.Key.ToDS(dns.SHA256)}
	cdsSig := win([]dns.RR{cds}, ksk)

	childHook := func(_, qtype string, _ *nameserver.QueryOptions) packet.Packet {
		switch qtype {
		case "DNSKEY":
			return dnssecAnswer(testZone, dns.TypeDNSKEY, ksk.Key, dnskeySig)
		case "CDS":
			return dnssecAnswer(testZone, dns.TypeCDS, cds, cdsSig)
		}
		return packet.Packet{}
	}
	parentHook := func(_, qtype string, _ *nameserver.QueryOptions) packet.Packet {
		if qtype == "DS" {
			return dnssecAnswer(testZone, dns.TypeDS) // NOERROR, no DS
		}
		return packet.Packet{}
	}
	childNS := hookedNS(t, ctx, "ns1."+testZone, "203.0.113.1", childHook)
	parentNS := hookedNS(t, ctx, "ns1."+testParent, "192.0.2.1", parentHook)

	warm(t, ctx, childNS, testZone, "DNSKEY")
	warm(t, ctx, childNS, testZone, "CDS")
	warm(t, ctx, parentNS, testZone, "DS")

	got := Extract(ctx, Input{
		Zone:       dnsname.New(testZone),
		ParentZone: dnsname.New(testParent),
		ChildNS:    []nameserver.Nameserver{childNS},
		ParentNS:   []nameserver.Nameserver{parentNS},
		At:         fixedAt,
	})
	if got == nil {
		t.Fatal("expected a summary")
	}
	for _, s := range got.Child.Signed {
		if s.Type == "CDS" && s.DSMatch != "" {
			t.Errorf("CDS ds_match = %q, want empty (no parent DS)", s.DSMatch)
		}
	}
}

func TestExtractDigestMismatch(t *testing.T) {
	ctx, _, _ := testhelpers.Context(t)
	in := buildInput(t, ctx, fixtureOpts{badDigest: true})

	got := Extract(ctx, in)
	if got == nil {
		t.Fatal("expected a summary")
	}
	if got.Status != StatusBroken {
		t.Errorf("status = %q, want broken", got.Status)
	}
	if len(got.Parent.DS) != 1 {
		t.Fatalf("want 1 DS, got %d", len(got.Parent.DS))
	}
	link, ok := findLink(got, got.Parent.DS[0].KeyTag)
	if !ok || link.Status != LinkDigestMismatch {
		t.Errorf("expected digest_mismatch link, got %+v", link)
	}
}

func TestExtractDSAlgorithmMismatch(t *testing.T) {
	// The parent's DS carries an algorithm field that disagrees with the
	// DNSKEY it was generated from, while key tag and digest both line up.
	// RFC 4034 section 5.2 makes the algorithm field part of the reference,
	// so validators ignore such a DS: the link must not count as a match,
	// no key may be anchored, and the chain must roll up broken.
	ctx, _, _ := testhelpers.Context(t)
	in := buildInput(t, ctx, fixtureOpts{wrongDSAlgo: true})

	got := Extract(ctx, in)
	if got == nil {
		t.Fatal("expected a summary")
	}
	if got.Status != StatusBroken {
		t.Errorf("status = %q, want broken", got.Status)
	}
	if len(got.Parent.DS) != 1 {
		t.Fatalf("want 1 DS, got %d", len(got.Parent.DS))
	}
	link, ok := findLink(got, got.Parent.DS[0].KeyTag)
	if !ok || link.Status != LinkAlgorithmMismatch {
		t.Errorf("expected algorithm_mismatch link, got %+v", link)
	}
	for _, k := range got.Child.DNSKEYs {
		if k.Anchored {
			t.Errorf("key %d must not be anchored by a mismatched DS", k.KeyTag)
		}
	}
}

func TestExtractDSAlgorithmMismatchWithValidSibling(t *testing.T) {
	// Two DS records reference the same KSK: one correct and one whose
	// algorithm field is wrong. A validator needs only one usable DS, so
	// the chain stays secure and the key anchored, while the unusable DS
	// still surfaces as an algorithm_mismatch link.
	ctx, _, _ := testhelpers.Context(t)
	in := buildInput(t, ctx, fixtureOpts{extraWrongAlgoDS: true})

	got := Extract(ctx, in)
	if got == nil {
		t.Fatal("expected a summary")
	}
	if got.Status != StatusSecure {
		t.Errorf("status = %q, want secure", got.Status)
	}
	if len(got.Parent.DS) != 2 {
		t.Fatalf("want 2 DS, got %d", len(got.Parent.DS))
	}
	statuses := map[string]int{}
	for _, l := range got.Links {
		statuses[l.Status]++
	}
	if statuses[LinkMatch] != 1 || statuses[LinkAlgorithmMismatch] != 1 {
		t.Errorf("links = %v, want one match and one algorithm_mismatch", statuses)
	}
	anchored := false
	for _, k := range got.Child.DNSKEYs {
		if k.SEP && k.Anchored {
			anchored = true
		}
	}
	if !anchored {
		t.Error("KSK must stay anchored via the correct DS")
	}
}

func TestExtractExpiredKeySignature(t *testing.T) {
	ctx, _, _ := testhelpers.Context(t)
	in := buildInput(t, ctx, fixtureOpts{expiredKeySig: true})

	got := Extract(ctx, in)
	if got == nil {
		t.Fatal("expected a summary")
	}
	// The DS still matches a key, but no valid DNSKEY signature -> broken.
	if got.Status != StatusBroken {
		t.Errorf("status = %q, want broken", got.Status)
	}
	if len(got.Child.DNSKEYRRSIG) == 0 {
		t.Fatal("expected a DNSKEY RRSIG")
	}
	for _, sig := range got.Child.DNSKEYRRSIG {
		if sig.State != SigExpired {
			t.Errorf("DNSKEY RRSIG state = %q, want expired", sig.State)
		}
	}
}

func TestExtractIsland(t *testing.T) {
	ctx, _, _ := testhelpers.Context(t)
	in := buildInput(t, ctx, fixtureOpts{noDS: true})

	got := Extract(ctx, in)
	if got == nil {
		t.Fatal("expected a summary")
	}
	if got.Status != StatusIsland {
		t.Errorf("status = %q, want island", got.Status)
	}
	if got.Parent.DSSource != DSSourceNone {
		t.Errorf("ds_source = %q, want none", got.Parent.DSSource)
	}
	if len(got.Parent.ServersWithoutDS) != 1 {
		t.Errorf("expected 1 server_without_ds, got %v", got.Parent.ServersWithoutDS)
	}
	if len(got.Child.DNSKEYs) == 0 {
		t.Error("expected child DNSKEYs")
	}
}

func TestExtractUnsigned(t *testing.T) {
	ctx, _, _ := testhelpers.Context(t)
	// Parent serves no DS and the child serves an authoritative empty DNSKEY
	// answer: the DNSSEC07 unsigned shape.
	in := buildInput(t, ctx, fixtureOpts{noDS: true, noDNSKEY: true})

	got := Extract(ctx, in)
	if got == nil {
		t.Fatal("expected a summary (positive unsigned evidence)")
	}
	if got.Status != StatusUnsigned {
		t.Errorf("status = %q, want unsigned", got.Status)
	}
	if len(got.Child.DNSKEYs) != 0 {
		t.Errorf("expected no DNSKEYs, got %d", len(got.Child.DNSKEYs))
	}
	if len(got.Child.ServersWithoutDNSKEY) != 1 {
		t.Errorf("expected 1 server_without_dnskey, got %v", got.Child.ServersWithoutDNSKEY)
	}
}

func TestExtractColdCacheReturnsNil(t *testing.T) {
	ctx, _, _ := testhelpers.Context(t)

	// Nameservers exist but nothing was warmed, so every cache-only read misses.
	childNS := hookedNS(t, ctx, "ns1."+testZone, "203.0.113.1", func(_, _ string, _ *nameserver.QueryOptions) packet.Packet {
		return packet.Packet{}
	})
	parentNS := hookedNS(t, ctx, "ns1."+testParent, "192.0.2.1", func(_, _ string, _ *nameserver.QueryOptions) packet.Packet {
		return packet.Packet{}
	})
	in := Input{
		Zone:       dnsname.New(testZone),
		ParentZone: dnsname.New(testParent),
		ChildNS:    []nameserver.Nameserver{childNS},
		ParentNS:   []nameserver.Nameserver{parentNS},
		At:         fixedAt,
	}

	if got := Extract(ctx, in); got != nil {
		t.Errorf("cold cache: expected nil, got status %q", got.Status)
	}
}

func TestExtractUndelegatedFakeDS(t *testing.T) {
	ctx, _, _ := testhelpers.Context(t)

	childKSK := genKey(t, testZone, true)
	childZSK := genKey(t, testZone, false)
	dnskeyRRset := []dns.RR{childKSK.Key, childZSK.Key}
	dnskeySig := dnstest.SignRRset(t, childKSK, dnskeyRRset, testZone, testZone, fixedAt.Add(-24*time.Hour), fixedAt.Add(24*time.Hour))
	ds := childKSK.Key.ToDS(dns.SHA256)

	childHook := func(qname, qtype string, _ *nameserver.QueryOptions) packet.Packet {
		if qtype == "DNSKEY" {
			return dnssecAnswer(testZone, dns.TypeDNSKEY, childKSK.Key, childZSK.Key, dnskeySig)
		}
		return packet.Packet{}
	}
	childNS := hookedNS(t, ctx, "ns1."+testZone, "203.0.113.1", childHook)
	parentNS := hookedNS(t, ctx, "ns1."+testParent, "192.0.2.1", func(_, _ string, _ *nameserver.QueryOptions) packet.Packet {
		return packet.Packet{}
	})

	// Attach fake DS to the parent server, as an undelegated run does.
	nsPtr := &parentNS
	if err := nsPtr.AddFakeDS(testZone, []nameserver.DSData{{
		KeyTag:     ds.KeyTag,
		Algorithm:  ds.Algorithm,
		DigestType: ds.DigestType,
		Digest:     ds.Digest,
	}}); err != nil {
		t.Fatalf("add fake DS: %v", err)
	}

	warm(t, ctx, childNS, testZone, "DNSKEY")

	in := Input{
		Zone:       dnsname.New(testZone),
		ParentZone: dnsname.New(testParent),
		ChildNS:    []nameserver.Nameserver{childNS},
		ParentNS:   []nameserver.Nameserver{parentNS},
		At:         fixedAt,
	}
	got := Extract(ctx, in)
	if got == nil {
		t.Fatal("expected a summary")
	}
	if got.Delegation != DelegationUndelegated {
		t.Errorf("delegation = %q, want undelegated", got.Delegation)
	}
	if got.Parent.DSSource != DSSourceInput {
		t.Errorf("ds_source = %q, want input", got.Parent.DSSource)
	}
	if len(got.Parent.DS) != 1 || len(got.Parent.DS[0].Servers) != 1 || got.Parent.DS[0].Servers[0] != "-" {
		t.Errorf("expected DS with servers [-], got %+v", got.Parent.DS)
	}
	if len(got.Parent.DSRRSIG) != 0 {
		t.Errorf("fake DS has no signatures, got %+v", got.Parent.DSRRSIG)
	}
	link, ok := findLink(got, ds.KeyTag)
	if !ok || link.Status != LinkMatch {
		t.Errorf("expected matching link for input DS, got %+v", link)
	}
}

func TestExtractPerServerDisagreement(t *testing.T) {
	ctx, _, _ := testhelpers.Context(t)

	childKSK := genKey(t, testZone, true)
	childZSK := genKey(t, testZone, false)
	dnskeyRRset := []dns.RR{childKSK.Key, childZSK.Key}
	dnskeySig := dnstest.SignRRset(t, childKSK, dnskeyRRset, testZone, testZone, fixedAt.Add(-24*time.Hour), fixedAt.Add(24*time.Hour))
	ds := childKSK.Key.ToDS(dns.SHA256)
	dsSig := dnstest.SignRRset(t, childKSK, []dns.RR{ds}, testZone, testParent, fixedAt.Add(-24*time.Hour), fixedAt.Add(24*time.Hour))

	childHook := func(_, qtype string, _ *nameserver.QueryOptions) packet.Packet {
		if qtype == "DNSKEY" {
			return dnssecAnswer(testZone, dns.TypeDNSKEY, childKSK.Key, childZSK.Key, dnskeySig)
		}
		return packet.Packet{}
	}
	// One parent server serves the DS, the other serves an empty DS answer.
	parentWithDS := hookedNS(t, ctx, "p1."+testParent, "192.0.2.1", func(_, qtype string, _ *nameserver.QueryOptions) packet.Packet {
		if qtype == "DS" {
			return dnssecAnswer(testZone, dns.TypeDS, ds, dsSig)
		}
		return packet.Packet{}
	})
	parentNoDS := hookedNS(t, ctx, "p2."+testParent, "192.0.2.2", func(_, qtype string, _ *nameserver.QueryOptions) packet.Packet {
		if qtype == "DS" {
			return dnssecAnswer(testZone, dns.TypeDS)
		}
		return packet.Packet{}
	})
	childNS := hookedNS(t, ctx, "ns1."+testZone, "203.0.113.1", childHook)

	warm(t, ctx, childNS, testZone, "DNSKEY")
	warm(t, ctx, parentWithDS, testZone, "DS")
	warm(t, ctx, parentNoDS, testZone, "DS")

	in := Input{
		Zone:       dnsname.New(testZone),
		ParentZone: dnsname.New(testParent),
		ChildNS:    []nameserver.Nameserver{childNS},
		ParentNS:   []nameserver.Nameserver{parentWithDS, parentNoDS},
		At:         fixedAt,
	}
	got := Extract(ctx, in)
	if got == nil {
		t.Fatal("expected a summary")
	}
	if len(got.Parent.ServersQueried) != 2 {
		t.Errorf("expected 2 servers queried, got %v", got.Parent.ServersQueried)
	}
	if len(got.Parent.ServersDisagreeing) != 1 {
		t.Errorf("expected 1 disagreeing server, got %v", got.Parent.ServersDisagreeing)
	}
	if len(got.Parent.ServersWithoutDS) != 1 || got.Parent.ServersWithoutDS[0] != "192.0.2.2" {
		t.Errorf("expected 192.0.2.2 without DS, got %v", got.Parent.ServersWithoutDS)
	}
}

func TestExtractNoQueryGuarantee(t *testing.T) {
	ctx, _, _ := testhelpers.Context(t)

	var calls int64
	childKSK := genKey(t, testZone, true)
	childZSK := genKey(t, testZone, false)
	dnskeyRRset := []dns.RR{childKSK.Key, childZSK.Key}
	dnskeySig := dnstest.SignRRset(t, childKSK, dnskeyRRset, testZone, testZone, fixedAt.Add(-24*time.Hour), fixedAt.Add(24*time.Hour))
	ds := childKSK.Key.ToDS(dns.SHA256)
	dsSig := dnstest.SignRRset(t, childKSK, []dns.RR{ds}, testZone, testParent, fixedAt.Add(-24*time.Hour), fixedAt.Add(24*time.Hour))

	countingChild := func(_, qtype string, _ *nameserver.QueryOptions) packet.Packet {
		atomic.AddInt64(&calls, 1)
		if qtype == "DNSKEY" {
			return dnssecAnswer(testZone, dns.TypeDNSKEY, childKSK.Key, childZSK.Key, dnskeySig)
		}
		return packet.Packet{}
	}
	countingParent := func(_, qtype string, _ *nameserver.QueryOptions) packet.Packet {
		atomic.AddInt64(&calls, 1)
		if qtype == "DS" {
			return dnssecAnswer(testZone, dns.TypeDS, ds, dsSig)
		}
		return packet.Packet{}
	}
	childNS := hookedNS(t, ctx, "ns1."+testZone, "203.0.113.1", countingChild)
	parentNS := hookedNS(t, ctx, "ns1."+testParent, "192.0.2.1", countingParent)

	warm(t, ctx, childNS, testZone, "DNSKEY")
	warm(t, ctx, parentNS, testZone, "DS")
	warm(t, ctx, parentNS, testParent, "DNSKEY")
	before := atomic.LoadInt64(&calls)

	in := Input{
		Zone:       dnsname.New(testZone),
		ParentZone: dnsname.New(testParent),
		ChildNS:    []nameserver.Nameserver{childNS},
		ParentNS:   []nameserver.Nameserver{parentNS},
		At:         fixedAt,
	}
	got := Extract(ctx, in)
	if got == nil {
		t.Fatal("expected a summary from warm cache")
	}
	if after := atomic.LoadInt64(&calls); after != before {
		t.Errorf("extractor issued %d queries; must be cache-only", after-before)
	}
}

func TestExtractCacheKeyParity(t *testing.T) {
	ctx, _, _ := testhelpers.Context(t)

	childKSK := genKey(t, testZone, true)
	childZSK := genKey(t, testZone, false)
	dnskeyRRset := []dns.RR{childKSK.Key, childZSK.Key}
	dnskeySig := dnstest.SignRRset(t, childKSK, dnskeyRRset, testZone, testZone, fixedAt.Add(-24*time.Hour), fixedAt.Add(24*time.Hour))
	ds := childKSK.Key.ToDS(dns.SHA256)
	dsSig := dnstest.SignRRset(t, childKSK, []dns.RR{ds}, testZone, testParent, fixedAt.Add(-24*time.Hour), fixedAt.Add(24*time.Hour))

	childNS := hookedNS(t, ctx, "ns1."+testZone, "203.0.113.1", func(_, qtype string, _ *nameserver.QueryOptions) packet.Packet {
		if qtype == "DNSKEY" {
			return dnssecAnswer(testZone, dns.TypeDNSKEY, childKSK.Key, childZSK.Key, dnskeySig)
		}
		return packet.Packet{}
	})
	parentNS := hookedNS(t, ctx, "ns1."+testParent, "192.0.2.1", func(_, qtype string, _ *nameserver.QueryOptions) packet.Packet {
		if qtype == "DS" {
			return dnssecAnswer(testZone, dns.TypeDS, ds, dsSig)
		}
		return packet.Packet{}
	})

	// Warm with the testcase-style options (explicit UseVC=false) rather than
	// the extractor style; parity means the extractor still hits.
	on := true
	useVC := false
	opts := &nameserver.QueryOptions{DNSSEC: &on, UseVC: &useVC}
	if _, err := childNS.QueryWithOptions(ctx, testZone, "DNSKEY", opts); err != nil {
		t.Fatalf("warm DNSKEY: %v", err)
	}
	if _, err := parentNS.QueryWithOptions(ctx, testZone, "DS", opts); err != nil {
		t.Fatalf("warm DS: %v", err)
	}

	in := Input{
		Zone:       dnsname.New(testZone),
		ParentZone: dnsname.New(testParent),
		ChildNS:    []nameserver.Nameserver{childNS},
		ParentNS:   []nameserver.Nameserver{parentNS},
		At:         fixedAt,
	}
	got := Extract(ctx, in)
	if got == nil {
		t.Fatal("cache-key parity broken: extractor read nothing")
	}
	if len(got.Parent.DS) != 1 {
		t.Errorf("expected DS via parity read, got %d", len(got.Parent.DS))
	}
	if len(got.Child.DNSKEYs) != 2 {
		t.Errorf("expected DNSKEYs via parity read, got %d", len(got.Child.DNSKEYs))
	}
}

func TestExtractCapsTruncate(t *testing.T) {
	ctx, _, _ := testhelpers.Context(t)

	// Build a DS answer with more than maxDS records to trip the cap.
	var dsRecords []dns.RR
	for i := 0; i < maxDS+5; i++ {
		ds := &dns.DS{Hdr: dns.Header{Name: dnsutil.Fqdn(testZone), Class: dns.ClassINET, TTL: 3600}}
		ds.KeyTag = uint16(1000 + i)
		ds.Algorithm = dns.ECDSAP256SHA256
		ds.DigestType = dns.SHA256
		ds.Digest = strings.Repeat("ab", 32)
		dsRecords = append(dsRecords, ds)
	}

	childNS := hookedNS(t, ctx, "ns1."+testZone, "203.0.113.1", func(_, qtype string, _ *nameserver.QueryOptions) packet.Packet {
		return packet.Packet{}
	})
	parentNS := hookedNS(t, ctx, "ns1."+testParent, "192.0.2.1", func(_, qtype string, _ *nameserver.QueryOptions) packet.Packet {
		if qtype == "DS" {
			return dnssecAnswer(testZone, dns.TypeDS, dsRecords...)
		}
		return packet.Packet{}
	})

	warm(t, ctx, childNS, testZone, "DNSKEY")
	warm(t, ctx, parentNS, testZone, "DS")

	in := Input{
		Zone:       dnsname.New(testZone),
		ParentZone: dnsname.New(testParent),
		ChildNS:    []nameserver.Nameserver{childNS},
		ParentNS:   []nameserver.Nameserver{parentNS},
		At:         fixedAt,
	}
	got := Extract(ctx, in)
	if got == nil {
		t.Fatal("expected a summary")
	}
	if len(got.Parent.DS) != maxDS {
		t.Errorf("DS not capped: got %d, want %d", len(got.Parent.DS), maxDS)
	}
	if !got.Truncated {
		t.Error("expected truncated = true when a cap is hit")
	}
}

func TestDSLinkStatusMatchesAnyKeyWithTag(t *testing.T) {
	// Key tags are a 16-bit checksum and can collide between distinct keys.
	// The DS below is derived from the second candidate; matching must try
	// every key with the tag instead of only the first one seen.
	k1 := genKey(t, testZone, true)
	k2 := genKey(t, testZone, true)
	real := k2.Key.ToDS(dns.SHA256)
	if real == nil {
		t.Fatal("ToDS returned nil")
	}
	ds := DS{KeyTag: real.KeyTag, Algorithm: real.Algorithm, DigestType: real.DigestType, Digest: strings.ToLower(real.Digest)}
	if got := dsLinkStatus(ds, []*dns.DNSKEY{k1.Key, k2.Key}); got != LinkMatch {
		t.Errorf("status = %q, want %q", got, LinkMatch)
	}
	if got := dsLinkStatus(ds, []*dns.DNSKEY{k1.Key}); got != LinkDigestMismatch {
		t.Errorf("status = %q, want %q", got, LinkDigestMismatch)
	}
}

func TestDSLinkStatusAlgorithmMismatchPrecedence(t *testing.T) {
	// Mixed candidate set under one key tag: one key matches the DS
	// algorithm but not its digest, the other matches the digest but not
	// the algorithm. The digest match proves which key the DS was
	// generated from, so the actionable verdict is algorithm_mismatch,
	// not digest_mismatch.
	right := genKey(t, testZone, true)
	wrong := genKey(t, testZone, true)
	wrong.Key.Algorithm = dns.RSASHA256 // rewrite before deriving the DS
	real := wrong.Key.ToDS(dns.SHA256)
	if real == nil {
		t.Fatal("ToDS returned nil")
	}
	ds := DS{KeyTag: real.KeyTag, Algorithm: dns.ECDSAP256SHA256, DigestType: real.DigestType, Digest: strings.ToLower(real.Digest)}
	if got := dsLinkStatus(ds, []*dns.DNSKEY{right.Key, wrong.Key}); got != LinkAlgorithmMismatch {
		t.Errorf("status = %q, want %q", got, LinkAlgorithmMismatch)
	}
	// A lone wrong-algorithm key with a matching digest classifies the same.
	if got := dsLinkStatus(ds, []*dns.DNSKEY{wrong.Key}); got != LinkAlgorithmMismatch {
		t.Errorf("status = %q, want %q", got, LinkAlgorithmMismatch)
	}
}

func TestDSLinkStatusGOSTDigestUnsupported(t *testing.T) {
	// Digest type 3 (GOST) is a known IANA type but the DNS library cannot
	// compute it (ToDS returns nil). That must classify as unsupported_digest,
	// never as digest_mismatch: we cannot prove a mismatch we cannot compute.
	k := genKey(t, testZone, true)
	ds := DS{KeyTag: k.Key.KeyTag(), Algorithm: k.Key.Algorithm, DigestType: 3, Digest: "abcd"}
	if got := dsLinkStatus(ds, []*dns.DNSKEY{k.Key}); got != LinkUnsupportedDigest {
		t.Errorf("status = %q, want %q", got, LinkUnsupportedDigest)
	}
}

func TestExtractKeySizeAndLinkDigestType(t *testing.T) {
	ctx, _, _ := testhelpers.Context(t)
	in := buildInput(t, ctx, fixtureOpts{})
	got := Extract(ctx, in)
	if got == nil {
		t.Fatal("expected a summary")
	}
	// The fixture keys are ECDSA P-256: key_size must be the curve size, not
	// a bogus value from parsing the EC point as an RSA modulus.
	for _, k := range got.Child.DNSKEYs {
		if k.KeySize != 256 {
			t.Errorf("key %d size = %d, want 256", k.KeyTag, k.KeySize)
		}
	}
	// Links carry the DS digest type so the UI can tell dual-digest DS
	// records for the same key tag apart.
	link, ok := findLink(got, got.Parent.DS[0].KeyTag)
	if !ok {
		t.Fatal("expected a link for the DS")
	}
	if link.DSDigestType != got.Parent.DS[0].DigestType {
		t.Errorf("link ds_digest_type = %d, want %d", link.DSDigestType, got.Parent.DS[0].DigestType)
	}
}

func TestExtractIndeterminateWithoutChildEvidence(t *testing.T) {
	// DS cached at the parent but no child DNSKEY answer cached at all: there
	// is no positive evidence about the child, so the roll-up must be
	// indeterminate rather than claiming the chain is broken.
	ctx, _, _ := testhelpers.Context(t)

	childKSK := genKey(t, testZone, true)
	ds := childKSK.Key.ToDS(dns.SHA256)
	if ds == nil {
		t.Fatal("ToDS returned nil")
	}
	dsSig := dnstest.SignRRset(t, childKSK, []dns.RR{ds}, testZone, testParent, fixedAt.Add(-24*time.Hour), fixedAt.Add(24*time.Hour))

	parentHook := func(_, qtype string, _ *nameserver.QueryOptions) packet.Packet {
		if qtype == "DS" {
			return dnssecAnswer(testZone, dns.TypeDS, ds, dsSig)
		}
		return packet.Packet{}
	}
	childNS := hookedNS(t, ctx, "ns1."+testZone, "203.0.113.1", func(_, _ string, _ *nameserver.QueryOptions) packet.Packet {
		return packet.Packet{}
	})
	parentNS := hookedNS(t, ctx, "ns1."+testParent, "192.0.2.1", parentHook)
	warm(t, ctx, parentNS, testZone, "DS")
	// The child cache is deliberately left cold.

	got := Extract(ctx, Input{
		Zone:       dnsname.New(testZone),
		ParentZone: dnsname.New(testParent),
		ChildNS:    []nameserver.Nameserver{childNS},
		ParentNS:   []nameserver.Nameserver{parentNS},
		At:         fixedAt,
	})
	if got == nil {
		t.Fatal("expected a summary")
	}
	if got.Status != StatusIndeterminate {
		t.Errorf("status = %q, want %q", got.Status, StatusIndeterminate)
	}
	if len(got.Child.ServersQueried) != 0 {
		t.Errorf("expected no child servers queried, got %v", got.Child.ServersQueried)
	}
}

func TestZeroTTLIsSerialized(t *testing.T) {
	// NSEC3PARAM commonly uses TTL 0 (RFC 5155); a zero TTL must still appear
	// in the JSON so the UI can show "TTL: 0" rather than dropping it.
	cases := []any{
		SignedRRset{Type: "NSEC3PARAM", TTL: 0, RRSIG: []RRSIG{}},
		DS{KeyTag: 1, TTL: 0},
		DNSKEY{KeyTag: 1, TTL: 0},
	}
	for _, c := range cases {
		b, err := json.Marshal(c)
		if err != nil {
			t.Fatalf("marshal %T: %v", c, err)
		}
		if !strings.Contains(string(b), `"ttl":0`) {
			t.Errorf("%T: expected ttl:0 in JSON, got %s", c, b)
		}
	}
}

// dummyRRSIG builds a well-formed DNSKEY RRSIG for keytag. The signature bytes
// are deliberately not real: verification of a large-exponent key fails on the
// key before the math runs, so any parseable RRSIG exercises the path, and for
// a normal-exponent key it fails as bogus, which is exactly what the control
// asserts.
func dummyRRSIG(keytag uint16) *dns.RRSIG {
	sig := &dns.RRSIG{Hdr: dns.Header{Name: dnsutil.Fqdn(testZone), Class: dns.ClassINET, TTL: 3600}}
	sig.TypeCovered = dns.TypeDNSKEY
	sig.Algorithm = dns.RSASHA256
	sig.Labels = 2
	sig.OrigTTL = 3600
	sig.Inception = uint32(fixedAt.Add(-24 * time.Hour).Unix())
	sig.Expiration = uint32(fixedAt.Add(24 * time.Hour).Unix())
	sig.KeyTag = keytag
	sig.SignerName = dnsutil.Fqdn(testZone)
	sig.Signature = "ZHVtbXlzaWc="
	return sig
}

func TestSigStateRSAExponentUnsupported(t *testing.T) {
	lv := dnstest.RSADNSKEY(testZone, 257, dnstest.LVKSK42018)
	lb := dnstest.RSADNSKEY(testZone, 257, dnstest.LBKSK3842)
	e65 := dnstest.RSADNSKEY(testZone, 257, dnstest.LVKSK42018E65)

	// Only an exponent past the local RSA path leaves a signature unproven.
	if got := sigState(dummyRRSIG(e65.KeyTag()), []dns.RR{e65}, []*dns.DNSKEY{e65}, fixedAt); got != SigUnsupportedKey {
		t.Errorf("65-bit exponent sigState = %q, want %q", got, SigUnsupportedKey)
	}
	// The .lv KSK is verified locally, so a mismatch is bogus, as for the .lb KSK.
	for name, key := range map[string]*dns.DNSKEY{"lv": lv, "lb": lb} {
		if got := sigState(dummyRRSIG(key.KeyTag()), []dns.RR{key}, []*dns.DNSKEY{key}, fixedAt); got != SigBogus {
			t.Errorf("%s KSK sigState = %q, want %q", name, got, SigBogus)
		}
	}
	// A real signature under the .lv exponent is valid.
	kp := dnstest.GenRSAKeyWithExponent(t, testZone, dnstest.LVExponent, 1024, true)
	sig := dnstest.SignRRset(t, kp, []dns.RR{kp.Key}, testZone, testZone, fixedAt.Add(-24*time.Hour), fixedAt.Add(24*time.Hour))
	if got := sigState(sig, []dns.RR{kp.Key}, []*dns.DNSKEY{kp.Key}, fixedAt); got != SigValid {
		t.Errorf("large-exponent sigState = %q, want %q", got, SigValid)
	}
}

func TestExtractRSAExponentPartial(t *testing.T) {
	ctx, _, _ := testhelpers.Context(t)

	// A 65-bit exponent no local path verifies: DS anchored, DNSKEY signature unchecked -> partial.
	ksk := dnstest.RSADNSKEY(testZone, 257, dnstest.LVKSK42018E65)
	keytag := ksk.KeyTag()
	dnskeySig := dummyRRSIG(keytag)

	ds := ksk.ToDS(dns.SHA256)
	if ds == nil {
		t.Fatal("KSK ToDS returned nil")
	}
	parentKSK := genKey(t, testParent, true)
	dsSig := dnstest.SignRRset(t, parentKSK, []dns.RR{ds}, testZone, testParent, fixedAt.Add(-24*time.Hour), fixedAt.Add(24*time.Hour))

	childHook := func(_, qtype string, _ *nameserver.QueryOptions) packet.Packet {
		if qtype == "DNSKEY" {
			return dnssecAnswer(testZone, dns.TypeDNSKEY, ksk, dnskeySig)
		}
		return packet.Packet{}
	}
	parentHook := func(_, qtype string, _ *nameserver.QueryOptions) packet.Packet {
		if qtype == "DS" {
			return dnssecAnswer(testZone, dns.TypeDS, ds, dsSig)
		}
		return packet.Packet{}
	}

	childNS := hookedNS(t, ctx, "ns1."+testZone, "203.0.113.1", childHook)
	parentNS := hookedNS(t, ctx, "ns1."+testParent, "192.0.2.1", parentHook)
	warm(t, ctx, childNS, testZone, "DNSKEY")
	warm(t, ctx, parentNS, testZone, "DS")

	in := Input{
		Zone:       dnsname.New(testZone),
		ParentZone: dnsname.New(testParent),
		ChildNS:    []nameserver.Nameserver{childNS},
		ParentNS:   []nameserver.Nameserver{parentNS},
		At:         fixedAt,
	}

	got := Extract(ctx, in)
	if got == nil {
		t.Fatal("expected a summary")
	}
	if got.Status != StatusPartial {
		t.Errorf("status = %q, want %q", got.Status, StatusPartial)
	}
	sig, ok := findDNSKEYSig(got, keytag)
	if !ok {
		t.Fatalf("no DNSKEY RRSIG for keytag %d", keytag)
	}
	if sig.State != SigUnsupportedKey {
		t.Errorf("DNSKEY RRSIG state = %q, want %q", sig.State, SigUnsupportedKey)
	}
	link, ok := findLink(got, keytag)
	if !ok || link.Status != LinkMatch {
		t.Errorf("expected matching DS link for keytag %d, got %+v", keytag, link)
	}
}
