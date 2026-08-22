package spatest

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// cacheControl is what an SPA must send for a content-hashed asset.
const cacheControl = "public, max-age=31536000, immutable"

// faviconFiles are the icon and manifest files every SPA embeds.
var faviconFiles = []string{
	"favicon.svg",
	"favicon-16x16.png",
	"favicon-32x32.png",
	"apple-touch-icon.png",
	"android-chrome-192x192.png",
	"android-chrome-512x512.png",
	"site.webmanifest",
}

// serve runs one request through h and returns the recorder.
func serve(h http.Handler, method string, target string) *httptest.ResponseRecorder {
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(method, target, nil))
	return rr
}

// MethodNotAllowed asserts a write method is rejected rather than served.
func MethodNotAllowed(t testing.TB, h http.Handler) {
	t.Helper()
	rr := serve(h, http.MethodPost, "/")
	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusMethodNotAllowed)
	}
}

// IndexBody returns the body served for "/" and asserts it is a non-empty page.
func IndexBody(t testing.TB, h http.Handler) string {
	t.Helper()
	rr := serve(h, http.MethodGet, "/")
	if rr.Code != http.StatusOK {
		t.Fatalf("root status = %d, want %d", rr.Code, http.StatusOK)
	}
	if rr.Body.Len() == 0 {
		t.Fatalf("root response body is empty")
	}
	if ct := rr.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Fatalf("root Content-Type = %q, want text/html", ct)
	}
	return rr.Body.String()
}

// IndexForRootAndUnknownPaths asserts an unrouted SPA path serves the index.
func IndexForRootAndUnknownPaths(t testing.TB, h http.Handler, unknownPath string) {
	t.Helper()
	root := IndexBody(t, h)

	rr := serve(h, http.MethodGet, unknownPath)
	if rr.Code != http.StatusOK {
		t.Fatalf("%s: status = %d, want %d", unknownPath, rr.Code, http.StatusOK)
	}
	if rr.Body.String() != root {
		t.Fatalf("%s did not serve the SPA index content", unknownPath)
	}
}

// PathTraversalCannotEscape asserts each target falls through to the SPA index
// instead of reaching a file outside the embedded dist.
func PathTraversalCannotEscape(t testing.TB, h http.Handler, targets ...string) {
	t.Helper()
	root := IndexBody(t, h)

	for _, target := range targets {
		rr := serve(h, http.MethodGet, target)
		if rr.Code != http.StatusOK {
			t.Errorf("%s: status = %d, want 200 (SPA fallback)", target, rr.Code)
			continue
		}
		if rr.Body.String() != root {
			t.Errorf("%s: served content other than the SPA index", target)
		}
	}
}

// PathCase is one cleanRequestPath input and the path it must yield.
type PathCase struct {
	In   string
	Want string
}

// CleanPath checks a package's cleanRequestPath against the cases all three
// share - including the dot-segment rejects - plus the caller's own.
func CleanPath(t testing.TB, clean func(string) string, extra ...PathCase) {
	t.Helper()
	cases := []PathCase{
		{In: "", Want: ""},
		{In: "/", Want: ""},
		{In: "/index.html", Want: "index.html"},
		{In: "index.html", Want: "index.html"},
		// Anything still containing ".." after normalization is rejected.
		{In: "/..", Want: ""},
		{In: "..", Want: ""},
		{In: "/..foo", Want: ""},
		{In: "/foo/..bar", Want: ""},
	}
	for _, tc := range append(cases, extra...) {
		if got := clean(tc.In); got != tc.Want {
			t.Errorf("cleanRequestPath(%q) = %q, want %q", tc.In, got, tc.Want)
		}
	}
}

// AssetsHaveImmutableCacheControl asserts a hashed build asset is served with
// the long-lived cache header. It skips when the SPA is not built.
func AssetsHaveImmutableCacheControl(t testing.TB, h http.Handler, fsys fs.FS) {
	t.Helper()
	name, ok := firstFile(fsys, "assets")
	if !ok {
		t.Skip("no embedded asset files found under dist/assets")
	}

	rr := serve(h, http.MethodGet, "/assets/"+name)
	if rr.Code != http.StatusOK {
		t.Fatalf("asset status = %d, want %d", rr.Code, http.StatusOK)
	}
	if got := rr.Header().Get("Cache-Control"); got != cacheControl {
		t.Fatalf("Cache-Control = %q, want %q", got, cacheControl)
	}
}

// FaviconFilesAndManifest asserts every favicon file is embedded and served.
// It skips when the SPA is not built.
func FaviconFilesAndManifest(t testing.TB, h http.Handler, fsys fs.FS) {
	t.Helper()
	requireFavicons(t, fsys)

	for _, name := range faviconFiles {
		if _, err := fs.Stat(fsys, name); err != nil {
			t.Fatalf("expected embedded file %q: %v", name, err)
		}
		rr := serve(h, http.MethodGet, "/"+name)
		if rr.Code != http.StatusOK {
			t.Fatalf("GET /%s status = %d, want %d", name, rr.Code, http.StatusOK)
		}
		if rr.Body.Len() == 0 {
			t.Fatalf("GET /%s returned an empty body", name)
		}
	}
}

// IndexLinksFavicons asserts the served index links the icons under prefix,
// which is where the SPA is mounted. It skips when the SPA is not built.
func IndexLinksFavicons(t testing.TB, h http.Handler, fsys fs.FS, prefix string) {
	t.Helper()
	requireFavicons(t, fsys)

	body := IndexBody(t, h)
	for _, name := range []string{"favicon.svg", "favicon-32x32.png", "favicon-16x16.png", "apple-touch-icon.png", "site.webmanifest"} {
		snippet := `href="` + prefix + name + `"`
		if !strings.Contains(body, snippet) {
			t.Fatalf("index.html missing %q", snippet)
		}
	}
}

// NoUIIndexPage asserts the nogui handler explains that the UI is missing.
func NoUIIndexPage(t testing.TB, h http.Handler) {
	t.Helper()
	body := IndexBody(t, h)
	if !strings.Contains(body, "UI is not embedded") {
		t.Fatalf("expected fallback message in body, got %q", body)
	}
}

// NoUIAssetNotFound asserts the nogui handler serves no assets.
func NoUIAssetNotFound(t testing.TB, h http.Handler, assetPath string) {
	t.Helper()
	rr := serve(h, http.MethodGet, assetPath)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusNotFound)
	}
}

// requireFavicons skips unless the icon set is embedded.
func requireFavicons(t testing.TB, fsys fs.FS) {
	t.Helper()
	if _, err := fs.Stat(fsys, "site.webmanifest"); err != nil {
		t.Skip("favicon assets are not embedded; run make ui-build")
	}
}

// firstFile returns the name of the first regular file in dir.
func firstFile(fsys fs.FS, dir string) (string, bool) {
	entries, err := fs.ReadDir(fsys, dir)
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
