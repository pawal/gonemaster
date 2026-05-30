package server

import (
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"testing"
)

func authGet(s *Server, path string, mutate func(*http.Request)) *httptest.ResponseRecorder {
	return authDo(s, http.MethodGet, path, "", mutate)
}

func authDo(s *Server, method, path, body string, mutate func(*http.Request)) *httptest.ResponseRecorder {
	var r io.Reader
	if body != "" {
		r = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, r)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if mutate != nil {
		mutate(req)
	}
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	return rec
}

func cookieNamed(rec *httptest.ResponseRecorder, name string) *http.Cookie {
	for _, c := range rec.Result().Cookies() {
		if c.Name == name {
			return c
		}
	}
	return nil
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

func TestWhoamiOpenMode(t *testing.T) {
	s := New(DefaultConfig())
	rec := authGet(s, "/api/v1/whoami", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"mode":"open"`) || !strings.Contains(rec.Body.String(), `"authenticated":true`) {
		t.Fatalf("unexpected whoami body: %s", rec.Body.String())
	}
}

func TestWhoamiTokenMode(t *testing.T) {
	tok := "gm_whoami"
	s := tokenServer(tok)
	rec := authGet(s, "/api/v1/whoami", nil)
	if !strings.Contains(rec.Body.String(), `"mode":"token"`) || !strings.Contains(rec.Body.String(), `"authenticated":false`) {
		t.Fatalf("unauthed whoami: %s", rec.Body.String())
	}
	rec = authGet(s, "/api/v1/whoami", func(r *http.Request) { r.Header.Set("Authorization", "Bearer "+tok) })
	if !strings.Contains(rec.Body.String(), `"authenticated":true`) {
		t.Fatalf("authed whoami: %s", rec.Body.String())
	}
}

func TestSessionLoginAndUseCookie(t *testing.T) {
	tok := "gm_session"
	s := tokenServer(tok)
	rec := authDo(s, http.MethodPost, "/api/v1/session", `{"token":"`+tok+`"}`, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("login want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	cookie := cookieNamed(rec, adminCookieName)
	if cookie == nil {
		t.Fatal("login did not set admin cookie")
	}
	if !cookie.HttpOnly || cookie.SameSite != http.SameSiteStrictMode {
		t.Fatalf("cookie flags: HttpOnly=%v SameSite=%v", cookie.HttpOnly, cookie.SameSite)
	}
	rec = authGet(s, "/api/v1/locales", func(r *http.Request) { r.AddCookie(cookie) })
	if rec.Code == http.StatusUnauthorized {
		t.Fatal("cookie from login should authenticate gated requests")
	}
}

func TestSessionLoginRejectsBadToken(t *testing.T) {
	s := tokenServer("gm_real")
	rec := authDo(s, http.MethodPost, "/api/v1/session", `{"token":"wrong"}`, nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", rec.Code)
	}
	if cookieNamed(rec, adminCookieName) != nil {
		t.Fatal("no cookie should be set on a failed login")
	}
}

func TestSessionLogoutClearsCookie(t *testing.T) {
	s := tokenServer("gm_logout")
	rec := authDo(s, http.MethodDelete, "/api/v1/session", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("logout want 200, got %d", rec.Code)
	}
	c := cookieNamed(rec, adminCookieName)
	if c == nil || c.MaxAge >= 0 {
		t.Fatalf("logout should clear the admin cookie, got %+v", c)
	}
}

// TestAuthEndToEndOverHTTP drives the auth flow over a real loopback socket
// with a real HTTP client and cookie jar, mirroring the operator curl smoke.
func TestAuthEndToEndOverHTTP(t *testing.T) {
	tok := "gm_e2e"
	ts := httptest.NewServer(tokenServer(tok).Handler())
	defer ts.Close()

	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	code := func(method, path string, header http.Header, body string) int {
		var r io.Reader
		if body != "" {
			r = strings.NewReader(body)
		}
		req, err := http.NewRequest(method, ts.URL+path, r)
		if err != nil {
			t.Fatal(err)
		}
		for k, vs := range header {
			for _, v := range vs {
				req.Header.Add(k, v)
			}
		}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}

	if got := code(http.MethodGet, "/api/v1/locales", nil, ""); got != http.StatusUnauthorized {
		t.Fatalf("no token: want 401, got %d", got)
	}
	if got := code(http.MethodGet, "/api/v1/healthz", nil, ""); got != http.StatusOK {
		t.Fatalf("healthz exempt: want 200, got %d", got)
	}
	bearer := http.Header{"Authorization": []string{"Bearer " + tok}}
	if got := code(http.MethodGet, "/api/v1/locales", bearer, ""); got != http.StatusOK {
		t.Fatalf("bearer: want 200, got %d", got)
	}
	cType := http.Header{"Content-Type": []string{"application/json"}}
	if got := code(http.MethodPost, "/api/v1/session", cType, `{"token":"`+tok+`"}`); got != http.StatusOK {
		t.Fatalf("session login: want 200, got %d", got)
	}
	// The jar now holds the session cookie; a bare request must authenticate.
	if got := code(http.MethodGet, "/api/v1/locales", nil, ""); got != http.StatusOK {
		t.Fatalf("cookie from jar: want 200, got %d", got)
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
