package normalization

import (
	"strings"
	"testing"
)

func TestNormalizeLabelASCII(t *testing.T) {
	errs, label := NormalizeLabel("WWW")
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if label != "www" {
		t.Fatalf("expected lowercase label, got %q", label)
	}
}

func TestNormalizeNameCases(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantTag  string
		wantName string
	}{
		{"empty", "", "EMPTY_DOMAIN_NAME", ""},
		{"initial-dot", ".example", "INITIAL_DOT", ""},
		{"repeated-dots", "example..com", "REPEATED_DOTS", ""},
		{"invalid-ascii", "bad!label.com", "INVALID_ASCII", ""},
		{"label-too-long", strings.Repeat("a", 64) + ".com", "LABEL_TOO_LONG", ""},
		{"ambiguous", "example\u0130.com", "AMBIGUOUS_DOWNCASING", ""},
		{"root", ".", "", "."},
		{"fullwidth-dot", "example\uFF0Ecom", "", "example.com"},
		{"idn", "r\u00e4ksm\u00f6rg\u00e5s.se", "", "xn--rksmrgs-5wao1o.se"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			errs, out := NormalizeName(tc.input)
			if tc.wantTag != "" {
				if len(errs) == 0 {
					t.Fatalf("expected error %q", tc.wantTag)
				}
				if errs[0].Tag != tc.wantTag {
					t.Fatalf("expected tag %q, got %q", tc.wantTag, errs[0].Tag)
				}
				return
			}
			if len(errs) != 0 {
				t.Fatalf("unexpected errors: %v", errs)
			}
			if out != tc.wantName {
				t.Fatalf("expected %q, got %q", tc.wantName, out)
			}
		})
	}
}

func TestTrimSpace(t *testing.T) {
	input := "\u00A0example.com\u2003"
	out := TrimSpace(input)
	if out != "example.com" {
		t.Fatalf("unexpected trim result: %q", out)
	}
}
