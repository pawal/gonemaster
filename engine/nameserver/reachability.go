package nameserver

import (
	"errors"
	"net"
	"strings"
	"sync"
	"syscall"
	"time"
)

type reachabilityCache struct {
	mu      sync.Mutex
	data    map[string]time.Time
	pending map[string]time.Time
	met     cacheMetrics
}

func newReachabilityCache() *reachabilityCache {
	return &reachabilityCache{
		data:    map[string]time.Time{},
		pending: map[string]time.Time{},
	}
}

func (c *reachabilityCache) shouldSkip(addr string) (bool, time.Duration) {
	if c == nil || addr == "" {
		return false, 0
	}
	now := time.Now()
	c.mu.Lock()
	defer c.mu.Unlock()
	expiry, ok := c.data[addr]
	if !ok {
		c.met.miss()
		return false, 0
	}
	if now.Before(expiry) {
		c.met.hit()
		return true, expiry.Sub(now)
	}
	delete(c.data, addr)
	c.met.evict(1)
	c.met.miss()
	return false, 0
}

// mark debounces: a single hard error records the address as pending but
// does not block. Only the second hard error within ttl promotes the
// address to a full blackout. This tolerates transient ICMP unreachables
// and IPv6 routing flaps that resolve on retry.
func (c *reachabilityCache) mark(addr string, ttl time.Duration) {
	if c == nil || addr == "" || ttl <= 0 {
		return
	}
	now := time.Now()
	c.mu.Lock()
	defer c.mu.Unlock()
	if expiry, ok := c.data[addr]; ok && now.Before(expiry) {
		return
	}
	if firstSeen, ok := c.pending[addr]; ok && now.Sub(firstSeen) <= ttl {
		c.data[addr] = now.Add(ttl)
		delete(c.pending, addr)
		return
	}
	c.pending[addr] = now
}

// observeSuccess clears the pending hard-error count for addr after a
// successful query. A reachable address must not accumulate stale
// pending state from earlier transient failures.
func (c *reachabilityCache) observeSuccess(addr string) {
	if c == nil || addr == "" {
		return
	}
	c.mu.Lock()
	delete(c.pending, addr)
	c.mu.Unlock()
}

func (c *reachabilityCache) clear() {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.met.evict(len(c.data))
	c.data = map[string]time.Time{}
	c.pending = map[string]time.Time{}
	c.met = cacheMetrics{}
	c.mu.Unlock()
}

var globalReachability = newReachabilityCache()

func clearReachabilityCache() {
	globalReachability.clear()
}

func reachabilityMetricsSnapshot() CacheMetrics {
	return globalReachability.met.snapshot()
}

// isHardNetworkError reports whether err is a "host not reachable from here"
// failure (no-route, host/net unreachable/down). Timeouts return false here.
// Caller gates on outer ctx.Err() for job-cancellation attribution.
func isHardNetworkError(err error) bool {
	if err == nil {
		return false
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return false
	}

	hardErrs := []error{
		syscall.EHOSTUNREACH,
		syscall.ENETUNREACH,
		syscall.EHOSTDOWN,
		syscall.ENETDOWN,
	}
	for _, target := range hardErrs {
		if errors.Is(err, target) {
			return true
		}
	}

	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "no route to host") ||
		strings.Contains(msg, "network is unreachable") ||
		strings.Contains(msg, "host is unreachable") ||
		strings.Contains(msg, "host unreachable") {
		return true
	}

	return false
}
