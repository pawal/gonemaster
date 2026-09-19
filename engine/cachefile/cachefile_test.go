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
	path := savedCache(t, "cache.json", withRecursor())

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
	tests := []struct {
		name   string
		file   File
		strict bool
	}{
		{name: "missing format", file: File{Version: Version}},
		{name: "unsupported format", file: File{Format: "other", Version: Version}},
		{name: "unsupported version", file: File{Format: Format, Version: Version + 1}},
		{
			name:   "unknown entry kind",
			file:   File{Format: Format, Version: Version, Entries: []Entry{{Kind: "other"}}},
			strict: true,
		},
		{
			name:   "missing entry kind",
			file:   File{Format: Format, Version: Version, Entries: []Entry{{}}},
			strict: true,
		},
		{
			name: "message is not base64",
			file: File{Format: Format, Version: Version, Entries: []Entry{{
				Kind:    KindNameserver,
				Address: "192.0.2.53",
				Key:     "k",
				Message: "!!!",
			}}},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var opts []Option
			if tc.strict {
				opts = append(opts, WithStrict())
			}
			err := Import(tc.file, nameserver.NewCacheStore(), &recursor.Recursor{}, nil, opts...)
			if err == nil {
				t.Fatalf("expected %s to be rejected", tc.name)
			}
		})
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
	path := savedCache(t, "cache.json", withRecursor())

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

// savedSpec is the cache file savedCache writes.
type savedSpec struct {
	nameservers bool
	recursor    bool
	asn         bool
	reuseNS     *nameserver.CacheStore
	saveOptions []SaveOption
}

// savedOpt shapes the file savedCache writes.
type savedOpt func(*savedSpec)

// withRecursor adds a seeded recursor cache to the file.
func withRecursor() savedOpt {
	return func(spec *savedSpec) { spec.recursor = true }
}

// withASN adds a seeded ASN cache to the file.
func withASN() savedOpt {
	return func(spec *savedSpec) { spec.asn = true }
}

// withoutNameservers omits the nameserver cache.
func withoutNameservers() savedOpt {
	return func(spec *savedSpec) { spec.nameservers = false }
}

// reusingNameservers writes the caller's cache instead of a freshly seeded one.
func reusingNameservers(ns *nameserver.CacheStore) savedOpt {
	return func(spec *savedSpec) { spec.reuseNS = ns }
}

// savingWith passes options through to Save.
func savingWith(opts ...SaveOption) savedOpt {
	return func(spec *savedSpec) { spec.saveOptions = opts }
}

// savedCache seeds the requested caches, writes them to name under the test's
// temp dir and returns the path.
func savedCache(t *testing.T, name string, opts ...savedOpt) string {
	t.Helper()

	spec := savedSpec{nameservers: true}
	for _, opt := range opts {
		opt(&spec)
	}

	var ns *nameserver.CacheStore
	switch {
	case spec.reuseNS != nil:
		ns = spec.reuseNS
	case spec.nameservers:
		ns = nameserver.NewCacheStore()
		seedNameserverCache(t, ns)
	}
	var rec *recursor.Recursor
	if spec.recursor {
		rec = &recursor.Recursor{}
		seedRecursorCache(t, rec)
	}
	var asn *asnlookup.Cache
	if spec.asn {
		asn = asnlookup.NewCache()
		seedASNCache(t, asn)
	}

	path := filepath.Join(t.TempDir(), name)
	if err := Save(path, ns, rec, asn, spec.saveOptions...); err != nil {
		t.Fatalf("save %s: %v", name, err)
	}
	return path
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
	path := savedCache(t, "asn-cache.json", withoutNameservers(), withASN())

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

// hasGzipMagic reports whether data starts with the gzip signature.
func hasGzipMagic(data []byte) bool {
	return len(data) >= 2 && data[0] == 0x1f && data[1] == 0x8b
}

func TestCachefileGzipSaveRestore(t *testing.T) {
	tests := []struct {
		name           string
		file           string
		saveOpts       []savedOpt
		wantGzip       bool
		strict         bool
		wantNSEntries  int
		wantRecEntries int
	}{
		{
			name:           "explicit compression round trips",
			file:           "cache.json",
			saveOpts:       []savedOpt{withRecursor(), savingWith(WithCompression())},
			wantGzip:       true,
			strict:         true,
			wantNSEntries:  2,
			wantRecEntries: 2,
		},
		{
			name:     "gz suffix implies compression",
			file:     "cache.json.gz",
			wantGzip: true,
		},
		{
			name:     "gz suffix is case insensitive",
			file:     "cache.JSON.GZ",
			wantGzip: true,
		},
		{
			name:     "restore sniffs gzip regardless of name",
			file:     "cache.bin",
			saveOpts: []savedOpt{savingWith(WithCompression())},
			wantGzip: true,
			strict:   true,
		},
		{
			name:     "plain save restores",
			file:     "cache.json",
			wantGzip: false,
			strict:   true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := savedCache(t, tc.file, tc.saveOpts...)

			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read %s: %v", tc.file, err)
			}
			if got := hasGzipMagic(data); got != tc.wantGzip {
				t.Fatalf("gzip magic in %s: got %v want %v (head %x)", tc.file, got, tc.wantGzip, data[:min(len(data), 8)])
			}

			var restoreOpts []Option
			if tc.strict {
				restoreOpts = append(restoreOpts, WithStrict())
			}
			restoredNS := nameserver.NewCacheStore()
			var restoredRec *recursor.Recursor
			if tc.wantRecEntries > 0 {
				restoredRec = &recursor.Recursor{}
			}
			if err := Restore(path, restoredNS, restoredRec, nil, restoreOpts...); err != nil {
				t.Fatalf("restore %s: %v", tc.file, err)
			}

			if tc.wantNSEntries > 0 {
				if entries, _ := restoredNS.ExportEntries(); len(entries) != tc.wantNSEntries {
					t.Fatalf("restored ns entries: got %d want %d", len(entries), tc.wantNSEntries)
				}
			}
			if tc.wantRecEntries > 0 {
				if entries, _ := restoredRec.ExportCacheEntries(); len(entries) != tc.wantRecEntries {
					t.Fatalf("restored recursor entries: got %d want %d", len(entries), tc.wantRecEntries)
				}
			}
		})
	}
}

func TestCachefileRestoreCorruptedGzip(t *testing.T) {
	path := savedCache(t, "cache.json.gz")

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

func TestCachefileSaveMaxEntries(t *testing.T) {
	tests := []struct {
		name            string
		recursor        bool
		asn             bool
		max             int
		wantErrContains string
	}{
		{name: "under limit passes", max: 5},   // 2 entries, limit 5
		{name: "zero means unlimited", max: 0}, // 2 entries, no limit
		{name: "over limit fails", max: 1, wantErrContains: "max-entries=1"},
		{
			name:            "counts all kinds",
			recursor:        true,
			asn:             true,
			max:             5, // 2 entries per kind = 6 total
			wantErrContains: "6 entries",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ns := nameserver.NewCacheStore()
			seedNameserverCache(t, ns)
			var rec *recursor.Recursor
			if tc.recursor {
				rec = &recursor.Recursor{}
				seedRecursorCache(t, rec)
			}
			var asn *asnlookup.Cache
			if tc.asn {
				asn = asnlookup.NewCache()
				seedASNCache(t, asn)
			}

			path := filepath.Join(t.TempDir(), "cache.json")
			err := Save(path, ns, rec, asn, WithMaxEntries(tc.max))

			if tc.wantErrContains == "" {
				if err != nil {
					t.Fatalf("save with max-entries=%d: %v", tc.max, err)
				}
				if _, statErr := os.Stat(path); statErr != nil {
					t.Fatalf("expected file to exist when under limit: %v", statErr)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error when entry count exceeds max-entries=%d", tc.max)
			}
			if !strings.Contains(err.Error(), tc.wantErrContains) {
				t.Fatalf("error %v does not mention %q", err, tc.wantErrContains)
			}
			if _, statErr := os.Stat(path); statErr == nil {
				t.Fatalf("expected file NOT to be written when over limit")
			}
		})
	}
}

func TestCachefileLoadRoundTrip(t *testing.T) {
	// Two entries of each kind. The nameserver cache is built here because the
	// test asserts Load leaves it untouched.
	ns := nameserver.NewCacheStore()
	seedNameserverCache(t, ns)
	path := savedCache(t, "cache.json", reusingNameservers(ns), withRecursor(), withASN())

	file, err := Load(path, WithStrict())
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(file.Entries) != 6 {
		t.Fatalf("expected 6 entries, got %d", len(file.Entries))
	}
	// Load must not touch any cache.
	if entries, _ := ns.ExportEntries(); len(entries) != 2 {
		t.Fatalf("source cache changed by Load: %d ns entries", len(entries))
	}
}

func TestCachefileLoadGzipAndStrict(t *testing.T) {
	path := savedCache(t, "cache.json.gz")
	if _, err := Load(path, WithStrict()); err != nil {
		t.Fatalf("load gzip: %v", err)
	}
}

func TestCachefileLoadStrictRejectsUnknownField(t *testing.T) {
	blob := []byte(`{"format":"gonemaster.packet-cache","version":2,"mystery":1,"entries":[]}`)
	path := filepath.Join(t.TempDir(), "cache.json")
	if err := os.WriteFile(path, blob, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := Load(path, WithStrict()); err == nil {
		t.Fatalf("expected strict-mode error for unknown field")
	}
}

func TestCachefileLoadRejectsChecksumMismatch(t *testing.T) {
	path := savedCache(t, "cache.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	corrupted := strings.Replace(string(data), "192.0.2.53", "198.51.100.1", 1)
	if err := os.WriteFile(path, []byte(corrupted), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := Load(path); err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("expected checksum mismatch, got %v", err)
	}
}

func TestCachefileStats(t *testing.T) {
	ns := nameserver.NewCacheStore()
	rec := &recursor.Recursor{}
	asn := asnlookup.NewCache()
	seedNameserverCache(t, ns) // 2, both on 192.0.2.53
	seedRecursorCache(t, rec)  // 2
	seedASNCache(t, asn)       // 2

	file, err := Export(ns, rec, asn)
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	s := file.Stats()
	if s.Total != 6 {
		t.Fatalf("expected total 6, got %d", s.Total)
	}
	if s.ByKind[KindNameserver] != 2 || s.ByKind[KindRecursor] != 2 || s.ByKind[KindASN] != 2 {
		t.Fatalf("unexpected by-kind counts: %+v", s.ByKind)
	}
	if s.ByAddress["192.0.2.53"] != 2 {
		t.Fatalf("expected 2 ns entries on 192.0.2.53, got %d", s.ByAddress["192.0.2.53"])
	}
}

func buildAXFRRRBase64(t *testing.T, owner string) string {
	t.Helper()
	m := new(dns.Msg)
	soa := &dns.SOA{Hdr: dns.Header{Name: dnsutil.Fqdn(owner), Class: dns.ClassINET, TTL: 60}}
	soa.Ns = dnsutil.Fqdn("ns1." + owner)
	soa.Mbox = dnsutil.Fqdn("hostmaster." + owner)
	soa.Serial = 1
	m.Answer = []dns.RR{soa}
	if err := m.Pack(); err != nil {
		t.Fatalf("pack soa: %v", err)
	}
	return base64.StdEncoding.EncodeToString(m.Data)
}

func TestCachefileAXFRRoundTrip(t *testing.T) {
	file := File{Format: Format, Version: Version, Entries: []Entry{
		{Kind: KindAXFR, Address: "192.0.2.53", Name: "example.com", QClass: "IN", RRs: []string{buildAXFRRRBase64(t, "example.com")}},
		{Kind: KindAXFR, Address: "2001:db8::1", Name: "fail.example", QClass: "IN", NoTransfer: true},
	}}
	file.Checksum, _ = checksumFor(file)

	ns := nameserver.NewCacheStore()
	if err := Import(file, ns, nil, nil); err != nil {
		t.Fatalf("import: %v", err)
	}

	out, err := ns.ExportAXFREntries()
	if err != nil {
		t.Fatalf("re-export: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("expected 2 axfr entries, got %d", len(out))
	}
	var available, failed int
	for _, e := range out {
		if e.NoTransfer {
			failed++
		} else if len(e.RRs) == 1 {
			available++
		}
	}
	if available != 1 || failed != 1 {
		t.Fatalf("expected 1 available + 1 failed, got %d/%d", available, failed)
	}
}

func TestCachefileMixedRoundTripWithAXFR(t *testing.T) {
	ns := nameserver.NewCacheStore()
	rec := &recursor.Recursor{}
	asn := asnlookup.NewCache()
	seedNameserverCache(t, ns) // 2 query entries
	seedRecursorCache(t, rec)  // 2
	seedASNCache(t, asn)       // 2

	wire, err := base64.StdEncoding.DecodeString(buildAXFRRRBase64(t, "zone.example"))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if err := ns.ImportAXFREntries([]nameserver.AXFREntry{
		{Address: "192.0.2.9", Name: "zone.example", QClass: "IN", RRs: [][]byte{wire}},
	}); err != nil {
		t.Fatalf("seed axfr: %v", err)
	}

	path := filepath.Join(t.TempDir(), "cache.json")
	if err := Save(path, ns, rec, asn); err != nil {
		t.Fatalf("save: %v", err)
	}

	restoredNS := nameserver.NewCacheStore()
	if err := Restore(path, restoredNS, &recursor.Recursor{}, asnlookup.NewCache(), WithStrict()); err != nil {
		t.Fatalf("strict restore: %v", err)
	}
	axfr, err := restoredNS.ExportAXFREntries()
	if err != nil {
		t.Fatalf("re-export axfr: %v", err)
	}
	if len(axfr) != 1 {
		t.Fatalf("expected 1 restored axfr entry, got %d", len(axfr))
	}
	if q, _ := restoredNS.ExportEntries(); len(q) != 2 {
		t.Fatalf("expected 2 restored query entries, got %d", len(q))
	}
}

func TestCachefileAXFRRejectsMalformed(t *testing.T) {
	ns := nameserver.NewCacheStore()

	bad := File{Format: Format, Version: Version, Entries: []Entry{
		{Kind: KindAXFR, Address: "192.0.2.1", Name: "e.com", RRs: []string{"!!!"}},
	}}
	bad.Checksum, _ = checksumFor(bad)
	if err := Import(bad, ns, nil, nil); err == nil {
		t.Fatalf("expected base64 decode error")
	}

	both := File{Format: Format, Version: Version, Entries: []Entry{
		{Kind: KindAXFR, Address: "192.0.2.1", Name: "e.com", RRs: []string{buildAXFRRRBase64(t, "e.com")}, NoTransfer: true},
	}}
	both.Checksum, _ = checksumFor(both)
	if err := Import(both, ns, nil, nil); err == nil {
		t.Fatalf("expected error for both rrs and no_transfer")
	}

	neither := File{Format: Format, Version: Version, Entries: []Entry{
		{Kind: KindAXFR, Address: "192.0.2.1", Name: "e.com"},
	}}
	neither.Checksum, _ = checksumFor(neither)
	if err := Import(neither, ns, nil, nil); err == nil {
		t.Fatalf("expected error for neither rrs nor no_transfer")
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

// A replayed run should see the same transport the recorded run saw, so the
// field has to survive the file round trip.
func TestCachefileProtocolRoundTrip(t *testing.T) {
	ns := nameserver.NewCacheStore()
	if err := ns.ImportEntries([]nameserver.Entry{{
		Address:  "192.0.2.53",
		Key:      "tcp/a",
		Message:  buildPackedMsg(t, "example.com.", dns.TypeA),
		Protocol: "tcp",
	}, {
		Address: "192.0.2.53",
		Key:     "unknown/a",
		Message: buildPackedMsg(t, "example.net.", dns.TypeA),
	}}); err != nil {
		t.Fatalf("seed ns cache: %v", err)
	}

	file, err := Export(ns, nil, nil)
	if err != nil {
		t.Fatalf("export: %v", err)
	}

	byKey := map[string]Entry{}
	for _, e := range file.Entries {
		byKey[e.Key] = e
	}
	if got := byKey["tcp/a"].Protocol; got != "tcp" {
		t.Errorf("exported protocol = %q, want tcp", got)
	}
	if got := byKey["unknown/a"].Protocol; got != "" {
		t.Errorf("an unknown transport exported %q, want empty", got)
	}

	// An unknown transport must not render as a field at all.
	blob, err := json.Marshal(byKey["unknown/a"])
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(blob), "protocol") {
		t.Errorf("empty protocol rendered a field: %s", blob)
	}

	restored := nameserver.NewCacheStore()
	if err := Import(file, restored, nil, nil, WithStrict()); err != nil {
		t.Fatalf("import: %v", err)
	}
	entries, err := restored.ExportEntries()
	if err != nil {
		t.Fatalf("re-export: %v", err)
	}
	if len(entries) != len(file.Entries) {
		t.Fatalf("restored %d entries, want the exported %d", len(entries), len(file.Entries))
	}
	for _, e := range entries {
		want := ""
		if e.Key == "tcp/a" {
			want = "tcp"
		}
		if e.Protocol != want {
			t.Errorf("%s restored protocol %q, want %q", e.Key, e.Protocol, want)
		}
	}
}

// Save and Restore carry the transport through the on-disk file too.
func TestCachefileProtocolSurvivesSaveRestore(t *testing.T) {
	ns := nameserver.NewCacheStore()
	if err := ns.ImportEntries([]nameserver.Entry{{
		Address:  "192.0.2.53",
		Key:      "udp/a",
		Message:  buildPackedMsg(t, "example.com.", dns.TypeA),
		Protocol: "udp",
	}}); err != nil {
		t.Fatalf("seed ns cache: %v", err)
	}

	path := filepath.Join(t.TempDir(), "cache.json")
	if err := Save(path, ns, nil, nil); err != nil {
		t.Fatalf("save: %v", err)
	}

	restored := nameserver.NewCacheStore()
	if err := Restore(path, restored, nil, nil, WithStrict()); err != nil {
		t.Fatalf("restore: %v", err)
	}
	entries, err := restored.ExportEntries()
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if len(entries) != 1 || entries[0].Protocol != "udp" {
		t.Fatalf("restored entries %+v, want one udp entry", entries)
	}
}

// version2Fixture was written by the release that predates the protocol field.
// Its checksum was computed over the old struct, so it also pins the invariant
// that makes multi-version reading sound: adding an omitempty field must not
// change how an old file marshals.
const version2Fixture = `{
  "format": "gonemaster.packet-cache",
  "version": 2,
  "checksum": "f8274d7aaed52c42e4417aa660f607dee391134067e535252a2e82b8a97a0139",
  "entries": [
    {
      "kind": "nameserver",
      "address": "192.0.2.53",
      "key": "k.response",
      "answer_from": "192.0.2.53:53",
      "message": "EjSBAAABAAEAAAAAB2V4YW1wbGUDY29tAAABAAHADAABAAEAAAA8AATAAAIK"
    },
    {
      "kind": "nameserver",
      "address": "192.0.2.53",
      "key": "k.empty",
      "no_message": true
    }
  ]
}`

func TestCachefileRestoresVersion2Files(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v2.json")
	if err := os.WriteFile(path, []byte(version2Fixture), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Strict mode leaves no room for a checksum or unknown-field complaint.
	ns := nameserver.NewCacheStore()
	if err := Restore(path, ns, nil, nil, WithStrict()); err != nil {
		t.Fatalf("restore version 2 file: %v", err)
	}

	entries, err := ns.ExportEntries()
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 restored entries, got %d", len(entries))
	}
	for _, e := range entries {
		if e.Protocol != "" {
			t.Errorf("%s restored protocol %q from a version 2 file, want empty", e.Key, e.Protocol)
		}
	}
	if entries[0].AnswerFrom == "" && entries[1].AnswerFrom == "" {
		t.Error("version 2 answer_from was lost")
	}
}

func TestCachefileVersionRange(t *testing.T) {
	for _, tc := range []struct {
		version int
		wantErr bool
	}{
		{minReadVersion - 1, true},
		{2, false},
		{3, false},
		{Version + 1, true},
	} {
		file := File{Format: Format, Version: tc.version, Entries: []Entry{}}
		sum, err := checksumFor(file)
		if err != nil {
			t.Fatalf("checksum: %v", err)
		}
		file.Checksum = sum

		err = Import(file, nameserver.NewCacheStore(), nil, nil)
		if tc.wantErr && err == nil {
			t.Errorf("version %d was accepted, want rejected", tc.version)
		}
		if !tc.wantErr && err != nil {
			t.Errorf("version %d was rejected: %v", tc.version, err)
		}
	}
}

func TestCachefileExportStampsCurrentVersion(t *testing.T) {
	ns := nameserver.NewCacheStore()
	seedNameserverCache(t, ns)

	file, err := Export(ns, nil, nil)
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if file.Version != 3 {
		t.Errorf("exported version %d, want 3", file.Version)
	}
	if err := Import(file, nameserver.NewCacheStore(), nil, nil, WithStrict()); err != nil {
		t.Errorf("exported file failed its own strict import: %v", err)
	}
}

// An unknown transport is a data error, not a reason to drop the packet.
func TestCachefileInvalidProtocol(t *testing.T) {
	file := File{Format: Format, Version: Version, Entries: []Entry{{
		Kind:     KindNameserver,
		Address:  "192.0.2.53",
		Key:      "bad/a",
		Message:  base64.StdEncoding.EncodeToString(buildPackedMsg(t, "example.com.", dns.TypeA)),
		Protocol: "quic",
	}}}
	sum, err := checksumFor(file)
	if err != nil {
		t.Fatalf("checksum: %v", err)
	}
	file.Checksum = sum

	if err := Import(file, nameserver.NewCacheStore(), nil, nil, WithStrict()); err == nil {
		t.Error("strict mode accepted an unknown protocol")
	} else if !strings.Contains(err.Error(), "entry 0") || !strings.Contains(err.Error(), "quic") {
		t.Errorf("error %q should name the entry and the value", err)
	}

	var warnings []string
	ns := nameserver.NewCacheStore()
	if err := Import(file, ns, nil, nil, WithWarnf(func(f string, a ...any) {
		warnings = append(warnings, fmt.Sprintf(f, a...))
	})); err != nil {
		t.Fatalf("lenient import: %v", err)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "quic") {
		t.Fatalf("expected one protocol warning, got %v", warnings)
	}
	entries, err := ns.ExportEntries()
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected the entry to be imported, got %d", len(entries))
	}
	if entries[0].Protocol != "" {
		t.Errorf("protocol = %q, want empty after an unknown value", entries[0].Protocol)
	}
}

// A cached empty response has no packet and so no transport.
func TestCachefileNoMessageEntryHasNoProtocol(t *testing.T) {
	ns := nameserver.NewCacheStore()
	if err := ns.ImportEntries([]nameserver.Entry{{
		Address:   "192.0.2.53",
		Key:       "empty/a",
		NoMessage: true,
	}}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	file, err := Export(ns, nil, nil)
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if len(file.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(file.Entries))
	}
	if file.Entries[0].Protocol != "" {
		t.Errorf("no_message entry carried protocol %q", file.Entries[0].Protocol)
	}
}
