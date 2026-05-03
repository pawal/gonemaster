package server

import (
	"testing"
	"time"
)

func TestSelectMetricsTrendWindow(t *testing.T) {
	tests := []struct {
		window             string
		wantFound          bool
		wantResolutionSecs int64
		wantPoints         int
	}{
		{window: "1h", wantFound: true, wantResolutionSecs: 60, wantPoints: 60},
		{window: "6h", wantFound: true, wantResolutionSecs: 60, wantPoints: 360},
		{window: "24h", wantFound: true, wantResolutionSecs: 300, wantPoints: 288},
		{window: "48h", wantFound: true, wantResolutionSecs: 300, wantPoints: 576},
		{window: "bad", wantFound: false},
	}

	for _, tc := range tests {
		spec, ok := selectMetricsTrendWindow(tc.window)
		if ok != tc.wantFound {
			t.Fatalf("selectMetricsTrendWindow(%q) found = %t, want %t", tc.window, ok, tc.wantFound)
		}
		if !ok {
			continue
		}
		if got := int64(spec.Resolution / time.Second); got != tc.wantResolutionSecs {
			t.Fatalf("selectMetricsTrendWindow(%q) resolution = %d, want %d", tc.window, got, tc.wantResolutionSecs)
		}
		if got := int(spec.Window / spec.Resolution); got != tc.wantPoints {
			t.Fatalf("selectMetricsTrendWindow(%q) points = %d, want %d", tc.window, got, tc.wantPoints)
		}
	}
}

func TestMetricsCollectorTrendRolloverAndWindowBounds(t *testing.T) {
	cfg := DefaultConfig()
	base := time.Date(2026, 2, 8, 12, 0, 0, 0, time.UTC)
	now := base
	collector := newMetricsCollector(cfg, base)
	collector.nowFn = func() time.Time { return now }

	for idx := range 370 {
		now = base.Add(time.Duration(idx) * time.Minute)
		collector.ObserveJobSubmitted(JobQueued)
		collector.ObserveJobStatusTransition(JobQueued, JobSucceeded)
		collector.ObserveJobCompletion(JobSucceeded, 500*time.Millisecond, map[string]int64{
			"NOTICE": 1,
		})
		collector.ObserveDNSQueries(3, 9)
		collector.ObserveAPIRequest("/api/v1/jobs", "GET", 200, 120*time.Millisecond, "")
	}

	snapshot := collector.Snapshot()

	window1h := snapshot.Trends.Windows["1h"]
	if window1h.ResolutionSeconds != 60 {
		t.Fatalf("1h.resolution_seconds = %d, want 60", window1h.ResolutionSeconds)
	}
	if len(window1h.Points) != 60 {
		t.Fatalf("1h.points size = %d, want 60", len(window1h.Points))
	}
	throughput1h := int64(0)
	for _, point := range window1h.Points {
		throughput1h += point.Throughput
	}
	if throughput1h != 60 {
		t.Fatalf("1h throughput total = %d, want 60", throughput1h)
	}

	window6h := snapshot.Trends.Windows["6h"]
	if window6h.ResolutionSeconds != 60 {
		t.Fatalf("6h.resolution_seconds = %d, want 60", window6h.ResolutionSeconds)
	}
	if len(window6h.Points) != 360 {
		t.Fatalf("6h.points size = %d, want 360", len(window6h.Points))
	}
	if !window6h.Points[0].Timestamp.Equal(base.Add(10 * time.Minute)) {
		t.Fatalf("6h first timestamp = %s, want %s", window6h.Points[0].Timestamp, base.Add(10*time.Minute))
	}
	if !window6h.Points[len(window6h.Points)-1].Timestamp.Equal(now) {
		t.Fatalf("6h last timestamp = %s, want %s", window6h.Points[len(window6h.Points)-1].Timestamp, now)
	}
	throughput6h := int64(0)
	notice6h := int64(0)
	for _, point := range window6h.Points {
		throughput6h += point.Throughput
		notice6h += point.Severity["NOTICE"]
	}
	if throughput6h != 360 {
		t.Fatalf("6h throughput total = %d, want 360", throughput6h)
	}
	if notice6h != 360 {
		t.Fatalf("6h severity NOTICE total = %d, want 360", notice6h)
	}
	if got := window6h.Points[len(window6h.Points)-1].APIP90Ms["GET /api/v1/jobs"]; got == 0 {
		t.Fatalf("expected api_p90_ms for GET /api/v1/jobs in latest 6h point")
	}
	if got := window6h.Points[len(window6h.Points)-1].DNSQueriesPerSecond; got <= 0 {
		t.Fatalf("expected dns_queries_per_second in latest 6h point, got %f", got)
	}
	if got := window6h.Points[len(window6h.Points)-1].DNSQueriesIPv4PerSecond; got <= 0 {
		t.Fatalf("expected dns_queries_ipv4_per_second in latest 6h point, got %f", got)
	}
	if got := window6h.Points[len(window6h.Points)-1].DNSQueriesIPv6PerSecond; got <= 0 {
		t.Fatalf("expected dns_queries_ipv6_per_second in latest 6h point, got %f", got)
	}

	window24h := snapshot.Trends.Windows["24h"]
	if window24h.ResolutionSeconds != 300 {
		t.Fatalf("24h.resolution_seconds = %d, want 300", window24h.ResolutionSeconds)
	}
	if len(window24h.Points) != 288 {
		t.Fatalf("24h.points size = %d, want 288", len(window24h.Points))
	}

	window48h := snapshot.Trends.Windows["48h"]
	if window48h.ResolutionSeconds != 300 {
		t.Fatalf("48h.resolution_seconds = %d, want 300", window48h.ResolutionSeconds)
	}
	if len(window48h.Points) != 576 {
		t.Fatalf("48h.points size = %d, want 576", len(window48h.Points))
	}
}

func TestMetricsCollectorTrendQueueDepthCarriesForward(t *testing.T) {
	cfg := DefaultConfig()
	base := time.Date(2026, 2, 8, 12, 0, 0, 0, time.UTC)
	now := base
	collector := newMetricsCollector(cfg, base)
	collector.nowFn = func() time.Time { return now }

	collector.ObserveJobSubmitted(JobQueued)
	now = base.Add(2 * time.Minute)

	snapshot := collector.Snapshot()
	window1h := snapshot.Trends.Windows["1h"]
	last := window1h.Points[len(window1h.Points)-1]
	if last.QueueDepth != 1 {
		t.Fatalf("queue_depth at %s = %d, want 1", last.Timestamp, last.QueueDepth)
	}

	now = base.Add(3 * time.Minute)
	collector.ObserveJobStatusTransition(JobQueued, JobRunning)
	snapshot = collector.Snapshot()
	window1h = snapshot.Trends.Windows["1h"]
	last = window1h.Points[len(window1h.Points)-1]
	if last.QueueDepth != 0 {
		t.Fatalf("queue_depth at %s = %d, want 0", last.Timestamp, last.QueueDepth)
	}
}
