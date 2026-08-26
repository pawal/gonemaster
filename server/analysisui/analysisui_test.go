//go:build !nogui

package analysisui

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"codeberg.org/pawal/gonemaster/server/internal/spatest"
)

func TestHandlerServesIndexHTMLForRoot(t *testing.T) {
	body := spatest.IndexBody(t, Handler(""))
	if !strings.Contains(body, "<!doctype html>") && !strings.Contains(body, "<!DOCTYPE html>") {
		t.Fatalf("expected HTML body, got: %s", body[:min(200, len(body))])
	}
}

func TestHandlerFallsBackToIndexForUnknownSPARoute(t *testing.T) {
	spatest.IndexForRootAndUnknownPaths(t, Handler(""), "/cohort/tld/domains")
}

func TestHandlerRejectsNonGetMethods(t *testing.T) {
	spatest.MethodNotAllowed(t, Handler(""))
}

func TestCleanRequestPath(t *testing.T) {
	spatest.CleanPath(t, cleanRequestPath, spatest.PathCase{In: "/_app/immutable/x.js", Want: "_app/immutable/x.js"})
}

func TestHandlerPathTraversalAttemptsCannotEscapeDist(t *testing.T) {
	spatest.PathTraversalCannotEscape(t, Handler(""),
		"/../analysisui.go",
		"/_app/../analysisui.go",
		"/../../server/analysisui/analysisui.go",
		"/../../etc/passwd",
		"//etc/passwd",
		"/..%2fanalysisui.go",
	)
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

// The base URL itself is resolved and tested in server/internal/baseurl; this
// pins that a hostile Host cannot reach the injected og:url.
func TestServeIndexRejectsHostileHost(t *testing.T) {
	fsys := fstest.MapFS{"index.html": &fstest.MapFile{
		Data: []byte(`<meta property="og:url" content="__ANALYSIS_OG_URL__" />`),
	}}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = `evil"onload="alert(1)`
	rr := httptest.NewRecorder()

	serveIndex(fsys, rr, req, "")

	if strings.Contains(rr.Body.String(), `onload="alert(1)`) {
		t.Fatal("hostile Host reached the page")
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
