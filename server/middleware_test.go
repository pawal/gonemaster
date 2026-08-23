package server

import (
	"bytes"
	"encoding/json"
	"net/http"
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

func TestRouteLabel(t *testing.T) {
	tests := []struct {
		prefix  string
		pattern string
		want    string
	}{
		// Method-qualified patterns: the method is dropped because the access
		// log records it in its own field.
		{"/pub/api/v1", "GET /locales", "/pub/api/v1/locales"},
		{"/pub/api/v1", "GET /jobs/{publicID}", "/pub/api/v1/jobs/{publicID}"},
		{"/pub/api/v1", "GET /jobs/{publicID}/result", "/pub/api/v1/jobs/{publicID}/result"},
		{"/pub/api/v1", "POST /jobs", "/pub/api/v1/jobs"},
		// Method-less patterns pass through unchanged apart from the prefix.
		{"/api/v1", "/healthz", "/api/v1/healthz"},
		// Subtree patterns keep the trailing slash out of the label.
		{"/api/v1", "/jobs/", "/api/v1/jobs"},
		// An unmatched request has no pattern and lands in the mount's bucket.
		{"/pub/api/v1", "", "/pub/api/v1/unknown"},
	}
	for _, tc := range tests {
		if got := routeLabel(tc.prefix, tc.pattern); got != tc.want {
			t.Errorf("routeLabel(%q, %q) = %q, want %q", tc.prefix, tc.pattern, got, tc.want)
		}
	}
}

// TestAccessLogRouteFromMatchedPattern drives the fully wired handler so the
// route field is derived from the router's matched pattern. Before the fix the
// public surface, mounted under /pub/api/v1, always logged /api/v1/unknown
// because apiRouteTemplate only understood the admin /api/v1 prefix.
func TestAccessLogRouteFromMatchedPattern(t *testing.T) {
	tests := []struct {
		name      string
		path      string
		wantRoute string
	}{
		{"public static route", "/pub/api/v1/locales", "/pub/api/v1/locales"},
		{"public info route", "/pub/api/v1/info", "/pub/api/v1/info"},
		{"public templated route", "/pub/api/v1/jobs/does-not-exist", "/pub/api/v1/jobs/{publicID}"},
		{"public unmatched route", "/pub/api/v1/no-such-endpoint", "/pub/api/v1/unknown"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			srv := newTestServer(t, withLogTo(&buf, "info"))

			doJSON(t, srv, http.MethodGet, tc.path, nil)

			line := findLogLine(t, &buf, "http_request")
			if got := line["route"]; got != tc.wantRoute {
				t.Fatalf("route = %v, want %q (path %q)", got, tc.wantRoute, tc.path)
			}
			if got := line["path"]; got != tc.path {
				t.Fatalf("path = %v, want %q", got, tc.path)
			}
		})
	}
}

// findLogLine returns the first decoded log line whose msg matches want.
func findLogLine(t *testing.T, buf *bytes.Buffer, want string) map[string]any {
	t.Helper()
	for _, line := range decodeLogLines(t, buf) {
		if line["msg"] == want {
			return line
		}
	}
	t.Fatalf("no log line with msg=%q in output: %s", want, buf.String())
	return nil
}

func TestRecoverMiddlewareReturnsCleanError(t *testing.T) {
	srv := newTestServer(t)
	canary := "secret-stack-frame-marker-XYZ123"
	panicker := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		panic(canary)
	})

	resp := doHandler(t, srv.recoverMiddleware(panicker), http.MethodGet, "/whatever", nil)

	wantStatus(t, resp, http.StatusInternalServerError)
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
	srv := newTestServer(t)
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})

	resp := doHandler(t, srv.recoverMiddleware(ok), http.MethodGet, "/whatever", nil)

	wantStatus(t, resp, http.StatusTeapot)
	if got := srv.metrics.Snapshot().API.PanicsTotal; got != 0 {
		t.Fatalf("panics_total = %d, want 0 for non-panicking handler", got)
	}
}

const (
	untrustedAddr    = "203.0.113.7:1234"
	trustedProxyAddr = "127.0.0.1:5000"
)

func TestRequestIDMiddlewareGeneratesForUntrusted(t *testing.T) {
	srv := newTestServer(t) // no trusted proxies
	handler := srv.requestIDMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	resp := doHandler(t, handler, http.MethodGet, "/api/v1/healthz", nil, withRemoteAddr(untrustedAddr))

	got := resp.Header().Get("X-Request-Id")
	if got == "" {
		t.Fatal("expected generated X-Request-Id response header")
	}
	if len(got) != 16 {
		t.Fatalf("expected 16-char generated ID, got %q", got)
	}
}

func TestRequestIDMiddlewareEchoesFromTrustedProxy(t *testing.T) {
	srv := newTestServer(t, withConfig(func(c *Config) { c.TrustedProxyCIDRs = []string{"127.0.0.1/32"} }))
	handler := srv.requestIDMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	resp := doHandler(t, handler, http.MethodGet, "/api/v1/healthz", nil, withRemoteAddr(trustedProxyAddr), withHeader("X-Request-Id", "edge-abc123"))

	if got := resp.Header().Get("X-Request-Id"); got != "edge-abc123" {
		t.Fatalf("expected trusted inbound ID echoed, got %q", got)
	}
}

func TestRequestIDMiddlewareIgnoresUntrustedInbound(t *testing.T) {
	srv := newTestServer(t) // trusts nothing
	handler := srv.requestIDMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	resp := doHandler(t, handler, http.MethodGet, "/api/v1/healthz", nil, withRemoteAddr(untrustedAddr), withHeader("X-Request-Id", "spoofed"))

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
		srv := newTestServer(t, withLogTo(&buf, "info"))

		// requestID wraps accessLog so the line carries a request_id.
		handler := srv.requestIDMiddleware(srv.accessLogMiddleware(
			http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
			})))

		doHandler(t, handler, http.MethodGet, "/api/v1/jobs/j1", nil)

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
	srv := newTestServer(t, withLogTo(&buf, "info")) // info: no body capture

	handler := srv.accessLogMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("secret-response-body"))
	}))
	doHandler(t, handler, http.MethodGet, "/api/v1/jobs", nil)

	if strings.Contains(buf.String(), "secret-response-body") {
		t.Fatalf("body must not be logged at info level: %s", buf.String())
	}
}

func TestAccessLogMiddlewareCapturesBodyAtDebug(t *testing.T) {
	var buf bytes.Buffer
	srv := newTestServer(t, withLogTo(&buf, "debug")) // debug: body captured

	handler := srv.accessLogMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("visible-at-debug"))
	}))
	doHandler(t, handler, http.MethodGet, "/api/v1/jobs", nil)

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
	srv := newTestServer(t, withLogTo(&buf, "info"))

	canary := "panic-canary-42"
	panicker := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		panic(canary)
	})
	// requestID wraps recover so the panic line carries a request_id.
	handler := srv.requestIDMiddleware(srv.recoverMiddleware(panicker))

	resp := doHandler(t, handler, http.MethodGet, "/api/v1/jobs", nil)

	wantStatus(t, resp, http.StatusInternalServerError)
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
	srv := newTestServer(t)
	resp := doJSON(t, srv, http.MethodGet, "/public/", nil)
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
	srv := newTestServer(t)
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
		resp := doJSON(t, srv, http.MethodGet, tc.path, nil)
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
	srv := newTestServer(t)
	panicker := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		panic(http.ErrAbortHandler)
	})

	defer func() {
		rec := recover()
		if rec != http.ErrAbortHandler {
			t.Fatalf("expected ErrAbortHandler to propagate, got %v", rec)
		}
	}()

	doHandler(t, srv.recoverMiddleware(panicker), http.MethodGet, "/whatever", nil)
	t.Fatal("expected panic to propagate")
}
