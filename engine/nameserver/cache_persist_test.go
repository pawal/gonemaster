package nameserver

import (
	"net/netip"
	"testing"

	dns "codeberg.org/miekg/dns"
	"codeberg.org/miekg/dns/dnsutil"

	"codeberg.org/pawal/gonemaster/engine/packet"
)

func TestCacheStoreExportImportRoundTrip(t *testing.T) {
	cache := NewCacheStore()

	msg := new(dns.Msg)
	dnsutil.SetQuestion(msg, "example.com.", dns.TypeA)
	msg.Response = true
	aRR := &dns.A{Hdr: dns.Header{Name: "example.com.", Class: dns.ClassINET, TTL: 60}}
	aRR.Addr = netip.AddrFrom4([4]byte{192, 0, 2, 10})
	msg.Answer = []dns.RR{aRR}

	cache.cacheForAddress("192.0.2.53").set("k.response", &packet.Packet{
		Msg:        msg,
		AnswerFrom: "192.0.2.53:53",
	})
	cache.cacheForAddress("192.0.2.53").set("k.empty", nil)

	entries, err := cache.ExportEntries()
	if err != nil {
		t.Fatalf("export entries: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}

	restored := NewCacheStore()
	if err := restored.ImportEntries(entries); err != nil {
		t.Fatalf("import entries: %v", err)
	}

	loadedResponse, ok := restored.cacheForAddress("192.0.2.53").get("k.response")
	if !ok || loadedResponse == nil || loadedResponse.Msg == nil {
		t.Fatalf("expected restored response packet entry")
	}
	if loadedResponse.AnswerFrom != "192.0.2.53:53" {
		t.Fatalf("unexpected AnswerFrom %q", loadedResponse.AnswerFrom)
	}
	if len(loadedResponse.Msg.Answer) != 1 {
		t.Fatalf("expected restored answer section")
	}

	loadedEmpty, ok := restored.cacheForAddress("192.0.2.53").get("k.empty")
	if !ok {
		t.Fatalf("expected restored empty entry")
	}
	if loadedEmpty != nil {
		t.Fatalf("expected nil packet for empty entry")
	}
}

func TestCacheStoreImportValidationErrors(t *testing.T) {
	cache := NewCacheStore()

	if err := cache.ImportEntries([]Entry{{Address: "not-an-ip", Key: "k1"}}); err == nil {
		t.Fatalf("expected invalid address error")
	}

	if err := cache.ImportEntries([]Entry{{Address: "192.0.2.53", Key: ""}}); err == nil {
		t.Fatalf("expected empty key error")
	}

	if err := cache.ImportEntries([]Entry{{Address: "192.0.2.53", Key: "k"}}); err == nil {
		t.Fatalf("expected missing message error")
	}

	if err := cache.ImportEntries([]Entry{{Address: "192.0.2.53", Key: "k", Message: []byte{0xff, 0xff}}}); err == nil {
		t.Fatalf("expected unpack error for malformed message")
	}
}
