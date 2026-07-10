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
	case dns.RSASHA1, dns.RSASHA1NSEC3SHA1, dns.RSASHA256, dns.RSASHA512, dns.ECDSAP256SHA256, dns.ECDSAP384SHA384, dns.ED25519:
		return true
	default:
		return false
	}
}

// KeySize returns the RSA modulus size in bits, or 0 when not derivable.
func KeySize(key *dns.DNSKEY) int {
	if key == nil || key.PublicKey == "" {
		return 0
	}
	keybuf, err := base64.StdEncoding.DecodeString(key.PublicKey)
	if err != nil || len(keybuf) == 0 {
		return 0
	}

	explen := int(keybuf[0])
	keyoff := 1
	if explen == 0 {
		if len(keybuf) < 3 {
			return 0
		}
		explen = int(keybuf[1])<<8 | int(keybuf[2])
		keyoff = 3
	}
	if explen <= 0 || keyoff+explen >= len(keybuf) {
		return 0
	}

	modulus := keybuf[keyoff+explen:]
	if len(modulus) == 0 {
		return 0
	}
	return new(big.Int).SetBytes(modulus).BitLen()
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
	return sig.Verify(key, rrset, &dns.SignOption{})
}
