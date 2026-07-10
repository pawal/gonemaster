package dnssecutil

import (
	"crypto"
	"net/netip"
	"testing"
	"time"

	dns "codeberg.org/miekg/dns"
	"codeberg.org/miekg/dns/dnsutil"
)

func TestAlgorithmSupported(t *testing.T) {
	supported := []uint8{dns.RSASHA1, dns.RSASHA1NSEC3SHA1, dns.RSASHA256, dns.RSASHA512, dns.ECDSAP256SHA256, dns.ECDSAP384SHA384, dns.ED25519}
	for _, algo := range supported {
		if !AlgorithmSupported(algo) {
			t.Errorf("algorithm %d should be supported", algo)
		}
	}
	unsupported := []uint8{dns.RSAMD5, dns.DSA, dns.ECCGOST, dns.ED448, 0, 99}
	for _, algo := range unsupported {
		if AlgorithmSupported(algo) {
			t.Errorf("algorithm %d should not be supported", algo)
		}
	}
}

func TestKeySize(t *testing.T) {
	if got := KeySize(nil); got != 0 {
		t.Errorf("nil key: got %d, want 0", got)
	}
	if got := KeySize(&dns.DNSKEY{}); got != 0 {
		t.Errorf("empty key: got %d, want 0", got)
	}

	key := &dns.DNSKEY{Hdr: dns.Header{Name: dnsutil.Fqdn("example.test"), Class: dns.ClassINET, TTL: 3600}}
	key.Flags = dns.FlagZONE | dns.FlagSEP
	key.Protocol = 3
	key.Algorithm = dns.RSASHA256
	if _, err := key.Generate(1024); err != nil {
		t.Fatalf("generate key: %v", err)
	}
	if got := KeySize(key); got < 1000 || got > 1024 {
		t.Errorf("1024-bit RSA key: got %d bits, want ~1024", got)
	}
}

func TestVerifyRRSIG(t *testing.T) {
	if err := VerifyRRSIG(nil, nil, nil, time.Unix(1000, 0)); err == nil {
		t.Error("nil sig/key should error")
	}

	key := &dns.DNSKEY{Hdr: dns.Header{Name: dnsutil.Fqdn("example.test"), Class: dns.ClassINET, TTL: 3600}}
	key.Flags = dns.FlagZONE
	key.Protocol = 3
	key.Algorithm = dns.RSASHA256
	priv, err := key.Generate(1024)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	signer, ok := priv.(crypto.Signer)
	if !ok {
		t.Fatalf("private key does not implement crypto.Signer")
	}

	aRR := &dns.A{Hdr: dns.Header{Name: dnsutil.Fqdn("example.test"), Class: dns.ClassINET, TTL: 3600}}
	aRR.Addr = netip.MustParseAddr("192.0.2.1")
	rrset := []dns.RR{aRR}

	now := time.Now().UTC()
	sig := &dns.RRSIG{Hdr: dns.Header{Name: dnsutil.Fqdn("example.test"), Class: dns.ClassINET, TTL: 3600}}
	sig.Algorithm = key.Algorithm
	sig.Inception = uint32(now.Add(-time.Hour).Unix())
	sig.Expiration = uint32(now.Add(24 * time.Hour).Unix())
	sig.KeyTag = key.KeyTag()
	sig.SignerName = dnsutil.Fqdn("example.test")
	if err := sig.Sign(signer, rrset, &dns.SignOption{}); err != nil {
		t.Fatalf("sign RRset: %v", err)
	}

	if err := VerifyRRSIG(sig, rrset, key, now); err != nil {
		t.Errorf("valid signature should verify: %v", err)
	}
	beforeInception := time.Unix(int64(sig.Inception)-3600, 0)
	if err := VerifyRRSIG(sig, rrset, key, beforeInception); err == nil {
		t.Error("signature before inception should not verify")
	}
	afterExpiration := time.Unix(int64(sig.Expiration)+3600, 0)
	if err := VerifyRRSIG(sig, rrset, key, afterExpiration); err == nil {
		t.Error("expired signature should not verify")
	}
}
