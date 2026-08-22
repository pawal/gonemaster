package server

import (
	"database/sql"
	"net/http"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

// TestAnalysisTimeoutMiddlewareReturnsTimeout verifies that a slow handler
// behind /pub/api/v1/analysis/ is cut off and the client receives 503.
func TestAnalysisTimeoutMiddlewareReturnsTimeout(t *testing.T) {
	slow := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(500 * time.Millisecond):
			w.WriteHeader(http.StatusOK)
		case <-r.Context().Done():
			return
		}
	})
	wrapped := analysisTimeoutMiddleware(50*time.Millisecond, slow)

	rr := doHandler(t, wrapped, http.MethodGet, "/pub/api/v1/analysis/nameservers", nil)

	wantStatus(t, rr, http.StatusServiceUnavailable)
	if !strings.Contains(rr.Body.String(), "request_timeout") {
		t.Errorf("body = %q, want request_timeout", rr.Body.String())
	}
}

// TestAnalysisTimeoutMiddlewareLetsFastRequestsThrough verifies that a
// fast handler is unaffected by the wrapping.
func TestAnalysisTimeoutMiddlewareLetsFastRequestsThrough(t *testing.T) {
	fast := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
	wrapped := analysisTimeoutMiddleware(time.Second, fast)

	rr := doHandler(t, wrapped, http.MethodGet, "/pub/api/v1/analysis/nameservers", nil)

	wantStatus(t, rr, http.StatusOK)
}

// TestAnalysisTimeoutMiddlewareSkipsNonAnalysisPaths verifies that
// non-analysis paths bypass the timeout wrap entirely.
func TestAnalysisTimeoutMiddlewareSkipsNonAnalysisPaths(t *testing.T) {
	hits := 0
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		// Sleep longer than the wrap's deadline; if the wrap applied,
		// this would 503. /jobs is not under /analysis/, so it must not.
		time.Sleep(80 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	})
	wrapped := analysisTimeoutMiddleware(20*time.Millisecond, next)

	rr := doHandler(t, wrapped, http.MethodGet, "/pub/api/v1/jobs/abc", nil)

	wantStatus(t, rr, http.StatusOK)
	if hits != 1 {
		t.Fatalf("next handler hits = %d, want 1", hits)
	}
}

// TestAnalysisTimeoutMiddlewareDisabledOnZeroDuration verifies the
// passthrough path when the operator turns the timeout off.
func TestAnalysisTimeoutMiddlewareDisabledOnZeroDuration(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	wrapped := analysisTimeoutMiddleware(0, next)
	if wrapped == nil {
		t.Fatal("middleware must not return nil on zero duration")
	}
	rr := doHandler(t, wrapped, http.MethodGet, "/pub/api/v1/analysis/nameservers", nil)
	wantStatus(t, rr, http.StatusOK)
}

// TestConfigurePoolWithOverrides exercises the operator-tunable pool
// numbers: zeros fall back to defaults, non-zeros override.
func TestConfigurePoolWithOverrides(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	configurePoolWith(db, "sqlite", DatabaseConfig{MaxOpenConns: 4})
	stats := db.Stats()
	if stats.MaxOpenConnections != 4 {
		t.Errorf("sqlite override: MaxOpenConnections = %d, want 4", stats.MaxOpenConnections)
	}

	configurePoolWith(db, "sqlite", DatabaseConfig{})
	stats = db.Stats()
	if stats.MaxOpenConnections != 1 {
		t.Errorf("sqlite default: MaxOpenConnections = %d, want 1", stats.MaxOpenConnections)
	}

	// Postgres-style defaults: 25 / 5 / 5min when unspecified, and
	// overrides when supplied.
	configurePoolWith(db, "postgres", DatabaseConfig{})
	stats = db.Stats()
	if stats.MaxOpenConnections != 25 {
		t.Errorf("postgres default: MaxOpenConnections = %d, want 25", stats.MaxOpenConnections)
	}

	configurePoolWith(db, "postgres", DatabaseConfig{
		MaxOpenConns: 50, MaxIdleConns: 10, ConnMaxLifetimeSeconds: 600,
	})
	stats = db.Stats()
	if stats.MaxOpenConnections != 50 {
		t.Errorf("postgres override: MaxOpenConnections = %d, want 50", stats.MaxOpenConnections)
	}
}
