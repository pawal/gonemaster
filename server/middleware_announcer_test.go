//go:build !nogui

package server

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"regexp"
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

// extractAnalysisAnnouncerStyle pulls the inline style attribute of
// #svelte-announcer out of the embedded SvelteKit entry chunk by walking
// the same Handler the production server uses.
func extractAnalysisAnnouncerStyle(t *testing.T) string {
	t.Helper()
	h := analysisui.Handler("")

	indexBody := fetch(t, h, "/index.html")
	entryRE := regexp.MustCompile(`/_app/immutable/entry/app\.[A-Za-z0-9_-]+\.js`)
	match := entryRE.FindString(indexBody)
	if match == "" {
		t.Fatalf("could not find entry script path in analysis-ui index.html; bundle layout changed?")
	}

	chunkBody := fetch(t, h, match)
	styleRE := regexp.MustCompile(`id="svelte-announcer"[^>]*style="([^"]*)"`)
	m := styleRE.FindStringSubmatch(chunkBody)
	if len(m) != 2 {
		t.Fatalf("could not find #svelte-announcer inline style in %s; SvelteKit template changed?", match)
	}
	return m[1]
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
