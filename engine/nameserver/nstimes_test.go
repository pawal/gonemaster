package nameserver

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	dns "codeberg.org/miekg/dns"
	"codeberg.org/pawal/gonemaster/engine/packet"
)

func TestComputeTimingStats(t *testing.T) {
	ms := func(values ...int) []time.Duration {
		out := make([]time.Duration, 0, len(values))
		for _, v := range values {
			out = append(out, time.Duration(v)*time.Millisecond)
		}
		return out
	}

	tests := []struct {
		name       string
		times      []time.Duration
		wantCount  int
		wantMin    float64
		wantMax    float64
		wantAvg    float64
		wantMedian float64
		wantTotal  float64
		wantStddev float64
	}{
		{
			name:  "empty",
			times: nil,
		},
		{
			name:       "single",
			times:      ms(10),
			wantCount:  1,
			wantMin:    10,
			wantMax:    10,
			wantAvg:    10,
			wantMedian: 10,
			wantTotal:  10,
		},
		{
			name:       "odd count",
			times:      ms(10, 20, 30, 40, 50),
			wantCount:  5,
			wantMin:    10,
			wantMax:    50,
			wantAvg:    30,
			wantMedian: 30,
			wantTotal:  150,
			wantStddev: math.Sqrt(200),
		},
		{
			name:       "even count averages the two middle samples",
			times:      ms(10, 20, 30, 40),
			wantCount:  4,
			wantMin:    10,
			wantMax:    40,
			wantAvg:    25,
			wantMedian: 25,
			wantTotal:  100,
			wantStddev: math.Sqrt(125),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			stats := ComputeTimingStats(tc.times)

			if stats.Count != tc.wantCount {
				t.Fatalf("Count = %d, want %d", stats.Count, tc.wantCount)
			}
			if stats.Min != tc.wantMin {
				t.Fatalf("Min = %f, want %f", stats.Min, tc.wantMin)
			}
			if stats.Max != tc.wantMax {
				t.Fatalf("Max = %f, want %f", stats.Max, tc.wantMax)
			}
			if stats.Avg != tc.wantAvg {
				t.Fatalf("Avg = %f, want %f", stats.Avg, tc.wantAvg)
			}
			if stats.Median != tc.wantMedian {
				t.Fatalf("Median = %f, want %f", stats.Median, tc.wantMedian)
			}
			if stats.Total != tc.wantTotal {
				t.Fatalf("Total = %f, want %f", stats.Total, tc.wantTotal)
			}
			if math.Abs(stats.Stddev-tc.wantStddev) > 0.01 {
				t.Fatalf("Stddev = %f, want %f", stats.Stddev, tc.wantStddev)
			}
		})
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
	synctest.Test(t, func(t *testing.T) {
		ctx, prof := testContext(t)
		prof.Resolver.Defaults.ErrorCacheTTL = 0

		store := NewCacheStore()
		ns := hookedNS(t, store, "ns.example", "192.0.2.253", func(_ context.Context, _ string, _ string, _ string, _ *QueryOptions) (packet.Packet, error) {
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
	})
}

// TestQueryNetworkRecordsRefused verifies the other half of the load signal.
// REFUSED arrives as a parsed answer with a normal response time, not as an
// error, so it is invisible in both the timeout count and the timing stats -
// a farm that starts refusing under load looks, to every existing counter,
// exactly like a farm answering promptly. The response itself must reach the
// caller untouched so UNEXPECTED_RCODE-class findings still fire.
func TestQueryNetworkRecordsRefused(t *testing.T) {
	ctx, prof := testContext(t)
	prof.Resolver.Defaults.ErrorCacheTTL = 0

	store := NewCacheStore()
	ns := hookedNS(t, store, "ns.example", "192.0.2.251", func(_ context.Context, qname string, _ string, _ string, _ *QueryOptions) (packet.Packet, error) {
		msg := new(dns.Msg)
		// A NOERROR answer for the second name proves the counter keys on
		// the rcode, not merely on "a response arrived".
		if qname == "clean" {
			msg.Rcode = dns.RcodeSuccess
		} else {
			msg.Rcode = dns.RcodeRefused
		}
		return packet.Packet{Msg: msg, QueryTime: 2 * time.Millisecond}, nil
	})

	resp, err := ns.QueryWithOptions(ctx, "refused", "A", &QueryOptions{BlacklistingDisabled: true})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if resp.Msg == nil || resp.Msg.Rcode != dns.RcodeRefused {
		t.Fatalf("REFUSED response must reach the caller unchanged, got %+v", resp.Msg)
	}
	if _, err := ns.QueryWithOptions(ctx, "clean", "A", &QueryOptions{BlacklistingDisabled: true}); err != nil {
		t.Fatalf("query: %v", err)
	}

	const key = "ns.example/192.0.2.251"
	if got := store.QueryRefused()[key]; got != 1 {
		t.Fatalf("QueryRefused[%q] = %d, want 1 (%+v)", key, got, store.QueryRefused())
	}
	// A REFUSED answer is a response, not a failure to answer: it must not
	// be double-counted as a timeout.
	if got := store.QueryTimeouts()[key]; got != 0 {
		t.Fatalf("QueryTimeouts[%q] = %d, want 0", key, got)
	}
}

// TestQueryNetworkRefusedNotCountedOnCancelledContext mirrors the timeout
// rule: a cancelled job must not be charged to the nameserver, or a batch
// that shuts down mid-run would inflate every farm's refused count.
func TestQueryNetworkRefusedNotCountedOnCancelledContext(t *testing.T) {
	ctx, prof := testContext(t)
	prof.Resolver.Defaults.ErrorCacheTTL = 0
	ctx, cancel := context.WithCancel(ctx)

	store := NewCacheStore()
	ns := hookedNS(t, store, "ns.example", "192.0.2.250", func(_ context.Context, _ string, _ string, _ string, _ *QueryOptions) (packet.Packet, error) {
		cancel()
		msg := new(dns.Msg)
		msg.Rcode = dns.RcodeRefused
		return packet.Packet{Msg: msg, QueryTime: 2 * time.Millisecond}, nil
	})

	if _, err := ns.QueryWithOptions(ctx, "refused", "A", &QueryOptions{BlacklistingDisabled: true}); err != nil {
		t.Fatalf("query: %v", err)
	}
	if got := store.QueryRefused()["ns.example/192.0.2.250"]; got != 0 {
		t.Fatalf("cancelled query charged to the nameserver: refused count %d, want 0", got)
	}
}

// TestRecordQueryRefusedOnSnapshotStore is the sibling of the timeout guard
// below: SnapshotForRun builds its store from a literal, not NewCacheStore,
// so a map added to only one of the two constructors panics on the server
// path while every CLI test stays green.
func TestRecordQueryRefusedOnSnapshotStore(t *testing.T) {
	base := NewCacheStore()
	run := base.SnapshotForRun()

	run.RecordQueryRefused("ns.example/192.0.2.1")
	run.RecordQueryRefused("ns.example/192.0.2.1")

	if got := run.QueryRefused()["ns.example/192.0.2.1"]; got != 2 {
		t.Fatalf("expected refused count 2 on snapshot store, got %d", got)
	}
	// Refused counts are run-local: a snapshot must not write through to
	// the shared parent, where a later run would inherit them.
	if got := base.QueryRefused()["ns.example/192.0.2.1"]; got != 0 {
		t.Fatalf("snapshot refused count leaked into the parent store: %d", got)
	}
}

// TestRecordQueryTimeoutOnSnapshotStore guards against the nil-map panic that
// hit the server: SnapshotForRun builds a run-local store, and every timed-out
// query in that run calls RecordQueryTimeout on it. The count must be recorded
// without panicking regardless of how the store was constructed.
func TestRecordQueryTimeoutOnSnapshotStore(t *testing.T) {
	base := NewCacheStore()
	run := base.SnapshotForRun()

	run.RecordQueryTimeout("ns.example/192.0.2.1")
	run.RecordQueryTimeout("ns.example/192.0.2.1")

	if got := run.QueryTimeouts()["ns.example/192.0.2.1"]; got != 2 {
		t.Fatalf("expected timeout count 2 on snapshot store, got %d", got)
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
	out := TimingsFromQueryMap(nil, nil, nil)
	if len(out) != 0 {
		t.Fatalf("expected empty slice, got %d entries", len(out))
	}
	out = TimingsFromQueryMap(map[string][]time.Duration{}, map[string]int{}, map[string]int{})
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
	out := TimingsFromQueryMap(timings, timeouts, nil)
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

// TestTimingsFromQueryMapCarriesTimeoutCount pins the per-address timeout
// count onto the emitted rows. An address that answers most queries but
// times out on some is exactly the signature a rate limiter produces, and
// before this field the count was visible only in a local --debug-queries
// run, never in a stored result.
// Both row shapes must carry it: the "ok" row for an address that answered
// at least once, and the "unreachable" row for one that never did.
func TestTimingsFromQueryMapCarriesTimeoutCount(t *testing.T) {
	timings := map[string][]time.Duration{
		"ns1.example.com/192.0.2.1": {10 * time.Millisecond, 20 * time.Millisecond},
		"ns3.example.com/192.0.2.3": {5 * time.Millisecond},
	}
	timeouts := map[string]int{
		"ns1.example.com/192.0.2.1": 4,
		"ns2.example.com/192.0.2.2": 3,
	}
	out := TimingsFromQueryMap(timings, timeouts, nil)

	byKey := map[string]NameserverTiming{}
	for _, item := range out {
		byKey[item.Nameserver+"/"+item.Address] = item
	}

	if got := byKey["ns1.example.com/192.0.2.1"]; got.TimeoutCount != 4 {
		t.Fatalf("mixed answer/timeout row: TimeoutCount = %d, want 4 (%+v)", got.TimeoutCount, got)
	}
	if got := byKey["ns2.example.com/192.0.2.2"]; got.TimeoutCount != 3 {
		t.Fatalf("timeout-only row: TimeoutCount = %d, want 3 (%+v)", got.TimeoutCount, got)
	}
	// An address with no timeouts at all must leave the field zero so the
	// omitempty JSON tag keeps it out of stored blobs entirely.
	if got := byKey["ns3.example.com/192.0.2.3"]; got.TimeoutCount != 0 {
		t.Fatalf("clean row: TimeoutCount = %d, want 0 (%+v)", got.TimeoutCount, got)
	}
}

// TestNameserverTimingCountsOmittedWhenZero pins the JSON additivity claim:
// rows without the new counters must serialize exactly as before, so old
// stored blobs and new ones stay comparable.
func TestNameserverTimingCountsOmittedWhenZero(t *testing.T) {
	clean, err := json.Marshal(NameserverTiming{Nameserver: "ns1.example.com", Address: "192.0.2.1", Status: NameserverTimingStatusOK})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(clean), "timeout_count") || strings.Contains(string(clean), "refused_count") {
		t.Fatalf("zero counters must be omitted, got %s", clean)
	}

	counted, err := json.Marshal(NameserverTiming{Nameserver: "ns1.example.com", Address: "192.0.2.1", TimeoutCount: 2, RefusedCount: 5})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(counted), `"timeout_count":2`) {
		t.Fatalf("expected timeout_count in %s", counted)
	}
	if !strings.Contains(string(counted), `"refused_count":5`) {
		t.Fatalf("expected refused_count in %s", counted)
	}
}

func TestTimingsFromQueryMapSingle(t *testing.T) {
	timings := map[string][]time.Duration{
		"ns1.example.com/192.0.2.1": {10 * time.Millisecond, 20 * time.Millisecond},
	}
	out := TimingsFromQueryMap(timings, nil, nil)
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
	out := TimingsFromQueryMap(timings, nil, nil)
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
	out := TimingsFromQueryMap(timings, nil, nil)
	if len(out) != 1 {
		t.Fatalf("expected 1 entry (malformed key skipped), got %d", len(out))
	}
	if out[0].Nameserver != "ns1.example.com" {
		t.Fatalf("Nameserver = %q, want %q", out[0].Nameserver, "ns1.example.com")
	}
}
