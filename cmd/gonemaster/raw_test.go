package main

import (
	"bytes"
	"strings"
	"testing"

	"codeberg.org/pawal/gonemaster/engine/logger"
)

func TestFormatRawArgValueEndpointList(t *testing.T) {
	value := []map[string]any{
		{"ns": "ns1.example", "address": "192.0.2.1"},
		{"ns": "ns2.example", "address": "192.0.2.2"},
	}
	got := formatRawArgValue(value)
	want := "ns1.example/192.0.2.1,ns2.example/192.0.2.2"
	if got != want {
		t.Fatalf("unexpected formatted endpoint list: got %q want %q", got, want)
	}
}

func TestFormatRawArgValueGenericMap(t *testing.T) {
	value := map[string]any{
		"query_type": "SOA",
		"query_name": "example.com",
	}
	got := formatRawArgValue(value)
	want := "{query_name=example.com;query_type=SOA}"
	if got != want {
		t.Fatalf("unexpected formatted map: got %q want %q", got, want)
	}
}

func TestRawReporterFormatsCollectionsWithoutJSON(t *testing.T) {
	var out bytes.Buffer
	reporter := newRawReporter(&out, "")

	entry, err := logger.NewEntry(
		"EXTRA_PROCESSING_OK",
		map[string]any{
			"servers": []map[string]any{
				{"ns": "ns1.example", "address": "192.0.2.1"},
				{"ns": "ns2.example", "address": "192.0.2.2"},
			},
			"mail_targets": []string{"mx1.example", "mx2.example"},
			"details": map[string]any{
				"query_type": "SOA",
				"query_name": "example.com",
			},
		},
		"DNSSEC06",
		"DNSSEC",
	)
	if err != nil {
		t.Fatalf("new entry: %v", err)
	}

	if err := reporter.Callback(entry); err != nil {
		t.Fatalf("callback: %v", err)
	}
	line := strings.TrimSpace(out.String())
	if !strings.Contains(line, "servers=ns1.example/192.0.2.1,ns2.example/192.0.2.2") {
		t.Fatalf("expected human-readable servers list, got %q", line)
	}
	if !strings.Contains(line, "mail_targets=mx1.example,mx2.example") {
		t.Fatalf("expected human-readable string list, got %q", line)
	}
	if !strings.Contains(line, "details={query_name=example.com;query_type=SOA}") {
		t.Fatalf("expected human-readable map, got %q", line)
	}
	if strings.Contains(line, `{"ns":"ns1.example"`) || strings.Contains(line, `["mx1.example"`) {
		t.Fatalf("did not expect JSON-formatted collections in raw output: %q", line)
	}
}

// TestFormatRawEntrySanitizesAttackerControlChars ensures that hostile bytes
// returned in a DNS response (ANSI colour codes, CR, LF, NUL) cannot reach
// the terminal verbatim via the raw output format.
func TestFormatRawEntrySanitizesAttackerControlChars(t *testing.T) {
	entry, err := logger.NewEntry(
		"ATTACK_DEMO",
		map[string]any{
			"version_string": "\x1b[34mblue\x1b[0m",
			"payload":        "line1\rline2\nline3",
			"nul":            "a\x00b",
		},
		"TC", "MOD",
	)
	if err != nil {
		t.Fatalf("NewEntry: %v", err)
	}
	line := formatRawEntry(entry)
	for i, b := range []byte(line) {
		if b < 0x20 || b == 0x7f || (b >= 0x80 && b <= 0x9f) {
			t.Fatalf("formatRawEntry leaked control byte %#x at offset %d: %q", b, i, line)
		}
	}
	for _, want := range []string{
		`\x1b[34mblue\x1b[0m`,
		`\x0d`, // CR
		`\x0a`, // LF
		`\x00`, // NUL
	} {
		if !strings.Contains(line, want) {
			t.Fatalf("expected escape %q in raw output, got %q", want, line)
		}
	}
}

// TestFormatRawEntrySanitizesNestedMap ensures sanitization reaches values
// hidden inside nested map args (e.g. the {ns, address} endpoint shape).
func TestFormatRawEntrySanitizesNestedMap(t *testing.T) {
	entry, err := logger.NewEntry(
		"ATTACK_NESTED",
		map[string]any{
			"details": map[string]any{
				"version_string": "x\x1by",
			},
		},
		"TC", "MOD",
	)
	if err != nil {
		t.Fatalf("NewEntry: %v", err)
	}
	line := formatRawEntry(entry)
	for _, b := range []byte(line) {
		if b == 0x1b {
			t.Fatalf("formatRawEntry leaked ESC byte in nested-map output: %q", line)
		}
	}
	if !strings.Contains(line, `x\x1by`) {
		t.Fatalf("expected escaped ESC inside nested map, got %q", line)
	}
}

// TestFormatRawEntryPreservesSafeContent guards against over-sanitization:
// printable ASCII, structure separators, and UTF-8 characters must pass
// through unchanged.
func TestFormatRawEntryPreservesSafeContent(t *testing.T) {
	entry, err := logger.NewEntry(
		"SAFE_DEMO",
		map[string]any{
			"ns":      "ns1.example.com",
			"address": "2001:db8::1",
			"label":   "café résumé",
		},
		"TC", "MOD",
	)
	if err != nil {
		t.Fatalf("NewEntry: %v", err)
	}
	line := formatRawEntry(entry)
	for _, want := range []string{"ns1.example.com", "2001:db8::1", "café résumé"} {
		if !strings.Contains(line, want) {
			t.Fatalf("expected %q to pass through, got %q", want, line)
		}
	}
	if strings.Contains(line, `\x`) {
		t.Fatalf("unexpected escape in safe output: %q", line)
	}
}

// TestRawReporterCallbackSanitizesOutput verifies that the full Callback path
// (not just formatRawEntry) emits sanitized bytes to the writer.
func TestRawReporterCallbackSanitizesOutput(t *testing.T) {
	var out bytes.Buffer
	reporter := newRawReporter(&out, "")

	entry, err := logger.NewEntry(
		"N15_SOFTWARE_VERSION",
		map[string]any{"string": "\x1b]0;hijack\x07"},
		"NS01", "NAMESERVER",
	)
	if err != nil {
		t.Fatalf("NewEntry: %v", err)
	}
	if err := reporter.Callback(entry); err != nil {
		t.Fatalf("Callback: %v", err)
	}

	for _, b := range out.Bytes() {
		if b == '\n' {
			continue
		}
		if b < 0x20 || b == 0x7f || (b >= 0x80 && b <= 0x9f) {
			t.Fatalf("raw reporter leaked control byte %#x: %q", b, out.String())
		}
	}
	if !strings.Contains(out.String(), `\x1b]0;hijack\x07`) {
		t.Fatalf("expected escaped OSC + BEL sequence, got %q", out.String())
	}
}
