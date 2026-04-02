package nameserver

import (
	"math"
	"testing"
	"time"
)

func TestComputeTimingStatsEmpty(t *testing.T) {
	stats := ComputeTimingStats(nil)
	if stats.Count != 0 {
		t.Fatalf("expected Count=0, got %d", stats.Count)
	}
}

func TestComputeTimingStatsSingle(t *testing.T) {
	stats := ComputeTimingStats([]time.Duration{10 * time.Millisecond})
	if stats.Count != 1 {
		t.Fatalf("expected Count=1, got %d", stats.Count)
	}
	if stats.Min != 10 || stats.Max != 10 || stats.Avg != 10 || stats.Median != 10 {
		t.Fatalf("expected all 10ms, got min=%f max=%f avg=%f median=%f", stats.Min, stats.Max, stats.Avg, stats.Median)
	}
	if stats.Stddev != 0 {
		t.Fatalf("expected stddev=0, got %f", stats.Stddev)
	}
	if stats.Total != 10 {
		t.Fatalf("expected total=10, got %f", stats.Total)
	}
}

func TestComputeTimingStatsMultiple(t *testing.T) {
	times := []time.Duration{
		10 * time.Millisecond,
		20 * time.Millisecond,
		30 * time.Millisecond,
		40 * time.Millisecond,
		50 * time.Millisecond,
	}
	stats := ComputeTimingStats(times)

	if stats.Count != 5 {
		t.Fatalf("Count = %d, want 5", stats.Count)
	}
	if stats.Min != 10 {
		t.Fatalf("Min = %f, want 10", stats.Min)
	}
	if stats.Max != 50 {
		t.Fatalf("Max = %f, want 50", stats.Max)
	}
	if stats.Total != 150 {
		t.Fatalf("Total = %f, want 150", stats.Total)
	}
	if stats.Avg != 30 {
		t.Fatalf("Avg = %f, want 30", stats.Avg)
	}
	if stats.Median != 30 {
		t.Fatalf("Median = %f, want 30", stats.Median)
	}
	// stddev of {10,20,30,40,50} = sqrt(200) ≈ 14.14
	if math.Abs(stats.Stddev-math.Sqrt(200)) > 0.01 {
		t.Fatalf("Stddev = %f, want ≈ %f", stats.Stddev, math.Sqrt(200))
	}
}

func TestComputeTimingStatsEvenCount(t *testing.T) {
	times := []time.Duration{
		10 * time.Millisecond,
		20 * time.Millisecond,
		30 * time.Millisecond,
		40 * time.Millisecond,
	}
	stats := ComputeTimingStats(times)
	// median of {10,20,30,40} = (20+30)/2 = 25
	if stats.Median != 25 {
		t.Fatalf("Median = %f, want 25", stats.Median)
	}
}

func TestCacheStoreRecordQueryTime(t *testing.T) {
	cs := NewCacheStore()
	cs.RecordQueryTime("ns1.example.com/192.0.2.1", 10*time.Millisecond)
	cs.RecordQueryTime("ns1.example.com/192.0.2.1", 20*time.Millisecond)
	cs.RecordQueryTime("ns2.example.com/192.0.2.2", 5*time.Millisecond)

	timings := cs.QueryTimings()
	if len(timings) != 2 {
		t.Fatalf("expected 2 nameservers, got %d", len(timings))
	}
	if len(timings["ns1.example.com/192.0.2.1"]) != 2 {
		t.Fatalf("expected 2 entries for ns1, got %d", len(timings["ns1.example.com/192.0.2.1"]))
	}
	if len(timings["ns2.example.com/192.0.2.2"]) != 1 {
		t.Fatalf("expected 1 entry for ns2, got %d", len(timings["ns2.example.com/192.0.2.2"]))
	}
}

func TestQueryTimingsReturnsDeepCopy(t *testing.T) {
	cs := NewCacheStore()
	cs.RecordQueryTime("key", 10*time.Millisecond)
	copy1 := cs.QueryTimings()
	copy1["key"][0] = 999 * time.Millisecond
	copy2 := cs.QueryTimings()
	if copy2["key"][0] != 10*time.Millisecond {
		t.Fatal("QueryTimings did not return a deep copy")
	}
}

func TestQueryTimingsNilCacheStore(t *testing.T) {
	var cs *CacheStore
	timings := cs.QueryTimings()
	if timings != nil {
		t.Fatal("expected nil for nil CacheStore")
	}
}
