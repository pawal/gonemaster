package server

import (
	"fmt"
	"testing"
	"time"
)

var (
	benchmarkMetricsPayload  []byte
	benchmarkMetricsSnapshot MetricsSnapshot
)

func BenchmarkMetricsCollectorSnapshotWithLimits(b *testing.B) {
	collector := benchmarkMetricsCollectorFixture()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchmarkMetricsSnapshot = collector.SnapshotWithLimits(20, 20)
	}
}

func BenchmarkMetricsBuildResponseBodyJSONHealthJobsQuality(b *testing.B) {
	srv := benchmarkMetricsServerFixture()
	options := metricsQueryOptions{
		format:       metricsFormatJSON,
		includeAll:   false,
		include:      map[string]bool{"health": true, "jobs": true, "quality": true},
		limitDomains: 20,
		limitBatches: 20,
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		payload, err := srv.buildMetricsResponseBody(options)
		if err != nil {
			b.Fatalf("build metrics response: %v", err)
		}
		benchmarkMetricsPayload = payload
	}
}

func benchmarkMetricsServerFixture() *Server {
	return &Server{
		metrics:      benchmarkMetricsCollectorFixture(),
		metricsCache: map[string]metricsCacheEntry{},
	}
}

func benchmarkMetricsCollectorFixture() *MetricsCollector {
	cfg := DefaultConfig()
	startedAt := time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC)
	now := startedAt
	collector := newMetricsCollector(cfg, startedAt)
	collector.nowFn = func() time.Time { return now }

	locales := []string{"en", "sv", "fr", "ja"}
	routes := []struct {
		method string
		route  string
		status int
		err    string
	}{
		{method: "GET", route: "/api/v1/jobs", status: 200},
		{method: "GET", route: "/api/v1/metrics", status: 200},
		{method: "POST", route: "/api/v1/jobs/batch", status: 202},
		{method: "GET", route: "/pub/api/v1/jobs/{id}", status: 200},
		{method: "POST", route: "/pub/api/v1/jobs", status: 429, err: "rate_limited"},
	}

	for i := 0; i < 512; i++ {
		batchID := fmt.Sprintf("batch-%03d", i%48)
		domain := fmt.Sprintf("domain-%04d.example", i)

		collector.ObserveJobSubmittedWithContext(batchID, domain, JobQueued)
		collector.ObserveJobStatusTransition(JobQueued, JobRunning)

		status := JobSucceeded
		duration := time.Duration(500+(i%60)*25) * time.Millisecond
		severityTotals := zeroMetricsSeverityTotals()
		switch {
		case i%23 == 0:
			status = JobFailed
			severityTotals["ERROR"] = 2
			severityTotals["CRITICAL"] = 1
		case i%17 == 0:
			status = JobCanceled
		case i%5 == 0:
			severityTotals["WARNING"] = 1
		default:
			severityTotals["NOTICE"] = 1
		}

		collector.ObserveJobStatusTransition(JobRunning, status)
		collector.ObserveJobCompletionWithContext(batchID, domain, status, duration, severityTotals)
		collector.ObserveResultLocale(locales[i%len(locales)])

		route := routes[i%len(routes)]
		collector.ObserveAPIRequest(
			route.route,
			route.method,
			route.status,
			time.Duration(10+(i%40))*time.Millisecond,
			route.err,
		)

		collector.ObserveDNSQueries(int64(20+(i%7)), int64(12+(i%5)))
		collector.ObserveCacheMetrics(int64(40+(i%11)), int64(8+(i%3)), int64(i%2))

		now = now.Add(15 * time.Second)
	}

	return collector
}
