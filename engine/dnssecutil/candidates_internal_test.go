package dnssecutil

import (
	"fmt"
	"testing"

	dns "codeberg.org/miekg/dns"
)

func TestLabelsContained(t *testing.T) {
	tests := []struct {
		owner, signer string
		labels        uint8
		want          bool
	}{
		{"a.c.example.", "c.example.", 3, true},
		{"a.c.example.", "c.example.", 2, true},
		{"a.b.c.example.", "c.example.", 2, true},
		{"a.c.example.", "example.", 1, true},
		{"a.c.example.", "c.example.", 1, false},
		{"a.c.example.", "c.example.", 0, false},
		{"a.c.example.", "c.example.", 4, false},
		{"*.c.example.", "c.example.", 2, true},
		{"*.example.", "*.example.", 1, true},
		{"x.*.example.", "*.example.", 1, false},
		{"x.*.example.", "*.example.", 2, true},
		{".", ".", 0, true},
		{"host.", ".", 0, true},
		{"child.example.", "example.", 2, true},
		{"A.C.Example.", "c.EXAMPLE.", 2, true},
		{"a.bc.example.", "c.example.", 3, false},
		{"a.other.", "c.example.", 2, false},
		{"a.c.example", "c.example.", 3, false},
	}
	for _, tc := range tests {
		t.Run(fmt.Sprintf("%s %s %d", tc.owner, tc.signer, tc.labels), func(t *testing.T) {
			sig := &dns.RRSIG{SignerName: tc.signer, Labels: tc.labels}
			if got := labelsContained(sig, tc.owner); got != tc.want {
				t.Errorf("labelsContained = %t, want %t", got, tc.want)
			}
		})
	}
}
