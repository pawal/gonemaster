package dnssecchain

import (
	"testing"
	"time"

	dns "codeberg.org/miekg/dns"

	"codeberg.org/pawal/gonemaster/engine/internal/dnstest"
)

// The chain-view half of Ed448 validation. sigState short-circuited to
// SigUnsupported for algorithm 16, so a good link reported
// unsupported_algorithm; it now verifies like any other. Generating the key
// rather than replaying one also exercises the test helper's Ed448 branch.
func TestSigStateEd448(t *testing.T) {
	kp := dnstest.GenKey(t, testZone, dns.ED448, true)
	rrset := []dns.RR{kp.Key}
	inception := fixedAt.Add(-24 * time.Hour)
	expiration := fixedAt.Add(24 * time.Hour)
	sig := dnstest.SignRRset(t, kp, rrset, testZone, testZone, inception, expiration)
	keys := []*dns.DNSKEY{kp.Key}

	if got := sigState(sig, rrset, keys, fixedAt); got != SigValid {
		t.Errorf("Ed448 sigState = %q, want %q", got, SigValid)
	}

	// Window checks run before the algorithm check, so they must still decide
	// the outcome outside the validity period.
	if got := sigState(sig, rrset, keys, inception.Add(-time.Hour)); got != SigNotYetValid {
		t.Errorf("before inception: sigState = %q, want %q", got, SigNotYetValid)
	}
	if got := sigState(sig, rrset, keys, expiration.Add(time.Hour)); got != SigExpired {
		t.Errorf("after expiration: sigState = %q, want %q", got, SigExpired)
	}

	// Widening the supported set is only correct if it does not soften real
	// failures: a signature that does not verify must come back bogus.
	tampered, ok := kp.Key.Clone().(*dns.DNSKEY)
	if !ok {
		t.Fatal("dnskey clone changed type")
	}
	tampered.Flags ^= 0x0001
	if got := sigState(sig, []dns.RR{tampered}, keys, fixedAt); got != SigBogus {
		t.Errorf("modified RRset: sigState = %q, want %q", got, SigBogus)
	}
}
