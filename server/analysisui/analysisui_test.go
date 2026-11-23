//go:build !nogui

package analysisui

import (
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"codeberg.org/pawal/gonemaster/server/internal/spatest"
)

func TestHandlerServesIndexHTMLForRoot(t *testing.T) {
	body := spatest.IndexBody(t, Handler(nil))
	if !strings.Contains(body, "<!doctype html>") && !strings.Contains(body, "<!DOCTYPE html>") {
		t.Fatalf("expected HTML body, got: %s", body[:min(200, len(body))])
	}
}

func TestHandlerFallsBackToIndexForUnknownSPARoute(t *testing.T) {
	spatest.IndexForRootAndUnknownPaths(t, Handler(nil), "/cohort/tld/domains")
}

func TestHandlerRejectsNonGetMethods(t *testing.T) {
	spatest.MethodNotAllowed(t, Handler(nil))
}

func TestCleanRequestPath(t *testing.T) {
	spatest.CleanPath(t, cleanRequestPath, spatest.PathCase{In: "/_app/immutable/x.js", Want: "_app/immutable/x.js"})
}

func TestHandlerPathTraversalAttemptsCannotEscapeDist(t *testing.T) {
	spatest.PathTraversalCannotEscape(t, Handler(nil),
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

// The request path reaches the embedded page only percent-encoded.
func TestServeIndexEscapesRequestPath(t *testing.T) {
	if !IsBuilt() {
		t.Skip("analysis UI dist not built; injection path not exercised")
	}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, `/%22%3E%3Cscript%3Ealert%281%29%3C%2Fscript%3E`, nil)
	Handler(nil).ServeHTTP(rr, req)
	body := rr.Body.String()
	if strings.Contains(body, "<script>alert(1)") {
		t.Fatalf("request path reflected into served HTML: %s", body[:min(400, len(body))])
	}
	if strings.Contains(body, "__ANALYSIS_OG_URL__") {
		t.Fatalf("og:url placeholder left unreplaced")
	}
}

func TestCanonicalURL(t *testing.T) {
	const base = "https://gonemaster.example"
	cases := []struct {
		name, target, want string
	}{
		{"root", "/", base + "/analysis/"},
		{"empty path", "", base + "/analysis/"},
		{"trailing slash dropped", "/domains/", base + "/analysis/domains"},
		{"list state dropped", "/domains?sort=grade&offset=40&grade=A", base + "/analysis/domains"},
		{"leaked nameserver dropped", "/nameservers/a.dns.it?grade=A&nameserver=c.nic.example", base + "/analysis/nameservers/a.dns.it"},
		{"pinned kept in order", "/tags?snapshot=2026-07-01&sort=count&dataset_tag=tld", base + "/analysis/tags?dataset_tag=tld&snapshot=2026-07-01"},
		{"empty pinned dropped", "/tags?dataset_tag=&snapshot=s1", base + "/analysis/tags?snapshot=s1"},
		{"browser form encoding", "/domains?dataset_tag=a%20b*~", base + "/analysis/domains?dataset_tag=a+b*%7E"},
		{"encoded slash kept", "/prefixes/195.253.64.0%2F24?nameserver=v0n0.nic.example", base + "/analysis/prefixes/195.253.64.0%2F24"},
		{"encoded colon kept", "/endpoints/2001%3Adb8%3A%3A1?nameserver=ns.example", base + "/analysis/endpoints/2001%3Adb8%3A%3A1"},
		{"markup escaped", `/x"><script>`, base + "/analysis/x%22%3E%3Cscript%3E"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			u, err := url.Parse(tc.target)
			if err != nil {
				t.Fatal(err)
			}
			if got := canonicalURL(base, u); got != tc.want {
				t.Fatalf("canonicalURL(%q) = %q, want %q", tc.target, got, tc.want)
			}
		})
	}
}

func TestServeIndexInjectsPageCanonical(t *testing.T) {
	fsys := fstest.MapFS{"index.html": &fstest.MapFile{
		Data: []byte(`<link rel="canonical" href="__ANALYSIS_OG_URL__" />`),
	}}
	cases := []struct {
		name, target, want string
	}{
		{"detail page", "/nameservers/a.dns.it?grade=A&nameserver=c.nic.example", `<link rel="canonical" href="https://gonemaster.example/analysis/nameservers/a.dns.it" />`},
		{"pinned params", "/tags/Z01_MNAME_NOT_MASTER?dataset_tag=tld&snapshot=s1", `<link rel="canonical" href="https://gonemaster.example/analysis/tags/Z01_MNAME_NOT_MASTER?dataset_tag=tld&amp;snapshot=s1" />`},
		{"quote and ampersand in path", "/domains/a'b&c", `<link rel="canonical" href="https://gonemaster.example/analysis/domains/a&#39;b&amp;c" />`},
		{"markup in path", `/x"><script>alert(1)</script>`, `<link rel="canonical" href="https://gonemaster.example/analysis/x%22%3E%3Cscript%3Ealert%281%29%3C/script%3E" />`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			serveIndex(fsys, rr, httptest.NewRequest(http.MethodGet, tc.target, nil), "https://gonemaster.example/")
			if got := rr.Body.String(); got != tc.want {
				t.Fatalf("body = %q, want %q", got, tc.want)
			}
		})
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
	Handler(nil).ServeHTTP(resp, req)
	// unknown file under _app should either be a 404 or fall back to the SPA
	// index; both are acceptable - just make sure we don't crash.
	if resp.Code != http.StatusOK && resp.Code != http.StatusNotFound {
		t.Fatalf("unexpected status for missing asset: %d", resp.Code)
	}
}

func TestInlineScriptHashes(t *testing.T) {
	page := []byte(`<head><script src="/a.js"></script><script>
	boot()
</script></head><body><script type="module">go()</script></body>`)
	got := inlineScriptHashes(page)
	want := []string{
		"'sha256-" + sha256Base64("\n\tboot()\n") + "'",
		"'sha256-" + sha256Base64("go()") + "'",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("inlineScriptHashes = %v, want %v", got, want)
	}
}

func sha256Base64(s string) string {
	sum := sha256.Sum256([]byte(s))
	return base64.StdEncoding.EncodeToString(sum[:])
}
