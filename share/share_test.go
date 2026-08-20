package share

import (
	"strings"
	"testing"
)

func TestEmbeddedAssetsPresent(t *testing.T) {
	if NamedRoot == "" {
		t.Fatalf("expected named.root to be embedded")
	}
	if len(ProfileJSON) == 0 {
		t.Fatalf("expected profile.json to be embedded")
	}
	if IanaIPv4CSV == "" || IanaIPv6CSV == "" {
		t.Fatalf("expected IANA CSV data to be embedded")
	}
}

// gonemaster and Zonemaster are distinct projects, and the message catalogs
// were inherited from an upstream reference that named itself in its own
// result text. A leftover reference reaches real users: a DNSSEC notice once
// told them their key was unsupported "by this installation of Zonemaster",
// naming a product that is not the one they ran.
//
// The catalogs are data, so no compiler catches this. Guard every embedded
// locale at once rather than one string at a time, since the failure mode is a
// new or re-synced translation quietly reintroducing the name.
func TestNoForeignProductNameInCatalogs(t *testing.T) {
	entries, err := POFiles.ReadDir("lang")
	if err != nil {
		t.Fatalf("read lang dir: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("expected embedded translation catalogs")
	}
	for _, entry := range entries {
		data, err := POFiles.ReadFile("lang/" + entry.Name())
		if err != nil {
			t.Errorf("read %s: %v", entry.Name(), err)
			continue
		}
		for i, line := range strings.Split(string(data), "\n") {
			if strings.Contains(line, "Zonemaster") {
				t.Errorf("%s:%d names a different project in a user-facing catalog: %s", entry.Name(), i+1, line)
			}
		}
	}
}
