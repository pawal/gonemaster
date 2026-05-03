package transport

import (
	"context"
	"sync"
)

type limiterKey struct{}

// Limiter caps the number of concurrent DNS queries.
type Limiter struct {
	mu     sync.Mutex
	limit  int
	tokens chan struct{}
}

// NewLimiter returns a limiter configured with the given limit.
// A limit <= 0 disables the limiter.
func NewLimiter(limit int) *Limiter {
	l := &Limiter{}
	l.SetLimit(limit)
	return l
}

// WithLimiter stores l in ctx for downstream access.
func WithLimiter(ctx context.Context, l *Limiter) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, limiterKey{}, l)
}

// LimiterFromContext returns the limiter stored in ctx, or nil.
func LimiterFromContext(ctx context.Context) *Limiter {
	if ctx == nil {
		return nil
	}
	limiter, _ := ctx.Value(limiterKey{}).(*Limiter)
	return limiter
}

// SetLimit configures the limiter.
func (l *Limiter) SetLimit(limit int) {
	if l == nil {
		return
	}
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
	for range limit {
		tokens <- struct{}{}
	}
	l.tokens = tokens
}

func acquireQuerySlot(ctx context.Context) error {
	limiter := LimiterFromContext(ctx)
	if limiter == nil {
		return nil
	}
	return limiter.acquire(ctx)
}

func releaseQuerySlot(ctx context.Context) {
	limiter := LimiterFromContext(ctx)
	if limiter == nil {
		return
	}
	limiter.release()
}

func (l *Limiter) acquire(ctx context.Context) error {
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

func (l *Limiter) release() {
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
