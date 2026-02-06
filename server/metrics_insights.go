package server

import (
	"sort"
	"strings"
	"time"
)

var metricsBatchOutcomeStatuses = [...]JobStatus{
	JobSucceeded,
	JobFailed,
	JobCanceled,
	JobExpired,
}

type MetricsInsightsSnapshot struct {
	Batches MetricsBatchInsightsSnapshot  `json:"batches"`
	Domains MetricsDomainInsightsSnapshot `json:"domains"`
}

type MetricsBatchInsightsSnapshot struct {
	Limit int                   `json:"limit"`
	Cap   int                   `json:"cap"`
	Items []MetricsBatchInsight `json:"items"`
	Other *MetricsBatchInsight  `json:"other,omitempty"`
}

type MetricsBatchInsight struct {
	BatchID        string           `json:"batch_id"`
	SizeTotal      int64            `json:"size_total"`
	ProcessedTotal int64            `json:"processed_total"`
	Outcomes       map[string]int64 `json:"outcomes"`
	SeverityTotals map[string]int64 `json:"severity_totals"`
}

type MetricsDomainInsightsSnapshot struct {
	Limit int                    `json:"limit"`
	Cap   int                    `json:"cap"`
	Items []MetricsDomainInsight `json:"items"`
	Other *MetricsDomainInsight  `json:"other,omitempty"`
}

type MetricsDomainInsight struct {
	Domain         string           `json:"domain"`
	RunsTotal      int64            `json:"runs_total"`
	LastStatus     JobStatus        `json:"last_status"`
	AvgDurationMs  float64          `json:"avg_duration_ms"`
	SeverityTotals map[string]int64 `json:"severity_totals"`
}

type metricsBatchInsight struct {
	BatchID        string
	SizeTotal      int64
	ProcessedTotal int64
	Outcomes       map[string]int64
	SeverityTotals map[string]int64
	LastUpdated    time.Time
}

type metricsDomainInsight struct {
	Domain          string
	RunsTotal       int64
	LastStatus      JobStatus
	DurationCount   int64
	DurationTotalMs int64
	SeverityTotals  map[string]int64
	LastUpdated     time.Time
}

func (m *MetricsCollector) observeBatchSubmissionLocked(now time.Time, batchID string) {
	batchID = strings.TrimSpace(batchID)
	if batchID == "" {
		return
	}
	entry := m.getOrCreateBatchInsightLocked(batchID, now)
	entry.SizeTotal++
	entry.LastUpdated = now
}

func (m *MetricsCollector) observeBatchCompletionLocked(now time.Time, batchID string, status JobStatus, severityTotals map[string]int64) {
	batchID = strings.TrimSpace(batchID)
	if batchID == "" || !isTerminalMetricsStatus(status) {
		return
	}
	entry := m.getOrCreateBatchInsightLocked(batchID, now)
	entry.ProcessedTotal++
	entry.Outcomes[string(status)]++
	mergeMetricsSeverityTotals(entry.SeverityTotals, severityTotals)
	entry.LastUpdated = now
}

func (m *MetricsCollector) observeDomainCompletionLocked(now time.Time, domain string, status JobStatus, duration time.Duration, severityTotals map[string]int64) {
	domain = normalizeMetricsDomain(domain)
	if domain == "" || !isTerminalMetricsStatus(status) {
		return
	}
	entry := m.getOrCreateDomainInsightLocked(domain, now)
	entry.RunsTotal++
	entry.LastStatus = status
	if duration >= 0 {
		durationMs := int64(duration / time.Millisecond)
		if durationMs < 0 {
			durationMs = 0
		}
		entry.DurationCount++
		entry.DurationTotalMs += durationMs
	}
	mergeMetricsSeverityTotals(entry.SeverityTotals, severityTotals)
	entry.LastUpdated = now
}

func (m *MetricsCollector) getOrCreateBatchInsightLocked(batchID string, now time.Time) *metricsBatchInsight {
	if entry, ok := m.batchInsights[batchID]; ok {
		return entry
	}
	if m.batchInsightCap <= 0 {
		mergeBatchInsight(&m.batchOther, newBatchInsight(batchID, now))
		return &m.batchOther
	}
	if len(m.batchInsights) >= m.batchInsightCap {
		m.evictOldestBatchInsightLocked()
	}
	entry := newBatchInsight(batchID, now)
	m.batchInsights[batchID] = entry
	return entry
}

func (m *MetricsCollector) getOrCreateDomainInsightLocked(domain string, now time.Time) *metricsDomainInsight {
	if entry, ok := m.domainInsights[domain]; ok {
		return entry
	}
	if m.domainInsightCap <= 0 {
		mergeDomainInsight(&m.domainOther, newDomainInsight(domain, now))
		return &m.domainOther
	}
	if len(m.domainInsights) >= m.domainInsightCap {
		m.evictOldestDomainInsightLocked()
	}
	entry := newDomainInsight(domain, now)
	m.domainInsights[domain] = entry
	return entry
}

func (m *MetricsCollector) evictOldestBatchInsightLocked() {
	var oldestKey string
	var oldest *metricsBatchInsight
	for key, entry := range m.batchInsights {
		if entry == nil {
			continue
		}
		if oldest == nil || entry.LastUpdated.Before(oldest.LastUpdated) {
			oldestKey = key
			oldest = entry
		}
	}
	if oldest == nil {
		return
	}
	mergeBatchInsight(&m.batchOther, oldest)
	delete(m.batchInsights, oldestKey)
}

func (m *MetricsCollector) evictOldestDomainInsightLocked() {
	var oldestKey string
	var oldest *metricsDomainInsight
	for key, entry := range m.domainInsights {
		if entry == nil {
			continue
		}
		if oldest == nil || entry.LastUpdated.Before(oldest.LastUpdated) {
			oldestKey = key
			oldest = entry
		}
	}
	if oldest == nil {
		return
	}
	mergeDomainInsight(&m.domainOther, oldest)
	delete(m.domainInsights, oldestKey)
}

func (m *MetricsCollector) snapshotInsightsLocked(domainLimit int, batchLimit int) MetricsInsightsSnapshot {
	batchLimit = clampMetricsLimit(batchLimit, m.defaultBatchLimit, m.maxBatchLimit)
	domainLimit = clampMetricsLimit(domainLimit, m.defaultDomainLimit, m.maxDomainLimit)

	batchItems := make([]MetricsBatchInsight, 0, len(m.batchInsights))
	for _, entry := range m.batchInsights {
		if entry == nil {
			continue
		}
		batchItems = append(batchItems, toMetricsBatchInsight(entry))
	}
	sort.Slice(batchItems, func(i, j int) bool {
		if batchItems[i].ProcessedTotal != batchItems[j].ProcessedTotal {
			return batchItems[i].ProcessedTotal > batchItems[j].ProcessedTotal
		}
		if batchItems[i].SizeTotal != batchItems[j].SizeTotal {
			return batchItems[i].SizeTotal > batchItems[j].SizeTotal
		}
		return batchItems[i].BatchID < batchItems[j].BatchID
	})
	if len(batchItems) > batchLimit {
		batchItems = batchItems[:batchLimit]
	}

	domainItems := make([]MetricsDomainInsight, 0, len(m.domainInsights))
	for _, entry := range m.domainInsights {
		if entry == nil {
			continue
		}
		domainItems = append(domainItems, toMetricsDomainInsight(entry))
	}
	sort.Slice(domainItems, func(i, j int) bool {
		if domainItems[i].RunsTotal != domainItems[j].RunsTotal {
			return domainItems[i].RunsTotal > domainItems[j].RunsTotal
		}
		if domainItems[i].AvgDurationMs != domainItems[j].AvgDurationMs {
			return domainItems[i].AvgDurationMs > domainItems[j].AvgDurationMs
		}
		return domainItems[i].Domain < domainItems[j].Domain
	})
	if len(domainItems) > domainLimit {
		domainItems = domainItems[:domainLimit]
	}

	snapshot := MetricsInsightsSnapshot{
		Batches: MetricsBatchInsightsSnapshot{
			Limit: batchLimit,
			Cap:   m.batchInsightCap,
			Items: batchItems,
		},
		Domains: MetricsDomainInsightsSnapshot{
			Limit: domainLimit,
			Cap:   m.domainInsightCap,
			Items: domainItems,
		},
	}
	if hasBatchInsightData(&m.batchOther) {
		other := toMetricsBatchInsight(&m.batchOther)
		snapshot.Batches.Other = &other
	}
	if hasDomainInsightData(&m.domainOther) {
		other := toMetricsDomainInsight(&m.domainOther)
		snapshot.Domains.Other = &other
	}
	return snapshot
}

func newBatchInsight(batchID string, now time.Time) *metricsBatchInsight {
	return &metricsBatchInsight{
		BatchID:        batchID,
		Outcomes:       zeroBatchOutcomeCounts(),
		SeverityTotals: zeroMetricsSeverityTotals(),
		LastUpdated:    now,
	}
}

func newDomainInsight(domain string, now time.Time) *metricsDomainInsight {
	return &metricsDomainInsight{
		Domain:         domain,
		SeverityTotals: zeroMetricsSeverityTotals(),
		LastUpdated:    now,
	}
}

func mergeBatchInsight(dst *metricsBatchInsight, src *metricsBatchInsight) {
	if dst == nil || src == nil {
		return
	}
	if dst.Outcomes == nil {
		dst.Outcomes = zeroBatchOutcomeCounts()
	}
	if dst.SeverityTotals == nil {
		dst.SeverityTotals = zeroMetricsSeverityTotals()
	}
	dst.SizeTotal += src.SizeTotal
	dst.ProcessedTotal += src.ProcessedTotal
	for _, status := range metricsBatchOutcomeStatuses {
		dst.Outcomes[string(status)] += src.Outcomes[string(status)]
	}
	mergeMetricsSeverityTotals(dst.SeverityTotals, src.SeverityTotals)
}

func mergeDomainInsight(dst *metricsDomainInsight, src *metricsDomainInsight) {
	if dst == nil || src == nil {
		return
	}
	if dst.SeverityTotals == nil {
		dst.SeverityTotals = zeroMetricsSeverityTotals()
	}
	dst.RunsTotal += src.RunsTotal
	dst.DurationCount += src.DurationCount
	dst.DurationTotalMs += src.DurationTotalMs
	dst.LastStatus = src.LastStatus
	mergeMetricsSeverityTotals(dst.SeverityTotals, src.SeverityTotals)
}

func toMetricsBatchInsight(in *metricsBatchInsight) MetricsBatchInsight {
	if in == nil {
		return MetricsBatchInsight{
			Outcomes:       zeroBatchOutcomeCounts(),
			SeverityTotals: zeroMetricsSeverityTotals(),
		}
	}
	return MetricsBatchInsight{
		BatchID:        in.BatchID,
		SizeTotal:      in.SizeTotal,
		ProcessedTotal: in.ProcessedTotal,
		Outcomes:       copyBatchOutcomeCounts(in.Outcomes),
		SeverityTotals: copyStringCounts(in.SeverityTotals),
	}
}

func toMetricsDomainInsight(in *metricsDomainInsight) MetricsDomainInsight {
	if in == nil {
		return MetricsDomainInsight{
			SeverityTotals: zeroMetricsSeverityTotals(),
		}
	}
	avgDuration := 0.0
	if in.DurationCount > 0 {
		avgDuration = float64(in.DurationTotalMs) / float64(in.DurationCount)
	}
	return MetricsDomainInsight{
		Domain:         in.Domain,
		RunsTotal:      in.RunsTotal,
		LastStatus:     in.LastStatus,
		AvgDurationMs:  avgDuration,
		SeverityTotals: copyStringCounts(in.SeverityTotals),
	}
}

func hasBatchInsightData(in *metricsBatchInsight) bool {
	if in == nil {
		return false
	}
	if in.SizeTotal > 0 || in.ProcessedTotal > 0 {
		return true
	}
	for _, status := range metricsBatchOutcomeStatuses {
		if in.Outcomes[string(status)] > 0 {
			return true
		}
	}
	for _, level := range metricsSeverityLevels {
		if in.SeverityTotals[level] > 0 {
			return true
		}
	}
	return false
}

func hasDomainInsightData(in *metricsDomainInsight) bool {
	if in == nil {
		return false
	}
	if in.RunsTotal > 0 || in.DurationCount > 0 || in.DurationTotalMs > 0 {
		return true
	}
	for _, level := range metricsSeverityLevels {
		if in.SeverityTotals[level] > 0 {
			return true
		}
	}
	return false
}

func mergeMetricsSeverityTotals(dst map[string]int64, src map[string]int64) {
	if dst == nil {
		return
	}
	for _, level := range metricsSeverityLevels {
		dst[level] += src[level]
	}
}

func normalizeMetricsDomain(domain string) string {
	domain = strings.TrimSpace(domain)
	return strings.ToLower(domain)
}

func zeroBatchOutcomeCounts() map[string]int64 {
	out := make(map[string]int64, len(metricsBatchOutcomeStatuses))
	for _, status := range metricsBatchOutcomeStatuses {
		out[string(status)] = 0
	}
	return out
}

func copyBatchOutcomeCounts(in map[string]int64) map[string]int64 {
	out := zeroBatchOutcomeCounts()
	for _, status := range metricsBatchOutcomeStatuses {
		out[string(status)] = in[string(status)]
	}
	return out
}

func clampMetricsLimit(value int, fallback int, hardCap int) int {
	if hardCap < 1 {
		hardCap = 1
	}
	if fallback < 1 {
		fallback = 1
	}
	if fallback > hardCap {
		fallback = hardCap
	}
	if value <= 0 {
		return fallback
	}
	if value > hardCap {
		return hardCap
	}
	return value
}
