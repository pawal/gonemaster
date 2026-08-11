package nameserver

import (
	"errors"
	"net"
	"strings"
	"sync"
	"syscall"
	"time"
)

// defaultReachabilityMaxEntries caps blackout and pending entries per cache.
const defaultReachabilityMaxEntries = 1024

type reachabilityCache struct {
	mu         sync.Mutex
	data       map[string]time.Time
	pending    map[string]time.Time
	met        cacheMetrics
	maxEntries int
}

func newReachabilityCache() *reachabilityCache {
	return newReachabilityCacheWithLimit(defaultReachabilityMaxEntries)
}

// newReachabilityCacheWithLimit builds a cache with an explicit entry cap.
// A limit of 0 or less means unbounded.
func newReachabilityCacheWithLimit(max int) *reachabilityCache {
	return &reachabilityCache{
		data:       map[string]time.Time{},
		pending:    map[string]time.Time{},
		maxEntries: max,
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
		c.sweepLocked(now, ttl)
		c.evictOldestLocked(c.data)
		c.data[addr] = now.Add(ttl)
		delete(c.pending, addr)
		return
	}
	c.sweepLocked(now, ttl)
	c.evictOldestLocked(c.pending)
	c.pending[addr] = now
}

// sweepLocked drops expired blackouts and pending strikes too old to promote.
// mark is the rare path, so an O(n) sweep here keeps shouldSkip O(1).
func (c *reachabilityCache) sweepLocked(now time.Time, ttl time.Duration) {
	evicted := 0
	for addr, expiry := range c.data {
		if !now.Before(expiry) {
			delete(c.data, addr)
			evicted++
		}
	}
	for addr, firstSeen := range c.pending {
		if now.Sub(firstSeen) > ttl {
			delete(c.pending, addr)
			evicted++
		}
	}
	c.met.evict(evicted)
}

// evictOldestLocked makes room in m when it is at the entry cap.
func (c *reachabilityCache) evictOldestLocked(m map[string]time.Time) {
	if c.maxEntries <= 0 || len(m) < c.maxEntries {
		return
	}
	oldestAddr := ""
	var oldest time.Time
	for addr, stamp := range m {
		if oldestAddr == "" || stamp.Before(oldest) {
			oldestAddr, oldest = addr, stamp
		}
	}
	if oldestAddr != "" {
		delete(m, oldestAddr)
		c.met.evict(1)
	}
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
	c.data = map[string]time.Time{}
	c.pending = map[string]time.Time{}
	c.met.reset()
	c.mu.Unlock()
}

// metrics reports the counters. It takes the lock so it cannot observe a
// half-finished clear.
func (c *reachabilityCache) metrics() CacheMetrics {
	if c == nil {
		return CacheMetrics{}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.met.snapshot()
}

// len reports live blackout and pending counts, for bound and leak tests.
func (c *reachabilityCache) len() (blocked int, pending int) {
	if c == nil {
		return 0, 0
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.data), len(c.pending)
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
