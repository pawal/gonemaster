package dnssecutil

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"strings"
	"testing"
	"time"

	dns "codeberg.org/miekg/dns"
	"github.com/cloudflare/circl/sign/ed448"
)

// The RFC 8080 section 6.2 examples, transcribed under testdata/. Their
// validity window is fixed in 2015, so a pinned time keeps them deterministic.
var ed448Vectors = []struct {
	zone   string
	keyTag uint16
}{
	{"ed448-rfc8080-9713.example.com.zone", 9713},
	{"ed448-rfc8080-38353.example.com.zone", 38353},
}

func ed448VectorTime() time.Time { return time.Date(2015, 8, 5, 0, 0, 0, 0, time.UTC) }

// mxRecords returns the MX records an Ed448 fixture RRSIG covers.
func mxRecords(t *testing.T, rest []dns.RR) []dns.RR {
	t.Helper()
	var mx []dns.RR
	for _, rr := range rest {
		if _, ok := rr.(*dns.MX); ok {
			mx = append(mx, rr)
		}
	}
	if len(mx) == 0 {
		t.Fatal("fixture has no MX record")
	}
	return mx
}

// sigForTag returns the fixture signature made by the given key tag.
func sigForTag(t *testing.T, sigs []*dns.RRSIG, tag uint16) *dns.RRSIG {
	t.Helper()
	for _, sig := range sigs {
		if sig.KeyTag == tag {
			return sig
		}
	}
	t.Fatalf("no signature with key tag %d in fixture", tag)
	return nil
}

// The specification's own vectors, so they pin RFC 8080 signing: PureEdDSA,
// empty context, no prehash. They are also the guard on the verification hook,
// which is what stands in for a DNS library that cannot verify algorithm 16.
func TestVerifyRRSIGEd448RFC8080Vectors(t *testing.T) {
	for _, v := range ed448Vectors {
		t.Run(v.zone, func(t *testing.T) {
			keys, sigs, rest := loadZoneFixture(t, v.zone)
			key := keyForTag(t, keys, v.keyTag)
			if key.Algorithm != dns.ED448 {
				t.Fatalf("fixture key: algorithm %d, want %d (ED448)", key.Algorithm, dns.ED448)
			}

			verified := 0
			for _, sig := range sigs {
				if sig.KeyTag != v.keyTag {
					continue
				}
				if err := VerifyRRSIG(sig, mxRecords(t, rest), key, ed448VectorTime()); err != nil {
					t.Fatalf("RFC 8080 RRSIG over MX should verify: %v", err)
				}
				verified++
			}
			if verified != 1 {
				t.Fatalf("verified %d signatures, want exactly 1", verified)
			}
		})
	}
}

// The published DS is derived from the published DNSKEY, so recomputing it
// proves the fixture keys were transcribed byte for byte.
func TestEd448FixtureDSMatches(t *testing.T) {
	for _, v := range ed448Vectors {
		t.Run(v.zone, func(t *testing.T) {
			keys, _, rest := loadZoneFixture(t, v.zone)
			key := keyForTag(t, keys, v.keyTag)

			var ds *dns.DS
			for _, rr := range rest {
				if d, ok := rr.(*dns.DS); ok {
					ds = d
				}
			}
			if ds == nil {
				t.Fatal("fixture has no DS record")
			}

			want := key.ToDS(dns.SHA256)
			if want == nil {
				t.Fatal("ToDS returned nil")
			}
			if ds.KeyTag != want.KeyTag || !strings.EqualFold(ds.Digest, want.Digest) {
				t.Errorf("fixture DS %d/%s, computed %d/%s", ds.KeyTag, ds.Digest, want.KeyTag, want.Digest)
			}
		})
	}
}

// Without these the positive tests could pass on a hook that accepted anything.
func TestVerifyRRSIGEd448Negative(t *testing.T) {
	v := ed448Vectors[0]
	keys, sigs, rest := loadZoneFixture(t, v.zone)
	key := keyForTag(t, keys, v.keyTag)
	mx := mxRecords(t, rest)
	sig := sigForTag(t, sigs, v.keyTag)

	t.Run("flipped bit in the signature", func(t *testing.T) {
		raw, err := base64.StdEncoding.DecodeString(sig.Signature)
		if err != nil {
			t.Fatalf("decode signature: %v", err)
		}
		// Staying inside the signature keeps every other field valid, so the
		// call has to reach the Ed448 verifier to fail.
		tampered := *sig
		raw[len(raw)-1] ^= 0x01
		tampered.Signature = base64.StdEncoding.EncodeToString(raw)

		if err := VerifyRRSIG(&tampered, mx, key, ed448VectorTime()); !errors.Is(err, dns.ErrSig) {
			t.Errorf("err = %v, want %v", err, dns.ErrSig)
		}
	})

	t.Run("modified covered data", func(t *testing.T) {
		altered, ok := mx[0].Clone().(*dns.MX)
		if !ok {
			t.Fatal("mx clone changed type")
		}
		altered.Preference++

		if err := VerifyRRSIG(sig, []dns.RR{altered}, key, ed448VectorTime()); !errors.Is(err, dns.ErrSig) {
			t.Errorf("err = %v, want %v", err, dns.ErrSig)
		}
	})

	t.Run("outside the validity window", func(t *testing.T) {
		before := time.Unix(int64(sig.Inception)-3600, 0)
		if err := VerifyRRSIG(sig, mx, key, before); err == nil {
			t.Error("signature before inception should not verify")
		}
		after := time.Unix(int64(sig.Expiration)+3600, 0)
		if err := VerifyRRSIG(sig, mx, key, after); err == nil {
			t.Error("expired signature should not verify")
		}
	})
}

// The two vectors sign the same RRset under different keys. Verify rejects a
// key-tag mismatch before it looks at the signature, so each pair is checked
// twice: untouched, which pins that guard, and with the tag rewritten, which is
// the only way to reach the Ed448 verifier with the wrong key.
func TestVerifyRRSIGEd448WrongKey(t *testing.T) {
	_, sigsA, restA := loadZoneFixture(t, ed448Vectors[0].zone)
	keysB, _, _ := loadZoneFixture(t, ed448Vectors[1].zone)

	keyB := keyForTag(t, keysB, ed448Vectors[1].keyTag)
	sigA := sigForTag(t, sigsA, ed448Vectors[0].keyTag)
	mx := mxRecords(t, restA)

	if err := VerifyRRSIG(sigA, mx, keyB, ed448VectorTime()); !errors.Is(err, dns.ErrKey) {
		t.Errorf("untouched: err = %v, want %v", err, dns.ErrKey)
	}

	// The key tag is part of the signed RDATA, so rewriting it changes the
	// signed message as well as the key. Either way the rejection has to come
	// from the signature check, not from the tag guard.
	crossed := *sigA
	crossed.KeyTag = KeyTag(keyB)
	if err := VerifyRRSIG(&crossed, mx, keyB, ed448VectorTime()); !errors.Is(err, dns.ErrSig) {
		t.Errorf("crossed: err = %v, want %v", err, dns.ErrSig)
	}
}

// The hook lands in the library's default branch, shared by every algorithm it
// cannot verify. Attaching it for those would report them as broken signatures
// instead of unsupported algorithms, which DNSSEC02 and DNSSEC10 classify from
// ErrAlg alone.
func TestVerifyRRSIGPreservesErrAlg(t *testing.T) {
	v := ed448Vectors[0]
	keys, sigs, rest := loadZoneFixture(t, v.zone)
	key := keyForTag(t, keys, v.keyTag)
	mx := mxRecords(t, rest)
	base := sigForTag(t, sigs, v.keyTag)

	for _, algo := range []uint8{dns.ECCGOST, dns.DSA, dns.SM2SM3} {
		t.Run(dns.AlgorithmToString[algo], func(t *testing.T) {
			foreign, ok := key.Clone().(*dns.DNSKEY)
			if !ok {
				t.Fatal("dnskey clone changed type")
			}
			foreign.Algorithm = algo

			// Every precondition Verify checks before the algorithm switch has
			// to hold, or ErrKey would mask the result.
			sig := *base
			sig.Algorithm = algo
			sig.KeyTag = KeyTag(foreign)

			if err := VerifyRRSIG(&sig, mx, foreign, ed448VectorTime()); !errors.Is(err, dns.ErrAlg) {
				t.Errorf("err = %v, want %v", err, dns.ErrAlg)
			}
		})
	}
}

func TestEd448VerifyRejectsMalformed(t *testing.T) {
	pub, priv, err := ed448.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate ed448 key: %v", err)
	}
	message := []byte("canonical signed data")
	signature := ed448.Sign(priv, message, "")
	encoded := base64.StdEncoding.EncodeToString(pub)

	key := func(algo uint8, pubkey string) *dns.DNSKEY {
		return &dns.DNSKEY{Algorithm: algo, PublicKey: pubkey}
	}

	// Positive control: without it every case below could pass on a hook that
	// rejected everything.
	if !ed448Verify(key(dns.ED448, encoded), message, signature) {
		t.Fatal("a valid Ed448 signature must verify")
	}

	tampered := append([]byte(nil), signature...)
	tampered[0] ^= 0x01

	cases := []struct {
		name string
		key  *dns.DNSKEY
		sig  []byte
	}{
		{"nil key", nil, signature},
		{"wrong algorithm", key(dns.ED25519, encoded), signature},
		{"public key too short", key(dns.ED448, base64.StdEncoding.EncodeToString(pub[:len(pub)-1])), signature},
		{"public key not base64", key(dns.ED448, "!!!"), signature},
		{"signature too short", key(dns.ED448, encoded), signature[:len(signature)-1]},
		{"tampered signature", key(dns.ED448, encoded), tampered},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if ed448Verify(tc.key, message, tc.sig) {
				t.Error("verification should have failed")
			}
		})
	}
}
