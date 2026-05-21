package cliterm

import "testing"

func TestSanitizePassesThroughSafeText(t *testing.T) {
	cases := []string{
		"",
		"plain ascii",
		"ns1.example.com/192.0.2.1",
		"unicode: café résumé naïve",
		"japanese: 日本語",
		"emoji: \U0001f600",
		"high-codepoint passthrough   ",
	}
	for _, in := range cases {
		if got := Sanitize(in); got != in {
			t.Fatalf("Sanitize(%q) = %q, want unchanged", in, got)
		}
	}
}

func TestSanitizeEscapesControls(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"ansi color", "\x1b[34mblue\x1b[0m", `\x1b[34mblue\x1b[0m`},
		{"carriage return overwrite", "real\rfake", `real\x0dfake`},
		{"newline injection", "line1\nline2", `line1\x0aline2`},
		{"tab", "a\tb", `a\x09b`},
		{"bell", "alert\x07", `alert\x07`},
		{"null", "x\x00y", `x\x00y`},
		{"del", "x\x7fy", `x\x7fy`},
		{"c1 control", "x\x9by", `x\x9by`},
		{"all c0 unaffected printable", "abc!~", "abc!~"},
		{"sql-like payload kept verbatim", "'; DROP TABLE x; --", "'; DROP TABLE x; --"},
		{"xss-like payload kept verbatim", "<script>alert('xss')</script>", "<script>alert('xss')</script>"},
		{"backspace", "ab\x08c", `ab\x08c`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Sanitize(tc.in); got != tc.want {
				t.Fatalf("Sanitize(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestSanitizeInvalidUTF8EscapesBareBytes(t *testing.T) {
	// Bare invalid UTF-8 bytes must be visible in the output as \xNN
	// rather than silently replaced with U+FFFD; otherwise a hostile
	// nameserver could smuggle the raw byte through (e.g. 0x9b which
	// is a bare CSI introducer in 8-bit terminals).
	cases := []struct {
		in   string
		want string
	}{
		{"\xff", `\xff`},
		{"\x9b", `\x9b`},
		{"ok\xffbad", `ok\xffbad`},
		{"\xc3", `\xc3`}, // incomplete UTF-8 lead byte
	}
	for _, tc := range cases {
		if got := Sanitize(tc.in); got != tc.want {
			t.Fatalf("Sanitize(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestSanitizeC1ControlsFromBareByteAndNEL(t *testing.T) {
	// C1 controls can reach us two ways: as a bare invalid UTF-8 byte
	// (e.g. 0x9b alone) or as the legitimate UTF-8 encoding of a C1
	// codepoint (U+0085 NEL = 0xc2 0x85). Both must be escaped.
	if got := Sanitize("\x9b"); got != `\x9b` {
		t.Fatalf("bare C1 byte: Sanitize = %q, want %q", got, `\x9b`)
	}
	if got := Sanitize(""); got != `\x85` {
		t.Fatalf("NEL codepoint: Sanitize = %q, want %q", got, `\x85`)
	}
}
