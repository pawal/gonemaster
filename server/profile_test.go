package server

import (
	"os"
	"testing"

	"codeberg.org/pawal/gonemaster/engine"
	"codeberg.org/pawal/gonemaster/engine/profile"
)

func TestApplyProfileOverridesWithBase(t *testing.T) {
	base := `{"net":{"ipv4":false}}`
	baseFile, err := os.CreateTemp("", "gm-profile-*.json")
	if err != nil {
		t.Fatalf("temp file: %v", err)
	}
	if _, err := baseFile.WriteString(base); err != nil {
		_ = baseFile.Close()
		_ = os.Remove(baseFile.Name())
		t.Fatalf("write base: %v", err)
	}
	if err := baseFile.Close(); err != nil {
		_ = os.Remove(baseFile.Name())
		t.Fatalf("close base: %v", err)
	}
	defer os.Remove(baseFile.Name())

	overrides := map[string]any{
		"resolver": map[string]any{
			"defaults": map[string]any{
				"timeout": 3,
			},
		},
	}

	req := engine.RunRequest{Domain: "example.com"}
	cleanup, err := applyProfileOverrides(&req, nil, nil, overrides, baseFile.Name())
	if err != nil {
		t.Fatalf("apply overrides: %v", err)
	}
	if cleanup == nil {
		t.Fatalf("expected cleanup function")
	}
	defer cleanup()

	if req.Profile == "" {
		t.Fatalf("expected profile path set")
	}
	payload, err := os.ReadFile(req.Profile)
	if err != nil {
		t.Fatalf("read profile: %v", err)
	}
	merged, err := profile.FromYAML(string(payload))
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
	baseFile, err := os.CreateTemp("", "gm-profile-*.json")
	if err != nil {
		t.Fatalf("temp file: %v", err)
	}
	if _, err := baseFile.WriteString(base); err != nil {
		_ = baseFile.Close()
		_ = os.Remove(baseFile.Name())
		t.Fatalf("write base: %v", err)
	}
	if err := baseFile.Close(); err != nil {
		_ = os.Remove(baseFile.Name())
		t.Fatalf("close base: %v", err)
	}
	defer os.Remove(baseFile.Name())

	req := engine.RunRequest{Domain: "example.com"}
	cleanup, err := applyProfileOverrides(&req, nil, nil, nil, baseFile.Name())
	if err != nil {
		t.Fatalf("apply overrides: %v", err)
	}
	if cleanup != nil {
		t.Fatalf("expected nil cleanup for base-only profile")
	}
	if req.Profile != baseFile.Name() {
		t.Fatalf("expected profile path to be base file")
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
	cleanup, err := applyProfileOverrides(&req, store, &stored.ID, overrides, "")
	if err != nil {
		t.Fatalf("apply overrides: %v", err)
	}
	if cleanup == nil {
		t.Fatal("expected cleanup function")
	}
	defer cleanup()

	payload, err := os.ReadFile(req.Profile)
	if err != nil {
		t.Fatalf("read profile: %v", err)
	}
	merged, err := profile.FromYAML(string(payload))
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
