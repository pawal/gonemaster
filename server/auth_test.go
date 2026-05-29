package server

import "testing"

func TestDefaultConfigOpenMode(t *testing.T) {
	if len(DefaultConfig().Auth.AdminTokens) != 0 {
		t.Fatalf("default config should be open mode (no admin tokens)")
	}
}

func TestApplyFileConfigAuth(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ApplyFileConfig(FileConfig{Auth: &AuthConfig{
		AdminTokens: []AdminToken{{Label: "ci", Hash: "sha256:abc"}},
	}})
	if len(cfg.Auth.AdminTokens) != 1 || cfg.Auth.AdminTokens[0].Label != "ci" {
		t.Fatalf("ApplyFileConfig did not apply auth block: %+v", cfg.Auth)
	}
}
