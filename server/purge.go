package server

import (
	"context"
	"time"
)

// startPurgeLoop runs a background goroutine that calls store.PurgeOlderThan
// every hour. It exits when ctx is cancelled. retentionDays must be > 0.
// logger is called with fmt.Sprintf-style arguments for each purge cycle that
// deletes at least one job; it is never called when count is zero.
func startPurgeLoop(ctx context.Context, store JobStore, retentionDays int, logger func(string, ...any)) {
	startPurgeLoopWithInterval(ctx, store, retentionDays, logger, time.Hour)
}

// startPurgeLoopWithInterval is the testable implementation; interval is
// exposed so tests can use a short tick without sleeping for an hour.
func startPurgeLoopWithInterval(ctx context.Context, store JobStore, retentionDays int, logger func(string, ...any), interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				cutoff := time.Now().UTC().Add(-time.Duration(retentionDays) * 24 * time.Hour)
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
