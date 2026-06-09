package cachefile

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"

	dns "codeberg.org/miekg/dns"
	"codeberg.org/miekg/dns/dnsutil"

	"codeberg.org/pawal/gonemaster/engine/asnlookup"
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

	file, err := Export(ns, rec, nil)
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
	if err := Import(file, restoredNS, restoredRec, nil); err != nil {
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
	if err := Save(path, ns, rec, nil); err != nil {
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
	if err := Restore(path, restoredNS, restoredRec, nil); err != nil {
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

	if err := Import(File{Version: Version}, ns, rec, nil); err == nil {
		t.Fatalf("expected missing format error")
	}
	if err := Import(File{Format: "other", Version: Version}, ns, rec, nil); err == nil {
		t.Fatalf("expected unsupported format error")
	}
	if err := Import(File{Format: Format, Version: Version + 1}, ns, rec, nil); err == nil {
		t.Fatalf("expected unsupported version error")
	}
	if err := Import(File{Format: Format, Version: Version, Entries: []Entry{{Kind: "other"}}}, ns, rec, nil, WithStrict()); err == nil {
		t.Fatalf("expected unknown kind error in strict mode")
	}
	if err := Import(File{Format: Format, Version: Version, Entries: []Entry{{}}}, ns, rec, nil, WithStrict()); err == nil {
		t.Fatalf("expected missing kind error in strict mode")
	}
	badBase64 := File{Format: Format, Version: Version, Entries: []Entry{{
		Kind:    KindNameserver,
		Address: "192.0.2.53",
		Key:     "k",
		Message: "!!!",
	}}}
	if err := Import(badBase64, ns, rec, nil); err == nil {
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
	if err := Restore(path, ns, rec, nil); err == nil {
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
	if err := Import(nsOnly, ns, nil, nil); err != nil {
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
	if err := Import(recOnly, nameserver.NewCacheStore(), nil, nil); err == nil {
		t.Fatalf("expected error when recursor entries present without recursor")
	}
}

func TestCachefileExportSkipsNilPointers(t *testing.T) {
	// Make sure a Recursor that was never populated round-trips as empty
	// without surprising the exporter - exercises nil/empty paths on both
	// sides of the Import/Export helpers.
	file, err := Export(nil, &recursor.Recursor{}, nil)
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

func TestCachefileExportStampsChecksum(t *testing.T) {
	ns := nameserver.NewCacheStore()
	rec := &recursor.Recursor{}
	seedNameserverCache(t, ns)
	seedRecursorCache(t, rec)

	file, err := Export(ns, rec, nil)
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if len(file.Checksum) != 64 {
		t.Fatalf("expected 64-char hex checksum, got %q", file.Checksum)
	}
	// Re-exporting the same caches yields the same checksum (deterministic).
	again, err := Export(ns, rec, nil)
	if err != nil {
		t.Fatalf("re-export: %v", err)
	}
	if again.Checksum != file.Checksum {
		t.Fatalf("checksum not deterministic: %s vs %s", file.Checksum, again.Checksum)
	}
}

func TestCachefileChecksumMismatchFails(t *testing.T) {
	ns := nameserver.NewCacheStore()
	rec := &recursor.Recursor{}
	seedNameserverCache(t, ns)

	file, err := Export(ns, rec, nil)
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	// Tamper with an entry but keep the old checksum.
	file.Entries[0].AnswerFrom = "tampered"

	err = Import(file, nameserver.NewCacheStore(), &recursor.Recursor{}, nil)
	if err == nil {
		t.Fatalf("expected checksum mismatch error")
	}
	if !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCachefileMissingChecksum(t *testing.T) {
	file := File{Format: Format, Version: Version, Entries: []Entry{}}

	// Non-strict: missing checksum emits a warning but Import succeeds.
	var warnings []string
	if err := Import(file, nameserver.NewCacheStore(), &recursor.Recursor{}, nil, WithWarnf(func(f string, a ...any) {
		warnings = append(warnings, fmt.Sprintf(f, a...))
	})); err != nil {
		t.Fatalf("non-strict import with missing checksum: %v", err)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "checksum is missing") {
		t.Fatalf("expected missing-checksum warning, got %v", warnings)
	}

	// Strict: same file must now fail.
	if err := Import(file, nameserver.NewCacheStore(), &recursor.Recursor{}, nil, WithStrict()); err == nil {
		t.Fatalf("expected strict-mode error for missing checksum")
	}
}

func TestCachefileUnknownKindWarning(t *testing.T) {
	file := File{Format: Format, Version: Version, Entries: []Entry{
		{Kind: "future-kind"},
	}}
	sum, err := checksumFor(file)
	if err != nil {
		t.Fatalf("checksum: %v", err)
	}
	file.Checksum = sum

	var warnings []string
	if err := Import(file, nameserver.NewCacheStore(), &recursor.Recursor{}, nil, WithWarnf(func(f string, a ...any) {
		warnings = append(warnings, fmt.Sprintf(f, a...))
	})); err != nil {
		t.Fatalf("non-strict import with unknown kind: %v", err)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "unknown kind") {
		t.Fatalf("expected unknown-kind warning, got %v", warnings)
	}

	if err := Import(file, nameserver.NewCacheStore(), &recursor.Recursor{}, nil, WithStrict()); err == nil {
		t.Fatalf("expected strict-mode error for unknown kind")
	}
}

func TestCachefileRestoreUnknownFieldWarning(t *testing.T) {
	blob := []byte(`{
  "format": "gonemaster.packet-cache",
  "version": 2,
  "mystery": "who cares",
  "entries": [
    {"kind": "nameserver", "address": "192.0.2.53", "key": "k", "no_message": true, "extra": true}
  ]
}`)
	path := filepath.Join(t.TempDir(), "warn.json")
	if err := os.WriteFile(path, blob, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	var warnings []string
	err := Restore(path, nameserver.NewCacheStore(), &recursor.Recursor{}, nil, WithWarnf(func(f string, a ...any) {
		warnings = append(warnings, fmt.Sprintf(f, a...))
	}))
	if err != nil {
		t.Fatalf("restore: %v", err)
	}

	var sawFileField, sawEntryField, sawMissingChecksum bool
	for _, w := range warnings {
		if strings.Contains(w, `unknown field "mystery"`) {
			sawFileField = true
		}
		if strings.Contains(w, `entry 0: unknown field "extra"`) {
			sawEntryField = true
		}
		if strings.Contains(w, "checksum is missing") {
			sawMissingChecksum = true
		}
	}
	if !sawFileField {
		t.Fatalf("expected file-level unknown-field warning, got %v", warnings)
	}
	if !sawEntryField {
		t.Fatalf("expected entry-level unknown-field warning, got %v", warnings)
	}
	if !sawMissingChecksum {
		t.Fatalf("expected missing-checksum warning, got %v", warnings)
	}

	// Strict mode fails on the first unknown field.
	err = Restore(path, nameserver.NewCacheStore(), &recursor.Recursor{}, nil, WithStrict())
	if err == nil {
		t.Fatalf("expected strict-mode error for unknown field")
	}
}

func TestCachefileSaveRestoreChecksumRoundTrip(t *testing.T) {
	ns := nameserver.NewCacheStore()
	rec := &recursor.Recursor{}
	seedNameserverCache(t, ns)
	seedRecursorCache(t, rec)

	path := filepath.Join(t.TempDir(), "cache.json")
	if err := Save(path, ns, rec, nil); err != nil {
		t.Fatalf("save: %v", err)
	}

	// Strict restore must succeed on a freshly written file.
	if err := Restore(path, nameserver.NewCacheStore(), &recursor.Recursor{}, nil, WithStrict()); err != nil {
		t.Fatalf("strict restore of freshly written file: %v", err)
	}

	// Corrupt the file and expect checksum mismatch on restore.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	corrupted := strings.Replace(string(data), "192.0.2.53", "198.51.100.1", 1)
	if corrupted == string(data) {
		t.Fatalf("corruption substitution did not change file")
	}
	if err := os.WriteFile(path, []byte(corrupted), 0o644); err != nil {
		t.Fatalf("write corrupted: %v", err)
	}
	err = Restore(path, nameserver.NewCacheStore(), &recursor.Recursor{}, nil)
	if err == nil {
		t.Fatalf("expected corruption to fail the checksum")
	}
	if !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func seedASNCache(t *testing.T, c *asnlookup.Cache) {
	t.Helper()
	if err := c.ImportEntries([]asnlookup.CacheEntry{
		{IP: "192.0.2.10", ASNs: []int{64496}, Prefix: "192.0.2.0/24", Raw: "raw1", Code: asnlookup.CodeFound},
		{IP: "2001:db8::1", Code: asnlookup.CodeEmpty},
	}); err != nil {
		t.Fatalf("seed asn cache: %v", err)
	}
}

func TestCachefileMixedRoundTripWithASN(t *testing.T) {
	ns := nameserver.NewCacheStore()
	rec := &recursor.Recursor{}
	asn := asnlookup.NewCache()
	seedNameserverCache(t, ns)
	seedRecursorCache(t, rec)
	seedASNCache(t, asn)

	file, err := Export(ns, rec, asn)
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	var nsCount, recCount, asnCount int
	for _, e := range file.Entries {
		switch e.Kind {
		case KindNameserver:
			nsCount++
		case KindRecursor:
			recCount++
		case KindASN:
			asnCount++
		}
	}
	if nsCount != 2 || recCount != 2 || asnCount != 2 {
		t.Fatalf("expected 2/2/2, got ns=%d rec=%d asn=%d", nsCount, recCount, asnCount)
	}

	restoredNS := nameserver.NewCacheStore()
	restoredRec := &recursor.Recursor{}
	restoredASN := asnlookup.NewCache()
	if err := Import(file, restoredNS, restoredRec, restoredASN); err != nil {
		t.Fatalf("import: %v", err)
	}
	if entries := restoredASN.ExportEntries(); len(entries) != 2 {
		t.Fatalf("expected 2 restored asn entries, got %d", len(entries))
	}
}

func TestCachefileSaveRestoreASNRoundTrip(t *testing.T) {
	asn := asnlookup.NewCache()
	seedASNCache(t, asn)

	path := filepath.Join(t.TempDir(), "asn-cache.json")
	if err := Save(path, nil, nil, asn); err != nil {
		t.Fatalf("save: %v", err)
	}

	restored := asnlookup.NewCache()
	if err := Restore(path, nameserver.NewCacheStore(), &recursor.Recursor{}, restored, WithStrict()); err != nil {
		t.Fatalf("strict restore: %v", err)
	}
	entries := restored.ExportEntries()
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].IP != "192.0.2.10" || entries[0].Code != asnlookup.CodeFound {
		t.Fatalf("unexpected first entry: %+v", entries[0])
	}
	if entries[1].IP != "2001:db8::1" || entries[1].Code != asnlookup.CodeEmpty {
		t.Fatalf("unexpected second entry: %+v", entries[1])
	}
}

func TestCachefileASNEntriesWithoutCacheErrors(t *testing.T) {
	file := File{Format: Format, Version: Version, Entries: []Entry{{
		Kind: KindASN, IP: "192.0.2.1", Code: asnlookup.CodeFound,
	}}}
	file.Checksum, _ = checksumFor(file)

	if err := Import(file, nameserver.NewCacheStore(), &recursor.Recursor{}, nil); err == nil {
		t.Fatal("expected error when asn entries present but cache is nil")
	}
}

func TestCachefileASNStrictRejectsMalformed(t *testing.T) {
	asn := asnlookup.NewCache()

	file := File{Format: Format, Version: Version, Entries: []Entry{{
		Kind: KindASN, IP: "not-an-ip", Code: asnlookup.CodeFound,
	}}}
	file.Checksum, _ = checksumFor(file)
	if err := Import(file, nameserver.NewCacheStore(), &recursor.Recursor{}, asn); err == nil {
		t.Fatal("expected error for bad IP in asn entry")
	}

	file2 := File{Format: Format, Version: Version, Entries: []Entry{{
		Kind: KindASN, IP: "192.0.2.1", ASNs: []int{-1}, Code: asnlookup.CodeFound,
	}}}
	file2.Checksum, _ = checksumFor(file2)
	if err := Import(file2, nameserver.NewCacheStore(), &recursor.Recursor{}, asn); err == nil {
		t.Fatal("expected error for negative ASN")
	}

	file3 := File{Format: Format, Version: Version, Entries: []Entry{{
		Kind: KindASN, IP: "192.0.2.1", Code: "BOGUS",
	}}}
	file3.Checksum, _ = checksumFor(file3)
	if err := Import(file3, nameserver.NewCacheStore(), &recursor.Recursor{}, asn); err == nil {
		t.Fatal("expected error for unknown code")
	}
}

func TestCachefileSaveCompressedRoundTrip(t *testing.T) {
	ns := nameserver.NewCacheStore()
	rec := &recursor.Recursor{}
	seedNameserverCache(t, ns)
	seedRecursorCache(t, rec)

	path := filepath.Join(t.TempDir(), "cache.json")
	if err := Save(path, ns, rec, nil, WithCompression()); err != nil {
		t.Fatalf("save: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if len(data) < 2 || data[0] != 0x1f || data[1] != 0x8b {
		t.Fatalf("expected gzip magic bytes in saved file, got %x", data[:min(len(data), 8)])
	}

	restoredNS := nameserver.NewCacheStore()
	restoredRec := &recursor.Recursor{}
	if err := Restore(path, restoredNS, restoredRec, nil, WithStrict()); err != nil {
		t.Fatalf("strict restore of compressed file: %v", err)
	}
	if entries, _ := restoredNS.ExportEntries(); len(entries) != 2 {
		t.Fatalf("expected 2 restored ns entries, got %d", len(entries))
	}
	if entries, _ := restoredRec.ExportCacheEntries(); len(entries) != 2 {
		t.Fatalf("expected 2 restored recursor entries, got %d", len(entries))
	}
}

func TestCachefileSaveGzSuffixImpliesCompression(t *testing.T) {
	ns := nameserver.NewCacheStore()
	seedNameserverCache(t, ns)

	path := filepath.Join(t.TempDir(), "cache.json.gz")
	if err := Save(path, ns, nil, nil); err != nil {
		t.Fatalf("save: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if len(data) < 2 || data[0] != 0x1f || data[1] != 0x8b {
		t.Fatalf("expected gzip magic bytes for .gz path even without WithCompression(), got %x", data[:min(len(data), 8)])
	}
	if err := Restore(path, nameserver.NewCacheStore(), nil, nil); err != nil {
		t.Fatalf("restore: %v", err)
	}
}

func TestCachefileSaveGzSuffixCaseInsensitive(t *testing.T) {
	ns := nameserver.NewCacheStore()
	seedNameserverCache(t, ns)

	path := filepath.Join(t.TempDir(), "cache.JSON.GZ")
	if err := Save(path, ns, nil, nil); err != nil {
		t.Fatalf("save: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if len(data) < 2 || data[0] != 0x1f || data[1] != 0x8b {
		t.Fatalf("expected gzip magic bytes for upper-case .GZ path, got %x", data[:min(len(data), 8)])
	}
}

func TestCachefileRestoreSniffsGzipRegardlessOfName(t *testing.T) {
	ns := nameserver.NewCacheStore()
	seedNameserverCache(t, ns)

	// Write with explicit compression but a non-.gz name.
	path := filepath.Join(t.TempDir(), "cache.bin")
	if err := Save(path, ns, nil, nil, WithCompression()); err != nil {
		t.Fatalf("save: %v", err)
	}

	if err := Restore(path, nameserver.NewCacheStore(), nil, nil, WithStrict()); err != nil {
		t.Fatalf("restore should sniff gzip magic regardless of file name: %v", err)
	}
}

func TestCachefileRestorePlainStillWorks(t *testing.T) {
	ns := nameserver.NewCacheStore()
	seedNameserverCache(t, ns)

	path := filepath.Join(t.TempDir(), "cache.json")
	if err := Save(path, ns, nil, nil); err != nil {
		t.Fatalf("save: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(data) >= 2 && data[0] == 0x1f && data[1] == 0x8b {
		t.Fatalf("expected plain JSON without gzip magic, got %x", data[:8])
	}
	if err := Restore(path, nameserver.NewCacheStore(), nil, nil, WithStrict()); err != nil {
		t.Fatalf("restore plain: %v", err)
	}
}

func TestCachefileRestoreCorruptedGzip(t *testing.T) {
	ns := nameserver.NewCacheStore()
	seedNameserverCache(t, ns)

	path := filepath.Join(t.TempDir(), "cache.json.gz")
	if err := Save(path, ns, nil, nil); err != nil {
		t.Fatalf("save: %v", err)
	}

	// Corrupt the gzip body (keep the magic so we still try to decompress).
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(data) < 32 {
		t.Fatalf("file too small to corrupt safely: %d bytes", len(data))
	}
	// Flip several bytes well past the gzip header to break the deflate body.
	data[len(data)-8]++
	data[len(data)-16]++
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	if err := Restore(path, nameserver.NewCacheStore(), nil, nil); err == nil {
		t.Fatalf("expected error on corrupted gzip body")
	}
}

func TestCachefileSaveMaxEntriesUnderLimitPasses(t *testing.T) {
	ns := nameserver.NewCacheStore()
	seedNameserverCache(t, ns) // 2 entries

	path := filepath.Join(t.TempDir(), "cache.json")
	if err := Save(path, ns, nil, nil, WithMaxEntries(5)); err != nil {
		t.Fatalf("save under limit: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected file to exist when under limit: %v", err)
	}
}

func TestCachefileSaveMaxEntriesOverLimitFails(t *testing.T) {
	ns := nameserver.NewCacheStore()
	seedNameserverCache(t, ns) // 2 entries

	path := filepath.Join(t.TempDir(), "cache.json")
	err := Save(path, ns, nil, nil, WithMaxEntries(1))
	if err == nil {
		t.Fatalf("expected error when entry count exceeds max")
	}
	if !strings.Contains(err.Error(), "max-entries=1") {
		t.Fatalf("expected max-entries error, got %v", err)
	}
	if _, statErr := os.Stat(path); statErr == nil {
		t.Fatalf("expected file NOT to be written when over limit")
	}
}

func TestCachefileSaveMaxEntriesZeroMeansUnlimited(t *testing.T) {
	ns := nameserver.NewCacheStore()
	seedNameserverCache(t, ns)

	path := filepath.Join(t.TempDir(), "cache.json")
	if err := Save(path, ns, nil, nil, WithMaxEntries(0)); err != nil {
		t.Fatalf("save with WithMaxEntries(0) should be unlimited: %v", err)
	}
}

func TestCachefileSaveMaxEntriesCountsAllKinds(t *testing.T) {
	ns := nameserver.NewCacheStore()
	rec := &recursor.Recursor{}
	asn := asnlookup.NewCache()
	seedNameserverCache(t, ns) // 2
	seedRecursorCache(t, rec)  // 2
	seedASNCache(t, asn)       // 2
	// Total = 6 entries. Limit at 5 should reject.

	path := filepath.Join(t.TempDir(), "cache.json")
	err := Save(path, ns, rec, asn, WithMaxEntries(5))
	if err == nil {
		t.Fatalf("expected error when combined entry count of all kinds exceeds max")
	}
	if !strings.Contains(err.Error(), "6 entries") {
		t.Fatalf("expected message to mention total of 6 entries, got %v", err)
	}
}

func TestCachefileCompressedFileIsSmallerForRepetitiveData(t *testing.T) {
	ns := nameserver.NewCacheStore()
	// Seed many duplicate-looking entries to give gzip something to compress.
	msg := buildPackedMsg(t, "example.com.", 1)
	var entries []nameserver.Entry
	for i := 0; i < 200; i++ {
		entries = append(entries, nameserver.Entry{
			Address:    "192.0.2.53",
			Key:        fmt.Sprintf("example.com./A/IN/%d", i),
			Message:    msg,
			AnswerFrom: "192.0.2.53:53",
		})
	}
	if err := ns.ImportEntries(entries); err != nil {
		t.Fatalf("seed: %v", err)
	}

	dir := t.TempDir()
	plain := filepath.Join(dir, "cache.json")
	gz := filepath.Join(dir, "cache.json.gz")
	if err := Save(plain, ns, nil, nil); err != nil {
		t.Fatalf("save plain: %v", err)
	}
	if err := Save(gz, ns, nil, nil, WithCompression()); err != nil {
		t.Fatalf("save gzip: %v", err)
	}

	plainStat, err := os.Stat(plain)
	if err != nil {
		t.Fatalf("stat plain: %v", err)
	}
	gzStat, err := os.Stat(gz)
	if err != nil {
		t.Fatalf("stat gz: %v", err)
	}
	if gzStat.Size() >= plainStat.Size() {
		t.Fatalf("expected gzip to be smaller than plain (got plain=%d, gz=%d)", plainStat.Size(), gzStat.Size())
	}
}
