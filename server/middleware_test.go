package server

import (
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
		"clipboard-read=()", "interest-cohort=()",
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
			path:     "/analysis/",
			mustHave: []string{"style-src 'self';", "script-src 'self' 'unsafe-inline'"},
			// analysisCSP keeps script-src 'unsafe-inline' for the bootstrap
			// inline <script>, but style-src must stay clean.
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
