package dnssecutil

import (
	"time"

	dns "codeberg.org/miekg/dns"
)

// Caps on RRSIG verification; a key tag is not a unique identifier (RFC 4034 Appendix B).
const (
	MaxKeysPerTag     = 4
	MaxRRSIGsPerRRset = 8
)

// CandidateKeys returns the first MaxKeysPerTag keys with sig's key tag and algorithm, in order, and how many carry both.
func CandidateKeys(sig *dns.RRSIG, keys []*dns.DNSKEY) ([]*dns.DNSKEY, int) {
	if sig == nil {
		return nil, 0
	}
	var out []*dns.DNSKEY
	matched := 0
	for _, key := range keys {
		if key == nil || key.Algorithm != sig.Algorithm || KeyTag(key) != sig.KeyTag {
			continue
		}
		matched++
		if len(out) < MaxKeysPerTag {
			out = append(out, key)
		}
	}
	return out, matched
}

// VerifyCandidates tries keys in order over one copy of rrset; it returns the key that verifies, or nil, and the errors before it.
func VerifyCandidates(sig *dns.RRSIG, rrset []dns.RR, keys []*dns.DNSKEY, at time.Time) (*dns.DNSKEY, []error) {
	errs := make([]error, 0, len(keys))
	fail := func(err error) (*dns.DNSKEY, []error) {
		for range keys {
			errs = append(errs, err)
		}
		return nil, errs
	}
	if sig == nil {
		return fail(errMissing)
	}
	if !sig.ValidPeriod(at) {
		return fail(errOutOfRange)
	}
	copies, err := cloneRRset(rrset)
	if err != nil {
		return fail(err)
	}
	for _, key := range keys {
		if err := verifyCopy(sig, copies, key); err != nil {
			errs = append(errs, err)
			continue
		}
		return key, errs
	}
	return nil, errs
}
