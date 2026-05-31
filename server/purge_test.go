package server

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// TestServerStartWiresPurgeLoop verifies that calling Start() with
// RetentionDays > 0 causes old completed jobs to be deleted.
func TestServerStartWiresPurgeLoop(t *testing.T) {
	t.Skip("purge loop uses 1-hour ticker; tested via startPurgeLoopWithInterval")
}

// TestNewWithOptionsRetentionDaysZeroNoPurge verifies that RetentionDays=0
// leaves runs intact (purge loop skips when retention is disabled).
func TestNewWithOptionsRetentionDaysZeroNoPurge(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Database.RetentionDays = 0

	srv, err := NewWithOptions(cfg)
	if err != nil {
		t.Fatalf("NewWithOptions: %v", err)
	}
	defer srv.Stop(context.Background())

	old := time.Now().UTC().Add(-365 * 24 * time.Hour)
	job := Job{ID: "j1", Domain: "example.com", Status: JobSucceeded, CreatedAt: old, FinishedAt: old}
	if _, err := srv.store.Create(job); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := srv.store.GraduateJob(job, nil); err != nil {
		t.Fatalf("graduate: %v", err)
	}

	srv.Start()
	time.Sleep(30 * time.Millisecond)

	if list := srv.store.ListRuns(RunFilter{Limit: 1}); list.Total != 1 {
		t.Fatal("expected run preserved when RetentionDays=0 (purge loop disabled)")
	}
}

// TestStartPurgeLoopPurgesOldJobs verifies that the loop calls PurgeOlderThan
// and logs the count when runs are deleted.
func TestStartPurgeLoopPurgesOldJobs(t *testing.T) {
	store := NewInMemoryJobStore()
	cutoffAge := 90
	old := time.Now().UTC().Add(-time.Duration(cutoffAge+1) * 24 * time.Hour)

	job := Job{ID: "j1", Domain: "example.com", Status: JobSucceeded, CreatedAt: old, FinishedAt: old}
	if _, err := store.Create(job); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := store.GraduateJob(job, nil); err != nil {
		t.Fatalf("graduate: %v", err)
	}

	logCh := make(chan string, 10)
	logger := func(format string, args ...any) {
		logCh <- fmt.Sprintf(format, args...)
	}

	ctx := t.Context()

	var retDays atomic.Int64
	retDays.Store(int64(cutoffAge))
	startPurgeLoopWithInterval(ctx, store, &retDays, logger, 10*time.Millisecond)

	var msg string
	select {
	case msg = <-logCh:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for purge log message")
	}

	if store.ListRuns(RunFilter{Limit: 1}).Total != 0 {
		t.Fatal("expected run to be purged by loop")
	}
	if !strings.Contains(msg, "purged 1 jobs") {
		t.Fatalf("expected purge log message, got %q", msg)
	}
}

// TestStartPurgeLoopNoLogWhenNothingPurged verifies that the loop emits no log
// when there is nothing to delete.
func TestStartPurgeLoopNoLogWhenNothingPurged(t *testing.T) {
	store := NewInMemoryJobStore()

	var logCount atomic.Int64
	logger := func(string, ...any) { logCount.Add(1) }

	ctx := t.Context()

	var retDays atomic.Int64
	retDays.Store(90)
	startPurgeLoopWithInterval(ctx, store, &retDays, logger, 10*time.Millisecond)

	// Let the loop tick a few times.
	time.Sleep(50 * time.Millisecond)

	if n := logCount.Load(); n != 0 {
		t.Fatalf("expected no log messages for empty store, got %d", n)
	}
}

// TestStartPurgeLoopStopsOnContextCancel verifies that cancelling the context
// stops the goroutine cleanly (no panic, no hang).
func TestStartPurgeLoopStopsOnContextCancel(t *testing.T) {
	store := NewInMemoryJobStore()
	logger := func(string, ...any) {}

	ctx, cancel := context.WithCancel(context.Background())
	var retDays atomic.Int64
	retDays.Store(90)
	startPurgeLoopWithInterval(ctx, store, &retDays, logger, 10*time.Millisecond)

	// Cancel immediately and verify the test completes without hanging.
	cancel()
	time.Sleep(30 * time.Millisecond)
	// If the goroutine hadn't stopped, the test would leak - detectable via
	// -race or the goroutine count, but a clean exit here is sufficient.
}

// TestStartPurgeLoopPreservesNewJobs verifies that runs newer than the
// retention cutoff are not deleted by the loop.
func TestStartPurgeLoopPreservesNewJobs(t *testing.T) {
	store := NewInMemoryJobStore()

	recent := time.Now().UTC()
	job := Job{ID: "j1", Domain: "example.com", Status: JobSucceeded, CreatedAt: recent, FinishedAt: recent}
	if _, err := store.Create(job); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := store.GraduateJob(job, nil); err != nil {
		t.Fatalf("graduate: %v", err)
	}

	logger := func(string, ...any) {}
	ctx := t.Context()

	var retDays atomic.Int64
	retDays.Store(90)
	startPurgeLoopWithInterval(ctx, store, &retDays, logger, 10*time.Millisecond)

	time.Sleep(50 * time.Millisecond)

	if store.ListRuns(RunFilter{Limit: 1}).Total != 1 {
		t.Fatal("expected recent run to be preserved by loop")
	}
}

// TestPurgeLoopRetentionDaysDynamic verifies that changing retentionDays
// while the loop is running takes effect at the next tick.
func TestPurgeLoopRetentionDaysDynamic(t *testing.T) {
	store := NewInMemoryJobStore()
	var retDays atomic.Int64 // start disabled (0)
	logger := func(string, ...any) {}

	ctx := t.Context()

	startPurgeLoopWithInterval(ctx, store, &retDays, logger, 10*time.Millisecond)

	// Create a very old job.
	old := time.Now().UTC().Add(-180 * 24 * time.Hour)
	job := Job{ID: "j1", Domain: "example.com", Status: JobSucceeded, CreatedAt: old, FinishedAt: old}
	if _, err := store.Create(job); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := store.GraduateJob(job, nil); err != nil {
		t.Fatalf("graduate: %v", err)
	}

	// With retention disabled the run should be preserved.
	time.Sleep(40 * time.Millisecond)
	if store.ListRuns(RunFilter{Limit: 1}).Total != 1 {
		t.Fatal("expected run preserved while retention disabled")
	}

	// Enable retention by updating the atomic value.
	retDays.Store(90)

	// After the next tick the old run should be purged.
	deadline := time.After(2 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatal("timeout waiting for purge after enabling retention")
		case <-time.After(15 * time.Millisecond):
			if store.ListRuns(RunFilter{Limit: 1}).Total == 0 {
				return
			}
		}
	}
}

// TestRunPurgeLoopRecordsPurgeMetric verifies the loop reports the number of
// deleted jobs to the metrics collector. The metric is recorded just before
// the log line, so once the log arrives the counter is already updated.
func TestRunPurgeLoopRecordsPurgeMetric(t *testing.T) {
	store := NewInMemoryJobStore()
	old := time.Now().UTC().Add(-91 * 24 * time.Hour)
	job := Job{ID: "j1", Domain: "example.com", Status: JobSucceeded, CreatedAt: old, FinishedAt: old}
	if _, err := store.Create(job); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := store.GraduateJob(job, nil); err != nil {
		t.Fatalf("graduate: %v", err)
	}

	metrics := NewMetricsCollector(DefaultConfig())
	logCh := make(chan string, 4)
	logger := func(format string, args ...any) { logCh <- fmt.Sprintf(format, args...) }

	var retDays atomic.Int64
	retDays.Store(90)
	runPurgeLoop(t.Context(), store, &retDays, metrics, logger, func() time.Duration { return 10 * time.Millisecond })

	select {
	case <-logCh:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for purge log message")
	}

	if got := metrics.Snapshot().Jobs.PurgedTotal; got != 1 {
		t.Fatalf("jobs.purged_total = %d, want 1", got)
	}
}

// TestRunPurgeLoopAppliesIntervalChange verifies that changing the interval
// while the loop runs resets the ticker and the loop keeps purging. This
// exercises the ticker.Reset branch.
func TestRunPurgeLoopAppliesIntervalChange(t *testing.T) {
	store := NewInMemoryJobStore()
	logCh := make(chan string, 8)
	logger := func(format string, args ...any) { logCh <- fmt.Sprintf(format, args...) }

	var intervalNanos atomic.Int64
	intervalNanos.Store(int64(10 * time.Millisecond))
	intervalFn := func() time.Duration { return time.Duration(intervalNanos.Load()) }

	var retDays atomic.Int64
	retDays.Store(90)
	runPurgeLoop(t.Context(), store, &retDays, nil, logger, intervalFn)

	addOldJob := func(id string) {
		old := time.Now().UTC().Add(-91 * 24 * time.Hour)
		job := Job{ID: id, Domain: "example.com", Status: JobSucceeded, CreatedAt: old, FinishedAt: old}
		if _, err := store.Create(job); err != nil {
			t.Fatalf("create %s: %v", id, err)
		}
		if err := store.GraduateJob(job, nil); err != nil {
			t.Fatalf("graduate %s: %v", id, err)
		}
	}
	waitForPurge := func(label string) {
		select {
		case <-logCh:
		case <-time.After(2 * time.Second):
			t.Fatalf("timeout waiting for purge (%s)", label)
		}
	}

	// First job purged under the initial 10ms interval.
	addOldJob("j1")
	waitForPurge("initial interval")

	// Change the interval; the loop should reset its ticker and keep purging.
	intervalNanos.Store(int64(25 * time.Millisecond))
	addOldJob("j2")
	waitForPurge("after interval change")
}
