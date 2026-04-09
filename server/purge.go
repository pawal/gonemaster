package server

import (
	"context"
	"sync/atomic"
	"time"
)

// startPurgeLoop runs a background goroutine that calls store.PurgeOlderThan
// every hour. It exits when ctx is cancelled. retentionDays is an atomic so
// that changes made via the settings API take effect at the next tick without
// a server restart. The goroutine is a no-op when retentionDays.Load() <= 0.
// logger is called for each purge cycle that deletes at least one job.
func startPurgeLoop(ctx context.Context, store JobStore, retentionDays *atomic.Int64, logger func(string, ...any)) {
	startPurgeLoopWithInterval(ctx, store, retentionDays, logger, time.Hour)
}

// startPurgeLoopWithInterval is the testable implementation; interval is
// exposed so tests can use a short tick without sleeping for an hour.
func startPurgeLoopWithInterval(ctx context.Context, store JobStore, retentionDays *atomic.Int64, logger func(string, ...any), interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				days := int(retentionDays.Load())
				if days <= 0 {
					continue
				}
				cutoff := time.Now().UTC().Add(-time.Duration(days) * 24 * time.Hour)
				n, err := store.PurgeOlderThan(cutoff)
				if err != nil {
					logger("purge error: %v", err)
					continue
				}
				if n > 0 {
					logger("purged %d jobs older than %s", n, cutoff.Format(time.RFC3339))
				}
			}
		}
	}()
}
