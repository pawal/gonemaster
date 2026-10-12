package dnssecchain

import (
	"slices"
	"testing"
	"time"

	dns "codeberg.org/miekg/dns"

	"codeberg.org/pawal/gonemaster/engine/dnssecutil"
	"codeberg.org/pawal/gonemaster/engine/internal/dnstest"
)

func TestSigStateCapsKeysPerTag(t *testing.T) {
	kp := dnstest.GenKey(t, testZone, dns.ED25519, true)
	decoys := dnstest.SameTagKeys(t, kp.Key, 5)
	rrset := []dns.RR{kp.Key}
	sig := dnstest.SignRRset(t, kp, rrset, testZone, testZone, fixedAt.Add(-time.Hour), fixedAt.Add(time.Hour))

	cases := []struct {
		name string
		at   int
		want string
	}{
		{"signing key fourth", 3, SigValid},
		{"signing key fifth", 4, SigBogus},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			keys := slices.Insert(slices.Clone(decoys), tc.at, kp.Key)
			if got := sigState(sig, rrset, keys, fixedAt); got != tc.want {
				t.Errorf("sigState = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestCapRRSIGsPerOwner(t *testing.T) {
	kp := dnstest.GenKey(t, testZone, dns.ED25519, false)
	var sigs []*dns.RRSIG
	for _, owner := range []string{"a." + testZone, "b." + testZone} {
		for range dnssecutil.MaxRRSIGsPerRRset + 2 {
			rrset := []dns.RR{dnstest.ARR(owner, "192.0.2.1")}
			sigs = append(sigs, dnstest.SignRRset(t, kp, rrset, owner, testZone, fixedAt.Add(-time.Hour), fixedAt.Add(time.Hour)))
		}
	}
	got := capRRSIGs(sigs)
	if len(got) != 2*dnssecutil.MaxRRSIGsPerRRset {
		t.Fatalf("kept %d RRSIGs, want %d", len(got), 2*dnssecutil.MaxRRSIGsPerRRset)
	}
	if got[0] != sigs[0] || got[dnssecutil.MaxRRSIGsPerRRset] != sigs[dnssecutil.MaxRRSIGsPerRRset+2] {
		t.Error("kept RRSIGs are not the first of each owner")
	}
}
