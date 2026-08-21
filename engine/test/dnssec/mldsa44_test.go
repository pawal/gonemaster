package dnssec

import (
	"context"
	"crypto"
	"testing"
	"time"

	dns "codeberg.org/miekg/dns"
	"codeberg.org/miekg/dns/dnsutil"
	"codeberg.org/pawal/gonemaster/engine/dnsname"
	"codeberg.org/pawal/gonemaster/engine/logger"
	"codeberg.org/pawal/gonemaster/engine/nameserver"
	"codeberg.org/pawal/gonemaster/engine/packet"
	"codeberg.org/pawal/gonemaster/engine/profile"
	"codeberg.org/pawal/gonemaster/engine/test/internal/tctest"
	"codeberg.org/pawal/gonemaster/engine/util"
	"codeberg.org/pawal/gonemaster/engine/zone"
)

// mldsa44Key generates an ML-DSA-44 zone key; Generate takes the seed size,
// not a modulus size.
func mldsa44Key(t *testing.T, owner string) (*dns.DNSKEY, crypto.Signer) {
	t.Helper()
	key := &dns.DNSKEY{Hdr: dns.Header{Name: dnsutil.Fqdn(owner), Class: dns.ClassINET, TTL: 60}}
	key.Flags = dns.FlagZONE | dns.FlagSEP
	key.Protocol = 3
	key.Algorithm = dns.MLDSA44
	priv, err := key.Generate(256)
	if err != nil {
		t.Fatalf("generate ML-DSA-44 key: %v", err)
	}
	signer, ok := priv.(crypto.Signer)
	if !ok {
		t.Fatalf("ML-DSA-44 private key is not a crypto.Signer")
	}
	return key, signer
}

// mldsa44Sig produces a real RRSIG over rrset. Unlike rrsigRecord's unsigned
// stub, it must actually verify: these tests exist because verification now
// runs for algorithm 18 instead of being skipped.
func mldsa44Sig(t *testing.T, owner string, typeCovered uint16, key *dns.DNSKEY, signer crypto.Signer, rrset []dns.RR, now time.Time) *dns.RRSIG {
	t.Helper()
	sig := &dns.RRSIG{Hdr: dns.Header{Name: dnsutil.Fqdn(owner), Class: dns.ClassINET, TTL: 60}}
	sig.TypeCovered = typeCovered
	sig.Algorithm = key.Algorithm
	sig.Labels = uint8(dnsutil.Labels(dnsutil.Fqdn(owner)))
	sig.OrigTTL = 60
	sig.Inception = uint32(now.Add(-time.Hour).Unix())
	sig.Expiration = uint32(now.Add(time.Hour).Unix())
	sig.KeyTag = key.KeyTag()
	sig.SignerName = dnsutil.Fqdn(owner)
	if err := sig.Sign(signer, rrset, &dns.SignOption{}); err != nil {
		t.Fatalf("sign %s RRset with ML-DSA-44: %v", dns.TypeToString[typeCovered], err)
	}
	return sig
}

// The user-visible half of ML-DSA-44 validation. DNSSEC08 checks
// dnssecAlgorithmSupported before verifying, so a correctly signed ML-DSA-44
// zone used to draw DS08_ALGO_NOT_SUPPORTED_BY_ZM at NOTICE.
func TestDNSSEC08MLDSA44Valid(t *testing.T) {
	ctx := testCtx()
	t.Cleanup(profile.ResetEffective)

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	origM4 := glueNameservers
	origM5 := apexNameservers
	t.Cleanup(func() {
		glueNameservers = origM4
		apexNameservers = origM5
	})

	now := time.Unix(1700000000, 0).UTC()
	key, signer := mldsa44Key(t, "example")
	sig := mldsa44Sig(t, "example", dns.TypeDNSKEY, key, signer, []dns.RR{key}, now)

	ns := newNameserver(t, ctx, "ns1.example", "192.0.2.96", func(qname string, qtype string, _ *nameserver.QueryOptions) packet.Packet {
		if qtype != "DNSKEY" {
			return packet.Packet{}
		}
		pkt := answerPacket(qname, dns.TypeDNSKEY, key, sig)
		pkt.Timestamp = now
		return pkt
	})

	glueNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{ns}, nil
	}
	apexNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return nil, nil
	}

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
	ctx := testCtx()
	t.Cleanup(profile.ResetEffective)

	util.SetLogger(logger.New())
	t.Cleanup(func() { util.SetLogger(nil) })

	origM4 := glueNameservers
	origM5 := apexNameservers
	t.Cleanup(func() {
		glueNameservers = origM4
		apexNameservers = origM5
	})

	now := time.Unix(1700000000, 0).UTC()
	key, signer := mldsa44Key(t, "example")
	soa := soaRecord("example")
	sig := mldsa44Sig(t, "example", dns.TypeSOA, key, signer, []dns.RR{soa}, now)

	ns := newNameserver(t, ctx, "ns1.example", "192.0.2.97", func(qname string, qtype string, _ *nameserver.QueryOptions) packet.Packet {
		switch qtype {
		case "DNSKEY":
			pkt := dnskeyPacket(qname, key)
			pkt.Timestamp = now
			return pkt
		case "SOA":
			pkt := answerPacket(qname, dns.TypeSOA, soa, sig)
			pkt.Timestamp = now
			return pkt
		default:
			return packet.Packet{}
		}
	})

	glueNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return []nameserver.Nameserver{ns}, nil
	}
	apexNameservers = func(_ context.Context, _ *zone.Zone) ([]nameserver.Nameserver, error) {
		return nil, nil
	}

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
	// Ed448 is still a real gap, so the widening has to stop at ML-DSA-44.
	if dnssecAlgorithmSupported(dns.ED448) {
		t.Error("algorithm 16 (ED448) is still unsupported by the DNS library")
	}
}
