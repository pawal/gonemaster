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

func TestServerAuthModeOpenByDefault(t *testing.T) {
	s := New(DefaultConfig())
	if mode, n := s.authMode(); mode != "open" || n != 0 {
		t.Fatalf("expected open mode, got %q n=%d", mode, n)
	}
}

func TestServerReloadAuth(t *testing.T) {
	s := New(DefaultConfig())
	tok := "gm_reloadtest"
	if err := s.ReloadAuth(AuthConfig{AdminTokens: []AdminToken{{Label: "a", Hash: hashToken(tok)}}}); err != nil {
		t.Fatal(err)
	}
	if mode, n := s.authMode(); mode != "token" || n != 1 {
		t.Fatalf("expected token mode 1, got %q n=%d", mode, n)
	}
	if _, ok := s.authTokens().match(tok); !ok {
		t.Fatal("reloaded token should match")
	}
	if err := s.ReloadAuth(AuthConfig{}); err != nil {
		t.Fatal(err)
	}
	if mode, _ := s.authMode(); mode != "open" {
		t.Fatalf("expected open mode after clearing tokens, got %q", mode)
	}
}

func TestReloadAuthRejectsBadHashKeepsPrevious(t *testing.T) {
	s := New(DefaultConfig())
	good := "gm_keepme"
	if err := s.ReloadAuth(AuthConfig{AdminTokens: []AdminToken{{Hash: hashToken(good)}}}); err != nil {
		t.Fatal(err)
	}
	if err := s.ReloadAuth(AuthConfig{AdminTokens: []AdminToken{{Hash: "bogus"}}}); err == nil {
		t.Fatal("expected error reloading bad hash")
	}
	if _, ok := s.authTokens().match(good); !ok {
		t.Fatal("previous token set should survive a failed reload")
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
