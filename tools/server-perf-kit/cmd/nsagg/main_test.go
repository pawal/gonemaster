package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"
)

// TestAddrStatsAccumulatesAcrossRuns pins the central claim of the tool: one
// address is seen once per run, and the interesting quantities are sums over
// the whole batch. A rate limiter that drops one query in ten is invisible in
// any single run's row and obvious in the sum over 500 of them.
func TestAddrStatsAccumulatesAcrossRuns(t *testing.T) {
	s := &addrStats{nameserver: "ns01.example", address: "192.0.2.1"}
	s.add(nameserverTiming{Count: 40, MedianMS: 10, MaxMS: 30, Status: "ok"}, 0)
	s.add(nameserverTiming{Count: 38, MedianMS: 20, MaxMS: 90, Status: "ok", TimeoutCount: 2, RefusedCount: 1}, 0)
	s.add(nameserverTiming{Count: 0, Status: "unreachable", TimeoutCount: 5}, 0)

	if s.runs != 3 {
		t.Fatalf("runs = %d, want 3", s.runs)
	}
	if s.queries != 78 {
		t.Fatalf("queries = %d, want 78", s.queries)
	}
	if s.timeouts != 7 || s.refused != 1 {
		t.Fatalf("timeouts/refused = %d/%d, want 7/1", s.timeouts, s.refused)
	}
	if s.unreachable != 1 {
		t.Fatalf("unreachable runs = %d, want 1", s.unreachable)
	}
	// Two of the three runs saw a failure; the clean first run must not be
	// counted, since "how often does this address misbehave at all" is a
	// different question from "how many queries did it drop".
	if s.runsWithFail != 2 {
		t.Fatalf("runs with failures = %d, want 2", s.runsWithFail)
	}
	if s.maxMS != 90 {
		t.Fatalf("maxMS = %f, want 90", s.maxMS)
	}
	// The unreachable run contributes no median: averaging a zero in would
	// drag the response time of a healthy-but-limited address toward zero,
	// which is the opposite of the truth.
	if got := avg(s.medianSum, s.medianRuns); got != "15.00" {
		t.Fatalf("avg median = %q, want 15.00", got)
	}
}

// TestEngagementReplayIsPerRunAndRequiresAPriorAnswer pins the replay used
// to estimate how often a backoff trigger would fire on a corpus that is
// not being rate limited. Two conditions have to hold together, and each
// has a distinct failure mode if dropped:
//
//   - Per run, never summed. An address seen in fifty runs with one timeout
//     each sums to fifty and engages in none of them; summing first would
//     report a spurious-engagement rate near 100% on any large corpus.
//   - The address must have answered. "Timed out after answering" is the
//     whole thing separating a rate limiter (drops excess, answers the
//     rest) from a dead server, which fast-fail and blacklisting already
//     handle.
func TestEngagementReplayIsPerRunAndRequiresAPriorAnswer(t *testing.T) {
	s := &addrStats{}
	// Below threshold in each run, and three runs' worth in total: a sum
	// would engage here, a per-run replay must not.
	s.add(nameserverTiming{Count: 40, TimeoutCount: 1, Status: "ok"}, 3)
	s.add(nameserverTiming{Count: 40, TimeoutCount: 1, Status: "ok"}, 3)
	s.add(nameserverTiming{Count: 40, TimeoutCount: 1, Status: "ok"}, 3)
	if s.runsEngaged != 0 {
		t.Fatalf("runs_engaged = %d, want 0: engagement must not accumulate across runs", s.runsEngaged)
	}
	// Past threshold within one run, on an address that answered: engaged.
	s.add(nameserverTiming{Count: 30, TimeoutCount: 4, Status: "ok"}, 3)
	if s.runsEngaged != 1 {
		t.Fatalf("runs_engaged = %d, want 1", s.runsEngaged)
	}
	// Past threshold but never answered: a dead address, not a limited one.
	s.add(nameserverTiming{Count: 0, TimeoutCount: 9, Status: "unreachable"}, 3)
	if s.runsEngaged != 1 {
		t.Fatalf("runs_engaged = %d, want 1: an address that never answered is not evidence of limiting", s.runsEngaged)
	}
	// Threshold 0 disables the replay entirely.
	off := &addrStats{}
	off.add(nameserverTiming{Count: 30, TimeoutCount: 99, Status: "ok"}, 0)
	if off.runsEngaged != 0 {
		t.Fatalf("runs_engaged = %d with the replay off, want 0", off.runsEngaged)
	}
}

// TestRatioUsesAttemptsNotAnswers documents the denominator choice. A timeout
// produces no answer and therefore no entry in Count, so dividing by Count
// alone would understate the drop rate; the denominator has to be answers
// plus timeouts, i.e. what we actually asked the address for.
func TestRatioUsesAttemptsNotAnswers(t *testing.T) {
	if got := ratio(10, 100); got != "0.100000" {
		t.Fatalf("ratio(10, 100) = %q, want 0.100000", got)
	}
	if got := ratio(0, 0); got != "0" {
		t.Fatalf("ratio with a zero denominator = %q, want 0", got)
	}
}

// TestAggregateBatchMergesAddressAcrossDomains is the tool's reason to exist:
// the same farm address serves many customer domains, so its rows arrive from
// unrelated runs and must land in one bucket keyed by (nameserver, address).
func TestAggregateBatchMergesAddressAcrossDomains(t *testing.T) {
	results := map[string]runResult{
		"run-1": {NameserverTimings: []nameserverTiming{
			{Nameserver: "ns01.farm.example", Address: "192.0.2.1", Count: 40, MedianMS: 12, Status: "ok"},
			{Nameserver: "ns02.farm.example", Address: "192.0.2.2", Count: 40, MedianMS: 14, Status: "ok"},
		}},
		"run-2": {NameserverTimings: []nameserverTiming{
			{Nameserver: "ns01.farm.example", Address: "192.0.2.1", Count: 35, MedianMS: 18, Status: "ok", TimeoutCount: 4, RefusedCount: 2},
		}},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/api/v1/runs":
			if got := r.URL.Query().Get("batch"); got != "batch-x" {
				t.Errorf("batch filter = %q, want batch-x", got)
			}
			_ = json.NewEncoder(w).Encode(runList{
				Items: []runListItem{
					{ID: "run-1", Domain: "a.example", Status: "completed"},
					{ID: "run-2", Domain: "b.example", Status: "completed"},
				},
				Total: 2,
			})
		default:
			id := r.URL.Path[len("/api/v1/runs/") : len(r.URL.Path)-len("/result")]
			res, ok := results[id]
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			_ = json.NewEncoder(w).Encode(res)
		}
	}))
	defer srv.Close()

	stats, err := aggregateBatch(&http.Client{Timeout: 5 * time.Second}, srv.URL+"/api/v1", "batch-x", 1000, 0)
	if err != nil {
		t.Fatalf("aggregate: %v", err)
	}
	if len(stats) != 2 {
		t.Fatalf("addresses = %d, want 2 (%+v)", len(stats), stats)
	}
	hot := stats["ns01.farm.example/192.0.2.1"]
	if hot == nil {
		t.Fatalf("missing the shared address in %+v", stats)
	}
	if hot.runs != 2 || hot.queries != 75 {
		t.Fatalf("shared address: runs=%d queries=%d, want 2/75", hot.runs, hot.queries)
	}
	if hot.timeouts != 4 || hot.refused != 2 {
		t.Fatalf("shared address: timeouts=%d refused=%d, want 4/2", hot.timeouts, hot.refused)
	}
	clean := stats["ns02.farm.example/192.0.2.2"]
	if clean == nil || clean.timeouts != 0 || clean.refused != 0 {
		t.Fatalf("second address should be clean, got %+v", clean)
	}
}

// TestAggregateBatchSurvivesAMissingRunResult keeps a long characterization
// batch from dying on one purged or still-running run: the address totals are
// worth more than strictness, and the skip is reported on stderr.
func TestAggregateBatchSurvivesAMissingRunResult(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v1/runs" {
			_ = json.NewEncoder(w).Encode(runList{
				Items: []runListItem{{ID: "gone", Domain: "a.example"}, {ID: "ok", Domain: "b.example"}},
				Total: 2,
			})
			return
		}
		if r.URL.Path == "/api/v1/runs/gone/result" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_ = json.NewEncoder(w).Encode(runResult{NameserverTimings: []nameserverTiming{
			{Nameserver: "ns.example", Address: "192.0.2.9", Count: 10, MedianMS: 5, Status: "ok"},
		}})
	}))
	defer srv.Close()

	stats, err := aggregateBatch(&http.Client{Timeout: 5 * time.Second}, srv.URL+"/api/v1", "batch-y", 1000, 0)
	if err != nil {
		t.Fatalf("aggregate: %v", err)
	}
	if len(stats) != 1 || stats["ns.example/192.0.2.9"].queries != 10 {
		t.Fatalf("expected the surviving run to be aggregated, got %+v", stats)
	}
}

// TestListBatchRunsPagesPastTheServerCap covers the trap that a 500-domain
// farm corpus walks straight into: the runs endpoint refuses any limit above
// 500, and a single unpaged request would have silently dropped every run
// past the cap - producing a smaller, cleaner-looking aggregate rather than
// an error.
func TestListBatchRunsPagesPastTheServerCap(t *testing.T) {
	const total = 1200
	var requests []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
		if limit > 500 {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		requests = append(requests, r.URL.RawQuery)
		items := []runListItem{}
		for i := offset; i < offset+limit && i < total; i++ {
			items = append(items, runListItem{ID: fmt.Sprintf("run-%d", i)})
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(runList{Items: items, Total: total})
	}))
	defer srv.Close()

	items, err := listBatchRuns(&http.Client{Timeout: 5 * time.Second}, srv.URL+"/api/v1", "batch-big", 100000)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(items) != total {
		t.Fatalf("got %d runs, want %d", len(items), total)
	}
	if len(requests) != 3 {
		t.Fatalf("expected 3 pages of 500, got %d requests: %v", len(requests), requests)
	}
	if items[0].ID != "run-0" || items[total-1].ID != fmt.Sprintf("run-%d", total-1) {
		t.Fatalf("pages stitched wrong: first=%s last=%s", items[0].ID, items[total-1].ID)
	}
}

// TestAggregateBatchRejectsEmptyBatch turns a typo in a batch id into an
// error instead of a silently empty CSV, which in a measurement pipeline
// reads as "no rate limiting found".
func TestAggregateBatchRejectsEmptyBatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(runList{Items: nil, Total: 0})
	}))
	defer srv.Close()

	if _, err := aggregateBatch(&http.Client{Timeout: 5 * time.Second}, srv.URL+"/api/v1", "typo", 1000, 0); err == nil {
		t.Fatal("expected an error for a batch with no runs")
	}
}
