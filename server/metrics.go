package server

import (
	"sync/atomic"
	"time"
)

const metricsSchemaVersion = "v1"

var metricsJobStatuses = [...]JobStatus{
	JobQueued,
	JobRunning,
	JobSucceeded,
	JobFailed,
	JobCanceled,
	JobExpired,
	JobPaused,
}

type MetricsSnapshot struct {
	SchemaVersion string                `json:"schema_version"`
	GeneratedAt   time.Time             `json:"generated_at"`
	Health        MetricsHealthSnapshot `json:"health"`
	Jobs          MetricsJobsSnapshot   `json:"jobs"`
}

type MetricsHealthSnapshot struct {
	StartedAt         time.Time `json:"started_at"`
	UptimeSeconds     int64     `json:"uptime_seconds"`
	WorkerCount       int       `json:"worker_count"`
	MaxConcurrentJobs int       `json:"max_concurrent_jobs"`
	QueuePaused       bool      `json:"queue_paused"`
	QueueDepth        int64     `json:"queue_depth"`
	InFlightJobs      int64     `json:"in_flight_jobs"`
}

type MetricsJobsSnapshot struct {
	SubmittedTotal int64            `json:"submitted_total"`
	StartedTotal   int64            `json:"started_total"`
	CompletedTotal int64            `json:"completed_total"`
	CanceledTotal  int64            `json:"canceled_total"`
	StatusCounts   map[string]int64 `json:"status_counts"`
}

// MetricsCollector stores low-overhead in-memory counters and gauges.
type MetricsCollector struct {
	startedAt         time.Time
	workerCount       int
	maxConcurrentJobs int

	queuePaused atomic.Bool
	queueDepth  atomic.Int64
	inFlight    atomic.Int64

	submittedTotal atomic.Int64
	startedTotal   atomic.Int64
	completedTotal atomic.Int64
	canceledTotal  atomic.Int64
}

func NewMetricsCollector(cfg Config) *MetricsCollector {
	return newMetricsCollector(cfg, time.Now().UTC())
}

func newMetricsCollector(cfg Config, startedAt time.Time) *MetricsCollector {
	return &MetricsCollector{
		startedAt:         startedAt.UTC(),
		workerCount:       cfg.WorkerCount,
		maxConcurrentJobs: cfg.MaxConcurrentJobs,
	}
}

func (m *MetricsCollector) Snapshot() MetricsSnapshot {
	return m.snapshotAt(time.Now().UTC())
}

func (m *MetricsCollector) snapshotAt(now time.Time) MetricsSnapshot {
	now = now.UTC()
	uptime := int64(now.Sub(m.startedAt).Seconds())
	if uptime < 0 {
		uptime = 0
	}

	statusCounts := make(map[string]int64, len(metricsJobStatuses))
	for _, status := range metricsJobStatuses {
		statusCounts[string(status)] = 0
	}

	return MetricsSnapshot{
		SchemaVersion: metricsSchemaVersion,
		GeneratedAt:   now,
		Health: MetricsHealthSnapshot{
			StartedAt:         m.startedAt,
			UptimeSeconds:     uptime,
			WorkerCount:       m.workerCount,
			MaxConcurrentJobs: m.maxConcurrentJobs,
			QueuePaused:       m.queuePaused.Load(),
			QueueDepth:        m.queueDepth.Load(),
			InFlightJobs:      m.inFlight.Load(),
		},
		Jobs: MetricsJobsSnapshot{
			SubmittedTotal: m.submittedTotal.Load(),
			StartedTotal:   m.startedTotal.Load(),
			CompletedTotal: m.completedTotal.Load(),
			CanceledTotal:  m.canceledTotal.Load(),
			StatusCounts:   statusCounts,
		},
	}
}
