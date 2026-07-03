package nameserver

import (
	"testing"

	dns "codeberg.org/miekg/dns"
	"codeberg.org/miekg/dns/dnsutil"
)

func testSOA(t *testing.T, owner string) dns.RR {
	t.Helper()
	soa := &dns.SOA{Hdr: dns.Header{Name: dnsutil.Fqdn(owner), Class: dns.ClassINET, TTL: 60}}
	soa.Ns = dnsutil.Fqdn("ns1." + owner)
	soa.Mbox = dnsutil.Fqdn("hostmaster." + owner)
	soa.Serial = 1
	return soa
}

func TestPackUnpackRRRoundTrip(t *testing.T) {
	wire, err := packRR(testSOA(t, "example.com"))
	if err != nil {
		t.Fatalf("packRR: %v", err)
	}
	rr, err := unpackRR(wire)
	if err != nil {
		t.Fatalf("unpackRR: %v", err)
	}
	if _, ok := rr.(*dns.SOA); !ok {
		t.Fatalf("expected *dns.SOA, got %T", rr)
	}
}

func TestUnpackRRRejectsBadWire(t *testing.T) {
	if _, err := unpackRR([]byte{0x00, 0x01, 0x02}); err == nil {
		t.Fatalf("expected error for malformed wire")
	}
}

func TestAXFRCacheExportImportRoundTrip(t *testing.T) {
	src := NewCacheStore()
	src.axfrStore("192.0.2.53", "example.com", "IN", []dns.RR{testSOA(t, "example.com")}, false)
	src.axfrStore("2001:db8::1", "fail.example", "", nil, true)

	entries, err := src.ExportAXFREntries()
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}

	dst := NewCacheStore()
	if err := dst.ImportAXFREntries(entries); err != nil {
		t.Fatalf("import: %v", err)
	}

	avail, ok := dst.axfrLookup("192.0.2.53", "example.com", "IN")
	if !ok || avail.noTransfer || len(avail.rrs) != 1 {
		t.Fatalf("unexpected available record: %+v ok=%v", avail, ok)
	}
	fail, ok := dst.axfrLookup("2001:db8::1", "fail.example", "IN")
	if !ok || !fail.noTransfer || len(fail.rrs) != 0 {
		t.Fatalf("unexpected failure record: %+v ok=%v", fail, ok)
	}
}

func TestAXFRCacheKeyNormalization(t *testing.T) {
	c := NewCacheStore()
	c.axfrStore("192.0.2.53", "Example.COM", "in", []dns.RR{testSOA(t, "example.com")}, false)
	// Lookup with different case and no trailing dot must hit.
	if _, ok := c.axfrLookup("192.0.2.53", "example.com", ""); !ok {
		t.Fatalf("expected normalized lookup to hit")
	}
}

func TestImportAXFREntriesRejectsInvalid(t *testing.T) {
	c := NewCacheStore()
	wire, _ := packRR(testSOA(t, "example.com"))

	cases := []struct {
		name  string
		entry AXFREntry
	}{
		{"both rrs and no_transfer", AXFREntry{Address: "192.0.2.1", Name: "e.com", RRs: [][]byte{wire}, NoTransfer: true}},
		{"neither rrs nor no_transfer", AXFREntry{Address: "192.0.2.1", Name: "e.com"}},
		{"bad address", AXFREntry{Address: "not-an-ip", Name: "e.com", NoTransfer: true}},
		{"empty name", AXFREntry{Address: "192.0.2.1", NoTransfer: true}},
		{"bad rr wire", AXFREntry{Address: "192.0.2.1", Name: "e.com", RRs: [][]byte{{0x00, 0x01}}}},
	}
	for _, tc := range cases {
		if err := c.ImportAXFREntries([]AXFREntry{tc.entry}); err == nil {
			t.Fatalf("%s: expected error", tc.name)
		}
	}
}
