package dnssecutil_test

import (
	"errors"
	"net/netip"
	"slices"
	"testing"
	"time"

	dns "codeberg.org/miekg/dns"
	"codeberg.org/miekg/dns/dnsutil"

	"codeberg.org/pawal/gonemaster/engine/dnssecutil"
	"codeberg.org/pawal/gonemaster/engine/internal/dnstest"
)

func signedA(t *testing.T, owner string) (dnstest.Keypair, []dns.RR, *dns.RRSIG, time.Time) {
	t.Helper()
	kp := dnstest.GenKey(t, "example.test", dns.ED25519, false)
	a := &dns.A{Hdr: dns.Header{Name: dnsutil.Fqdn(owner), Class: dns.ClassINET, TTL: 300}}
	a.Addr = netip.MustParseAddr("192.0.2.1")
	now := time.Now().UTC()
	sig := dnstest.SignRRset(t, kp, []dns.RR{a}, owner, "example.test", now.Add(-time.Hour), now.Add(time.Hour))
	return kp, []dns.RR{a}, sig, now
}

func TestCandidateKeysCapsKeysPerTag(t *testing.T) {
	kp, _, sig, _ := signedA(t, "www.example.test")
	other := dnstest.GenKey(t, "example.test", dns.ED25519, false).Key
	keys := append([]*dns.DNSKEY{other}, dnstest.SameTagKeys(t, kp.Key, 6)...)
	keys = append(keys, kp.Key)

	got, matched := dnssecutil.CandidateKeys(sig, keys)
	if matched != 7 {
		t.Errorf("matched = %d, want 7", matched)
	}
	if !slices.Equal(got, keys[1:5]) {
		t.Errorf("candidates are not the first 4 same-tag keys in order")
	}
}

func TestCandidateKeysSkipsOtherAlgorithms(t *testing.T) {
	kp, _, sig, _ := signedA(t, "www.example.test")
	foreign := dnstest.SameTagKeyWithAlgorithm(t, kp.Key, dns.ECDSAP256SHA256)
	keys := append([]*dns.DNSKEY{foreign}, dnstest.SameTagKeys(t, foreign, 4)...)
	keys = append(keys, kp.Key)

	got, matched := dnssecutil.CandidateKeys(sig, keys)
	if matched != 1 {
		t.Errorf("matched = %d, want 1", matched)
	}
	if !slices.Equal(got, []*dns.DNSKEY{kp.Key}) {
		t.Errorf("candidates = %v, want only the signing key", got)
	}
}

func TestVerifyCandidatesReusesOneCopy(t *testing.T) {
	kp, rrset, sig, now := signedA(t, "www.example.test")
	keys := append(dnstest.SameTagKeys(t, kp.Key, 3), kp.Key)
	owner := rrset[0].Header().Name

	key, errs := dnssecutil.VerifyCandidates(sig, rrset, keys, now)
	if key != kp.Key {
		t.Fatalf("verifying key = %v, want the signing key; errors %v", key, errs)
	}
	if len(errs) != 3 {
		t.Errorf("errors = %d, want 3 for the decoys", len(errs))
	}
	if got := rrset[0].Header().Name; got != owner {
		t.Errorf("caller's owner name changed to %q", got)
	}
}

func TestVerifyCandidatesWildcardAfterFailedKeys(t *testing.T) {
	kp, rrset, sig, now := signedA(t, "*.example.test")
	rrset[0].Header().Name = dnsutil.Fqdn("a.b.example.test")
	keys := append(dnstest.SameTagKeys(t, kp.Key, 2), kp.Key)

	if key, errs := dnssecutil.VerifyCandidates(sig, rrset, keys, now); key != kp.Key {
		t.Fatalf("wildcard expansion did not verify after decoys: %v", errs)
	}
}

func TestVerifyCandidatesWildcardAboveSigner(t *testing.T) {
	kp := dnstest.GenKey(t, "c.example.test", dns.ED25519, false)
	a := &dns.A{Hdr: dns.Header{Name: "*.example.test.", Class: dns.ClassINET, TTL: 300}}
	a.Addr = netip.MustParseAddr("192.0.2.1")
	now := time.Now().UTC()
	sig := dnstest.SignRRset(t, kp, []dns.RR{a}, "*.example.test", "c.example.test", now.Add(-time.Hour), now.Add(time.Hour))
	a.Hdr.Name = "a.c.example.test."

	key, errs := dnssecutil.VerifyCandidates(sig, []dns.RR{a}, []*dns.DNSKEY{kp.Key}, now)
	if key != nil || len(errs) != 1 || !errors.Is(errs[0], dnssecutil.ErrLabels) {
		t.Fatalf("key = %v, errors = %v; want nil and one ErrLabels", key, errs)
	}
}

func TestVerifyCandidatesOutsideWindowTriesNoKey(t *testing.T) {
	kp, rrset, sig, _ := signedA(t, "www.example.test")
	keys := append(dnstest.SameTagKeys(t, kp.Key, 1), kp.Key)
	late := time.Unix(int64(sig.Expiration)+60, 0)

	key, errs := dnssecutil.VerifyCandidates(sig, rrset, keys, late)
	if key != nil || len(errs) != 2 {
		t.Fatalf("key = %v, errors = %d; want nil and 2", key, len(errs))
	}
	if errs[0].Error() != "rrsig not valid at time" {
		t.Errorf("error = %q, want the validity window error", errs[0])
	}
}

func TestVerifyCandidatesWrongAlgorithmKey(t *testing.T) {
	kp, rrset, sig, now := signedA(t, "www.example.test")
	wrong := kp.Key.Clone().(*dns.DNSKEY)
	wrong.Algorithm = dns.ECDSAP256SHA256

	_, errs := dnssecutil.VerifyCandidates(sig, rrset, []*dns.DNSKEY{wrong}, now)
	if len(errs) != 1 || !errors.Is(errs[0], dns.ErrKey) {
		t.Fatalf("errors = %v, want one ErrKey", errs)
	}
}
