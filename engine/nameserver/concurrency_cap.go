package nameserver

import (
	"context"
	"sync"
)

// nameserverConcurrencyCap coordinates concurrent network queries for one
// nameserver address.
type nameserverConcurrencyCap struct {
	mu       sync.Mutex
	inflight int
	waitCh   chan struct{}
}

func (c *nameserverConcurrencyCap) acquire(ctx context.Context, limit int) error {
	if c == nil || limit <= 0 {
		return nil
	}
	for {
		c.mu.Lock()
		if c.inflight < limit {
			c.inflight++
			c.mu.Unlock()
			return nil
		}
		if c.waitCh == nil {
			c.waitCh = make(chan struct{})
		}
		waitCh := c.waitCh
		c.mu.Unlock()

		if ctx == nil {
			<-waitCh
			continue
		}
		select {
		case <-waitCh:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (c *nameserverConcurrencyCap) release() {
	if c == nil {
		return
	}
	c.mu.Lock()
	if c.inflight > 0 {
		c.inflight--
	}
	if c.waitCh != nil {
		close(c.waitCh)
		c.waitCh = nil
	}
	c.mu.Unlock()
}
