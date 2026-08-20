package dnssecchain

import (
	"crypto"
	"testing"
	"time"

	dns "codeberg.org/miekg/dns"
	"codeberg.org/miekg/dns/dnsutil"
)

// genMLDSA44Key generates an ML-DSA-44 DNSKEY; the bit argument is the seed
// size, not a modulus size. Generating rather than replaying a captured key
// means this also fails if the DNS library loses ML-DSA-44 support.
func genMLDSA44Key(t *testing.T, owner string) keypair {
	t.Helper()
	k := &dns.DNSKEY{Hdr: dns.Header{Name: dnsutil.Fqdn(owner), Class: dns.ClassINET, TTL: 3600}}
	k.Flags = dns.FlagZONE | dns.FlagSEP
	k.Protocol = 3
	k.Algorithm = dns.MLDSA44
	priv, err := k.Generate(256)
	if err != nil {
		t.Fatalf("generate ML-DSA-44 key for %s: %v", owner, err)
	}
	signer, ok := priv.(crypto.Signer)
	if !ok {
		t.Fatalf("ML-DSA-44 key for %s is not a crypto.Signer", owner)
	}
	return keypair{key: k, priv: signer}
}

// The chain-view half of ML-DSA-44 validation. sigState used to short-circuit
// to SigUnsupported for algorithm 18, so the chain reported
// "unsupported_algorithm" on a good link; it now verifies like any other.
func TestSigStateMLDSA44(t *testing.T) {
	kp := genMLDSA44Key(t, testZone)
	rrset := []dns.RR{kp.key}
	inception := fixedAt.Add(-24 * time.Hour)
	expiration := fixedAt.Add(24 * time.Hour)
	sig := signRRset(t, kp, rrset, testZone, testZone, inception, expiration)
	keys := []*dns.DNSKEY{kp.key}

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
	tampered, ok := kp.key.Clone().(*dns.DNSKEY)
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
	keys := []*dns.DNSKEY{kp.key}

	// The payload is irrelevant; sigState rejects on the algorithm first.
	for _, algo := range []uint8{dns.ED448, dns.DSA, dns.RSAMD5} {
		sig := dummyRRSIG(kp.key.KeyTag())
		sig.Algorithm = algo
		if got := sigState(sig, []dns.RR{kp.key}, keys, fixedAt); got != SigUnsupported {
			t.Errorf("algorithm %d: sigState = %q, want %q", algo, got, SigUnsupported)
		}
	}
}
