package server

import (
	"testing"
	"time"
)

func TestMetricsCollectorZeroStateSnapshot(t *testing.T) {
	cfg := DefaultConfig()
	cfg.WorkerCount = 7
	cfg.MaxConcurrentJobs = 3

	startedAt := time.Date(2026, 2, 7, 12, 0, 0, 0, time.UTC)
	now := startedAt.Add(95 * time.Second)
	collector := newMetricsCollector(cfg, startedAt)

	snapshot := collector.snapshotAt(now)

	if snapshot.SchemaVersion != metricsSchemaVersion {
		t.Fatalf("schema_version = %q, want %q", snapshot.SchemaVersion, metricsSchemaVersion)
	}
	if !snapshot.GeneratedAt.Equal(now) {
		t.Fatalf("generated_at = %s, want %s", snapshot.GeneratedAt, now)
	}

	if !snapshot.Health.StartedAt.Equal(startedAt) {
		t.Fatalf("health.started_at = %s, want %s", snapshot.Health.StartedAt, startedAt)
	}
	if snapshot.Health.UptimeSeconds != 95 {
		t.Fatalf("health.uptime_seconds = %d, want 95", snapshot.Health.UptimeSeconds)
	}
	if snapshot.Health.WorkerCount != 7 {
		t.Fatalf("health.worker_count = %d, want 7", snapshot.Health.WorkerCount)
	}
	if snapshot.Health.MaxConcurrentJobs != 3 {
		t.Fatalf("health.max_concurrent_jobs = %d, want 3", snapshot.Health.MaxConcurrentJobs)
	}
	if snapshot.Health.QueuePaused {
		t.Fatal("health.queue_paused = true, want false")
	}
	if snapshot.Health.QueueDepth != 0 {
		t.Fatalf("health.queue_depth = %d, want 0", snapshot.Health.QueueDepth)
	}
	if snapshot.Health.InFlightJobs != 0 {
		t.Fatalf("health.in_flight_jobs = %d, want 0", snapshot.Health.InFlightJobs)
	}

	if snapshot.Jobs.SubmittedTotal != 0 {
		t.Fatalf("jobs.submitted_total = %d, want 0", snapshot.Jobs.SubmittedTotal)
	}
	if snapshot.Jobs.StartedTotal != 0 {
		t.Fatalf("jobs.started_total = %d, want 0", snapshot.Jobs.StartedTotal)
	}
	if snapshot.Jobs.CompletedTotal != 0 {
		t.Fatalf("jobs.completed_total = %d, want 0", snapshot.Jobs.CompletedTotal)
	}
	if snapshot.Jobs.CanceledTotal != 0 {
		t.Fatalf("jobs.canceled_total = %d, want 0", snapshot.Jobs.CanceledTotal)
	}

	if len(snapshot.Jobs.StatusCounts) != len(metricsJobStatuses) {
		t.Fatalf("jobs.status_counts size = %d, want %d", len(snapshot.Jobs.StatusCounts), len(metricsJobStatuses))
	}
	for _, status := range metricsJobStatuses {
		if count := snapshot.Jobs.StatusCounts[string(status)]; count != 0 {
			t.Fatalf("jobs.status_counts[%q] = %d, want 0", status, count)
		}
	}
}

func TestMetricsCollectorUptimeDoesNotGoNegative(t *testing.T) {
	cfg := DefaultConfig()
	startedAt := time.Date(2026, 2, 7, 12, 0, 0, 0, time.UTC)
	collector := newMetricsCollector(cfg, startedAt)
	snapshot := collector.snapshotAt(startedAt.Add(-time.Second))

	if snapshot.Health.UptimeSeconds != 0 {
		t.Fatalf("health.uptime_seconds = %d, want 0", snapshot.Health.UptimeSeconds)
	}
}
