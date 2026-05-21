package main

import (
	"bytes"
	"strings"
	"testing"
)

// TestEntryTextSanitizesMessage ensures that an interpolated server-side
// message carrying attacker-controlled DNS bytes (ANSI colour escape, SQL
// payload literal) cannot reach the terminal unescaped.
func TestEntryTextSanitizesMessage(t *testing.T) {
	entry := jobResultEntry{
		Level:   "WARNING",
		Message: "ns1.evil responded \x1b[34m'; DROP TABLE x; --\x1b[0m",
	}
	got := entryText(entry)
	for _, b := range []byte(got) {
		if b < 0x20 || b == 0x7f || (b >= 0x80 && b <= 0x9f) {
			t.Fatalf("entryText leaked control byte %#x: %q", b, got)
		}
	}
	if !strings.Contains(got, `\x1b[34m'; DROP TABLE x; --\x1b[0m`) {
		t.Fatalf("expected escaped ANSI around SQL payload, got %q", got)
	}
}

// TestEntryTextSanitizesRawFallback covers the path where the server did
// not provide a translated Message but did provide a raw one.
func TestEntryTextSanitizesRawFallback(t *testing.T) {
	entry := jobResultEntry{
		Level: "WARNING",
		Raw:   "MOD:TC:TAG version_string=x\rfake",
	}
	got := entryText(entry)
	for _, b := range []byte(got) {
		if b < 0x20 || b == 0x7f || (b >= 0x80 && b <= 0x9f) {
			t.Fatalf("entryText leaked control byte %#x in Raw fallback: %q", b, got)
		}
	}
	if !strings.Contains(got, `\x0d`) {
		t.Fatalf("expected escaped CR in Raw fallback, got %q", got)
	}
}

// TestEntryTextPreservesSafeContent guards against over-sanitization.
func TestEntryTextPreservesSafeContent(t *testing.T) {
	cases := []jobResultEntry{
		{Message: "ns1.example.com is reachable"},
		{Message: "café résumé naïve 日本語"},
		{Raw: "MOD:TC:TAG arg=value"},
		{Module: "MOD", Testcase: "TC", Tag: "TAG"},
	}
	wants := []string{
		"ns1.example.com is reachable",
		"café résumé naïve 日本語",
		"MOD:TC:TAG arg=value",
		"MOD:TC:TAG",
	}
	for i, in := range cases {
		got := entryText(in)
		if got != wants[i] {
			t.Fatalf("case %d: entryText changed safe content: got %q want %q", i, got, wants[i])
		}
	}
}

// TestEntryTextFallbackJoinSanitizes covers the rarely-hit branch where
// the server returned no Message/Raw but a hostile Tag string still
// reaches entryText through the joined fallback.
func TestEntryTextFallbackJoinSanitizes(t *testing.T) {
	entry := jobResultEntry{
		Module:   "MOD",
		Testcase: "TC",
		Tag:      "tag-with-esc-\x1b[31m",
	}
	got := entryText(entry)
	if strings.ContainsRune(got, 0x1b) {
		t.Fatalf("entryText leaked ESC in joined fallback: %q", got)
	}
	if !strings.Contains(got, `\x1b[31m`) {
		t.Fatalf("expected escaped ESC in joined fallback, got %q", got)
	}
}

// TestPrintModulesPrettySanitizesEntry verifies the full printer path
// (which is what users actually see) ends up with sanitized bytes on
// stdout.
func TestPrintModulesPrettySanitizesEntry(t *testing.T) {
	view := modulesView{
		JobID:  "abc",
		Status: "succeeded",
		Domain: "example.com",
		Modules: []moduleGroup{{
			Module: "NAMESERVER",
			Counts: map[string]int{"WARNING": 1},
			Entries: []jobResultEntry{{
				Level:   "WARNING",
				Message: "evil \x1b[31mxss\x1b[0m and \rfake",
			}},
		}},
	}
	var out bytes.Buffer
	printModulesPretty(&out, view)

	for i, b := range out.Bytes() {
		if b == '\n' {
			continue
		}
		if b < 0x20 || b == 0x7f || (b >= 0x80 && b <= 0x9f) {
			t.Fatalf("printModulesPretty leaked control byte %#x at offset %d: %q", b, i, out.String())
		}
	}
	if !strings.Contains(out.String(), `\x1b[31mxss\x1b[0m`) {
		t.Fatalf("expected escaped ANSI in pretty output, got %q", out.String())
	}
	if !strings.Contains(out.String(), `\x0d`) {
		t.Fatalf("expected escaped CR in pretty output, got %q", out.String())
	}
}

// TestPrintRawPrettySanitizesEntry mirrors the above for the raw-view
// pretty printer (different code path with its own format string).
func TestPrintRawPrettySanitizesEntry(t *testing.T) {
	view := rawView{
		JobID:  "abc",
		Status: "succeeded",
		Entries: []jobResultEntry{{
			Level:   "WARNING",
			Message: "ns \x1b[34mvuln\x1b[0m",
		}},
	}
	var out bytes.Buffer
	printRawPretty(&out, view)

	for _, b := range out.Bytes() {
		if b == '\n' {
			continue
		}
		if b < 0x20 || b == 0x7f || (b >= 0x80 && b <= 0x9f) {
			t.Fatalf("printRawPretty leaked control byte %#x: %q", b, out.String())
		}
	}
	if !strings.Contains(out.String(), `\x1b[34mvuln\x1b[0m`) {
		t.Fatalf("expected escaped ANSI in raw pretty output, got %q", out.String())
	}
}
