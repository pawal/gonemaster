package dnssecutil_test

import (
	"errors"
	"math/big"
	"net/netip"
	"strings"
	"testing"
	"time"

	dns "codeberg.org/miekg/dns"

	"codeberg.org/pawal/gonemaster/engine/dnssecutil"
	"codeberg.org/pawal/gonemaster/engine/internal/dnstest"
)

// The live .lv DNSKEY RRset and its RRSIGs, captured 2026-09-04 with the wire
// TTL 204 the resolver returned (original TTL 1800). The KSK 42018 exponent is
// 2^32+1, which the library refuses; the ZSK 31113 exponent is 65537.
const lvDNSKEYAnswer = `
lv.	204	IN	DNSKEY	256 3 8 AwEAAcDygkF9GrWafQuFEgldV8cU2zKs//pQvWGt6Mr12kw+IH3ILlzG QodUem6BhD0/zEsbfy/KiHiQ7QbcXBn7etz70bmHzb/ovH3//uNfjFfU gBdcFbtTRoYA1Hk67ibAI/1phRsOkdycOnk85monJ8fakMVG+7IMoeAc p2S+XBKMfK7aaMTCjG8nRmVyw4m2PnHiiGrwPfEJQBy80SuSbECn5CMM YhmpLn9EzvW33itnM3lCy2rBZj2spPiCzlZEnTeLNgp/Tao45kselo6z N+WhmDLzi2Om86wnO5YYCtadCKVZAWNAc4gMC5OKKPIV8WMuIrcyMp5N I89nioHf+zM=
lv.	204	IN	DNSKEY	257 3 8 BQEAAAAByLU9dUcHHcl1eLgjLidTJKlwxsU9a580xierZ+WyfRBI47L3 LLXAZZ0ub6Sea3qKP2mhP5ZBG/reXvyh3OSlHa39WoMiUUZFcuouCajB g7XeLGVPL4U1Ja1UW9wq/Oc8WU1dq4e+2Q8Dt8tipFvbL0AD0BhJAsfQ uT3wperedwQAUKId0/JQOFNTWhEJaYN2P5IIhyRKWQp8OhtKmdNYQ5jf qqpXVO4zyqV+4ZxWurXJS8c7bKrE3OAewWEGAtTjeElfQ2CFAKWVjMOL eZ86+mgw7p3UHhGB+KuRaKg6fAtTcQYBF78Xe40wuj9EgGL19mp9v6tD wFe+Epow4SFSPQ==
lv.	204	IN	RRSIG	DNSKEY 8 1 1800 20260908071728 20260829071725 42018 lv. sBxSNx9Q5BTrhuP0Rzt53qUtCX39QK2O1qDWscxNeaUOF0fZ9rHH8N8D mj73Olmk0hU2lXlB55iVIv2gfUBfVhHhmijI28282GKjrPEKFbxXhWMy IkJSpQ3sWIk7ROW5OlRsEx3G/EnXXCakaoK+SdLHOdjYcGqw09KvLGR0 qhab0N0/NdZ1sXieALGVncXo+GbOzTADJw4vFZg7nlwfeRkj/RUJF+yZ tEh/9JNCxn+JOBlu2Nsj8R7vr8AXvIO9W/kYAKAyTFTeP7Nm9qlYKSS7 WPmLBWvaENY+0kwNA4uTGRW0vOWNDJCBdq3PbCRKJZR9UQVpZnA46AHJ DVdsHg==
lv.	204	IN	RRSIG	DNSKEY 8 1 1800 20260908071728 20260829071725 31113 lv. umUFvuAxz5GPcjYy7MzDRoBl2LWNBFFHse3S3LIuPOCSjyESCXdtAlEm Of39WHc+1Dk/2N3uUUJJl6brbaKUGzTggn2i0UrCaR9sRLM4+D53Ki8q IdWeIDdkzsrtkS1ziPGTLRSa46+3PfVHGHdhnNA+FB7Dc7XWupaXiZX6 d45P2gB13PsQ1uF6G2n4JVQQPQLCD1F7QuoOqLYkAbtbIAlVL0aaLgfw 1oZb7j1jU0buAVsurBCOfPM+/8ZhaNnbQ3Gs8Ojg71BFDh5wJXNBrery 6a2fp8colk3px2IyM9UQ7xmoTRr7lLS9UypQvWGhkAkEL55+m8879SfH 01XSZQ==
`

// lvAt lies inside the validity window of the captured RRSIGs.
var lvAt = time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)

// lvFixture parses the captured answer into the DNSKEY RRset plus the keys and
// RRSIGs by keytag. Every call parses afresh, so callers own their records.
func lvFixture(t *testing.T) (rrset []dns.RR, keys map[uint16]*dns.DNSKEY, sigs map[uint16]*dns.RRSIG) {
	t.Helper()
	keys = map[uint16]*dns.DNSKEY{}
	sigs = map[uint16]*dns.RRSIG{}
	zp := dns.NewZoneParser(strings.NewReader(lvDNSKEYAnswer), "", "lv")
	for rr, ok := zp.Next(); ok; rr, ok = zp.Next() {
		switch r := rr.(type) {
		case *dns.DNSKEY:
			rrset = append(rrset, r)
			keys[dnssecutil.KeyTag(r)] = r
		case *dns.RRSIG:
			sigs[r.KeyTag] = r
		}
	}
	if err := zp.Err(); err != nil {
		t.Fatalf("parse the .lv fixture: %v", err)
	}
	if len(rrset) != 2 || keys[42018] == nil || sigs[42018] == nil || keys[31113] == nil || sigs[31113] == nil {
		t.Fatalf("unexpected .lv fixture shape: %d DNSKEYs, keys %v, sigs %v", len(rrset), keys, sigs)
	}
	return rrset, keys, sigs
}

// The signature .lv publishes verifies through the local RSA path on the very
// records a resolver sees, while the library alone still refuses the key.
func TestVerifyRRSIGLargeExponentLV(t *testing.T) {
	rrset, keys, sigs := lvFixture(t)
	if err := dnssecutil.VerifyRRSIG(sigs[42018], rrset, keys[42018], lvAt); err != nil {
		t.Fatalf("live .lv KSK signature should verify: %v", err)
	}
	if err := dnssecutil.VerifyRRSIG(sigs[31113], rrset, keys[31113], lvAt); err != nil {
		t.Fatalf("live .lv ZSK signature should verify: %v", err)
	}
	// The caller's records keep their wire TTL; the verifier worked on copies.
	for i, rr := range rrset {
		if rr.Header().TTL != 204 {
			t.Errorf("record %d: TTL changed to %d, want 204", i, rr.Header().TTL)
		}
	}

	// The day the library verifies this key by itself, the local path can go.
	libRRset, libKeys, libSigs := lvFixture(t)
	if err := libSigs[42018].Verify(libKeys[42018], libRRset, &dns.SignOption{}); !errors.Is(err, dns.ErrKey) {
		t.Errorf("library Verify = %v, want %v: reconsider verifyLargeExponentRSA", err, dns.ErrKey)
	}
}

// A mismatching signature under a large-exponent key is a real failure, never
// the unsupported-exponent verdict, and the validity window still applies.
func TestVerifyRRSIGLargeExponentBadSignature(t *testing.T) {
	rrset, keys, sigs := lvFixture(t)
	rrset[0].(*dns.DNSKEY).Flags ^= 1
	err := dnssecutil.VerifyRRSIG(sigs[42018], rrset, keys[42018], lvAt)
	if err == nil || errors.Is(err, dnssecutil.ErrRSAExponentUnsupported) {
		t.Errorf("tampered RRset: err = %v, want a plain verification failure", err)
	}

	rrset, keys, sigs = lvFixture(t)
	sigs[42018].Signature = sigs[31113].Signature
	err = dnssecutil.VerifyRRSIG(sigs[42018], rrset, keys[42018], lvAt)
	if err == nil || errors.Is(err, dnssecutil.ErrRSAExponentUnsupported) {
		t.Errorf("foreign signature bytes: err = %v, want a plain verification failure", err)
	}

	rrset, keys, sigs = lvFixture(t)
	if err := dnssecutil.VerifyRRSIG(sigs[42018], rrset, keys[42018], lvAt.AddDate(0, 1, 0)); err == nil {
		t.Error("expired .lv KSK signature should not verify")
	}
}

// Past 64 exponent bits the local path declines as well, and says so with the
// sentinel the testcases turn into the NOTICE.
func TestVerifyRRSIGDeclinesExponentPast64Bits(t *testing.T) {
	key := dnstest.RSADNSKEY("lv.", dns.FlagZONE|dns.FlagSEP, dnstest.LVKSK42018E65)
	if !dnssecutil.RSAExponentBeyondLocalVerifier(key) {
		t.Fatal("a 65-bit exponent should be beyond the library verifier")
	}
	if bits := dnssecutil.KeySize(key); bits != 2048 {
		t.Fatalf("KeySize = %d, want 2048", bits)
	}
	rrset, _, sigs := lvFixture(t)
	sig := sigs[42018]
	sig.KeyTag = dnssecutil.KeyTag(key)
	if err := dnssecutil.VerifyRRSIG(sig, rrset, key, lvAt); !errors.Is(err, dnssecutil.ErrRSAExponentUnsupported) {
		t.Errorf("err = %v, want ErrRSAExponentUnsupported", err)
	}
}

// Keys generated with exponents the library refuses round-trip through the
// dnstest signer and the local verifier, right up to the 64-bit ceiling.
func TestVerifyRRSIGLargeExponentGenerated(t *testing.T) {
	now := time.Now().UTC()
	for _, tc := range []struct {
		name string
		e    *big.Int
	}{
		{"2^32+1 as .lv", dnstest.LVExponent},
		{"2^64-59, the largest 64-bit exponent", new(big.Int).SetUint64(1<<64 - 59)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			kp := dnstest.GenRSAKeyWithExponent(t, "example.test", tc.e, 1024, false)
			if !dnssecutil.RSAExponentBeyondLocalVerifier(kp.Key) {
				t.Fatal("the generated key should be beyond the library verifier")
			}
			a := &dns.A{Hdr: dns.Header{Name: "example.test.", Class: dns.ClassINET, TTL: 300}}
			a.Addr = netip.MustParseAddr("192.0.2.1")
			rrset := []dns.RR{a}
			sig := dnstest.SignRRset(t, kp, rrset, "example.test", "example.test", now.Add(-time.Hour), now.Add(time.Hour))
			if err := dnssecutil.VerifyRRSIG(sig, rrset, kp.Key, now); err != nil {
				t.Fatalf("generated large-exponent signature should verify: %v", err)
			}
			a.Addr = netip.MustParseAddr("192.0.2.2")
			err := dnssecutil.VerifyRRSIG(sig, rrset, kp.Key, now)
			if err == nil || errors.Is(err, dnssecutil.ErrRSAExponentUnsupported) {
				t.Errorf("changed record: err = %v, want a plain verification failure", err)
			}
		})
	}
}
