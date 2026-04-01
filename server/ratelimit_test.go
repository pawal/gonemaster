package server

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// --- RateLimiter unit tests ---

func TestRateLimiterAllowsUnderLimit(t *testing.T) {
	rl := NewRateLimiter(3, time.Minute)
	for i := 0; i < 3; i++ {
		if ok, _ := rl.Allow("1.2.3.4"); !ok {
			t.Fatalf("request %d should be allowed", i+1)
		}
	}
}

func TestRateLimiterBlocksAtLimit(t *testing.T) {
	rl := NewRateLimiter(3, time.Minute)
	for i := 0; i < 3; i++ {
		rl.Allow("1.2.3.4")
	}
	ok, retryAfter := rl.Allow("1.2.3.4")
	if ok {
		t.Fatal("4th request should be blocked")
	}
	if retryAfter < 1 {
		t.Fatalf("expected retryAfter >= 1, got %d", retryAfter)
	}
}

func TestRateLimiterDifferentIPsAreIndependent(t *testing.T) {
	rl := NewRateLimiter(1, time.Minute)
	rl.Allow("1.1.1.1")
	if ok, _ := rl.Allow("2.2.2.2"); !ok {
		t.Fatal("different IP should not be affected by other IP's limit")
	}
}

func TestRateLimiterWindowExpiryResetsCounter(t *testing.T) {
	rl := NewRateLimiter(1, 50*time.Millisecond)
	rl.Allow("1.2.3.4")

	if ok, _ := rl.Allow("1.2.3.4"); ok {
		t.Fatal("should be blocked before window expires")
	}

	time.Sleep(60 * time.Millisecond)

	if ok, _ := rl.Allow("1.2.3.4"); !ok {
		t.Fatal("should be allowed after window expires")
	}
}

func TestRateLimiterRetryAfterIsPositive(t *testing.T) {
	rl := NewRateLimiter(1, time.Minute)
	rl.Allow("1.2.3.4")
	_, retryAfter := rl.Allow("1.2.3.4")
	if retryAfter <= 0 {
		t.Fatalf("expected positive retryAfter, got %d", retryAfter)
	}
}

func TestRateLimiterCleanupEvictsExpiredEntries(t *testing.T) {
	rl := NewRateLimiter(5, 50*time.Millisecond)
	rl.Allow("1.2.3.4")
	rl.Allow("5.6.7.8")

	time.Sleep(60 * time.Millisecond)
	rl.Cleanup()

	rl.mu.Lock()
	n := len(rl.entries)
	rl.mu.Unlock()

	if n != 0 {
		t.Fatalf("expected 0 entries after cleanup, got %d", n)
	}
}

func TestRateLimiterCleanupKeepsActiveEntries(t *testing.T) {
	rl := NewRateLimiter(5, time.Minute)
	rl.Allow("1.2.3.4")
	rl.Allow("5.6.7.8")

	rl.Cleanup()

	rl.mu.Lock()
	n := len(rl.entries)
	rl.mu.Unlock()

	if n != 2 {
		t.Fatalf("expected 2 active entries after cleanup, got %d", n)
	}
}

// --- clientIP tests ---

func TestClientIPFromRemoteAddr(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "192.0.2.1:1234"
	if got := clientIP(r); got != "192.0.2.1" {
		t.Fatalf("expected 192.0.2.1, got %q", got)
	}
}

func TestClientIPFromXForwardedFor(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "10.0.0.1:1234"
	r.Header.Set("X-Forwarded-For", "203.0.113.5, 10.0.0.1")
	if got := clientIP(r); got != "203.0.113.5" {
		t.Fatalf("expected 203.0.113.5, got %q", got)
	}
}

func TestClientIPFromXRealIP(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "10.0.0.1:1234"
	r.Header.Set("X-Real-IP", "198.51.100.7")
	if got := clientIP(r); got != "198.51.100.7" {
		t.Fatalf("expected 198.51.100.7, got %q", got)
	}
}

func TestClientIPXForwardedForTakesPrecedenceOverXRealIP(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("X-Forwarded-For", "203.0.113.1")
	r.Header.Set("X-Real-IP", "198.51.100.7")
	if got := clientIP(r); got != "203.0.113.1" {
		t.Fatalf("expected X-Forwarded-For to win, got %q", got)
	}
}

// --- rateLimitMiddleware integration tests ---

func TestRateLimitMiddlewareAllowsGETUnconditionally(t *testing.T) {
	rl := NewRateLimiter(0, time.Minute) // max=0 blocks everything POST
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	h := rateLimitMiddleware(rl, ok)

	for i := 0; i < 5; i++ {
		resp := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/whatever", nil)
		h.ServeHTTP(resp, r)
		if resp.Code != http.StatusOK {
			t.Fatalf("GET request %d: expected 200, got %d", i+1, resp.Code)
		}
	}
}

func TestRateLimitMiddlewareBlocks429WithRetryAfter(t *testing.T) {
	rl := NewRateLimiter(2, time.Minute)
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})
	h := rateLimitMiddleware(rl, ok)

	for i := 0; i < 2; i++ {
		resp := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/jobs", bytes.NewBufferString(`{}`))
		r.RemoteAddr = "1.2.3.4:5000"
		h.ServeHTTP(resp, r)
		if resp.Code != http.StatusCreated {
			t.Fatalf("request %d should pass, got %d", i+1, resp.Code)
		}
	}

	resp := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/jobs", bytes.NewBufferString(`{}`))
	r.RemoteAddr = "1.2.3.4:5000"
	h.ServeHTTP(resp, r)
	if resp.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", resp.Code)
	}
	if resp.Header().Get("Retry-After") == "" {
		t.Fatal("expected Retry-After header on 429 response")
	}
}

func TestRateLimitMiddlewareXForwardedForRespected(t *testing.T) {
	rl := NewRateLimiter(1, time.Minute)
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})
	h := rateLimitMiddleware(rl, ok)

	makePost := func(xff string) int {
		resp := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/jobs", bytes.NewBufferString(`{}`))
		r.RemoteAddr = "10.0.0.1:1234"
		r.Header.Set("X-Forwarded-For", xff)
		h.ServeHTTP(resp, r)
		return resp.Code
	}

	// First request from client A passes.
	if code := makePost("203.0.113.1"); code != http.StatusCreated {
		t.Fatalf("first request from A: expected 201, got %d", code)
	}
	// Second request from same client A is blocked.
	if code := makePost("203.0.113.1"); code != http.StatusTooManyRequests {
		t.Fatalf("second request from A: expected 429, got %d", code)
	}
	// First request from different client B passes.
	if code := makePost("203.0.113.2"); code != http.StatusCreated {
		t.Fatalf("first request from B: expected 201, got %d", code)
	}
}

// --- End-to-end through the server ---

func TestServerRateLimitDisabledByDefault(t *testing.T) {
	cfg := DefaultConfig()
	// Rate limiting is off by default — repeated POSTs must all pass.
	srv := New(cfg)
	for i := 0; i < 5; i++ {
		resp := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/pub/api/v1/jobs",
			bytes.NewBufferString(`{"domain":"example.com"}`))
		r.Header.Set("Content-Type", "application/json")
		r.RemoteAddr = "1.2.3.4:1234"
		srv.Handler().ServeHTTP(resp, r)
		if resp.Code != http.StatusCreated {
			t.Fatalf("request %d: expected 201, got %d", i+1, resp.Code)
		}
	}
}

func TestServerRateLimitEnabledBlocks429(t *testing.T) {
	cfg := DefaultConfig()
	cfg.PublicAPI.RateLimitEnabled = true
	cfg.PublicAPI.RateLimitMax = 2
	cfg.PublicAPI.RateLimitWindow = Duration{time.Minute}
	srv := New(cfg)

	makePost := func() int {
		resp := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/pub/api/v1/jobs",
			bytes.NewBufferString(`{"domain":"example.com"}`))
		r.Header.Set("Content-Type", "application/json")
		r.RemoteAddr = "1.2.3.4:1234"
		srv.Handler().ServeHTTP(resp, r)
		return resp.Code
	}

	if code := makePost(); code != http.StatusCreated {
		t.Fatalf("1st request: expected 201, got %d", code)
	}
	if code := makePost(); code != http.StatusCreated {
		t.Fatalf("2nd request: expected 201, got %d", code)
	}
	if code := makePost(); code != http.StatusTooManyRequests {
		t.Fatalf("3rd request: expected 429, got %d", code)
	}
}

func TestServerRateLimitDoesNotApplyToGET(t *testing.T) {
	cfg := DefaultConfig()
	cfg.PublicAPI.RateLimitEnabled = true
	cfg.PublicAPI.RateLimitMax = 0 // blocks all POSTs immediately
	cfg.PublicAPI.RateLimitWindow = Duration{time.Minute}
	srv := New(cfg)

	// Seed a job directly so there's a public ID to look up.
	job, err := srv.store.Create(Job{ID: newID("job"), Domain: "example.com", Status: JobQueued})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	resp := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/pub/api/v1/jobs/"+job.PublicID, nil)
	r.RemoteAddr = "1.2.3.4:1234"
	srv.Handler().ServeHTTP(resp, r)
	if resp.Code != http.StatusOK {
		t.Fatalf("GET should not be rate-limited, got %d", resp.Code)
	}
}
