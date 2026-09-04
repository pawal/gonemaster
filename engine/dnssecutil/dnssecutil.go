// Package dnssecutil holds DNSSEC primitives shared by the DNSSEC testcases
// and the per-run chain extractor: algorithm support checks, RSA key sizing,
// RRSIG verification, the RFC 8624 algorithm and digest policy, and the fixed
// key and signature lengths.
package dnssecutil

import (
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	dns "codeberg.org/miekg/dns"
)

// AlgorithmSupported reports whether the signing algorithm can be verified.
func AlgorithmSupported(algo uint8) bool {
	switch algo {
	case dns.RSASHA1, dns.RSASHA1NSEC3SHA1, dns.RSASHA256, dns.RSASHA512, dns.ECDSAP256SHA256, dns.ECDSAP384SHA384, dns.ED25519, dns.ED448, dns.MLDSA44:
		return true
	default:
		return false
	}
}

// KeyTag returns the key tag without memoizing it into the caller's record,
// which may be a cached record shared between concurrent runs.
func KeyTag(key *dns.DNSKEY) uint16 {
	if key == nil {
		return 0
	}
	local := *key
	return local.KeyTag()
}

// DigestSupported reports whether DNSKEY.ToDS can recompute the digest type.
// GOST (3, 5) and SM3 (6) have no implementation.
func DigestSupported(digest uint8) bool {
	switch digest {
	case dns.SHA1, dns.SHA256, dns.SHA384:
		return true
	default:
		return false
	}
}

// parseRSAExponentLen decodes the RFC 3110 exponent-length prefix of an RSA
// DNSKEY public key, returning the exponent length in bytes and the offset at
// which the exponent begins.
func parseRSAExponentLen(keybuf []byte) (explen, off int, ok bool) {
	if len(keybuf) < 1 {
		return 0, 0, false
	}
	explen = int(keybuf[0])
	off = 1
	if explen == 0 {
		if len(keybuf) < 3 {
			return 0, 0, false
		}
		explen = int(keybuf[1])<<8 | int(keybuf[2])
		off = 3
	}
	return explen, off, true
}

// KeySize returns the key size in bits for the key's algorithm, or 0 when not derivable.
func KeySize(key *dns.DNSKEY) int {
	if key == nil {
		return 0
	}
	switch key.Algorithm {
	case dns.RSAMD5, dns.RSASHA1, dns.RSASHA1NSEC3SHA1, dns.RSASHA256, dns.RSASHA512:
		return rsaModulusBits(key)
	case dns.ECDSAP256SHA256:
		return 256
	case dns.ECDSAP384SHA384:
		return 384
	case dns.ED25519:
		return 256
	case dns.ED448:
		return 456
	case dns.ECCGOST, dns.ECCGOST12, dns.SM2SM3:
		return 256 // curve size, as for ECDSA
	default:
		return 0
	}
}

// Fixed rdata lengths in bits (RFC 5933, 6605, 8080). Encoded lengths, not the
// curve size KeySize reports: a P-256 key is two 256-bit coordinates.
var (
	expectedKeyBits = map[uint8]int{
		dns.ECCGOST: 512, dns.ECDSAP256SHA256: 512, dns.ECDSAP384SHA384: 768,
		dns.ED25519: 256, dns.ED448: 456,
	}
	expectedSignatureBits = map[uint8]int{
		dns.ECCGOST: 512, dns.ECDSAP256SHA256: 512, dns.ECDSAP384SHA384: 768,
		dns.ED25519: 512, dns.ED448: 912,
	}
)

// ExpectedKeyBits returns the DNSKEY public key length the algorithm fixes, or
// 0 where it fixes none, as RSA does not.
func ExpectedKeyBits(algorithm uint8) int { return expectedKeyBits[algorithm] }

// ExpectedSignatureBits returns the RRSIG signature length the algorithm
// fixes, or 0 where it fixes none.
func ExpectedSignatureBits(algorithm uint8) int { return expectedSignatureBits[algorithm] }

// rsaModulusBits returns the RSA modulus size in bits, or 0 when not derivable.
func rsaModulusBits(key *dns.DNSKEY) int {
	_, n, ok := rsaPublicKey(key)
	if !ok {
		return 0
	}
	return n.BitLen()
}

// RSAExponentBeyondLocalVerifier reports whether key is an RSA DNSKEY whose
// public exponent exceeds what the local RRSIG verifier (miekg/dns + crypto/rsa)
// can use. Both reject exponents encoded in more than 4 bytes or with a value
// greater than 2^31-1, although such signatures may be perfectly valid (e.g.
// the .lv TLD KSK, whose exponent is 2^32+1); VerifyRRSIG checks those keys
// itself. Returns false for non-RSA algorithms and for unparseable keys.
func RSAExponentBeyondLocalVerifier(key *dns.DNSKEY) bool {
	if key == nil {
		return false
	}
	switch key.Algorithm {
	case dns.RSASHA1, dns.RSASHA1NSEC3SHA1, dns.RSASHA256, dns.RSASHA512:
	default:
		return false
	}
	keybuf, err := base64.StdEncoding.DecodeString(key.PublicKey)
	if err != nil || len(keybuf) < 3 {
		return false
	}
	explen, off, ok := parseRSAExponentLen(keybuf)
	if !ok {
		return false
	}
	if explen > 4 {
		return true // a >4-byte exponent is necessarily > 2^31-1
	}
	if off+explen > len(keybuf) {
		return false // malformed; leave to existing handling
	}
	var expo uint64
	for _, b := range keybuf[off : off+explen] {
		expo = expo<<8 | uint64(b)
	}
	return expo > (1<<31 - 1)
}

// VerifyRRSIG checks the signature against the RRset and key at the given time.
// ErrRSAExponentUnsupported reports an RSA exponent no local path verifies.
func VerifyRRSIG(sig *dns.RRSIG, rrset []dns.RR, key *dns.DNSKEY, at time.Time) (err error) {
	if sig == nil || key == nil {
		return errors.New("missing rrsig or key")
	}
	if !sig.ValidPeriod(at) {
		return errors.New("rrsig not valid at time")
	}
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("dns library panic during RRSIG verification: %v", r)
		}
	}()
	// Verification canonicalizes in place, and cached records are shared between
	// concurrent runs, so verify on copies.
	local := *sig
	keyCopy, ok := key.Clone().(*dns.DNSKEY)
	if !ok {
		return errors.New("dnskey copy changed type")
	}
	copies := make([]dns.RR, 0, len(rrset))
	for _, rr := range rrset {
		if rr == nil {
			continue
		}
		copies = append(copies, rr.Clone())
	}
	if RSAExponentBeyondLocalVerifier(key) {
		return verifyLargeExponentRSA(&local, copies, keyCopy)
	}
	// The hook shares the library's default branch with every unverifiable
	// algorithm, so attaching it for those would turn ErrAlg into ErrSig.
	opts := &dns.SignOption{}
	if sig.Algorithm == dns.ED448 {
		opts.VerifyFunc = ed448Verify
	}
	return local.Verify(keyCopy, copies, opts)
}
