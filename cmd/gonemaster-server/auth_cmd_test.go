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
