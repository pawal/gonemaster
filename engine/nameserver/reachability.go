package nameserver

import (
	"context"
	"errors"
	"net"
	"strings"
	"sync"
	"syscall"
	"time"
)

type reachabilityCache struct {
	mu   sync.Mutex
	data map[string]time.Time
	met  cacheMetrics
}

func newReachabilityCache() *reachabilityCache {
	return &reachabilityCache{data: map[string]time.Time{}}
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

func (c *reachabilityCache) mark(addr string, ttl time.Duration) {
	if c == nil || addr == "" || ttl <= 0 {
		return
	}
	c.mu.Lock()
	c.data[addr] = time.Now().Add(ttl)
	c.mu.Unlock()
}

func (c *reachabilityCache) clear() {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.met.evict(len(c.data))
	c.data = map[string]time.Time{}
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

func isHardNetworkError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
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
