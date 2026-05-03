package server

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"sync"
	"time"
)

// RateLimiter is an in-memory per-IP sliding-window rate limiter.
type RateLimiter struct {
	mu      sync.Mutex
	entries map[string][]time.Time
	max     int
	window  time.Duration
}

// NewRateLimiter creates a RateLimiter allowing at most max requests per IP
// within the given window duration.
func NewRateLimiter(max int, window time.Duration) *RateLimiter {
	return &RateLimiter{
		entries: make(map[string][]time.Time),
		max:     max,
		window:  window,
	}
}

// Allow reports whether a new request from ip is within the rate limit.
// When denied, retryAfter is the number of seconds the caller should include
// in a Retry-After response header.
func (rl *RateLimiter) Allow(ip string) (allowed bool, retryAfter int) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-rl.window)

	// Remove timestamps that have left the window.
	times := rl.entries[ip]
	n := 0
	for _, t := range times {
		if t.After(cutoff) {
			times[n] = t
			n++
		}
	}
	times = times[:n]

	if len(times) >= rl.max {
		rl.entries[ip] = times
		// Retry-After: seconds until the oldest hit slides out of the window.
		ra := max(int(times[0].Add(rl.window).Sub(now).Seconds())+1, 1)
		return false, ra
	}

	rl.entries[ip] = append(times, now)
	return true, 0
}

// Cleanup removes entries for IPs whose timestamps have all left the window.
// Safe to call concurrently; typically called on a periodic ticker.
func (rl *RateLimiter) Cleanup() {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	cutoff := time.Now().Add(-rl.window)
	for ip, times := range rl.entries {
		active := false
		for _, t := range times {
			if t.After(cutoff) {
				active = true
				break
			}
		}
		if !active {
			delete(rl.entries, ip)
		}
	}
}

// parseTrustedProxies parses operator-supplied CIDRs / bare IPs into prefixes.
// Invalid entries are logged and skipped so a typo in one entry does not
// disable trust for the rest.
func parseTrustedProxies(cidrs []string) []netip.Prefix {
	var out []netip.Prefix
	for _, s := range cidrs {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if p, err := netip.ParsePrefix(s); err == nil {
			out = append(out, p)
			continue
		}
		if a, err := netip.ParseAddr(s); err == nil {
			out = append(out, netip.PrefixFrom(a, a.BitLen()))
			continue
		}
		log.Printf("server: ignoring invalid trusted_proxy_cidrs entry %q", s)
	}
	return out
}

func ipInPrefixes(addr netip.Addr, prefixes []netip.Prefix) bool {
	addr = addr.Unmap()
	for _, p := range prefixes {
		if p.Contains(addr) {
			return true
		}
	}
	return false
}

// clientIP returns the IP to attribute the request to. RemoteAddr is the
// authority unless it falls inside trusted (a configured reverse-proxy CIDR),
// in which case X-Forwarded-For is walked right-to-left and the first
// untrusted hop is returned. With no trusted proxies configured, XFF is
// ignored entirely so it cannot be spoofed.
func clientIP(r *http.Request, trusted []netip.Prefix) string {
	remoteHost, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		remoteHost = r.RemoteAddr
	}
	remote, err := netip.ParseAddr(remoteHost)
	if err != nil {
		return remoteHost
	}
	if !ipInPrefixes(remote, trusted) {
		return remote.Unmap().String()
	}
	xff := r.Header.Get("X-Forwarded-For")
	if xff == "" {
		return remote.Unmap().String()
	}
	parts := strings.Split(xff, ",")
	for i := len(parts) - 1; i >= 0; i-- {
		hop := strings.TrimSpace(parts[i])
		if hop == "" {
			continue
		}
		a, err := netip.ParseAddr(hop)
		if err != nil {
			return hop
		}
		if !ipInPrefixes(a, trusted) {
			return a.Unmap().String()
		}
	}
	return remote.Unmap().String()
}

// rateLimitMiddleware wraps next and applies rl to POST requests only.
// Non-POST requests pass through unconditionally.
// Blocked requests receive 429 with a Retry-After header.
func rateLimitMiddleware(rl *RateLimiter, trusted []netip.Prefix, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			ip := clientIP(r, trusted)
			if ok, retryAfter := rl.Allow(ip); !ok {
				w.Header().Set("Retry-After", fmt.Sprintf("%d", retryAfter))
				http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}
