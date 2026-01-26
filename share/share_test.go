package share

import "testing"

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
