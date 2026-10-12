package dnssecutil_test

import (
	"slices"
	"testing"

	dns "codeberg.org/miekg/dns"

	"codeberg.org/pawal/gonemaster/engine/dnssecutil"
)

func TestNSEC3HashesBound(t *testing.T) {
	var h dnssecutil.NSEC3Hashes
	// RFC 5155 Appendix A: example, salt aabbccdd, 12 iterations.
	for _, name := range []string{"example", "EXAMPLE."} {
		if got, ok := h.Hash(name, "aabbccdd", 12); !ok || got != "0p9mhaveqvm6t7vbl5lop2u3t2rp3tom" {
			t.Errorf("Hash(%s) = %q, %v; want the RFC 5155 hash", name, got, ok)
		}
	}
	if _, ok := h.Hash("example", "", dnssecutil.NSEC3MaxIterations); !ok {
		t.Error("hash at the bound refused")
	}
	if got, ok := h.Hash("example", "", dnssecutil.NSEC3MaxIterations+1); ok || got != "" {
		t.Errorf("Hash above the bound = %q, %v; want \"\", false", got, ok)
	}
	var none *dnssecutil.NSEC3Hashes
	if got, ok := none.Hash("example", "aabbccdd", 12); !ok || got != "0p9mhaveqvm6t7vbl5lop2u3t2rp3tom" {
		t.Errorf("nil memo Hash = %q, %v", got, ok)
	}
}

func nsec3(hash uint8, salt string, iterations uint16) *dns.NSEC3 {
	rr := &dns.NSEC3{Hdr: dns.Header{Name: "0p9mhaveqvm6t7vbl5lop2u3t2rp3tom.example.", Class: dns.ClassINET}}
	rr.Hash, rr.Salt, rr.Iterations = hash, salt, iterations
	return rr
}

func TestNSEC3Usable(t *testing.T) {
	sha1a, sha1b := nsec3(dns.SHA1, "AABB", 10), nsec3(dns.SHA1, "aabb", 10)
	unknown := nsec3(2, "ff", 500)
	cases := []struct {
		name    string
		records []*dns.NSEC3
		want    []*dns.NSEC3
		ok      bool
	}{
		{"one set, salt case ignored", []*dns.NSEC3{sha1a, sha1b}, []*dns.NSEC3{sha1a, sha1b}, true},
		{"unknown hash ignored", []*dns.NSEC3{unknown, sha1a}, []*dns.NSEC3{sha1a}, true},
		{"other iterations", []*dns.NSEC3{sha1a, nsec3(dns.SHA1, "aabb", 11)}, []*dns.NSEC3{sha1a}, false},
		{"other salt", []*dns.NSEC3{sha1a, nsec3(dns.SHA1, "cc", 10)}, []*dns.NSEC3{sha1a}, false},
		{"none", nil, nil, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := dnssecutil.NSEC3Usable(c.records)
			if ok != c.ok || !slices.Equal(got, c.want) {
				t.Errorf("NSEC3Usable = %d records, %v; want %d, %v", len(got), ok, len(c.want), c.ok)
			}
		})
	}
}
