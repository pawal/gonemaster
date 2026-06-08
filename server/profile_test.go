package server

import (
	"os"
	"testing"

	"codeberg.org/pawal/gonemaster/engine"
	"codeberg.org/pawal/gonemaster/engine/profile"
)

func TestApplyProfileOverridesWithBase(t *testing.T) {
	base := `{"net":{"ipv4":false}}`
	baseFile, err := os.CreateTemp(t.TempDir(), "gm-profile-*.json")
	if err != nil {
		t.Fatalf("temp file: %v", err)
	}
	if _, err := baseFile.WriteString(base); err != nil {
		t.Fatalf("write base: %v", err)
	}
	if err := baseFile.Close(); err != nil {
		t.Fatalf("close base: %v", err)
	}

	overrides := map[string]any{
		"resolver": map[string]any{
			"defaults": map[string]any{
				"timeout": 3,
			},
		},
	}

	req := engine.RunRequest{Domain: "example.com"}
	if err := applyProfileOverrides(&req, nil, nil, overrides, baseFile.Name()); err != nil {
		t.Fatalf("apply overrides: %v", err)
	}

	// The merged profile must be carried inline; the engine should never be
	// pointed at a temp file (which breaks on read-only filesystems).
	if req.ProfileData == "" {
		t.Fatalf("expected inline profile data to be set")
	}
	if req.Profile != "" {
		t.Fatalf("expected no profile file path, got %q", req.Profile)
	}
	merged, err := profile.FromYAML(req.ProfileData)
	if err != nil {
		t.Fatalf("parse merged: %v", err)
	}
	ipv4, err := merged.Get("net.ipv4")
	if err != nil {
		t.Fatalf("get net.ipv4: %v", err)
	}
	if value, ok := ipv4.(bool); !ok || value != false {
		t.Fatalf("expected net.ipv4 false, got %v", ipv4)
	}
	timeout, err := merged.Get("resolver.defaults.timeout")
	if err != nil {
		t.Fatalf("get resolver.defaults.timeout: %v", err)
	}
	if value, ok := timeout.(int); !ok || value != 3 {
		t.Fatalf("expected timeout 3, got %v", timeout)
	}
}

func TestApplyProfileOverridesBaseOnly(t *testing.T) {
	base := `{"net":{"ipv6":false}}`
	baseFile, err := os.CreateTemp(t.TempDir(), "gm-profile-*.json")
	if err != nil {
		t.Fatalf("temp file: %v", err)
	}
	if _, err := baseFile.WriteString(base); err != nil {
		t.Fatalf("write base: %v", err)
	}
	if err := baseFile.Close(); err != nil {
		t.Fatalf("close base: %v", err)
	}

	// With no stored profile and no overrides the base profile is passed
	// through by path (read-only access is fine) and nothing is merged.
	req := engine.RunRequest{Domain: "example.com"}
	if err := applyProfileOverrides(&req, nil, nil, nil, baseFile.Name()); err != nil {
		t.Fatalf("apply overrides: %v", err)
	}
	if req.Profile != baseFile.Name() {
		t.Fatalf("expected profile path to be base file, got %q", req.Profile)
	}
	if req.ProfileData != "" {
		t.Fatalf("expected no inline profile data for base-only profile")
	}
}

func TestApplyProfileOverridesWithStoredProfileAndOverrides(t *testing.T) {
	store := NewInMemoryJobStore()
	stored, err := store.CreateProfile(StoredProfile{
		Name:   "strict",
		Config: `{"net":{"ipv4":false},"resolver":{"defaults":{"timeout":5}}}`,
	})
	if err != nil {
		t.Fatalf("CreateProfile: %v", err)
	}

	overrides := map[string]any{
		"resolver": map[string]any{
			"defaults": map[string]any{
				"timeout": 7,
			},
		},
	}

	req := engine.RunRequest{Domain: "example.com"}
	if err := applyProfileOverrides(&req, store, &stored.ID, overrides, ""); err != nil {
		t.Fatalf("apply overrides: %v", err)
	}

	// The stored DB profile plus overrides are merged in memory; no file.
	if req.ProfileData == "" {
		t.Fatalf("expected inline profile data to be set")
	}
	if req.Profile != "" {
		t.Fatalf("expected no profile file path, got %q", req.Profile)
	}
	merged, err := profile.FromYAML(req.ProfileData)
	if err != nil {
		t.Fatalf("parse merged: %v", err)
	}
	ipv4, err := merged.Get("net.ipv4")
	if err != nil {
		t.Fatalf("get net.ipv4: %v", err)
	}
	if value, ok := ipv4.(bool); !ok || value != false {
		t.Fatalf("expected net.ipv4 false, got %v", ipv4)
	}
	timeout, err := merged.Get("resolver.defaults.timeout")
	if err != nil {
		t.Fatalf("get resolver.defaults.timeout: %v", err)
	}
	if value, ok := timeout.(int); !ok || value != 7 {
		t.Fatalf("expected timeout 7, got %v", timeout)
	}
}
