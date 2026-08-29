package server

import (
	"net/http"
	"net/http/httptest"
	"net/netip"
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

func TestRateLimiterKeysCountsDistinctIPs(t *testing.T) {
	rl := NewRateLimiter(5, time.Minute)
	if got := rl.Keys(); got != 0 {
		t.Fatalf("Keys on a fresh limiter = %d, want 0", got)
	}

	rl.Allow("1.2.3.4")
	rl.Allow("1.2.3.4")
	rl.Allow("5.6.7.8")

	if got := rl.Keys(); got != 2 {
		t.Fatalf("Keys = %d, want 2 distinct IPs", got)
	}
}

// The gauge exists to make the bucket-collapse failure visible: with the wrong
// trusted CIDR every visitor resolves to the proxy's own address, so many
// clients share one bucket and the gauge sticks at 1.
func TestRateLimitKeysGaugeShowsBucketCollapse(t *testing.T) {
	for _, tc := range []struct {
		name    string
		trusted []string
		want    int
	}{
		{name: "proxy trusted, per-client buckets", trusted: []string{"127.0.0.1/32"}, want: 3},
		{name: "proxy not trusted, buckets collapse", want: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := newTestServer(t,
				withConfig(func(c *Config) { c.TrustedProxyCIDRs = tc.trusted }),
				withPublicAPI(func(api *PublicAPIConfig) {
					api.RateLimitEnabled = true
					api.RateLimitMax = 10
				}))

			for _, client := range []string{"198.51.100.1", "198.51.100.2", "198.51.100.3"} {
				doJSON(t, srv, http.MethodPost, "/pub/api/v1/jobs", `{"domain":"example.com"}`,
					withRemoteAddr("127.0.0.1:5000"), withHeader("X-Forwarded-For", client))
			}

			if got := srv.metrics.Snapshot().API.Proxy.RateLimitKeys; got != tc.want {
				t.Fatalf("rate_limit_keys = %d, want %d", got, tc.want)
			}
		})
	}
}

// Reporting 1 with the limiter off would look like the collapse above.
func TestRateLimitKeysGaugeIsZeroWhenDisabled(t *testing.T) {
	srv := newTestServer(t)

	doJSON(t, srv, http.MethodPost, "/pub/api/v1/jobs", `{"domain":"example.com"}`)

	if got := srv.metrics.Snapshot().API.Proxy.RateLimitKeys; got != 0 {
		t.Fatalf("rate_limit_keys = %d, want 0 with the limiter disabled", got)
	}
}

// --- clientIP tests ---

func TestClientIP(t *testing.T) {
	for _, tc := range []struct {
		name       string
		trusted    []string
		remoteAddr string
		xff        string
		want       string
	}{
		{
			name:       "remote addr only",
			remoteAddr: "192.0.2.1:1234",
			want:       "192.0.2.1",
		},
		{
			// No trusted proxies configured: XFF must be ignored, even if
			// set. Spoofing XFF should not change the attribution.
			name:       "untrusted remote ignores xff",
			remoteAddr: "203.0.113.1:1234",
			xff:        "1.2.3.4",
			want:       "203.0.113.1",
		},
		{
			name:       "trusted remote honors xff",
			trusted:    []string{"10.0.0.0/8"},
			remoteAddr: "10.0.0.1:1234",
			xff:        "203.0.113.5",
			want:       "203.0.113.5",
		},
		{
			// client -> proxy1(10.0.0.5) -> proxy2(192.168.1.1).
			// Right-to-left: skip 10.0.0.5 (trusted), return 203.0.113.7.
			name:       "walks right to left skipping trusted hops",
			trusted:    []string{"10.0.0.0/8", "192.168.0.0/16"},
			remoteAddr: "192.168.1.1:1234",
			xff:        "203.0.113.7, 10.0.0.5",
			want:       "203.0.113.7",
		},
		{
			// Even when remote is trusted, an entirely-untrusted XFF chain is
			// taken at face value: the right-most untrusted hop wins. This
			// documents the "spoof from outside the trust boundary"
			// expectation - an attacker behind a trusted proxy sets XFF, and
			// with every hop untrusted the rightmost one wins.
			name:       "all-untrusted xff chain takes the rightmost hop",
			trusted:    []string{"10.0.0.0/8"},
			remoteAddr: "10.0.0.1:1234",
			xff:        "198.51.100.10, 203.0.113.20",
			want:       "203.0.113.20",
		},
		{
			name:       "trusted remote without xff falls back to remote",
			trusted:    []string{"10.0.0.0/8"},
			remoteAddr: "10.0.0.1:1234",
			want:       "10.0.0.1",
		},
		{
			name:       "v4-mapped v6 remote matches a v4 trusted cidr",
			trusted:    []string{"10.0.0.0/8"},
			remoteAddr: "[::ffff:10.0.0.1]:1234",
			xff:        "203.0.113.9",
			want:       "203.0.113.9",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var trusted []netip.Prefix
			if tc.trusted != nil {
				trusted = parseTrustedProxies(tc.trusted)
			}
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			r.RemoteAddr = tc.remoteAddr
			if tc.xff != "" {
				r.Header.Set("X-Forwarded-For", tc.xff)
			}
			if got := clientIP(r, trusted); got != tc.want {
				t.Fatalf("clientIP = %q, want %q", got, tc.want)
			}
		})
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
	// Rate limiting is off by default - repeated POSTs must all pass.
	srv := newTestServer(t)
	for range 5 {
		resp := doJSON(t, srv, http.MethodPost, "/pub/api/v1/jobs", `{"domain":"example.com"}`, withRemoteAddr("1.2.3.4:1234"))
		wantStatus(t, resp, http.StatusCreated)
	}
}

func TestServerRateLimitEnabledBlocks429(t *testing.T) {
	srv := newTestServer(t,
		withPublicAPI(func(c *PublicAPIConfig) {
			c.RateLimitEnabled = true
			c.RateLimitMax = 2
			c.RateLimitWindow = Duration{time.Minute}
		}))

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
	srv := newTestServer(t,
		withPublicAPI(func(c *PublicAPIConfig) {
			c.RateLimitEnabled = true
			c.RateLimitMax = 0 // blocks all POSTs immediately
			c.RateLimitWindow = Duration{time.Minute}
		}))

	// Seed a job directly so there's a public ID to look up.
	job, err := srv.store.Create(Job{ID: newID("job"), Domain: "example.com", Status: JobQueued})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	resp := doJSON(t, srv, http.MethodGet, "/pub/api/v1/jobs/"+job.PublicID, nil, withRemoteAddr("1.2.3.4:1234"))
	wantStatus(t, resp, http.StatusOK)
}
