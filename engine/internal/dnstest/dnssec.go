package dnstest

import (
	"crypto"
	"testing"
	"time"

	dns "codeberg.org/miekg/dns"
	"codeberg.org/miekg/dns/dnsutil"

	"codeberg.org/pawal/gonemaster/engine/dnssecutil"
)

// Known-good public keys captured from the live root-to-TLD chain. They are
// only ever parsed, never used to sign, so they stay stable across rotation.
const (
	// LVKSK42018 is the .lv KSK, keytag 42018: RSASHA256, 2048-bit, public
	// exponent 2^32+1 (4294967297, 5 bytes). miekg/dns and crypto/rsa both
	// reject exponents this large, so gonemaster cannot verify its signatures
	// locally even though .lv validates on 1.1.1.1 / 8.8.8.8.
	LVKSK42018 = "BQEAAAAByLU9dUcHHcl1eLgjLidTJKlwxsU9a580xierZ+WyfRBI47L3LLXAZZ0ub6Sea3qKP2mhP5ZBG/reXvyh3OSlHa39WoMiUUZFcuouCajBg7XeLGVPL4U1Ja1UW9wq/Oc8WU1dq4e+2Q8Dt8tipFvbL0AD0BhJAsfQuT3wperedwQAUKId0/JQOFNTWhEJaYN2P5IIhyRKWQp8OhtKmdNYQ5jfqqpXVO4zyqV+4ZxWurXJS8c7bKrE3OAewWEGAtTjeElfQ2CFAKWVjMOLeZ86+mgw7p3UHhGB+KuRaKg6fAtTcQYBF78Xe40wuj9EgGL19mp9v6tDwFe+Epow4SFSPQ=="

	// LBKSK3842 is the .lb KSK, keytag 3842: RSASHA256, 2048-bit, exponent
	// 65537 (normal). Negative control: .lb is NOT affected by the
	// large-exponent limitation, so it must not be reclassified.
	LBKSK3842 = "AwEAAcOaB0E27SPJIT/u/dQzN6NYXhVrBVGWPuh7gMJPHY1DULKuzAbZr4EwA/RcNnkBVygzDrZVxkJuBrT9uqjiuqK67VAupJDnTW3zKYzxmOpBQJW01B9LHyYMe3JYopl4BagGvzK3W5EGQBHuTk35/3y1a+d/M7Iky+9XRNBGwFbGlcXCBTg6uvdnGbyQkF2/ESmYOhXVw114YKREcFI3KBD5d6N+3nb6dy2nV3Oq+N7JRiepcxEzrXrjNo7yRoSToLJH4SfzVl/rdcJmqi59OgBfAFBQDTdwIddWkFSfOcEWYGxKCWvqmHbLCAKlmKaWcHvLIvABO0VTMnJz18JnVoE="
)

// Keypair is a generated DNSKEY with the signer that goes with it.
type Keypair struct {
	Key  *dns.DNSKEY
	Priv crypto.Signer
}

// GenKey generates a zone key for owner. A SEP key gets the SEP flag; the
// generated key exercises the real crypto path, so a library that loses support
// for algo fails the test rather than silently skipping it.
func GenKey(t testing.TB, owner string, algo uint8, sep bool) Keypair {
	t.Helper()

	key := &dns.DNSKEY{Hdr: dns.Header{Name: dnsutil.Fqdn(owner), Class: dns.ClassINET, TTL: 3600}}
	key.Flags = dns.FlagZONE
	if sep {
		key.Flags |= dns.FlagSEP
	}
	key.Protocol = 3
	key.Algorithm = algo
	priv, err := key.Generate(256)
	if err != nil {
		t.Fatalf("generate %s key for %s: %v", dns.AlgorithmToString[algo], owner, err)
	}
	signer, ok := priv.(crypto.Signer)
	if !ok {
		t.Fatalf("%s key for %s is not a crypto.Signer", dns.AlgorithmToString[algo], owner)
	}
	return Keypair{Key: key, Priv: signer}
}

// SignRRset produces an RRSIG over rrset signed by kp, valid across the window.
func SignRRset(t testing.TB, kp Keypair, rrset []dns.RR, owner string, signer string, inception time.Time, expiration time.Time) *dns.RRSIG {
	t.Helper()

	sig := &dns.RRSIG{Hdr: dns.Header{Name: dnsutil.Fqdn(owner), Class: dns.ClassINET, TTL: 3600}}
	sig.Algorithm = kp.Key.Algorithm
	sig.Inception = uint32(inception.Unix())
	sig.Expiration = uint32(expiration.Unix())
	sig.KeyTag = dnssecutil.KeyTag(kp.Key)
	sig.SignerName = dnsutil.Fqdn(signer)
	if err := sig.Sign(kp.Priv, rrset, &dns.SignOption{}); err != nil {
		t.Fatalf("sign rrset for %s: %v", owner, err)
	}
	return sig
}

// RSADNSKEY builds an RSASHA256 DNSKEY carrying a captured public key.
func RSADNSKEY(owner string, flags uint16, pub string) *dns.DNSKEY {
	key := &dns.DNSKEY{Hdr: dns.Header{Name: dnsutil.Fqdn(owner), Class: dns.ClassINET, TTL: 3600}}
	key.Flags = flags
	key.Protocol = 3
	key.Algorithm = dns.RSASHA256
	key.PublicKey = pub
	return key
}
