//go:build !nogui

package ui

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"codeberg.org/pawal/gonemaster/server/internal/spatest"
)

// mustDist returns the embedded UI files.
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

func TestHandlerMethodNotAllowed(t *testing.T) {
	spatest.MethodNotAllowed(t, Handler())
}

func TestHandlerServesIndexForRootAndUnknownPaths(t *testing.T) {
	spatest.IndexForRootAndUnknownPaths(t, Handler(), "/not/a/real/path")
}

func TestHandlerPathTraversalAttemptsCannotEscapeDist(t *testing.T) {
	spatest.PathTraversalCannotEscape(t, Handler(),
		"/../ui.go",
		"/assets/../ui.go",
		"/../../server/ui/ui.go",
		"/../../etc/passwd",
		"//etc/passwd",
		"/..%2fui.go",
	)
}

func TestHandlerServesAssetsWithCacheControl(t *testing.T) {
	spatest.AssetsHaveImmutableCacheControl(t, Handler(), mustDist(t))
}

func TestHandlerServesFaviconFilesAndManifest(t *testing.T) {
	spatest.FaviconFilesAndManifest(t, Handler(), mustDist(t))
}

func TestHandlerIndexIncludesFaviconLinks(t *testing.T) {
	spatest.IndexLinksFavicons(t, Handler(), mustDist(t), "/")
}

func TestServeIndexFallsBackToUnavailablePageWhenIndexMissing(t *testing.T) {
	fsys := fstest.MapFS{
		"placeholder.txt": &fstest.MapFile{Data: []byte("placeholder")},
	}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()

	serveIndex(fsys, rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "UI is not embedded in this binary") {
		t.Fatalf("unexpected fallback body: %q", body)
	}
}
