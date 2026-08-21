package dnssecchain

import (
	"testing"
	"time"

	dns "codeberg.org/miekg/dns"

	"codeberg.org/pawal/gonemaster/engine/internal/dnstest"
)

// genMLDSA44Key generates an ML-DSA-44 DNSKEY; generating rather than replaying
// a captured key means this also fails if the DNS library loses ML-DSA-44
// support.
func genMLDSA44Key(t *testing.T, owner string) dnstest.Keypair {
	t.Helper()
	return dnstest.GenKey(t, owner, dns.MLDSA44, true)
}

// The chain-view half of ML-DSA-44 validation. sigState used to short-circuit
// to SigUnsupported for algorithm 18, so the chain reported
// "unsupported_algorithm" on a good link; it now verifies like any other.
func TestSigStateMLDSA44(t *testing.T) {
	kp := genMLDSA44Key(t, testZone)
	rrset := []dns.RR{kp.Key}
	inception := fixedAt.Add(-24 * time.Hour)
	expiration := fixedAt.Add(24 * time.Hour)
	sig := dnstest.SignRRset(t, kp, rrset, testZone, testZone, inception, expiration)
	keys := []*dns.DNSKEY{kp.Key}

	if got := sigState(sig, rrset, keys, fixedAt); got != SigValid {
		t.Errorf("ML-DSA-44 sigState = %q, want %q", got, SigValid)
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

// Counterweight to the test above: adding ML-DSA-44 must not have widened the
// supported set generally. Ed448 is still a real gap in the DNS library.
func TestSigStateStillUnsupportedAlgorithms(t *testing.T) {
	kp := genMLDSA44Key(t, testZone)
	keys := []*dns.DNSKEY{kp.Key}

	// The payload is irrelevant; sigState rejects on the algorithm first.
	for _, algo := range []uint8{dns.ED448, dns.DSA, dns.RSAMD5} {
		sig := dummyRRSIG(kp.Key.KeyTag())
		sig.Algorithm = algo
		if got := sigState(sig, []dns.RR{kp.Key}, keys, fixedAt); got != SigUnsupported {
			t.Errorf("algorithm %d: sigState = %q, want %q", algo, got, SigUnsupported)
		}
	}
}
