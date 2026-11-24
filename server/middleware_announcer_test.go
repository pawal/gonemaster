//go:build !nogui

package server

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"testing"

	"codeberg.org/pawal/gonemaster/server/analysisui"
)

// SvelteKit injects #svelte-announcer with an inline `style="..."` attribute
// to keep its route-change a11y div visually hidden. The analysis CSP
// whitelists exactly that style via a SHA-256 hash (announcerStyleHash). If
// SvelteKit ever changes the announcer template, the hash drifts and CSP
// blocks the inline style at runtime - this test catches the drift at build
// time and tells you the new constant to paste in.
func TestAnalysisAnnouncerHashMatchesDist(t *testing.T) {
	if !analysisui.IsBuilt() {
		t.Skip("analysis UI not built (run make ui-build); skipping CSP hash check")
	}
	got := extractAnalysisAnnouncerStyle(t)
	sum := sha256.Sum256([]byte(got))
	want := fmt.Sprintf("'sha256-%s'", base64.StdEncoding.EncodeToString(sum[:]))

	if want != announcerStyleHash {
		t.Fatalf(`SvelteKit's #svelte-announcer inline style changed; CSP will block it.

Update server/server.go:
    const announcerStyleHash = %q

Found inline style in the embedded analysis-ui bundle:
    %s

(Hash above is SHA-256 of that exact string, base64-encoded, formatted for CSP style-src.)`, want, got)
	}
}

// extractAnalysisAnnouncerStyle reads the #svelte-announcer inline style from the chunks index.html references.
func extractAnalysisAnnouncerStyle(t *testing.T) string {
	t.Helper()
	h := analysisui.Handler(nil)

	indexBody := fetch(t, h, "/index.html")
	chunks := regexp.MustCompile(`/_app/immutable/[A-Za-z0-9_/.-]+\.js`).FindAllString(indexBody, -1)
	if len(chunks) == 0 {
		t.Fatalf("no immutable script paths in analysis-ui index.html; bundle layout changed?")
	}

	styleRE := regexp.MustCompile(`id="svelte-announcer"[^>]*style="([^"]*)"`)
	for _, chunk := range chunks {
		if m := styleRE.FindStringSubmatch(fetch(t, h, chunk)); len(m) == 2 {
			return m[1]
		}
	}
	t.Fatalf("no #svelte-announcer inline style in the %d chunks index.html references; SvelteKit template changed?", len(chunks))
	return ""
}

func fetch(t *testing.T, h http.Handler, path string) string {
	t.Helper()
	w := doHandler(t, h, http.MethodGet, path, nil)
	wantStatus(t, w, http.StatusOK)
	body, err := io.ReadAll(w.Body)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(body)
}

func TestAnalysisCSPAllowsTheServedBootstrapScript(t *testing.T) {
	if !analysisui.IsBuilt() {
		t.Skip("analysis UI not built (run make ui-build); skipping CSP script hash check")
	}
	srv := newTestServer(t)
	resp := doJSON(t, srv, http.MethodGet, "/analysis/", nil)
	scripts := regexp.MustCompile(`(?s)<script>(.*?)</script>`).FindAllStringSubmatch(resp.Body.String(), -1)
	if len(scripts) == 0 {
		t.Fatal("served /analysis/ has no inline bootstrap script")
	}
	csp := resp.Header().Get("Content-Security-Policy")
	for _, m := range scripts {
		sum := sha256.Sum256([]byte(m[1]))
		if want := "'sha256-" + base64.StdEncoding.EncodeToString(sum[:]) + "'"; !strings.Contains(csp, want) {
			t.Errorf("CSP %q lacks %s for the served bootstrap script", csp, want)
		}
	}
}
