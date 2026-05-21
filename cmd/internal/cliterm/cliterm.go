// Package cliterm sanitizes strings that may carry attacker-controlled
// bytes (DNS responses, CHAOS version strings, NS owner names) before
// they reach a terminal or other line-oriented sink.
package cliterm

import (
	"strings"
	"unicode/utf8"
)

// Sanitize escapes C0 controls (0x00-0x1F), DEL (0x7F), C1 controls
// (0x80-0x9F) and bare invalid-UTF-8 bytes as \xNN. All other runes,
// including printable UTF-8, pass through unchanged.
//
// The intent is to defuse attacker-controlled bytes flowing out of DNS
// responses (CHAOS version.bind strings, NS owner names, TXT data) so
// they cannot inject ANSI escape sequences, overwrite previous lines
// with \r, fake newlines into log streams, or smuggle bytes through
// formats that use control characters as separators (e.g. Nagios).
func Sanitize(s string) string {
	if !needsSanitize(s) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s) + 8)
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && size == 1 {
			writeHexEscape(&b, uint32(s[i]))
			i++
			continue
		}
		if isUnsafe(r) {
			writeHexEscape(&b, uint32(r))
			i += size
			continue
		}
		b.WriteRune(r)
		i += size
	}
	return b.String()
}

func needsSanitize(s string) bool {
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && size == 1 {
			return true
		}
		if isUnsafe(r) {
			return true
		}
		i += size
	}
	return false
}

func isUnsafe(r rune) bool {
	switch {
	case r < 0x20:
		return true
	case r == 0x7f:
		return true
	case r >= 0x80 && r <= 0x9f:
		return true
	}
	return false
}

const hexDigits = "0123456789abcdef"

// writeHexEscape appends `\xNN` for any value that fits in a byte. The
// caller guarantees v <= 0xff because the only callers are the invalid
// byte path and isUnsafe (whose range tops out at 0x9f).
func writeHexEscape(b *strings.Builder, v uint32) {
	b.WriteByte('\\')
	b.WriteByte('x')
	b.WriteByte(hexDigits[(v>>4)&0xf])
	b.WriteByte(hexDigits[v&0xf])
}
