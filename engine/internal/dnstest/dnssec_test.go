package dnstest

import (
	"testing"
	"time"

	dns "codeberg.org/miekg/dns"

	"codeberg.org/pawal/gonemaster/engine/dnssecutil"
)

func TestGenKeyProducesAVerifiableSignature(t *testing.T) {
	kp := GenKey(t, "example.test", dns.ECDSAP256SHA256, true)
	if kp.Key.Flags&dns.FlagSEP == 0 {
		t.Fatal("expected the SEP flag on a SEP key")
	}

	now := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)
	rrset := []dns.RR{kp.Key}
	sig := SignRRset(t, kp, rrset, "example.test", "example.test", now.Add(-time.Hour), now.Add(time.Hour))

	// Signing through the real crypto path is the point: a signature that does
	// not verify would make every fixture built on GenKey meaningless.
	if err := sig.Verify(kp.Key, rrset, &dns.SignOption{}); err != nil {
		t.Fatalf("expected the generated signature to verify: %v", err)
	}
	if want := dnssecutil.KeyTag(kp.Key); sig.KeyTag != want {
		t.Fatalf("expected the signature to carry the key tag %d, got %d", want, sig.KeyTag)
	}
}

// The library rejects an RRSIG whose key tag is zero before it looks at the
// key, so a generator that hands one back makes every signing test flaky.
func TestGenKeyKeyTagCanSign(t *testing.T) {
	kp := GenKey(t, "example.test", dns.ECDSAP256SHA256, false)
	if dnssecutil.KeyTag(kp.Key) == 0 {
		t.Fatal("generated key tag is zero")
	}

	now := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)
	rrset := []dns.RR{kp.Key}
	sig := &dns.RRSIG{Hdr: dns.Header{Name: kp.Key.Hdr.Name, Class: dns.ClassINET, TTL: 3600}}
	sig.Algorithm = kp.Key.Algorithm
	sig.Inception = uint32(now.Add(-time.Hour).Unix())
	sig.Expiration = uint32(now.Add(time.Hour).Unix())
	sig.SignerName = kp.Key.Hdr.Name

	sig.KeyTag = 0
	if err := sig.Sign(kp.Priv, rrset, &dns.SignOption{}); err == nil {
		t.Error("a zero key tag must not sign")
	}
	sig.KeyTag = dnssecutil.KeyTag(kp.Key)
	if err := sig.Sign(kp.Priv, rrset, &dns.SignOption{}); err != nil {
		t.Errorf("sign with the generated key tag: %v", err)
	}
}

func TestGenKeyWithoutSEP(t *testing.T) {
	kp := GenKey(t, "example.test", dns.ECDSAP256SHA256, false)
	if kp.Key.Flags&dns.FlagSEP != 0 {
		t.Fatal("expected no SEP flag on a zone-signing key")
	}
}

func TestGenKeySupportsMLDSA44(t *testing.T) {
	// Generating rather than replaying a captured key means this also fails if
	// the DNS library loses ML-DSA-44 support.
	kp := GenKey(t, "example.test", dns.MLDSA44, true)
	if kp.Key.Algorithm != dns.MLDSA44 {
		t.Fatalf("expected algorithm %d, got %d", dns.MLDSA44, kp.Key.Algorithm)
	}
}

func TestRSADNSKEYCarriesTheCapturedKeys(t *testing.T) {
	lv := RSADNSKEY("example.", dns.FlagZONE|dns.FlagSEP, LVKSK42018)
	if lv.Algorithm != dns.RSASHA256 || lv.Protocol != 3 {
		t.Fatalf("unexpected RSA DNSKEY shape: %#v", lv)
	}
	if got := dnssecutil.KeyTag(lv); got != 42018 {
		t.Fatalf("expected the .lv KSK keytag 42018, got %d", got)
	}

	lb := RSADNSKEY("example.", dns.FlagZONE|dns.FlagSEP, LBKSK3842)
	if got := dnssecutil.KeyTag(lb); got != 3842 {
		t.Fatalf("expected the .lb KSK keytag 3842, got %d", got)
	}
}
