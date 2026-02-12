package profile

import "testing"

func TestDefaultProfileLoads(t *testing.T) {
	p, err := Default()
	if err != nil {
		t.Fatalf("default profile: %v", err)
	}
	if !p.Net.IPv4 || !p.Net.IPv6 {
		t.Fatalf("expected IPv4/IPv6 enabled by default")
	}
	if p.Resolver.Defaults.Timeout != 5 {
		t.Fatalf("unexpected default timeout: %d", p.Resolver.Defaults.Timeout)
	}
	if p.Resolver.Defaults.AdaptiveTimeout {
		t.Fatalf("expected adaptive timeout disabled by default")
	}
	if p.TestLevels == nil {
		t.Fatalf("expected test levels to be loaded")
	}
	if p.TestCases == nil {
		t.Fatalf("expected test cases to be loaded")
	}
}

func TestResetEffective(t *testing.T) {
	Effective().Net.IPv4 = false
	ResetEffective()
	if !Effective().Net.IPv4 {
		t.Fatalf("expected ResetEffective to restore defaults")
	}
}
