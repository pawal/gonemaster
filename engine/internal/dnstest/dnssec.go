package dnstest

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"fmt"
	"io"
	"math/big"
	"testing"
	"time"

	dns "codeberg.org/miekg/dns"
	"codeberg.org/miekg/dns/dnsutil"
	"github.com/cloudflare/circl/sign/ed448"
	tssrsa "github.com/cloudflare/circl/tss/rsa"

	"codeberg.org/pawal/gonemaster/engine/dnssecutil"
)

// Known-good public keys captured from the live root-to-TLD chain. They are
// only ever parsed, never used to sign, so they stay stable across rotation.
const (
	// LVKSK42018 is the .lv KSK, keytag 42018: RSASHA256, 2048-bit, public
	// exponent 2^32+1 (5 bytes), which the library refuses and dnssecutil verifies itself.
	LVKSK42018 = "BQEAAAAByLU9dUcHHcl1eLgjLidTJKlwxsU9a580xierZ+WyfRBI47L3LLXAZZ0ub6Sea3qKP2mhP5ZBG/reXvyh3OSlHa39WoMiUUZFcuouCajBg7XeLGVPL4U1Ja1UW9wq/Oc8WU1dq4e+2Q8Dt8tipFvbL0AD0BhJAsfQuT3wperedwQAUKId0/JQOFNTWhEJaYN2P5IIhyRKWQp8OhtKmdNYQ5jfqqpXVO4zyqV+4ZxWurXJS8c7bKrE3OAewWEGAtTjeElfQ2CFAKWVjMOLeZ86+mgw7p3UHhGB+KuRaKg6fAtTcQYBF78Xe40wuj9EgGL19mp9v6tDwFe+Epow4SFSPQ=="

	// LVKSK42018E65 is LVKSK42018 with a 2^64+1 exponent, past the local ceiling; parse-only.
	LVKSK42018E65 = "CQEAAAAAAAAAAci1PXVHBx3JdXi4Iy4nUySpcMbFPWufNMYnq2flsn0QSOOy9yy1wGWdLm+knmt6ij9poT+WQRv63l78odzkpR2t/VqDIlFGRXLqLgmowYO13ixlTy+FNSWtVFvcKvznPFlNXauHvtkPA7fLYqRb2y9AA9AYSQLH0Lk98KXq3ncEAFCiHdPyUDhTU1oRCWmDdj+SCIckSlkKfDobSpnTWEOY36qqV1TuM8qlfuGcVrq1yUvHO2yqxNzgHsFhBgLU43hJX0NghQCllYzDi3mfOvpoMO6d1B4RgfirkWioOnwLU3EGARe/F3uNMLo/RIBi9fZqfb+rQ8BXvhKaMOEhUj0="

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
	signer, err := GenSigner(key)
	if err != nil {
		t.Fatalf("generate %s key for %s: %v", dns.AlgorithmToString[algo], owner, err)
	}
	return Keypair{Key: key, Priv: signer}
}

// GenSigner generates the private key for key's algorithm and fills in the
// public half.
func GenSigner(key *dns.DNSKEY) (crypto.Signer, error) {
	return GenSignerBits(key, 256)
}

// GenSignerBits is GenSigner at the given key size. It generates again when the
// key tag comes out zero, which the DNS library refuses to sign with.
func GenSignerBits(key *dns.DNSKEY, bits int) (crypto.Signer, error) {
	for {
		signer, err := genSigner(key, bits)
		if err != nil || dnssecutil.KeyTag(key) != 0 {
			return signer, err
		}
	}
}

// genSigner generates one key. The DNS library cannot generate Ed448, so that
// one is built here.
func genSigner(key *dns.DNSKEY, bits int) (crypto.Signer, error) {
	if key.Algorithm == dns.ED448 {
		pub, priv, err := ed448.GenerateKey(rand.Reader)
		if err != nil {
			return nil, err
		}
		key.PublicKey = base64.StdEncoding.EncodeToString(pub)
		return priv, nil
	}
	priv, err := key.Generate(bits)
	if err != nil {
		return nil, err
	}
	signer, ok := priv.(crypto.Signer)
	if !ok {
		return nil, fmt.Errorf("algorithm %d key is not a crypto.Signer", key.Algorithm)
	}
	return signer, nil
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

// LVExponent is the .lv KSK public exponent, 2^32+1.
var LVExponent = new(big.Int).SetUint64(1<<32 + 1)

// GenRSAKeyWithExponent generates an RSASHA256 zone key with odd exponent e, signing in math/big.
func GenRSAKeyWithExponent(t testing.TB, owner string, e *big.Int, bits int, sep bool) Keypair {
	t.Helper()
	if e.Bit(0) == 0 {
		t.Fatalf("RSA exponent %s is even", e)
	}
	key := &dns.DNSKEY{Hdr: dns.Header{Name: dnsutil.Fqdn(owner), Class: dns.ClassINET, TTL: 3600}}
	key.Flags = dns.FlagZONE
	if sep {
		key.Flags |= dns.FlagSEP
	}
	key.Protocol = 3
	key.Algorithm = dns.RSASHA256
	one := big.NewInt(1)
	for {
		p, err := rand.Prime(rand.Reader, bits/2)
		if err != nil {
			t.Fatalf("generate prime: %v", err)
		}
		q, err := rand.Prime(rand.Reader, bits-bits/2)
		if err != nil {
			t.Fatalf("generate prime: %v", err)
		}
		phi := new(big.Int).Mul(new(big.Int).Sub(p, one), new(big.Int).Sub(q, one))
		d := new(big.Int).ModInverse(e, phi)
		if d == nil {
			continue // e shares a factor with phi, draw again
		}
		n := new(big.Int).Mul(p, q)
		key.PublicKey = RSAPublicKey(e, n)
		if dnssecutil.KeyTag(key) == 0 {
			continue // a zero key tag cannot sign, draw again
		}
		return Keypair{Key: key, Priv: &bigRSASigner{n: n, d: d}}
	}
}

// RSAPublicKey encodes e and n per RFC 3110.
func RSAPublicKey(e, n *big.Int) string {
	eb := e.Bytes()
	buf := []byte{byte(len(eb))}
	if len(eb) > 255 {
		buf = []byte{0, byte(len(eb) >> 8), byte(len(eb))}
	}
	buf = append(buf, eb...)
	buf = append(buf, n.Bytes()...)
	return base64.StdEncoding.EncodeToString(buf)
}

// bigRSASigner signs PKCS#1 v1.5 in math/big; Sign never asks for Public.
type bigRSASigner struct{ n, d *big.Int }

func (s *bigRSASigner) Public() crypto.PublicKey { return nil }

func (s *bigRSASigner) Sign(_ io.Reader, digest []byte, opts crypto.SignerOpts) ([]byte, error) {
	em, err := tssrsa.PKCS1v15Padder{}.Pad(&rsa.PublicKey{N: s.n}, opts.HashFunc(), digest)
	if err != nil {
		return nil, err
	}
	out := make([]byte, (s.n.BitLen()+7)/8)
	new(big.Int).Exp(new(big.Int).SetBytes(em), s.d, s.n).FillBytes(out)
	return out, nil
}
