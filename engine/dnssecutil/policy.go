package dnssecutil

import (
	"slices"

	dns "codeberg.org/miekg/dns"
)

// RFC 8624 s3.1. ECC-GOST must not sign yet may still be validated, so the
// two directions are separate sets.
var (
	algorithmSigningProhibited     = []uint8{dns.RSAMD5, dns.DSA, dns.DSANSEC3SHA1, dns.ECCGOST}
	algorithmValidationProhibited  = []uint8{dns.RSAMD5, dns.DSA, dns.DSANSEC3SHA1}
	algorithmSigningNotRecommended = []uint8{dns.RSASHA1, dns.RSASHA1NSEC3SHA1, dns.RSASHA512}
)

// RFC 8624 s3.3. Digest 0 is the RFC 8078 delete signal; none is merely not
// recommended.
var (
	digestSigningProhibited    = []uint8{0, dns.SHA1, dns.GOST94}
	digestValidationProhibited = []uint8{0}
)

// AlgorithmSigningProhibited reports an algorithm a zone must not be signed
// with. RFC 8624 policy, not the registry status dnssec05 reports.
func AlgorithmSigningProhibited(algorithm uint8) bool {
	return slices.Contains(algorithmSigningProhibited, algorithm)
}

// AlgorithmValidationProhibited reports an algorithm a validator must not use.
func AlgorithmValidationProhibited(algorithm uint8) bool {
	return slices.Contains(algorithmValidationProhibited, algorithm)
}

// AlgorithmSigningNotRecommended reports an algorithm still validated but no
// longer recommended for signing.
func AlgorithmSigningNotRecommended(algorithm uint8) bool {
	return slices.Contains(algorithmSigningNotRecommended, algorithm)
}

// DigestSigningProhibited reports a digest type a delegation must not use.
func DigestSigningProhibited(digest uint8) bool {
	return slices.Contains(digestSigningProhibited, digest)
}

// DigestValidationProhibited reports a digest type a validator must not use.
func DigestValidationProhibited(digest uint8) bool {
	return slices.Contains(digestValidationProhibited, digest)
}

// DigestSupersedesSHA1 reports the digest that makes the SHA-1 records of the
// same DS RRset ignored (RFC 4509 s3, SHA-256 alone).
func DigestSupersedesSHA1(digest uint8) bool { return digest == dns.SHA256 }

// DigestStrongerThanSHA1 reports a digest that outranks SHA-1, mandate or not.
func DigestStrongerThanSHA1(digest uint8) bool {
	return digest == dns.SHA256 || digest == dns.SHA384
}
