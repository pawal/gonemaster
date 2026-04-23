package server

import (
	"reflect"
	"testing"
	"time"

	"codeberg.org/pawal/gonemaster/engine"
)

func TestFilterAnalysisRunAddressASNsToAuthoritativeDropsNonAuthoritativeAddresses(t *testing.T) {
	auth := map[int64]struct{}{1: {}, 2: {}}
	facts := []AnalysisRunAddressASN{
		{CohortID: 1, RunID: "r1", DomainID: 1, AddressID: 1},
		// AddressID 99 is not in the authoritative set — simulates a
		// parent-side address observed in entry args.
		{CohortID: 1, RunID: "r1", DomainID: 1, AddressID: 99},
		{CohortID: 1, RunID: "r2", DomainID: 2, AddressID: 2},
	}
	got := filterAnalysisRunAddressASNsToAuthoritative(facts, auth)
	want := []AnalysisRunAddressASN{
		{CohortID: 1, RunID: "r1", DomainID: 1, AddressID: 1},
		{CohortID: 1, RunID: "r2", DomainID: 2, AddressID: 2},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("filtered facts = %+v, want %+v", got, want)
	}
}

func TestFilterAnalysisRunAddressASNsToAuthoritativeEmptySetDropsEverything(t *testing.T) {
	// If no endpoint was classified authoritative (e.g. a run where
	// nameserver_timings was missing and every (ns, addr) pair defaulted
	// to parent), the cohort has no authoritative addresses and no fact
	// should leak through.
	facts := []AnalysisRunAddressASN{{CohortID: 1, RunID: "r", AddressID: 1}}
	if got := filterAnalysisRunAddressASNsToAuthoritative(facts, map[int64]struct{}{}); got != nil {
		t.Fatalf("expected nil with empty authoritative set, got %+v", got)
	}
}

// sameBacking reports whether a and b share the same backing array, which is
// a cheap proxy for "this slice was returned from the same computation."
func sameBacking[T any](a, b []T) bool {
	if len(a) == 0 || len(b) == 0 {
		return len(a) == 0 && len(b) == 0
	}
	return &a[0] == &b[0]
}

func TestAnalysisMatCacheHitsOnIdenticalStamp(t *testing.T) {
	f := newAnalysisAPITestFixture(t)
	ts := time.Date(2026, 4, 17, 12, 0, 0, 0, time.UTC)
	f.seedGraduatedRun("alpha.example", ts, []engine.LogEntry{
		{Module: "DNSSEC", Testcase: "dnssec07", Tag: "DS07_NOT_SIGNED", Level: "ERROR"},
	})

	first := f.srv.latestMaterializationForSnapshot(f.cohort, f.snapshot)
	if len(first.latest) != 1 {
		t.Fatalf("expected one latest summary, got %d", len(first.latest))
	}
	second := f.srv.latestMaterializationForSnapshot(f.cohort, f.snapshot)
	if !sameBacking(first.latest, second.latest) {
		t.Fatal("expected same backing array on repeated call (cache hit)")
	}
}

func TestAnalysisMatCacheInvalidatesOnStampChange(t *testing.T) {
	f := newAnalysisAPITestFixture(t)
	ts := time.Date(2026, 4, 17, 12, 0, 0, 0, time.UTC)
	f.seedGraduatedRun("alpha.example", ts, []engine.LogEntry{
		{Module: "DNSSEC", Testcase: "dnssec07", Tag: "DS07_NOT_SIGNED", Level: "ERROR"},
	})

	first := f.srv.latestMaterializationForSnapshot(f.cohort, f.snapshot)

	// A re-materialization (admin rematerialize action) moves CapturedAt
	// forward. The cache must recompute rather than return stale data.
	bumped := f.snapshot
	bumped.CapturedAt = f.snapshot.CapturedAt.Add(time.Hour)
	third := f.srv.latestMaterializationForSnapshot(f.cohort, bumped)
	if sameBacking(first.latest, third.latest) {
		t.Fatal("expected fresh compute after CapturedAt advanced")
	}
}

func TestAnalysisMatCacheIsKeyedPerSnapshot(t *testing.T) {
	f := newAnalysisAPITestFixture(t)
	ts := time.Date(2026, 4, 17, 12, 0, 0, 0, time.UTC)
	f.seedGraduatedRun("alpha.example", ts, []engine.LogEntry{
		{Module: "BASIC", Testcase: "basic01", Tag: "B01", Level: "NOTICE"},
	})

	f.srv.latestMaterializationForSnapshot(f.cohort, f.snapshot)

	other := AnalysisCohortSnapshot{ID: f.snapshot.ID + 1, CohortID: f.cohort.ID, BatchID: f.batchID, CapturedAt: f.snapshot.CapturedAt}
	f.srv.latestMaterializationForSnapshot(f.cohort, other)

	f.srv.analysisMatCacheMu.Lock()
	defer f.srv.analysisMatCacheMu.Unlock()
	if _, ok := f.srv.analysisMatCache[f.snapshot.ID]; !ok {
		t.Fatalf("expected cache entry for snapshot %d", f.snapshot.ID)
	}
	if _, ok := f.srv.analysisMatCache[other.ID]; !ok {
		t.Fatalf("expected separate cache entry for snapshot %d", other.ID)
	}
}
