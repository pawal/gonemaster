package spatest

import (
	"net/http"
	"path"
	"strings"
	"testing"
	"testing/fstest"

	"codeberg.org/pawal/gonemaster/internal/tbtest"
)

const testIndex = `<!doctype html><html><head>` +
	`<link rel="icon" href="/favicon.svg">` +
	`<link rel="icon" href="/favicon-32x32.png">` +
	`<link rel="icon" href="/favicon-16x16.png">` +
	`<link rel="apple-touch-icon" href="/apple-touch-icon.png">` +
	`<link rel="manifest" href="/site.webmanifest">` +
	`</head><body>app</body></html>`

// testFS mirrors a built dist: an index, one hashed asset and the icon set.
func testFS() fstest.MapFS {
	fsys := fstest.MapFS{
		"index.html":        &fstest.MapFile{Data: []byte(testIndex)},
		"assets/app.abc.js": &fstest.MapFile{Data: []byte("console.log(1)")},
	}
	for _, name := range faviconFiles {
		fsys[name] = &fstest.MapFile{Data: []byte(name)}
	}
	return fsys
}

// spaHandler is the behaviour the three real handlers share: GET only, files
// from the dist, the index for everything else.
func spaHandler(fsys fstest.MapFS) http.Handler {
	files := http.FileServer(http.FS(fsys))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		name := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
		if name != "" && !strings.Contains(name, "..") {
			if _, ok := fsys[name]; ok {
				if strings.HasPrefix(name, "assets/") {
					w.Header().Set("Cache-Control", cacheControl)
				}
				files.ServeHTTP(w, r)
				return
			}
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(testIndex))
	})
}

// cleanPath is the implementation the three packages share.
func cleanPath(requestPath string) string {
	if requestPath == "" {
		return ""
	}
	cleaned := strings.TrimPrefix(path.Clean("/"+requestPath), "/")
	if strings.Contains(cleaned, "..") {
		return ""
	}
	return cleaned
}

func TestChecksPassForAConformingHandler(t *testing.T) {
	fsys := testFS()
	h := spaHandler(fsys)

	MethodNotAllowed(t, h)
	IndexForRootAndUnknownPaths(t, h, "/deep/spa/route")
	PathTraversalCannotEscape(t, h, "/../spatest.go", "/assets/../spatest.go")
	CleanPath(t, cleanPath, PathCase{In: "/assets/app.js", Want: "assets/app.js"})
	AssetsHaveImmutableCacheControl(t, h, fsys)
	FaviconFilesAndManifest(t, h, fsys)
	IndexLinksFavicons(t, h, fsys, "/")
}

func TestMethodNotAllowedFailsWhenWritesAreServed(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})
	tbtest.MustFail(t, "want 405", func(tb *tbtest.TB) { MethodNotAllowed(tb, h) })
}

func TestIndexForRootAndUnknownPathsFailsOn404(t *testing.T) {
	fsys := testFS()
	files := http.FileServer(http.FS(fsys))
	tbtest.MustFail(t, "status = 404", func(tb *tbtest.TB) {
		IndexForRootAndUnknownPaths(tb, files, "/deep/spa/route")
	})
}

func TestPathTraversalCannotEscapeReportsAnEscape(t *testing.T) {
	fsys := testFS()
	fsys["secret.txt"] = &fstest.MapFile{Data: []byte("secret")}
	leaky := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "secret") {
			_, _ = w.Write([]byte("secret"))
			return
		}
		spaHandler(fsys).ServeHTTP(w, r)
	})

	tb := &tbtest.TB{}
	PathTraversalCannotEscape(tb, leaky, "/../secret.txt")
	if len(tb.Errs) != 1 {
		t.Fatalf("errors = %v, want one escape reported", tb.Errs)
	}
}

func TestCleanPathReportsEveryMismatch(t *testing.T) {
	// Without the ".." reject, the two cases path.Clean cannot collapse fail.
	leaky := func(requestPath string) string {
		return strings.TrimPrefix(path.Clean("/"+requestPath), "/")
	}
	tb := &tbtest.TB{}
	CleanPath(tb, leaky)
	if len(tb.Errs) != 2 {
		t.Fatalf("errors = %v, want the two dot-segment cases", tb.Errs)
	}
}

func TestIndexLinksFaviconsFailsOnTheWrongPrefix(t *testing.T) {
	fsys := testFS()
	h := spaHandler(fsys)
	tbtest.MustFail(t, "/public/favicon.svg", func(tb *tbtest.TB) {
		IndexLinksFavicons(tb, h, fsys, "/public/")
	})
}

func TestNoUIChecksAcceptTheFallbackHandler(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte("<h1>UI is not embedded in this binary.</h1>"))
	})

	NoUIIndexPage(t, h)
	NoUIAssetNotFound(t, h, "/assets/index.js")
}
