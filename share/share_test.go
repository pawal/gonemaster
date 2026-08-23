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

// A notice once told users their key was unsupported "by this installation of
// Zonemaster". The catalogs are data, so nothing else catches a re-synced one.
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
