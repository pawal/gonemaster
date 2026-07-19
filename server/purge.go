package server

import (
	"context"
	"log/slog"
	"sync/atomic"
	"time"
)

// startPurgeLoop runs the retention purge loop in the background, reading the
// retention window and sweep interval from atomics so both take effect at the
// next tick when changed via the settings API. metrics may be nil. The loop
// exits when ctx is cancelled and is a no-op while retentionDays.Load() <= 0.
func startPurgeLoop(ctx context.Context, store JobStore, retentionDays, purgeIntervalSec *atomic.Int64, metrics *MetricsCollector, logger *slog.Logger) {
	intervalFn := func() time.Duration {
		secs := purgeIntervalSec.Load()
		if secs <= 0 {
			return time.Hour
		}
		return time.Duration(secs) * time.Second
	}
	runPurgeLoop(ctx, store, retentionDays, metrics, logger, intervalFn)
}

// startPurgeLoopWithInterval runs the loop with a fixed interval and no metrics.
// Exposed so tests can use a short tick without sleeping for an hour.
func startPurgeLoopWithInterval(ctx context.Context, store JobStore, retentionDays *atomic.Int64, logger *slog.Logger, interval time.Duration) {
	runPurgeLoop(ctx, store, retentionDays, nil, logger, func() time.Duration { return interval })
}

// runPurgeLoop is the core loop. intervalFn is read each tick; when its result
// changes the ticker is reset so a new interval applies without a restart.
func runPurgeLoop(ctx context.Context, store JobStore, retentionDays *atomic.Int64, metrics *MetricsCollector, logger *slog.Logger, intervalFn func() time.Duration) {
	if logger == nil {
		logger = slog.Default()
	}
	go func() {
		interval := intervalFn()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if d := intervalFn(); d > 0 && d != interval {
					interval = d
					ticker.Reset(d)
				}
				days := int(retentionDays.Load())
				if days <= 0 {
					continue
				}
				cutoff := time.Now().UTC().Add(-time.Duration(days) * 24 * time.Hour)
				n, err := store.PurgeOlderThan(cutoff)
				if err != nil {
					logger.Error("purge failed", "err", err)
					continue
				}
				if n > 0 {
					if metrics != nil {
						metrics.ObserveJobsPurged(n)
					}
					logger.Info("jobs purged", "count", n, "cutoff", cutoff.Format(time.RFC3339))
				}
			}
		}
	}()
}
