package server

import (
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"codeberg.org/pawal/gonemaster/engine"
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

var metricsJobDurationBucketsMs = [...]int64{
	100,
	250,
	500,
	1000,
	2500,
	5000,
	10000,
	30000,
	60000,
	120000,
	300000,
	600000,
}

var metricsSeverityLevels = [...]string{
	"NOTICE",
	"WARNING",
	"ERROR",
	"CRITICAL",
}

const (
	metricsMaxLocaleBuckets   = 32
	metricsLocaleOtherKey     = "_other"
	metricsDefaultBatchLimit  = 20
	metricsDefaultDomainLimit = 20
	metricsMaxBatchLimit      = 100
	metricsMaxDomainLimit     = 100
	metricsBatchInsightCap    = 256
	metricsDomainInsightCap   = 256
)

// MetricsSnapshot is the top-level payload returned by the metrics endpoint.
type MetricsSnapshot struct {
	SchemaVersion string                  `json:"schema_version"`
	ServerVersion string                  `json:"server_version"`
	GeneratedAt   time.Time               `json:"generated_at"`
	Health        MetricsHealthSnapshot   `json:"health"`
	Jobs          MetricsJobsSnapshot     `json:"jobs"`
	API           MetricsAPISnapshot      `json:"api"`
	Quality       MetricsQualitySnapshot  `json:"quality"`
	Insights      MetricsInsightsSnapshot `json:"insights"`
	Trends        MetricsTrendsSnapshot   `json:"trends"`
}

// MetricsHealthSnapshot captures queue/worker runtime health.
type MetricsHealthSnapshot struct {
	StartedAt         time.Time `json:"started_at"`
	UptimeSeconds     int64     `json:"uptime_seconds"`
	WorkerCount       int       `json:"worker_count"`
	ActiveWorkers     int       `json:"active_workers"`
	MaxConcurrentJobs int       `json:"max_concurrent_jobs"`
	QueuePaused       bool      `json:"queue_paused"`
	QueueDepth        int64     `json:"queue_depth"`
	InFlightJobs      int64     `json:"in_flight_jobs"`
	DNSQueriesTotal    int64     `json:"dns_queries_total"`
	DNSQueriesIPv4     int64     `json:"dns_queries_ipv4_total"`
	DNSQueriesIPv6     int64     `json:"dns_queries_ipv6_total"`
	DNSCacheHits       int64     `json:"dns_cache_hits"`
	DNSCacheMisses     int64     `json:"dns_cache_misses"`
	DNSCacheEvictions  int64     `json:"dns_cache_evictions"`
}

// MetricsJobsSnapshot captures lifecycle counters for submitted jobs.
type MetricsJobsSnapshot struct {
	SubmittedTotal int64            `json:"submitted_total"`
	StartedTotal   int64            `json:"started_total"`
	CompletedTotal int64            `json:"completed_total"`
	CanceledTotal  int64            `json:"canceled_total"`
	StatusCounts   map[string]int64 `json:"status_counts"`
}

// MetricsAPISnapshot captures aggregate API request statistics.
type MetricsAPISnapshot struct {
	RequestsTotal     int64                    `json:"requests_total"`
	StatusClassCounts map[string]int64         `json:"status_class_counts"`
	ErrorCodeCounts   map[string]int64         `json:"error_code_counts"`
	Routes            []MetricsAPIRouteMetrics `json:"routes"`
}

// MetricsAPIRouteMetrics captures per-route API request statistics.
type MetricsAPIRouteMetrics struct {
	Route             string            `json:"route"`
	Method            string            `json:"method"`
	RequestsTotal     int64             `json:"requests_total"`
	StatusClassCounts map[string]int64  `json:"status_class_counts"`
	LatencyMs         MetricsPercentile `json:"latency_ms"`
}

// MetricsPercentile stores P50/P90/P99 values.
type MetricsPercentile struct {
	P50 int64 `json:"p50"`
	P90 int64 `json:"p90"`
	P99 int64 `json:"p99"`
}

// MetricsQualitySnapshot captures result-quality-oriented metrics.
type MetricsQualitySnapshot struct {
	JobDurationMs MetricsDurationSnapshot `json:"job_duration_ms"`
	Outcomes      MetricsOutcomesSnapshot `json:"outcomes"`
	Severity      MetricsSeveritySnapshot `json:"severity"`
	LocaleUsage   MetricsLocaleSnapshot   `json:"locale_usage"`
}

// MetricsDurationSnapshot captures latency histogram summaries.
type MetricsDurationSnapshot struct {
	Count int64             `json:"count"`
	Avg   float64           `json:"avg"`
	Pctl  MetricsPercentile `json:"percentiles"`
}

// MetricsOutcomesSnapshot captures terminal outcome counters and ratios.
type MetricsOutcomesSnapshot struct {
	SuccessTotal  int64   `json:"success_total"`
	FailedTotal   int64   `json:"failed_total"`
	CanceledTotal int64   `json:"canceled_total"`
	SuccessRate   float64 `json:"success_rate"`
	FailedRate    float64 `json:"failed_rate"`
	CanceledRate  float64 `json:"canceled_rate"`
}

// MetricsSeveritySnapshot captures severity totals and per-completed ratios.
type MetricsSeveritySnapshot struct {
	Totals            map[string]int64   `json:"totals"`
	PerCompletedRates map[string]float64 `json:"per_completed_rates"`
}

// MetricsLocaleSnapshot captures requested result locales.
type MetricsLocaleSnapshot struct {
	Counts map[string]int64 `json:"counts"`
}

type apiRouteMetrics struct {
	Route             string
	Method            string
	RequestsTotal     int64
	StatusClassCounts map[string]int64
	Latency           boundedHistogram
}

type boundedHistogram struct {
	BoundsMs []int64
	Counts   []int64
}

// MetricsCollector stores low-overhead in-memory counters and gauges.
type MetricsCollector struct {
	mu sync.Mutex

	startedAt         time.Time
	workerCount       int
	activeWorkers     int
	maxConcurrentJobs int
	nowFn             func() time.Time

	queuePaused bool
	queueDepth  int64
	inFlight    int64
	dnsQueries        int64
	dnsQueries4       int64
	dnsQueries6       int64
	dnsCacheHits      int64
	dnsCacheMisses    int64
	dnsCacheEvictions int64

	submittedTotal int64
	startedTotal   int64
	completedTotal int64
	succeededTotal int64
	failedTotal    int64
	canceledTotal  int64
	statusCounts   map[string]int64

	apiRequestsTotal     int64
	apiStatusClassCounts map[string]int64
	apiErrorCodeCounts   map[string]int64
	apiRoutes            map[string]*apiRouteMetrics

	jobDuration        boundedHistogram
	jobDurationCount   int64
	jobDurationTotalMs int64
	severityTotals     map[string]int64
	localeCounts       map[string]int64

	defaultBatchLimit  int
	defaultDomainLimit int
	maxBatchLimit      int
	maxDomainLimit     int
	batchInsightCap    int
	domainInsightCap   int
	batchInsights      map[string]*metricsBatchInsight
	domainInsights     map[string]*metricsDomainInsight
	batchOther         metricsBatchInsight
	domainOther        metricsDomainInsight

	trend1m trendRing
	trend5m trendRing
}

// NewMetricsCollector creates a metrics collector initialized from server config.
func NewMetricsCollector(cfg Config) *MetricsCollector {
	return newMetricsCollector(cfg, time.Now().UTC())
}

func newMetricsCollector(cfg Config, startedAt time.Time) *MetricsCollector {
	activeWorkers := cfg.WorkerCount
	if activeWorkers < 1 {
		activeWorkers = 1
	}
	return &MetricsCollector{
		startedAt:            startedAt.UTC(),
		workerCount:          cfg.WorkerCount,
		activeWorkers:        activeWorkers,
		maxConcurrentJobs:    cfg.MaxConcurrentJobs,
		nowFn:                func() time.Time { return time.Now().UTC() },
		statusCounts:         zeroStatusCounts(),
		apiStatusClassCounts: zeroStatusClassCounts(),
		apiErrorCodeCounts:   map[string]int64{},
		apiRoutes:            map[string]*apiRouteMetrics{},
		jobDuration:          newBoundedHistogram(metricsJobDurationBucketsMs[:]),
		severityTotals:       zeroMetricsSeverityTotals(),
		localeCounts:         map[string]int64{},
		defaultBatchLimit:    metricsDefaultBatchLimit,
		defaultDomainLimit:   metricsDefaultDomainLimit,
		maxBatchLimit:        metricsMaxBatchLimit,
		maxDomainLimit:       metricsMaxDomainLimit,
		batchInsightCap:      metricsBatchInsightCap,
		domainInsightCap:     metricsDomainInsightCap,
		batchInsights:        map[string]*metricsBatchInsight{},
		domainInsights:       map[string]*metricsDomainInsight{},
		batchOther: metricsBatchInsight{
			BatchID:        metricsLocaleOtherKey,
			SeverityTotals: zeroMetricsSeverityTotals(),
		},
		domainOther: metricsDomainInsight{
			Domain:         metricsLocaleOtherKey,
			SeverityTotals: zeroMetricsSeverityTotals(),
		},
		trend1m: newTrendRing(time.Minute, 6*time.Hour),
		trend5m: newTrendRing(5*time.Minute, 48*time.Hour),
	}
}

// Snapshot returns a metrics snapshot using default insight limits.
func (m *MetricsCollector) Snapshot() MetricsSnapshot {
	now := time.Now().UTC()
	if m.nowFn != nil {
		now = m.nowFn().UTC()
	}
	return m.snapshotAtWithLimits(now, m.defaultDomainLimit, m.defaultBatchLimit)
}

// SnapshotWithLimits returns a metrics snapshot with explicit insight limits.
func (m *MetricsCollector) SnapshotWithLimits(domainLimit int, batchLimit int) MetricsSnapshot {
	now := time.Now().UTC()
	if m.nowFn != nil {
		now = m.nowFn().UTC()
	}
	return m.snapshotAtWithLimits(now, domainLimit, batchLimit)
}

// ObserveQueuePaused records whether queue processing is paused.
func (m *MetricsCollector) ObserveQueuePaused(paused bool) {
	m.mu.Lock()
	m.queuePaused = paused
	m.mu.Unlock()
}

// ObserveDNSQueries records DNS query counters split by IP family.
func (m *MetricsCollector) ObserveDNSQueries(ipv4Queries int64, ipv6Queries int64) {
	if ipv4Queries < 0 {
		ipv4Queries = 0
	}
	if ipv6Queries < 0 {
		ipv6Queries = 0
	}
	total := ipv4Queries + ipv6Queries
	if total == 0 {
		return
	}

	m.mu.Lock()
	m.dnsQueries += total
	m.dnsQueries4 += ipv4Queries
	m.dnsQueries6 += ipv6Queries
	now := time.Now().UTC()
	if m.nowFn != nil {
		now = m.nowFn().UTC()
	}
	m.observeTrendDNSQueriesLocked(now, ipv4Queries, ipv6Queries)
	m.mu.Unlock()
}

// ObserveCacheMetrics records DNS resolver cache hit/miss/eviction counters.
func (m *MetricsCollector) ObserveCacheMetrics(hits int64, misses int64, evictions int64) {
	if hits < 0 {
		hits = 0
	}
	if misses < 0 {
		misses = 0
	}
	if evictions < 0 {
		evictions = 0
	}
	if hits == 0 && misses == 0 && evictions == 0 {
		return
	}

	m.mu.Lock()
	m.dnsCacheHits += hits
	m.dnsCacheMisses += misses
	m.dnsCacheEvictions += evictions
	m.mu.Unlock()
}

// ObserveJobSubmitted records a submitted job without batch/domain context.
func (m *MetricsCollector) ObserveJobSubmitted(initialStatus JobStatus) {
	m.ObserveJobSubmittedWithContext("", "", initialStatus)
}

// ObserveJobSubmittedWithContext records a submitted job with batch/domain context.
func (m *MetricsCollector) ObserveJobSubmittedWithContext(batchID string, domain string, initialStatus JobStatus) {
	m.mu.Lock()
	m.submittedTotal++
	m.observeStatusTransitionLocked("", initialStatus)
	now := time.Now().UTC()
	if m.nowFn != nil {
		now = m.nowFn().UTC()
	}
	m.observeBatchSubmissionLocked(now, batchID)
	m.mu.Unlock()
}

// ObserveJobStatusTransition records a job state transition.
func (m *MetricsCollector) ObserveJobStatusTransition(fromStatus, toStatus JobStatus) {
	m.mu.Lock()
	m.observeStatusTransitionLocked(fromStatus, toStatus)
	m.mu.Unlock()
}

// ObserveAPIRequest records one API request observation.
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
			Latency:           newBoundedHistogram(metricsLatencyBucketsMs[:]),
		}
		m.apiRoutes[key] = routeMetrics
	}
	routeMetrics.RequestsTotal++
	routeMetrics.StatusClassCounts[statusClass]++
	routeMetrics.Latency.Observe(duration)
	now := time.Now().UTC()
	if m.nowFn != nil {
		now = m.nowFn().UTC()
	}
	m.observeTrendAPIRequestLocked(now, key, duration)
	m.mu.Unlock()
}

// ObserveJobCompletion records completion data for a terminal job.
func (m *MetricsCollector) ObserveJobCompletion(status JobStatus, duration time.Duration, severityTotals map[string]int64) {
	m.ObserveJobCompletionWithContext("", "", status, duration, severityTotals)
}

// ObserveJobCompletionWithContext records completion data with batch/domain context.
func (m *MetricsCollector) ObserveJobCompletionWithContext(batchID string, domain string, status JobStatus, duration time.Duration, severityTotals map[string]int64) {
	if !isTerminalMetricsStatus(status) {
		return
	}

	m.mu.Lock()
	if duration >= 0 {
		m.jobDuration.Observe(duration)
		m.jobDurationCount++
		durationMs := int64(math.Round(float64(duration) / float64(time.Millisecond)))
		if durationMs < 0 {
			durationMs = 0
		}
		m.jobDurationTotalMs += durationMs
	}
	for _, level := range metricsSeverityLevels {
		m.severityTotals[level] += severityTotals[level]
	}
	now := time.Now().UTC()
	if m.nowFn != nil {
		now = m.nowFn().UTC()
	}
	m.observeTrendSeverityLocked(now, severityTotals)
	m.observeBatchCompletionLocked(now, batchID, status, severityTotals)
	m.observeDomainCompletionLocked(now, domain, status, duration, severityTotals)
	m.mu.Unlock()
}

// ObserveResultLocale records locale usage when rendering results.
func (m *MetricsCollector) ObserveResultLocale(locale string) {
	normalized := normalizeMetricsLocale(locale)

	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.localeCounts[normalized]; exists {
		m.localeCounts[normalized]++
		return
	}
	if len(m.localeCounts) < metricsMaxLocaleBuckets {
		m.localeCounts[normalized] = 1
		return
	}
	m.localeCounts[metricsLocaleOtherKey]++
}

func (m *MetricsCollector) observeStatusTransitionLocked(fromStatus, toStatus JobStatus) {
	if fromStatus == toStatus {
		return
	}
	now := time.Now().UTC()
	if m.nowFn != nil {
		now = m.nowFn().UTC()
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
		m.observeTrendQueueDepthLocked(now, m.queueDepth)
	}
	if fromStatus == JobQueued && toStatus != JobQueued && m.queueDepth > 0 {
		m.queueDepth--
		m.observeTrendQueueDepthLocked(now, m.queueDepth)
	}

	if !isTerminalMetricsStatus(fromStatus) && isTerminalMetricsStatus(toStatus) {
		m.completedTotal++
		if toStatus == JobSucceeded {
			m.succeededTotal++
		}
		if toStatus == JobFailed {
			m.failedTotal++
		}
		if toStatus == JobCanceled {
			m.canceledTotal++
		}
		m.observeTrendOutcomeLocked(now, toStatus)
	}
}

func (m *MetricsCollector) snapshotAt(now time.Time) MetricsSnapshot {
	return m.snapshotAtWithLimits(now, m.defaultDomainLimit, m.defaultBatchLimit)
}

func (m *MetricsCollector) snapshotAtWithLimits(now time.Time, domainLimit int, batchLimit int) MetricsSnapshot {
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
	dnsQueries := m.dnsQueries
	dnsQueries4 := m.dnsQueries4
	dnsQueries6 := m.dnsQueries6
	submittedTotal := m.submittedTotal
	startedTotal := m.startedTotal
	completedTotal := m.completedTotal
	succeededTotal := m.succeededTotal
	failedTotal := m.failedTotal
	canceledTotal := m.canceledTotal
	apiRequestsTotal := m.apiRequestsTotal
	apiStatusClassCounts := copyStatusClassCounts(m.apiStatusClassCounts)
	apiErrorCodeCounts := copyStringCounts(m.apiErrorCodeCounts)
	apiRoutes := m.copyAPIRouteMetricsLocked()
	jobDurationCount := m.jobDurationCount
	jobDurationTotalMs := m.jobDurationTotalMs
	jobDurationPercentiles := MetricsPercentile{
		P50: m.jobDuration.Quantile(0.50),
		P90: m.jobDuration.Quantile(0.90),
		P99: m.jobDuration.Quantile(0.99),
	}
	severityTotals := copyStringCounts(m.severityTotals)
	localeCounts := copyStringCounts(m.localeCounts)
	insights := m.snapshotInsightsLocked(domainLimit, batchLimit)
	trends := m.snapshotTrendsLocked(now)
	m.mu.Unlock()

	completedForRates := succeededTotal + failedTotal + canceledTotal
	avgDurationMs := 0.0
	if jobDurationCount > 0 {
		avgDurationMs = float64(jobDurationTotalMs) / float64(jobDurationCount)
	}
	successRate := safeRate(succeededTotal, completedForRates)
	failedRate := safeRate(failedTotal, completedForRates)
	canceledRate := safeRate(canceledTotal, completedForRates)
	perCompletedRates := map[string]float64{}
	for _, level := range metricsSeverityLevels {
		perCompletedRates[level] = safeRate(severityTotals[level], completedTotal)
	}

	return MetricsSnapshot{
		SchemaVersion: metricsSchemaVersion,
		ServerVersion: engine.VersionFull(),
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
			DNSQueriesTotal:   dnsQueries,
			DNSQueriesIPv4:    dnsQueries4,
			DNSQueriesIPv6:    dnsQueries6,
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
		Quality: MetricsQualitySnapshot{
			JobDurationMs: MetricsDurationSnapshot{
				Count: jobDurationCount,
				Avg:   avgDurationMs,
				Pctl:  jobDurationPercentiles,
			},
			Outcomes: MetricsOutcomesSnapshot{
				SuccessTotal:  succeededTotal,
				FailedTotal:   failedTotal,
				CanceledTotal: canceledTotal,
				SuccessRate:   successRate,
				FailedRate:    failedRate,
				CanceledRate:  canceledRate,
			},
			Severity: MetricsSeveritySnapshot{
				Totals:            severityTotals,
				PerCompletedRates: perCompletedRates,
			},
			LocaleUsage: MetricsLocaleSnapshot{
				Counts: localeCounts,
			},
		},
		Insights: insights,
		Trends:   trends,
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

func zeroMetricsSeverityTotals() map[string]int64 {
	totals := make(map[string]int64, len(metricsSeverityLevels))
	for _, level := range metricsSeverityLevels {
		totals[level] = 0
	}
	return totals
}

func normalizeMetricsLocale(locale string) string {
	locale = strings.TrimSpace(locale)
	if locale == "" {
		return "en"
	}
	locale = strings.ReplaceAll(locale, "-", "_")
	locale = strings.ToLower(locale)
	if idx := strings.IndexAny(locale, ".@"); idx >= 0 {
		locale = locale[:idx]
	}
	if locale == "" {
		return "en"
	}
	return locale
}

func safeRate(numerator int64, denominator int64) float64 {
	if denominator <= 0 {
		return 0
	}
	return float64(numerator) / float64(denominator)
}

func newBoundedHistogram(bounds []int64) boundedHistogram {
	copyBounds := make([]int64, len(bounds))
	copy(copyBounds, bounds)
	return boundedHistogram{
		BoundsMs: copyBounds,
		Counts:   make([]int64, len(copyBounds)+1),
	}
}

// Observe adds one duration sample to the histogram.
func (h *boundedHistogram) Observe(duration time.Duration) {
	if h == nil {
		return
	}
	ms := float64(duration) / float64(time.Millisecond)
	if ms < 0 {
		ms = 0
	}
	for idx, bucketUpperBound := range h.BoundsMs {
		if ms <= float64(bucketUpperBound) {
			h.Counts[idx]++
			return
		}
	}
	h.Counts[len(h.Counts)-1]++
}

// Quantile returns an approximate quantile in milliseconds from bucketed samples.
func (h boundedHistogram) Quantile(quantile float64) int64 {
	if len(h.BoundsMs) == 0 {
		return 0
	}
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
			if idx < len(h.BoundsMs) {
				return h.BoundsMs[idx]
			}
			return h.BoundsMs[len(h.BoundsMs)-1]
		}
	}
	return h.BoundsMs[len(h.BoundsMs)-1]
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
