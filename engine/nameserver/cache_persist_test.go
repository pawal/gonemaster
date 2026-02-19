package nameserver

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/miekg/dns"

	"codeberg.org/pawal/gonemaster/engine/packet"
)

func TestPacketCacheExportImportRoundTrip(t *testing.T) {
	cache := NewCacheStore()

	msg := new(dns.Msg)
	msg.SetQuestion("example.com.", dns.TypeA)
	msg.Response = true
	msg.Answer = []dns.RR{
		&dns.A{
			Hdr: dns.RR_Header{
				Name:   "example.com.",
				Rrtype: dns.TypeA,
				Class:  dns.ClassINET,
				Ttl:    60,
			},
			A: []byte{192, 0, 2, 10},
		},
	}

	cache.cacheForAddress("192.0.2.53").set("k.response", &packet.Packet{
		Msg:        msg,
		AnswerFrom: "192.0.2.53:53",
	})
	cache.cacheForAddress("192.0.2.53").set("k.empty", nil)

	exported, err := cache.ExportPacketCache()
	if err != nil {
		t.Fatalf("export packet cache: %v", err)
	}
	if exported.Format != PacketCacheFileFormat {
		t.Fatalf("unexpected format %q", exported.Format)
	}
	if exported.Version != PacketCacheFileVersion {
		t.Fatalf("unexpected version %d", exported.Version)
	}
	if len(exported.Entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(exported.Entries))
	}

	restored := NewCacheStore()
	if err := restored.ImportPacketCache(exported); err != nil {
		t.Fatalf("import packet cache: %v", err)
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

func TestPacketCacheSaveAndRestoreFile(t *testing.T) {
	cache := NewCacheStore()

	msg := new(dns.Msg)
	msg.SetQuestion("example.net.", dns.TypeAAAA)
	msg.Response = true
	wire, err := msg.Pack()
	if err != nil {
		t.Fatalf("pack dns msg: %v", err)
	}

	if err := cache.ImportPacketCache(PacketCacheFile{
		Format:  PacketCacheFileFormat,
		Version: PacketCacheFileVersion,
		Entries: []PacketCacheEntry{
			{
				Address:    "2001:db8::53",
				Key:        "k.ipv6",
				Message:    base64.StdEncoding.EncodeToString(wire),
				AnswerFrom: "2001:db8::53:53",
			},
		},
	}); err != nil {
		t.Fatalf("import packet cache fixture: %v", err)
	}

	path := filepath.Join(t.TempDir(), "cache.json")
	if err := cache.SavePacketCache(path); err != nil {
		t.Fatalf("save packet cache: %v", err)
	}

	restored := NewCacheStore()
	if err := restored.RestorePacketCache(path); err != nil {
		t.Fatalf("restore packet cache: %v", err)
	}

	exported, err := restored.ExportPacketCache()
	if err != nil {
		t.Fatalf("export restored cache: %v", err)
	}
	if len(exported.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(exported.Entries))
	}
	if exported.Entries[0].Address != "2001:db8::53" {
		t.Fatalf("unexpected address %q", exported.Entries[0].Address)
	}
	if exported.Entries[0].Key != "k.ipv6" {
		t.Fatalf("unexpected key %q", exported.Entries[0].Key)
	}
}

func TestPacketCacheImportValidationErrors(t *testing.T) {
	cache := NewCacheStore()
	if err := cache.ImportPacketCache(PacketCacheFile{
		Format:  PacketCacheFileFormat,
		Version: PacketCacheFileVersion + 1,
	}); err == nil {
		t.Fatalf("expected unsupported version error")
	}

	if err := cache.ImportPacketCache(PacketCacheFile{
		Format:  PacketCacheFileFormat,
		Version: PacketCacheFileVersion,
		Entries: []PacketCacheEntry{
			{
				Address: "not-an-ip",
				Key:     "k1",
			},
		},
	}); err == nil {
		t.Fatalf("expected invalid address error")
	}

	if err := cache.ImportPacketCache(PacketCacheFile{
		Format:  PacketCacheFileFormat,
		Version: PacketCacheFileVersion,
		Entries: []PacketCacheEntry{
			{
				Address: "192.0.2.53",
				Key:     "k2",
				Message: "!!!",
			},
		},
	}); err == nil {
		t.Fatalf("expected invalid base64 error")
	}
}

func TestPacketCacheRestoreInvalidJSON(t *testing.T) {
	cache := NewCacheStore()
	path := filepath.Join(t.TempDir(), "broken.json")
	if err := os.WriteFile(path, []byte("{"), 0o644); err != nil {
		t.Fatalf("write broken file: %v", err)
	}
	if err := cache.RestorePacketCache(path); err == nil {
		t.Fatalf("expected invalid JSON error")
	}
}

func TestPacketCacheFileJSONShape(t *testing.T) {
	cache := NewCacheStore()

	msg := new(dns.Msg)
	msg.SetQuestion("example.org.", dns.TypeTXT)
	msg.Response = true
	cache.cacheForAddress("192.0.2.99").set("k.shape", &packet.Packet{Msg: msg})

	path := filepath.Join(t.TempDir(), "shape.json")
	if err := cache.SavePacketCache(path); err != nil {
		t.Fatalf("save packet cache: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read saved file: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("unmarshal saved file: %v", err)
	}
	if payload["format"] != PacketCacheFileFormat {
		t.Fatalf("unexpected format: %#v", payload["format"])
	}
	if int(payload["version"].(float64)) != PacketCacheFileVersion {
		t.Fatalf("unexpected version: %#v", payload["version"])
	}
}
