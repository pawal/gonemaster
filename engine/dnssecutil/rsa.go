package dnssecutil

import (
	"bytes"
	"crypto"
	"crypto/rsa"
	"encoding/base64"
	"errors"
	"io"
	"math/big"

	dns "codeberg.org/miekg/dns"
	"codeberg.org/miekg/dns/dnsutil"
	tssrsa "github.com/cloudflare/circl/tss/rsa"
)

// ErrRSAExponentUnsupported marks an RSA public exponent no local path verifies.
var ErrRSAExponentUnsupported = errors.New("rsa public exponent unsupported")

// OpenSSL's limits: wider exponents are declined, larger moduli are bad keys.
const (
	rsaMaxExponentBits = 64
	rsaMaxModulusBits  = 16384
)

// rsaPublicKey parses the RFC 3110 exponent and modulus.
func rsaPublicKey(key *dns.DNSKEY) (e, n *big.Int, ok bool) {
	if key.PublicKey == "" {
		return nil, nil, false
	}
	keybuf, err := base64.StdEncoding.DecodeString(key.PublicKey)
	if err != nil {
		return nil, nil, false
	}
	explen, off, ok := parseRSAExponentLen(keybuf)
	if !ok || explen == 0 || off+explen >= len(keybuf) {
		return nil, nil, false
	}
	return new(big.Int).SetBytes(keybuf[off : off+explen]), new(big.Int).SetBytes(keybuf[off+explen:]), true
}

// verifyLargeExponentRSA checks an RSA signature the library refuses for its
// exponent size: the library builds the signed data, math/big does the rest.
func verifyLargeExponentRSA(sig *dns.RRSIG, rrset []dns.RR, key *dns.DNSKEY) error {
	if sig.KeyTag != KeyTag(key) || sig.Hdr.Class != key.Hdr.Class || sig.Algorithm != key.Algorithm ||
		key.Flags&dns.FlagZONE == 0 || key.Protocol != 3 || !dns.EqualName(sig.SignerName, key.Hdr.Name) {
		return dns.ErrKey
	}
	e, n, ok := rsaPublicKey(key)
	if !ok || n.Sign() <= 0 || n.BitLen() > rsaMaxModulusBits {
		return dns.ErrKey
	}
	if e.BitLen() > rsaMaxExponentBits {
		return ErrRSAExponentUnsupported
	}
	sigbuf, err := base64.StdEncoding.DecodeString(sig.Signature)
	if err != nil {
		return err
	}
	size := (n.BitLen() + 7) / 8
	s := new(big.Int).SetBytes(sigbuf)
	if len(sigbuf) != size || s.Cmp(n) >= 0 {
		return dns.ErrSig
	}
	digest, err := signedDataDigest(sig, rrset)
	if err != nil {
		return err
	}
	em := make([]byte, size)
	s.Exp(s, e, n).FillBytes(em)
	// Pad reads only the modulus, so the exponent never reaches crypto/rsa.
	want, err := tssrsa.PKCS1v15Padder{}.Pad(&rsa.PublicKey{N: n}, dns.AlgorithmToHash[sig.Algorithm], digest)
	if err != nil {
		return err
	}
	if !bytes.Equal(em, want) {
		return dns.ErrSig
	}
	return nil
}

// digestCapture is the crypto.Signer handed to Sign only to receive the digest.
type digestCapture struct{ digest []byte }

func (c *digestCapture) Public() crypto.PublicKey { return nil }

func (c *digestCapture) Sign(_ io.Reader, digest []byte, _ crypto.SignerOpts) ([]byte, error) {
	c.digest = append([]byte(nil), digest...)
	return []byte{0}, nil
}

// signedDataDigest hashes the RFC 4034 signed data by running Sign with a
// capturing signer; Sign derives OrigTTL and Labels from the records.
func signedDataDigest(sig *dns.RRSIG, rrset []dns.RR) ([]byte, error) {
	if len(rrset) == 0 || dns.RRToType(rrset[0]) != sig.TypeCovered {
		return nil, dns.ErrSig
	}
	for _, rr := range rrset {
		hdr := rr.Header()
		hdr.TTL = sig.OrigTTL
		skip := dnsutil.Labels(hdr.Name) - int(sig.Labels)
		if skip < 0 {
			return nil, dns.ErrSig
		}
		if skip > 0 {
			hdr.Name = wildcardOwner(hdr.Name, skip) // RFC 4035 5.3.2 wildcard expansion
		}
	}
	local := *sig
	c := &digestCapture{}
	if err := local.Sign(c, rrset, &dns.SignOption{}); err != nil {
		return nil, err
	}
	if c.digest == nil {
		return nil, dns.ErrSig
	}
	return c.digest, nil
}

// wildcardOwner drops the first skip labels of name behind a "*" label.
func wildcardOwner(name string, skip int) string {
	off := 0
	for range skip {
		off, _ = dnsutil.Next(name, off)
	}
	return "*." + name[off:]
}
