package transport

import (
	"context"
	"sync"
)

var globalQueryLimiter = &queryLimiter{}

type queryLimiter struct {
	mu     sync.Mutex
	limit  int
	tokens chan struct{}
}

// SetGlobalQueryLimit configures a process-wide limit for concurrent DNS queries.
// A limit <= 0 disables the limiter.
func SetGlobalQueryLimit(limit int) {
	globalQueryLimiter.set(limit)
}

func resetGlobalQueryLimit() {
	globalQueryLimiter.set(0)
}

func (l *queryLimiter) set(limit int) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if limit <= 0 {
		l.limit = 0
		l.tokens = nil
		return
	}
	if limit == l.limit && l.tokens != nil {
		return
	}

	l.limit = limit
	tokens := make(chan struct{}, limit)
	for i := 0; i < limit; i++ {
		tokens <- struct{}{}
	}
	l.tokens = tokens
}

func acquireQuerySlot(ctx context.Context) error {
	return globalQueryLimiter.acquire(ctx)
}

func releaseQuerySlot() {
	globalQueryLimiter.release()
}

func (l *queryLimiter) acquire(ctx context.Context) error {
	l.mu.Lock()
	tokens := l.tokens
	l.mu.Unlock()

	if tokens == nil {
		return nil
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-tokens:
		return nil
	}
}

func (l *queryLimiter) release() {
	l.mu.Lock()
	tokens := l.tokens
	l.mu.Unlock()

	if tokens == nil {
		return
	}

	select {
	case tokens <- struct{}{}:
	default:
	}
}
