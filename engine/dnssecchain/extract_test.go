package dnssecchain

import (
	"context"
	"crypto"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	dns "codeberg.org/miekg/dns"
	"codeberg.org/miekg/dns/dnsutil"

	"codeberg.org/pawal/gonemaster/engine/dnsname"
	"codeberg.org/pawal/gonemaster/engine/internal/testhelpers"
	"codeberg.org/pawal/gonemaster/engine/nameserver"
	"codeberg.org/pawal/gonemaster/engine/packet"
)

const (
	testZone   = "example.test"
	testParent = "test"
)

var fixedAt = time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)

// keypair is a generated DNSKEY with its signer.
type keypair struct {
	key  *dns.DNSKEY
	priv crypto.Signer
}

func genKey(t *testing.T, owner string, sep bool) keypair {
	t.Helper()
	k := &dns.DNSKEY{Hdr: dns.Header{Name: dnsutil.Fqdn(owner), Class: dns.ClassINET, TTL: 3600}}
	k.Flags = dns.FlagZONE
	if sep {
		k.Flags |= dns.FlagSEP
	}
	k.Protocol = 3
	k.Algorithm = dns.ECDSAP256SHA256
	priv, err := k.Generate(256)
	if err != nil {
		t.Fatalf("generate key for %s: %v", owner, err)
	}
	signer, ok := priv.(crypto.Signer)
	if !ok {
		t.Fatalf("key for %s is not a crypto.Signer", owner)
	}
	return keypair{key: k, priv: signer}
}

// signRRset produces an RRSIG over rrset signed by kp, valid across window.
func signRRset(t *testing.T, kp keypair, rrset []dns.RR, owner string, signer string, inception, expiration time.Time) *dns.RRSIG {
	t.Helper()
	sig := &dns.RRSIG{Hdr: dns.Header{Name: dnsutil.Fqdn(owner), Class: dns.ClassINET, TTL: 3600}}
	sig.Algorithm = kp.key.Algorithm
	sig.Inception = uint32(inception.Unix())
	sig.Expiration = uint32(expiration.Unix())
	sig.KeyTag = kp.key.KeyTag()
	sig.SignerName = dnsutil.Fqdn(signer)
	if err := sig.Sign(kp.priv, rrset, &dns.SignOption{}); err != nil {
		t.Fatalf("sign rrset for %s: %v", owner, err)
	}
	return sig
}

func dnssecAnswer(owner string, qtype uint16, answers ...dns.RR) packet.Packet {
	msg := new(dns.Msg)
	dnsutil.SetQuestion(msg, dnsutil.Fqdn(owner), qtype)
	msg.Response = true
	msg.Authoritative = true
	msg.Rcode = dns.RcodeSuccess
	msg.Answer = append(msg.Answer, answers...)
	msg.UDPSize = 1232
	msg.Security = true
	return packet.Packet{Msg: msg}
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
	badDigest     bool // corrupt the DS digest -> digest_mismatch
	expiredKeySig bool // child DNSKEY RRSIG expired
	noDS          bool // parent serves no DS (island)
	noDNSKEY      bool // child serves no DNSKEY
}

func buildInput(t *testing.T, ctx context.Context, opts fixtureOpts) Input {
	t.Helper()

	childKSK := genKey(t, testZone, true)
	childZSK := genKey(t, testZone, false)
	parentKSK := genKey(t, testParent, true)

	dnskeyRRset := []dns.RR{childKSK.key, childZSK.key}
	keyInception := fixedAt.Add(-24 * time.Hour)
	keyExpiration := fixedAt.Add(24 * time.Hour)
	if opts.expiredKeySig {
		keyInception = fixedAt.Add(-48 * time.Hour)
		keyExpiration = fixedAt.Add(-24 * time.Hour)
	}
	dnskeySig := signRRset(t, childKSK, dnskeyRRset, testZone, testZone, keyInception, keyExpiration)

	ds := childKSK.key.ToDS(dns.SHA256)
	if ds == nil {
		t.Fatalf("child KSK ToDS returned nil")
	}
	if opts.badDigest {
		ds.Digest = strings.Repeat("00", len(ds.Digest)/2)
	}
	dsRRset := []dns.RR{ds}
	dsSig := signRRset(t, parentKSK, dsRRset, testZone, testParent, fixedAt.Add(-24*time.Hour), fixedAt.Add(24*time.Hour))

	parentKeyRRset := []dns.RR{parentKSK.key}
	parentKeySig := signRRset(t, parentKSK, parentKeyRRset, testParent, testParent, fixedAt.Add(-24*time.Hour), fixedAt.Add(24*time.Hour))

	childHook := func(qname, qtype string, _ *nameserver.QueryOptions) packet.Packet {
		switch qtype {
		case "DNSKEY":
			if opts.noDNSKEY {
				return dnssecAnswer(testZone, dns.TypeDNSKEY)
			}
			return dnssecAnswer(testZone, dns.TypeDNSKEY, childKSK.key, childZSK.key, dnskeySig)
		}
		return packet.Packet{}
	}
	parentHook := func(qname, qtype string, _ *nameserver.QueryOptions) packet.Packet {
		switch qtype {
		case "DS":
			if opts.noDS {
				return dnssecAnswer(testZone, dns.TypeDS)
			}
			return dnssecAnswer(testZone, dns.TypeDS, ds, dsSig)
		case "DNSKEY":
			if qname == dnsutil.Fqdn(testParent) || qname == testParent {
				return dnssecAnswer(testParent, dns.TypeDNSKEY, parentKSK.key, parentKeySig)
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
	if len(got.Child.DNSKEYs) != 2 {
		t.Fatalf("want 2 DNSKEYs, got %d", len(got.Child.DNSKEYs))
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
	if len(got.Parent.DSRRSIG) != 1 || got.Parent.DSRRSIG[0].State != SigValid {
		t.Errorf("expected 1 valid DS RRSIG, got %+v", got.Parent.DSRRSIG)
	}
}

func TestExtractSignedRRsets(t *testing.T) {
	ctx, _, _ := testhelpers.Context(t)

	childKSK := genKey(t, testZone, true)
	childZSK := genKey(t, testZone, false)
	dnskeyRRset := []dns.RR{childKSK.key, childZSK.key}
	win := func(rr []dns.RR, signer keypair) *dns.RRSIG {
		return signRRset(t, signer, rr, testZone, testZone, fixedAt.Add(-24*time.Hour), fixedAt.Add(24*time.Hour))
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

	ds := childKSK.key.ToDS(dns.SHA256)
	cds := &dns.CDS{DS: *ds}
	cdsSig := win([]dns.RR{cds}, childKSK)

	childHook := func(_, qtype string, _ *nameserver.QueryOptions) packet.Packet {
		switch qtype {
		case "DNSKEY":
			return dnssecAnswer(testZone, dns.TypeDNSKEY, childKSK.key, childZSK.key, dnskeySig)
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
	for _, s := range got.Child.Signed {
		if len(s.RRSIG) != 1 || s.RRSIG[0].State != SigValid {
			t.Errorf("%s: expected 1 valid RRSIG, got %+v", s.Type, s.RRSIG)
		}
	}
	// The CDS record names the KSK by tag; SOA carries no ref.
	cdsEntry := got.Child.Signed[1]
	if len(cdsEntry.Refs) != 1 || cdsEntry.Refs[0] != childKSK.key.KeyTag() {
		t.Errorf("CDS refs = %v, want [%d]", cdsEntry.Refs, childKSK.key.KeyTag())
	}
	if len(got.Child.Signed[0].Refs) != 0 {
		t.Errorf("SOA should carry no refs, got %v", got.Child.Signed[0].Refs)
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
	dnskeyRRset := []dns.RR{childKSK.key, childZSK.key}
	dnskeySig := signRRset(t, childKSK, dnskeyRRset, testZone, testZone, fixedAt.Add(-24*time.Hour), fixedAt.Add(24*time.Hour))
	ds := childKSK.key.ToDS(dns.SHA256)

	childHook := func(qname, qtype string, _ *nameserver.QueryOptions) packet.Packet {
		if qtype == "DNSKEY" {
			return dnssecAnswer(testZone, dns.TypeDNSKEY, childKSK.key, childZSK.key, dnskeySig)
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
	dnskeyRRset := []dns.RR{childKSK.key, childZSK.key}
	dnskeySig := signRRset(t, childKSK, dnskeyRRset, testZone, testZone, fixedAt.Add(-24*time.Hour), fixedAt.Add(24*time.Hour))
	ds := childKSK.key.ToDS(dns.SHA256)
	dsSig := signRRset(t, childKSK, []dns.RR{ds}, testZone, testParent, fixedAt.Add(-24*time.Hour), fixedAt.Add(24*time.Hour))

	childHook := func(_, qtype string, _ *nameserver.QueryOptions) packet.Packet {
		if qtype == "DNSKEY" {
			return dnssecAnswer(testZone, dns.TypeDNSKEY, childKSK.key, childZSK.key, dnskeySig)
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
	dnskeyRRset := []dns.RR{childKSK.key, childZSK.key}
	dnskeySig := signRRset(t, childKSK, dnskeyRRset, testZone, testZone, fixedAt.Add(-24*time.Hour), fixedAt.Add(24*time.Hour))
	ds := childKSK.key.ToDS(dns.SHA256)
	dsSig := signRRset(t, childKSK, []dns.RR{ds}, testZone, testParent, fixedAt.Add(-24*time.Hour), fixedAt.Add(24*time.Hour))

	countingChild := func(_, qtype string, _ *nameserver.QueryOptions) packet.Packet {
		atomic.AddInt64(&calls, 1)
		if qtype == "DNSKEY" {
			return dnssecAnswer(testZone, dns.TypeDNSKEY, childKSK.key, childZSK.key, dnskeySig)
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
	dnskeyRRset := []dns.RR{childKSK.key, childZSK.key}
	dnskeySig := signRRset(t, childKSK, dnskeyRRset, testZone, testZone, fixedAt.Add(-24*time.Hour), fixedAt.Add(24*time.Hour))
	ds := childKSK.key.ToDS(dns.SHA256)
	dsSig := signRRset(t, childKSK, []dns.RR{ds}, testZone, testParent, fixedAt.Add(-24*time.Hour), fixedAt.Add(24*time.Hour))

	childNS := hookedNS(t, ctx, "ns1."+testZone, "203.0.113.1", func(_, qtype string, _ *nameserver.QueryOptions) packet.Packet {
		if qtype == "DNSKEY" {
			return dnssecAnswer(testZone, dns.TypeDNSKEY, childKSK.key, childZSK.key, dnskeySig)
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
	real := k2.key.ToDS(dns.SHA256)
	if real == nil {
		t.Fatal("ToDS returned nil")
	}
	ds := DS{KeyTag: real.KeyTag, Algorithm: real.Algorithm, DigestType: real.DigestType, Digest: strings.ToLower(real.Digest)}
	if got := dsLinkStatus(ds, []*dns.DNSKEY{k1.key, k2.key}); got != LinkMatch {
		t.Errorf("status = %q, want %q", got, LinkMatch)
	}
	if got := dsLinkStatus(ds, []*dns.DNSKEY{k1.key}); got != LinkDigestMismatch {
		t.Errorf("status = %q, want %q", got, LinkDigestMismatch)
	}
}

func TestDSLinkStatusGOSTDigestUnsupported(t *testing.T) {
	// Digest type 3 (GOST) is a known IANA type but the DNS library cannot
	// compute it (ToDS returns nil). That must classify as unsupported_digest,
	// never as digest_mismatch: we cannot prove a mismatch we cannot compute.
	k := genKey(t, testZone, true)
	ds := DS{KeyTag: k.key.KeyTag(), Algorithm: k.key.Algorithm, DigestType: 3, Digest: "abcd"}
	if got := dsLinkStatus(ds, []*dns.DNSKEY{k.key}); got != LinkUnsupportedDigest {
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
	ds := childKSK.key.ToDS(dns.SHA256)
	if ds == nil {
		t.Fatal("ToDS returned nil")
	}
	dsSig := signRRset(t, childKSK, []dns.RR{ds}, testZone, testParent, fixedAt.Add(-24*time.Hour), fixedAt.Add(24*time.Hour))

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
