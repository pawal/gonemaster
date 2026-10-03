package server

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"codeberg.org/pawal/gonemaster/engine"
	serverpublic "codeberg.org/pawal/gonemaster/server/public"
)

const protectTok = "gm_protecttest"

func protectedServer(t *testing.T) *Server {
	t.Helper()
	return newTestServer(t, withAuth(protectTok), withConfig(func(c *Config) { c.Auth.ProtectPublic = true }))
}

// loginPost marks a request as the token form posted from its own origin.
func loginPost() []reqOpt {
	return []reqOpt{withHeader("Content-Type", "application/x-www-form-urlencoded"), sameOrigin()}
}

func formBody(token string) string {
	return url.Values{"token": {token}}.Encode()
}

func isLoginPage(body string) bool {
	return strings.Contains(body, `<form class="login-card" method="post">`) && strings.Contains(body, `name="token"`)
}

func TestPublicSurfacesOpenWhenProtectOff(t *testing.T) {
	srv := newTestServer(t, withAuth(protectTok))
	for _, path := range []string{"/pub/api/v1/version", "/public/", "/analysis/"} {
		t.Run(path, func(t *testing.T) {
			wantStatus(t, doJSON(t, srv, http.MethodGet, path, nil), http.StatusOK)
		})
	}
}

func TestProtectRequiresTokens(t *testing.T) {
	err := ValidateAuthConfig(AuthConfig{ProtectPublic: true})
	if err == nil || !strings.Contains(err.Error(), "protect_public requires admin_tokens") {
		t.Fatalf("err = %v, want protect_public requires admin_tokens", err)
	}
}

func TestProtectedPagesServeLoginForm(t *testing.T) {
	srv := protectedServer(t)
	for _, path := range []string{"/public/", "/public/result/abc123def456", "/analysis/", "/analysis/tags"} {
		t.Run(path, func(t *testing.T) {
			rr := doJSON(t, srv, http.MethodGet, path, nil)
			wantStatus(t, rr, http.StatusUnauthorized)
			if !isLoginPage(rr.Body.String()) {
				t.Errorf("body is not the login page: %s", rr.Body.String())
			}
			if got := rr.Header().Get("Cache-Control"); got != "no-store" {
				t.Errorf("Cache-Control = %q, want no-store", got)
			}
			if got := rr.Header().Get("WWW-Authenticate"); got != "Bearer" {
				t.Errorf("WWW-Authenticate = %q, want Bearer", got)
			}
		})
	}
}

func TestProtectedResultPageHidesSummary(t *testing.T) {
	srv := protectedServer(t)
	job := seedPublicJob(t, srv, JobSucceeded, []engine.LogEntry{
		{Module: "ZONE", Testcase: "zone01", Tag: "Z_RETRY_MINIMUM_VALUE_LOWER", Level: "WARNING"},
	})

	rr := doJSON(t, srv, http.MethodGet, "/public/result/"+job.PublicID, nil)

	wantStatus(t, rr, http.StatusUnauthorized)
	if strings.Contains(rr.Body.String(), "example.com") {
		t.Error("login page names the tested domain")
	}
	if !serverpublic.IsBuilt() {
		return
	}
	rr = doJSON(t, srv, http.MethodGet, "/public/result/"+job.PublicID, nil, withBearer(protectTok))
	wantStatus(t, rr, http.StatusOK)
	if !strings.Contains(rr.Body.String(), "example.com") {
		t.Error("authenticated result page does not name the tested domain")
	}
}

func TestProtectedHeadLoginHasNoBody(t *testing.T) {
	srv := protectedServer(t)
	rr := doJSON(t, srv, http.MethodHead, "/public/", nil)
	wantStatus(t, rr, http.StatusUnauthorized)
	if rr.Body.Len() != 0 {
		t.Errorf("HEAD body length = %d, want 0", rr.Body.Len())
	}
}

func TestProtectedAssetIs401(t *testing.T) {
	srv := protectedServer(t)
	for _, path := range []string{"/public/assets/index.js", "/analysis/_app/immutable/start.js"} {
		t.Run(path, func(t *testing.T) {
			rr := doJSON(t, srv, http.MethodGet, path, nil)
			wantStatus(t, rr, http.StatusUnauthorized)
			if isLoginPage(rr.Body.String()) {
				t.Error("login page served to an asset request")
			}
		})
	}
}

func TestProtectedLoginStylesheet(t *testing.T) {
	srv := protectedServer(t)
	for _, path := range []string{"/public/_auth/login.css", "/analysis/_auth/login.css"} {
		rr := doJSON(t, srv, http.MethodGet, path, nil)
		wantStatus(t, rr, http.StatusOK)
		if got := rr.Header().Get("Content-Type"); got != "text/css; charset=utf-8" {
			t.Errorf("GET %s: Content-Type = %q", path, got)
		}
		if !strings.Contains(rr.Body.String(), ".login-card") {
			t.Errorf("GET %s: body is not the login stylesheet", path)
		}
	}
}

func TestProtectedLoginLinksStylesheetUnderItsPrefix(t *testing.T) {
	srv := protectedServer(t)
	for prefix, path := range map[string]string{"/public": "/public/", "/analysis": "/analysis/"} {
		rr := doJSON(t, srv, http.MethodGet, path, nil)
		want := `href="` + prefix + `/_auth/login.css"`
		if !strings.Contains(rr.Body.String(), want) {
			t.Errorf("GET %s: body lacks %s", path, want)
		}
	}
}

func TestProtectedPublicAPI(t *testing.T) {
	srv := protectedServer(t)

	rr := doJSON(t, srv, http.MethodGet, "/pub/api/v1/version", nil)
	if got := mustJSON[ErrorResponse](t, rr, http.StatusUnauthorized); got.Error.Message != "admin token required" {
		t.Errorf("no token: message = %q", got.Error.Message)
	}
	if got := rr.Header().Get("WWW-Authenticate"); got != "Bearer" {
		t.Errorf("WWW-Authenticate = %q, want Bearer", got)
	}

	rr = doJSON(t, srv, http.MethodGet, "/pub/api/v1/version", nil, withBearer("gm_wrong"))
	if got := mustJSON[ErrorResponse](t, rr, http.StatusUnauthorized); got.Error.Message != "invalid admin token" {
		t.Errorf("wrong token: message = %q", got.Error.Message)
	}

	rr = doJSON(t, srv, http.MethodGet, "/pub/api/v1/version", nil, withBearer(protectTok))
	wantStatus(t, rr, http.StatusOK)

	rr = doJSON(t, srv, http.MethodGet, "/pub/api/v1/version", nil, withCookie(&http.Cookie{Name: adminCookieName, Value: protectTok}))
	wantStatus(t, rr, http.StatusOK)
}

func TestProtectedResponsesArePrivate(t *testing.T) {
	open := newTestServer(t, withAuth(protectTok))
	rr := doJSON(t, open, http.MethodGet, "/pub/api/v1/version", nil)
	if got := rr.Header().Get("Cache-Control"); got != "public, max-age=3600" {
		t.Fatalf("unprotected Cache-Control = %q, want public, max-age=3600", got)
	}

	srv := protectedServer(t)
	rr = doJSON(t, srv, http.MethodGet, "/pub/api/v1/version", nil, withBearer(protectTok))
	if got := rr.Header().Get("Cache-Control"); got != "private, max-age=3600" {
		t.Errorf("protected API Cache-Control = %q, want private, max-age=3600", got)
	}
	rr = doJSON(t, srv, http.MethodGet, "/public/", nil, withBearer(protectTok))
	if strings.Contains(rr.Header().Get("Cache-Control"), "public") {
		t.Errorf("protected page Cache-Control = %q, want no public directive", rr.Header().Get("Cache-Control"))
	}
}

func TestPrivateCacheControl(t *testing.T) {
	for in, want := range map[string]string{
		"public, max-age=300":                 "private, max-age=300",
		"max-age=60, public":                  "max-age=60, private",
		"Public":                              "private",
		"no-cache":                            "no-cache",
		"public, max-age=31536000, immutable": "private, max-age=31536000, immutable",
	} {
		if got := privateCacheControl(in); got != want {
			t.Errorf("privateCacheControl(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestProtectedLoginPostSetsCookieAndReturns(t *testing.T) {
	srv := protectedServer(t)

	rr := doJSON(t, srv, http.MethodPost, "/public/result/abc123def456?lang=sv", formBody(protectTok), loginPost()...)

	wantStatus(t, rr, http.StatusSeeOther)
	if got := rr.Header().Get("Location"); got != "./abc123def456?lang=sv" {
		t.Errorf("Location = %q, want ./abc123def456?lang=sv", got)
	}
	c := cookieNamed(rr, adminCookieName)
	if c == nil {
		t.Fatal("no session cookie set")
	}
	if c.Value != protectTok || !c.HttpOnly || c.SameSite != http.SameSiteStrictMode {
		t.Errorf("cookie = %+v, want token value, HttpOnly, SameSite=Strict", c)
	}

	rr = doJSON(t, srv, http.MethodGet, "/analysis/", nil, withCookie(c))
	wantStatus(t, rr, http.StatusOK)
}

func TestProtectedLoginPostBadToken(t *testing.T) {
	srv := protectedServer(t)
	for _, tok := range []string{"gm_wrong", ""} {
		rr := doJSON(t, srv, http.MethodPost, "/analysis/", formBody(tok), loginPost()...)
		wantStatus(t, rr, http.StatusUnauthorized)
		if !strings.Contains(rr.Body.String(), `role="alert"`) {
			t.Errorf("token %q: login page lacks the error line", tok)
		}
		if cookieNamed(rr, adminCookieName) != nil {
			t.Errorf("token %q: session cookie set", tok)
		}
	}
}

func TestProtectedLoginPostForeignOrigin(t *testing.T) {
	srv := protectedServer(t)
	rr := doJSON(t, srv, http.MethodPost, "/public/", formBody(protectTok),
		withHeader("Content-Type", "application/x-www-form-urlencoded"), withOrigin("http://evil.example"))
	wantStatus(t, rr, http.StatusForbidden)
	if cookieNamed(rr, adminCookieName) != nil {
		t.Error("session cookie set on a cross-origin post")
	}
}

func TestSelfLocation(t *testing.T) {
	for in, want := range map[string]string{
		"/public/":                 "./",
		"/public/result/abc":       "./abc",
		"/analysis/tags/A:B?x=1":   "./A:B?x=1",
		"/analysis/ns/a%20b":       "./a%20b",
		"/analysis/?snapshot=s1":   "./?snapshot=s1",
		"/public/result/abc?lang=": "./abc?lang=",
	} {
		u, err := url.Parse(in)
		if err != nil {
			t.Fatal(err)
		}
		if got := selfLocation(u); got != want {
			t.Errorf("selfLocation(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestProtectedPublicJobPostChecksOrigin(t *testing.T) {
	srv := protectedServer(t)
	cookie := withCookie(&http.Cookie{Name: adminCookieName, Value: protectTok})

	rr := doJSON(t, srv, http.MethodPost, "/pub/api/v1/jobs", map[string]any{}, cookie, withOrigin("http://evil.example"))
	wantErrorCode(t, rr, http.StatusForbidden, "csrf_origin_mismatch")

	rr = doJSON(t, srv, http.MethodPost, "/pub/api/v1/jobs", map[string]any{}, cookie, sameOrigin())
	wantStatus(t, rr, http.StatusBadRequest)
}

func TestProtectedRobotsAndSitemap(t *testing.T) {
	srv := protectedServer(t)
	rr := doJSON(t, srv, http.MethodGet, "/robots.txt", nil)
	wantStatus(t, rr, http.StatusOK)
	if got := rr.Body.String(); got != "User-agent: *\nDisallow: /\n" {
		t.Errorf("robots.txt = %q", got)
	}
	rr = doJSON(t, srv, http.MethodGet, "/sitemap.xml", nil)
	wantStatus(t, rr, http.StatusNotFound)
}

func TestProtectLeavesAdminSurfaces(t *testing.T) {
	srv := protectedServer(t)
	rr := doJSON(t, srv, http.MethodGet, "/", nil)
	wantStatus(t, rr, http.StatusOK)
	if isLoginPage(rr.Body.String()) {
		t.Error("admin UI served the public login page")
	}
	rr = doJSON(t, srv, http.MethodGet, "/api/v1/whoami", nil)
	wantStatus(t, rr, http.StatusOK)
}

func TestReloadAuthTogglesProtection(t *testing.T) {
	srv := newTestServer(t, withAuth(protectTok))
	tokens := []AdminToken{{Label: "a", Hash: hashToken(protectTok)}}
	version := func() *httptest.ResponseRecorder { return doJSON(t, srv, http.MethodGet, "/pub/api/v1/version", nil) }

	wantStatus(t, version(), http.StatusOK)
	if err := srv.ReloadAuth(AuthConfig{AdminTokens: tokens, ProtectPublic: true}); err != nil {
		t.Fatal(err)
	}
	wantStatus(t, version(), http.StatusUnauthorized)
	if err := srv.ReloadAuth(AuthConfig{ProtectPublic: true}); err == nil {
		t.Fatal("reload with protect and no tokens succeeded")
	}
	wantStatus(t, version(), http.StatusUnauthorized)
	if err := srv.ReloadAuth(AuthConfig{AdminTokens: tokens}); err != nil {
		t.Fatal(err)
	}
	wantStatus(t, version(), http.StatusOK)
}

func TestProtectedLoginLocale(t *testing.T) {
	srv := protectedServer(t)

	rr := doJSON(t, srv, http.MethodGet, "/public/", nil, withHeader("Accept-Language", "sv-SE,sv;q=0.9"))
	if body := rr.Body.String(); !strings.Contains(body, `<html lang="sv">`) || !strings.Contains(body, "Administratörsinloggning krävs") {
		t.Errorf("Accept-Language sv: body not Swedish: %s", body)
	}

	rr = doJSON(t, srv, http.MethodGet, "/analysis/?lang=de", nil, withHeader("Accept-Language", "sv"))
	if body := rr.Body.String(); !strings.Contains(body, `<html lang="de">`) || !strings.Contains(body, "Admin-Authentifizierung erforderlich") {
		t.Errorf("?lang=de: body not German: %s", body)
	}
}

func TestLoadFileConfigProtectPublic(t *testing.T) {
	path := writeTempJSON(t, `{"auth":{"admin_tokens":[{"hash":"`+hashToken(protectTok)+`"}],"protect_public":true}}`)
	fc, err := LoadFileConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	cfg := DefaultConfig()
	cfg.ApplyFileConfig(fc)
	if !cfg.Auth.ProtectPublic {
		t.Fatal("auth.protect_public not applied from file")
	}
}
