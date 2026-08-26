//go:build !nogui

package public

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"codeberg.org/pawal/gonemaster/server/internal/spatest"
)

// mustDist returns the embedded public UI files.
func mustDist(t *testing.T) fs.FS {
	t.Helper()
	fsys, err := dist()
	if err != nil {
		t.Fatalf("dist(): %v", err)
	}
	return fsys
}

func TestCleanRequestPath(t *testing.T) {
	spatest.CleanPath(t, cleanRequestPath, spatest.PathCase{In: "/assets/app.js", Want: "assets/app.js"})
}

func TestHandlerPathTraversalAttemptsCannotEscapeDist(t *testing.T) {
	spatest.PathTraversalCannotEscape(t, Handler("", nil),
		"/../public.go",
		"/../../server/public/public.go",
		"/assets/../public.go",
		"/../../etc/passwd",
		"//etc/passwd",
		"/..%2fpublic.go",
	)
}

func TestHandlerMethodNotAllowed(t *testing.T) {
	spatest.MethodNotAllowed(t, Handler("", nil))
}

func TestHandlerServesIndexForRootAndUnknownPaths(t *testing.T) {
	spatest.IndexForRootAndUnknownPaths(t, Handler("", nil), "/not/a/real/path")
}

func TestHandlerServesAssetsWithCacheControl(t *testing.T) {
	spatest.AssetsHaveImmutableCacheControl(t, Handler("", nil), mustDist(t))
}

func TestHandlerServesFaviconFilesAndManifest(t *testing.T) {
	spatest.FaviconFilesAndManifest(t, Handler("", nil), mustDist(t))
}

func TestHandlerIndexIncludesFaviconLinks(t *testing.T) {
	spatest.IndexLinksFavicons(t, Handler("", nil), mustDist(t), "/public/")
}

func TestServeIndexFallsBackToUnavailablePageWhenIndexMissing(t *testing.T) {
	fsys := fstest.MapFS{
		"placeholder.txt": &fstest.MapFile{Data: []byte("placeholder")},
	}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()

	serveIndex(fsys, rr, req, "", nil)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "UI is not embedded in this binary") {
		t.Fatalf("unexpected fallback body: %q", body)
	}
}

// Identical alternate targets are not a language signal, so every locale must
// get its own URL, and none of them may be the base URL: that serves the admin
// UI, not the public one.
func TestBuildHreflang(t *testing.T) {
	result := buildHreflang("https://example.com/")

	for _, lang := range hreflangLangs {
		want := `<link rel="alternate" hreflang="` + lang +
			`" href="` + LocaleURL("https://example.com/", lang) + `" />`
		if !strings.Contains(result, want) {
			t.Errorf("missing alternate for %q\ngot: %s", lang, result)
		}
	}
	if !strings.Contains(result, `hreflang="x-default" href="https://example.com/public/" />`) {
		t.Errorf("x-default should name the unparameterised home page\ngot: %s", result)
	}
	// en shares the x-default URL instead of duplicating that page.
	if strings.Contains(result, "?lang=en") {
		t.Error("English should not get a second URL of its own")
	}
	if strings.Contains(result, `href="https://example.com/"`) {
		t.Error("an alternate points at the site root, which serves the admin UI")
	}
}

// Each locale URL must carry the whole alternate set, including itself, or
// search engines discard the block.
func TestHreflangIsSelfReferencingAndReciprocal(t *testing.T) {
	const base = "https://example.com/"
	block := buildHreflang(base)
	for _, lang := range hreflangLangs {
		// Every locale page renders the same block, so each URL appears in it.
		if !strings.Contains(block, `href="`+LocaleURL(base, lang)+`"`) {
			t.Errorf("the block served at %s does not reference itself", LocaleURL(base, lang))
		}
	}
}

// The hreflang language list drifted once (cs, de, nl were shipped as UI
// locales without being added here), so this test pins hreflangLangs to the
// locale catalogs actually shipped in ui-public/src/i18n.
func TestHreflangLangsMatchShippedLocales(t *testing.T) {
	entries, err := os.ReadDir(filepath.Join("..", "..", "ui-public", "src", "i18n"))
	if err != nil {
		t.Fatalf("read ui-public locale dir: %v", err)
	}
	locales := []string{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		locales = append(locales, strings.TrimSuffix(entry.Name(), ".json"))
	}
	slices.Sort(locales)
	langs := slices.Clone(hreflangLangs)
	slices.Sort(langs)
	if !slices.Equal(langs, locales) {
		t.Fatalf("hreflangLangs = %v, shipped locales = %v; keep them in sync", langs, locales)
	}
}

func TestServeIndexInjectsPlaceholders(t *testing.T) {
	const indexHTML = `<head>` +
		`<meta property="og:url" content="__OG_URL__" />` +
		`<meta property="og:image" content="__PUBLIC_URL__android-chrome-512x512.png" />` +
		`<!-- HREFLANG_TAGS -->` +
		`</head>`
	fsys := fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte(indexHTML)},
	}

	tests := []struct {
		name       string
		configured string
		reqHost    string
		wantURL    string
	}{
		{
			name:       "configured root",
			configured: "https://example.com/",
			reqHost:    "ignored.example.com",
			wantURL:    "https://example.com/",
		},
		{
			name:       "configured subpath",
			configured: "https://example.com/public/",
			reqHost:    "ignored.example.com",
			wantURL:    "https://example.com/public/",
		},
		{
			name:       "configured without trailing slash gets one",
			configured: "https://example.com/public",
			reqHost:    "ignored.example.com",
			wantURL:    "https://example.com/public/",
		},
		{
			name:       "auto-detected from host",
			configured: "",
			reqHost:    "test.example.com",
			wantURL:    "http://test.example.com/",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.Host = tt.reqHost
			rr := httptest.NewRecorder()

			serveIndex(fsys, rr, req, tt.configured, nil)

			body := rr.Body.String()
			if strings.Contains(body, "__PUBLIC_URL__") {
				t.Fatal("__PUBLIC_URL__ placeholder was not replaced")
			}
			if strings.Contains(body, "<!-- HREFLANG_TAGS -->") {
				t.Fatal("<!-- HREFLANG_TAGS --> placeholder was not replaced")
			}
			// og:url names the public UI; og:image is served from the root.
			if !strings.Contains(body, `content="`+tt.wantURL+`public/"`) {
				t.Fatalf("og:url not set to %qpublic/ in body: %s", tt.wantURL, body)
			}
			if !strings.Contains(body, `content="`+tt.wantURL+`android-chrome-512x512.png"`) {
				t.Fatalf("og:image not joined onto %q in body: %s", tt.wantURL, body)
			}
			if !strings.Contains(body, `hreflang="en"`) {
				t.Fatal("hreflang tags not injected")
			}
		})
	}
}
