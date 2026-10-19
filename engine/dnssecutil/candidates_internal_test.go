package dnssecutil

import (
	"errors"
	"fmt"
	"testing"
	"time"

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

// rrsigLabelsTime lies inside the captured RRSIG windows, 2026-10-17 to 2027-01-16.
func rrsigLabelsTime() time.Time { return time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC) }

func TestVerifyCandidatesCapturedLabelsZones(t *testing.T) {
	parent, _, _ := loadZoneFixture(t, "rrsig-labels-parent.zone")
	tests := []struct {
		zone    string
		covered uint16
		labels  uint8
		want    error
	}{
		{"valid", dns.TypeDS, 4, nil},
		{"valid", dns.TypeDNSKEY, 4, nil},
		{"valid", dns.TypeSOA, 4, nil},
		{"valid", dns.TypeNS, 4, nil},
		{"star.apex", dns.TypeDS, 4, nil},
		{"star.apex", dns.TypeDNSKEY, 4, nil},
		{"star.apex", dns.TypeSOA, 4, nil},
		{"star.apex", dns.TypeNS, 4, nil},
		{"underflow", dns.TypeDS, 4, nil},
		{"underflow", dns.TypeDNSKEY, 4, nil},
		{"underflow", dns.TypeSOA, 3, ErrLabels},
		{"underflow", dns.TypeNS, 4, nil},
		{"mixed", dns.TypeDS, 4, nil},
		{"mixed", dns.TypeSOA, 4, nil},
		{"mixed", dns.TypeSOA, 3, ErrLabels},
		{"badcrypto", dns.TypeDS, 4, nil},
		{"badcrypto", dns.TypeDNSKEY, 4, nil},
		{"badcrypto", dns.TypeSOA, 4, dns.ErrSig},
		{"badcrypto", dns.TypeNS, 4, nil},
	}
	for _, tc := range tests {
		t.Run(fmt.Sprintf("%s %s %d", tc.zone, dns.TypeToString[tc.covered], tc.labels), func(t *testing.T) {
			keys, sigs, rest := loadZoneFixture(t, "rrsig-labels-"+tc.zone+".zone")
			var sig *dns.RRSIG
			for _, s := range sigs {
				if s.TypeCovered == tc.covered && s.Labels == tc.labels {
					sig = s
				}
			}
			if sig == nil {
				t.Fatalf("no %s RRSIG with Labels %d in fixture", dns.TypeToString[tc.covered], tc.labels)
			}
			signing := keys
			if tc.covered == dns.TypeDS {
				signing = parent
			}
			rrset := asRRset(keys)
			if tc.covered != dns.TypeDNSKEY {
				rrset = nil
				for _, rr := range rest {
					if dns.RRToType(rr) == tc.covered {
						rrset = append(rrset, rr)
					}
				}
			}

			key, errs := VerifyCandidates(sig, rrset, []*dns.DNSKEY{keyForTag(t, signing, sig.KeyTag)}, rrsigLabelsTime())
			if tc.want == nil {
				if key == nil || len(errs) != 0 {
					t.Fatalf("key = %v, errors = %v; want a key and no error", key, errs)
				}
				return
			}
			if key != nil || len(errs) != 1 || !errors.Is(errs[0], tc.want) {
				t.Fatalf("key = %v, errors = %v; want nil and one %v", key, errs, tc.want)
			}
		})
	}
}
