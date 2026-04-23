package analysis

import (
	"context"
	"strings"
	"testing"
	"time"

	serverpkg "codeberg.org/pawal/gonemaster/server"
)

func TestControllerProjectRunUpdatesCohortMaterialization(t *testing.T) {
	finishedAt := time.Date(2026, 4, 17, 14, 0, 0, 0, time.UTC)
	run := testAnalysisRun("run-project", 100, "alpha.example", finishedAt, "192.0.2.10", "2001:db8::10")
	store := &fakeStore{
		runs: map[string]serverpkg.Run{run.ID: run},
		entries: map[string][]serverpkg.Entry{
			run.ID: testAnalysisEntries(run),
		},
		tags: map[int64][]string{
			run.DomainID: {"tld"},
		},
		batches: testAnalysisBatches(),
		cohorts: []serverpkg.AnalysisCohort{
			{
				ID:                    10,
				SourceType:            "tag",
				SourceTag:             "tld",
				Label:                 "TLD",
				AnalysisEnabled:       true,
				MaterializationStatus: serverpkg.AnalysisMaterializationPending,
			},
		},
	}

	controller := NewController(store)
	if err := controller.ProjectRun(run.ID); err != nil {
		t.Fatalf("ProjectRun: %v", err)
	}

	cohort, ok := store.GetAnalysisCohort(10)
	if !ok {
		t.Fatal("expected updated cohort")
	}
	if cohort.MaterializationStatus != serverpkg.AnalysisMaterializationReady {
		t.Fatalf("expected ready status, got %q", cohort.MaterializationStatus)
	}
	if !cohort.LastMaterializedAt.Equal(finishedAt) {
		t.Fatalf("expected last materialized at %s, got %s", finishedAt, cohort.LastMaterializedAt)
	}
	if cohort.LastMaterializationError != "" {
		t.Fatalf("expected empty materialization error, got %q", cohort.LastMaterializationError)
	}
}

func TestControllerRebuildCohortClearsStaleRowsAndBackfillsTaggedRuns(t *testing.T) {
	run1 := testAnalysisRun("run-rebuild-1", 100, "alpha.example", time.Date(2026, 4, 17, 10, 0, 0, 0, time.UTC), "192.0.2.10", "2001:db8::10")
	run2 := testAnalysisRun("run-rebuild-2", 101, "beta.example", time.Date(2026, 4, 17, 11, 0, 0, 0, time.UTC), "192.0.2.20", "2001:db8::20")
	runOther := testAnalysisRun("run-other", 102, "gamma.example", time.Date(2026, 4, 17, 12, 0, 0, 0, time.UTC), "192.0.2.30", "2001:db8::30")
	store := &fakeStore{
		runs: map[string]serverpkg.Run{
			run1.ID:     run1,
			run2.ID:     run2,
			runOther.ID: runOther,
		},
		entries: map[string][]serverpkg.Entry{
			run1.ID:     testAnalysisEntries(run1),
			run2.ID:     testAnalysisEntries(run2),
			runOther.ID: testAnalysisEntries(runOther),
		},
		tags: map[int64][]string{
			run1.DomainID:     {"tld"},
			run2.DomainID:     {"tld", "signed"},
			runOther.DomainID: {"gov"},
		},
		batches: testAnalysisBatches(),
		cohorts: []serverpkg.AnalysisCohort{
			{
				ID:                    10,
				SourceType:            "tag",
				SourceTag:             "tld",
				Label:                 "TLD",
				AnalysisEnabled:       true,
				MaterializationStatus: serverpkg.AnalysisMaterializationPending,
			},
			{
				ID:                    20,
				SourceType:            "tag",
				SourceTag:             "signed",
				Label:                 "Signed",
				AnalysisEnabled:       true,
				MaterializationStatus: serverpkg.AnalysisMaterializationPending,
			},
		},
		nsEndpoints: map[string][]serverpkg.AnalysisRunNameserverEndpoint{
			projectionKey(10, "stale-run"): {{CohortID: 10, RunID: "stale-run"}},
		},
		summaries: map[string]serverpkg.AnalysisRunDomainSummary{
			projectionKey(10, "stale-run"): {CohortID: 10, RunID: "stale-run"},
		},
		states: map[string]serverpkg.AnalysisProjectionState{
			projectionKey(10, "stale-run"): {CohortID: 10, RunID: "stale-run"},
		},
	}

	controller := NewController(store)
	if err := controller.RebuildCohort(context.Background(), 10); err != nil {
		t.Fatalf("RebuildCohort: %v", err)
	}

	if _, ok := store.nsEndpoints[projectionKey(10, "stale-run")]; ok {
		t.Fatal("expected stale nameserver endpoint rows to be cleared")
	}
	if _, ok := store.summaries[projectionKey(10, "stale-run")]; ok {
		t.Fatal("expected stale summary rows to be cleared")
	}
	if _, ok := store.states[projectionKey(10, "stale-run")]; ok {
		t.Fatal("expected stale projection state rows to be cleared")
	}

	if got := countProjectionKeysWithPrefix(store.summaries, "10/"); got != 2 {
		t.Fatalf("expected 2 rebuilt summaries for cohort 10, got %d", got)
	}
	if got := countProjectionKeysWithPrefix(store.summaries, "20/"); got != 1 {
		t.Fatalf("expected signed cohort to be updated for one overlapping run, got %d", got)
	}
	if _, ok := store.summaries[projectionKey(10, runOther.ID)]; ok {
		t.Fatal("unexpected summary for non-tagged run")
	}

	cohort, ok := store.GetAnalysisCohort(10)
	if !ok {
		t.Fatal("expected rebuilt cohort")
	}
	if cohort.MaterializationStatus != serverpkg.AnalysisMaterializationReady {
		t.Fatalf("expected ready status, got %q", cohort.MaterializationStatus)
	}
	if cohort.LastMaterializedAt.IsZero() {
		t.Fatal("expected non-zero LastMaterializedAt after rebuild")
	}

	other, ok := store.GetAnalysisCohort(20)
	if !ok {
		t.Fatal("expected overlapping cohort 20 to remain")
	}
	if other.MaterializationStatus != serverpkg.AnalysisMaterializationPending {
		t.Fatalf("rebuild of cohort 10 must not change cohort 20 status, got %q", other.MaterializationStatus)
	}
	if !other.LastMaterializedAt.IsZero() {
		t.Fatalf("rebuild of cohort 10 must not set cohort 20 LastMaterializedAt, got %s", other.LastMaterializedAt)
	}
}

func TestControllerRebuildCohortWithNoMatchingRunsLeavesLastMaterializedEmpty(t *testing.T) {
	store := &fakeStore{
		runs: map[string]serverpkg.Run{},
		cohorts: []serverpkg.AnalysisCohort{
			{
				ID:                    10,
				SourceType:            "tag",
				SourceTag:             "empty-cohort",
				Label:                 "Empty",
				AnalysisEnabled:       true,
				MaterializationStatus: serverpkg.AnalysisMaterializationPending,
			},
		},
	}

	controller := NewController(store)
	if err := controller.RebuildCohort(context.Background(), 10); err != nil {
		t.Fatalf("RebuildCohort: %v", err)
	}

	cohort, ok := store.GetAnalysisCohort(10)
	if !ok {
		t.Fatal("expected cohort 10")
	}
	if cohort.MaterializationStatus != serverpkg.AnalysisMaterializationReady {
		t.Fatalf("expected ready after empty rebuild, got %q", cohort.MaterializationStatus)
	}
	if !cohort.LastMaterializedAt.IsZero() {
		t.Fatalf("empty rebuild must leave LastMaterializedAt zero, got %s", cohort.LastMaterializedAt)
	}
}

func TestControllerRepairAllSkipsReadyCohortWithNoMissedRuns(t *testing.T) {
	finishedAt := time.Date(2026, 4, 17, 10, 0, 0, 0, time.UTC)
	run := testAnalysisRun("run-ready", 100, "alpha.example", finishedAt, "192.0.2.10", "2001:db8::10")
	// Pre-populate a marker row under a fabricated run ID. A full rebuild
	// would clear the cohort's materialization wholesale and wipe this
	// marker; a correct incremental repair leaves other runs' rows alone.
	markerKey := projectionKey(10, "pre-existing-run")
	store := &fakeStore{
		runs:    map[string]serverpkg.Run{run.ID: run},
		entries: map[string][]serverpkg.Entry{run.ID: testAnalysisEntries(run)},
		tags:    map[int64][]string{run.DomainID: {"tld"}},
		batches: testAnalysisBatches(),
		cohorts: []serverpkg.AnalysisCohort{
			{
				ID:                    10,
				SourceType:            "tag",
				SourceTag:             "tld",
				Label:                 "TLD",
				AnalysisEnabled:       true,
				MaterializationStatus: serverpkg.AnalysisMaterializationReady,
				LastMaterializedAt:    finishedAt,
			},
		},
		summaries: map[string]serverpkg.AnalysisRunDomainSummary{
			markerKey: {CohortID: 10, RunID: "pre-existing-run"},
		},
	}

	controller := NewController(store)
	if err := controller.RepairAllCohorts(context.Background()); err != nil {
		t.Fatalf("RepairAllCohorts: %v", err)
	}

	if _, ok := store.summaries[markerKey]; !ok {
		t.Fatal("expected pre-existing summary to be preserved (no rebuild should have run)")
	}
	if got := countProjectionKeysWithPrefix(store.summaries, "10/"); got != 1 {
		t.Fatalf("expected only the pre-existing summary, got %d", got)
	}

	cohort, ok := store.GetAnalysisCohort(10)
	if !ok {
		t.Fatal("expected cohort 10")
	}
	if !cohort.LastMaterializedAt.Equal(finishedAt) {
		t.Fatalf("expected LastMaterializedAt unchanged, got %s", cohort.LastMaterializedAt)
	}
	if cohort.MaterializationStatus != serverpkg.AnalysisMaterializationReady {
		t.Fatalf("expected status Ready, got %q", cohort.MaterializationStatus)
	}
}

func TestControllerRepairAllProjectsMissedRunsIncrementally(t *testing.T) {
	oldStamp := time.Date(2026, 4, 17, 10, 0, 0, 0, time.UTC)
	newFinishedAt := time.Date(2026, 4, 17, 12, 0, 0, 0, time.UTC)
	missedRun := testAnalysisRun("run-missed", 101, "beta.example", newFinishedAt, "192.0.2.20", "2001:db8::20")
	markerKey := projectionKey(10, "pre-existing-run")
	store := &fakeStore{
		runs:    map[string]serverpkg.Run{missedRun.ID: missedRun},
		entries: map[string][]serverpkg.Entry{missedRun.ID: testAnalysisEntries(missedRun)},
		tags:    map[int64][]string{missedRun.DomainID: {"tld"}},
		batches: testAnalysisBatches(),
		cohorts: []serverpkg.AnalysisCohort{
			{
				ID:                    10,
				SourceType:            "tag",
				SourceTag:             "tld",
				Label:                 "TLD",
				AnalysisEnabled:       true,
				MaterializationStatus: serverpkg.AnalysisMaterializationReady,
				LastMaterializedAt:    oldStamp,
			},
		},
		summaries: map[string]serverpkg.AnalysisRunDomainSummary{
			markerKey: {CohortID: 10, RunID: "pre-existing-run"},
		},
	}

	controller := NewController(store)
	if err := controller.RepairAllCohorts(context.Background()); err != nil {
		t.Fatalf("RepairAllCohorts: %v", err)
	}

	if _, ok := store.summaries[markerKey]; !ok {
		t.Fatal("expected pre-existing summary to be preserved (incremental repair must not clear other runs)")
	}
	if _, ok := store.summaries[projectionKey(10, missedRun.ID)]; !ok {
		t.Fatal("expected missed run to be projected into cohort")
	}

	cohort, ok := store.GetAnalysisCohort(10)
	if !ok {
		t.Fatal("expected cohort 10")
	}
	if !cohort.LastMaterializedAt.Equal(newFinishedAt) {
		t.Fatalf("expected LastMaterializedAt to advance to %s, got %s", newFinishedAt, cohort.LastMaterializedAt)
	}
	if cohort.MaterializationStatus != serverpkg.AnalysisMaterializationReady {
		t.Fatalf("expected status Ready, got %q", cohort.MaterializationStatus)
	}
}

func TestControllerRepairAllFailedCohortFallsBackToFullRebuild(t *testing.T) {
	finishedAt := time.Date(2026, 4, 17, 10, 0, 0, 0, time.UTC)
	run := testAnalysisRun("run-fresh", 100, "alpha.example", finishedAt, "192.0.2.10", "2001:db8::10")
	// A cohort stuck in Failed state should be reconciled from scratch —
	// its materialized rows may be partial or inconsistent.
	staleKey := projectionKey(10, "stale-run")
	store := &fakeStore{
		runs:    map[string]serverpkg.Run{run.ID: run},
		entries: map[string][]serverpkg.Entry{run.ID: testAnalysisEntries(run)},
		tags:    map[int64][]string{run.DomainID: {"tld"}},
		batches: testAnalysisBatches(),
		cohorts: []serverpkg.AnalysisCohort{
			{
				ID:                    10,
				SourceType:            "tag",
				SourceTag:             "tld",
				Label:                 "TLD",
				AnalysisEnabled:       true,
				MaterializationStatus: serverpkg.AnalysisMaterializationFailed,
			},
		},
		summaries: map[string]serverpkg.AnalysisRunDomainSummary{
			staleKey: {CohortID: 10, RunID: "stale-run"},
		},
	}

	controller := NewController(store)
	if err := controller.RepairAllCohorts(context.Background()); err != nil {
		t.Fatalf("RepairAllCohorts: %v", err)
	}

	if _, ok := store.summaries[staleKey]; ok {
		t.Fatal("expected Failed cohort to be cleared-and-rebuilt, removing stale rows")
	}
	if _, ok := store.summaries[projectionKey(10, run.ID)]; !ok {
		t.Fatal("expected run to be projected after full rebuild")
	}
}

func TestControllerRepairAllAndDisableChangeClearMaterializedRows(t *testing.T) {
	run := testAnalysisRun("run-repair", 100, "alpha.example", time.Date(2026, 4, 17, 9, 0, 0, 0, time.UTC), "192.0.2.10", "2001:db8::10")
	store := &fakeStore{
		runs: map[string]serverpkg.Run{
			run.ID: run,
		},
		entries: map[string][]serverpkg.Entry{
			run.ID: testAnalysisEntries(run),
		},
		tags: map[int64][]string{
			run.DomainID: {"tld"},
		},
		batches: testAnalysisBatches(),
		cohorts: []serverpkg.AnalysisCohort{
			{
				ID:                    10,
				SourceType:            "tag",
				SourceTag:             "tld",
				Label:                 "TLD",
				AnalysisEnabled:       true,
				MaterializationStatus: serverpkg.AnalysisMaterializationPending,
			},
			{
				ID:                    20,
				SourceType:            "tag",
				SourceTag:             "gov",
				Label:                 "Government",
				AnalysisEnabled:       false,
				MaterializationStatus: serverpkg.AnalysisMaterializationReady,
				LastMaterializedAt:    time.Date(2026, 4, 16, 9, 0, 0, 0, time.UTC),
			},
		},
		nsEndpoints: map[string][]serverpkg.AnalysisRunNameserverEndpoint{
			projectionKey(20, "stale-disabled"): {{CohortID: 20, RunID: "stale-disabled"}},
		},
		addrFacts: map[string][]serverpkg.AnalysisRunAddressASN{
			projectionKey(20, "stale-disabled"): {{CohortID: 20, RunID: "stale-disabled"}},
		},
		summaries: map[string]serverpkg.AnalysisRunDomainSummary{
			projectionKey(20, "stale-disabled"): {CohortID: 20, RunID: "stale-disabled"},
		},
		states: map[string]serverpkg.AnalysisProjectionState{
			projectionKey(20, "stale-disabled"): {CohortID: 20, RunID: "stale-disabled"},
		},
	}

	controller := NewController(store)
	if err := controller.RepairAllCohorts(context.Background()); err != nil {
		t.Fatalf("RepairAllCohorts: %v", err)
	}

	if got := countProjectionKeysWithPrefix(store.summaries, "10/"); got != 1 {
		t.Fatalf("expected enabled cohort to be rebuilt, got %d summaries", got)
	}
	if got := countProjectionKeysWithPrefix(store.summaries, "20/"); got != 0 {
		t.Fatalf("expected disabled cohort summaries cleared, got %d", got)
	}
	if hasProjectionPrefix(store.nsEndpoints, "20/") || hasProjectionPrefix(store.addrFacts, "20/") || hasProjectionPrefix(store.states, "20/") {
		t.Fatal("expected disabled cohort materialization to be fully cleared")
	}

	disabled, ok := store.GetAnalysisCohort(20)
	if !ok {
		t.Fatal("expected disabled cohort")
	}
	if disabled.MaterializationStatus != serverpkg.AnalysisMaterializationPending {
		t.Fatalf("expected disabled cohort to be pending after clear, got %q", disabled.MaterializationStatus)
	}
	if !disabled.LastMaterializedAt.IsZero() {
		t.Fatalf("expected cleared disabled cohort to reset last materialized at, got %s", disabled.LastMaterializedAt)
	}

	before := serverpkg.AnalysisCohort{
		ID:              10,
		SourceType:      "tag",
		SourceTag:       "tld",
		Label:           "TLD",
		AnalysisEnabled: true,
	}
	after := before
	after.AnalysisEnabled = false
	store.nsEndpoints[projectionKey(10, "disable-run")] = []serverpkg.AnalysisRunNameserverEndpoint{{CohortID: 10, RunID: "disable-run"}}
	store.summaries[projectionKey(10, "disable-run")] = serverpkg.AnalysisRunDomainSummary{CohortID: 10, RunID: "disable-run"}
	store.states[projectionKey(10, "disable-run")] = serverpkg.AnalysisProjectionState{CohortID: 10, RunID: "disable-run"}

	if err := controller.ReconcileCohortChange(context.Background(), before, after); err != nil {
		t.Fatalf("ReconcileCohortChange disable: %v", err)
	}
	if hasProjectionPrefix(store.nsEndpoints, "10/") || hasProjectionPrefix(store.summaries, "10/") || hasProjectionPrefix(store.states, "10/") {
		t.Fatal("expected disabled cohort change to clear cohort rows")
	}
}

// testAnalysisRunBatchID is the batch id every existing
// testAnalysisRun-seeded run is scoped under. Phase 2's pollution gates
// refuse to project a run with no batch_id or whose batch is not
// snapshot-intent, so the default test helper pins a single intent-true
// batch and buildBatches below wires it into the fakeStore.
const testAnalysisRunBatchID = "batch-test"

func testAnalysisRun(id string, domainID int64, domain string, finishedAt time.Time, ipv4Address, ipv6Address string) serverpkg.Run {
	score := 95
	grade := "A"
	return serverpkg.Run{
		ID:         id,
		DomainID:   domainID,
		Domain:     domain,
		BatchID:    testAnalysisRunBatchID,
		Status:     serverpkg.JobSucceeded,
		EntryCount: 2,
		FinishedAt: finishedAt,
		Score:      &score,
		Grade:      &grade,
		WorstLevel: "WARNING",
		NameserverTimings: []serverpkg.NameserverTiming{
			{Nameserver: "ns1." + domain, Address: ipv4Address, AvgMS: 11, MinMS: 10, MaxMS: 12, Count: 3},
			{Nameserver: "ns2." + domain, Address: ipv6Address, AvgMS: 18, MinMS: 17, MaxMS: 19, Count: 2},
		},
	}
}

// testAnalysisBatches returns a snapshot-intent batch catalog suitable for
// seeding fakeStore.batches in the ProjectRun / rebuild / repair tests.
func testAnalysisBatches() map[string]serverpkg.Batch {
	return map[string]serverpkg.Batch{
		testAnalysisRunBatchID: {
			ID:             testAnalysisRunBatchID,
			Tag:            "tld",
			CreatedAt:      time.Date(2026, 4, 17, 9, 0, 0, 0, time.UTC),
			DomainCount:    3,
			SnapshotIntent: true,
		},
	}
}

func testAnalysisEntries(run serverpkg.Run) []serverpkg.Entry {
	return []serverpkg.Entry{
		{
			RunID:    run.ID,
			DomainID: run.DomainID,
			Args: map[string]any{
				"address":  run.NameserverTimings[0].Address,
				"prefixes": []any{"192.0.2.0/24"},
				"asns":     []any{float64(64510), float64(64500)},
			},
		},
		{
			RunID:    run.ID,
			DomainID: run.DomainID,
			Args: map[string]any{
				"address":  run.NameserverTimings[1].Address,
				"prefixes": []any{"2001:db8::/32"},
				"asn":      int64(64520),
			},
		},
	}
}

func countProjectionKeysWithPrefix[V any](items map[string]V, prefix string) int {
	count := 0
	for key := range items {
		if strings.HasPrefix(key, prefix) {
			count++
		}
	}
	return count
}

func hasProjectionPrefix[V any](items map[string]V, prefix string) bool {
	return countProjectionKeysWithPrefix(items, prefix) > 0
}
