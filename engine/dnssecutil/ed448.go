package dnssecutil

import (
	"encoding/base64"

	dns "codeberg.org/miekg/dns"
	"github.com/cloudflare/circl/sign/ed448"
)

// ed448Verify verifies per RFC 8080: PureEdDSA, empty context, no prehash.
// The library has no ED448 case, so VerifyRRSIG passes this as VerifyFunc.
func ed448Verify(key *dns.DNSKEY, message, signature []byte) bool {
	if key == nil || key.Algorithm != dns.ED448 || len(signature) != ed448.SignatureSize {
		return false
	}
	pub, err := base64.StdEncoding.DecodeString(key.PublicKey)
	if err != nil || len(pub) != ed448.PublicKeySize {
		return false
	}
	return ed448.Verify(ed448.PublicKey(pub), message, signature, "")
}
