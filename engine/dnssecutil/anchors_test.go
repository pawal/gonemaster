package dnssecutil

import (
	"strings"
	"testing"

	dns "codeberg.org/miekg/dns"
)

// The public keys IANA publishes beside each digest in root-anchors.xml.
func TestRootTrustAnchorsMatchPublishedKeys(t *testing.T) {
	keys := []struct {
		keyTag    uint16
		publicKey string
	}{
		{20326, "AwEAAaz/tAm8yTn4Mfeh5eyI96WSVexTBAvkMgJzkKTOiW1vkIbzxeF3+/4RgWOq7HrxRixHlFlExOLAJr5emLvN7SWXgnLh4+B5xQlNVz8Og8kvArMtNROxVQuCaSnIDdD5LKyWbRd2n9WGe2R8PzgCmr3EgVLrjyBxWezF0jLHwVN8efS3rCj/EWgvIWgb9tarpVUDK/b58Da+sqqls3eNbuv7pr+eoZG+SrDK6nWeL3c6H5Apxz7LjVc1uTIdsIXxuOLYA4/ilBmSVIzuDWfdRUfhHdY6+cn8HFRm+2hM8AnXGXws9555KrUB5qihylGa8subX2Nn6UwNR1AkUTV74bU="},
		{38696, "AwEAAa96jeuknZlaeSrvyAJj6ZHv28hhOKkx3rLGXVaC6rXTsDc449/cidltpkyGwCJNnOAlFNKF2jBosZBU5eeHspaQWOmOElZsjICMQMC3aeHbGiShvZsx4wMYSjH8e7Vrhbu6irwCzVBApESjbUdpWWmEnhathWu1jo+siFUiRAAxm9qyJNg/wOZqqzL/dL/q8PkcRU5oUKEpUge71M3ej2/7CPqpdVwuMoTvoB+ZOT4YeGyxMvHmbrxlFzGOHOijtzN+u1TQNatX2XBuzZNQ1K+s2CXkPIZo7s6JgZyvaBevYtxPvYLw4z9mR7K2vaF18UYH9Z9GNUUeayffKC73PYc="},
	}
	anchors := RootTrustAnchors()
	if len(anchors) != len(keys) {
		t.Fatalf("anchors = %d, want %d", len(anchors), len(keys))
	}
	for i, want := range keys {
		key := &dns.DNSKEY{Hdr: dns.Header{Name: ".", Class: dns.ClassINET}}
		key.Flags = 257
		key.Protocol = 3
		key.Algorithm = dns.RSASHA256
		key.PublicKey = want.publicKey
		computed := key.ToDS(dns.SHA256)
		if computed == nil {
			t.Fatalf("key %d: ToDS returned nil", want.keyTag)
		}
		got := anchors[i]
		if got.Hdr.Name != "." || got.KeyTag != want.keyTag || KeyTag(key) != want.keyTag {
			t.Fatalf("anchor %d: owner %q keytag %d, key keytag %d", want.keyTag, got.Hdr.Name, got.KeyTag, KeyTag(key))
		}
		if got.Algorithm != dns.RSASHA256 || got.DigestType != dns.SHA256 || !strings.EqualFold(got.Digest, computed.Digest) {
			t.Fatalf("anchor %d = %d %d %s, want 8 2 %s", want.keyTag, got.Algorithm, got.DigestType, got.Digest, computed.Digest)
		}
	}
}

func TestRootTrustAnchorsAreFreshCopies(t *testing.T) {
	RootTrustAnchors()[0].Digest = "00"
	if got := RootTrustAnchors()[0].Digest; got == "00" {
		t.Fatal("a caller's change leaked into the anchors")
	}
}
