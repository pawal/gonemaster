package cachefile

import (
	"encoding/base64"
	"encoding/json"
	"net/netip"
	"os"
	"path/filepath"
	"testing"

	dns "codeberg.org/miekg/dns"
	"codeberg.org/miekg/dns/dnsutil"

	"codeberg.org/pawal/gonemaster/engine/nameserver"
	"codeberg.org/pawal/gonemaster/engine/recursor"
)

func buildPackedMsg(t *testing.T, name string, qtype uint16) []byte {
	t.Helper()
	msg := new(dns.Msg)
	dnsutil.SetQuestion(msg, name, qtype)
	msg.Response = true
	if err := msg.Pack(); err != nil {
		t.Fatalf("pack dns msg: %v", err)
	}
	return msg.Data
}

func seedNameserverCache(t *testing.T, cache *nameserver.CacheStore) {
	t.Helper()
	msg := new(dns.Msg)
	dnsutil.SetQuestion(msg, "example.com.", dns.TypeA)
	msg.Response = true
	// Store via the cache store helper; reuse the low-level path.
	if err := cache.ImportEntries([]nameserver.Entry{{
		Address:    "192.0.2.53",
		Key:        "example/a",
		Message:    buildPackedMsg(t, "example.com.", dns.TypeA),
		AnswerFrom: "192.0.2.53:53",
	}, {
		Address:   "192.0.2.53",
		Key:       "example/empty",
		NoMessage: true,
	}}); err != nil {
		t.Fatalf("seed ns cache: %v", err)
	}
}

func seedRecursorCache(t *testing.T, r *recursor.Recursor) {
	t.Helper()
	if err := r.ImportCacheEntries([]recursor.CacheEntry{
		{
			Name:    "example.com.",
			QType:   "A",
			QClass:  "IN",
			Message: buildPackedMsg(t, "example.com.", dns.TypeA),
		},
		{
			Name:   "example.net.",
			QType:  "NS",
			QClass: "IN",
			Nameservers: []recursor.NameserverRef{
				{Name: "ns1.example.", Address: "192.0.2.1"},
				{Name: "ns2.example.", Address: "2001:db8::1"},
			},
			Message: buildPackedMsg(t, "example.net.", dns.TypeNS),
		},
	}); err != nil {
		t.Fatalf("seed recursor cache: %v", err)
	}
}

func TestCachefileMixedRoundTrip(t *testing.T) {
	ns := nameserver.NewCacheStore()
	rec := &recursor.Recursor{}
	seedNameserverCache(t, ns)
	seedRecursorCache(t, rec)

	file, err := Export(ns, rec)
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if file.Format != Format || file.Version != Version {
		t.Fatalf("unexpected header: %+v", file)
	}
	var nsCount, recCount int
	for _, entry := range file.Entries {
		switch entry.Kind {
		case KindNameserver:
			nsCount++
		case KindRecursor:
			recCount++
		default:
			t.Fatalf("unexpected entry kind %q", entry.Kind)
		}
	}
	if nsCount != 2 {
		t.Fatalf("expected 2 nameserver entries, got %d", nsCount)
	}
	if recCount != 2 {
		t.Fatalf("expected 2 recursor entries, got %d", recCount)
	}

	restoredNS := nameserver.NewCacheStore()
	restoredRec := &recursor.Recursor{}
	if err := Import(file, restoredNS, restoredRec); err != nil {
		t.Fatalf("import: %v", err)
	}

	exportedNS, err := restoredNS.ExportEntries()
	if err != nil {
		t.Fatalf("re-export ns cache: %v", err)
	}
	if len(exportedNS) != 2 {
		t.Fatalf("expected 2 ns entries after import, got %d", len(exportedNS))
	}

	exportedRec, err := restoredRec.ExportCacheEntries()
	if err != nil {
		t.Fatalf("re-export recursor cache: %v", err)
	}
	if len(exportedRec) != 2 {
		t.Fatalf("expected 2 recursor entries after import, got %d", len(exportedRec))
	}
}

func TestCachefileSaveAndRestore(t *testing.T) {
	ns := nameserver.NewCacheStore()
	rec := &recursor.Recursor{}
	seedNameserverCache(t, ns)
	seedRecursorCache(t, rec)

	path := filepath.Join(t.TempDir(), "cache.json")
	if err := Save(path, ns, rec); err != nil {
		t.Fatalf("save: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	var shape map[string]any
	if err := json.Unmarshal(data, &shape); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if shape["format"] != Format {
		t.Fatalf("unexpected format: %#v", shape["format"])
	}
	if int(shape["version"].(float64)) != Version {
		t.Fatalf("unexpected version: %#v", shape["version"])
	}

	restoredNS := nameserver.NewCacheStore()
	restoredRec := &recursor.Recursor{}
	if err := Restore(path, restoredNS, restoredRec); err != nil {
		t.Fatalf("restore: %v", err)
	}
	if entries, _ := restoredNS.ExportEntries(); len(entries) != 2 {
		t.Fatalf("expected 2 restored ns entries, got %d", len(entries))
	}
	if entries, _ := restoredRec.ExportCacheEntries(); len(entries) != 2 {
		t.Fatalf("expected 2 restored recursor entries, got %d", len(entries))
	}
}

func TestCachefileImportValidationErrors(t *testing.T) {
	ns := nameserver.NewCacheStore()
	rec := &recursor.Recursor{}

	if err := Import(File{Version: Version}, ns, rec); err == nil {
		t.Fatalf("expected missing format error")
	}
	if err := Import(File{Format: "other", Version: Version}, ns, rec); err == nil {
		t.Fatalf("expected unsupported format error")
	}
	if err := Import(File{Format: Format, Version: Version + 1}, ns, rec); err == nil {
		t.Fatalf("expected unsupported version error")
	}
	if err := Import(File{Format: Format, Version: Version, Entries: []Entry{{Kind: "other"}}}, ns, rec); err == nil {
		t.Fatalf("expected unknown kind error")
	}
	if err := Import(File{Format: Format, Version: Version, Entries: []Entry{{}}}, ns, rec); err == nil {
		t.Fatalf("expected missing kind error")
	}
	badBase64 := File{Format: Format, Version: Version, Entries: []Entry{{
		Kind:    KindNameserver,
		Address: "192.0.2.53",
		Key:     "k",
		Message: "!!!",
	}}}
	if err := Import(badBase64, ns, rec); err == nil {
		t.Fatalf("expected base64 decode error")
	}
}

func TestCachefileRestoreInvalidJSON(t *testing.T) {
	ns := nameserver.NewCacheStore()
	rec := &recursor.Recursor{}
	path := filepath.Join(t.TempDir(), "broken.json")
	if err := os.WriteFile(path, []byte("{"), 0o644); err != nil {
		t.Fatalf("write broken file: %v", err)
	}
	if err := Restore(path, ns, rec); err == nil {
		t.Fatalf("expected invalid JSON error")
	}
}

func TestCachefileNameserverOnlyAndRecursorOnly(t *testing.T) {
	// Nameserver entries only, recursor not supplied.
	nsOnly := File{Format: Format, Version: Version, Entries: []Entry{{
		Kind:    KindNameserver,
		Address: "192.0.2.53",
		Key:     "k",
		Message: base64.StdEncoding.EncodeToString(buildPackedMsg(t, "example.com.", dns.TypeA)),
	}}}
	ns := nameserver.NewCacheStore()
	if err := Import(nsOnly, ns, nil); err != nil {
		t.Fatalf("nameserver-only import: %v", err)
	}

	// Recursor entries but no recursor supplied: should error.
	recOnly := File{Format: Format, Version: Version, Entries: []Entry{{
		Kind:    KindRecursor,
		Name:    "example.com.",
		QType:   "A",
		QClass:  "IN",
		Message: base64.StdEncoding.EncodeToString(buildPackedMsg(t, "example.com.", dns.TypeA)),
	}}}
	if err := Import(recOnly, nameserver.NewCacheStore(), nil); err == nil {
		t.Fatalf("expected error when recursor entries present without recursor")
	}
}

func TestCachefileExportSkipsNilPointers(t *testing.T) {
	// Make sure a Recursor that was never populated round-trips as empty
	// without surprising the exporter — exercises nil/empty paths on both
	// sides of the Import/Export helpers.
	file, err := Export(nil, &recursor.Recursor{})
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if len(file.Entries) != 0 {
		t.Fatalf("expected 0 entries, got %d", len(file.Entries))
	}
}

// sanity check: cache key parsing via export reconstructs the same key.
func TestCachefileRecursorKeyRoundTrip(t *testing.T) {
	r := &recursor.Recursor{}
	msg := new(dns.Msg)
	dnsutil.SetQuestion(msg, "zone.example.", dns.TypeNS)
	msg.Response = true
	if err := r.ImportCacheEntries([]recursor.CacheEntry{{
		Name:   "zone.example.",
		QType:  "NS",
		QClass: "IN",
		Nameservers: []recursor.NameserverRef{
			{Name: "ns1.Example.", Address: "192.0.2.10"},
		},
		Message: mustPack(t, msg),
	}}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	entries, err := r.ExportCacheEntries()
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	// Names are normalized lowercase by cacheNameKey (trailing dot stripped).
	if entries[0].Nameservers[0].Name != "ns1.example" {
		t.Fatalf("expected lowercased ns name, got %q", entries[0].Nameservers[0].Name)
	}
	// Reimport and confirm the exported addr parses.
	if _, err := netip.ParseAddr(entries[0].Nameservers[0].Address); err != nil {
		t.Fatalf("invalid ns address: %v", err)
	}

}

func mustPack(t *testing.T, msg *dns.Msg) []byte {
	t.Helper()
	if err := msg.Pack(); err != nil {
		t.Fatalf("pack: %v", err)
	}
	return msg.Data
}
