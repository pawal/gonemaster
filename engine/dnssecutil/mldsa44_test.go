package dnssecutil

import (
	"encoding/base64"
	"os"
	"strings"
	"testing"
	"time"

	dns "codeberg.org/miekg/dns"
)

// Fixtures under testdata/ are zone-file captures: an ML-DSA-44 key is 1312
// bytes and a signature 2420, too large to inline. Every test pins the
// verification time inside the captured window, so the live captures cannot
// start failing on their own when their RRSIGs expire.
const (
	// draft-westerbaan-dnssec-mldsa-04 section 6; its window is fixed in 2015.
	draftVectorZone = "mldsa44-example.com.zone"
	draftVectorTag  = 59829

	// Live captures, retrieved 2026-08-19.
	kochenSpeckerZone = "mldsa44-kochen-specker.info.zone"
	huqueZone         = "mldsa44-mldsa.huque.com.zone"
)

func draftVectorTime() time.Time { return time.Date(2015, 8, 5, 0, 0, 0, 0, time.UTC) }
func liveCaptureTime() time.Time { return time.Date(2026, 8, 20, 0, 0, 0, 0, time.UTC) }

// loadZoneFixture parses a testdata zone file, splitting DNSKEYs and RRSIGs
// from the rest of the capture.
func loadZoneFixture(t *testing.T, name string) (keys []*dns.DNSKEY, sigs []*dns.RRSIG, rest []dns.RR) {
	t.Helper()
	raw, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	zp := dns.NewZoneParser(strings.NewReader(string(raw)), ".", "")
	for rr, ok := zp.Next(); ok; rr, ok = zp.Next() {
		switch v := rr.(type) {
		case *dns.DNSKEY:
			keys = append(keys, v)
		case *dns.RRSIG:
			sigs = append(sigs, v)
		default:
			rest = append(rest, rr)
		}
	}
	if err := zp.Err(); err != nil {
		t.Fatalf("parse fixture %s: %v", name, err)
	}
	if len(keys) == 0 || len(sigs) == 0 {
		t.Fatalf("fixture %s: got %d keys and %d signatures, want at least one of each", name, len(keys), len(sigs))
	}
	return keys, sigs, rest
}

// keyForTag returns the fixture key with the given key tag.
func keyForTag(t *testing.T, keys []*dns.DNSKEY, tag uint16) *dns.DNSKEY {
	t.Helper()
	for _, k := range keys {
		if KeyTag(k) == tag {
			return k
		}
	}
	t.Fatalf("no key with tag %d in fixture", tag)
	return nil
}

// asRRset widens keys for VerifyRRSIG. A DNSKEY RRSIG covers the whole key
// set, so every key goes back, not just the signing one.
func asRRset(keys []*dns.DNSKEY) []dns.RR {
	out := make([]dns.RR, 0, len(keys))
	for _, k := range keys {
		out = append(out, k)
	}
	return out
}

// The specification's own vector, so it pins the draft's definition of
// ML-DSA-44 signing: pure ML-DSA, empty context, no prehash. It is also the
// guard against a DNS library without ML-DSA-44 verification, which would turn
// the NOTICE DNSSEC08/09 used to emit into a spurious ERROR.
func TestVerifyRRSIGMLDSA44DraftVector(t *testing.T) {
	keys, sigs, rest := loadZoneFixture(t, draftVectorZone)

	var mx []dns.RR
	for _, rr := range rest {
		if _, ok := rr.(*dns.MX); ok {
			mx = append(mx, rr)
		}
	}
	if len(mx) == 0 {
		t.Fatal("draft vector has no MX record to verify")
	}

	key := keyForTag(t, keys, draftVectorTag)
	if key.Algorithm != dns.MLDSA44 {
		t.Fatalf("draft vector key: algorithm %d, want %d (MLDSA44)", key.Algorithm, dns.MLDSA44)
	}

	verified := 0
	for _, sig := range sigs {
		if sig.Algorithm != dns.MLDSA44 || sig.KeyTag != draftVectorTag {
			continue
		}
		if err := VerifyRRSIG(sig, mx, key, draftVectorTime()); err != nil {
			t.Fatalf("draft section 6 RRSIG over MX should verify: %v", err)
		}
		verified++
	}
	if verified != 1 {
		t.Fatalf("verified %d draft signatures, want exactly 1", verified)
	}
}

// The two public ML-DSA-44 zones, covering both deployment shapes:
// kochen-specker.info mixes RSASHA256 and ML-DSA-44 in one RRset,
// mldsa.huque.com is ML-DSA-44 only. Two independent signers, so agreement is
// evidence about the algorithm rather than about one signer's quirks.
func TestVerifyRRSIGMLDSA44LiveZones(t *testing.T) {
	cases := []struct {
		name     string
		fixture  string
		wantSigs int // ML-DSA-44 RRSIGs expected over the DNSKEY RRset
	}{
		{"kochen-specker.info (dual algorithm, RSASHA256 + ML-DSA-44)", kochenSpeckerZone, 2},
		{"mldsa.huque.com (ML-DSA-44 only)", huqueZone, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			keys, sigs, _ := loadZoneFixture(t, tc.fixture)
			rrset := asRRset(keys)

			verified := 0
			for _, sig := range sigs {
				if sig.Algorithm != dns.MLDSA44 {
					continue
				}
				key := keyForTag(t, keys, sig.KeyTag)
				if err := VerifyRRSIG(sig, rrset, key, liveCaptureTime()); err != nil {
					t.Errorf("RRSIG with key tag %d should verify: %v", sig.KeyTag, err)
					continue
				}
				verified++
			}
			if verified != tc.wantSigs {
				t.Errorf("verified %d ML-DSA-44 signatures, want %d", verified, tc.wantSigs)
			}
		})
	}
}

// mldsa44SigFrom returns the first ML-DSA-44 RRSIG with its key and RRset.
func mldsa44SigFrom(t *testing.T, fixture string) (*dns.RRSIG, *dns.DNSKEY, []dns.RR) {
	t.Helper()
	keys, sigs, _ := loadZoneFixture(t, fixture)
	for _, sig := range sigs {
		if sig.Algorithm == dns.MLDSA44 {
			return sig, keyForTag(t, keys, sig.KeyTag), asRRset(keys)
		}
	}
	t.Fatalf("fixture %s has no ML-DSA-44 signature", fixture)
	return nil, nil, nil
}

// Proves the positive tests exercise the signature rather than short-circuiting.
// Each case breaks exactly one thing about a signature known to verify.
func TestVerifyRRSIGMLDSA44Negative(t *testing.T) {
	sig, key, rrset := mldsa44SigFrom(t, huqueZone)

	// Without this, a bug that broke verification outright would make every
	// case below "pass".
	if err := VerifyRRSIG(sig, rrset, key, liveCaptureTime()); err != nil {
		t.Fatalf("baseline signature should verify: %v", err)
	}

	t.Run("flipped bit in the signature", func(t *testing.T) {
		raw, err := base64.StdEncoding.DecodeString(sig.Signature)
		if err != nil {
			t.Fatalf("decode signature: %v", err)
		}
		// Staying inside the signature keeps every other field valid, so the
		// call has to reach mldsa.Verify to fail.
		tampered := *sig
		raw[len(raw)-1] ^= 0x01
		tampered.Signature = base64.StdEncoding.EncodeToString(raw)

		if err := VerifyRRSIG(&tampered, rrset, key, liveCaptureTime()); err == nil {
			t.Error("a signature with one flipped bit should not verify")
		}
	})

	t.Run("modified covered data", func(t *testing.T) {
		// The signature covers DNSKEY RDATA, so a flipped flag bit changes the
		// signed message while leaving the signature intact.
		altered := make([]dns.RR, len(rrset))
		copy(altered, rrset)
		clone, ok := key.Clone().(*dns.DNSKEY)
		if !ok {
			t.Fatal("dnskey clone changed type")
		}
		clone.Flags ^= 0x0001
		for i, rr := range altered {
			if rr == dns.RR(key) {
				altered[i] = clone
			}
		}

		if err := VerifyRRSIG(sig, altered, key, liveCaptureTime()); err == nil {
			t.Error("a modified RRset should not verify")
		}
	})

	t.Run("outside the validity window", func(t *testing.T) {
		beforeInception := time.Unix(int64(sig.Inception)-3600, 0)
		if err := VerifyRRSIG(sig, rrset, key, beforeInception); err == nil {
			t.Error("signature before inception should not verify")
		}
		afterExpiration := time.Unix(int64(sig.Expiration)+3600, 0)
		if err := VerifyRRSIG(sig, rrset, key, afterExpiration); err == nil {
			t.Error("expired signature should not verify")
		}
	})
}

// kochen-specker.info publishes two ML-DSA-44 keys, so this cross-checks real
// keys. The key tag is part of the signed RDATA, so rejection comes from either
// the tag mismatch or the signature; the point is that neither is accepted.
func TestVerifyRRSIGMLDSA44WrongKey(t *testing.T) {
	keys, sigs, _ := loadZoneFixture(t, kochenSpeckerZone)
	rrset := asRRset(keys)

	var mldsaKeys []*dns.DNSKEY
	for _, k := range keys {
		if k.Algorithm == dns.MLDSA44 {
			mldsaKeys = append(mldsaKeys, k)
		}
	}
	if len(mldsaKeys) < 2 {
		t.Fatalf("fixture has %d ML-DSA-44 keys, want at least 2", len(mldsaKeys))
	}

	checked := 0
	for _, sig := range sigs {
		if sig.Algorithm != dns.MLDSA44 {
			continue
		}
		for _, k := range mldsaKeys {
			if KeyTag(k) == sig.KeyTag {
				continue
			}
			if err := VerifyRRSIG(sig, rrset, k, liveCaptureTime()); err == nil {
				t.Errorf("signature with key tag %d should not verify under key %d", sig.KeyTag, KeyTag(k))
			}
			checked++
		}
	}
	if checked == 0 {
		t.Fatal("no mismatched key pairs were checked")
	}
}

// KeySize reads an RFC 3110 exponent-length prefix, which an ML-DSA-44 key
// does not have, so its result describes nothing. Harmless only because
// rsaKeySizeByAlgo keeps algorithm 18 off that path; the engine-side test
// asserts that gate, this one pins why it matters.
func TestKeySizeMLDSA44(t *testing.T) {
	keys, _, _ := loadZoneFixture(t, huqueZone)
	key := keys[0]
	if key.Algorithm != dns.MLDSA44 {
		t.Fatalf("fixture key: algorithm %d, want %d (MLDSA44)", key.Algorithm, dns.MLDSA44)
	}

	// A result in RSA-modulus range would mean KeySize had produced a
	// plausible, and therefore dangerous, number.
	if bits := KeySize(key); bits >= 512 && bits <= 4096 {
		t.Errorf("KeySize on an ML-DSA-44 key returned %d bits, which looks like a valid RSA modulus size", bits)
	}
}
