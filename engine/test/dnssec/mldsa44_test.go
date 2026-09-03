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

func mldsa44Key(t *testing.T, owner string) (*dns.DNSKEY, crypto.Signer) {
	t.Helper()
	return tctest.SignedKey(t, owner, dns.MLDSA44, tctest.SEP())
}

// mldsa44Sig produces a real RRSIG over rrset. Unlike rrsigRecord's unsigned
// stub, it must actually verify: these tests exist because verification now
// runs for algorithm 18 instead of being skipped.
func mldsa44Sig(t *testing.T, owner string, typeCovered uint16, key *dns.DNSKEY, signer crypto.Signer, rrset []dns.RR, now time.Time) *dns.RRSIG {
	t.Helper()
	return tctest.Sign(t, key, signer, typeCovered, rrset,
		tctest.Signer(owner), tctest.Inception(now.Add(-time.Hour)),
		tctest.Expiration(now.Add(time.Hour)))
}

// The user-visible half of ML-DSA-44 validation. DNSSEC08 checks
// dnssecAlgorithmSupported before verifying, so a correctly signed ML-DSA-44
// zone used to draw DS08_ALGO_NOT_SUPPORTED_BY_ZM at NOTICE.
func TestDNSSEC08MLDSA44Valid(t *testing.T) {
	ctx := tctest.Context(t)

	now := time.Unix(1700000000, 0).UTC()
	key, signer := mldsa44Key(t, "example")
	sig := mldsa44Sig(t, "example", dns.TypeDNSKEY, key, signer, []dns.RR{key}, now)

	ns := tctest.NS(t, ctx, "ns1.example", "192.0.2.96", func(q tctest.Query) packet.Packet {
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
			t.Errorf("did not expect %s for a valid ML-DSA-44 signature", tag)
		}
	}
}

// The same change on the SOA RRSIG path.
func TestDNSSEC09MLDSA44Valid(t *testing.T) {
	ctx := tctest.Context(t)

	now := time.Unix(1700000000, 0).UTC()
	key, signer := mldsa44Key(t, "example")
	soa := soaRecord("example")
	sig := mldsa44Sig(t, "example", dns.TypeSOA, key, signer, []dns.RR{soa}, now)

	ns := tctest.NS(t, ctx, "ns1.example", "192.0.2.97", func(q tctest.Query) packet.Packet {
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
	for _, tag := range []string{"DS09_ALGO_NOT_SUPPORTED_BY_ZM", "DS09_RRSIG_NOT_VALID_BY_DNSKEY", "DS09_NO_MATCHING_DNSKEY"} {
		if tctest.Has(entries, tag) {
			t.Errorf("did not expect %s for a valid ML-DSA-44 signature", tag)
		}
	}
}

// Guards what keeps ML-DSA-44 away from the RSA key-size check. KeySize reads
// an RFC 3110 exponent prefix that an ML-DSA-44 key does not have, so adding 18
// to rsaKeySizeByAlgo would give a confidently wrong key-size verdict.
func TestMLDSA44NotRSAKeySized(t *testing.T) {
	if _, ok := rsaKeySizeByAlgo[dns.MLDSA44]; ok {
		t.Error("algorithm 18 (ML-DSA-44) must not be in rsaKeySizeByAlgo: it has no RSA modulus to size")
	}
	for _, algo := range []uint8{5, 7, 8, 10} {
		if _, ok := rsaKeySizeByAlgo[algo]; !ok {
			t.Errorf("RSA algorithm %d disappeared from rsaKeySizeByAlgo", algo)
		}
	}
}

// Pins the table DNSSEC08, DNSSEC09 and the chain extractor consult. It is the
// seam where a DNS library without ML-DSA-44 verification would turn a NOTICE
// into a spurious ERROR.
func TestMLDSA44Supported(t *testing.T) {
	if !dnssecAlgorithmSupported(dns.MLDSA44) {
		t.Error("algorithm 18 (ML-DSA-44) should be verifiable")
	}
	// ECC-GOST has no local verifier, so the widening has to stop short of it.
	if dnssecAlgorithmSupported(dns.ECCGOST) {
		t.Error("algorithm 12 (ECC-GOST) is still unsupported")
	}
}
