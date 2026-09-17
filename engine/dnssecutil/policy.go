package dnssecutil

import (
	"slices"

	dns "codeberg.org/miekg/dns"
)

// IANA DNS Security Algorithm Numbers, the signing and validation columns (RFC 9904).
var (
	algorithmSigningProhibited     = []uint8{dns.RSAMD5, dns.DSA, dns.DSANSEC3SHA1, dns.ECCGOST}
	algorithmValidationProhibited  = []uint8{dns.RSAMD5, dns.DSA, dns.DSANSEC3SHA1}
	algorithmSigningNotRecommended = []uint8{dns.RSASHA1, dns.RSASHA1NSEC3SHA1, dns.RSASHA512}
)

// IANA DS Digest Algorithms, the delegation and validation columns (RFC 9904); 0 is the RFC 8078 delete signal.
var (
	digestSigningProhibited    = []uint8{0, dns.SHA1, dns.GOST94}
	digestValidationProhibited = []uint8{0}
)

// AlgorithmSigningProhibited reports an algorithm marked MUST NOT for signing.
func AlgorithmSigningProhibited(algorithm uint8) bool {
	return slices.Contains(algorithmSigningProhibited, algorithm)
}

// AlgorithmValidationProhibited reports an algorithm marked MUST NOT for validation.
func AlgorithmValidationProhibited(algorithm uint8) bool {
	return slices.Contains(algorithmValidationProhibited, algorithm)
}

// AlgorithmSigningNotRecommended reports an algorithm marked NOT RECOMMENDED for signing.
func AlgorithmSigningNotRecommended(algorithm uint8) bool {
	return slices.Contains(algorithmSigningNotRecommended, algorithm)
}

// DigestSigningProhibited reports a digest type marked MUST NOT for delegation.
func DigestSigningProhibited(digest uint8) bool {
	return slices.Contains(digestSigningProhibited, digest)
}

// DigestValidationProhibited reports a digest type marked MUST NOT for validation.
func DigestValidationProhibited(digest uint8) bool {
	return slices.Contains(digestValidationProhibited, digest)
}

// DigestSupersedesSHA1 reports the digest whose presence makes SHA-1 DS records ignored (RFC 4509 s3).
func DigestSupersedesSHA1(digest uint8) bool { return digest == dns.SHA256 }

// DigestStrongerThanSHA1 reports a digest that outranks SHA-1, mandate or not.
func DigestStrongerThanSHA1(digest uint8) bool {
	return digest == dns.SHA256 || digest == dns.SHA384
}
