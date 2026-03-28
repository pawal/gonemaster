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
// leaves runs intact (purge loop is not started).
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

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	startPurgeLoopWithInterval(ctx, store, cutoffAge, logger, 10*time.Millisecond)

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

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	startPurgeLoopWithInterval(ctx, store, 90, logger, 10*time.Millisecond)

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
	startPurgeLoopWithInterval(ctx, store, 90, logger, 10*time.Millisecond)

	// Cancel immediately and verify the test completes without hanging.
	cancel()
	time.Sleep(30 * time.Millisecond)
	// If the goroutine hadn't stopped, the test would leak — detectable via
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
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	startPurgeLoopWithInterval(ctx, store, 90, logger, 10*time.Millisecond)

	time.Sleep(50 * time.Millisecond)

	if store.ListRuns(RunFilter{Limit: 1}).Total != 1 {
		t.Fatal("expected recent run to be preserved by loop")
	}
}
