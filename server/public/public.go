//go:build !nogui

package public

import (
	"bytes"
	"embed"
	"fmt"
	"io/fs"
	"net/http"
	"path"
	"strings"
	"sync"
	"time"
)

//go:embed dist
var distFS embed.FS

var (
	distOnce sync.Once
	distSub  fs.FS
	distErr  error
)

// Must match the locale catalogs shipped in ui-public/src/i18n (see
// TestHreflangLangsMatchShippedLocales).
var hreflangLangs = []string{"cs", "da", "de", "en", "es", "fi", "fr", "ja", "nb", "nl", "sl", "sv"}

func resolvePublicURL(configured string, r *http.Request) string {
	if configured != "" {
		return configured
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if proto := r.Header.Get("X-Forwarded-Proto"); proto == "https" || proto == "http" {
		scheme = proto
	}
	host := r.Host
	if fwdHost := r.Header.Get("X-Forwarded-Host"); fwdHost != "" {
		host = fwdHost
	}
	return scheme + "://" + host + "/"
}

func buildHreflang(baseURL string) string {
	var b strings.Builder
	for _, lang := range hreflangLangs {
		fmt.Fprintf(&b, "    <link rel=\"alternate\" hreflang=\"%s\" href=\"%s\" />\n", lang, baseURL)
	}
	fmt.Fprintf(&b, "    <link rel=\"alternate\" hreflang=\"x-default\" href=\"%s\" />", baseURL)
	return b.String()
}

const noEmbeddedUIPage = `<!doctype html>
<html lang="en">
  <head>
    <meta charset="utf-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1" />
    <title>Gonemaster Public UI Unavailable</title>
  </head>
  <body>
    <h1>Gonemaster public UI is not embedded in this binary.</h1>
    <p>Run <code>make ui-build</code> before building the server binary to include the public web app.</p>
  </body>
</html>
`

// Handler serves the embedded public UI with a basic SPA fallback.
// publicURL is the canonical base URL of the deployment (e.g. "https://example.com/");
// leave empty to auto-detect from the request's Host and X-Forwarded-Proto headers.
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
			if strings.HasPrefix(cleanPath, "assets/") {
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			}
			serveFile(fileServer, w, r, cleanPath)
			return
		}

		serveIndex(fsys, w, r, publicURL)
	})
}

func dist() (fs.FS, error) {
	distOnce.Do(func() {
		distSub, distErr = fs.Sub(distFS, "dist")
	})
	return distSub, distErr
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
	url := resolvePublicURL(publicURL, r)
	data = bytes.ReplaceAll(data, []byte("__PUBLIC_URL__"), []byte(url))
	data = bytes.ReplaceAll(data, []byte("<!-- HREFLANG_TAGS -->"), []byte(buildHreflang(url)))
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
