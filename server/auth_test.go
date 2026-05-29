package server

import (
	"strings"
	"testing"
)

func TestTokenSetMatch(t *testing.T) {
	tok := "gm_testtoken"
	ts, err := newTokenSet(AuthConfig{AdminTokens: []AdminToken{{Label: "x", Hash: hashToken(tok)}}})
	if err != nil {
		t.Fatal(err)
	}
	if !ts.enabled {
		t.Fatal("expected token mode enabled")
	}
	if lbl, ok := ts.match(tok); !ok || lbl != "x" {
		t.Fatalf("expected match label x, got %q %v", lbl, ok)
	}
	if _, ok := ts.match("wrong"); ok {
		t.Fatal("did not expect a match for the wrong token")
	}
}

func TestEmptyTokenSetIsOpenMode(t *testing.T) {
	ts, err := newTokenSet(AuthConfig{})
	if err != nil {
		t.Fatal(err)
	}
	if ts.enabled {
		t.Fatal("empty token set should be open mode")
	}
}

func TestNewTokenSetRejectsBadHash(t *testing.T) {
	bad := []string{"abc", "sha256:zz", "sha256:" + strings.Repeat("a", 10), "md5:" + strings.Repeat("a", 64)}
	for _, h := range bad {
		if _, err := newTokenSet(AuthConfig{AdminTokens: []AdminToken{{Hash: h}}}); err == nil {
			t.Fatalf("expected error for bad hash %q", h)
		}
	}
}

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
