//go:build !nogui
// +build !nogui

package public

import (
	"crypto/tls"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func TestCleanRequestPath(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{in: "", want: ""},
		{in: "/", want: ""},
		{in: "/index.html", want: "index.html"},
		{in: "index.html", want: "index.html"},
		{in: "/assets/app.js", want: "assets/app.js"},
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
	// path.Clean collapses dot-segments and the embed.FS is sandboxed via
	// fs.Sub(distFS, "dist"), so any traversal target that does not exist in
	// dist/ falls through to the SPA index. This test pins that behavior.
	h := Handler("")

	rootReq := httptest.NewRequest(http.MethodGet, "/", nil)
	rootRR := httptest.NewRecorder()
	h.ServeHTTP(rootRR, rootReq)
	rootBody, _ := io.ReadAll(rootRR.Result().Body)

	traversals := []string{
		"/../public.go",
		"/../../server/public/public.go",
		"/assets/../public.go",
		"/../../etc/passwd",
		"//etc/passwd",
		"/..%2fpublic.go",
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

func TestHandlerMethodNotAllowed(t *testing.T) {
	h := Handler("")

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandlerServesIndexForRootAndUnknownPaths(t *testing.T) {
	h := Handler("")

	rootReq := httptest.NewRequest(http.MethodGet, "/", nil)
	rootRR := httptest.NewRecorder()
	h.ServeHTTP(rootRR, rootReq)
	if rootRR.Code != http.StatusOK {
		t.Fatalf("root status = %d, want %d", rootRR.Code, http.StatusOK)
	}
	rootBody, err := io.ReadAll(rootRR.Result().Body)
	if err != nil {
		t.Fatalf("read root body: %v", err)
	}
	if len(rootBody) == 0 {
		t.Fatal("root response body is empty")
	}

	unknownReq := httptest.NewRequest(http.MethodGet, "/not/a/real/path", nil)
	unknownRR := httptest.NewRecorder()
	h.ServeHTTP(unknownRR, unknownReq)
	if unknownRR.Code != http.StatusOK {
		t.Fatalf("unknown path status = %d, want %d", unknownRR.Code, http.StatusOK)
	}
	unknownBody, err := io.ReadAll(unknownRR.Result().Body)
	if err != nil {
		t.Fatalf("read unknown path body: %v", err)
	}
	if string(unknownBody) != string(rootBody) {
		t.Fatal("unknown path did not serve the SPA index content")
	}
}

func TestHandlerServesAssetsWithCacheControl(t *testing.T) {
	fsys, err := dist()
	if err != nil {
		t.Fatalf("dist(): %v", err)
	}
	assetName, ok := firstAssetName(t, fsys)
	if !ok {
		t.Skip("no embedded asset files found under dist/assets")
	}

	h := Handler("")
	req := httptest.NewRequest(http.MethodGet, "/assets/"+assetName, nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("asset status = %d, want %d", rr.Code, http.StatusOK)
	}
	const wantCacheControl = "public, max-age=31536000, immutable"
	if got := rr.Header().Get("Cache-Control"); got != wantCacheControl {
		t.Fatalf("Cache-Control = %q, want %q", got, wantCacheControl)
	}
}

func TestHandlerServesFaviconFilesAndManifest(t *testing.T) {
	fsys, err := dist()
	if err != nil {
		t.Fatalf("dist(): %v", err)
	}
	if _, err := fs.Stat(fsys, "site.webmanifest"); err != nil {
		t.Skip("favicon assets are not embedded; run make ui-build")
	}

	requiredFiles := []string{
		"favicon.svg",
		"favicon-16x16.png",
		"favicon-32x32.png",
		"apple-touch-icon.png",
		"android-chrome-192x192.png",
		"android-chrome-512x512.png",
		"site.webmanifest",
	}

	h := Handler("")
	for _, name := range requiredFiles {
		if _, err := fs.Stat(fsys, name); err != nil {
			t.Fatalf("expected embedded file %q: %v", name, err)
		}

		req := httptest.NewRequest(http.MethodGet, "/"+name, nil)
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("GET /%s status = %d, want %d", name, rr.Code, http.StatusOK)
		}
		if rr.Body.Len() == 0 {
			t.Fatalf("GET /%s returned an empty body", name)
		}
	}
}

func TestHandlerIndexIncludesFaviconLinks(t *testing.T) {
	fsys, err := dist()
	if err != nil {
		t.Fatalf("dist(): %v", err)
	}
	if _, err := fs.Stat(fsys, "site.webmanifest"); err != nil {
		t.Skip("favicon assets are not embedded; run make ui-build")
	}

	h := Handler("")
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}

	body := rr.Body.String()
	for _, snippet := range []string{
		`href="/public/favicon.svg"`,
		`href="/public/favicon-32x32.png"`,
		`href="/public/favicon-16x16.png"`,
		`href="/public/apple-touch-icon.png"`,
		`href="/public/site.webmanifest"`,
	} {
		if !strings.Contains(body, snippet) {
			t.Fatalf("index.html missing %q", snippet)
		}
	}
}

func TestServeIndexFallsBackToUnavailablePageWhenIndexMissing(t *testing.T) {
	fsys := fstest.MapFS{
		"placeholder.txt": &fstest.MapFile{Data: []byte("placeholder")},
	}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()

	serveIndex(fsys, rr, req, "")

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "UI is not embedded in this binary") {
		t.Fatalf("unexpected fallback body: %q", body)
	}
}

func TestResolvePublicURL(t *testing.T) {
	tests := []struct {
		name       string
		configured string
		host       string
		fwdProto   string
		fwdHost    string
		tls        bool
		want       string
	}{
		{
			name:       "configured root",
			configured: "https://example.com/",
			host:       "ignored.example.com",
			want:       "https://example.com/",
		},
		{
			name:       "configured subpath",
			configured: "https://example.com/public/",
			host:       "ignored.example.com",
			want:       "https://example.com/public/",
		},
		{
			name: "auto-detect http",
			host: "myhost.example.com",
			want: "http://myhost.example.com/",
		},
		{
			name:     "auto-detect tls",
			host:     "myhost.example.com",
			tls:      true,
			want:     "https://myhost.example.com/",
		},
		{
			name:     "X-Forwarded-Proto https",
			host:     "myhost.example.com",
			fwdProto: "https",
			want:     "https://myhost.example.com/",
		},
		{
			name:    "X-Forwarded-Host overrides Host",
			host:    "internal:8080",
			fwdHost: "public.example.com",
			want:    "http://public.example.com/",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.Host = tt.host
			if tt.fwdProto != "" {
				req.Header.Set("X-Forwarded-Proto", tt.fwdProto)
			}
			if tt.fwdHost != "" {
				req.Header.Set("X-Forwarded-Host", tt.fwdHost)
			}
			if tt.tls {
				req.TLS = &tls.ConnectionState{}
			}
			got := resolvePublicURL(tt.configured, req)
			if got != tt.want {
				t.Fatalf("resolvePublicURL() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBuildHreflang(t *testing.T) {
	result := buildHreflang("https://example.com/")
	for _, lang := range hreflangLangs {
		if !strings.Contains(result, `hreflang="`+lang+`"`) {
			t.Fatalf("buildHreflang missing hreflang=%q", lang)
		}
	}
	if !strings.Contains(result, `hreflang="x-default"`) {
		t.Fatal("buildHreflang missing x-default")
	}
	if !strings.Contains(result, `href="https://example.com/"`) {
		t.Fatal("buildHreflang missing expected href")
	}
}

func TestServeIndexInjectsPlaceholders(t *testing.T) {
	const indexHTML = `<head>` +
		`<meta property="og:url" content="__PUBLIC_URL__" />` +
		`<!-- HREFLANG_TAGS -->` +
		`</head>`
	fsys := fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte(indexHTML)},
	}

	tests := []struct {
		name        string
		configured  string
		reqHost     string
		wantURL     string
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

			serveIndex(fsys, rr, req, tt.configured)

			body := rr.Body.String()
			if strings.Contains(body, "__PUBLIC_URL__") {
				t.Fatal("__PUBLIC_URL__ placeholder was not replaced")
			}
			if strings.Contains(body, "<!-- HREFLANG_TAGS -->") {
				t.Fatal("<!-- HREFLANG_TAGS --> placeholder was not replaced")
			}
			if !strings.Contains(body, `content="`+tt.wantURL+`"`) {
				t.Fatalf("og:url not set to %q in body: %s", tt.wantURL, body)
			}
			if !strings.Contains(body, `hreflang="en"`) {
				t.Fatal("hreflang tags not injected")
			}
		})
	}
}

func firstAssetName(t *testing.T, fsys fs.FS) (string, bool) {
	t.Helper()
	entries, err := fs.ReadDir(fsys, "assets")
	if err != nil {
		return "", false
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			return entry.Name(), true
		}
	}
	return "", false
}
