//go:build !nogui

// Package analysisui serves the embedded cohort analysis dashboard SPA.
package analysisui

import (
	"bytes"
	"embed"
	"fmt"
	"html"
	"io/fs"
	"net/http"
	"path"
	"strings"
	"sync"
	"time"

	"codeberg.org/pawal/gonemaster/server/internal/baseurl"
)

// mountPath is where the dashboard is served under; used to build absolute
// canonical and Open Graph URLs from the request path.
const mountPath = "/analysis"

//go:embed all:dist
var distFS embed.FS

var (
	distOnce sync.Once
	distSub  fs.FS
	distErr  error
)

const noEmbeddedUIPage = `<!doctype html>
<html lang="en">
  <head>
    <meta charset="utf-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1" />
    <title>Gonemaster Analysis UI Unavailable</title>
  </head>
  <body>
    <h1>Gonemaster analysis dashboard is not embedded in this binary.</h1>
    <p>Run <code>make ui-build</code> before building the server binary to include the analysis UI.</p>
  </body>
</html>
`

// Handler serves the embedded analysis dashboard with SPA fallback. Unknown
// paths under the mount resolve to index.html so the client-side router can
// handle them. publicURL is the canonical base URL of the deployment (e.g.
// "https://example.com/"); empty means auto-detect from the request.
func Handler(publicURL string) http.Handler {
	fsys, err := dist()
	if err != nil {
		return unavailableUIHandler()
	}
	fileServer := http.FileServer(http.FS(fsys))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		cleanPath := cleanRequestPath(r.URL.Path)
		if cleanPath == "" || cleanPath == "index.html" {
			serveIndex(fsys, w, r, publicURL)
			return
		}

		if isFile(fsys, cleanPath) {
			if strings.HasPrefix(cleanPath, "_app/") {
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			} else {
				w.Header().Set("Cache-Control", "public, max-age=3600")
			}
			serveFile(fileServer, w, r, cleanPath)
			return
		}

		serveIndex(fsys, w, r, publicURL)
	})
}

// injectMeta fills the canonical/Open Graph placeholders in index.html with
// URLs derived from the request. Non-JS crawlers rely on these; the client
// refines them per route once the SPA hydrates.
func injectMeta(data []byte, ogURL, ogImage string) []byte {
	data = bytes.ReplaceAll(data, []byte("__ANALYSIS_OG_URL__"), []byte(ogURL))
	data = bytes.ReplaceAll(data, []byte("__ANALYSIS_OG_IMAGE__"), []byte(ogImage))
	return data
}

func dist() (fs.FS, error) {
	distOnce.Do(func() {
		distSub, distErr = fs.Sub(distFS, "dist")
	})
	return distSub, distErr
}

// IsBuilt reports whether the analysis UI assets have been embedded.
// Returns false when only the placeholder was committed (e.g. CI without make ui-build).
func IsBuilt() bool {
	fsys, err := dist()
	if err != nil {
		return false
	}
	_, err = fs.Stat(fsys, "index.html")
	return err == nil
}

func cleanRequestPath(requestPath string) string {
	if requestPath == "" {
		return ""
	}
	cleaned := path.Clean("/" + requestPath)
	cleaned = strings.TrimPrefix(cleaned, "/")
	if strings.Contains(cleaned, "..") {
		return ""
	}
	return cleaned
}

func isFile(fsys fs.FS, name string) bool {
	info, err := fs.Stat(fsys, name)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

func serveIndex(fsys fs.FS, w http.ResponseWriter, r *http.Request, publicURL string) {
	data, err := fs.ReadFile(fsys, "index.html")
	if err != nil {
		serveUnavailableUIPage(w, r)
		return
	}
	// Inject only the deployment base, not the request path: the path is
	// attacker-controlled and would be reflected into an HTML attribute. The
	// client refines canonical/og:url per route once the SPA hydrates.
	base := strings.TrimRight(baseurl.Resolve(publicURL, r), "/")
	ogURL := html.EscapeString(base + mountPath + "/")
	ogImage := html.EscapeString(base + mountPath + "/og-image.png")
	data = injectMeta(data, ogURL, ogImage)
	modTime := time.Time{}
	if info, err := fs.Stat(fsys, "index.html"); err == nil {
		modTime = info.ModTime()
	}
	w.Header().Set("Cache-Control", "no-cache")
	http.ServeContent(w, r, "index.html", modTime, bytes.NewReader(data))
}

func serveFile(fileServer http.Handler, w http.ResponseWriter, r *http.Request, name string) {
	clone := r.Clone(r.Context())
	clone.URL.Path = "/" + name
	fileServer.ServeHTTP(w, clone)
}

func unavailableUIHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		serveUnavailableUIPage(w, r)
	})
}

func serveUnavailableUIPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	if r.Method != http.MethodHead {
		_, _ = fmt.Fprint(w, noEmbeddedUIPage)
	}
}
