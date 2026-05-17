package cache

import (
	"strings"
	"testing"

	dns "codeberg.org/miekg/dns"
)

func TestBuildKeyBasic(t *testing.T) {
	key, err := BuildKey(KeyParts{
		ServerAddr: "192.0.2.10:53",
		Name:       "Example.COM.",
		Qtype:      "a",
		Qclass:     "in",
	})
	if err != nil {
		t.Fatalf("BuildKey: %v", err)
	}

	wantParts := []string{
		"SERVER=192.0.2.10:53",
		"TRANSPORT=udp",
		"NAME=Example.COM",
		"TYPE=A",
		"CLASS=IN",
		"DNSSEC=false",
		"RECURSE=false",
		"EDNS_SIZE=0",
	}
	for _, part := range wantParts {
		if !strings.Contains(key, part) {
			t.Fatalf("expected key to include %q, got %q", part, key)
		}
	}
	if strings.Contains(key, "EDNS_VERSION=") || strings.Contains(key, "EDNS_DATA=") {
		t.Fatalf("unexpected EDNS detail in key: %q", key)
	}
}

func TestBuildKeyPreservesQNameCase(t *testing.T) {
	mixedKey, err := BuildKey(KeyParts{
		ServerAddr: "192.0.2.10",
		Name:       "ExAmPlE.CoM.",
		Qtype:      "A",
		Qclass:     "IN",
	})
	if err != nil {
		t.Fatalf("BuildKey mixed: %v", err)
	}
	lowerKey, err := BuildKey(KeyParts{
		ServerAddr: "192.0.2.10",
		Name:       "example.com.",
		Qtype:      "A",
		Qclass:     "IN",
	})
	if err != nil {
		t.Fatalf("BuildKey lower: %v", err)
	}
	if mixedKey == lowerKey {
		t.Fatalf("cache key must preserve QNAME case:\n mixed: %s\n lower: %s", mixedKey, lowerKey)
	}
	if !strings.Contains(mixedKey, "NAME=ExAmPlE.CoM") {
		t.Fatalf("mixed-case key lost QNAME case: %q", mixedKey)
	}
}

func TestBuildKeyWithEDNS(t *testing.T) {
	version := uint8(0)
	z := uint16(0x8000)
	rcode := uint8(0)
	key, err := BuildKey(KeyParts{
		ServerAddr:  "2001:db8::1",
		Name:        "example.net",
		Qtype:       "AAAA",
		Qclass:      "IN",
		DNSSEC:      true,
		Recurse:     true,
		UseVC:       true,
		EDNSSize:    1232,
		EDNSVersion: &version,
		EDNSZ:       &z,
		EDNSRcode:   &rcode,
		EDNSData:    []dns.EDNS0{&dns.NSID{Nsid: "aa"}},
	})
	if err != nil {
		t.Fatalf("BuildKey: %v", err)
	}

	wantParts := []string{
		"SERVER=2001:db8::1",
		"TRANSPORT=tcp",
		"NAME=example.net",
		"TYPE=AAAA",
		"CLASS=IN",
		"DNSSEC=true",
		"RECURSE=true",
		"EDNS_VERSION=0",
		"EDNS_Z=32768",
		"EDNS_RCODE=0",
		"EDNS_DATA=",
		"EDNS_SIZE=1232",
	}
	for _, part := range wantParts {
		if !strings.Contains(key, part) {
			t.Fatalf("expected key to include %q, got %q", part, key)
		}
	}
}

func TestBuildKeyRejectsEDNSSize(t *testing.T) {
	_, err := BuildKey(KeyParts{
		ServerAddr: "192.0.2.1",
		Name:       "example.com",
		Qtype:      "A",
		Qclass:     "IN",
		EDNSSize:   70000,
	})
	if err == nil {
		t.Fatalf("expected error for oversized EDNS size")
	}
}
