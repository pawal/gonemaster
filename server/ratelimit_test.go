package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/synctest"
	"time"
)

// --- RateLimiter unit tests ---

func TestRateLimiterAllowsUnderLimit(t *testing.T) {
	rl := NewRateLimiter(3, time.Minute)
	for i := range 3 {
		if ok, _ := rl.Allow("1.2.3.4"); !ok {
			t.Fatalf("request %d should be allowed", i+1)
		}
	}
}

func TestRateLimiterBlocksAtLimit(t *testing.T) {
	rl := NewRateLimiter(3, time.Minute)
	for range 3 {
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
	synctest.Test(t, func(t *testing.T) {
		const window = 50 * time.Millisecond
		rl := NewRateLimiter(1, window)
		rl.Allow("1.2.3.4")

		if ok, _ := rl.Allow("1.2.3.4"); ok {
			t.Fatal("should be blocked before window expires")
		}

		time.Sleep(window + time.Millisecond)

		if ok, _ := rl.Allow("1.2.3.4"); !ok {
			t.Fatal("should be allowed after window expires")
		}
	})
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
	synctest.Test(t, func(t *testing.T) {
		const window = 50 * time.Millisecond
		rl := NewRateLimiter(5, window)
		rl.Allow("1.2.3.4")
		rl.Allow("5.6.7.8")

		time.Sleep(window + time.Millisecond)
		rl.Cleanup()

		rl.mu.Lock()
		n := len(rl.entries)
		rl.mu.Unlock()

		if n != 0 {
			t.Fatalf("expected 0 entries after cleanup, got %d", n)
		}
	})
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
	if got := clientIP(r, nil); got != "192.0.2.1" {
		t.Fatalf("expected 192.0.2.1, got %q", got)
	}
}

func TestClientIPIgnoresXForwardedForFromUntrustedRemote(t *testing.T) {
	// No trusted proxies configured: XFF must be ignored, even if set.
	// Spoofing XFF should not change the attribution.
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "203.0.113.1:1234"
	r.Header.Set("X-Forwarded-For", "1.2.3.4")
	if got := clientIP(r, nil); got != "203.0.113.1" {
		t.Fatalf("expected RemoteAddr 203.0.113.1, got %q", got)
	}
}

func TestClientIPHonorsXForwardedForFromTrustedRemote(t *testing.T) {
	trusted := parseTrustedProxies([]string{"10.0.0.0/8"})
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "10.0.0.1:1234"
	r.Header.Set("X-Forwarded-For", "203.0.113.5")
	if got := clientIP(r, trusted); got != "203.0.113.5" {
		t.Fatalf("expected 203.0.113.5, got %q", got)
	}
}

func TestClientIPWalksRightToLeftSkippingTrustedHops(t *testing.T) {
	trusted := parseTrustedProxies([]string{"10.0.0.0/8", "192.168.0.0/16"})
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "192.168.1.1:1234"
	// client → proxy1(10.0.0.5) → proxy2(192.168.1.1).
	// Right-to-left: skip 10.0.0.5 (trusted), return 203.0.113.7 (untrusted).
	r.Header.Set("X-Forwarded-For", "203.0.113.7, 10.0.0.5")
	if got := clientIP(r, trusted); got != "203.0.113.7" {
		t.Fatalf("expected 203.0.113.7, got %q", got)
	}
}

func TestClientIPRejectsSpoofedXForwardedForViaTrustedRemote(t *testing.T) {
	// Even when remote is trusted, an entirely-untrusted XFF chain is taken
	// at face value: the right-most untrusted hop wins. This documents the
	// "spoof from outside the trust boundary" expectation.
	trusted := parseTrustedProxies([]string{"10.0.0.0/8"})
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "10.0.0.1:1234"
	// Attacker behind a trusted proxy sets XFF; rightmost untrusted hop is
	// the value just before the trusted proxy in the real chain - here all
	// hops are untrusted so the rightmost wins.
	r.Header.Set("X-Forwarded-For", "198.51.100.10, 203.0.113.20")
	if got := clientIP(r, trusted); got != "203.0.113.20" {
		t.Fatalf("expected rightmost untrusted hop 203.0.113.20, got %q", got)
	}
}

func TestClientIPRemoteAddrTrustedButNoXFFFallsBackToRemote(t *testing.T) {
	trusted := parseTrustedProxies([]string{"10.0.0.0/8"})
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "10.0.0.1:1234"
	if got := clientIP(r, trusted); got != "10.0.0.1" {
		t.Fatalf("expected 10.0.0.1, got %q", got)
	}
}

func TestClientIPHandlesV4MappedV6InTrustedCIDR(t *testing.T) {
	trusted := parseTrustedProxies([]string{"10.0.0.0/8"})
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "[::ffff:10.0.0.1]:1234"
	r.Header.Set("X-Forwarded-For", "203.0.113.9")
	if got := clientIP(r, trusted); got != "203.0.113.9" {
		t.Fatalf("expected 203.0.113.9, got %q", got)
	}
}

func TestParseTrustedProxiesAcceptsBareIPAndCIDR(t *testing.T) {
	got := parseTrustedProxies([]string{"127.0.0.1", "10.0.0.0/8", "  ", "::1"})
	if len(got) != 3 {
		t.Fatalf("expected 3 entries, got %d: %v", len(got), got)
	}
}

// --- rateLimitMiddleware integration tests ---

func TestRateLimitMiddlewareAllowsGETUnconditionally(t *testing.T) {
	rl := NewRateLimiter(0, time.Minute) // max=0 blocks everything POST
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	h := rateLimitMiddleware(rl, nil, ok)

	for range 5 {
		resp := doHandler(t, h, http.MethodGet, "/whatever", nil)
		wantStatus(t, resp, http.StatusOK)
	}
}

func TestRateLimitMiddlewareBlocks429WithRetryAfter(t *testing.T) {
	rl := NewRateLimiter(2, time.Minute)
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})
	h := rateLimitMiddleware(rl, nil, ok)

	for range 2 {
		resp := doHandler(t, h, http.MethodPost, "/jobs", `{}`, withRemoteAddr("1.2.3.4:5000"), noContentType())
		wantStatus(t, resp, http.StatusCreated)
	}

	resp := doHandler(t, h, http.MethodPost, "/jobs", `{}`, withRemoteAddr("1.2.3.4:5000"), noContentType())
	wantStatus(t, resp, http.StatusTooManyRequests)
	if resp.Header().Get("Retry-After") == "" {
		t.Fatal("expected Retry-After header on 429 response")
	}
}

func TestRateLimitMiddlewareXForwardedForRespectedFromTrustedProxy(t *testing.T) {
	rl := NewRateLimiter(1, time.Minute)
	trusted := parseTrustedProxies([]string{"10.0.0.0/8"})
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})
	h := rateLimitMiddleware(rl, trusted, ok)

	makePost := func(xff string) int {
		resp := doHandler(t, h, http.MethodPost, "/jobs", `{}`, withRemoteAddr("10.0.0.1:1234"), withHeader("X-Forwarded-For", xff), noContentType())
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

func TestRateLimitMiddlewareIgnoresSpoofedXForwardedFor(t *testing.T) {
	// No trusted proxies configured; XFF must be ignored. An attacker
	// rotating XFF cannot escape per-IP attribution by RemoteAddr.
	rl := NewRateLimiter(1, time.Minute)
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})
	h := rateLimitMiddleware(rl, nil, ok)

	makePost := func(xff string) int {
		resp := doHandler(t, h, http.MethodPost, "/jobs", `{}`, withRemoteAddr("203.0.113.99:1234"), withHeader("X-Forwarded-For", xff), noContentType())
		return resp.Code
	}

	if code := makePost("1.1.1.1"); code != http.StatusCreated {
		t.Fatalf("first request: expected 201, got %d", code)
	}
	// Different XFF, same RemoteAddr - must be blocked.
	if code := makePost("2.2.2.2"); code != http.StatusTooManyRequests {
		t.Fatalf("second request with rotated XFF: expected 429, got %d", code)
	}
}

// --- End-to-end through the server ---

func TestServerRateLimitDisabledByDefault(t *testing.T) {
	cfg := DefaultConfig()
	// Rate limiting is off by default - repeated POSTs must all pass.
	srv := New(cfg)
	for range 5 {
		resp := doJSON(t, srv, http.MethodPost, "/pub/api/v1/jobs", `{"domain":"example.com"}`, withRemoteAddr("1.2.3.4:1234"))
		wantStatus(t, resp, http.StatusCreated)
	}
}

func TestServerRateLimitEnabledBlocks429(t *testing.T) {
	cfg := DefaultConfig()
	cfg.PublicAPI.RateLimitEnabled = true
	cfg.PublicAPI.RateLimitMax = 2
	cfg.PublicAPI.RateLimitWindow = Duration{time.Minute}
	srv := New(cfg)

	makePost := func() int {
		resp := doJSON(t, srv, http.MethodPost, "/pub/api/v1/jobs", `{"domain":"example.com"}`, withRemoteAddr("1.2.3.4:1234"))
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

	resp := doJSON(t, srv, http.MethodGet, "/pub/api/v1/jobs/"+job.PublicID, nil, withRemoteAddr("1.2.3.4:1234"))
	wantStatus(t, resp, http.StatusOK)
}
