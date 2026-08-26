//go:build !nogui

package public

import (
	"bytes"
	"embed"
	"fmt"
	"html"
	"io/fs"
	"net/http"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"codeberg.org/pawal/gonemaster/server/internal/baseurl"
)

//go:embed dist
var distFS embed.FS

var (
	distOnce sync.Once
	distSub  fs.FS
	distErr  error
)

// Every alternate names a distinct URL that really answers in that language,
// and each of those pages repeats this same set, which is what makes the block
// a language signal rather than noise.
func buildHreflang(site Site) string {
	var b strings.Builder
	for _, lang := range hreflangLangs {
		fmt.Fprintf(&b, "    <link rel=\"alternate\" hreflang=\"%s\" href=\"%s\" />\n",
			lang, html.EscapeString(site.Locale(lang)))
	}
	fmt.Fprintf(&b, "    <link rel=\"alternate\" hreflang=\"x-default\" href=\"%s\" />",
		html.EscapeString(site.Home()))
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
// uiPath is where visitors reach the UI under publicURL (see DefaultUIPath).
// lookup renders result pages for non-scripting clients; nil disables that.
func Handler(publicURL, uiPath string, lookup LookupResult) http.Handler {
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
			serveIndex(fsys, w, r, publicURL, uiPath, nil)
			return
		}

		if isFile(fsys, cleanPath) {
			if strings.HasPrefix(cleanPath, "assets/") {
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			}
			serveFile(fileServer, w, r, cleanPath)
			return
		}

		serveIndex(fsys, w, r, publicURL, uiPath, lookup)
	})
}

// resultID returns the public id a cleaned SPA path asks for, or "".
func resultID(cleanPath string) string {
	rest, ok := strings.CutPrefix(cleanPath, "result/")
	if !ok {
		return ""
	}
	rest = strings.TrimSuffix(rest, "/")
	if rest == "" || strings.Contains(rest, "/") {
		return ""
	}
	for _, c := range rest {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z') {
			return ""
		}
	}
	return rest
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

// page holds the per-request substitutions for index.html.
type page struct {
	title       string
	description string
	lang        string
	url         string // og:url and canonical, which must never disagree
	robots      string
	hreflang    string
	summary     string
}

func homePage(site Site, locale string) page {
	if locale == "" {
		locale = "en"
	}
	return page{
		title:       "Gonemaster",
		description: textFor(locale).homeDescription,
		lang:        locale,
		url:         site.Locale(locale),
		hreflang:    buildHreflang(site),
	}
}

func render(data []byte, site Site, p page) []byte {
	canonical := ""
	if p.url != "" {
		canonical = `<link rel="canonical" href="` + html.EscapeString(p.url) + `" />`
	}
	for _, sub := range [][2]string{
		{"__PUBLIC_URL__", site.base},
		{"__UI_BASE__", html.EscapeString(site.ClientBase())},
		{"__PAGE_TITLE__", html.EscapeString(p.title)},
		{"__PAGE_DESCRIPTION__", html.EscapeString(p.description)},
		{"__PAGE_LANG__", html.EscapeString(p.lang)},
		{"__OG_URL__", html.EscapeString(p.url)},
		{"<!-- CANONICAL -->", canonical},
		{"<!-- ROBOTS_TAG -->", p.robots},
		{"<!-- HREFLANG_TAGS -->", p.hreflang},
		{"<!-- RESULT_SUMMARY -->", p.summary},
	} {
		data = bytes.ReplaceAll(data, []byte(sub[0]), []byte(sub[1]))
	}
	if p.summary != "" {
		return cutRegion(data, genericStart, genericEnd)
	}
	for _, marker := range []string{genericStart, genericEnd} {
		data = bytes.ReplaceAll(data, []byte(marker), nil)
	}
	return data
}

const (
	genericStart = "<!-- NOSCRIPT_GENERIC_START -->"
	genericEnd   = "<!-- NOSCRIPT_GENERIC_END -->"
)

// cutRegion removes start..end inclusive, leaving the data alone if either
// marker is missing.
func cutRegion(data []byte, start, end string) []byte {
	i := bytes.Index(data, []byte(start))
	if i < 0 {
		return data
	}
	j := bytes.Index(data[i:], []byte(end))
	if j < 0 {
		return data
	}
	return append(data[:i:i], data[i+j+len(end):]...)
}

func serveIndex(fsys fs.FS, w http.ResponseWriter, r *http.Request, publicURL, uiPath string, lookup LookupResult) {
	data, err := fs.ReadFile(fsys, "index.html")
	if err != nil {
		serveUnavailableUIPage(w, r)
		return
	}
	site := NewSite(baseurl.Resolve(publicURL, r), uiPath)

	if id := resultID(cleanRequestPath(r.URL.Path)); id != "" && lookup != nil {
		serveResult(data, w, r, site, id, lookup)
		return
	}

	w.Header().Set("Cache-Control", "no-cache")
	modTime := time.Time{}
	if info, err := fs.Stat(fsys, "index.html"); err == nil {
		modTime = info.ModTime()
	}
	// Only ?lang, never Accept-Language: the home shell stays one cacheable
	// response per URL, and each locale has its own URL.
	page := homePage(site, matchLocale(r.URL.Query().Get("lang")))
	http.ServeContent(w, r, "index.html", modTime, bytes.NewReader(render(data, site, page)))
}

// serveResult renders one result page. Results name domains someone chose to
// test, so they carry noindex and never reach a search index.
func serveResult(data []byte, w http.ResponseWriter, r *http.Request, site Site, id string, lookup LookupResult) {
	locale := negotiateLocale(r.URL.Query().Get("lang"), r.Header.Get("Accept-Language"))
	summary, status := lookup(id, locale)

	p := homePage(site, locale)
	p.url = site.Result(id)
	p.robots = `<meta name="robots" content="noindex, nofollow" />`
	p.hreflang = ""

	code := http.StatusOK
	switch status {
	case LookupFound:
		p.title = summaryTitle(summary, locale)
		p.description = summaryDescription(summary, locale)
		p.summary = renderSummary(summary, locale)
		w.Header().Set("Cache-Control", "public, max-age=300")
		w.Header().Add("Vary", "Accept-Language")
	case LookupNotFound:
		code = http.StatusNotFound
		w.Header().Set("Cache-Control", "no-cache")
	default:
		w.Header().Set("Cache-Control", "no-cache")
	}

	body := render(data, site, p)
	w.Header().Set("X-Robots-Tag", "noindex")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	w.WriteHeader(code)
	if r.Method != http.MethodHead {
		_, _ = w.Write(body)
	}
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
