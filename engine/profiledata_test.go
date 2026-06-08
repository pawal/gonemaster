package engine

import "testing"

// TestEffectiveProfileUsesProfileData verifies that inline profile content on
// the request is merged into the effective profile without any filesystem
// read, which is the path the server uses for stored/overridden profiles.
func TestEffectiveProfileUsesProfileData(t *testing.T) {
	req := RunRequest{
		Domain:      "example.com",
		ProfileData: `{"resolver":{"defaults":{"timeout":11}}}`,
	}
	p, err := EffectiveProfile(req)
	if err != nil {
		t.Fatalf("EffectiveProfile: %v", err)
	}
	timeout, err := p.Get("resolver.defaults.timeout")
	if err != nil {
		t.Fatalf("get resolver.defaults.timeout: %v", err)
	}
	if value, ok := timeout.(int); !ok || value != 11 {
		t.Fatalf("expected timeout 11, got %v", timeout)
	}
}

// TestProfileDataTakesPrecedenceOverProfile verifies ProfileData wins over the
// Profile file path. A non-existent path must not be read when ProfileData is
// set, so a bogus path here proves the file is never opened.
func TestProfileDataTakesPrecedenceOverProfile(t *testing.T) {
	req := RunRequest{
		Domain:      "example.com",
		Profile:     "/nonexistent/should-not-be-read.json",
		ProfileData: `{"resolver":{"defaults":{"timeout":13}}}`,
	}
	p, err := EffectiveProfile(req)
	if err != nil {
		t.Fatalf("EffectiveProfile: %v", err)
	}
	timeout, err := p.Get("resolver.defaults.timeout")
	if err != nil {
		t.Fatalf("get resolver.defaults.timeout: %v", err)
	}
	if value, ok := timeout.(int); !ok || value != 13 {
		t.Fatalf("expected timeout 13, got %v", timeout)
	}
}

// TestPlannedTestcasesUsesProfileData verifies the test planner honors the
// inline profile's test_cases selection, matching the engine's run path.
func TestPlannedTestcasesUsesProfileData(t *testing.T) {
	req := RunRequest{
		Domain:      "example.com",
		ProfileData: `{"test_cases":["basic02"]}`,
	}
	planned, err := PlannedTestcases(req)
	if err != nil {
		t.Fatalf("PlannedTestcases: %v", err)
	}
	if len(planned) != 1 || planned[0] != "basic02" {
		t.Fatalf("expected [basic02], got %v", planned)
	}
}
