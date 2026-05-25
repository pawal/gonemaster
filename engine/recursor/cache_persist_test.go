package recursor

import (
	"net/netip"
	"testing"

	dns "codeberg.org/miekg/dns"
	"codeberg.org/miekg/dns/dnsutil"

	"codeberg.org/pawal/gonemaster/engine/dnsname"
	"codeberg.org/pawal/gonemaster/engine/nameserver"
	"codeberg.org/pawal/gonemaster/engine/packet"
)

func buildAnswer(t *testing.T, name string, qtype uint16) *dns.Msg {
	t.Helper()
	msg := new(dns.Msg)
	dnsutil.SetQuestion(msg, name, qtype)
	msg.Response = true
	return msg
}

func TestRecursorExportImportRootEntry(t *testing.T) {
	r := &Recursor{}

	msg := buildAnswer(t, "example.com.", dns.TypeA)
	aRR := &dns.A{Hdr: dns.Header{Name: "example.com.", Class: dns.ClassINET, TTL: 60}}
	aRR.Addr = netip.AddrFrom4([4]byte{192, 0, 2, 10})
	msg.Answer = []dns.RR{aRR}

	r.cacheStore(cacheNameKey(dnsname.New("example.com."), nil), "A", "IN", packet.Packet{Msg: msg})

	entries, err := r.ExportCacheEntries()
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Name != "example.com" || entries[0].QType != "A" || entries[0].QClass != "IN" {
		t.Fatalf("unexpected entry: %+v", entries[0])
	}
	if len(entries[0].Nameservers) != 0 {
		t.Fatalf("expected no nameservers for root entry, got %+v", entries[0].Nameservers)
	}

	restored := &Recursor{}
	if err := restored.ImportCacheEntries(entries); err != nil {
		t.Fatalf("import: %v", err)
	}
	cached, _, ok := restored.cacheLookup(cacheNameKey(dnsname.New("example.com."), nil), "A", "IN")
	if !ok {
		t.Fatalf("expected cached root entry after import")
	}
	if len(cached.Msg.Answer) != 1 {
		t.Fatalf("expected answer section in restored packet")
	}
}

func TestRecursorExportImportNSEntry(t *testing.T) {
	r := &Recursor{}

	nsA := nameserver.Nameserver{Name: dnsname.New("ns1.example."), Address: netip.MustParseAddr("192.0.2.1")}
	nsB := nameserver.Nameserver{Name: dnsname.New("ns2.example."), Address: netip.MustParseAddr("2001:db8::1")}

	msg := buildAnswer(t, "example.com.", dns.TypeNS)
	r.cacheStore(cacheNameKey(dnsname.New("example.com."), []nameserver.Nameserver{nsA, nsB}), "NS", "IN", packet.Packet{Msg: msg})

	entries, err := r.ExportCacheEntries()
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	got := entries[0]
	if got.Name != "example.com" || got.QType != "NS" {
		t.Fatalf("unexpected entry: %+v", got)
	}
	if len(got.Nameservers) != 2 {
		t.Fatalf("expected 2 nameserver refs, got %d", len(got.Nameservers))
	}

	restored := &Recursor{}
	if err := restored.ImportCacheEntries(entries); err != nil {
		t.Fatalf("import: %v", err)
	}
	cached, _, ok := restored.cacheLookup(cacheNameKey(dnsname.New("example.com."), []nameserver.Nameserver{nsA, nsB}), "NS", "IN")
	if !ok {
		t.Fatalf("expected cached ns entry after import")
	}
	if cached.Msg == nil {
		t.Fatalf("expected restored packet to carry Msg")
	}
}

func TestRecursorImportValidationErrors(t *testing.T) {
	r := &Recursor{}

	if err := r.ImportCacheEntries([]CacheEntry{{QType: "A"}}); err == nil {
		t.Fatalf("expected missing name error")
	}
	if err := r.ImportCacheEntries([]CacheEntry{{Name: "example.com."}}); err == nil {
		t.Fatalf("expected missing qtype error")
	}
	if err := r.ImportCacheEntries([]CacheEntry{{Name: "example.com.", QType: "A"}}); err == nil {
		t.Fatalf("expected missing message error")
	}
	if err := r.ImportCacheEntries([]CacheEntry{{
		Name:        "example.com.",
		QType:       "A",
		Nameservers: []NameserverRef{{Name: "ns1.example.", Address: "not-an-ip"}},
		Message:     []byte{0x00},
	}}); err == nil {
		t.Fatalf("expected invalid nameserver address error")
	}
}
