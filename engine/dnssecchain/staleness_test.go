package dnssecchain

import (
	"context"
	"encoding/json"
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

// Signature windows around fixedAt: one current, one already expired. A stale
// secondary serves records signed inside the expired window.
var (
	freshFrom = fixedAt.Add(-24 * time.Hour)
	freshTo   = fixedAt.Add(24 * time.Hour)
	staleFrom = fixedAt.Add(-72 * time.Hour)
	staleTo   = fixedAt.Add(-24 * time.Hour)
)

// signedZone is the material every staleness fixture shares: one signed child
// zone plus the parent DS that anchors its KSK.
type signedZone struct {
	ksk      dnstest.Keypair
	zsk      dnstest.Keypair
	keyRRset []dns.RR
	ds       *dns.DS
	dsSig    *dns.RRSIG
	soa      *dns.SOA
}

func newSignedZone(t *testing.T) signedZone {
	t.Helper()
	z := signedZone{ksk: genKey(t, testZone, true), zsk: genKey(t, testZone, false)}
	z.keyRRset = []dns.RR{z.ksk.Key, z.zsk.Key}
	z.ds = z.ksk.Key.ToDS(dns.SHA256)
	if z.ds == nil {
		t.Fatal("child KSK ToDS returned nil")
	}
	z.dsSig = dnstest.SignRRset(t, z.ksk, []dns.RR{z.ds}, testZone, testParent, freshFrom, freshTo)
	z.soa = &dns.SOA{Hdr: dns.Header{Name: dnsutil.Fqdn(testZone), Class: dns.ClassINET, TTL: 3600}}
	z.soa.Ns = dnsutil.Fqdn("ns1." + testZone)
	z.soa.Mbox = dnsutil.Fqdn("hostmaster." + testZone)
	z.soa.Serial = 1
	z.soa.Refresh = 3600
	z.soa.Retry = 600
	z.soa.Expire = 604800
	z.soa.Minttl = 3600
	return z
}

func (z signedZone) keySig(t *testing.T, from, to time.Time) *dns.RRSIG {
	t.Helper()
	return dnstest.SignRRset(t, z.ksk, z.keyRRset, testZone, testZone, from, to)
}

func (z signedZone) soaSig(t *testing.T, from, to time.Time) *dns.RRSIG {
	t.Helper()
	return dnstest.SignRRset(t, z.zsk, []dns.RR{z.soa}, testZone, testZone, from, to)
}

// dnssecAuthority builds an authoritative NODATA answer carrying the denial
// proof in the authority section, the shape an NSEC3 zone returns for an apex
// NSEC query.
func dnssecAuthority(owner string, qtype uint16, authority ...dns.RR) packet.Packet {
	return dnstest.Response(dnstest.Question(owner, qtype), dnstest.Reply(),
		dnstest.Authority(authority...), dnstest.Secure())
}

// parentWithDS returns a parent server serving the zone's DS RRset, with a DS
// signature valid across the given window.
func parentWithDS(t *testing.T, ctx context.Context, z signedZone, ip string, sig *dns.RRSIG) nameserver.Nameserver {
	t.Helper()
	ns := hookedNS(t, ctx, "p."+testParent, ip, func(_, qtype string, _ *nameserver.QueryOptions) packet.Packet {
		if qtype == "DS" {
			return dnssecAnswer(testZone, dns.TypeDS, z.ds, sig)
		}
		return packet.Packet{}
	})
	warm(t, ctx, ns, testZone, "DS")
	return ns
}

// childServers builds one child nameserver per address, each answering from
// answers (keyed by query type), and warms every type it serves.
func childServers(t *testing.T, ctx context.Context, ips []string, answers func(ip string) map[string]packet.Packet) []nameserver.Nameserver {
	t.Helper()
	var out []nameserver.Nameserver
	for _, ip := range ips {
		byType := answers(ip)
		ns := hookedNS(t, ctx, "ns."+testZone, ip, func(_, qtype string, _ *nameserver.QueryOptions) packet.Packet {
			return byType[qtype]
		})
		for qtype := range byType {
			warm(t, ctx, ns, testZone, qtype)
		}
		out = append(out, ns)
	}
	return out
}

func chainInput(child, parent []nameserver.Nameserver) Input {
	return Input{
		Zone:       dnsname.New(testZone),
		ParentZone: dnsname.New(testParent),
		ChildNS:    child,
		ParentNS:   parent,
		At:         fixedAt,
	}
}

func equalStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

// A lagging secondary serves the same keys with an expired signature: the
// validated path still holds through the fresh servers, so the roll-up is
// partial, not secure and not broken.
func TestExtractStaleSecondaryDNSKEYSignature(t *testing.T) {
	ctx, _, _ := testhelpers.Context(t)
	z := newSignedZone(t)
	fresh := z.keySig(t, freshFrom, freshTo)
	expired := z.keySig(t, staleFrom, staleTo)

	staleIPs := []string{"203.0.113.10", "203.0.113.11"}
	child := childServers(t, ctx, []string{"203.0.113.1", "203.0.113.2", "203.0.113.3", staleIPs[0], staleIPs[1]}, func(ip string) map[string]packet.Packet {
		sig := fresh
		if ip == staleIPs[0] || ip == staleIPs[1] {
			sig = expired
		}
		return map[string]packet.Packet{
			"DNSKEY": dnssecAnswer(testZone, dns.TypeDNSKEY, z.ksk.Key, z.zsk.Key, sig),
		}
	})
	parent := []nameserver.Nameserver{parentWithDS(t, ctx, z, "192.0.2.1", z.dsSig)}

	got := Extract(ctx, chainInput(child, parent))
	if got == nil {
		t.Fatal("expected a summary")
	}
	if got.Status != StatusPartial {
		t.Errorf("status = %q, want %q", got.Status, StatusPartial)
	}
	if !equalStrings(got.Child.ServersStale, staleIPs) {
		t.Errorf("servers_stale = %v, want %v", got.Child.ServersStale, staleIPs)
	}
	// The key set is identical everywhere, so only the signature profile can
	// mark the two lagging servers as disagreeing.
	if !equalStrings(got.Child.ServersDisagreeing, staleIPs) {
		t.Errorf("servers_disagreeing = %v, want %v", got.Child.ServersDisagreeing, staleIPs)
	}
	if len(got.Parent.ServersStale) != 0 {
		t.Errorf("parent servers_stale = %v, want none", got.Parent.ServersStale)
	}
	states := map[string]int{}
	for _, sig := range got.Child.DNSKEYRRSIG {
		states[sig.State]++
	}
	if states[SigValid] != 1 || states[SigExpired] != 1 {
		t.Errorf("dnskey_rrsig states = %v, want one valid and one expired", states)
	}
}

// The same shape one level down: identical DNSKEY signatures everywhere, but a
// subset serves zone data (SOA) signed inside an expired window.
func TestExtractStaleSecondarySOASignature(t *testing.T) {
	ctx, _, _ := testhelpers.Context(t)
	z := newSignedZone(t)
	keySig := z.keySig(t, freshFrom, freshTo)
	freshSOA := z.soaSig(t, freshFrom, freshTo)
	expiredSOA := z.soaSig(t, staleFrom, staleTo)

	staleIP := "203.0.113.10"
	child := childServers(t, ctx, []string{"203.0.113.1", "203.0.113.2", staleIP}, func(ip string) map[string]packet.Packet {
		soaSig := freshSOA
		if ip == staleIP {
			soaSig = expiredSOA
		}
		return map[string]packet.Packet{
			"DNSKEY": dnssecAnswer(testZone, dns.TypeDNSKEY, z.ksk.Key, z.zsk.Key, keySig),
			"SOA":    dnssecAnswer(testZone, dns.TypeSOA, z.soa, soaSig),
		}
	})
	parent := []nameserver.Nameserver{parentWithDS(t, ctx, z, "192.0.2.1", z.dsSig)}

	got := Extract(ctx, chainInput(child, parent))
	if got == nil {
		t.Fatal("expected a summary")
	}
	if got.Status != StatusPartial {
		t.Errorf("status = %q, want %q", got.Status, StatusPartial)
	}
	if !equalStrings(got.Child.ServersStale, []string{staleIP}) {
		t.Errorf("servers_stale = %v, want [%s]", got.Child.ServersStale, staleIP)
	}
	if len(got.Child.Signed) != 1 || got.Child.Signed[0].Type != "SOA" {
		t.Fatalf("expected one signed SOA RRset, got %+v", got.Child.Signed)
	}
	if len(got.Child.Signed[0].RRSIG) != 2 {
		t.Fatalf("expected both SOA signatures, got %+v", got.Child.Signed[0].RRSIG)
	}
}

// With no server serving a signature inside its window there is no validatable
// path left: that is broken, not partial.
func TestExtractAllServersStaleIsBroken(t *testing.T) {
	ctx, _, _ := testhelpers.Context(t)
	z := newSignedZone(t)
	expired := z.keySig(t, staleFrom, staleTo)

	child := childServers(t, ctx, []string{"203.0.113.1", "203.0.113.2"}, func(string) map[string]packet.Packet {
		return map[string]packet.Packet{
			"DNSKEY": dnssecAnswer(testZone, dns.TypeDNSKEY, z.ksk.Key, z.zsk.Key, expired),
		}
	})
	parent := []nameserver.Nameserver{parentWithDS(t, ctx, z, "192.0.2.1", z.dsSig)}

	got := Extract(ctx, chainInput(child, parent))
	if got == nil {
		t.Fatal("expected a summary")
	}
	if got.Status != StatusBroken {
		t.Errorf("status = %q, want %q", got.Status, StatusBroken)
	}
	if len(got.Child.ServersStale) != 2 {
		t.Errorf("servers_stale = %v, want both servers", got.Child.ServersStale)
	}
}

// An expired DS signature on every parent server breaks the delegation even
// though the child side is impeccable.
func TestExtractExpiredDSSignatureEverywhereIsBroken(t *testing.T) {
	ctx, _, _ := testhelpers.Context(t)
	z := newSignedZone(t)
	keySig := z.keySig(t, freshFrom, freshTo)
	expiredDSSig := dnstest.SignRRset(t, z.ksk, []dns.RR{z.ds}, testZone, testParent, staleFrom, staleTo)

	child := childServers(t, ctx, []string{"203.0.113.1"}, func(string) map[string]packet.Packet {
		return map[string]packet.Packet{
			"DNSKEY": dnssecAnswer(testZone, dns.TypeDNSKEY, z.ksk.Key, z.zsk.Key, keySig),
		}
	})
	parent := []nameserver.Nameserver{parentWithDS(t, ctx, z, "192.0.2.1", expiredDSSig)}

	got := Extract(ctx, chainInput(child, parent))
	if got == nil {
		t.Fatal("expected a summary")
	}
	if got.Status != StatusBroken {
		t.Errorf("status = %q, want %q", got.Status, StatusBroken)
	}
	if !equalStrings(got.Parent.ServersStale, []string{"192.0.2.1"}) {
		t.Errorf("parent servers_stale = %v, want [192.0.2.1]", got.Parent.ServersStale)
	}
}

// A clean zone must stay secure with no stale servers named.
func TestExtractSecureHasNoStaleServers(t *testing.T) {
	ctx, _, _ := testhelpers.Context(t)
	got := Extract(ctx, buildInput(t, ctx, fixtureOpts{}))
	if got == nil {
		t.Fatal("expected a summary")
	}
	if got.Status != StatusSecure {
		t.Errorf("status = %q, want %q", got.Status, StatusSecure)
	}
	if len(got.Child.ServersStale) != 0 || len(got.Parent.ServersStale) != 0 {
		t.Errorf("stale servers = %v / %v, want none", got.Parent.ServersStale, got.Child.ServersStale)
	}
}

// nsecRR builds the apex NSEC record an NSEC-signed zone answers with.
func nsecRR() *dns.NSEC {
	rr := &dns.NSEC{Hdr: dns.Header{Name: dnsutil.Fqdn(testZone), Class: dns.ClassINET, TTL: 3600}}
	rr.NextDomain = dnsutil.Fqdn("a." + testZone)
	rr.TypeBitMap = []uint16{dns.TypeNS, dns.TypeSOA, dns.TypeRRSIG, dns.TypeNSEC, dns.TypeDNSKEY}
	return rr
}

// nsec3RR builds the hashed-apex NSEC3 record an NSEC3 zone puts in the
// authority section of an apex NSEC NODATA answer.
func nsec3RR() *dns.NSEC3 {
	owner := "1avvrsvb0nc2q1sbl4rdlrkc8g5s5b1h." + testZone
	rr := &dns.NSEC3{Hdr: dns.Header{Name: dnsutil.Fqdn(owner), Class: dns.ClassINET, TTL: 3600}}
	rr.Hash = dns.SHA1
	rr.Iterations = 0
	rr.SaltLength = 0
	rr.Salt = "-"
	rr.HashLength = 20
	rr.NextDomain = "2avvrsvb0nc2q1sbl4rdlrkc8g5s5b1h"
	rr.TypeBitMap = []uint16{dns.TypeNS, dns.TypeSOA, dns.TypeRRSIG, dns.TypeDNSKEY, dns.TypeNSEC3PARAM}
	return rr
}

// DNSSEC10 caches the apex NSEC answer; an NSEC zone returns the record itself,
// so an expired denial signature on a subset must surface like any other.
func TestExtractStaleNSECSignature(t *testing.T) {
	ctx, _, _ := testhelpers.Context(t)
	z := newSignedZone(t)
	keySig := z.keySig(t, freshFrom, freshTo)
	nsec := nsecRR()
	freshNSEC := dnstest.SignRRset(t, z.zsk, []dns.RR{nsec}, testZone, testZone, freshFrom, freshTo)
	expiredNSEC := dnstest.SignRRset(t, z.zsk, []dns.RR{nsec}, testZone, testZone, staleFrom, staleTo)

	staleIP := "203.0.113.10"
	child := childServers(t, ctx, []string{"203.0.113.1", "203.0.113.2", staleIP}, func(ip string) map[string]packet.Packet {
		sig := freshNSEC
		if ip == staleIP {
			sig = expiredNSEC
		}
		return map[string]packet.Packet{
			"DNSKEY": dnssecAnswer(testZone, dns.TypeDNSKEY, z.ksk.Key, z.zsk.Key, keySig),
			"NSEC":   dnssecAnswer(testZone, dns.TypeNSEC, nsec, sig),
		}
	})
	parent := []nameserver.Nameserver{parentWithDS(t, ctx, z, "192.0.2.1", z.dsSig)}

	got := Extract(ctx, chainInput(child, parent))
	if got == nil {
		t.Fatal("expected a summary")
	}
	if got.Status != StatusPartial {
		t.Errorf("status = %q, want %q", got.Status, StatusPartial)
	}
	if !equalStrings(got.Child.ServersStale, []string{staleIP}) {
		t.Errorf("servers_stale = %v, want [%s]", got.Child.ServersStale, staleIP)
	}
	if len(got.Child.Signed) != 1 || got.Child.Signed[0].Type != "NSEC" {
		t.Fatalf("expected one signed NSEC RRset, got %+v", got.Child.Signed)
	}
	seen := map[string]bool{}
	for _, sig := range got.Child.Signed[0].RRSIG {
		seen[sig.State] = true
	}
	if !seen[SigValid] || !seen[SigExpired] {
		t.Errorf("NSEC signature states = %v, want a valid and an expired one", seen)
	}
}

// An NSEC3 zone answers the same apex NSEC query with NODATA and proves it in
// the authority section, which is the only place that signature ever appears.
func TestExtractStaleNSEC3SignatureFromAuthority(t *testing.T) {
	ctx, _, _ := testhelpers.Context(t)
	z := newSignedZone(t)
	keySig := z.keySig(t, freshFrom, freshTo)
	nsec3 := nsec3RR()
	owner := nsec3.Hdr.Name
	freshNSEC3 := dnstest.SignRRset(t, z.zsk, []dns.RR{nsec3}, owner, testZone, freshFrom, freshTo)
	expiredNSEC3 := dnstest.SignRRset(t, z.zsk, []dns.RR{nsec3}, owner, testZone, staleFrom, staleTo)

	staleIP := "203.0.113.10"
	child := childServers(t, ctx, []string{"203.0.113.1", staleIP}, func(ip string) map[string]packet.Packet {
		sig := freshNSEC3
		if ip == staleIP {
			sig = expiredNSEC3
		}
		return map[string]packet.Packet{
			"DNSKEY": dnssecAnswer(testZone, dns.TypeDNSKEY, z.ksk.Key, z.zsk.Key, keySig),
			"NSEC":   dnssecAuthority(testZone, dns.TypeNSEC, nsec3, sig),
		}
	})
	parent := []nameserver.Nameserver{parentWithDS(t, ctx, z, "192.0.2.1", z.dsSig)}

	got := Extract(ctx, chainInput(child, parent))
	if got == nil {
		t.Fatal("expected a summary")
	}
	if got.Status != StatusPartial {
		t.Errorf("status = %q, want %q", got.Status, StatusPartial)
	}
	if len(got.Child.Signed) != 1 || got.Child.Signed[0].Type != "NSEC3" {
		t.Fatalf("expected one signed NSEC3 RRset, got %+v", got.Child.Signed)
	}
	if got.Child.Signed[0].TTL != 3600 {
		t.Errorf("NSEC3 TTL = %d, want 3600", got.Child.Signed[0].TTL)
	}
	// The fresh signature verifies against the RRset from the same packet, so
	// the authority-section RRset lookup is exercised, not just the window.
	valid := 0
	for _, sig := range got.Child.Signed[0].RRSIG {
		if sig.State == SigValid {
			valid++
		}
	}
	if valid != 1 {
		t.Errorf("expected exactly one valid NSEC3 signature, got %+v", got.Child.Signed[0].RRSIG)
	}
}

// The denial proof must come from the entry DNSSEC10 already cached: the
// extractor replays that query key and issues nothing of its own.
func TestExtractAuthoritySectionCacheParity(t *testing.T) {
	ctx, _, _ := testhelpers.Context(t)
	z := newSignedZone(t)
	keySig := z.keySig(t, freshFrom, freshTo)
	nsec3 := nsec3RR()
	nsec3Sig := dnstest.SignRRset(t, z.zsk, []dns.RR{nsec3}, nsec3.Hdr.Name, testZone, freshFrom, freshTo)

	var calls int64
	childNS := hookedNS(t, ctx, "ns1."+testZone, "203.0.113.1", func(_, qtype string, _ *nameserver.QueryOptions) packet.Packet {
		atomic.AddInt64(&calls, 1)
		switch qtype {
		case "DNSKEY":
			return dnssecAnswer(testZone, dns.TypeDNSKEY, z.ksk.Key, z.zsk.Key, keySig)
		case "NSEC":
			return dnssecAuthority(testZone, dns.TypeNSEC, nsec3, nsec3Sig)
		}
		return packet.Packet{}
	})

	// Warm exactly as DNSSEC10 does: apex name, NSEC, DO set.
	on := true
	for _, qtype := range []string{"DNSKEY", "NSEC"} {
		if _, err := childNS.QueryWithOptions(ctx, testZone, qtype, &nameserver.QueryOptions{DNSSEC: &on}); err != nil {
			t.Fatalf("warm %s: %v", qtype, err)
		}
	}
	parent := []nameserver.Nameserver{parentWithDS(t, ctx, z, "192.0.2.1", z.dsSig)}
	before := atomic.LoadInt64(&calls)

	got := Extract(ctx, chainInput([]nameserver.Nameserver{childNS}, parent))
	if got == nil {
		t.Fatal("expected a summary")
	}
	if after := atomic.LoadInt64(&calls); after != before {
		t.Errorf("extractor issued %d queries; must be cache-only", after-before)
	}
	if len(got.Child.Signed) != 1 || got.Child.Signed[0].Type != "NSEC3" {
		t.Fatalf("authority-section parity broken: signed = %+v", got.Child.Signed)
	}
}

// The stored document must announce schema 2 and carry the new arrays, empty
// rather than null, so consumers can index them without a nil check.
func TestSummaryJSONVersionAndStaleArrays(t *testing.T) {
	ctx, _, _ := testhelpers.Context(t)
	got := Extract(ctx, buildInput(t, ctx, fixtureOpts{}))
	if got == nil {
		t.Fatal("expected a summary")
	}
	blob, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var doc struct {
		Version int `json:"version"`
		Parent  struct {
			ServersStale []string `json:"servers_stale"`
		} `json:"parent"`
		Child struct {
			ServersStale []string `json:"servers_stale"`
		} `json:"child"`
	}
	if err := json.Unmarshal(blob, &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if doc.Version != 2 {
		t.Errorf("version = %d, want 2", doc.Version)
	}
	if doc.Parent.ServersStale == nil || doc.Child.ServersStale == nil {
		t.Errorf("servers_stale must marshal as an array, got %s", blob)
	}
}
