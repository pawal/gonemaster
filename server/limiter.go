package server

import "context"

type engineLimiter struct {
	tokens chan struct{}
}

func newEngineLimiter(max int) *engineLimiter {
	if max < 1 {
		return nil
	}
	return &engineLimiter{tokens: make(chan struct{}, max)}
}

func (l *engineLimiter) Acquire(ctx context.Context) error {
	if l == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case l.tokens <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (l *engineLimiter) Release() {
	if l == nil {
		return
	}
	<-l.tokens
}
