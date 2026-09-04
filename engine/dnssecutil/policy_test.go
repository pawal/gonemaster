package dnssecutil

import (
	"testing"

	dns "codeberg.org/miekg/dns"
)

// RFC 8624 section 3.1, one case per row of the DNSKEY algorithm table.
// Signing and validation are judged apart: ECC-GOST must not sign, yet a
// validator that implements it may still use it, and RSASHA1 is the reverse
// of nothing at all - discouraged for signing while validation stays a MUST.
func TestAlgorithmPolicy(t *testing.T) {
	for _, c := range []struct {
		algorithm                                     uint8
		signing, validation, notRecommendedForSigning bool
	}{
		{dns.RSAMD5, true, true, false},
		{dns.DSA, true, true, false},
		{dns.RSASHA1, false, false, true},
		{dns.DSANSEC3SHA1, true, true, false},
		{dns.RSASHA1NSEC3SHA1, false, false, true},
		{dns.RSASHA256, false, false, false},
		{dns.RSASHA512, false, false, true},
		{dns.ECCGOST, true, false, false},
		{dns.ECDSAP256SHA256, false, false, false},
		{dns.ECDSAP384SHA384, false, false, false},
		{dns.ED25519, false, false, false},
		{dns.ED448, false, false, false},
	} {
		name := dns.AlgorithmToString[c.algorithm]
		if got := AlgorithmSigningProhibited(c.algorithm); got != c.signing {
			t.Errorf("AlgorithmSigningProhibited(%s) = %v, want %v", name, got, c.signing)
		}
		if got := AlgorithmValidationProhibited(c.algorithm); got != c.validation {
			t.Errorf("AlgorithmValidationProhibited(%s) = %v, want %v", name, got, c.validation)
		}
		if got := AlgorithmSigningNotRecommended(c.algorithm); got != c.notRecommendedForSigning {
			t.Errorf("AlgorithmSigningNotRecommended(%s) = %v, want %v", name, got, c.notRecommendedForSigning)
		}
	}
}

// Prohibited and not recommended are alternatives in RFC 8624, never both, so
// an algorithm reported under each at once would be a table error.
func TestAlgorithmPolicyDoesNotOverlap(t *testing.T) {
	for algorithm := range 256 {
		algo := uint8(algorithm)
		if AlgorithmSigningProhibited(algo) && AlgorithmSigningNotRecommended(algo) {
			t.Errorf("algorithm %d is both prohibited and not recommended for signing", algo)
		}
	}
}

// RFC 8624 section 3.3, the DS and CDS digest table. Digest 0 is the RFC 8078
// delete signal and belongs in neither direction; SHA-1 and GOST must not be
// used in a delegation while validation still accepts them.
func TestDigestPolicy(t *testing.T) {
	for _, c := range []struct {
		digest              uint8
		signing, validation bool
	}{
		{0, true, true},
		{dns.SHA1, true, false},
		{dns.SHA256, false, false},
		{dns.GOST94, true, false},
		{dns.SHA384, false, false},
	} {
		if got := DigestSigningProhibited(c.digest); got != c.signing {
			t.Errorf("DigestSigningProhibited(%d) = %v, want %v", c.digest, got, c.signing)
		}
		if got := DigestValidationProhibited(c.digest); got != c.validation {
			t.Errorf("DigestValidationProhibited(%d) = %v, want %v", c.digest, got, c.validation)
		}
	}
}

// RFC 4509 section 3 names SHA-256 alone: its presence in a DS RRset makes the
// SHA-1 records of that RRset ignored. SHA-384 is stronger without carrying
// that mandate, which is why the two questions are asked separately.
func TestSHA1IsSupersededBySHA256Alone(t *testing.T) {
	for _, c := range []struct {
		digest               uint8
		supersedes, stronger bool
	}{
		{dns.SHA1, false, false},
		{dns.SHA256, true, true},
		{dns.GOST94, false, false},
		{dns.SHA384, false, true},
	} {
		if got := DigestSupersedesSHA1(c.digest); got != c.supersedes {
			t.Errorf("DigestSupersedesSHA1(%d) = %v, want %v", c.digest, got, c.supersedes)
		}
		if got := DigestStrongerThanSHA1(c.digest); got != c.stronger {
			t.Errorf("DigestStrongerThanSHA1(%d) = %v, want %v", c.digest, got, c.stronger)
		}
	}
}
