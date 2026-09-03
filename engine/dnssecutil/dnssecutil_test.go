package dnssecutil_test

import (
	"crypto"
	"net/netip"
	"testing"
	"time"

	dns "codeberg.org/miekg/dns"

	"codeberg.org/miekg/dns/dnsutil"
	"codeberg.org/pawal/gonemaster/engine/dnssecutil"
	"codeberg.org/pawal/gonemaster/engine/internal/dnstest"
)

func TestAlgorithmSupported(t *testing.T) {
	supported := []uint8{dns.RSASHA1, dns.RSASHA1NSEC3SHA1, dns.RSASHA256, dns.RSASHA512, dns.ECDSAP256SHA256, dns.ECDSAP384SHA384, dns.ED25519, dns.ED448, dns.MLDSA44}
	for _, algo := range supported {
		if !dnssecutil.AlgorithmSupported(algo) {
			t.Errorf("algorithm %d should be supported", algo)
		}
	}
	unsupported := []uint8{dns.RSAMD5, dns.DSA, dns.ECCGOST, dns.SM2SM3, 0, 99}
	for _, algo := range unsupported {
		if dnssecutil.AlgorithmSupported(algo) {
			t.Errorf("algorithm %d should not be supported", algo)
		}
	}
}

func TestKeySize(t *testing.T) {
	if got := dnssecutil.KeySize(nil); got != 0 {
		t.Errorf("nil key: got %d, want 0", got)
	}
	if got := dnssecutil.KeySize(&dns.DNSKEY{}); got != 0 {
		t.Errorf("empty key: got %d, want 0", got)
	}

	key := &dns.DNSKEY{Hdr: dns.Header{Name: dnsutil.Fqdn("example.test"), Class: dns.ClassINET, TTL: 3600}}
	key.Flags = dns.FlagZONE | dns.FlagSEP
	key.Protocol = 3
	key.Algorithm = dns.RSASHA256
	if _, err := key.Generate(1024); err != nil {
		t.Fatalf("generate key: %v", err)
	}
	if got := dnssecutil.KeySize(key); got < 1000 || got > 1024 {
		t.Errorf("1024-bit RSA key: got %d bits, want ~1024", got)
	}
}

// Each key carries a real 2048-bit RSA payload, so a KeySize that still parsed
// the modulus would answer 2048 instead of the algorithm's fixed size.
func TestKeySizeFixedSizeAlgorithms(t *testing.T) {
	cases := []struct {
		algo uint8
		want int
	}{
		{dns.ECDSAP256SHA256, 256},
		{dns.ECDSAP384SHA384, 384},
		{dns.ED25519, 256},
		{dns.ED448, 456},
		// Recognised, never verified; the size still comes from the curve.
		{dns.ECCGOST, 256},
		{dns.ECCGOST12, 256},
		{dns.SM2SM3, 256},
	}
	for _, tc := range cases {
		t.Run(dns.AlgorithmToString[tc.algo], func(t *testing.T) {
			key := dnstest.RSADNSKEY("example.", dns.FlagZONE, dnstest.LBKSK3842)
			key.Algorithm = tc.algo
			if got := dnssecutil.KeySize(key); got != tc.want {
				t.Errorf("KeySize = %d, want %d", got, tc.want)
			}
		})
	}
}

// Types 5 (GOST R 34.11-2012) and 6 (SM3) are IANA-assigned, so reading the
// gate as "known type" would admit them. The library computes an experimental
// SHA-512 for 5, which would fail a sound delegation on a digest mismatch.
func TestDigestSupportedExcludesUncomputableTypes(t *testing.T) {
	for _, digest := range []uint8{1, 2, 3, 4} {
		if !dnssecutil.DigestSupported(digest) {
			t.Errorf("digest type %d should be supported", digest)
		}
	}
	for _, digest := range []uint8{0, 5, 6, 7, 255} {
		if dnssecutil.DigestSupported(digest) {
			t.Errorf("digest type %d should not be supported", digest)
		}
	}

	key := dnstest.RSADNSKEY("example.", dns.FlagZONE, dnstest.LBKSK3842)
	ds := key.ToDS(5)
	if ds == nil {
		t.Skip("library no longer computes digest type 5; the exclusion can be revisited")
	}
	if len(ds.Digest) != 128 {
		t.Errorf("digest type 5 produced %d hex chars, want 128 (SHA-512): the collision this guards may be gone",
			len(ds.Digest))
	}
}

// Generated keys confirm the mapped sizes match real key material.
func TestKeySizeGeneratedCurveKeys(t *testing.T) {
	for _, tc := range []struct {
		algo uint8
		want int
	}{
		{dns.ECDSAP256SHA256, 256},
		{dns.ED25519, 256},
	} {
		t.Run(dns.AlgorithmToString[tc.algo], func(t *testing.T) {
			key := dnstest.GenKey(t, "example.test", tc.algo, false).Key
			if got := dnssecutil.KeySize(key); got != tc.want {
				t.Errorf("KeySize = %d, want %d", got, tc.want)
			}
		})
	}
}

// An unknown algorithm has no derivable size, even when the payload happens to
// parse as an RSA modulus.
func TestKeySizeUnknownAlgorithmIsZero(t *testing.T) {
	for _, algo := range []uint8{dns.DSA, dns.MLDSA44, 0, 99} {
		key := dnstest.RSADNSKEY("example.", dns.FlagZONE|dns.FlagSEP, dnstest.LBKSK3842)
		key.Algorithm = algo
		if got := dnssecutil.KeySize(key); got != 0 {
			t.Errorf("algorithm %d: KeySize = %d, want 0", algo, got)
		}
	}
}

// Every RSA algorithm keeps the modulus-derived size, RSAMD5 included.
func TestKeySizeRSAAlgorithms(t *testing.T) {
	for _, algo := range []uint8{dns.RSAMD5, dns.RSASHA1, dns.RSASHA1NSEC3SHA1, dns.RSASHA256, dns.RSASHA512} {
		key := dnstest.RSADNSKEY("example.", dns.FlagZONE|dns.FlagSEP, dnstest.LBKSK3842)
		key.Algorithm = algo
		if got := dnssecutil.KeySize(key); got != 2048 {
			t.Errorf("algorithm %d: KeySize = %d, want 2048", algo, got)
		}
	}
}

func TestVerifyRRSIG(t *testing.T) {
	if err := dnssecutil.VerifyRRSIG(nil, nil, nil, time.Unix(1000, 0)); err == nil {
		t.Error("nil sig/key should error")
	}

	key := &dns.DNSKEY{Hdr: dns.Header{Name: dnsutil.Fqdn("example.test"), Class: dns.ClassINET, TTL: 3600}}
	key.Flags = dns.FlagZONE
	key.Protocol = 3
	key.Algorithm = dns.RSASHA256
	priv, err := key.Generate(1024)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	signer, ok := priv.(crypto.Signer)
	if !ok {
		t.Fatalf("private key does not implement crypto.Signer")
	}

	aRR := &dns.A{Hdr: dns.Header{Name: dnsutil.Fqdn("example.test"), Class: dns.ClassINET, TTL: 3600}}
	aRR.Addr = netip.MustParseAddr("192.0.2.1")
	rrset := []dns.RR{aRR}

	now := time.Now().UTC()
	sig := &dns.RRSIG{Hdr: dns.Header{Name: dnsutil.Fqdn("example.test"), Class: dns.ClassINET, TTL: 3600}}
	sig.Algorithm = key.Algorithm
	sig.Inception = uint32(now.Add(-time.Hour).Unix())
	sig.Expiration = uint32(now.Add(24 * time.Hour).Unix())
	sig.KeyTag = key.KeyTag()
	sig.SignerName = dnsutil.Fqdn("example.test")
	if err := sig.Sign(signer, rrset, &dns.SignOption{}); err != nil {
		t.Fatalf("sign RRset: %v", err)
	}

	if err := dnssecutil.VerifyRRSIG(sig, rrset, key, now); err != nil {
		t.Errorf("valid signature should verify: %v", err)
	}
	beforeInception := time.Unix(int64(sig.Inception)-3600, 0)
	if err := dnssecutil.VerifyRRSIG(sig, rrset, key, beforeInception); err == nil {
		t.Error("signature before inception should not verify")
	}
	afterExpiration := time.Unix(int64(sig.Expiration)+3600, 0)
	if err := dnssecutil.VerifyRRSIG(sig, rrset, key, afterExpiration); err == nil {
		t.Error("expired signature should not verify")
	}
}

func TestRSAExponentBeyondLocalVerifier(t *testing.T) {
	cases := []struct {
		name string
		key  *dns.DNSKEY
		want bool
	}{
		{"lv KSK exponent 2^32+1 is beyond local verifier", dnstest.RSADNSKEY("example.", dns.FlagZONE|dns.FlagSEP, dnstest.LVKSK42018), true},
		{"lb KSK exponent 65537 is fine (control)", dnstest.RSADNSKEY("example.", dns.FlagZONE|dns.FlagSEP, dnstest.LBKSK3842), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := dnssecutil.RSAExponentBeyondLocalVerifier(tc.key); got != tc.want {
				t.Errorf("RSAExponentBeyondLocalVerifier = %v, want %v", got, tc.want)
			}
		})
	}
}

// A nil key, a non-RSA key, and empty/garbage public keys must never be
// reported as "beyond the local verifier": the reclassification only applies to
// parseable RSA keys, and everything else stays on the existing code paths.
func TestRSAExponentBeyondLocalVerifierNonRSAAndMalformed(t *testing.T) {
	ecdsa := dnstest.RSADNSKEY("example.", dns.FlagZONE|dns.FlagSEP, dnstest.LVKSK42018)
	ecdsa.Algorithm = dns.ECDSAP256SHA256 // payload is irrelevant; the algorithm gate must reject first
	cases := []struct {
		name string
		key  *dns.DNSKEY
	}{
		{"nil key", nil},
		{"non-RSA algorithm", ecdsa},
		{"empty public key", dnstest.RSADNSKEY("example.", dns.FlagZONE|dns.FlagSEP, "")},
		{"non-base64 public key", dnstest.RSADNSKEY("example.", dns.FlagZONE|dns.FlagSEP, "!!!not base64!!!")},
		{"too short to hold an exponent", dnstest.RSADNSKEY("example.", dns.FlagZONE|dns.FlagSEP, "AQ==")}, // one byte
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if dnssecutil.RSAExponentBeyondLocalVerifier(tc.key) {
				t.Errorf("RSAExponentBeyondLocalVerifier = true, want false")
			}
		})
	}
}

// Both real fixtures are otherwise well-formed 2048-bit RSA keys; only the
// exponent differs. This documents that the large exponent does not break the
// modulus-sizing path, so KeySize still reports a correct bit length for the
// affected key.
func TestKeySizeUnaffected(t *testing.T) {
	for _, pub := range []string{dnstest.LVKSK42018, dnstest.LBKSK3842} {
		if bits := dnssecutil.KeySize(dnstest.RSADNSKEY("example.", dns.FlagZONE|dns.FlagSEP, pub)); bits != 2048 {
			t.Errorf("KeySize = %d, want 2048", bits)
		}
	}
}

// The DNS library verifies by canonicalizing the records it is handed: owner
// names are lowercased, every covered record is forced to the RRSIG original
// TTL, and the RRset is sorted. Those writes used to land on the caller's
// records, which are pointers into a packet cache shared between concurrent
// runs. Verification must leave the caller's records untouched.
func TestVerifyRRSIGDoesNotMutateCallerRecords(t *testing.T) {
	kp := dnstest.GenKey(t, "Example.Test", dns.ECDSAP256SHA256, false)
	key, signer := kp.Key, kp.Priv

	// Two records with distinct TTLs and mixed-case owner names, in an order
	// the canonical sort would change.
	second := &dns.A{Hdr: dns.Header{Name: dnsutil.Fqdn("B.Example.Test"), Class: dns.ClassINET, TTL: 900}}
	second.Addr = netip.MustParseAddr("192.0.2.2")
	first := &dns.A{Hdr: dns.Header{Name: dnsutil.Fqdn("a.Example.Test"), Class: dns.ClassINET, TTL: 300}}
	first.Addr = netip.MustParseAddr("192.0.2.1")
	rrset := []dns.RR{second, first}

	now := time.Now().UTC()
	sig := &dns.RRSIG{Hdr: dns.Header{Name: dnsutil.Fqdn("Example.Test"), Class: dns.ClassINET, TTL: 3600}}
	sig.TypeCovered = dns.TypeA
	sig.Algorithm = key.Algorithm
	sig.Labels = uint8(dnsutil.Labels(dnsutil.Fqdn("a.Example.Test")))
	sig.OrigTTL = 7200
	sig.Inception = uint32(now.Add(-time.Hour).Unix())
	sig.Expiration = uint32(now.Add(24 * time.Hour).Unix())
	sig.KeyTag = key.KeyTag()
	sig.SignerName = key.Hdr.Name
	if err := sig.Sign(signer, rrset, &dns.SignOption{}); err != nil {
		t.Fatalf("sign RRset: %v", err)
	}

	// Signing canonicalizes as well, so put the records back into the shape a
	// cached response holds: wire TTLs, mixed-case owner names, answer order.
	// Verification has to cope with that and hand it back unchanged.
	second.Hdr.Name = dnsutil.Fqdn("B.Example.Test")
	second.Hdr.TTL = 900
	first.Hdr.Name = dnsutil.Fqdn("a.Example.Test")
	first.Hdr.TTL = 300
	sig.Hdr.Name = dnsutil.Fqdn("Example.Test")
	rrset = []dns.RR{second, first}

	wantOrder := []dns.RR{rrset[0], rrset[1]}
	wantNames := []string{rrset[0].Header().Name, rrset[1].Header().Name}
	wantTTLs := []uint32{rrset[0].Header().TTL, rrset[1].Header().TTL}
	wantSigName := sig.Hdr.Name
	wantSignature := sig.Signature

	if err := dnssecutil.VerifyRRSIG(sig, rrset, key, now); err != nil {
		t.Fatalf("valid signature should verify: %v", err)
	}

	for i := range rrset {
		if rrset[i] != wantOrder[i] {
			t.Errorf("record %d: the RRset was reordered", i)
		}
		if got := rrset[i].Header().Name; got != wantNames[i] {
			t.Errorf("record %d: owner name changed to %q, want %q", i, got, wantNames[i])
		}
		if got := rrset[i].Header().TTL; got != wantTTLs[i] {
			t.Errorf("record %d: TTL changed to %d, want %d", i, got, wantTTLs[i])
		}
	}
	if sig.Hdr.Name != wantSigName {
		t.Errorf("RRSIG owner name changed to %q, want %q", sig.Hdr.Name, wantSigName)
	}
	if sig.Signature != wantSignature {
		t.Error("RRSIG signature field was rewritten")
	}
}

// The library's KeyTag method memoizes its result into the record it is called
// on. Engine code calls it on DNSKEYs that live in a packet cache shared with
// concurrent runs, so the helper has to return the same value while leaving the
// caller's record alone.
func TestKeyTagDoesNotMemoizeIntoCallerRecord(t *testing.T) {
	if got := dnssecutil.KeyTag(nil); got != 0 {
		t.Errorf("nil key: got %d, want 0", got)
	}

	key := &dns.DNSKEY{Hdr: dns.Header{Name: dnsutil.Fqdn("example.test"), Class: dns.ClassINET, TTL: 3600}}
	key.Flags = dns.FlagZONE
	key.Protocol = 3
	key.Algorithm = dns.ECDSAP256SHA256
	if _, err := key.Generate(256); err != nil {
		t.Fatalf("generate key: %v", err)
	}

	// A reference copy carries the memoized tag; the caller's record must not.
	reference := *key
	want := reference.KeyTag()

	if got := dnssecutil.KeyTag(key); got != want {
		t.Errorf("KeyTag = %d, want %d", got, want)
	}
	if key.Tag != 0 {
		t.Errorf("the caller's record was memoized into: Tag = %d, want 0", key.Tag)
	}

	// Repeat calls must stay stable even though nothing is cached on the record.
	if got := dnssecutil.KeyTag(key); got != want {
		t.Errorf("second KeyTag = %d, want %d", got, want)
	}

	// A record that already carries a tag is answered with the same value.
	memoized := *key
	memoized.Tag = want
	if got := dnssecutil.KeyTag(&memoized); got != want {
		t.Errorf("pre-memoized key: KeyTag = %d, want %d", got, want)
	}
}
