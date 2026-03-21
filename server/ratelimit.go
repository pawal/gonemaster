package server

import (
	"fmt"
	"net"
	"net/http"
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
		ra := int(times[0].Add(rl.window).Sub(now).Seconds()) + 1
		if ra < 1 {
			ra = 1
		}
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

// clientIP extracts the client IP from r, preferring X-Forwarded-For (first
// value), then X-Real-IP, then RemoteAddr.
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if idx := strings.IndexByte(xff, ','); idx >= 0 {
			xff = xff[:idx]
		}
		if ip := strings.TrimSpace(xff); ip != "" {
			return ip
		}
	}
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		if ip := strings.TrimSpace(xri); ip != "" {
			return ip
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// rateLimitMiddleware wraps next and applies rl to POST requests only.
// Non-POST requests pass through unconditionally.
// Blocked requests receive 429 with a Retry-After header.
func rateLimitMiddleware(rl *RateLimiter, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			ip := clientIP(r)
			if ok, retryAfter := rl.Allow(ip); !ok {
				w.Header().Set("Retry-After", fmt.Sprintf("%d", retryAfter))
				http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}
