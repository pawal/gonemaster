package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAPIRouteTemplate(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{path: "/api/v1", want: "/api/v1"},
		{path: "/api/v1/", want: "/api/v1/"},
		{path: "/api/v1/jobs", want: "/api/v1/jobs"},
		{path: "/api/v1/jobs/batch", want: "/api/v1/jobs/batch"},
		{path: "/api/v1/jobs/job-1", want: "/api/v1/jobs/{job_id}"},
		{path: "/api/v1/jobs/job-1/result", want: "/api/v1/jobs/{job_id}/result"},
		{path: "/api/v1/jobs/job-1/events", want: "/api/v1/jobs/{job_id}/events"},
		{path: "/api/v1/jobs/job-1/cancel", want: "/api/v1/jobs/{job_id}/cancel"},
		{path: "/api/v1/jobs/job-1/extra", want: "/api/v1/jobs/unknown"},
		{path: "/api/v1/batches/batch-1", want: "/api/v1/batches/{batch_id}"},
		{path: "/api/v1/queue/pause", want: "/api/v1/queue/pause"},
		{path: "/api/v1/queue/resume", want: "/api/v1/queue/resume"},
		{path: "/api/v1/queue/reorder", want: "/api/v1/queue/reorder"},
		{path: "/api/v1/queue/remove", want: "/api/v1/queue/remove"},
		{path: "/api/v1/metrics", want: "/api/v1/metrics"},
		{path: "/api/v1/healthz", want: "/api/v1/healthz"},
		{path: "/api/v1/unknown/path", want: "/api/v1/unknown"},
		{path: "/not-api", want: "/api/v1/unknown"},
	}

	for _, tc := range tests {
		if got := apiRouteTemplate(tc.path); got != tc.want {
			t.Fatalf("apiRouteTemplate(%q) = %q, want %q", tc.path, got, tc.want)
		}
	}
}

func TestRecoverMiddlewareReturnsCleanError(t *testing.T) {
	srv := New(DefaultConfig())
	canary := "secret-stack-frame-marker-XYZ123"
	panicker := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		panic(canary)
	})

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/whatever", nil)
	srv.recoverMiddleware(panicker).ServeHTTP(resp, req)

	if resp.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", resp.Code)
	}
	body := resp.Body.String()
	if strings.Contains(body, canary) {
		t.Fatalf("response leaked panic value: %s", body)
	}
	var out ErrorResponse
	if err := json.NewDecoder(strings.NewReader(body)).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.Error.Code != "internal_error" {
		t.Fatalf("expected internal_error, got %q", out.Error.Code)
	}
	if got := srv.metrics.Snapshot().API.PanicsTotal; got != 1 {
		t.Fatalf("panics_total = %d, want 1", got)
	}
}

func TestRecoverMiddlewarePassesThroughNormalRequests(t *testing.T) {
	srv := New(DefaultConfig())
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/whatever", nil)
	srv.recoverMiddleware(ok).ServeHTTP(resp, req)

	if resp.Code != http.StatusTeapot {
		t.Fatalf("expected 418, got %d", resp.Code)
	}
	if got := srv.metrics.Snapshot().API.PanicsTotal; got != 0 {
		t.Fatalf("panics_total = %d, want 0 for non-panicking handler", got)
	}
}

func TestRequestIDMiddlewareGeneratesForUntrusted(t *testing.T) {
	srv := New(DefaultConfig()) // no trusted proxies
	handler := srv.requestIDMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/healthz", nil)
	req.RemoteAddr = "203.0.113.7:1234" // untrusted
	handler.ServeHTTP(resp, req)

	got := resp.Header().Get("X-Request-Id")
	if got == "" {
		t.Fatal("expected generated X-Request-Id response header")
	}
	if len(got) != 16 {
		t.Fatalf("expected 16-char generated ID, got %q", got)
	}
}

func TestRequestIDMiddlewareEchoesFromTrustedProxy(t *testing.T) {
	cfg := DefaultConfig()
	cfg.TrustedProxyCIDRs = []string{"127.0.0.1/32"}
	srv := New(cfg)
	handler := srv.requestIDMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/healthz", nil)
	req.RemoteAddr = "127.0.0.1:5000" // trusted proxy
	req.Header.Set("X-Request-Id", "edge-abc123")
	handler.ServeHTTP(resp, req)

	if got := resp.Header().Get("X-Request-Id"); got != "edge-abc123" {
		t.Fatalf("expected trusted inbound ID echoed, got %q", got)
	}
}

func TestRequestIDMiddlewareIgnoresUntrustedInbound(t *testing.T) {
	srv := New(DefaultConfig()) // trusts nothing
	handler := srv.requestIDMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/healthz", nil)
	req.RemoteAddr = "203.0.113.7:1234" // untrusted
	req.Header.Set("X-Request-Id", "spoofed")
	handler.ServeHTTP(resp, req)

	got := resp.Header().Get("X-Request-Id")
	if got == "spoofed" {
		t.Fatal("must not echo a spoofed ID from an untrusted remote")
	}
	if got == "" {
		t.Fatal("expected a regenerated X-Request-Id")
	}
}

func TestAccessLogMiddlewareEmitsStructuredLine(t *testing.T) {
	cases := []struct {
		status    int
		wantLevel string
	}{
		{http.StatusOK, "INFO"},
		{http.StatusNotFound, "WARN"},
		{http.StatusInternalServerError, "ERROR"},
	}
	for _, tc := range cases {
		var buf bytes.Buffer
		srv := New(DefaultConfig())
		srv.logger = newLogger("json", "info", &buf)

		// requestID wraps accessLog so the line carries a request_id.
		handler := srv.requestIDMiddleware(srv.accessLogMiddleware(
			http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
			})))

		resp := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/jobs/j1", nil)
		handler.ServeHTTP(resp, req)

		lines := decodeLogLines(t, &buf)
		if len(lines) != 1 {
			t.Fatalf("status %d: expected 1 access line, got %d", tc.status, len(lines))
		}
		line := lines[0]
		if line["msg"] != "http_request" {
			t.Fatalf("status %d: msg = %v, want http_request", tc.status, line["msg"])
		}
		if line["level"] != tc.wantLevel {
			t.Fatalf("status %d: level = %v, want %v", tc.status, line["level"], tc.wantLevel)
		}
		if line["method"] != http.MethodGet {
			t.Fatalf("status %d: method = %v", tc.status, line["method"])
		}
		if line["path"] != "/api/v1/jobs/j1" {
			t.Fatalf("status %d: path = %v", tc.status, line["path"])
		}
		if line["status"] != float64(tc.status) {
			t.Fatalf("status %d: status field = %v", tc.status, line["status"])
		}
		if _, ok := line["duration_ms"]; !ok {
			t.Fatalf("status %d: missing duration_ms", tc.status)
		}
		if id, ok := line["request_id"].(string); !ok || id == "" {
			t.Fatalf("status %d: missing request_id, got %v", tc.status, line["request_id"])
		}
	}
}

func TestAccessLogMiddlewareOmitsBodyByDefault(t *testing.T) {
	var buf bytes.Buffer
	srv := New(DefaultConfig())
	srv.logger = newLogger("json", "info", &buf) // info: no body capture

	handler := srv.accessLogMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("secret-response-body"))
	}))
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/api/v1/jobs", nil))

	if strings.Contains(buf.String(), "secret-response-body") {
		t.Fatalf("body must not be logged at info level: %s", buf.String())
	}
}

func TestAccessLogMiddlewareCapturesBodyAtDebug(t *testing.T) {
	var buf bytes.Buffer
	srv := New(DefaultConfig())
	srv.logger = newLogger("json", "debug", &buf) // debug: body captured

	handler := srv.accessLogMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("visible-at-debug"))
	}))
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/api/v1/jobs", nil))

	lines := decodeLogLines(t, &buf)
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(lines))
	}
	if body, _ := lines[0]["body"].(string); !strings.Contains(body, "visible-at-debug") {
		t.Fatalf("expected body captured at debug, got %v", lines[0]["body"])
	}
}

func TestRecoverMiddlewareLogsStructuredPanicWithRequestID(t *testing.T) {
	var buf bytes.Buffer
	srv := New(DefaultConfig())
	srv.logger = newLogger("json", "info", &buf)

	canary := "panic-canary-42"
	panicker := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		panic(canary)
	})
	// requestID wraps recover so the panic line carries a request_id.
	handler := srv.requestIDMiddleware(srv.recoverMiddleware(panicker))

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/jobs", nil)
	handler.ServeHTTP(resp, req)

	if resp.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", resp.Code)
	}
	lines := decodeLogLines(t, &buf)
	if len(lines) != 1 {
		t.Fatalf("expected 1 panic line, got %d", len(lines))
	}
	line := lines[0]
	if line["msg"] != "panic" {
		t.Fatalf("msg = %v, want panic", line["msg"])
	}
	if line["level"] != "ERROR" {
		t.Fatalf("level = %v, want ERROR", line["level"])
	}
	if id, ok := line["request_id"].(string); !ok || id == "" {
		t.Fatalf("expected request_id on panic line, got %v", line["request_id"])
	}
	if val, _ := line["value"].(string); !strings.Contains(val, canary) {
		t.Fatalf("expected panic value %q logged, got %v", canary, line["value"])
	}
}

func TestSecurityHeadersPermissionsPolicy(t *testing.T) {
	srv := New(DefaultConfig())
	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/public/", nil)
	srv.Handler().ServeHTTP(resp, req)
	pp := resp.Header().Get("Permissions-Policy")
	if pp == "" {
		t.Fatal("missing Permissions-Policy header")
	}
	mustLock := []string{
		"geolocation=()", "microphone=()", "camera=()",
		"payment=()", "usb=()", "bluetooth=()", "serial=()", "midi=()", "hid=()",
		"accelerometer=()", "gyroscope=()", "magnetometer=()",
		"fullscreen=()", "display-capture=()",
		"idle-detection=()", "screen-wake-lock=()", "xr-spatial-tracking=()",
		"clipboard-read=()",
	}
	for _, want := range mustLock {
		if !strings.Contains(pp, want) {
			t.Errorf("Permissions-Policy missing %q, got %q", want, pp)
		}
	}
	// clipboard-write must NOT be locked: ShareButton.svelte uses it.
	if strings.Contains(pp, "clipboard-write=") {
		t.Errorf("Permissions-Policy must not lock clipboard-write (ShareButton uses it), got %q", pp)
	}
	// interest-cohort was deprecated and triggers a console warning.
	if strings.Contains(pp, "interest-cohort") {
		t.Errorf("Permissions-Policy must not include deprecated interest-cohort, got %q", pp)
	}
}

func TestSecurityHeadersCSPDropsUnsafeInlineStyles(t *testing.T) {
	srv := New(DefaultConfig())
	cases := []struct {
		path        string
		mustHave    []string
		mustNotHave []string
	}{
		{
			path:        "/pub/api/v1/version",
			mustHave:    []string{"default-src 'none'"},
			mustNotHave: []string{"'unsafe-inline'"},
		},
		{
			path:        "/public/",
			mustHave:    []string{"style-src 'self';", "script-src 'self';"},
			mustNotHave: []string{"'unsafe-inline'"},
		},
		{
			path: "/analysis/",
			mustHave: []string{
				"script-src 'self' 'unsafe-inline'",
				"style-src 'self' 'unsafe-hashes' 'sha256-",
			},
			// script-src keeps 'unsafe-inline' for the bootstrap <script>;
			// style-src must not regress to a blanket 'unsafe-inline'.
			mustNotHave: []string{"style-src 'self' 'unsafe-inline'"},
		},
	}
	for _, tc := range cases {
		resp := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, tc.path, nil)
		srv.Handler().ServeHTTP(resp, req)
		csp := resp.Header().Get("Content-Security-Policy")
		if csp == "" {
			t.Fatalf("%s: missing Content-Security-Policy header", tc.path)
		}
		for _, want := range tc.mustHave {
			if !strings.Contains(csp, want) {
				t.Errorf("%s: CSP missing %q, got %q", tc.path, want, csp)
			}
		}
		for _, banned := range tc.mustNotHave {
			if strings.Contains(csp, banned) {
				t.Errorf("%s: CSP contains banned token %q, got %q", tc.path, banned, csp)
			}
		}
	}
}

func TestRecoverMiddlewareDoesNotSwallowErrAbortHandler(t *testing.T) {
	srv := New(DefaultConfig())
	panicker := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		panic(http.ErrAbortHandler)
	})

	defer func() {
		rec := recover()
		if rec != http.ErrAbortHandler {
			t.Fatalf("expected ErrAbortHandler to propagate, got %v", rec)
		}
	}()

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/whatever", nil)
	srv.recoverMiddleware(panicker).ServeHTTP(resp, req)
	t.Fatal("expected panic to propagate")
}
