package main

import (
	"strings"
	"testing"

	"codeberg.org/pawal/gonemaster/server"
)

func TestGenerateTokenFormat(t *testing.T) {
	a, err := generateToken()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(a, "gm_") {
		t.Fatalf("token should be gm_-prefixed: %s", a)
	}
	if b, _ := generateToken(); a == b {
		t.Fatal("generated tokens should be unique")
	}
}

func TestGeneratedTokenHashValidates(t *testing.T) {
	tok, err := generateToken()
	if err != nil {
		t.Fatal(err)
	}
	cfg := server.AuthConfig{AdminTokens: []server.AdminToken{{Hash: server.HashToken(tok)}}}
	if err := server.ValidateAuthConfig(cfg); err != nil {
		t.Fatalf("minted token hash should validate: %v", err)
	}
}

func TestParseAdminTokenHashes(t *testing.T) {
	toks := parseAdminTokenHashes("ci=sha256:aa, sha256:bb ,")
	if len(toks) != 2 {
		t.Fatalf("want 2 tokens, got %d", len(toks))
	}
	if toks[0].Label != "ci" || toks[0].Hash != "sha256:aa" {
		t.Fatalf("first token wrong: %+v", toks[0])
	}
	if toks[1].Label != "" || toks[1].Hash != "sha256:bb" {
		t.Fatalf("second token wrong: %+v", toks[1])
	}
}

func TestResolveAuthConfigEnvAndFlag(t *testing.T) {
	full := server.HashToken("gm_x")
	auth, err := resolveAuthConfig("", "lbl="+full, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(auth.AdminTokens) != 1 || auth.AdminTokens[0].Label != "lbl" {
		t.Fatalf("env not applied: %+v", auth)
	}
	auth, err = resolveAuthConfig("", "lbl="+full, server.HashToken("gm_y"))
	if err != nil {
		t.Fatal(err)
	}
	if len(auth.AdminTokens) != 1 || auth.AdminTokens[0].Label != "" {
		t.Fatalf("flag should override env: %+v", auth)
	}
}
