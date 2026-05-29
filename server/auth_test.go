package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func authGet(s *Server, path string, mutate func(*http.Request)) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if mutate != nil {
		mutate(req)
	}
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	return rec
}

func tokenServer(tok string) *Server {
	cfg := DefaultConfig()
	cfg.Auth.AdminTokens = []AdminToken{{Label: "t", Hash: hashToken(tok)}}
	return New(cfg)
}

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

func TestAuthMiddlewareOpenModeAllows(t *testing.T) {
	s := New(DefaultConfig())
	if rec := authGet(s, "/api/v1/locales", nil); rec.Code == http.StatusUnauthorized {
		t.Fatalf("open mode should not require auth, got %d", rec.Code)
	}
}

func TestAuthMiddlewareTokenMode(t *testing.T) {
	tok := "gm_mwtest"
	s := tokenServer(tok)

	if rec := authGet(s, "/api/v1/locales", nil); rec.Code != http.StatusUnauthorized {
		t.Fatalf("missing token: want 401, got %d", rec.Code)
	}
	bearer := func(r *http.Request) { r.Header.Set("Authorization", "Bearer "+tok) }
	if rec := authGet(s, "/api/v1/locales", bearer); rec.Code == http.StatusUnauthorized {
		t.Fatal("valid bearer should not be rejected")
	}
	cookie := func(r *http.Request) { r.AddCookie(&http.Cookie{Name: adminCookieName, Value: tok}) }
	if rec := authGet(s, "/api/v1/locales", cookie); rec.Code == http.StatusUnauthorized {
		t.Fatal("valid cookie should not be rejected")
	}
	wrong := func(r *http.Request) { r.Header.Set("Authorization", "Bearer nope") }
	if rec := authGet(s, "/api/v1/locales", wrong); rec.Code != http.StatusUnauthorized {
		t.Fatalf("wrong token: want 401, got %d", rec.Code)
	}
	if rec := authGet(s, "/api/v1/healthz", nil); rec.Code != http.StatusOK {
		t.Fatalf("healthz must stay exempt: want 200, got %d", rec.Code)
	}
	if rec := authGet(s, "/api/v1/metrics", nil); rec.Code != http.StatusUnauthorized {
		t.Fatalf("metrics must be gated: want 401, got %d", rec.Code)
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
