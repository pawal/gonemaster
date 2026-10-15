package dnsname

import "testing"

func TestEscape(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"ns1.example.", "ns1.example."},
		{"a\x1b[31mb.example", `a\027[31mb.example`},
		{"a b.example", `a\032b.example`},
		{"caf\xc3\xa9.example", `caf\195\169.example`},
		{"\x00\x7f", `\000\127`},
		{`a\.b.example`, `a\.b.example`},
	} {
		if got := Escape(tc.in); got != tc.want {
			t.Errorf("Escape(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
