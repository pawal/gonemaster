package cache

import (
	"strings"
	"testing"

	dns "codeberg.org/miekg/dns"
)

// mustBuildKey builds a cache key and fails the test if BuildKey errors.
func mustBuildKey(t *testing.T, parts KeyParts) string {
	t.Helper()
	key, err := BuildKey(parts)
	if err != nil {
		t.Fatalf("BuildKey(%+v): %v", parts, err)
	}
	return key
}

// requireKeyParts fails unless every part appears in the key.
func requireKeyParts(t *testing.T, key string, parts ...string) {
	t.Helper()
	for _, part := range parts {
		if !strings.Contains(key, part) {
			t.Fatalf("expected key to include %q, got %q", part, key)
		}
	}
}

// requireNoKeyParts fails if any part appears in the key.
func requireNoKeyParts(t *testing.T, key string, parts ...string) {
	t.Helper()
	for _, part := range parts {
		if strings.Contains(key, part) {
			t.Fatalf("unexpected %q in key: %q", part, key)
		}
	}
}

func TestBuildKeyBasic(t *testing.T) {
	key := mustBuildKey(t, KeyParts{
		ServerAddr: "192.0.2.10:53",
		Name:       "Example.COM.",
		Qtype:      "a",
		Qclass:     "in",
	})

	requireKeyParts(t, key,
		"SERVER=192.0.2.10:53",
		"TRANSPORT=udp",
		"NAME=Example.COM",
		"TYPE=A",
		"CLASS=IN",
		"DNSSEC=false",
		"RECURSE=false",
		"EDNS_SIZE=0",
	)
	requireNoKeyParts(t, key, "EDNS_VERSION=", "EDNS_DATA=")
}

func TestBuildKeyPreservesQNameCase(t *testing.T) {
	mixedKey := mustBuildKey(t, KeyParts{
		ServerAddr: "192.0.2.10",
		Name:       "ExAmPlE.CoM.",
		Qtype:      "A",
		Qclass:     "IN",
	})
	lowerKey := mustBuildKey(t, KeyParts{
		ServerAddr: "192.0.2.10",
		Name:       "example.com.",
		Qtype:      "A",
		Qclass:     "IN",
	})

	if mixedKey == lowerKey {
		t.Fatalf("cache key must preserve QNAME case:\n mixed: %s\n lower: %s", mixedKey, lowerKey)
	}
	requireKeyParts(t, mixedKey, "NAME=ExAmPlE.CoM")
}

func TestBuildKeyWithEDNS(t *testing.T) {
	version := uint8(0)
	z := uint16(0x8000)
	rcode := uint8(0)
	key := mustBuildKey(t, KeyParts{
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

	requireKeyParts(t, key,
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
	)
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
