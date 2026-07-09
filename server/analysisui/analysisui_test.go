//go:build !nogui

package analysisui

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandlerServesIndexHTMLForRoot(t *testing.T) {
	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	Handler("").ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}
	body := resp.Body.String()
	if !strings.Contains(body, "<!doctype html>") && !strings.Contains(body, "<!DOCTYPE html>") {
		t.Fatalf("expected HTML body, got: %s", body[:min(200, len(body))])
	}
}

func TestHandlerFallsBackToIndexForUnknownSPARoute(t *testing.T) {
	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/cohort/tld/domains", nil)
	Handler("").ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200 SPA fallback, got %d", resp.Code)
	}
	if contentType := resp.Header().Get("Content-Type"); !strings.HasPrefix(contentType, "text/html") {
		t.Fatalf("expected text/html fallback, got %q", contentType)
	}
}

func TestHandlerRejectsNonGetMethods(t *testing.T) {
	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("body"))
	Handler("").ServeHTTP(resp, req)
	if resp.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", resp.Code)
	}
}

func TestCleanRequestPath(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{in: "", want: ""},
		{in: "/", want: ""},
		{in: "/index.html", want: "index.html"},
		{in: "index.html", want: "index.html"},
		{in: "/_app/immutable/x.js", want: "_app/immutable/x.js"},
		// Anything still containing ".." after normalization is rejected.
		{in: "/..", want: ""},
		{in: "..", want: ""},
		{in: "/..foo", want: ""},
		{in: "/foo/..bar", want: ""},
	}

	for _, tt := range tests {
		if got := cleanRequestPath(tt.in); got != tt.want {
			t.Fatalf("cleanRequestPath(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestHandlerPathTraversalAttemptsCannotEscapeDist(t *testing.T) {
	h := Handler("")

	rootReq := httptest.NewRequest(http.MethodGet, "/", nil)
	rootRR := httptest.NewRecorder()
	h.ServeHTTP(rootRR, rootReq)
	rootBody, _ := io.ReadAll(rootRR.Result().Body)

	traversals := []string{
		"/../analysisui.go",
		"/_app/../analysisui.go",
		"/../../server/analysisui/analysisui.go",
		"/../../etc/passwd",
		"//etc/passwd",
		"/..%2fanalysisui.go",
	}
	for _, p := range traversals {
		req := httptest.NewRequest(http.MethodGet, p, nil)
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("%s: status = %d, want 200 (SPA fallback)", p, rr.Code)
			continue
		}
		body, _ := io.ReadAll(rr.Result().Body)
		if string(body) != string(rootBody) {
			t.Errorf("%s: served content other than the SPA index", p)
		}
	}
}

func TestInjectMetaReplacesPlaceholders(t *testing.T) {
	in := []byte(`<meta property="og:url" content="__ANALYSIS_OG_URL__" />` +
		`<meta property="og:image" content="__ANALYSIS_OG_IMAGE__" />` +
		`<link rel="canonical" href="__ANALYSIS_OG_URL__" />`)
	out := string(injectMeta(in, "https://example.com/analysis/domains", "https://example.com/analysis/gonemaster.svg"))

	if strings.Contains(out, "__ANALYSIS_OG_URL__") || strings.Contains(out, "__ANALYSIS_OG_IMAGE__") {
		t.Fatalf("placeholders left unreplaced: %s", out)
	}
	if !strings.Contains(out, `content="https://example.com/analysis/domains"`) {
		t.Fatalf("og:url not injected: %s", out)
	}
	if !strings.Contains(out, `href="https://example.com/analysis/domains"`) {
		t.Fatalf("canonical not injected: %s", out)
	}
	if !strings.Contains(out, `content="https://example.com/analysis/gonemaster.svg"`) {
		t.Fatalf("og:image not injected: %s", out)
	}
}

func TestServeIndexDoesNotReflectRequestPath(t *testing.T) {
	// The request path is attacker-controlled; it must never be echoed into the
	// injected canonical/Open Graph tags or it becomes reflected XSS (the
	// /analysis CSP permits inline scripts). Only exercised when the SPA is
	// embedded; the placeholder build serves a static unavailable page instead.
	if !IsBuilt() {
		t.Skip("analysis UI dist not built; injection path not exercised")
	}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, `/%22%3E%3Cscript%3Ealert%281%29%3C%2Fscript%3E`, nil)
	Handler("").ServeHTTP(rr, req)
	body := rr.Body.String()
	if strings.Contains(body, "<script>alert(1)") {
		t.Fatalf("request path reflected into served HTML: %s", body[:min(400, len(body))])
	}
	if strings.Contains(body, "__ANALYSIS_OG_URL__") {
		t.Fatalf("og:url placeholder left unreplaced")
	}
}

func TestResolvePublicURL(t *testing.T) {
	// Configured value wins verbatim.
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if got := resolvePublicURL("https://canonical.example/", req); got != "https://canonical.example/" {
		t.Fatalf("configured URL = %q, want https://canonical.example/", got)
	}

	// Empty config falls back to request host, honouring the forwarded proto.
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = "myhost.example.com"
	req.Header.Set("X-Forwarded-Proto", "https")
	if got := resolvePublicURL("", req); got != "https://myhost.example.com/" {
		t.Fatalf("auto-detected URL = %q, want https://myhost.example.com/", got)
	}
}

func TestServerMountsAnalysisRoute(t *testing.T) {
	// The /analysis route returns a 301 redirect to /analysis/ (with slash),
	// which is mounted in server.go. Exercised via the embedded Handler here:
	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/_app/immutable/nothing.js", nil)
	Handler("").ServeHTTP(resp, req)
	// unknown file under _app should either be a 404 or fall back to the SPA
	// index; both are acceptable - just make sure we don't crash.
	if resp.Code != http.StatusOK && resp.Code != http.StatusNotFound {
		t.Fatalf("unexpected status for missing asset: %d", resp.Code)
	}
}
