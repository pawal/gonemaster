// Package dnssecutil holds DNSSEC primitives shared by the DNSSEC testcases
// and the per-run chain extractor: algorithm support checks, RSA key sizing,
// and RRSIG verification.
package dnssecutil

import (
	"encoding/base64"
	"errors"
	"fmt"
	"math/big"
	"time"

	dns "codeberg.org/miekg/dns"
)

// AlgorithmSupported reports whether the signing algorithm can be verified.
func AlgorithmSupported(algo uint8) bool {
	switch algo {
	case dns.RSASHA1, dns.RSASHA1NSEC3SHA1, dns.RSASHA256, dns.RSASHA512, dns.ECDSAP256SHA256, dns.ECDSAP384SHA384, dns.ED25519, dns.MLDSA44:
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

// DigestSupported reports whether the DS digest type is a known IANA type.
func DigestSupported(digest uint8) bool {
	switch digest {
	case 1, 2, 3, 4:
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
	default:
		return 0
	}
}

// rsaModulusBits returns the RSA modulus size in bits, or 0 when not derivable.
func rsaModulusBits(key *dns.DNSKEY) int {
	if key.PublicKey == "" {
		return 0
	}
	keybuf, err := base64.StdEncoding.DecodeString(key.PublicKey)
	if err != nil || len(keybuf) == 0 {
		return 0
	}

	explen, keyoff, ok := parseRSAExponentLen(keybuf)
	if !ok || explen <= 0 || keyoff+explen >= len(keybuf) {
		return 0
	}

	modulus := keybuf[keyoff+explen:]
	if len(modulus) == 0 {
		return 0
	}
	return new(big.Int).SetBytes(modulus).BitLen()
}

// RSAExponentBeyondLocalVerifier reports whether key is an RSA DNSKEY whose
// public exponent exceeds what the local RRSIG verifier (miekg/dns + crypto/rsa)
// can use. Both reject exponents encoded in more than 4 bytes or with a value
// greater than 2^31-1. Such a key cannot be checked here even though its
// signatures may be perfectly valid (e.g. the .lv TLD KSK, whose exponent is
// 2^32+1). Returns false for non-RSA algorithms and for unparseable keys.
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
	return local.Verify(keyCopy, copies, &dns.SignOption{})
}
