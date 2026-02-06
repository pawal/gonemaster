package server

import (
	"math"
	"sort"
	"strings"
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

var metricsStatusClasses = [...]string{
	"1xx",
	"2xx",
	"3xx",
	"4xx",
	"5xx",
}

var metricsLatencyBucketsMs = [...]int64{
	5,
	10,
	25,
	50,
	100,
	250,
	500,
	1000,
	2500,
	5000,
	10000,
}

type MetricsSnapshot struct {
	SchemaVersion string                `json:"schema_version"`
	GeneratedAt   time.Time             `json:"generated_at"`
	Health        MetricsHealthSnapshot `json:"health"`
	Jobs          MetricsJobsSnapshot   `json:"jobs"`
	API           MetricsAPISnapshot    `json:"api"`
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

type MetricsAPISnapshot struct {
	RequestsTotal     int64                    `json:"requests_total"`
	StatusClassCounts map[string]int64         `json:"status_class_counts"`
	ErrorCodeCounts   map[string]int64         `json:"error_code_counts"`
	Routes            []MetricsAPIRouteMetrics `json:"routes"`
}

type MetricsAPIRouteMetrics struct {
	Route             string            `json:"route"`
	Method            string            `json:"method"`
	RequestsTotal     int64             `json:"requests_total"`
	StatusClassCounts map[string]int64  `json:"status_class_counts"`
	LatencyMs         MetricsPercentile `json:"latency_ms"`
}

type MetricsPercentile struct {
	P50 int64 `json:"p50"`
	P90 int64 `json:"p90"`
	P99 int64 `json:"p99"`
}

type apiRouteMetrics struct {
	Route             string
	Method            string
	RequestsTotal     int64
	StatusClassCounts map[string]int64
	Latency           latencyHistogram
}

type latencyHistogram struct {
	Counts []int64
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

	apiRequestsTotal     int64
	apiStatusClassCounts map[string]int64
	apiErrorCodeCounts   map[string]int64
	apiRoutes            map[string]*apiRouteMetrics
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
		apiStatusClassCounts: zeroStatusClassCounts(),
		apiErrorCodeCounts:   map[string]int64{},
		apiRoutes:            map[string]*apiRouteMetrics{},
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

func (m *MetricsCollector) ObserveAPIRequest(route string, method string, statusCode int, duration time.Duration, errorCode string) {
	if route == "" {
		route = "/api/v1/unknown"
	}
	if method == "" {
		method = "UNKNOWN"
	}
	method = strings.ToUpper(method)
	statusClass := statusClassFromCode(statusCode)
	key := method + " " + route

	m.mu.Lock()
	m.apiRequestsTotal++
	m.apiStatusClassCounts[statusClass]++
	if errorCode != "" {
		m.apiErrorCodeCounts[errorCode]++
	}

	routeMetrics := m.apiRoutes[key]
	if routeMetrics == nil {
		routeMetrics = &apiRouteMetrics{
			Route:             route,
			Method:            method,
			StatusClassCounts: zeroStatusClassCounts(),
			Latency:           newLatencyHistogram(),
		}
		m.apiRoutes[key] = routeMetrics
	}
	routeMetrics.RequestsTotal++
	routeMetrics.StatusClassCounts[statusClass]++
	routeMetrics.Latency.Observe(duration)
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
	apiRequestsTotal := m.apiRequestsTotal
	apiStatusClassCounts := copyStatusClassCounts(m.apiStatusClassCounts)
	apiErrorCodeCounts := copyStringCounts(m.apiErrorCodeCounts)
	apiRoutes := m.copyAPIRouteMetricsLocked()
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
		API: MetricsAPISnapshot{
			RequestsTotal:     apiRequestsTotal,
			StatusClassCounts: apiStatusClassCounts,
			ErrorCodeCounts:   apiErrorCodeCounts,
			Routes:            apiRoutes,
		},
	}
}

func (m *MetricsCollector) copyAPIRouteMetricsLocked() []MetricsAPIRouteMetrics {
	routes := make([]MetricsAPIRouteMetrics, 0, len(m.apiRoutes))
	for _, route := range m.apiRoutes {
		if route == nil {
			continue
		}
		routes = append(routes, MetricsAPIRouteMetrics{
			Route:             route.Route,
			Method:            route.Method,
			RequestsTotal:     route.RequestsTotal,
			StatusClassCounts: copyStatusClassCounts(route.StatusClassCounts),
			LatencyMs: MetricsPercentile{
				P50: route.Latency.Quantile(0.50),
				P90: route.Latency.Quantile(0.90),
				P99: route.Latency.Quantile(0.99),
			},
		})
	}
	sort.Slice(routes, func(i, j int) bool {
		if routes[i].Route == routes[j].Route {
			return routes[i].Method < routes[j].Method
		}
		return routes[i].Route < routes[j].Route
	})
	return routes
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

func zeroStatusClassCounts() map[string]int64 {
	counts := make(map[string]int64, len(metricsStatusClasses))
	for _, statusClass := range metricsStatusClasses {
		counts[statusClass] = 0
	}
	return counts
}

func copyStatusClassCounts(in map[string]int64) map[string]int64 {
	out := make(map[string]int64, len(metricsStatusClasses))
	for _, statusClass := range metricsStatusClasses {
		out[statusClass] = in[statusClass]
	}
	return out
}

func copyStringCounts(in map[string]int64) map[string]int64 {
	out := make(map[string]int64, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func statusClassFromCode(statusCode int) string {
	switch {
	case statusCode >= 100 && statusCode < 200:
		return "1xx"
	case statusCode >= 200 && statusCode < 300:
		return "2xx"
	case statusCode >= 300 && statusCode < 400:
		return "3xx"
	case statusCode >= 400 && statusCode < 500:
		return "4xx"
	case statusCode >= 500 && statusCode < 600:
		return "5xx"
	default:
		return "5xx"
	}
}

func newLatencyHistogram() latencyHistogram {
	return latencyHistogram{Counts: make([]int64, len(metricsLatencyBucketsMs)+1)}
}

func (h *latencyHistogram) Observe(duration time.Duration) {
	if h == nil {
		return
	}
	ms := float64(duration) / float64(time.Millisecond)
	if ms < 0 {
		ms = 0
	}
	for idx, bucketUpperBound := range metricsLatencyBucketsMs {
		if ms <= float64(bucketUpperBound) {
			h.Counts[idx]++
			return
		}
	}
	h.Counts[len(h.Counts)-1]++
}

func (h latencyHistogram) Quantile(quantile float64) int64 {
	total := int64(0)
	for _, count := range h.Counts {
		total += count
	}
	if total == 0 {
		return 0
	}
	target := int64(math.Ceil(quantile * float64(total)))
	if target < 1 {
		target = 1
	}

	seen := int64(0)
	for idx, count := range h.Counts {
		seen += count
		if seen >= target {
			if idx < len(metricsLatencyBucketsMs) {
				return metricsLatencyBucketsMs[idx]
			}
			return metricsLatencyBucketsMs[len(metricsLatencyBucketsMs)-1]
		}
	}
	return metricsLatencyBucketsMs[len(metricsLatencyBucketsMs)-1]
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
