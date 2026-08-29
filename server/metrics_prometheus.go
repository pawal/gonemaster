package server

import (
	"bytes"
	"maps"
	"slices"
	"sort"
	"strconv"
	"strings"

	"codeberg.org/pawal/gonemaster/engine"
)

const prometheusMetricsContentType = "text/plain; version=0.0.4; charset=utf-8"

type metricsPromSnapshot struct {
	ServerVersion     string
	StartedAtUnix     int64
	WorkerCount       int
	ActiveWorkers     int
	MaxConcurrentJobs int
	QueuePaused       bool
	QueueDepth        int64
	InFlightJobs      int64

	DNSQueriesIPv4    int64
	DNSQueriesIPv6    int64
	DNSCacheHits      int64
	DNSCacheMisses    int64
	DNSCacheEvictions int64

	SubmittedTotal int64
	StartedTotal   int64
	CompletedTotal int64
	SucceededTotal int64
	FailedTotal    int64
	CanceledTotal  int64
	ExpiredTotal   int64
	PurgedTotal    int64
	StatusCounts   map[string]int64

	APIRequestsTotal     int64
	APIStatusClassCounts map[string]int64
	APIErrorCodeCounts   map[string]int64
	APIRoutes            []metricsPromAPIRoute

	ForwardedHeadersStrippedTotal int64
	RateLimitKeys                 int

	JobDurationHistogram boundedHistogram
	JobDurationCount     int64
	JobDurationTotalMs   int64

	SeverityTotals map[string]int64
	LocaleCounts   map[string]int64
}

type metricsPromAPIRoute struct {
	Route             string
	Method            string
	RequestsTotal     int64
	StatusClassCounts map[string]int64
	LatencyHistogram  boundedHistogram
	LatencyTotalMs    int64
}

func (m *MetricsCollector) prometheusSnapshot() metricsPromSnapshot {
	if m == nil {
		return metricsPromSnapshot{ServerVersion: engine.VersionFull()}
	}

	rateLimitKeys := m.rateLimitKeys()

	m.mu.Lock()
	defer m.mu.Unlock()

	expiredTotal := max(m.completedTotal-m.succeededTotal-m.failedTotal-m.canceledTotal, 0)

	return metricsPromSnapshot{
		ServerVersion:        engine.VersionFull(),
		StartedAtUnix:        m.startedAt.Unix(),
		WorkerCount:          m.workerCount,
		ActiveWorkers:        m.activeWorkers,
		MaxConcurrentJobs:    m.maxConcurrentJobs,
		QueuePaused:          m.queuePaused,
		QueueDepth:           m.queueDepth,
		InFlightJobs:         m.inFlight,
		DNSQueriesIPv4:       m.dnsQueries4,
		DNSQueriesIPv6:       m.dnsQueries6,
		DNSCacheHits:         m.dnsCacheHits,
		DNSCacheMisses:       m.dnsCacheMisses,
		DNSCacheEvictions:    m.dnsCacheEvictions,
		SubmittedTotal:       m.submittedTotal,
		StartedTotal:         m.startedTotal,
		CompletedTotal:       m.completedTotal,
		SucceededTotal:       m.succeededTotal,
		FailedTotal:          m.failedTotal,
		CanceledTotal:        m.canceledTotal,
		ExpiredTotal:         expiredTotal,
		PurgedTotal:          m.purgedTotal,
		StatusCounts:         copyStatusCounts(m.statusCounts),
		APIRequestsTotal:     m.apiRequestsTotal,
		APIStatusClassCounts: copyStatusClassCounts(m.apiStatusClassCounts),
		APIErrorCodeCounts:   copyStringCounts(m.apiErrorCodeCounts),
		APIRoutes:            m.copyPromAPIRouteMetricsLocked(),

		ForwardedHeadersStrippedTotal: m.forwardedStrippedTotal,
		RateLimitKeys:                 rateLimitKeys,

		JobDurationHistogram: cloneBoundedHistogram(m.jobDuration),
		JobDurationCount:     m.jobDurationCount,
		JobDurationTotalMs:   m.jobDurationTotalMs,
		SeverityTotals:       copyStringCounts(m.severityTotals),
		LocaleCounts:         copyStringCounts(m.localeCounts),
	}
}

func (m *MetricsCollector) copyPromAPIRouteMetricsLocked() []metricsPromAPIRoute {
	routes := make([]metricsPromAPIRoute, 0, len(m.apiRoutes))
	for _, route := range m.apiRoutes {
		if route == nil {
			continue
		}
		routes = append(routes, metricsPromAPIRoute{
			Route:             route.Route,
			Method:            route.Method,
			RequestsTotal:     route.RequestsTotal,
			StatusClassCounts: copyStatusClassCounts(route.StatusClassCounts),
			LatencyHistogram:  cloneBoundedHistogram(route.Latency),
			LatencyTotalMs:    route.LatencyTotalMs,
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

func cloneBoundedHistogram(in boundedHistogram) boundedHistogram {
	bounds := make([]int64, len(in.BoundsMs))
	copy(bounds, in.BoundsMs)
	counts := make([]int64, len(in.Counts))
	copy(counts, in.Counts)
	return boundedHistogram{
		BoundsMs: bounds,
		Counts:   counts,
	}
}

func (s *Server) buildMetricsPrometheusResponse() ([]byte, error) {
	return renderPrometheusMetrics(s.metrics.prometheusSnapshot()), nil
}

func renderPrometheusMetrics(snapshot metricsPromSnapshot) []byte {
	var buf bytes.Buffer

	writePromHeader(&buf, "gonemaster_build_info", "Build information for the gonemaster server.", "gauge")
	writePromSample(&buf, "gonemaster_build_info", map[string]string{"version": snapshot.ServerVersion}, 1)

	writePromHeader(&buf, "gonemaster_start_time_seconds", "Unix time when the gonemaster server process started.", "gauge")
	writePromSample(&buf, "gonemaster_start_time_seconds", nil, snapshot.StartedAtUnix)

	writePromHeader(&buf, "gonemaster_worker_count", "Configured worker goroutine count.", "gauge")
	writePromSample(&buf, "gonemaster_worker_count", nil, snapshot.WorkerCount)

	writePromHeader(&buf, "gonemaster_active_workers", "Workers currently available to process jobs.", "gauge")
	writePromSample(&buf, "gonemaster_active_workers", nil, snapshot.ActiveWorkers)

	writePromHeader(&buf, "gonemaster_max_concurrent_jobs", "Configured maximum concurrent engine runs.", "gauge")
	writePromSample(&buf, "gonemaster_max_concurrent_jobs", nil, snapshot.MaxConcurrentJobs)

	writePromHeader(&buf, "gonemaster_queue_paused", "Whether queue processing is paused (1 paused, 0 running).", "gauge")
	writePromSample(&buf, "gonemaster_queue_paused", nil, promBool(snapshot.QueuePaused))

	writePromHeader(&buf, "gonemaster_queue_depth", "Current number of queued jobs.", "gauge")
	writePromSample(&buf, "gonemaster_queue_depth", nil, snapshot.QueueDepth)

	writePromHeader(&buf, "gonemaster_in_flight_jobs", "Current number of running jobs.", "gauge")
	writePromSample(&buf, "gonemaster_in_flight_jobs", nil, snapshot.InFlightJobs)

	writePromHeader(&buf, "gonemaster_dns_external_queries_total", "Lifetime external DNS queries by IP family.", "counter")
	writePromSample(&buf, "gonemaster_dns_external_queries_total", map[string]string{"family": "ipv4"}, snapshot.DNSQueriesIPv4)
	writePromSample(&buf, "gonemaster_dns_external_queries_total", map[string]string{"family": "ipv6"}, snapshot.DNSQueriesIPv6)

	writePromHeader(&buf, "gonemaster_dns_cache_lookups_total", "Lifetime resolver cache lookups by result.", "counter")
	writePromSample(&buf, "gonemaster_dns_cache_lookups_total", map[string]string{"result": "hit"}, snapshot.DNSCacheHits)
	writePromSample(&buf, "gonemaster_dns_cache_lookups_total", map[string]string{"result": "miss"}, snapshot.DNSCacheMisses)

	writePromHeader(&buf, "gonemaster_dns_cache_evictions_total", "Lifetime resolver cache evictions.", "counter")
	writePromSample(&buf, "gonemaster_dns_cache_evictions_total", nil, snapshot.DNSCacheEvictions)

	writePromHeader(&buf, "gonemaster_jobs_submitted_total", "Lifetime submitted jobs.", "counter")
	writePromSample(&buf, "gonemaster_jobs_submitted_total", nil, snapshot.SubmittedTotal)

	writePromHeader(&buf, "gonemaster_jobs_started_total", "Lifetime jobs that entered running state.", "counter")
	writePromSample(&buf, "gonemaster_jobs_started_total", nil, snapshot.StartedTotal)

	writePromHeader(&buf, "gonemaster_jobs_completed_total", "Lifetime terminal job transitions.", "counter")
	writePromSample(&buf, "gonemaster_jobs_completed_total", nil, snapshot.CompletedTotal)

	writePromHeader(&buf, "gonemaster_jobs_purged_total", "Lifetime jobs deleted by the retention purge loop.", "counter")
	writePromSample(&buf, "gonemaster_jobs_purged_total", nil, snapshot.PurgedTotal)

	writePromHeader(&buf, "gonemaster_job_terminal_outcomes_total", "Lifetime terminal job outcomes by outcome.", "counter")
	writePromSample(&buf, "gonemaster_job_terminal_outcomes_total", map[string]string{"outcome": "succeeded"}, snapshot.SucceededTotal)
	writePromSample(&buf, "gonemaster_job_terminal_outcomes_total", map[string]string{"outcome": "failed"}, snapshot.FailedTotal)
	writePromSample(&buf, "gonemaster_job_terminal_outcomes_total", map[string]string{"outcome": "canceled"}, snapshot.CanceledTotal)
	writePromSample(&buf, "gonemaster_job_terminal_outcomes_total", map[string]string{"outcome": "expired"}, snapshot.ExpiredTotal)

	writePromHeader(&buf, "gonemaster_jobs_status", "Current stored jobs by status.", "gauge")
	for _, status := range metricsJobStatuses {
		writePromSample(&buf, "gonemaster_jobs_status", map[string]string{"status": string(status)}, snapshot.StatusCounts[string(status)])
	}

	writePromHeader(&buf, "gonemaster_api_requests_total", "Lifetime API requests handled by the server.", "counter")
	writePromSample(&buf, "gonemaster_api_requests_total", nil, snapshot.APIRequestsTotal)

	writePromHeader(&buf, "gonemaster_api_requests_by_status_class_total", "Lifetime API requests by HTTP status class.", "counter")
	for _, statusClass := range metricsStatusClasses {
		writePromSample(&buf, "gonemaster_api_requests_by_status_class_total", map[string]string{"status_class": statusClass}, snapshot.APIStatusClassCounts[statusClass])
	}

	writePromHeader(&buf, "gonemaster_forwarded_headers_stripped_total", "Lifetime requests whose X-Forwarded-* headers were dropped as untrusted.", "counter")
	writePromSample(&buf, "gonemaster_forwarded_headers_stripped_total", nil, snapshot.ForwardedHeadersStrippedTotal)

	writePromHeader(&buf, "gonemaster_rate_limit_keys", "Distinct client IPs currently tracked by the public rate limiter.", "gauge")
	writePromSample(&buf, "gonemaster_rate_limit_keys", nil, snapshot.RateLimitKeys)

	if len(snapshot.APIErrorCodeCounts) > 0 {
		writePromHeader(&buf, "gonemaster_api_errors_total", "Lifetime structured API errors by error code.", "counter")
		errorCodes := sortedStringKeys(snapshot.APIErrorCodeCounts)
		for _, code := range errorCodes {
			writePromSample(&buf, "gonemaster_api_errors_total", map[string]string{"code": code}, snapshot.APIErrorCodeCounts[code])
		}
	}

	if len(snapshot.APIRoutes) > 0 {
		writePromHeader(&buf, "gonemaster_api_route_requests_total", "Lifetime API requests by route and method.", "counter")
		for _, route := range snapshot.APIRoutes {
			labels := map[string]string{"route": route.Route, "method": route.Method}
			writePromSample(&buf, "gonemaster_api_route_requests_total", labels, route.RequestsTotal)
		}

		writePromHeader(&buf, "gonemaster_api_route_requests_by_status_class_total", "Lifetime API requests by route, method, and HTTP status class.", "counter")
		for _, route := range snapshot.APIRoutes {
			for _, statusClass := range metricsStatusClasses {
				labels := map[string]string{
					"route":        route.Route,
					"method":       route.Method,
					"status_class": statusClass,
				}
				writePromSample(&buf, "gonemaster_api_route_requests_by_status_class_total", labels, route.StatusClassCounts[statusClass])
			}
		}

		writePromHistogramHeader(&buf, "gonemaster_api_request_duration_seconds", "API request latency by route and method.")
		for _, route := range snapshot.APIRoutes {
			labels := map[string]string{"route": route.Route, "method": route.Method}
			writePromHistogramSamples(&buf, "gonemaster_api_request_duration_seconds", labels, route.LatencyHistogram, route.RequestsTotal, millisecondsToSeconds(route.LatencyTotalMs))
		}
	}

	writePromHistogramHeader(&buf, "gonemaster_job_duration_seconds", "Terminal job duration histogram.")
	writePromHistogramSamples(&buf, "gonemaster_job_duration_seconds", nil, snapshot.JobDurationHistogram, snapshot.JobDurationCount, millisecondsToSeconds(snapshot.JobDurationTotalMs))

	writePromHeader(&buf, "gonemaster_job_severity_total", "Lifetime severity totals produced by completed jobs.", "counter")
	for _, level := range metricsSeverityLevels {
		writePromSample(&buf, "gonemaster_job_severity_total", map[string]string{"severity": level}, snapshot.SeverityTotals[level])
	}

	if len(snapshot.LocaleCounts) > 0 {
		writePromHeader(&buf, "gonemaster_result_locale_requests_total", "Lifetime localized result requests by locale.", "counter")
		locales := sortedStringKeys(snapshot.LocaleCounts)
		for _, locale := range locales {
			writePromSample(&buf, "gonemaster_result_locale_requests_total", map[string]string{"locale": locale}, snapshot.LocaleCounts[locale])
		}
	}

	return buf.Bytes()
}

func writePromHeader(buf *bytes.Buffer, name string, help string, metricType string) {
	buf.WriteString("# HELP ")
	buf.WriteString(name)
	buf.WriteByte(' ')
	buf.WriteString(help)
	buf.WriteByte('\n')
	buf.WriteString("# TYPE ")
	buf.WriteString(name)
	buf.WriteByte(' ')
	buf.WriteString(metricType)
	buf.WriteByte('\n')
}

func writePromHistogramHeader(buf *bytes.Buffer, name string, help string) {
	writePromHeader(buf, name, help, "histogram")
}

func writePromHistogramSamples(buf *bytes.Buffer, name string, labels map[string]string, histogram boundedHistogram, count int64, sumSeconds float64) {
	if count < 0 {
		count = 0
	}
	cumulative := int64(0)
	for idx, upperBoundMs := range histogram.BoundsMs {
		if idx < len(histogram.Counts) {
			cumulative += histogram.Counts[idx]
		}
		bucketLabels := cloneLabels(labels)
		bucketLabels["le"] = millisecondsLabel(upperBoundMs)
		writePromSample(buf, name+"_bucket", bucketLabels, cumulative)
	}
	if len(histogram.Counts) > len(histogram.BoundsMs) {
		cumulative += histogram.Counts[len(histogram.Counts)-1]
	}
	bucketLabels := cloneLabels(labels)
	bucketLabels["le"] = "+Inf"
	writePromSample(buf, name+"_bucket", bucketLabels, cumulative)
	writePromSample(buf, name+"_sum", labels, formatPromFloat(sumSeconds))
	writePromSample(buf, name+"_count", labels, count)
}

func writePromSample(buf *bytes.Buffer, name string, labels map[string]string, value any) {
	buf.WriteString(name)
	if len(labels) > 0 {
		keys := sortedStringKeysFromMap(labels)
		buf.WriteByte('{')
		for idx, key := range keys {
			if idx > 0 {
				buf.WriteByte(',')
			}
			buf.WriteString(key)
			buf.WriteString("=\"")
			buf.WriteString(escapePromLabelValue(labels[key]))
			buf.WriteByte('"')
		}
		buf.WriteByte('}')
	}
	buf.WriteByte(' ')
	switch typed := value.(type) {
	case string:
		buf.WriteString(typed)
	case int:
		buf.WriteString(strconv.Itoa(typed))
	case int64:
		buf.WriteString(strconv.FormatInt(typed, 10))
	case float64:
		buf.WriteString(formatPromFloat(typed))
	default:
		buf.WriteString("0")
	}
	buf.WriteByte('\n')
}

func sortedStringKeys(values map[string]int64) []string {
	keys := slices.Sorted(maps.Keys(values))
	return keys
}

func sortedStringKeysFromMap(values map[string]string) []string {
	keys := slices.Sorted(maps.Keys(values))
	return keys
}

func cloneLabels(labels map[string]string) map[string]string {
	if len(labels) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(labels))
	maps.Copy(out, labels)
	return out
}

func escapePromLabelValue(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, "\n", `\n`)
	value = strings.ReplaceAll(value, `"`, `\"`)
	return value
}

func promBool(value bool) int64 {
	if value {
		return 1
	}
	return 0
}

func millisecondsToSeconds(value int64) float64 {
	return float64(value) / 1000.0
}

func millisecondsLabel(value int64) string {
	return formatPromFloat(millisecondsToSeconds(value))
}

func formatPromFloat(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}
