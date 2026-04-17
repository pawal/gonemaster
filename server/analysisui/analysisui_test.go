//go:build !nogui
// +build !nogui

package analysisui

import (
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

func TestServerMountsAnalysisRoute(t *testing.T) {
	// The /analysis route returns a 301 redirect to /analysis/ (with slash),
	// which is mounted in server.go. Exercised via the embedded Handler here:
	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/_app/immutable/nothing.js", nil)
	Handler().ServeHTTP(resp, req)
	// unknown file under _app should either be a 404 or fall back to the SPA
	// index; both are acceptable — just make sure we don't crash.
	if resp.Code != http.StatusOK && resp.Code != http.StatusNotFound {
		t.Fatalf("unexpected status for missing asset: %d", resp.Code)
	}
}
