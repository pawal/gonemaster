package dnssec

import (
	"context"
	"crypto"
	"testing"
	"time"

	dns "codeberg.org/miekg/dns"
	"codeberg.org/pawal/gonemaster/engine/dnsname"
	"codeberg.org/pawal/gonemaster/engine/nameserver"
	"codeberg.org/pawal/gonemaster/engine/packet"
	"codeberg.org/pawal/gonemaster/engine/test/internal/tctest"
	"codeberg.org/pawal/gonemaster/engine/zone"
)

func ed448Key(t *testing.T, owner string) (*dns.DNSKEY, crypto.Signer) {
	t.Helper()
	return tctest.SignedKey(t, owner, dns.ED448, tctest.SEP())
}

// ed448Sig produces a real RRSIG over rrset. Unlike rrsigRecord's unsigned
// stub, it must actually verify: these tests exist because verification now
// runs for algorithm 16 instead of being skipped.
func ed448Sig(t *testing.T, owner string, typeCovered uint16, key *dns.DNSKEY, signer crypto.Signer, rrset []dns.RR, now time.Time) *dns.RRSIG {
	t.Helper()
	return tctest.Sign(t, key, signer, typeCovered, rrset,
		tctest.Signer(owner), tctest.Inception(now.Add(-time.Hour)),
		tctest.Expiration(now.Add(time.Hour)))
}

// The user-visible half of Ed448 validation. DNSSEC08 checks
// dnssecAlgorithmSupported before verifying, so a correctly signed Ed448 zone
// used to draw DS08_ALGO_NOT_SUPPORTED_BY_ZM at NOTICE.
func TestDNSSEC08Ed448Valid(t *testing.T) {
	ctx := tctest.Context(t)

	now := time.Unix(1700000000, 0).UTC()
	key, signer := ed448Key(t, "example")
	sig := ed448Sig(t, "example", dns.TypeDNSKEY, key, signer, []dns.RR{key}, now)

	ns := tctest.NS(t, ctx, "ns1.example", "192.0.2.98", func(q tctest.Query) packet.Packet {
		if q.Type != "DNSKEY" {
			return packet.Packet{}
		}
		pkt := answerPacket(q.Name, dns.TypeDNSKEY, key, sig)
		pkt.Timestamp = now
		return pkt
	})

	tctest.Stub(t, &glueNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{ns}, nil
	})
	tctest.Stub(t, &apexNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return nil, nil
	})

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := DNSSEC08(ctx, &z)
	if err != nil {
		t.Fatalf("dnssec08: %v", err)
	}
	tctest.RequireTags(t, entries, "DS08_DNSKEY_RRSIG_VALID")
	for _, tag := range []string{"DS08_ALGO_NOT_SUPPORTED_BY_ZM", "DS08_RRSIG_NOT_VALID_BY_DNSKEY", "DS08_NO_MATCHING_DNSKEY"} {
		if tctest.Has(entries, tag) {
			t.Errorf("did not expect %s for a valid Ed448 signature", tag)
		}
	}
}

// The same change on the SOA RRSIG path.
func TestDNSSEC09Ed448Valid(t *testing.T) {
	ctx := tctest.Context(t)

	now := time.Unix(1700000000, 0).UTC()
	key, signer := ed448Key(t, "example")
	soa := soaRecord("example")
	sig := ed448Sig(t, "example", dns.TypeSOA, key, signer, []dns.RR{soa}, now)

	ns := tctest.NS(t, ctx, "ns1.example", "192.0.2.99", func(q tctest.Query) packet.Packet {
		switch q.Type {
		case "DNSKEY":
			pkt := dnskeyPacket(q.Name, key)
			pkt.Timestamp = now
			return pkt
		case "SOA":
			pkt := answerPacket(q.Name, dns.TypeSOA, soa, sig)
			pkt.Timestamp = now
			return pkt
		default:
			return packet.Packet{}
		}
	})

	tctest.Stub(t, &glueNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{ns}, nil
	})
	tctest.Stub(t, &apexNameservers, func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return nil, nil
	})

	z := zone.Zone{Name: dnsname.New("example")}
	entries, err := DNSSEC09(ctx, &z)
	if err != nil {
		t.Fatalf("dnssec09: %v", err)
	}
	tctest.RequireTags(t, entries, "DS09_SOA_RRSIG_VALID")
	for _, tag := range []string{"DS09_ALGO_NOT_SUPPORTED_BY_ZM", "DS09_MISSING_RRSIG_IN_RESPONSE", "DS09_RRSIG_NOT_VALID_BY_DNSKEY", "DS09_NO_MATCHING_DNSKEY"} {
		if tctest.Has(entries, tag) {
			t.Errorf("did not expect %s for a valid Ed448 signature", tag)
		}
	}
}

// Pins the table DNSSEC08, DNSSEC09 and the chain extractor consult.
func TestEd448Supported(t *testing.T) {
	if !dnssecAlgorithmSupported(dns.ED448) {
		t.Error("algorithm 16 (ED448) should be verifiable")
	}
	// The widening stops at the algorithms with a local verifier.
	for _, algo := range []uint8{dns.ECCGOST, dns.SM2SM3, dns.DSA} {
		if dnssecAlgorithmSupported(algo) {
			t.Errorf("algorithm %d has no local verifier and must stay unsupported", algo)
		}
	}
}
