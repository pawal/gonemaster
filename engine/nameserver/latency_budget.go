package nameserver

import (
	"sync"
	"time"
)

// latencyTracker bounds cumulative query time per nameserver address per run,
// catching slow-but-responding servers that fast-fail's timeout count misses.
type latencyTracker struct {
	mu      sync.Mutex
	total   time.Duration
	blocked bool
}

// observe adds one query's elapsed time and reports whether this call just
// crossed the budget. budget <= 0 disables the tracker.
func (t *latencyTracker) observe(elapsed, budget time.Duration) bool {
	if budget <= 0 {
		return false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.blocked {
		return false
	}
	t.total += elapsed
	if t.total >= budget {
		t.blocked = true
		return true
	}
	return false
}

// shouldSkip reports whether the address is blocked. budget <= 0 disables it.
func (t *latencyTracker) shouldSkip(budget time.Duration) bool {
	if budget <= 0 {
		return false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.blocked
}
