package server

import (
	"strings"
	"time"
)

var metricsTrendWindowOrder = [...]string{
	"1h",
	"6h",
	"24h",
	"48h",
}

type metricsTrendWindowSpec struct {
	Window     time.Duration
	Resolution time.Duration
}

var metricsTrendWindowSpecs = map[string]metricsTrendWindowSpec{
	"1h": {
		Window:     time.Hour,
		Resolution: time.Minute,
	},
	"6h": {
		Window:     6 * time.Hour,
		Resolution: time.Minute,
	},
	"24h": {
		Window:     24 * time.Hour,
		Resolution: 5 * time.Minute,
	},
	"48h": {
		Window:     48 * time.Hour,
		Resolution: 5 * time.Minute,
	},
}

type MetricsTrendsSnapshot struct {
	Windows map[string]MetricsTrendWindowSnapshot `json:"windows"`
}

type MetricsTrendWindowSnapshot struct {
	ResolutionSeconds int64               `json:"resolution_seconds"`
	Points            []MetricsTrendPoint `json:"points"`
}

type MetricsTrendPoint struct {
	Timestamp  time.Time        `json:"timestamp"`
	Throughput int64            `json:"throughput"`
	Failed     int64            `json:"failed"`
	QueueDepth int64            `json:"queue_depth"`
	Severity   map[string]int64 `json:"severity"`
	APIP90Ms   map[string]int64 `json:"api_p90_ms,omitempty"`
}

type trendBucket struct {
	Start         time.Time
	Throughput    int64
	Failed        int64
	QueueDepth    int64
	QueueDepthSet bool
	Severity      map[string]int64
	APILatency    map[string]boundedHistogram
}

type trendRing struct {
	Resolution time.Duration
	Slots      []trendBucket
}

func newTrendRing(resolution time.Duration, window time.Duration) trendRing {
	if resolution <= 0 {
		resolution = time.Minute
	}
	if window < resolution {
		window = resolution
	}
	if window%resolution != 0 {
		window = ((window / resolution) + 1) * resolution
	}
	size := int(window / resolution)
	if size < 1 {
		size = 1
	}
	return trendRing{
		Resolution: resolution,
		Slots:      make([]trendBucket, size),
	}
}

func selectMetricsTrendWindow(window string) (metricsTrendWindowSpec, bool) {
	window = strings.TrimSpace(strings.ToLower(window))
	spec, ok := metricsTrendWindowSpecs[window]
	return spec, ok
}

func (m *MetricsCollector) observeTrendQueueDepthLocked(now time.Time, queueDepth int64) {
	m.trend1m.ObserveQueueDepth(now, queueDepth)
	m.trend5m.ObserveQueueDepth(now, queueDepth)
}

func (m *MetricsCollector) observeTrendOutcomeLocked(now time.Time, status JobStatus) {
	m.trend1m.ObserveOutcome(now, status)
	m.trend5m.ObserveOutcome(now, status)
}

func (m *MetricsCollector) observeTrendSeverityLocked(now time.Time, severity map[string]int64) {
	m.trend1m.ObserveSeverity(now, severity)
	m.trend5m.ObserveSeverity(now, severity)
}

func (m *MetricsCollector) observeTrendAPIRequestLocked(now time.Time, routeKey string, duration time.Duration) {
	m.trend1m.ObserveAPIRequest(now, routeKey, duration)
	m.trend5m.ObserveAPIRequest(now, routeKey, duration)
}

func (m *MetricsCollector) snapshotTrendsLocked(now time.Time) MetricsTrendsSnapshot {
	windows := make(map[string]MetricsTrendWindowSnapshot, len(metricsTrendWindowOrder))
	for _, window := range metricsTrendWindowOrder {
		spec, ok := selectMetricsTrendWindow(window)
		if !ok {
			continue
		}
		windows[window] = m.snapshotTrendWindowLocked(now, spec)
	}
	return MetricsTrendsSnapshot{Windows: windows}
}

func (m *MetricsCollector) snapshotTrendWindowLocked(now time.Time, spec metricsTrendWindowSpec) MetricsTrendWindowSnapshot {
	switch spec.Resolution {
	case time.Minute:
		return m.trend1m.Snapshot(now, spec.Window)
	case 5 * time.Minute:
		return m.trend5m.Snapshot(now, spec.Window)
	default:
		return MetricsTrendWindowSnapshot{}
	}
}

func (r *trendRing) ObserveQueueDepth(at time.Time, queueDepth int64) {
	bucket := r.bucketForWrite(at)
	if bucket == nil {
		return
	}
	bucket.QueueDepth = queueDepth
	bucket.QueueDepthSet = true
}

func (r *trendRing) ObserveOutcome(at time.Time, status JobStatus) {
	if !isTerminalMetricsStatus(status) {
		return
	}
	bucket := r.bucketForWrite(at)
	if bucket == nil {
		return
	}
	bucket.Throughput++
	if status == JobFailed {
		bucket.Failed++
	}
}

func (r *trendRing) ObserveSeverity(at time.Time, severity map[string]int64) {
	if len(severity) == 0 {
		return
	}
	bucket := r.bucketForWrite(at)
	if bucket == nil {
		return
	}
	if bucket.Severity == nil {
		bucket.Severity = zeroMetricsSeverityTotals()
	}
	for _, level := range metricsSeverityLevels {
		bucket.Severity[level] += severity[level]
	}
}

func (r *trendRing) ObserveAPIRequest(at time.Time, routeKey string, duration time.Duration) {
	if strings.TrimSpace(routeKey) == "" {
		return
	}
	bucket := r.bucketForWrite(at)
	if bucket == nil {
		return
	}
	if bucket.APILatency == nil {
		bucket.APILatency = map[string]boundedHistogram{}
	}
	histogram, ok := bucket.APILatency[routeKey]
	if !ok {
		histogram = newBoundedHistogram(metricsLatencyBucketsMs[:])
	}
	histogram.Observe(duration)
	bucket.APILatency[routeKey] = histogram
}

func (r *trendRing) Snapshot(now time.Time, window time.Duration) MetricsTrendWindowSnapshot {
	if r == nil || len(r.Slots) == 0 || r.Resolution <= 0 {
		return MetricsTrendWindowSnapshot{}
	}
	if window <= 0 {
		window = r.Resolution
	}
	if window > time.Duration(len(r.Slots))*r.Resolution {
		window = time.Duration(len(r.Slots)) * r.Resolution
	}
	if window%r.Resolution != 0 {
		window = (window / r.Resolution) * r.Resolution
	}
	pointCount := int(window / r.Resolution)
	if pointCount < 1 {
		pointCount = 1
	}

	now = now.UTC()
	end := now.Truncate(r.Resolution)
	start := end.Add(-time.Duration(pointCount-1) * r.Resolution)

	points := make([]MetricsTrendPoint, 0, pointCount)
	lastQueueDepth := int64(0)
	hasQueueDepth := false
	for idx := 0; idx < pointCount; idx++ {
		timestamp := start.Add(time.Duration(idx) * r.Resolution)
		point := MetricsTrendPoint{
			Timestamp: timestamp,
			Severity:  zeroMetricsSeverityTotals(),
		}
		bucket := r.bucketForRead(timestamp)
		if bucket != nil {
			point.Throughput = bucket.Throughput
			point.Failed = bucket.Failed
			if bucket.Severity != nil {
				point.Severity = copyStringCounts(bucket.Severity)
			}
			if len(bucket.APILatency) > 0 {
				point.APIP90Ms = map[string]int64{}
				for routeKey, histogram := range bucket.APILatency {
					point.APIP90Ms[routeKey] = histogram.Quantile(0.90)
				}
			}
			if bucket.QueueDepthSet {
				lastQueueDepth = bucket.QueueDepth
				hasQueueDepth = true
			}
		}
		if hasQueueDepth {
			point.QueueDepth = lastQueueDepth
		}
		points = append(points, point)
	}

	return MetricsTrendWindowSnapshot{
		ResolutionSeconds: int64(r.Resolution / time.Second),
		Points:            points,
	}
}

func (r *trendRing) bucketForWrite(at time.Time) *trendBucket {
	if r == nil || len(r.Slots) == 0 || r.Resolution <= 0 {
		return nil
	}
	start := at.UTC().Truncate(r.Resolution)
	index := r.slotIndex(start)
	if index < 0 {
		return nil
	}
	bucket := &r.Slots[index]
	if !bucket.Start.Equal(start) {
		resetTrendBucket(bucket, start)
	}
	return bucket
}

func (r *trendRing) bucketForRead(start time.Time) *trendBucket {
	if r == nil || len(r.Slots) == 0 || r.Resolution <= 0 {
		return nil
	}
	start = start.UTC().Truncate(r.Resolution)
	index := r.slotIndex(start)
	if index < 0 {
		return nil
	}
	bucket := &r.Slots[index]
	if bucket.Start.Equal(start) {
		return bucket
	}
	return nil
}

func (r *trendRing) slotIndex(start time.Time) int {
	if len(r.Slots) == 0 || r.Resolution <= 0 {
		return -1
	}
	resolutionSeconds := int64(r.Resolution / time.Second)
	if resolutionSeconds <= 0 {
		return -1
	}
	tick := start.Unix() / resolutionSeconds
	size := int64(len(r.Slots))
	index := tick % size
	if index < 0 {
		index += size
	}
	return int(index)
}

func resetTrendBucket(bucket *trendBucket, start time.Time) {
	if bucket == nil {
		return
	}
	bucket.Start = start
	bucket.Throughput = 0
	bucket.Failed = 0
	bucket.QueueDepth = 0
	bucket.QueueDepthSet = false
	bucket.Severity = nil
	bucket.APILatency = nil
}
