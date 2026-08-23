//go:build nogui
// +build nogui

package analysisui

import (
	"fmt"
	"net/http"
	"strings"
)

const noUIPage = `<!doctype html>
<html lang="en">
  <head>
    <meta charset="utf-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1" />
    <title>Gonemaster Analysis UI Unavailable</title>
  </head>
  <body>
    <h1>Gonemaster analysis UI is not embedded in this binary.</h1>
    <p>Build the server without the <code>nogui</code> tag (or run <code>make ui-build</code> first) to include the analysis UI.</p>
  </body>
</html>
`

// Handler returns a minimal info page for API-only builds. The publicURL
// argument is accepted for signature parity with the default build.
func Handler(_ string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		p := strings.TrimSpace(r.URL.Path)
		if p == "" || p == "/" || p == "/index.html" {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusOK)
			if r.Method != http.MethodHead {
				_, _ = fmt.Fprint(w, noUIPage)
			}
			return
		}

		http.Error(w, "ui not available", http.StatusNotFound)
	})
}
