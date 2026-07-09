package nameserver

import (
	"context"
	"fmt"
	"math"
	"testing"
	"time"

	"codeberg.org/pawal/gonemaster/engine/packet"
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

// TestQueryNetworkRecordsTimeoutSeparately verifies that a timed-out query is
// counted as a timeout, not folded into the response-time samples. A server
// that only ever times out must therefore have no QueryTimings entry (so it is
// reported as unreachable rather than as a very slow "ok" server) while still
// showing up in QueryTimeouts so it stays visible in the timings output. The
// hook returns a timeout-pattern error after a short delay to model a dead
// server that swallows the query.
func TestQueryNetworkRecordsTimeoutSeparately(t *testing.T) {
	ctx, prof := testContext(t)
	prof.Resolver.Defaults.ErrorCacheTTL = 0

	store := NewCacheStore()
	ns, err := NewWithCache(store, "ns.example", "192.0.2.253", nil)
	if err != nil {
		t.Fatalf("new nameserver: %v", err)
	}
	ns.SetQueryHook(func(_ context.Context, _ string, _ string, _ string, _ *QueryOptions) (packet.Packet, error) {
		time.Sleep(3 * time.Millisecond)
		return packet.Packet{}, fmt.Errorf("read timeout")
	})

	if _, err := ns.QueryWithOptions(ctx, "example", "A", &QueryOptions{BlacklistingDisabled: true}); err == nil {
		t.Fatalf("expected timeout error")
	}

	const key = "ns.example/192.0.2.253"
	if got := store.QueryTimings()[key]; len(got) != 0 {
		t.Fatalf("expected no QueryTimings entry for a timed-out query, got %v", got)
	}
	if got := store.QueryTimeouts()[key]; got == 0 {
		t.Fatalf("expected a timeout count for %q, got none: %+v", key, store.QueryTimeouts())
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

func TestTimingsFromQueryMapEmpty(t *testing.T) {
	out := TimingsFromQueryMap(nil, nil)
	if len(out) != 0 {
		t.Fatalf("expected empty slice, got %d entries", len(out))
	}
	out = TimingsFromQueryMap(map[string][]time.Duration{}, map[string]int{})
	if len(out) != 0 {
		t.Fatalf("expected empty slice for empty map, got %d entries", len(out))
	}
}

// TestTimingsFromQueryMapTimeoutOnlyKey checks that a nameserver present only
// in the timeout counts (every query timed out) surfaces as an unreachable row
// with zero stats, while a server that answered stays "ok".
func TestTimingsFromQueryMapTimeoutOnlyKey(t *testing.T) {
	timings := map[string][]time.Duration{
		"ns1.example.com/192.0.2.1": {10 * time.Millisecond, 20 * time.Millisecond},
	}
	timeouts := map[string]int{
		"ns2.example.com/192.0.2.2": 3,
		// ns1 also timed out on some queries but answered others: it must
		// stay "ok", not be demoted to unreachable.
		"ns1.example.com/192.0.2.1": 1,
	}
	out := TimingsFromQueryMap(timings, timeouts)
	if len(out) != 2 {
		t.Fatalf("expected 2 entries, got %d: %+v", len(out), out)
	}

	byKey := map[string]NameserverTiming{}
	for _, item := range out {
		byKey[item.Nameserver+"/"+item.Address] = item
	}

	dead := byKey["ns2.example.com/192.0.2.2"]
	if dead.Status != NameserverTimingStatusUnreachable {
		t.Fatalf("expected ns2 unreachable, got %+v", dead)
	}
	if dead.Count != 0 || dead.AvgMS != 0 || dead.MaxMS != 0 {
		t.Fatalf("expected zero stats for unreachable row, got %+v", dead)
	}

	alive := byKey["ns1.example.com/192.0.2.1"]
	if alive.Status != NameserverTimingStatusOK || alive.Count != 2 {
		t.Fatalf("expected ns1 ok with 2 samples, got %+v", alive)
	}
}

func TestTimingsFromQueryMapSingle(t *testing.T) {
	timings := map[string][]time.Duration{
		"ns1.example.com/192.0.2.1": {10 * time.Millisecond, 20 * time.Millisecond},
	}
	out := TimingsFromQueryMap(timings, nil)
	if len(out) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(out))
	}
	nt := out[0]
	if nt.Nameserver != "ns1.example.com" {
		t.Fatalf("Nameserver = %q, want %q", nt.Nameserver, "ns1.example.com")
	}
	if nt.Address != "192.0.2.1" {
		t.Fatalf("Address = %q, want %q", nt.Address, "192.0.2.1")
	}
	if nt.Count != 2 {
		t.Fatalf("Count = %d, want 2", nt.Count)
	}
	if nt.AvgMS != 15 {
		t.Fatalf("AvgMS = %f, want 15", nt.AvgMS)
	}
	if nt.Status != NameserverTimingStatusOK {
		t.Fatalf("Status = %q, want %q", nt.Status, NameserverTimingStatusOK)
	}
}

func TestTimingsFromQueryMapSortedByNameThenAddress(t *testing.T) {
	timings := map[string][]time.Duration{
		"ns2.example.com/192.0.2.2": {30 * time.Millisecond},
		"ns1.example.com/192.0.2.2": {20 * time.Millisecond},
		"ns1.example.com/192.0.2.1": {10 * time.Millisecond},
	}
	out := TimingsFromQueryMap(timings, nil)
	if len(out) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(out))
	}
	if out[0].Nameserver != "ns1.example.com" || out[0].Address != "192.0.2.1" {
		t.Fatalf("entry[0] = %s/%s, want ns1.example.com/192.0.2.1", out[0].Nameserver, out[0].Address)
	}
	if out[1].Nameserver != "ns1.example.com" || out[1].Address != "192.0.2.2" {
		t.Fatalf("entry[1] = %s/%s, want ns1.example.com/192.0.2.2", out[1].Nameserver, out[1].Address)
	}
	if out[2].Nameserver != "ns2.example.com" || out[2].Address != "192.0.2.2" {
		t.Fatalf("entry[2] = %s/%s, want ns2.example.com/192.0.2.2", out[2].Nameserver, out[2].Address)
	}
}

func TestTimingsFromQueryMapMalformedKeySkipped(t *testing.T) {
	timings := map[string][]time.Duration{
		"no-slash":                  {10 * time.Millisecond},
		"ns1.example.com/192.0.2.1": {20 * time.Millisecond},
	}
	out := TimingsFromQueryMap(timings, nil)
	if len(out) != 1 {
		t.Fatalf("expected 1 entry (malformed key skipped), got %d", len(out))
	}
	if out[0].Nameserver != "ns1.example.com" {
		t.Fatalf("Nameserver = %q, want %q", out[0].Nameserver, "ns1.example.com")
	}
}
