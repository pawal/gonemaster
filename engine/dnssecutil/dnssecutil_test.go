package dnssecutil

import (
	"crypto"
	"net/netip"
	"testing"
	"time"

	dns "codeberg.org/miekg/dns"
	"codeberg.org/miekg/dns/dnsutil"
)

func TestAlgorithmSupported(t *testing.T) {
	supported := []uint8{dns.RSASHA1, dns.RSASHA1NSEC3SHA1, dns.RSASHA256, dns.RSASHA512, dns.ECDSAP256SHA256, dns.ECDSAP384SHA384, dns.ED25519}
	for _, algo := range supported {
		if !AlgorithmSupported(algo) {
			t.Errorf("algorithm %d should be supported", algo)
		}
	}
	unsupported := []uint8{dns.RSAMD5, dns.DSA, dns.ECCGOST, dns.ED448, 0, 99}
	for _, algo := range unsupported {
		if AlgorithmSupported(algo) {
			t.Errorf("algorithm %d should not be supported", algo)
		}
	}
}

func TestKeySize(t *testing.T) {
	if got := KeySize(nil); got != 0 {
		t.Errorf("nil key: got %d, want 0", got)
	}
	if got := KeySize(&dns.DNSKEY{}); got != 0 {
		t.Errorf("empty key: got %d, want 0", got)
	}

	key := &dns.DNSKEY{Hdr: dns.Header{Name: dnsutil.Fqdn("example.test"), Class: dns.ClassINET, TTL: 3600}}
	key.Flags = dns.FlagZONE | dns.FlagSEP
	key.Protocol = 3
	key.Algorithm = dns.RSASHA256
	if _, err := key.Generate(1024); err != nil {
		t.Fatalf("generate key: %v", err)
	}
	if got := KeySize(key); got < 1000 || got > 1024 {
		t.Errorf("1024-bit RSA key: got %d bits, want ~1024", got)
	}
}

func TestVerifyRRSIG(t *testing.T) {
	if err := VerifyRRSIG(nil, nil, nil, time.Unix(1000, 0)); err == nil {
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

	if err := VerifyRRSIG(sig, rrset, key, now); err != nil {
		t.Errorf("valid signature should verify: %v", err)
	}
	beforeInception := time.Unix(int64(sig.Inception)-3600, 0)
	if err := VerifyRRSIG(sig, rrset, key, beforeInception); err == nil {
		t.Error("signature before inception should not verify")
	}
	afterExpiration := time.Unix(int64(sig.Expiration)+3600, 0)
	if err := VerifyRRSIG(sig, rrset, key, afterExpiration); err == nil {
		t.Error("expired signature should not verify")
	}
}

// Real DNSKEY public keys captured 2026-07-14. The public keys are stable
// across RRSIG rotation, so these fixtures are deterministic and need no
// network access.
const (
	// .lv KSK, keytag 42018: RSASHA256, 2048-bit, public exponent 2^32+1
	// (4294967297, 5 bytes). miekg/dns and crypto/rsa both reject exponents
	// this large, so gonemaster cannot verify its signatures locally even
	// though .lv validates on 1.1.1.1 / 8.8.8.8.
	lvKSK42018 = "BQEAAAAByLU9dUcHHcl1eLgjLidTJKlwxsU9a580xierZ+WyfRBI47L3LLXAZZ0ub6Sea3qKP2mhP5ZBG/reXvyh3OSlHa39WoMiUUZFcuouCajBg7XeLGVPL4U1Ja1UW9wq/Oc8WU1dq4e+2Q8Dt8tipFvbL0AD0BhJAsfQuT3wperedwQAUKId0/JQOFNTWhEJaYN2P5IIhyRKWQp8OhtKmdNYQ5jfqqpXVO4zyqV+4ZxWurXJS8c7bKrE3OAewWEGAtTjeElfQ2CFAKWVjMOLeZ86+mgw7p3UHhGB+KuRaKg6fAtTcQYBF78Xe40wuj9EgGL19mp9v6tDwFe+Epow4SFSPQ=="

	// .lb KSK, keytag 3842: RSASHA256, 2048-bit, exponent 65537 (normal).
	// Negative control: .lb is NOT affected by the large-exponent limitation,
	// so it must not be reclassified.
	lbKSK3842 = "AwEAAcOaB0E27SPJIT/u/dQzN6NYXhVrBVGWPuh7gMJPHY1DULKuzAbZr4EwA/RcNnkBVygzDrZVxkJuBrT9uqjiuqK67VAupJDnTW3zKYzxmOpBQJW01B9LHyYMe3JYopl4BagGvzK3W5EGQBHuTk35/3y1a+d/M7Iky+9XRNBGwFbGlcXCBTg6uvdnGbyQkF2/ESmYOhXVw114YKREcFI3KBD5d6N+3nb6dy2nV3Oq+N7JRiepcxEzrXrjNo7yRoSToLJH4SfzVl/rdcJmqi59OgBfAFBQDTdwIddWkFSfOcEWYGxKCWvqmHbLCAKlmKaWcHvLIvABO0VTMnJz18JnVoE="
)

func rsaDNSKEY(flags uint16, pub string) *dns.DNSKEY {
	key := &dns.DNSKEY{Hdr: dns.Header{Name: dnsutil.Fqdn("example."), Class: dns.ClassINET, TTL: 3600}}
	key.Flags = flags
	key.Protocol = 3
	key.Algorithm = dns.RSASHA256
	key.PublicKey = pub
	return key
}

func TestRSAExponentBeyondLocalVerifier(t *testing.T) {
	cases := []struct {
		name string
		key  *dns.DNSKEY
		want bool
	}{
		{"lv KSK exponent 2^32+1 is beyond local verifier", rsaDNSKEY(dns.FlagZONE|dns.FlagSEP, lvKSK42018), true},
		{"lb KSK exponent 65537 is fine (control)", rsaDNSKEY(dns.FlagZONE|dns.FlagSEP, lbKSK3842), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := RSAExponentBeyondLocalVerifier(tc.key); got != tc.want {
				t.Errorf("RSAExponentBeyondLocalVerifier = %v, want %v", got, tc.want)
			}
		})
	}
}

// A nil key, a non-RSA key, and empty/garbage public keys must never be
// reported as "beyond the local verifier": the reclassification only applies to
// parseable RSA keys, and everything else stays on the existing code paths.
func TestRSAExponentBeyondLocalVerifierNonRSAAndMalformed(t *testing.T) {
	ecdsa := rsaDNSKEY(dns.FlagZONE|dns.FlagSEP, lvKSK42018)
	ecdsa.Algorithm = dns.ECDSAP256SHA256 // payload is irrelevant; the algorithm gate must reject first
	cases := []struct {
		name string
		key  *dns.DNSKEY
	}{
		{"nil key", nil},
		{"non-RSA algorithm", ecdsa},
		{"empty public key", rsaDNSKEY(dns.FlagZONE|dns.FlagSEP, "")},
		{"non-base64 public key", rsaDNSKEY(dns.FlagZONE|dns.FlagSEP, "!!!not base64!!!")},
		{"too short to hold an exponent", rsaDNSKEY(dns.FlagZONE|dns.FlagSEP, "AQ==")}, // one byte
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if RSAExponentBeyondLocalVerifier(tc.key) {
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
	for _, pub := range []string{lvKSK42018, lbKSK3842} {
		if bits := KeySize(rsaDNSKEY(dns.FlagZONE|dns.FlagSEP, pub)); bits != 2048 {
			t.Errorf("KeySize = %d, want 2048", bits)
		}
	}
}
