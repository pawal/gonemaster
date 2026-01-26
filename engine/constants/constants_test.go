package constants

import "testing"

func TestSpecialAddressListsLoaded(t *testing.T) {
	if len(IPv4SpecialAddresses) == 0 {
		t.Fatalf("expected IPv4 special addresses to be loaded")
	}
	if len(IPv6SpecialAddresses) == 0 {
		t.Fatalf("expected IPv6 special addresses to be loaded")
	}

	for _, block := range IPv4SpecialAddresses {
		if !block.Prefix.IsValid() {
			t.Fatalf("invalid IPv4 prefix: %v", block.Prefix)
		}
		if block.Name == "" {
			t.Fatalf("missing IPv4 block name")
		}
		break
	}
	for _, block := range IPv6SpecialAddresses {
		if !block.Prefix.IsValid() {
			t.Fatalf("invalid IPv6 prefix: %v", block.Prefix)
		}
		if block.Name == "" {
			t.Fatalf("missing IPv6 block name")
		}
		break
	}
}
