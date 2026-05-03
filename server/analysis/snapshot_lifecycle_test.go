package analysis

import (
	"context"
	"strings"
	"testing"
	"time"

	serverpkg "codeberg.org/pawal/gonemaster/server"
)

// seedSnapshotBatch registers a batch with the given id and snapshot-intent
// flag in the fakeStore. Used by every test in this file.
func seedSnapshotBatch(store *fakeStore, batchID string, snapshotIntent bool) {
	store.ensureSnapshotMaps()
	store.batches[batchID] = serverpkg.Batch{
		ID:             batchID,
		Tag:            "tld",
		CreatedAt:      time.Date(2026, 4, 20, 9, 0, 0, 0, time.UTC),
		DomainCount:    2,
		SnapshotIntent: snapshotIntent,
	}
}

func snapshotLifecycleStore(t *testing.T) (*fakeStore, serverpkg.AnalysisCohort) {
	t.Helper()
	cohort := serverpkg.AnalysisCohort{
		ID:                    10,
		SourceType:            "tag",
		SourceTag:             "tld",
		Label:                 "TLD",
		AnalysisEnabled:       true,
		MaterializationStatus: serverpkg.AnalysisMaterializationPending,
	}
	store := &fakeStore{
		runs:    map[string]serverpkg.Run{},
		entries: map[string][]serverpkg.Entry{},
		tags:    map[int64][]string{},
		cohorts: []serverpkg.AnalysisCohort{cohort},
	}
	store.ensureSnapshotMaps()
	return store, cohort
}

func TestDefaultSnapshotSlugUsesFullBatchIDHash(t *testing.T) {
	createdAt := time.Date(2026, 4, 24, 15, 30, 0, 0, time.UTC)
	first := defaultSnapshotSlug(serverpkg.Batch{
		ID:        "batch_1777044617680247000_5",
		CreatedAt: createdAt,
	})
	second := defaultSnapshotSlug(serverpkg.Batch{
		ID:        "batch_1777044617685621000_8",
		CreatedAt: createdAt,
	})

	for _, slug := range []string{first, second} {
		if !strings.HasPrefix(slug, "2026-04-24-") {
			t.Fatalf("slug %q does not carry the expected date prefix", slug)
		}
		if len(slug) != len("2026-04-24-")+12 {
			t.Fatalf("slug %q has length %d, want %d", slug, len(slug), len("2026-04-24-")+12)
		}
	}
	if first == second {
		t.Fatalf("same-day batch IDs produced the same slug %q", first)
	}
}

// TestControllerProjectRunSkipsEmptyBatchID covers the plan's first
// pollution gate: a graduated run with no batch_id (e.g. a public-UI
// one-off) must not enter the analysis layer.
func TestControllerProjectRunSkipsEmptyBatchID(t *testing.T) {
	store, _ := snapshotLifecycleStore(t)
	run := testAnalysisRun("run-public", 100, "alpha.example", time.Now().UTC(), "192.0.2.10", "2001:db8::10")
	run.BatchID = ""
	store.runs[run.ID] = run
	store.entries[run.ID] = testAnalysisEntries(run)
	store.tags[run.DomainID] = []string{"tld"}

	controller := NewController(store)
	if err := controller.ProjectRun(run.ID); err != nil {
		t.Fatalf("ProjectRun: %v", err)
	}
	if _, ok := store.summaries[projectionKey(10, run.ID)]; ok {
		t.Fatal("expected no summary row for batch-less run")
	}
	if len(store.snapshots) != 0 {
		t.Fatalf("expected no snapshot rows, got %d", len(store.snapshots))
	}
}

// TestControllerProjectRunSkipsNonSnapshotIntentBatch covers the second
// pollution gate: a batched run whose batch has snapshot_intent = false is
// projected nowhere in the analysis layer - the per-run tables stay empty
// and no snapshot row is created.
func TestControllerProjectRunSkipsNonSnapshotIntentBatch(t *testing.T) {
	store, _ := snapshotLifecycleStore(t)
	seedSnapshotBatch(store, "batch-retest", false)
	run := testAnalysisRun("run-retest", 100, "alpha.example", time.Now().UTC(), "192.0.2.10", "2001:db8::10")
	run.BatchID = "batch-retest"
	store.runs[run.ID] = run
	store.entries[run.ID] = testAnalysisEntries(run)
	store.tags[run.DomainID] = []string{"tld"}

	controller := NewController(store)
	if err := controller.ProjectRun(run.ID); err != nil {
		t.Fatalf("ProjectRun: %v", err)
	}
	if _, ok := store.summaries[projectionKey(10, run.ID)]; ok {
		t.Fatal("expected no summary row for non-intent batch")
	}
	if _, ok := store.states[projectionKey(10, run.ID)]; ok {
		t.Fatal("expected no projection state row for non-intent batch")
	}
	if len(store.snapshots) != 0 {
		t.Fatalf("expected no snapshot rows, got %d", len(store.snapshots))
	}
}

// TestControllerProjectRunCreatesPendingSnapshot covers the happy path:
// a batched run with snapshot_intent = true produces a pending snapshot
// per matching cohort, with counters and denormalized profile copied from
// the run.
func TestControllerProjectRunCreatesPendingSnapshot(t *testing.T) {
	store, _ := snapshotLifecycleStore(t)
	seedSnapshotBatch(store, "batch-tld", true)
	profileID := int64(7)
	run := testAnalysisRun("run-1", 100, "alpha.example", time.Date(2026, 4, 20, 10, 0, 0, 0, time.UTC), "192.0.2.10", "2001:db8::10")
	run.BatchID = "batch-tld"
	run.ProfileID = &profileID
	run.ProfileName = "strict"
	store.runs[run.ID] = run
	store.entries[run.ID] = testAnalysisEntries(run)
	store.tags[run.DomainID] = []string{"tld"}

	controller := NewController(store)
	if err := controller.ProjectRun(run.ID); err != nil {
		t.Fatalf("ProjectRun: %v", err)
	}

	snap, ok := store.GetAnalysisCohortSnapshotByBatch(10, "batch-tld")
	if !ok {
		t.Fatal("expected pending snapshot for cohort=10 batch=batch-tld")
	}
	if snap.Status != serverpkg.AnalysisSnapshotStatusPending {
		t.Fatalf("Status = %q, want pending", snap.Status)
	}
	if !snap.IsPublic {
		t.Fatal("expected pending snapshot to default is_public=true")
	}
	if snap.Slug == "" || snap.Slug[0] != '2' {
		t.Fatalf("Slug = %q, want YYYY-MM-DD prefix", snap.Slug)
	}
	if snap.ProfileID == nil || *snap.ProfileID != profileID || snap.ProfileName != "strict" {
		t.Fatalf("denormalized profile not captured: %+v %+v", snap.ProfileID, snap.ProfileName)
	}
	if snap.RunCount != 1 || snap.DomainCount != 1 {
		t.Fatalf("counters = run:%d domain:%d, want 1/1", snap.RunCount, snap.DomainCount)
	}
}

// TestControllerProjectRunFlagsMixedProfiles covers the plan's
// mixed-profile case: a second run in the same batch carrying a different
// profile flips the snapshot to failed_mixed_profiles and drops is_public.
func TestControllerProjectRunFlagsMixedProfiles(t *testing.T) {
	store, _ := snapshotLifecycleStore(t)
	seedSnapshotBatch(store, "batch-mixed", true)

	profileA := int64(1)
	runA := testAnalysisRun("run-a", 100, "alpha.example", time.Date(2026, 4, 20, 10, 0, 0, 0, time.UTC), "192.0.2.10", "2001:db8::10")
	runA.BatchID = "batch-mixed"
	runA.ProfileID = &profileA
	runA.ProfileName = "strict"
	store.runs[runA.ID] = runA
	store.entries[runA.ID] = testAnalysisEntries(runA)
	store.tags[runA.DomainID] = []string{"tld"}

	profileB := int64(2)
	runB := testAnalysisRun("run-b", 101, "beta.example", time.Date(2026, 4, 20, 10, 5, 0, 0, time.UTC), "192.0.2.20", "2001:db8::20")
	runB.BatchID = "batch-mixed"
	runB.ProfileID = &profileB
	runB.ProfileName = "lenient"
	store.runs[runB.ID] = runB
	store.entries[runB.ID] = testAnalysisEntries(runB)
	store.tags[runB.DomainID] = []string{"tld"}

	controller := NewController(store)
	if err := controller.ProjectRun(runA.ID); err != nil {
		t.Fatalf("ProjectRun A: %v", err)
	}
	if err := controller.ProjectRun(runB.ID); err != nil {
		t.Fatalf("ProjectRun B: %v", err)
	}

	snap, ok := store.GetAnalysisCohortSnapshotByBatch(10, "batch-mixed")
	if !ok {
		t.Fatal("expected snapshot")
	}
	if snap.Status != serverpkg.AnalysisSnapshotStatusFailedMixedProfiles {
		t.Fatalf("Status = %q, want failed_mixed_profiles", snap.Status)
	}
	if snap.IsPublic {
		t.Fatal("mixed-profile snapshot must be hidden from public path")
	}
}

// TestControllerCaptureCompletedSnapshotsPromotesOnBatchDrain covers the
// polling promotion path: a pending snapshot whose batch has zero
// outstanding jobs is promoted to captured with aggregates written.
func TestControllerCaptureCompletedSnapshotsPromotesOnBatchDrain(t *testing.T) {
	store, _ := snapshotLifecycleStore(t)
	seedSnapshotBatch(store, "batch-drain", true)

	run := testAnalysisRun("run-drain", 100, "alpha.example", time.Date(2026, 4, 20, 10, 0, 0, 0, time.UTC), "192.0.2.10", "2001:db8::10")
	run.BatchID = "batch-drain"
	store.runs[run.ID] = run
	store.entries[run.ID] = testAnalysisEntries(run)
	store.tags[run.DomainID] = []string{"tld"}

	// Mark the batch as having one outstanding job - capture should skip.
	store.queuedJobs["batch-drain"] = []string{"job-pending"}

	controller := NewController(store)
	if err := controller.ProjectRun(run.ID); err != nil {
		t.Fatalf("ProjectRun: %v", err)
	}
	if err := controller.CaptureCompletedSnapshots(context.Background()); err != nil {
		t.Fatalf("CaptureCompletedSnapshots: %v", err)
	}
	snap, ok := store.GetAnalysisCohortSnapshotByBatch(10, "batch-drain")
	if !ok {
		t.Fatal("expected snapshot")
	}
	if snap.Status != serverpkg.AnalysisSnapshotStatusPending {
		t.Fatalf("premature capture: status = %q", snap.Status)
	}

	// Drain the batch and run the capture pass again.
	delete(store.queuedJobs, "batch-drain")
	if err := controller.CaptureCompletedSnapshots(context.Background()); err != nil {
		t.Fatalf("CaptureCompletedSnapshots (drained): %v", err)
	}
	snap, _ = store.GetAnalysisCohortSnapshotByBatch(10, "batch-drain")
	if snap.Status != serverpkg.AnalysisSnapshotStatusCaptured {
		t.Fatalf("Status = %q, want captured", snap.Status)
	}
	if snap.CapturedAt.IsZero() {
		t.Fatal("expected CapturedAt to be stamped on promotion")
	}
	if _, ok := store.snapshotOverviews[snap.ID]; !ok {
		t.Fatal("expected overview row to be written on capture")
	}
	if _, ok := store.snapshotViews[snap.ID]; !ok {
		t.Fatal("expected entity-view replace to be called on capture")
	}
}

func TestControllerCaptureWaitsForProjectionDrain(t *testing.T) {
	store, _ := snapshotLifecycleStore(t)
	seedSnapshotBatch(store, "batch-projecting", true)

	runA := testAnalysisRun("run-projected", 100, "alpha.example", time.Date(2026, 4, 20, 10, 0, 0, 0, time.UTC), "192.0.2.10", "2001:db8::10")
	runA.BatchID = "batch-projecting"
	store.runs[runA.ID] = runA
	store.entries[runA.ID] = testAnalysisEntries(runA)
	store.tags[runA.DomainID] = []string{"tld"}

	runB := testAnalysisRun("run-waiting", 101, "bravo.example", time.Date(2026, 4, 20, 10, 1, 0, 0, time.UTC), "192.0.2.11", "2001:db8::11")
	runB.BatchID = "batch-projecting"
	store.runs[runB.ID] = runB
	store.entries[runB.ID] = testAnalysisEntries(runB)
	store.tags[runB.DomainID] = []string{"tld"}

	controller := NewController(store)
	if err := controller.ProjectRun(runA.ID); err != nil {
		t.Fatalf("ProjectRun A: %v", err)
	}
	if err := controller.CaptureCompletedSnapshots(context.Background()); err != nil {
		t.Fatalf("CaptureCompletedSnapshots: %v", err)
	}
	snap, ok := store.GetAnalysisCohortSnapshotByBatch(10, "batch-projecting")
	if !ok {
		t.Fatal("expected snapshot")
	}
	if snap.Status != serverpkg.AnalysisSnapshotStatusPending {
		t.Fatalf("capture before projection drain: status = %q", snap.Status)
	}

	if err := controller.ProjectRun(runB.ID); err != nil {
		t.Fatalf("ProjectRun B: %v", err)
	}
	if err := controller.CaptureCompletedSnapshots(context.Background()); err != nil {
		t.Fatalf("CaptureCompletedSnapshots after projection drain: %v", err)
	}
	snap, _ = store.GetAnalysisCohortSnapshotByBatch(10, "batch-projecting")
	if snap.Status != serverpkg.AnalysisSnapshotStatusCaptured {
		t.Fatalf("Status = %q, want captured", snap.Status)
	}
	if snap.RunCount != 2 || snap.DomainCount != 2 {
		t.Fatalf("snapshot counts = %d/%d, want 2/2", snap.RunCount, snap.DomainCount)
	}
}

// TestControllerCaptureSkipsMixedProfileSnapshot verifies a broken
// (mixed-profile) snapshot is not promoted to captured even once its
// batch has drained - flipping it to captured would hide the error from
// the admin.
func TestControllerCaptureSkipsMixedProfileSnapshot(t *testing.T) {
	store, _ := snapshotLifecycleStore(t)
	seedSnapshotBatch(store, "batch-mixed", true)

	profileA := int64(1)
	runA := testAnalysisRun("run-a", 100, "alpha.example", time.Date(2026, 4, 20, 10, 0, 0, 0, time.UTC), "192.0.2.10", "2001:db8::10")
	runA.BatchID = "batch-mixed"
	runA.ProfileID = &profileA
	runA.ProfileName = "strict"
	store.runs[runA.ID] = runA
	store.entries[runA.ID] = testAnalysisEntries(runA)
	store.tags[runA.DomainID] = []string{"tld"}

	profileB := int64(2)
	runB := testAnalysisRun("run-b", 101, "beta.example", time.Date(2026, 4, 20, 10, 5, 0, 0, time.UTC), "192.0.2.20", "2001:db8::20")
	runB.BatchID = "batch-mixed"
	runB.ProfileID = &profileB
	runB.ProfileName = "lenient"
	store.runs[runB.ID] = runB
	store.entries[runB.ID] = testAnalysisEntries(runB)
	store.tags[runB.DomainID] = []string{"tld"}

	controller := NewController(store)
	if err := controller.ProjectRun(runA.ID); err != nil {
		t.Fatalf("ProjectRun A: %v", err)
	}
	if err := controller.ProjectRun(runB.ID); err != nil {
		t.Fatalf("ProjectRun B: %v", err)
	}
	if err := controller.CaptureCompletedSnapshots(context.Background()); err != nil {
		t.Fatalf("CaptureCompletedSnapshots: %v", err)
	}
	snap, _ := store.GetAnalysisCohortSnapshotByBatch(10, "batch-mixed")
	if snap.Status != serverpkg.AnalysisSnapshotStatusFailedMixedProfiles {
		t.Fatalf("expected status to stay failed_mixed_profiles, got %q", snap.Status)
	}
}

// TestControllerProjectRunTracksConcurrentBatchesPerCohort covers the
// concurrent-batches case: two batches against the same cohort produce
// two distinct pending snapshots keyed on (cohort, batch).
func TestControllerProjectRunTracksConcurrentBatchesPerCohort(t *testing.T) {
	store, _ := snapshotLifecycleStore(t)
	seedSnapshotBatch(store, "batch-alpha", true)
	seedSnapshotBatch(store, "batch-bravo", true)

	run1 := testAnalysisRun("run-1", 100, "alpha.example", time.Date(2026, 4, 20, 10, 0, 0, 0, time.UTC), "192.0.2.10", "2001:db8::10")
	run1.BatchID = "batch-alpha"
	run2 := testAnalysisRun("run-2", 101, "beta.example", time.Date(2026, 4, 20, 10, 5, 0, 0, time.UTC), "192.0.2.20", "2001:db8::20")
	run2.BatchID = "batch-bravo"

	store.runs[run1.ID] = run1
	store.runs[run2.ID] = run2
	store.entries[run1.ID] = testAnalysisEntries(run1)
	store.entries[run2.ID] = testAnalysisEntries(run2)
	store.tags[run1.DomainID] = []string{"tld"}
	store.tags[run2.DomainID] = []string{"tld"}

	controller := NewController(store)
	if err := controller.ProjectRun(run1.ID); err != nil {
		t.Fatalf("ProjectRun 1: %v", err)
	}
	if err := controller.ProjectRun(run2.ID); err != nil {
		t.Fatalf("ProjectRun 2: %v", err)
	}
	if len(store.snapshots) != 2 {
		t.Fatalf("expected 2 snapshots, got %d", len(store.snapshots))
	}
	if _, ok := store.GetAnalysisCohortSnapshotByBatch(10, "batch-alpha"); !ok {
		t.Fatal("missing snapshot for batch-alpha")
	}
	if _, ok := store.GetAnalysisCohortSnapshotByBatch(10, "batch-bravo"); !ok {
		t.Fatal("missing snapshot for batch-bravo")
	}
}

// TestControllerRebuildCohortRegeneratesSnapshotsPerBatch covers rebuild
// idempotence: rebuilding a cohort clears stale snapshot rows and
// regenerates one snapshot per snapshot-intent batch.
func TestControllerRebuildCohortRegeneratesSnapshotsPerBatch(t *testing.T) {
	store, _ := snapshotLifecycleStore(t)
	seedSnapshotBatch(store, "batch-alpha", true)
	seedSnapshotBatch(store, "batch-bravo", true)

	run1 := testAnalysisRun("run-1", 100, "alpha.example", time.Date(2026, 4, 20, 10, 0, 0, 0, time.UTC), "192.0.2.10", "2001:db8::10")
	run1.BatchID = "batch-alpha"
	run2 := testAnalysisRun("run-2", 101, "beta.example", time.Date(2026, 4, 20, 11, 0, 0, 0, time.UTC), "192.0.2.20", "2001:db8::20")
	run2.BatchID = "batch-bravo"
	store.runs[run1.ID] = run1
	store.runs[run2.ID] = run2
	store.entries[run1.ID] = testAnalysisEntries(run1)
	store.entries[run2.ID] = testAnalysisEntries(run2)
	store.tags[run1.DomainID] = []string{"tld"}
	store.tags[run2.DomainID] = []string{"tld"}

	// Seed a stale snapshot that rebuild must clear before re-materializing.
	if _, err := store.UpsertAnalysisCohortSnapshot(serverpkg.AnalysisCohortSnapshot{
		CohortID: 10,
		BatchID:  "batch-orphan",
		Slug:     "stale",
		Status:   serverpkg.AnalysisSnapshotStatusCaptured,
	}); err != nil {
		t.Fatalf("seed stale snapshot: %v", err)
	}

	controller := NewController(store)
	if err := controller.RebuildCohort(context.Background(), 10); err != nil {
		t.Fatalf("RebuildCohort: %v", err)
	}

	if _, ok := store.GetAnalysisCohortSnapshotByBatch(10, "batch-orphan"); ok {
		t.Fatal("expected stale snapshot to be cleared by rebuild")
	}
	if _, ok := store.GetAnalysisCohortSnapshotByBatch(10, "batch-alpha"); !ok {
		t.Fatal("missing snapshot for batch-alpha after rebuild")
	}
	if _, ok := store.GetAnalysisCohortSnapshotByBatch(10, "batch-bravo"); !ok {
		t.Fatal("missing snapshot for batch-bravo after rebuild")
	}

	// Running rebuild a second time must still end with exactly the two
	// batch-keyed snapshots - the (cohort, batch) natural key is
	// idempotent.
	if err := controller.RebuildCohort(context.Background(), 10); err != nil {
		t.Fatalf("RebuildCohort 2: %v", err)
	}
	snapshotsForCohort := 0
	for key := range store.snapshots {
		if key.cohortID == 10 {
			snapshotsForCohort++
		}
	}
	if snapshotsForCohort != 2 {
		t.Fatalf("expected 2 snapshots after idempotent rebuild, got %d", snapshotsForCohort)
	}
}

// TestControllerRebuildCohortSkipsNonIntentBatches confirms that a
// rebuild encountering a batch whose snapshot_intent = false never
// materializes facts or snapshot rows for that batch.
func TestControllerRebuildCohortSkipsNonIntentBatches(t *testing.T) {
	store, _ := snapshotLifecycleStore(t)
	seedSnapshotBatch(store, "batch-intent", true)
	seedSnapshotBatch(store, "batch-retest", false)

	run1 := testAnalysisRun("run-intent", 100, "alpha.example", time.Date(2026, 4, 20, 10, 0, 0, 0, time.UTC), "192.0.2.10", "2001:db8::10")
	run1.BatchID = "batch-intent"
	run2 := testAnalysisRun("run-retest", 101, "beta.example", time.Date(2026, 4, 20, 11, 0, 0, 0, time.UTC), "192.0.2.20", "2001:db8::20")
	run2.BatchID = "batch-retest"
	store.runs[run1.ID] = run1
	store.runs[run2.ID] = run2
	store.entries[run1.ID] = testAnalysisEntries(run1)
	store.entries[run2.ID] = testAnalysisEntries(run2)
	store.tags[run1.DomainID] = []string{"tld"}
	store.tags[run2.DomainID] = []string{"tld"}

	controller := NewController(store)
	if err := controller.RebuildCohort(context.Background(), 10); err != nil {
		t.Fatalf("RebuildCohort: %v", err)
	}

	if _, ok := store.GetAnalysisCohortSnapshotByBatch(10, "batch-intent"); !ok {
		t.Fatal("missing snapshot for snapshot-intent batch")
	}
	if _, ok := store.GetAnalysisCohortSnapshotByBatch(10, "batch-retest"); ok {
		t.Fatal("rebuild must not create a snapshot for a non-intent batch")
	}
	if _, ok := store.summaries[projectionKey(10, run2.ID)]; ok {
		t.Fatal("rebuild must not project a non-intent batch into cohort summaries")
	}
}

// TestControllerRebuildCohortFlagsMixedProfilesAcrossRuns confirms that
// the deferred-snapshot reconciliation pass still detects mixed
// profiles even though it sees only a sample run per (cohort, batch)
// instead of every run.
func TestControllerRebuildCohortFlagsMixedProfilesAcrossRuns(t *testing.T) {
	store, _ := snapshotLifecycleStore(t)
	seedSnapshotBatch(store, "batch-mixed", true)

	profileA := int64(1)
	runA := testAnalysisRun("run-a", 100, "alpha.example", time.Date(2026, 4, 20, 10, 0, 0, 0, time.UTC), "192.0.2.10", "2001:db8::10")
	runA.BatchID = "batch-mixed"
	runA.ProfileID = &profileA
	runA.ProfileName = "strict"
	store.runs[runA.ID] = runA
	store.entries[runA.ID] = testAnalysisEntries(runA)
	store.tags[runA.DomainID] = []string{"tld"}

	profileB := int64(2)
	runB := testAnalysisRun("run-b", 101, "beta.example", time.Date(2026, 4, 20, 10, 5, 0, 0, time.UTC), "192.0.2.20", "2001:db8::20")
	runB.BatchID = "batch-mixed"
	runB.ProfileID = &profileB
	runB.ProfileName = "lenient"
	store.runs[runB.ID] = runB
	store.entries[runB.ID] = testAnalysisEntries(runB)
	store.tags[runB.DomainID] = []string{"tld"}

	controller := NewController(store)
	if err := controller.RebuildCohort(context.Background(), 10); err != nil {
		t.Fatalf("RebuildCohort: %v", err)
	}

	snap, ok := store.GetAnalysisCohortSnapshotByBatch(10, "batch-mixed")
	if !ok {
		t.Fatal("expected snapshot for mixed-profile rebuild")
	}
	if snap.Status != serverpkg.AnalysisSnapshotStatusFailedMixedProfiles {
		t.Fatalf("Status = %q, want failed_mixed_profiles", snap.Status)
	}
	if snap.IsPublic {
		t.Fatal("mixed-profile snapshot must be hidden from public path")
	}
}
