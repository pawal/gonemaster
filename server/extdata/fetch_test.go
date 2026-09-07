package extdata

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"
)

func TestValidateURL(t *testing.T) {
	p := enabledProvider()
	if _, err := p.validateURL("https://data.iana.org/rdap/dns.json"); err != nil {
		t.Fatalf("validateURL rejected a public https URL: %v", err)
	}
	rejected := map[string]string{
		"plain http":  "http://data.iana.org/rdap/dns.json",
		"no host":     "https:///rdap/dns.json",
		"loopback":    "https://127.0.0.1/rdap/dns.json",
		"ipv6 v4":     "https://[::1]/rdap/dns.json",
		"private":     "https://10.0.0.1/rdap/dns.json",
		"link-local":  "https://169.254.169.254/latest/meta-data/",
		"cgnat":       "https://100.64.0.1/rdap/dns.json",
		"unspecified": "https://0.0.0.0/rdap/dns.json",
	}
	for name, raw := range rejected {
		if _, err := p.validateURL(raw); err == nil {
			t.Errorf("%s: validateURL accepted %q", name, raw)
		}
	}
}

func TestValidateURLAllowsInsecureInTests(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Enabled = true
	cfg.allowInsecure = true
	p := New(cfg)
	if _, err := p.validateURL("http://127.0.0.1:8080/tlds"); err != nil {
		t.Fatalf("validateURL rejected a test URL with the guards relaxed: %v", err)
	}
}

func TestBlockedAddrReason(t *testing.T) {
	blocked := []string{"127.0.0.1", "::1", "10.1.2.3", "192.168.0.1", "169.254.1.1",
		"100.64.0.1", "224.0.0.1", "ff02::1", "0.0.0.0", "255.255.255.255", "::ffff:127.0.0.1"}
	for _, ip := range blocked {
		if reason := blockedAddrReason(netip.MustParseAddr(ip)); reason == "" {
			t.Errorf("blockedAddrReason(%s) = \"\", want a reason", ip)
		}
	}
	for _, ip := range []string{"192.0.2.1", "2001:db8::1", "8.8.8.8"} {
		if reason := blockedAddrReason(netip.MustParseAddr(ip)); reason != "" {
			t.Errorf("blockedAddrReason(%s) = %q, want globally routable", ip, reason)
		}
	}
	// A DNS name is not an address; the dialer guard checks it after resolution.
	if reason := blockedHostReason("data.iana.org"); reason != "" {
		t.Errorf("blockedHostReason(name) = %q, want empty", reason)
	}
}

func TestRedirectPolicy(t *testing.T) {
	client := newHTTPClient(DefaultConfig())
	req := func(raw string) *http.Request {
		r, err := http.NewRequest(http.MethodGet, raw, nil)
		if err != nil {
			t.Fatalf("build request: %v", err)
		}
		return r
	}
	if err := client.CheckRedirect(req("https://rdap.example.org/domain/x"), nil); err != nil {
		t.Fatalf("redirect to https rejected: %v", err)
	}
	if err := client.CheckRedirect(req("http://rdap.example.org/domain/x"), nil); err == nil {
		t.Error("redirect to plain http accepted")
	}
	if err := client.CheckRedirect(req("https://127.0.0.1/domain/x"), nil); err == nil {
		t.Error("redirect to loopback accepted")
	}
	via := make([]*http.Request, maxRedirects)
	if err := client.CheckRedirect(req("https://rdap.example.org/domain/x"), via); err == nil {
		t.Error("redirect chain past the limit accepted")
	}
}

func TestRequireJSON(t *testing.T) {
	for _, ct := range []string{"application/json", "application/rdap+json; charset=utf-8"} {
		if err := requireJSON(ct); err != nil {
			t.Errorf("requireJSON(%q) = %v, want nil", ct, err)
		}
	}
	for _, ct := range []string{"", "text/html", "not/a/type"} {
		if err := requireJSON(ct); err == nil {
			t.Errorf("requireJSON(%q) accepted a non-JSON type", ct)
		}
	}
}

func TestReadCapped(t *testing.T) {
	if _, err := readCapped(bytes.NewReader(make([]byte, maxResponseBytes+1))); err == nil {
		t.Fatal("readCapped accepted a body past the cap")
	}
	body, err := readCapped(bytes.NewReader([]byte("small")))
	if err != nil || string(body) != "small" {
		t.Fatalf("readCapped = %q/%v, want the body", body, err)
	}
}

func TestGetSendsIdentifyingHeaders(t *testing.T) {
	var got http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Clone()
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("# Version 1\nSE\n"))
	}))
	t.Cleanup(srv.Close)
	clock := newTestClock()
	p := newTestProvider(t, srv, clock, func(c *Config) { c.UserAgent = "gonemaster/9.9.9" })
	if _, err := p.get(t.Context(), srv.URL, "\"etag-1\"", "Mon, 07 Sep 2026 00:00:00 GMT", false); err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Get("User-Agent") != "gonemaster/9.9.9" {
		t.Errorf("User-Agent = %q", got.Get("User-Agent"))
	}
	if got.Get("If-None-Match") != "\"etag-1\"" {
		t.Errorf("If-None-Match = %q", got.Get("If-None-Match"))
	}
	if got.Get("If-Modified-Since") == "" {
		t.Error("If-Modified-Since not sent")
	}
}

func TestDatasetRefreshUses304(t *testing.T) {
	var conditional int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("If-None-Match") == "\"v1\"" {
			conditional++
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", "\"v1\"")
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write(readFixture(t, "tlds-alpha-by-domain.txt"))
	}))
	t.Cleanup(srv.Close)
	clock := newTestClock()
	p := newTestProvider(t, srv, clock, func(c *Config) {
		c.RefreshInterval = time.Hour
		c.Sources = Sources{IANATLDs: srv.URL, RDAPBootstrap: srv.URL}
	})

	key := datasetKey(datasetIANATLDs)
	p.Lookup(key)
	waitFor(t, "the first load", func() bool { return !fetchedAt(p, key).IsZero() })

	clock.Advance(2 * time.Hour)
	p.Lookup(key)
	waitFor(t, "the conditional refresh", func() bool { return fetchedAt(p, key).Equal(clock.Now()) })
	if conditional != 1 {
		t.Fatalf("conditional requests = %d, want 1", conditional)
	}
	// The unchanged file keeps its parsed value.
	list, state := p.TLDs()
	if state != StateFresh || !list.Has("se") {
		t.Fatalf("TLDs() = %v/%q, want the cached list preserved across a 304", list, state)
	}
}

func TestGetRejectsOversizedBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write(bytes.Repeat([]byte("x"), maxResponseBytes+16))
	}))
	t.Cleanup(srv.Close)
	p := newTestProvider(t, srv, newTestClock(), nil)
	if _, err := p.get(t.Context(), srv.URL, "", "", false); err == nil {
		t.Fatal("get accepted a body past the size cap")
	}
}

func TestGetRejectsNonJSONRecord(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<html>not rdap</html>"))
	}))
	t.Cleanup(srv.Close)
	p := newTestProvider(t, srv, newTestClock(), nil)
	_, err := p.get(t.Context(), srv.URL, "", "", true)
	if err == nil || !strings.Contains(err.Error(), "content-type") {
		t.Fatalf("err = %v, want a content-type rejection", err)
	}
}

func TestGetRejectsErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)
	p := newTestProvider(t, srv, newTestClock(), nil)
	if _, err := p.get(t.Context(), srv.URL, "", "", false); err == nil {
		t.Fatal("get accepted a 404")
	}
}

func TestTokenBucketRefills(t *testing.T) {
	clock := newTestClock()
	bucket := newTokenBucket(2, time.Minute, clock.Now)
	if !bucket.take() || !bucket.take() {
		t.Fatal("the first two takes must succeed")
	}
	if bucket.take() {
		t.Fatal("the third take must fail on an empty bucket")
	}
	clock.Advance(30 * time.Second)
	if !bucket.take() {
		t.Fatal("half a window must refill one token")
	}
	clock.Advance(time.Hour)
	if !bucket.take() || !bucket.take() || bucket.take() {
		t.Fatal("refill must stop at the bucket capacity")
	}
}
