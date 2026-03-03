package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestComparePlaceholdersParityPass(t *testing.T) {
	missing, extra := comparePlaceholders("Nameserver {ns} has address {address}.", "Adresse {address} pa {ns}.")
	if len(missing) != 0 || len(extra) != 0 {
		t.Fatalf("expected no parity issues, got missing=%v extra=%v", missing, extra)
	}
}

func TestComparePlaceholdersParityFail(t *testing.T) {
	missing, extra := comparePlaceholders("Nameserver {ns} has address {address}.", "Adresse pa {ns}.")
	if !reflect.DeepEqual(missing, []string{"address"}) {
		t.Fatalf("expected missing [address], got %v", missing)
	}
	if len(extra) != 0 {
		t.Fatalf("expected no extra placeholders, got %v", extra)
	}
}

func TestLegacyPlaceholderFailForNonAllowlistedContext(t *testing.T) {
	e := entry{
		context: "ADDRESS:TEST",
		msgID:   "Nameserver {nsname} has IP {ip}.",
	}
	got := legacyKeysForEntry(e)
	want := []string{"ip", "nsname"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected legacy keys %v, got %v", want, got)
	}
}

func TestLegacyPlaceholderPassForAllowlistedContext(t *testing.T) {
	e := entry{
		context: "SYSTEM:LOOKUP_ERROR",
		msgID:   "DNS query to {ns} for {domain}/{type}/{class} failed with error: {message}",
	}
	got := legacyKeysForEntry(e)
	if len(got) != 0 {
		t.Fatalf("expected no legacy violations for allowlisted placeholders, got %v", got)
	}
}

func TestParsePOIgnoresObsoleteEntries(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "sample.po")
	content := `#~ msgctxt "OLD:LEGACY"
#~ msgid "Old {nsname}"
#~ msgstr "Gammal {nsname}"

msgctxt "BASIC:OK"
msgid "All good for {ns}."
msgstr "Allt bra for {ns}."
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write po file: %v", err)
	}

	entries, err := parsePO(path)
	if err != nil {
		t.Fatalf("parsePO: %v", err)
	}
	var nonEmpty []entry
	for _, e := range entries {
		if e.context != "" || e.msgID != "" || e.msgStr != "" {
			nonEmpty = append(nonEmpty, e)
		}
	}
	if len(nonEmpty) != 1 {
		t.Fatalf("expected exactly one non-empty parsed entry, got %d", len(nonEmpty))
	}
	if nonEmpty[0].context != "BASIC:OK" {
		t.Fatalf("expected BASIC:OK context, got %q", nonEmpty[0].context)
	}
	if got := legacyKeysForEntry(nonEmpty[0]); len(got) != 0 {
		t.Fatalf("expected no legacy keys in parsed non-obsolete entry, got %v", got)
	}
}
