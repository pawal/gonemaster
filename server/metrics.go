package server

import (
	"sync"
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

var metricsTerminalStatuses = [...]JobStatus{
	JobSucceeded,
	JobFailed,
	JobCanceled,
	JobExpired,
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
	ActiveWorkers     int       `json:"active_workers"`
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
	mu sync.Mutex

	startedAt         time.Time
	workerCount       int
	activeWorkers     int
	maxConcurrentJobs int

	queuePaused bool
	queueDepth  int64
	inFlight    int64

	submittedTotal int64
	startedTotal   int64
	completedTotal int64
	canceledTotal  int64
	statusCounts   map[string]int64
}

func NewMetricsCollector(cfg Config) *MetricsCollector {
	return newMetricsCollector(cfg, time.Now().UTC())
}

func newMetricsCollector(cfg Config, startedAt time.Time) *MetricsCollector {
	activeWorkers := cfg.WorkerCount
	if activeWorkers < 1 {
		activeWorkers = 1
	}
	return &MetricsCollector{
		startedAt:         startedAt.UTC(),
		workerCount:       cfg.WorkerCount,
		activeWorkers:     activeWorkers,
		maxConcurrentJobs: cfg.MaxConcurrentJobs,
		statusCounts:      zeroStatusCounts(),
	}
}

func (m *MetricsCollector) Snapshot() MetricsSnapshot {
	return m.snapshotAt(time.Now().UTC())
}

func (m *MetricsCollector) ObserveQueuePaused(paused bool) {
	m.mu.Lock()
	m.queuePaused = paused
	m.mu.Unlock()
}

func (m *MetricsCollector) ObserveJobSubmitted(initialStatus JobStatus) {
	m.mu.Lock()
	m.submittedTotal++
	m.observeStatusTransitionLocked("", initialStatus)
	m.mu.Unlock()
}

func (m *MetricsCollector) ObserveJobStatusTransition(fromStatus, toStatus JobStatus) {
	m.mu.Lock()
	m.observeStatusTransitionLocked(fromStatus, toStatus)
	m.mu.Unlock()
}

func (m *MetricsCollector) observeStatusTransitionLocked(fromStatus, toStatus JobStatus) {
	if fromStatus == toStatus {
		return
	}

	if isKnownMetricsStatus(fromStatus) {
		key := string(fromStatus)
		if m.statusCounts[key] > 0 {
			m.statusCounts[key]--
		}
	}
	if isKnownMetricsStatus(toStatus) {
		m.statusCounts[string(toStatus)]++
	}

	if fromStatus != JobRunning && toStatus == JobRunning {
		m.startedTotal++
		m.inFlight++
	}
	if fromStatus == JobRunning && toStatus != JobRunning && m.inFlight > 0 {
		m.inFlight--
	}

	if fromStatus != JobQueued && toStatus == JobQueued {
		m.queueDepth++
	}
	if fromStatus == JobQueued && toStatus != JobQueued && m.queueDepth > 0 {
		m.queueDepth--
	}

	if !isTerminalMetricsStatus(fromStatus) && isTerminalMetricsStatus(toStatus) {
		m.completedTotal++
		if toStatus == JobCanceled {
			m.canceledTotal++
		}
	}
}

func (m *MetricsCollector) snapshotAt(now time.Time) MetricsSnapshot {
	now = now.UTC()
	uptime := int64(now.Sub(m.startedAt).Seconds())
	if uptime < 0 {
		uptime = 0
	}

	m.mu.Lock()
	statusCounts := copyStatusCounts(m.statusCounts)
	queuePaused := m.queuePaused
	queueDepth := m.queueDepth
	inFlight := m.inFlight
	submittedTotal := m.submittedTotal
	startedTotal := m.startedTotal
	completedTotal := m.completedTotal
	canceledTotal := m.canceledTotal
	m.mu.Unlock()

	return MetricsSnapshot{
		SchemaVersion: metricsSchemaVersion,
		GeneratedAt:   now,
		Health: MetricsHealthSnapshot{
			StartedAt:         m.startedAt,
			UptimeSeconds:     uptime,
			WorkerCount:       m.workerCount,
			ActiveWorkers:     m.activeWorkers,
			MaxConcurrentJobs: m.maxConcurrentJobs,
			QueuePaused:       queuePaused,
			QueueDepth:        queueDepth,
			InFlightJobs:      inFlight,
		},
		Jobs: MetricsJobsSnapshot{
			SubmittedTotal: submittedTotal,
			StartedTotal:   startedTotal,
			CompletedTotal: completedTotal,
			CanceledTotal:  canceledTotal,
			StatusCounts:   statusCounts,
		},
	}
}

func zeroStatusCounts() map[string]int64 {
	counts := make(map[string]int64, len(metricsJobStatuses))
	for _, status := range metricsJobStatuses {
		counts[string(status)] = 0
	}
	return counts
}

func copyStatusCounts(in map[string]int64) map[string]int64 {
	out := make(map[string]int64, len(metricsJobStatuses))
	for _, status := range metricsJobStatuses {
		key := string(status)
		out[key] = in[key]
	}
	return out
}

func isKnownMetricsStatus(status JobStatus) bool {
	for _, known := range metricsJobStatuses {
		if status == known {
			return true
		}
	}
	return false
}

func isTerminalMetricsStatus(status JobStatus) bool {
	for _, terminal := range metricsTerminalStatuses {
		if status == terminal {
			return true
		}
	}
	return false
}
