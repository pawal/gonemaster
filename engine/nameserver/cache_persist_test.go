package nameserver

import (
	"context"
	"fmt"
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

func TestCacheStoreExportsTimeoutAsNoMessage(t *testing.T) {
	ctx, prof := testContext(t)
	prof.Resolver.Defaults.ErrorCacheTTL = 0

	store := NewCacheStore()
	ns := hookedNS(t, store, "ns.example", "192.0.2.252", func(_ context.Context, _ string, _ string, _ string, _ *QueryOptions) (packet.Packet, error) {
		return packet.Packet{}, fmt.Errorf("timeout")
	})

	if _, err := ns.QueryWithOptions(ctx, "example", "SOA", &QueryOptions{BlacklistingDisabled: true}); err == nil {
		t.Fatalf("expected timeout error")
	}

	entries, err := store.ExportEntries()
	if err != nil {
		t.Fatalf("export entries: %v", err)
	}
	var noMessageEntries int
	for _, e := range entries {
		if e.Address == "192.0.2.252" && e.NoMessage {
			noMessageEntries++
		}
	}
	if noMessageEntries == 0 {
		t.Fatalf("expected at least one NoMessage entry from timed-out query, got entries: %+v", entries)
	}
}

func TestCacheStoreImportValidationErrors(t *testing.T) {
	tests := []struct {
		name  string
		entry Entry
	}{
		{name: "invalid address", entry: Entry{Address: "not-an-ip", Key: "k1"}},
		{name: "empty key", entry: Entry{Address: "192.0.2.53", Key: ""}},
		{name: "missing message", entry: Entry{Address: "192.0.2.53", Key: "k"}},
		{name: "unpackable message", entry: Entry{Address: "192.0.2.53", Key: "k", Message: []byte{0xff, 0xff}}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := NewCacheStore().ImportEntries([]Entry{tc.entry}); err == nil {
				t.Fatalf("expected %s to be rejected", tc.name)
			}
		})
	}
}

// The transport belongs to the recorded response, so it has to survive the
// export/import pair that backs cache files.
func TestCacheStoreExportImportPreservesProtocol(t *testing.T) {
	cache := NewCacheStore()

	msg := new(dns.Msg)
	dnsutil.SetQuestion(msg, "example.com.", dns.TypeA)
	msg.Response = true

	cache.cacheForAddress("192.0.2.53").set("k.tcp", &packet.Packet{Msg: msg, Protocol: "tcp"})
	cache.cacheForAddress("192.0.2.53").set("k.unknown", &packet.Packet{Msg: msg})

	entries, err := cache.ExportEntries()
	if err != nil {
		t.Fatalf("export entries: %v", err)
	}
	for _, entry := range entries {
		want := ""
		if entry.Key == "k.tcp" {
			want = "tcp"
		}
		if entry.Protocol != want {
			t.Errorf("%s exported protocol %q, want %q", entry.Key, entry.Protocol, want)
		}
	}

	restored := NewCacheStore()
	if err := restored.ImportEntries(entries); err != nil {
		t.Fatalf("import entries: %v", err)
	}
	got, ok := restored.cacheForAddress("192.0.2.53").get("k.tcp")
	if !ok || got == nil {
		t.Fatal("expected the restored tcp entry")
	}
	if got.Protocol != "tcp" {
		t.Errorf("restored protocol = %q, want tcp", got.Protocol)
	}
	unknown, ok := restored.cacheForAddress("192.0.2.53").get("k.unknown")
	if !ok || unknown == nil {
		t.Fatal("expected the restored unknown-transport entry")
	}
	if unknown.Protocol != "" {
		t.Errorf("restored protocol = %q, want empty", unknown.Protocol)
	}
}
