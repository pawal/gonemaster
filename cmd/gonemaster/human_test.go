package main

import (
	"bytes"
	"strings"
	"testing"

	"codeberg.org/pawal/gonemaster/engine"
	"codeberg.org/pawal/gonemaster/engine/logger"
)

// TestWriteHumanSanitizesControlCharsInArgs feeds a DNS-data-style arg
// containing ANSI colour codes, NUL and CR through writeHuman and asserts
// the terminal-bound output cannot be hijacked by the attacker bytes.
//
// We deliberately use an unregistered tag so i18n translation fails and
// writeHuman falls back to rawEntryString, which formats Module:Tag plus
// the args inline. That keeps the attacker bytes in the rendered message.
func TestWriteHumanSanitizesControlCharsInArgs(t *testing.T) {
	entries := []engine.LogEntry{{
		Module:    "MOD",
		Testcase:  "TC01",
		Tag:       "UNREGISTERED_TAG_FOR_HUMAN_TEST",
		Level:     "WARNING",
		Timestamp: 1.5,
		Args: map[string]any{
			"version_string": "\x1b[34mblue\x1b[0m",
			"payload":        "line1\rfake\nline2",
		},
	}}

	var out bytes.Buffer
	if err := writeHuman(entries, "en", &out); err != nil {
		t.Fatalf("writeHuman: %v", err)
	}

	body := out.Bytes()

	// Header line + divider + one entry line = exactly three newlines.
	// An attacker-injected raw \n would make four.
	if n := bytes.Count(body, []byte{'\n'}); n != 3 {
		t.Fatalf("expected 3 newlines (header+divider+entry), got %d in %q", n, body)
	}
	for i, b := range body {
		if b == '\n' || b == ' ' || b == '=' {
			continue
		}
		if b < 0x20 || b == 0x7f || (b >= 0x80 && b <= 0x9f) {
			t.Fatalf("human output leaked control byte %#x at offset %d: %q", b, i, body)
		}
	}
	for _, want := range []string{
		`\x1b[34mblue\x1b[0m`,
		`\x0d`, // CR escape
		`\x0a`, // LF escape inside the message body
	} {
		if !strings.Contains(string(body), want) {
			t.Fatalf("expected %q in human output, got %q", want, body)
		}
	}
}

// TestHumanReporterCallbackSanitizesOutput covers the streaming path used
// when entries are emitted live via a logger.Entry rather than collected
// up front. The Callback variant has its own translation call and must
// also sanitize.
func TestHumanReporterCallbackSanitizesOutput(t *testing.T) {
	var out bytes.Buffer
	reporter := newHumanReporter(&out, "en", "", false)
	if reporter == nil {
		t.Fatal("newHumanReporter returned nil")
	}

	entry, err := logger.NewEntry(
		"UNREGISTERED_STREAM_TAG",
		map[string]any{"version_string": "evil\x1b[31mxss\x1b[0m"},
		"TC01", "MOD",
	)
	if err != nil {
		t.Fatalf("NewEntry: %v", err)
	}
	if err := reporter.Callback(entry); err != nil {
		t.Fatalf("Callback: %v", err)
	}

	body := out.Bytes()
	for _, b := range body {
		if b == '\n' || b == ' ' || b == '=' {
			continue
		}
		if b < 0x20 || b == 0x7f || (b >= 0x80 && b <= 0x9f) {
			t.Fatalf("human reporter leaked control byte %#x: %q", b, body)
		}
	}
	if !strings.Contains(string(body), `evil\x1b[31mxss\x1b[0m`) {
		t.Fatalf("expected escaped ANSI in streaming output, got %q", body)
	}
}

// TestWriteHumanPreservesSafeUnicode is the negative case: a well-formed
// UTF-8 message must not get mangled by the sanitizer.
func TestWriteHumanPreservesSafeUnicode(t *testing.T) {
	entries := []engine.LogEntry{{
		Module:    "MOD",
		Testcase:  "TC01",
		Tag:       "UNREGISTERED_BENIGN_TAG",
		Level:     "INFO",
		Timestamp: 0.5,
		Args:      map[string]any{"label": "café résumé naïve"},
	}}

	var out bytes.Buffer
	if err := writeHuman(entries, "en", &out); err != nil {
		t.Fatalf("writeHuman: %v", err)
	}
	if !strings.Contains(out.String(), "café résumé naïve") {
		t.Fatalf("safe UTF-8 was altered: %q", out.String())
	}
	if strings.Contains(out.String(), `\x`) {
		t.Fatalf("benign input produced escapes: %q", out.String())
	}
}
