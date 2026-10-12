package dnssecutil

import (
	"strings"
	"sync"

	dns "codeberg.org/miekg/dns"
	"codeberg.org/miekg/dns/dnsutil"
)

// NSEC3MaxIterations is the highest iteration count hashed, below the insecure limit of RFC 9276 Appendix A.
const NSEC3MaxIterations = 50

// nsec3Name is the hash function, replaced in tests.
var nsec3Name = dnsutil.NSEC3Name

// NSEC3Hashes memoizes NSEC3 owner hashes; the zero value is ready and a nil pointer does not memoize.
type NSEC3Hashes struct {
	mu     sync.Mutex
	hashes map[nsec3Input]string
}

type nsec3Input struct {
	name       string
	salt       string
	iterations uint16
}

// Hash returns the lowercase owner hash of name (RFC 5155 section 5), or false above NSEC3MaxIterations.
func (h *NSEC3Hashes) Hash(name, salt string, iterations uint16) (string, bool) {
	if iterations > NSEC3MaxIterations {
		return "", false
	}
	in := nsec3Input{name: strings.ToLower(dnsutil.Fqdn(name)), salt: strings.ToLower(salt), iterations: iterations}
	if h == nil {
		return strings.ToLower(nsec3Name(in.name, in.salt, in.iterations)), true
	}
	h.mu.Lock()
	hash, ok := h.hashes[in]
	h.mu.Unlock()
	if ok {
		return hash, true
	}
	hash = strings.ToLower(nsec3Name(in.name, in.salt, in.iterations))
	h.mu.Lock()
	if h.hashes == nil {
		h.hashes = map[nsec3Input]string{}
	}
	h.hashes[in] = hash
	h.mu.Unlock()
	return hash, true
}

// NSEC3Usable returns the SHA-1 records (RFC 5155 section 8.1) and whether they share one parameter set (section 7.2).
func NSEC3Usable(records []*dns.NSEC3) ([]*dns.NSEC3, bool) {
	var out []*dns.NSEC3
	for _, rr := range records {
		if rr == nil || rr.Hash != dns.SHA1 {
			continue
		}
		if len(out) > 0 && (rr.Iterations != out[0].Iterations || !strings.EqualFold(rr.Salt, out[0].Salt)) {
			return out, false
		}
		out = append(out, rr)
	}
	return out, true
}
