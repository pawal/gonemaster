package dnssecutil

import (
	"crypto"
	"errors"
	"net/netip"
	"testing"
	"time"

	dns "codeberg.org/miekg/dns"
	"codeberg.org/miekg/dns/dnsutil"
)

// rsaKey generates a normal RSASHA256 key so both verifiers can run on it.
func rsaKey(t *testing.T, owner string) (*dns.DNSKEY, crypto.Signer) {
	t.Helper()
	key := &dns.DNSKEY{Hdr: dns.Header{Name: dnsutil.Fqdn(owner), Class: dns.ClassINET, TTL: 3600}}
	key.Flags = dns.FlagZONE
	key.Protocol = 3
	key.Algorithm = dns.RSASHA256
	priv, err := key.Generate(1024)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	return key, priv.(crypto.Signer)
}

func aRR(owner string, ttl uint32, addr string) *dns.A {
	a := &dns.A{Hdr: dns.Header{Name: dnsutil.Fqdn(owner), Class: dns.ClassINET, TTL: ttl}}
	a.Addr = netip.MustParseAddr(addr)
	return a
}

func cloneRRs(rrset []dns.RR) []dns.RR {
	out := make([]dns.RR, len(rrset))
	for i, rr := range rrset {
		out[i] = rr.Clone()
	}
	return out
}

// sign builds an RRSIG over rrset with the library, valid around now. It signs
// copies, so the caller's records keep their wire shape.
func sign(t *testing.T, key *dns.DNSKEY, signer crypto.Signer, rrset []dns.RR) *dns.RRSIG {
	t.Helper()
	now := time.Now().UTC()
	sig := &dns.RRSIG{Hdr: dns.Header{Name: rrset[0].Header().Name, Class: dns.ClassINET, TTL: 3600}}
	sig.Algorithm = key.Algorithm
	sig.Inception = uint32(now.Add(-time.Hour).Unix())
	sig.Expiration = uint32(now.Add(time.Hour).Unix())
	sig.KeyTag = KeyTag(key)
	sig.SignerName = key.Hdr.Name
	if err := sig.Sign(signer, cloneRRs(rrset), &dns.SignOption{}); err != nil {
		t.Fatalf("sign: %v", err)
	}
	return sig
}

// verifyLocal runs the large-exponent path on copies, as VerifyRRSIG does.
func verifyLocal(sig *dns.RRSIG, rrset []dns.RR, key *dns.DNSKEY) error {
	local := *sig
	return verifyLargeExponentRSA(&local, cloneRRs(rrset), key.Clone().(*dns.DNSKEY))
}

// The local path must rebuild the signed data exactly as the library does, so
// its signatures under a normal exponent have to pass here as well.
func TestLargeExponentPathMatchesLibrary(t *testing.T) {
	key, signer := rsaKey(t, "example.test")

	single := []dns.RR{aRR("example.test", 300, "192.0.2.1")}

	// Two records with wire TTLs unlike the original TTL, mixed-case owners,
	// in the order the canonical sort reverses.
	mixed := []dns.RR{aRR("B.Example.Test", 900, "192.0.2.2"), aRR("a.Example.Test", 300, "192.0.2.1")}

	soa := &dns.SOA{Hdr: dns.Header{Name: "example.test.", Class: dns.ClassINET, TTL: 3600}}
	soa.Ns, soa.Mbox = "NS1.Example.Test.", "Hostmaster.Example.Test."
	soa.Serial, soa.Refresh, soa.Retry, soa.Expire, soa.Minttl = 1, 3600, 600, 86400, 60

	// Signed at the wildcard, presented as the expanded name: Labels stays 2.
	wildcardSig := sign(t, key, signer, []dns.RR{aRR("*.example.test", 300, "192.0.2.3")})
	expanded := []dns.RR{aRR("www.example.test", 300, "192.0.2.3")}

	for _, tc := range []struct {
		name  string
		rrset []dns.RR
		sig   *dns.RRSIG
	}{
		{"single record", single, sign(t, key, signer, single)},
		{"mixed case owners, wire TTLs, unsorted", mixed, sign(t, key, signer, mixed)},
		{"SOA with mixed case rdata names", []dns.RR{soa}, sign(t, key, signer, []dns.RR{soa})},
		{"wildcard expansion", expanded, wildcardSig},
	} {
		t.Run(tc.name, func(t *testing.T) {
			lib := *tc.sig
			if err := lib.Verify(key.Clone().(*dns.DNSKEY), cloneRRs(tc.rrset), &dns.SignOption{}); err != nil {
				t.Fatalf("library Verify: %v", err)
			}
			if err := verifyLocal(tc.sig, tc.rrset, key); err != nil {
				t.Errorf("local path: %v", err)
			}
		})
	}
}

// Mismatching records or RRSIG fields fail as a bad signature, a foreign key as
// a bad key; neither is softened to the unsupported-exponent verdict.
func TestLargeExponentPathRejects(t *testing.T) {
	key, signer := rsaKey(t, "example.test")
	rrset := []dns.RR{aRR("example.test", 300, "192.0.2.1")}
	sig := sign(t, key, signer, rrset)
	if err := verifyLocal(sig, rrset, key); err != nil {
		t.Fatalf("unmodified signature: %v", err)
	}

	for _, tc := range []struct {
		name   string
		mutate func(sig *dns.RRSIG, rrset []dns.RR, key *dns.DNSKEY)
		want   error
	}{
		{"changed record", func(_ *dns.RRSIG, rrset []dns.RR, _ *dns.DNSKEY) {
			rrset[0].(*dns.A).Addr = netip.MustParseAddr("192.0.2.9")
		}, dns.ErrSig},
		{"labels above the owner", func(s *dns.RRSIG, _ []dns.RR, _ *dns.DNSKEY) { s.Labels++ }, dns.ErrSig},
		{"type covered mismatch", func(s *dns.RRSIG, _ []dns.RR, _ *dns.DNSKEY) { s.TypeCovered = dns.TypeAAAA }, dns.ErrSig},
		{"original TTL changed", func(s *dns.RRSIG, _ []dns.RR, _ *dns.DNSKEY) { s.OrigTTL++ }, dns.ErrSig},
		{"truncated signature", func(s *dns.RRSIG, _ []dns.RR, _ *dns.DNSKEY) {
			s.Signature = s.Signature[:len(s.Signature)-8]
		}, dns.ErrSig},
		{"empty signature", func(s *dns.RRSIG, _ []dns.RR, _ *dns.DNSKEY) { s.Signature = "" }, dns.ErrSig},
		{"keytag mismatch", func(s *dns.RRSIG, _ []dns.RR, _ *dns.DNSKEY) { s.KeyTag++ }, dns.ErrKey},
		{"signer name mismatch", func(s *dns.RRSIG, _ []dns.RR, _ *dns.DNSKEY) { s.SignerName = "other.test." }, dns.ErrKey},
		{"key without the ZONE flag", func(_ *dns.RRSIG, _ []dns.RR, k *dns.DNSKEY) { k.Flags &^= dns.FlagZONE }, dns.ErrKey},
		{"key protocol not 3", func(_ *dns.RRSIG, _ []dns.RR, k *dns.DNSKEY) { k.Protocol = 2 }, dns.ErrKey},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := *sig
			set := cloneRRs(rrset)
			k := key.Clone().(*dns.DNSKEY)
			tc.mutate(&s, set, k)
			if err := verifyLocal(&s, set, k); !errors.Is(err, tc.want) {
				t.Errorf("err = %v, want %v", err, tc.want)
			}
		})
	}
}

// wildcardOwner trims whole labels, down to the root.
func TestWildcardOwner(t *testing.T) {
	for _, tc := range []struct {
		name string
		skip int
		want string
	}{
		{"a.b.example.", 1, "*.b.example."},
		{"a.b.example.", 2, "*.example."},
		{"a.b.example.", 3, "*."},
	} {
		if got := wildcardOwner(tc.name, tc.skip); got != tc.want {
			t.Errorf("wildcardOwner(%q, %d) = %q, want %q", tc.name, tc.skip, got, tc.want)
		}
	}
}
