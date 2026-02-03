package ui

import (
	"bytes"
	"embed"
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

// Handler serves the embedded UI with a basic SPA fallback.
func Handler() http.Handler {
	fsys, err := dist()
	if err != nil {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "ui not available", http.StatusNotFound)
		})
	}
	fileServer := http.FileServer(http.FS(fsys))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		cleanPath := cleanRequestPath(r.URL.Path)
		if cleanPath == "" || cleanPath == "index.html" {
			serveIndex(fsys, w, r)
			return
		}

		if isFile(fsys, cleanPath) {
			if strings.HasPrefix(cleanPath, "assets/") {
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			}
			serveFile(fileServer, w, r, cleanPath)
			return
		}

		serveIndex(fsys, w, r)
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

func serveIndex(fsys fs.FS, w http.ResponseWriter, r *http.Request) {
	data, err := fs.ReadFile(fsys, "index.html")
	if err != nil {
		http.Error(w, "ui not available", http.StatusNotFound)
		return
	}
	modTime := time.Time{}
	if info, err := fs.Stat(fsys, "index.html"); err == nil {
		modTime = info.ModTime()
	}
	http.ServeContent(w, r, "index.html", modTime, bytes.NewReader(data))
}

func serveFile(fileServer http.Handler, w http.ResponseWriter, r *http.Request, name string) {
	clone := r.Clone(r.Context())
	clone.URL.Path = "/" + name
	fileServer.ServeHTTP(w, clone)
}
