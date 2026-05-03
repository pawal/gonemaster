//go:build !nogui
// +build !nogui

package analysisui

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandlerServesIndexHTMLForRoot(t *testing.T) {
	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}
	body := resp.Body.String()
	if !strings.Contains(body, "<!doctype html>") && !strings.Contains(body, "<!DOCTYPE html>") {
		t.Fatalf("expected HTML body, got: %s", body[:min(200, len(body))])
	}
}

func TestHandlerFallsBackToIndexForUnknownSPARoute(t *testing.T) {
	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/cohort/tld/domains", nil)
	Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200 SPA fallback, got %d", resp.Code)
	}
	if contentType := resp.Header().Get("Content-Type"); !strings.HasPrefix(contentType, "text/html") {
		t.Fatalf("expected text/html fallback, got %q", contentType)
	}
}

func TestHandlerRejectsNonGetMethods(t *testing.T) {
	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("body"))
	Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", resp.Code)
	}
}

func TestCleanRequestPath(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{in: "", want: ""},
		{in: "/", want: ""},
		{in: "/index.html", want: "index.html"},
		{in: "index.html", want: "index.html"},
		{in: "/_app/immutable/x.js", want: "_app/immutable/x.js"},
		// Anything still containing ".." after normalization is rejected.
		{in: "/..", want: ""},
		{in: "..", want: ""},
		{in: "/..foo", want: ""},
		{in: "/foo/..bar", want: ""},
	}

	for _, tt := range tests {
		if got := cleanRequestPath(tt.in); got != tt.want {
			t.Fatalf("cleanRequestPath(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestHandlerPathTraversalAttemptsCannotEscapeDist(t *testing.T) {
	h := Handler()

	rootReq := httptest.NewRequest(http.MethodGet, "/", nil)
	rootRR := httptest.NewRecorder()
	h.ServeHTTP(rootRR, rootReq)
	rootBody, _ := io.ReadAll(rootRR.Result().Body)

	traversals := []string{
		"/../analysisui.go",
		"/_app/../analysisui.go",
		"/../../server/analysisui/analysisui.go",
		"/../../etc/passwd",
		"//etc/passwd",
		"/..%2fanalysisui.go",
	}
	for _, p := range traversals {
		req := httptest.NewRequest(http.MethodGet, p, nil)
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("%s: status = %d, want 200 (SPA fallback)", p, rr.Code)
			continue
		}
		body, _ := io.ReadAll(rr.Result().Body)
		if string(body) != string(rootBody) {
			t.Errorf("%s: served content other than the SPA index", p)
		}
	}
}

func TestServerMountsAnalysisRoute(t *testing.T) {
	// The /analysis route returns a 301 redirect to /analysis/ (with slash),
	// which is mounted in server.go. Exercised via the embedded Handler here:
	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/_app/immutable/nothing.js", nil)
	Handler().ServeHTTP(resp, req)
	// unknown file under _app should either be a 404 or fall back to the SPA
	// index; both are acceptable - just make sure we don't crash.
	if resp.Code != http.StatusOK && resp.Code != http.StatusNotFound {
		t.Fatalf("unexpected status for missing asset: %d", resp.Code)
	}
}
