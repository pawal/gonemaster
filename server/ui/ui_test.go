package ui

import (
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCleanRequestPath(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{in: "", want: ""},
		{in: "/", want: ""},
		{in: "/index.html", want: "index.html"},
		{in: "index.html", want: "index.html"},
		{in: "/assets/app.js", want: "assets/app.js"},
	}

	for _, tt := range tests {
		if got := cleanRequestPath(tt.in); got != tt.want {
			t.Fatalf("cleanRequestPath(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestHandlerMethodNotAllowed(t *testing.T) {
	h := Handler()

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandlerServesIndexForRootAndUnknownPaths(t *testing.T) {
	h := Handler()

	rootReq := httptest.NewRequest(http.MethodGet, "/", nil)
	rootRR := httptest.NewRecorder()
	h.ServeHTTP(rootRR, rootReq)
	if rootRR.Code != http.StatusOK {
		t.Fatalf("root status = %d, want %d", rootRR.Code, http.StatusOK)
	}
	rootBody, err := io.ReadAll(rootRR.Result().Body)
	if err != nil {
		t.Fatalf("read root body: %v", err)
	}
	if len(rootBody) == 0 {
		t.Fatal("root response body is empty")
	}

	unknownReq := httptest.NewRequest(http.MethodGet, "/not/a/real/path", nil)
	unknownRR := httptest.NewRecorder()
	h.ServeHTTP(unknownRR, unknownReq)
	if unknownRR.Code != http.StatusOK {
		t.Fatalf("unknown path status = %d, want %d", unknownRR.Code, http.StatusOK)
	}
	unknownBody, err := io.ReadAll(unknownRR.Result().Body)
	if err != nil {
		t.Fatalf("read unknown path body: %v", err)
	}
	if string(unknownBody) != string(rootBody) {
		t.Fatal("unknown path did not serve the SPA index content")
	}
}

func TestHandlerServesAssetsWithCacheControl(t *testing.T) {
	fsys, err := dist()
	if err != nil {
		t.Fatalf("dist(): %v", err)
	}
	assetName, ok := firstAssetName(t, fsys)
	if !ok {
		t.Skip("no embedded asset files found under dist/assets")
	}

	h := Handler()
	req := httptest.NewRequest(http.MethodGet, "/assets/"+assetName, nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("asset status = %d, want %d", rr.Code, http.StatusOK)
	}
	const wantCacheControl = "public, max-age=31536000, immutable"
	if got := rr.Header().Get("Cache-Control"); got != wantCacheControl {
		t.Fatalf("Cache-Control = %q, want %q", got, wantCacheControl)
	}
}

func firstAssetName(t *testing.T, fsys fs.FS) (string, bool) {
	t.Helper()
	entries, err := fs.ReadDir(fsys, "assets")
	if err != nil {
		return "", false
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			return entry.Name(), true
		}
	}
	return "", false
}
