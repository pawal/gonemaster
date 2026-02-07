package server

import (
	"math"
	"strconv"
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
	if snapshot.Health.ActiveWorkers != 7 {
		t.Fatalf("health.active_workers = %d, want 7", snapshot.Health.ActiveWorkers)
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
	if snapshot.Health.DNSQueriesTotal != 0 {
		t.Fatalf("health.dns_queries_total = %d, want 0", snapshot.Health.DNSQueriesTotal)
	}
	if snapshot.Health.DNSQueriesIPv4 != 0 {
		t.Fatalf("health.dns_queries_ipv4_total = %d, want 0", snapshot.Health.DNSQueriesIPv4)
	}
	if snapshot.Health.DNSQueriesIPv6 != 0 {
		t.Fatalf("health.dns_queries_ipv6_total = %d, want 0", snapshot.Health.DNSQueriesIPv6)
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

	if snapshot.API.RequestsTotal != 0 {
		t.Fatalf("api.requests_total = %d, want 0", snapshot.API.RequestsTotal)
	}
	if len(snapshot.API.Routes) != 0 {
		t.Fatalf("api.routes size = %d, want 0", len(snapshot.API.Routes))
	}
	if len(snapshot.API.StatusClassCounts) != len(metricsStatusClasses) {
		t.Fatalf("api.status_class_counts size = %d, want %d", len(snapshot.API.StatusClassCounts), len(metricsStatusClasses))
	}
	for _, statusClass := range metricsStatusClasses {
		if count := snapshot.API.StatusClassCounts[statusClass]; count != 0 {
			t.Fatalf("api.status_class_counts[%q] = %d, want 0", statusClass, count)
		}
	}
	if len(snapshot.API.ErrorCodeCounts) != 0 {
		t.Fatalf("api.error_code_counts size = %d, want 0", len(snapshot.API.ErrorCodeCounts))
	}

	if snapshot.Quality.JobDurationMs.Count != 0 {
		t.Fatalf("quality.job_duration_ms.count = %d, want 0", snapshot.Quality.JobDurationMs.Count)
	}
	if snapshot.Quality.JobDurationMs.Avg != 0 {
		t.Fatalf("quality.job_duration_ms.avg = %f, want 0", snapshot.Quality.JobDurationMs.Avg)
	}
	if snapshot.Quality.Outcomes.SuccessTotal != 0 || snapshot.Quality.Outcomes.FailedTotal != 0 || snapshot.Quality.Outcomes.CanceledTotal != 0 {
		t.Fatalf("quality.outcomes totals = %+v, want all zero", snapshot.Quality.Outcomes)
	}
	if len(snapshot.Quality.LocaleUsage.Counts) != 0 {
		t.Fatalf("quality.locale_usage.counts size = %d, want 0", len(snapshot.Quality.LocaleUsage.Counts))
	}
	for _, level := range metricsSeverityLevels {
		if got := snapshot.Quality.Severity.Totals[level]; got != 0 {
			t.Fatalf("quality.severity.totals[%q] = %d, want 0", level, got)
		}
		if got := snapshot.Quality.Severity.PerCompletedRates[level]; got != 0 {
			t.Fatalf("quality.severity.per_completed_rates[%q] = %f, want 0", level, got)
		}
	}
	if len(snapshot.Trends.Windows) != len(metricsTrendWindowOrder) {
		t.Fatalf("trends.windows size = %d, want %d", len(snapshot.Trends.Windows), len(metricsTrendWindowOrder))
	}
	if got := len(snapshot.Trends.Windows["1h"].Points); got != 60 {
		t.Fatalf("trends.windows[1h].points size = %d, want 60", got)
	}
	if got := len(snapshot.Trends.Windows["6h"].Points); got != 360 {
		t.Fatalf("trends.windows[6h].points size = %d, want 360", got)
	}
	if got := len(snapshot.Trends.Windows["24h"].Points); got != 288 {
		t.Fatalf("trends.windows[24h].points size = %d, want 288", got)
	}
	if got := len(snapshot.Trends.Windows["48h"].Points); got != 576 {
		t.Fatalf("trends.windows[48h].points size = %d, want 576", got)
	}
	if snapshot.Insights.Batches.Limit != metricsDefaultBatchLimit {
		t.Fatalf("insights.batches.limit = %d, want %d", snapshot.Insights.Batches.Limit, metricsDefaultBatchLimit)
	}
	if snapshot.Insights.Batches.Cap != metricsBatchInsightCap {
		t.Fatalf("insights.batches.cap = %d, want %d", snapshot.Insights.Batches.Cap, metricsBatchInsightCap)
	}
	if len(snapshot.Insights.Batches.Items) != 0 {
		t.Fatalf("insights.batches.items size = %d, want 0", len(snapshot.Insights.Batches.Items))
	}
	if snapshot.Insights.Batches.Other != nil {
		t.Fatal("insights.batches.other expected nil")
	}
	if snapshot.Insights.Domains.Limit != metricsDefaultDomainLimit {
		t.Fatalf("insights.domains.limit = %d, want %d", snapshot.Insights.Domains.Limit, metricsDefaultDomainLimit)
	}
	if snapshot.Insights.Domains.Cap != metricsDomainInsightCap {
		t.Fatalf("insights.domains.cap = %d, want %d", snapshot.Insights.Domains.Cap, metricsDomainInsightCap)
	}
	if len(snapshot.Insights.Domains.Items) != 0 {
		t.Fatalf("insights.domains.items size = %d, want 0", len(snapshot.Insights.Domains.Items))
	}
	if snapshot.Insights.Domains.Other != nil {
		t.Fatal("insights.domains.other expected nil")
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

func TestMetricsCollectorTracksLifecycle(t *testing.T) {
	cfg := DefaultConfig()
	cfg.WorkerCount = 0
	startedAt := time.Date(2026, 2, 7, 12, 0, 0, 0, time.UTC)
	collector := newMetricsCollector(cfg, startedAt)

	collector.ObserveQueuePaused(true)
	collector.ObserveJobSubmitted(JobQueued)
	collector.ObserveJobSubmitted(JobQueued)
	collector.ObserveJobStatusTransition(JobQueued, JobRunning)
	collector.ObserveJobStatusTransition(JobRunning, JobSucceeded)
	collector.ObserveJobStatusTransition(JobQueued, JobCanceled)
	collector.ObserveQueuePaused(false)

	snapshot := collector.snapshotAt(startedAt.Add(2 * time.Minute))

	if snapshot.Health.WorkerCount != 0 {
		t.Fatalf("health.worker_count = %d, want 0", snapshot.Health.WorkerCount)
	}
	if snapshot.Health.ActiveWorkers != 1 {
		t.Fatalf("health.active_workers = %d, want 1", snapshot.Health.ActiveWorkers)
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

	if snapshot.Jobs.SubmittedTotal != 2 {
		t.Fatalf("jobs.submitted_total = %d, want 2", snapshot.Jobs.SubmittedTotal)
	}
	if snapshot.Jobs.StartedTotal != 1 {
		t.Fatalf("jobs.started_total = %d, want 1", snapshot.Jobs.StartedTotal)
	}
	if snapshot.Jobs.CompletedTotal != 2 {
		t.Fatalf("jobs.completed_total = %d, want 2", snapshot.Jobs.CompletedTotal)
	}
	if snapshot.Jobs.CanceledTotal != 1 {
		t.Fatalf("jobs.canceled_total = %d, want 1", snapshot.Jobs.CanceledTotal)
	}

	if got := snapshot.Jobs.StatusCounts[string(JobQueued)]; got != 0 {
		t.Fatalf("jobs.status_counts[%q] = %d, want 0", JobQueued, got)
	}
	if got := snapshot.Jobs.StatusCounts[string(JobRunning)]; got != 0 {
		t.Fatalf("jobs.status_counts[%q] = %d, want 0", JobRunning, got)
	}
	if got := snapshot.Jobs.StatusCounts[string(JobSucceeded)]; got != 1 {
		t.Fatalf("jobs.status_counts[%q] = %d, want 1", JobSucceeded, got)
	}
	if got := snapshot.Jobs.StatusCounts[string(JobCanceled)]; got != 1 {
		t.Fatalf("jobs.status_counts[%q] = %d, want 1", JobCanceled, got)
	}
}

func TestMetricsCollectorTracksDNSQueries(t *testing.T) {
	cfg := DefaultConfig()
	startedAt := time.Date(2026, 2, 7, 12, 0, 0, 0, time.UTC)
	now := startedAt
	collector := newMetricsCollector(cfg, startedAt)
	collector.nowFn = func() time.Time { return now }

	collector.ObserveDNSQueries(7, 3)
	now = now.Add(30 * time.Second)
	collector.ObserveDNSQueries(2, 8)

	snapshot := collector.snapshotAt(startedAt.Add(time.Minute))
	if snapshot.Health.DNSQueriesTotal != 20 {
		t.Fatalf("health.dns_queries_total = %d, want 20", snapshot.Health.DNSQueriesTotal)
	}
	if snapshot.Health.DNSQueriesIPv4 != 9 {
		t.Fatalf("health.dns_queries_ipv4_total = %d, want 9", snapshot.Health.DNSQueriesIPv4)
	}
	if snapshot.Health.DNSQueriesIPv6 != 11 {
		t.Fatalf("health.dns_queries_ipv6_total = %d, want 11", snapshot.Health.DNSQueriesIPv6)
	}
	maxRate := 0.0
	maxRateIPv4 := 0.0
	maxRateIPv6 := 0.0
	for _, point := range snapshot.Trends.Windows["1h"].Points {
		if point.DNSQueriesPerSecond > maxRate {
			maxRate = point.DNSQueriesPerSecond
		}
		if point.DNSQueriesIPv4PerSecond > maxRateIPv4 {
			maxRateIPv4 = point.DNSQueriesIPv4PerSecond
		}
		if point.DNSQueriesIPv6PerSecond > maxRateIPv6 {
			maxRateIPv6 = point.DNSQueriesIPv6PerSecond
		}
	}
	if math.Abs(maxRate-(20.0/60.0)) > 0.0001 {
		t.Fatalf("trends.windows[1h].max.dns_queries_per_second = %f, want %f", maxRate, 20.0/60.0)
	}
	if math.Abs(maxRateIPv4-(9.0/60.0)) > 0.0001 {
		t.Fatalf("trends.windows[1h].max.dns_queries_ipv4_per_second = %f, want %f", maxRateIPv4, 9.0/60.0)
	}
	if math.Abs(maxRateIPv6-(11.0/60.0)) > 0.0001 {
		t.Fatalf("trends.windows[1h].max.dns_queries_ipv6_per_second = %f, want %f", maxRateIPv6, 11.0/60.0)
	}
}

func TestMetricsCollectorTracksAPIRequests(t *testing.T) {
	cfg := DefaultConfig()
	startedAt := time.Date(2026, 2, 7, 12, 0, 0, 0, time.UTC)
	collector := newMetricsCollector(cfg, startedAt)

	collector.ObserveAPIRequest("/api/v1/jobs", "GET", 200, 8*time.Millisecond, "")
	collector.ObserveAPIRequest("/api/v1/jobs", "GET", 201, 120*time.Millisecond, "")
	collector.ObserveAPIRequest("/api/v1/jobs", "GET", 503, 900*time.Millisecond, "queue_error")
	collector.ObserveAPIRequest("/api/v1/jobs/{job_id}/cancel", "POST", 404, 20*time.Millisecond, "not_found")

	snapshot := collector.snapshotAt(startedAt.Add(time.Minute))

	if snapshot.API.RequestsTotal != 4 {
		t.Fatalf("api.requests_total = %d, want 4", snapshot.API.RequestsTotal)
	}
	if snapshot.API.StatusClassCounts["2xx"] != 2 {
		t.Fatalf("api.status_class_counts[2xx] = %d, want 2", snapshot.API.StatusClassCounts["2xx"])
	}
	if snapshot.API.StatusClassCounts["4xx"] != 1 {
		t.Fatalf("api.status_class_counts[4xx] = %d, want 1", snapshot.API.StatusClassCounts["4xx"])
	}
	if snapshot.API.StatusClassCounts["5xx"] != 1 {
		t.Fatalf("api.status_class_counts[5xx] = %d, want 1", snapshot.API.StatusClassCounts["5xx"])
	}
	if snapshot.API.ErrorCodeCounts["queue_error"] != 1 {
		t.Fatalf("api.error_code_counts[queue_error] = %d, want 1", snapshot.API.ErrorCodeCounts["queue_error"])
	}
	if snapshot.API.ErrorCodeCounts["not_found"] != 1 {
		t.Fatalf("api.error_code_counts[not_found] = %d, want 1", snapshot.API.ErrorCodeCounts["not_found"])
	}

	if len(snapshot.API.Routes) != 2 {
		t.Fatalf("api.routes size = %d, want 2", len(snapshot.API.Routes))
	}
	first := snapshot.API.Routes[0]
	if first.Route != "/api/v1/jobs" || first.Method != "GET" {
		t.Fatalf("first route = %s %s, want GET /api/v1/jobs", first.Method, first.Route)
	}
	if first.RequestsTotal != 3 {
		t.Fatalf("first.requests_total = %d, want 3", first.RequestsTotal)
	}
	if first.LatencyMs.P50 != 250 || first.LatencyMs.P90 != 1000 || first.LatencyMs.P99 != 1000 {
		t.Fatalf("first.latency_ms = %+v, want p50=250 p90=1000 p99=1000", first.LatencyMs)
	}
	second := snapshot.API.Routes[1]
	if second.Route != "/api/v1/jobs/{job_id}/cancel" || second.Method != "POST" {
		t.Fatalf("second route = %s %s, want POST /api/v1/jobs/{job_id}/cancel", second.Method, second.Route)
	}
	if second.LatencyMs.P50 != 25 || second.LatencyMs.P90 != 25 || second.LatencyMs.P99 != 25 {
		t.Fatalf("second.latency_ms = %+v, want p50=25 p90=25 p99=25", second.LatencyMs)
	}
}

func TestMetricsCollectorTracksQualityMetrics(t *testing.T) {
	cfg := DefaultConfig()
	startedAt := time.Date(2026, 2, 7, 12, 0, 0, 0, time.UTC)
	collector := newMetricsCollector(cfg, startedAt)

	collector.ObserveJobSubmitted(JobQueued)
	collector.ObserveJobStatusTransition(JobQueued, JobRunning)
	collector.ObserveJobStatusTransition(JobRunning, JobSucceeded)
	collector.ObserveJobCompletion(JobSucceeded, 1200*time.Millisecond, map[string]int64{
		"NOTICE": 2,
		"ERROR":  1,
	})

	collector.ObserveJobSubmitted(JobQueued)
	collector.ObserveJobStatusTransition(JobQueued, JobRunning)
	collector.ObserveJobStatusTransition(JobRunning, JobFailed)
	collector.ObserveJobCompletion(JobFailed, 3200*time.Millisecond, map[string]int64{
		"WARNING":  3,
		"CRITICAL": 1,
	})

	collector.ObserveJobSubmitted(JobQueued)
	collector.ObserveJobStatusTransition(JobQueued, JobCanceled)
	collector.ObserveResultLocale("en")
	collector.ObserveResultLocale("SV-SE")
	collector.ObserveResultLocale("sv_se")

	snapshot := collector.snapshotAt(startedAt.Add(2 * time.Minute))

	if snapshot.Quality.JobDurationMs.Count != 2 {
		t.Fatalf("quality.job_duration_ms.count = %d, want 2", snapshot.Quality.JobDurationMs.Count)
	}
	if snapshot.Quality.JobDurationMs.Pctl.P50 != 2500 || snapshot.Quality.JobDurationMs.Pctl.P90 != 5000 || snapshot.Quality.JobDurationMs.Pctl.P99 != 5000 {
		t.Fatalf("quality.job_duration_ms.percentiles = %+v, want p50=2500 p90=5000 p99=5000", snapshot.Quality.JobDurationMs.Pctl)
	}
	if math.Abs(snapshot.Quality.JobDurationMs.Avg-2200) > 0.001 {
		t.Fatalf("quality.job_duration_ms.avg = %f, want 2200", snapshot.Quality.JobDurationMs.Avg)
	}

	if snapshot.Quality.Outcomes.SuccessTotal != 1 || snapshot.Quality.Outcomes.FailedTotal != 1 || snapshot.Quality.Outcomes.CanceledTotal != 1 {
		t.Fatalf("quality.outcomes totals = %+v, want success=1 failed=1 canceled=1", snapshot.Quality.Outcomes)
	}
	if math.Abs(snapshot.Quality.Outcomes.SuccessRate-(1.0/3.0)) > 0.0001 {
		t.Fatalf("quality.outcomes.success_rate = %f, want %f", snapshot.Quality.Outcomes.SuccessRate, 1.0/3.0)
	}
	if math.Abs(snapshot.Quality.Outcomes.FailedRate-(1.0/3.0)) > 0.0001 {
		t.Fatalf("quality.outcomes.failed_rate = %f, want %f", snapshot.Quality.Outcomes.FailedRate, 1.0/3.0)
	}
	if math.Abs(snapshot.Quality.Outcomes.CanceledRate-(1.0/3.0)) > 0.0001 {
		t.Fatalf("quality.outcomes.canceled_rate = %f, want %f", snapshot.Quality.Outcomes.CanceledRate, 1.0/3.0)
	}

	if snapshot.Quality.Severity.Totals["NOTICE"] != 2 || snapshot.Quality.Severity.Totals["WARNING"] != 3 || snapshot.Quality.Severity.Totals["ERROR"] != 1 || snapshot.Quality.Severity.Totals["CRITICAL"] != 1 {
		t.Fatalf("quality.severity.totals = %+v", snapshot.Quality.Severity.Totals)
	}
	if math.Abs(snapshot.Quality.Severity.PerCompletedRates["NOTICE"]-(2.0/3.0)) > 0.0001 {
		t.Fatalf("quality.severity.per_completed_rates[NOTICE] = %f, want %f", snapshot.Quality.Severity.PerCompletedRates["NOTICE"], 2.0/3.0)
	}
	if math.Abs(snapshot.Quality.Severity.PerCompletedRates["WARNING"]-1.0) > 0.0001 {
		t.Fatalf("quality.severity.per_completed_rates[WARNING] = %f, want 1", snapshot.Quality.Severity.PerCompletedRates["WARNING"])
	}
	if math.Abs(snapshot.Quality.Severity.PerCompletedRates["ERROR"]-(1.0/3.0)) > 0.0001 {
		t.Fatalf("quality.severity.per_completed_rates[ERROR] = %f, want %f", snapshot.Quality.Severity.PerCompletedRates["ERROR"], 1.0/3.0)
	}
	if math.Abs(snapshot.Quality.Severity.PerCompletedRates["CRITICAL"]-(1.0/3.0)) > 0.0001 {
		t.Fatalf("quality.severity.per_completed_rates[CRITICAL] = %f, want %f", snapshot.Quality.Severity.PerCompletedRates["CRITICAL"], 1.0/3.0)
	}

	if snapshot.Quality.LocaleUsage.Counts["en"] != 1 {
		t.Fatalf("quality.locale_usage.counts[en] = %d, want 1", snapshot.Quality.LocaleUsage.Counts["en"])
	}
	if snapshot.Quality.LocaleUsage.Counts["sv_se"] != 2 {
		t.Fatalf("quality.locale_usage.counts[sv_se] = %d, want 2", snapshot.Quality.LocaleUsage.Counts["sv_se"])
	}
}

func TestMetricsCollectorLocaleUsageIsBounded(t *testing.T) {
	cfg := DefaultConfig()
	collector := newMetricsCollector(cfg, time.Now().UTC())

	for i := 0; i < metricsMaxLocaleBuckets+5; i++ {
		collector.ObserveResultLocale("loc_" + strconv.Itoa(i))
	}

	snapshot := collector.Snapshot()
	if len(snapshot.Quality.LocaleUsage.Counts) != metricsMaxLocaleBuckets+1 {
		t.Fatalf("quality.locale_usage.counts size = %d, want %d", len(snapshot.Quality.LocaleUsage.Counts), metricsMaxLocaleBuckets+1)
	}
	if snapshot.Quality.LocaleUsage.Counts[metricsLocaleOtherKey] != 5 {
		t.Fatalf("quality.locale_usage.counts[_other] = %d, want 5", snapshot.Quality.LocaleUsage.Counts[metricsLocaleOtherKey])
	}
}

func TestMetricsCollectorTracksBatchAndDomainInsights(t *testing.T) {
	cfg := DefaultConfig()
	startedAt := time.Date(2026, 2, 7, 12, 0, 0, 0, time.UTC)
	collector := newMetricsCollector(cfg, startedAt)
	now := startedAt
	collector.nowFn = func() time.Time { return now }

	collector.ObserveJobSubmittedWithContext("batch-a", "alpha.example", JobQueued)
	collector.ObserveJobStatusTransition(JobQueued, JobRunning)
	now = now.Add(time.Minute)
	collector.ObserveJobStatusTransition(JobRunning, JobSucceeded)
	collector.ObserveJobCompletionWithContext("batch-a", "alpha.example", JobSucceeded, 1200*time.Millisecond, map[string]int64{
		"NOTICE": 1,
	})

	now = now.Add(time.Minute)
	collector.ObserveJobSubmittedWithContext("batch-a", "beta.example", JobQueued)
	collector.ObserveJobStatusTransition(JobQueued, JobRunning)
	now = now.Add(time.Minute)
	collector.ObserveJobStatusTransition(JobRunning, JobFailed)
	collector.ObserveJobCompletionWithContext("batch-a", "beta.example", JobFailed, 2200*time.Millisecond, map[string]int64{
		"ERROR": 2,
	})

	now = now.Add(time.Minute)
	collector.ObserveJobSubmittedWithContext("batch-b", "alpha.example", JobQueued)
	collector.ObserveJobStatusTransition(JobQueued, JobCanceled)
	collector.ObserveJobCompletionWithContext("batch-b", "alpha.example", JobCanceled, -1, zeroMetricsSeverityTotals())

	snapshot := collector.Snapshot()
	if len(snapshot.Insights.Batches.Items) != 2 {
		t.Fatalf("insights.batches.items size = %d, want 2", len(snapshot.Insights.Batches.Items))
	}
	firstBatch := snapshot.Insights.Batches.Items[0]
	if firstBatch.BatchID != "batch-a" {
		t.Fatalf("first batch id = %q, want batch-a", firstBatch.BatchID)
	}
	if firstBatch.SizeTotal != 2 || firstBatch.ProcessedTotal != 2 {
		t.Fatalf("batch-a size/processed = %d/%d, want 2/2", firstBatch.SizeTotal, firstBatch.ProcessedTotal)
	}
	if firstBatch.Outcomes[string(JobSucceeded)] != 1 || firstBatch.Outcomes[string(JobFailed)] != 1 {
		t.Fatalf("batch-a outcomes = %+v", firstBatch.Outcomes)
	}
	if firstBatch.SeverityTotals["NOTICE"] != 1 || firstBatch.SeverityTotals["ERROR"] != 2 {
		t.Fatalf("batch-a severity totals = %+v", firstBatch.SeverityTotals)
	}
	secondBatch := snapshot.Insights.Batches.Items[1]
	if secondBatch.BatchID != "batch-b" {
		t.Fatalf("second batch id = %q, want batch-b", secondBatch.BatchID)
	}
	if secondBatch.SizeTotal != 1 || secondBatch.ProcessedTotal != 1 || secondBatch.Outcomes[string(JobCanceled)] != 1 {
		t.Fatalf("batch-b aggregate = %+v", secondBatch)
	}

	if len(snapshot.Insights.Domains.Items) != 2 {
		t.Fatalf("insights.domains.items size = %d, want 2", len(snapshot.Insights.Domains.Items))
	}
	firstDomain := snapshot.Insights.Domains.Items[0]
	if firstDomain.Domain != "alpha.example" {
		t.Fatalf("first domain = %q, want alpha.example", firstDomain.Domain)
	}
	if firstDomain.RunsTotal != 2 {
		t.Fatalf("alpha.example runs_total = %d, want 2", firstDomain.RunsTotal)
	}
	if firstDomain.LastStatus != JobCanceled {
		t.Fatalf("alpha.example last_status = %s, want canceled", firstDomain.LastStatus)
	}
	if math.Abs(firstDomain.AvgDurationMs-1200) > 0.001 {
		t.Fatalf("alpha.example avg_duration_ms = %f, want 1200", firstDomain.AvgDurationMs)
	}
	if firstDomain.SeverityTotals["NOTICE"] != 1 {
		t.Fatalf("alpha.example severity totals = %+v", firstDomain.SeverityTotals)
	}
	secondDomain := snapshot.Insights.Domains.Items[1]
	if secondDomain.Domain != "beta.example" {
		t.Fatalf("second domain = %q, want beta.example", secondDomain.Domain)
	}
	if secondDomain.RunsTotal != 1 || secondDomain.LastStatus != JobFailed {
		t.Fatalf("beta.example aggregate = %+v", secondDomain)
	}
}

func TestMetricsCollectorInsightsEvictionAndCaps(t *testing.T) {
	cfg := DefaultConfig()
	startedAt := time.Date(2026, 2, 7, 12, 0, 0, 0, time.UTC)
	collector := newMetricsCollector(cfg, startedAt)
	now := startedAt
	collector.nowFn = func() time.Time { return now }
	collector.batchInsightCap = 2
	collector.domainInsightCap = 2
	collector.maxBatchLimit = 2
	collector.maxDomainLimit = 2

	inputs := []struct {
		batch  string
		domain string
	}{
		{batch: "batch-a", domain: "a.example"},
		{batch: "batch-b", domain: "b.example"},
		{batch: "batch-c", domain: "c.example"},
	}
	for _, input := range inputs {
		collector.ObserveJobSubmittedWithContext(input.batch, input.domain, JobQueued)
		collector.ObserveJobStatusTransition(JobQueued, JobSucceeded)
		collector.ObserveJobCompletionWithContext(input.batch, input.domain, JobSucceeded, 800*time.Millisecond, map[string]int64{
			"WARNING": 1,
		})
		now = now.Add(time.Minute)
	}

	snapshot := collector.SnapshotWithLimits(50, 50)
	if snapshot.Insights.Batches.Limit != 2 {
		t.Fatalf("insights.batches.limit = %d, want 2", snapshot.Insights.Batches.Limit)
	}
	if snapshot.Insights.Domains.Limit != 2 {
		t.Fatalf("insights.domains.limit = %d, want 2", snapshot.Insights.Domains.Limit)
	}
	if len(snapshot.Insights.Batches.Items) != 2 {
		t.Fatalf("insights.batches.items size = %d, want 2", len(snapshot.Insights.Batches.Items))
	}
	if len(snapshot.Insights.Domains.Items) != 2 {
		t.Fatalf("insights.domains.items size = %d, want 2", len(snapshot.Insights.Domains.Items))
	}
	if snapshot.Insights.Batches.Other == nil {
		t.Fatal("insights.batches.other expected non-nil")
	}
	if snapshot.Insights.Batches.Other.SizeTotal < 1 || snapshot.Insights.Batches.Other.ProcessedTotal < 1 {
		t.Fatalf("insights.batches.other = %+v, expected contribution from evicted entries", snapshot.Insights.Batches.Other)
	}
	if snapshot.Insights.Domains.Other == nil {
		t.Fatal("insights.domains.other expected non-nil")
	}
	if snapshot.Insights.Domains.Other.RunsTotal < 1 {
		t.Fatalf("insights.domains.other = %+v, expected contribution from evicted entries", snapshot.Insights.Domains.Other)
	}
}
