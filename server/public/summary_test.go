//go:build !nogui

package public

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

// Mirrors the hooks in ui-public/index.html; TestEmbeddedIndexHasEveryHook
// checks the real file against the same renderer.
const indexFixture = `<!doctype html>
<html lang="en">
  <head>
    <meta name="description" content="__PAGE_DESCRIPTION__" />
    <meta property="og:title" content="__PAGE_TITLE__" />
    <meta property="og:description" content="__PAGE_DESCRIPTION__" />
    <meta property="og:url" content="__OG_URL__" />
    <meta property="og:image" content="__PUBLIC_URL__android-chrome-512x512.png" />
    <!-- CANONICAL -->
    <!-- ROBOTS_TAG -->
    <!-- HREFLANG_TAGS -->
    <title>__PAGE_TITLE__</title>
  </head>
  <body>
    <noscript>
      <!-- RESULT_SUMMARY -->
      <!-- NOSCRIPT_GENERIC_START -->
      <h1>Gonemaster</h1>
      <p>The interactive checker needs JavaScript.</p>
      <!-- NOSCRIPT_GENERIC_END -->
    </noscript>
  </body>
</html>
`

const genericMarker = "The interactive checker needs JavaScript."

func fixtureFS() fstest.MapFS {
	return fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte(indexFixture)}}
}

func sampleSummary() ResultSummary {
	return ResultSummary{
		Domain:     "example.com",
		Grade:      "A",
		Score:      96,
		FinishedAt: time.Date(2026, 8, 26, 14, 3, 0, 0, time.UTC),
		Warnings:   2,
		Errors:     1,
		Findings: []Finding{
			{Level: "WARNING", Message: "SOA refresh is low", Testcase: "Zone01"},
			{Level: "WARNING", Message: "No IPv6 nameserver", Testcase: "Nameserver05"},
			{Level: "ERROR", Message: "Delegation is broken", Testcase: "Delegation02"},
		},
	}
}

// lookupOf answers with one fixed summary and status for any id.
func lookupOf(s ResultSummary, status LookupStatus) LookupResult {
	return func(string, string) (ResultSummary, LookupStatus) { return s, status }
}

func get(t *testing.T, path string, lookup LookupResult, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Host = "example.com"
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rr := httptest.NewRecorder()
	serveIndex(fixtureFS(), rr, req, "", lookup)
	return rr
}

func TestResultID(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"result/abc12345", "abc12345"},
		{"result/abc12345/", "abc12345"},
		{"result/A1", "A1"},
		{"result/", ""},
		{"result", ""},
		{"result/abc-123", ""},
		{"result/abc/extra", ""},
		{"", ""},
		{"assets/app.js", ""},
	}
	for _, tt := range tests {
		if got := resultID(tt.in); got != tt.want {
			t.Errorf("resultID(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestNegotiateLocale(t *testing.T) {
	tests := []struct {
		name   string
		param  string
		accept string
		want   string
	}{
		{name: "no signal falls back to en", want: "en"},
		{name: "param wins", param: "sv", accept: "de", want: "sv"},
		{name: "param region is stripped", param: "sv-SE", want: "sv"},
		{name: "unshipped param is ignored", param: "zz", accept: "de", want: "de"},
		{name: "accept-language first match", accept: "de-DE,en;q=0.8", want: "de"},
		{name: "accept-language skips unshipped", accept: "zz,sl;q=0.9", want: "sl"},
		{name: "accept-language q values stripped", accept: "ja;q=0.9", want: "ja"},
		{name: "nothing shipped falls back to en", accept: "zz,yy", want: "en"},
		{name: "injection attempt is not a locale", param: `"><script>`, want: "en"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := negotiateLocale(tt.param, tt.accept); got != tt.want {
				t.Fatalf("negotiateLocale(%q, %q) = %q, want %q", tt.param, tt.accept, got, tt.want)
			}
		})
	}
}

func TestServeResultRendersSummary(t *testing.T) {
	rr := get(t, "/result/abc12345", lookupOf(sampleSummary(), LookupFound), nil)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	body := rr.Body.String()
	for _, want := range []string{
		"<title>example.com - grade A - Gonemaster</title>",
		"Results for example.com",
		"Grade A, score 96/100.",
		"Tested: 2026-08-26 14:03 UTC",
		"Issues: 3",
		"WARNING Zone01: SOA refresh is low",
		"ERROR Delegation02: Delegation is broken",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q", want)
		}
	}
	if strings.Contains(body, genericMarker) {
		t.Error("generic noscript body was not replaced on a result page")
	}
	if strings.Contains(body, "__PAGE_TITLE__") || strings.Contains(body, "__OG_URL__") {
		t.Error("placeholders left unsubstituted")
	}
}

// og:url is the link target scrapers follow, so it must name the result page
// and agree with the canonical.
func TestServeResultOgURLMatchesCanonical(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		lookup  LookupResult
		wantURL string
	}{
		{
			name:    "result page names the result",
			path:    "/result/abc12345",
			lookup:  lookupOf(sampleSummary(), LookupFound),
			wantURL: "http://example.com/public/result/abc12345",
		},
		{
			name:    "home page names the base",
			path:    "/",
			lookup:  nil,
			wantURL: "http://example.com/",
		},
		{
			name:    "unknown path is home, not the requested path",
			path:    "/garbage",
			lookup:  lookupOf(ResultSummary{}, LookupNotFound),
			wantURL: "http://example.com/",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := get(t, tt.path, tt.lookup, nil).Body.String()
			if !strings.Contains(body, `<meta property="og:url" content="`+tt.wantURL+`" />`) {
				t.Errorf("og:url is not %q", tt.wantURL)
			}
			if !strings.Contains(body, `<link rel="canonical" href="`+tt.wantURL+`" />`) {
				t.Errorf("canonical is not %q", tt.wantURL)
			}
		})
	}
}

func TestServeResultStatusAndCaching(t *testing.T) {
	tests := []struct {
		name        string
		status      LookupStatus
		wantCode    int
		wantCache   string
		wantVary    string
		wantGeneric bool
	}{
		{
			name: "found is cacheable and varies by language", status: LookupFound,
			wantCode: http.StatusOK, wantCache: "public, max-age=300", wantVary: "Accept-Language",
		},
		{
			// A link pasted in chat mid-run must not be cached as dead.
			name: "pending is 200 and uncached", status: LookupPending,
			wantCode: http.StatusOK, wantCache: "no-cache", wantGeneric: true,
		},
		{
			name: "unknown is 404 and uncached", status: LookupNotFound,
			wantCode: http.StatusNotFound, wantCache: "no-cache", wantGeneric: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rr := get(t, "/result/abc12345", lookupOf(sampleSummary(), tt.status), nil)
			if rr.Code != tt.wantCode {
				t.Errorf("status = %d, want %d", rr.Code, tt.wantCode)
			}
			if got := rr.Header().Get("Cache-Control"); got != tt.wantCache {
				t.Errorf("Cache-Control = %q, want %q", got, tt.wantCache)
			}
			if got := rr.Header().Get("Vary"); got != tt.wantVary {
				t.Errorf("Vary = %q, want %q", got, tt.wantVary)
			}
			if got := strings.Contains(rr.Body.String(), genericMarker); got != tt.wantGeneric {
				t.Errorf("generic body present = %v, want %v", got, tt.wantGeneric)
			}
		})
	}
}

// Results name domains someone chose to test, so no result page may be indexed,
// and none may advertise language alternates.
func TestResultPagesAreNoIndexAndCarryNoHreflang(t *testing.T) {
	for _, status := range []LookupStatus{LookupFound, LookupPending, LookupNotFound} {
		rr := get(t, "/result/abc12345", lookupOf(sampleSummary(), status), nil)
		body := rr.Body.String()
		if !strings.Contains(body, `<meta name="robots" content="noindex, nofollow" />`) {
			t.Errorf("status %v: robots meta missing", status)
		}
		if rr.Header().Get("X-Robots-Tag") != "noindex" {
			t.Errorf("status %v: X-Robots-Tag = %q", status, rr.Header().Get("X-Robots-Tag"))
		}
		if strings.Contains(body, "hreflang=") {
			t.Errorf("status %v: result page advertises hreflang alternates", status)
		}
	}
}

func TestHomePageIsIndexableAndKeepsHreflang(t *testing.T) {
	rr := get(t, "/", nil, nil)
	body := rr.Body.String()
	if strings.Contains(body, "noindex") {
		t.Error("home page must stay indexable")
	}
	if rr.Header().Get("X-Robots-Tag") != "" {
		t.Error("home page must not send X-Robots-Tag")
	}
	if !strings.Contains(body, `hreflang="en"`) {
		t.Error("home page lost its hreflang alternates")
	}
	if !strings.Contains(body, "<title>Gonemaster</title>") {
		t.Error("home page title is not Gonemaster")
	}
	if !strings.Contains(body, genericMarker) {
		t.Error("home page lost the generic noscript body")
	}
}

// A domain reaches this code from user input, and message args carry remote
// data, so a rendering mistake here is stored XSS.
func TestServeResultEscapesUserInput(t *testing.T) {
	nasty := ResultSummary{
		Domain:   `"><script>alert(1)</script>`,
		Grade:    "F",
		Warnings: 1,
		Findings: []Finding{{
			Level:    "WARNING",
			Testcase: "Zone01",
			Message:  `ns "><img src=x onerror=alert(2)>.example`,
		}},
	}
	body := get(t, "/result/abc12345", lookupOf(nasty, LookupFound), nil).Body.String()

	for _, leaked := range []string{"<script>alert(1)", "<img src=x", `"><`} {
		if strings.Contains(body, leaked) {
			t.Errorf("unescaped %q reached the page", leaked)
		}
	}
	if !strings.Contains(body, "&lt;script&gt;alert(1)") {
		t.Error("domain was not escaped into the page")
	}
	if !strings.Contains(body, "onerror=alert(2)&gt;") {
		t.Error("message arg was not escaped into the page")
	}
}

// The block renders where CSP forbids inline styles and scripts never run.
func TestRenderedSummaryHasNoScriptOrInlineStyle(t *testing.T) {
	body := renderSummary(sampleSummary(), "en")
	if strings.Contains(body, "<script") {
		t.Error("rendered summary contains a script tag")
	}
	if strings.Contains(body, "style=") {
		t.Error("rendered summary contains an inline style")
	}
}

func TestServeResultCapsFindings(t *testing.T) {
	s := ResultSummary{Domain: "example.com", Warnings: 30}
	for i := 0; i < MaxSummaryFindings; i++ {
		s.Findings = append(s.Findings, Finding{
			Level: "WARNING", Testcase: "Zone01", Message: fmt.Sprintf("finding %d", i),
		})
	}
	body := get(t, "/result/abc12345", lookupOf(s, LookupFound), nil).Body.String()

	if got := strings.Count(body, "<li>"); got != MaxSummaryFindings {
		t.Errorf("rendered %d findings, want %d", got, MaxSummaryFindings)
	}
	if !strings.Contains(body, "Issues: 30") {
		t.Error("the full issue count is not reported")
	}
	if !strings.Contains(body, "and 5 more") {
		t.Error("the remainder past the cap is not reported")
	}
}

func TestServeResultWithoutGradeOmitsScoring(t *testing.T) {
	s := sampleSummary()
	s.Grade = ""
	s.Score = 0
	body := get(t, "/result/abc12345", lookupOf(s, LookupFound), nil).Body.String()

	if !strings.Contains(body, "<title>example.com - Gonemaster</title>") {
		t.Error("title should not claim a grade")
	}
	if strings.Contains(body, "/100") {
		t.Error("a score was rendered while scoring is hidden")
	}
}

func TestServeResultLocalizesFromRequest(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		accept   string
		wantText string
	}{
		{name: "lang query", path: "/result/abc12345?lang=sv", wantText: "Resultat för example.com"},
		{name: "accept-language", path: "/result/abc12345", accept: "de-DE,en;q=0.8", wantText: "Ergebnisse für example.com"},
		{name: "query beats header", path: "/result/abc12345?lang=ja", accept: "de", wantText: "example.com の結果"},
		{name: "unknown falls back to en", path: "/result/abc12345?lang=zz", wantText: "Results for example.com"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			headers := map[string]string{}
			if tt.accept != "" {
				headers["Accept-Language"] = tt.accept
			}
			body := get(t, tt.path, lookupOf(sampleSummary(), LookupFound), headers).Body.String()
			if !strings.Contains(body, tt.wantText) {
				t.Errorf("body missing localized %q", tt.wantText)
			}
		})
	}
}

// A hostile Host header would otherwise land raw in a content="..." attribute,
// and result pages are cacheable.
func TestServeIndexRejectsHostileHost(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/result/abc12345", nil)
	req.Host = `evil"onload="alert(1)`
	rr := httptest.NewRecorder()
	serveIndex(fixtureFS(), rr, req, "", lookupOf(sampleSummary(), LookupFound))

	if strings.Contains(rr.Body.String(), `onload="alert(1)`) {
		t.Fatal("hostile Host reached the page unescaped")
	}
}

func TestEmbeddedIndexHasEveryHook(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "ui-public", "index.html"))
	if err != nil {
		t.Fatalf("read index.html: %v", err)
	}
	hooks := []string{
		"__PAGE_TITLE__", "__PAGE_DESCRIPTION__", "__OG_URL__", "__PUBLIC_URL__",
		"<!-- CANONICAL -->", "<!-- ROBOTS_TAG -->", "<!-- HREFLANG_TAGS -->",
		"<!-- RESULT_SUMMARY -->", "<!-- NOSCRIPT_GENERIC_START -->", "<!-- NOSCRIPT_GENERIC_END -->",
	}
	for _, hook := range hooks {
		if !strings.Contains(string(data), hook) {
			t.Errorf("index.html lost the %s hook", hook)
		}
	}

	// Both page kinds must consume every hook, or a placeholder ships to users.
	home := string(render(data, "https://example.com/", homePage("https://example.com/")))
	p := homePage("https://example.com/")
	p.url = "https://example.com/public/result/abc12345"
	p.robots = `<meta name="robots" content="noindex" />`
	p.hreflang = ""
	p.summary = renderSummary(sampleSummary(), "en")
	result := string(render(data, "https://example.com/", p))
	for _, page := range []struct {
		name string
		body string
	}{{"home", home}, {"result", result}} {
		for _, hook := range hooks {
			if strings.Contains(page.body, hook) {
				t.Errorf("%s page left %s unsubstituted", page.name, hook)
			}
		}
	}
	if strings.Contains(result, genericMarker) {
		t.Error("result page kept the generic noscript body")
	}
	if !strings.Contains(home, genericMarker) {
		t.Error("home page lost the generic noscript body")
	}
}

// The Go copies of the three catalog templates must match the shipped JSON, or
// the tab title and the rendered page disagree.
func TestSummaryStringsMatchCatalogs(t *testing.T) {
	for locale, want := range summaryText {
		path := filepath.Join("..", "..", "ui-public", "src", "i18n", locale+".json")
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		var catalog map[string]string
		if err := json.Unmarshal(data, &catalog); err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		for _, pair := range []struct {
			key string
			got string
		}{
			{"pub.doc_title_grade", want.titleGrade},
			{"pub.doc_title_result", want.titleResult},
			{"pub.result_heading", want.heading},
		} {
			if catalog[pair.key] != pair.got {
				t.Errorf("%s %s: Go has %q, catalog has %q", locale, pair.key, pair.got, catalog[pair.key])
			}
		}
	}
}

func TestSummaryTextCoversShippedLocales(t *testing.T) {
	for _, lang := range hreflangLangs {
		if _, ok := summaryText[lang]; !ok {
			t.Errorf("summaryText has no entry for shipped locale %q", lang)
		}
	}
	if len(summaryText) != len(hreflangLangs) {
		t.Errorf("summaryText has %d locales, hreflangLangs has %d", len(summaryText), len(hreflangLangs))
	}
}
