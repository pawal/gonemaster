package server

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func gzipTestServer(handler http.HandlerFunc) http.Handler {
	return gzipMiddleware(handler)
}

func runGzip(t *testing.T, h http.Handler, acceptEncoding string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/x", nil)
	if acceptEncoding != "" {
		r.Header.Set("Accept-Encoding", acceptEncoding)
	}
	h.ServeHTTP(w, r)
	return w
}

func TestGzipCompressesLargeJSON(t *testing.T) {
	body := strings.Repeat(`{"key":"value"},`, 100)
	h := gzipTestServer(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, body)
	})

	resp := runGzip(t, h, "gzip")
	if got := resp.Header().Get("Content-Encoding"); got != "gzip" {
		t.Fatalf("Content-Encoding = %q, want gzip", got)
	}
	if !strings.Contains(resp.Header().Get("Vary"), "Accept-Encoding") {
		t.Errorf("Vary header missing Accept-Encoding, got %q", resp.Header().Get("Vary"))
	}
	zr, err := gzip.NewReader(resp.Body)
	if err != nil {
		t.Fatalf("gzip.NewReader: %v", err)
	}
	got, err := io.ReadAll(zr)
	if err != nil {
		t.Fatalf("read gzipped body: %v", err)
	}
	if string(got) != body {
		t.Fatalf("decompressed body mismatch\n got: %q\nwant: %q", got, body)
	}
}

func TestGzipPassThroughWhenClientDoesNotAcceptGzip(t *testing.T) {
	body := strings.Repeat(`{"key":"value"},`, 100)
	h := gzipTestServer(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, body)
	})

	resp := runGzip(t, h, "")
	if got := resp.Header().Get("Content-Encoding"); got != "" {
		t.Fatalf("Content-Encoding = %q, want empty", got)
	}
	if !strings.Contains(resp.Header().Get("Vary"), "Accept-Encoding") {
		t.Errorf("Vary header missing Accept-Encoding, got %q", resp.Header().Get("Vary"))
	}
	if resp.Body.String() != body {
		t.Fatalf("body altered when no compression should have applied")
	}
}

func TestGzipSkipsTinyResponses(t *testing.T) {
	body := `{"ok":true}`
	h := gzipTestServer(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, body)
	})

	resp := runGzip(t, h, "gzip")
	if got := resp.Header().Get("Content-Encoding"); got != "" {
		t.Fatalf("tiny response was compressed: Content-Encoding = %q", got)
	}
	if resp.Body.String() != body {
		t.Fatalf("body altered: %q", resp.Body.String())
	}
}

func TestGzipSkipsBinaryContentTypes(t *testing.T) {
	body := bytes.Repeat([]byte{0x89, 0x50, 0x4E, 0x47}, 200)
	h := gzipTestServer(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(body)
	})

	resp := runGzip(t, h, "gzip")
	if got := resp.Header().Get("Content-Encoding"); got != "" {
		t.Fatalf("binary response was compressed: Content-Encoding = %q", got)
	}
	if !bytes.Equal(resp.Body.Bytes(), body) {
		t.Fatalf("binary body altered")
	}
}

func TestGzipRespectsExistingContentEncoding(t *testing.T) {
	body := strings.Repeat("payload-", 100)
	h := gzipTestServer(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Encoding", "br")
		_, _ = io.WriteString(w, body)
	})

	resp := runGzip(t, h, "gzip")
	if got := resp.Header().Get("Content-Encoding"); got != "br" {
		t.Fatalf("Content-Encoding = %q, want br (left alone)", got)
	}
	if resp.Body.String() != body {
		t.Fatalf("body altered when upstream already encoded it")
	}
}

func TestGzipPassesThroughNotModified(t *testing.T) {
	h := gzipTestServer(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotModified)
	})

	resp := runGzip(t, h, "gzip")
	if resp.Code != http.StatusNotModified {
		t.Fatalf("status = %d, want 304", resp.Code)
	}
	if got := resp.Header().Get("Content-Encoding"); got != "" {
		t.Fatalf("304 had Content-Encoding = %q", got)
	}
}

func TestAcceptsGzipParsing(t *testing.T) {
	cases := map[string]bool{
		"":                      false,
		"identity":              false,
		"gzip":                  true,
		"deflate, gzip":         true,
		"gzip;q=1.0, deflate":   true,
		"GZIP":                  true,
		"deflate":               false,
		"gzipfoo":               false,
		"foo, bar; q=0.5, gzip": true,
	}
	for in, want := range cases {
		if got := acceptsGzip(in); got != want {
			t.Errorf("acceptsGzip(%q) = %v, want %v", in, got, want)
		}
	}
}
