package server

import (
	"testing"
	"time"
)

// seedCohortForSnapshotTest creates a catalog row and returns its ID. Every
// snapshot test in this file needs at least one cohort to attach snapshots to.
func seedCohortForSnapshotTest(t *testing.T, s *SQLJobStore, sourceTag string) int64 {
	t.Helper()
	cohort, err := s.UpsertAnalysisCohort(AnalysisCohort{
		SourceType:      "tag",
		SourceTag:       sourceTag,
		Label:           sourceTag,
		AnalysisEnabled: true,
		PublicEnabled:   true,
	})
	if err != nil {
		t.Fatalf("seed cohort %q: %v", sourceTag, err)
	}
	return cohort.ID
}

func TestSQLJobStoreUpsertAnalysisCohortSnapshot(t *testing.T) {
	forEachBackend(t, func(t *testing.T, s *SQLJobStore) {
		cohortID := seedCohortForSnapshotTest(t, s, "tld")
		now := time.Now().UTC().Truncate(time.Microsecond)
		profileID := int64(42)

		created, err := s.UpsertAnalysisCohortSnapshot(AnalysisCohortSnapshot{
			CohortID:    cohortID,
			BatchID:     "batch-1",
			Slug:        "2026-04-20",
			Label:       "April 2026",
			Description: "Monthly cycle",
			ProfileID:   &profileID,
			ProfileName: "strict",
			Status:      AnalysisSnapshotStatusPending,
			IsPublic:    true,
			RunCount:    1,
			DomainCount: 1,
			FirstRunAt:  now,
			LastRunAt:   now,
			CreatedAt:   now,
			UpdatedAt:   now,
		})
		if err != nil {
			t.Fatalf("UpsertAnalysisCohortSnapshot insert: %v", err)
		}
		if created.ID == 0 {
			t.Fatal("expected non-zero ID for created snapshot")
		}
		if created.Status != AnalysisSnapshotStatusPending {
			t.Fatalf("Status: got %q want pending", created.Status)
		}
		if created.ProfileID == nil || *created.ProfileID != profileID {
			t.Fatalf("ProfileID not round-tripped: %+v", created.ProfileID)
		}
		if !created.IsPublic {
			t.Fatal("expected is_public=true after round trip")
		}
		if !created.FirstRunAt.Equal(now) {
			t.Fatalf("FirstRunAt: got %s want %s", created.FirstRunAt, now)
		}

		// Upsert with the same (cohort, batch) key must update in place and
		// preserve CreatedAt.
		capturedAt := now.Add(5 * time.Minute)
		updated, err := s.UpsertAnalysisCohortSnapshot(AnalysisCohortSnapshot{
			CohortID:    cohortID,
			BatchID:     "batch-1",
			Slug:        "2026-04-20",
			Label:       "April 2026 (captured)",
			Status:      AnalysisSnapshotStatusCaptured,
			IsPublic:    true,
			RunCount:    3,
			DomainCount: 2,
			FirstRunAt:  now,
			LastRunAt:   capturedAt,
			CapturedAt:  capturedAt,
			UpdatedAt:   capturedAt,
		})
		if err != nil {
			t.Fatalf("UpsertAnalysisCohortSnapshot update: %v", err)
		}
		if updated.ID != created.ID {
			t.Fatalf("update changed ID: got %d want %d", updated.ID, created.ID)
		}
		if !updated.CreatedAt.Equal(created.CreatedAt) {
			t.Fatalf("CreatedAt not preserved: created=%s updated=%s", created.CreatedAt, updated.CreatedAt)
		}
		if updated.Status != AnalysisSnapshotStatusCaptured {
			t.Fatalf("Status: got %q want captured", updated.Status)
		}
		if !updated.CapturedAt.Equal(capturedAt) {
			t.Fatalf("CapturedAt: got %s want %s", updated.CapturedAt, capturedAt)
		}
		if updated.RunCount != 3 || updated.DomainCount != 2 {
			t.Fatalf("counts: got run=%d domain=%d want 3/2", updated.RunCount, updated.DomainCount)
		}

		bySlug, ok := s.GetAnalysisCohortSnapshotBySlug(cohortID, "2026-04-20")
		if !ok {
			t.Fatal("GetAnalysisCohortSnapshotBySlug: not found")
		}
		if bySlug.ID != updated.ID {
			t.Fatalf("GetAnalysisCohortSnapshotBySlug ID: got %d want %d", bySlug.ID, updated.ID)
		}
	})
}

// Engine provenance has to survive the round trip like any other column, and
// the "unknown" case has to stay distinguishable from a real version string.
// An empty EngineVersion means we could not recover the provenance; it must
// never be silently filled in, so this asserts empty stays empty.
func TestSQLJobStoreAnalysisCohortSnapshotEngineVersionRoundTrip(t *testing.T) {
	forEachBackend(t, func(t *testing.T, s *SQLJobStore) {
		cohortID := seedCohortForSnapshotTest(t, s, "tld")

		stamped, err := s.UpsertAnalysisCohortSnapshot(AnalysisCohortSnapshot{
			CohortID:      cohortID,
			BatchID:       "batch-stamped",
			Slug:          "2026-08-15-stamped",
			Status:        AnalysisSnapshotStatusCaptured,
			EngineVersion: "v1.6.6",
		})
		if err != nil {
			t.Fatalf("upsert stamped: %v", err)
		}
		if stamped.EngineVersion != "v1.6.6" {
			t.Fatalf("EngineVersion: got %q want v1.6.6", stamped.EngineVersion)
		}
		if stamped.MixedEngineVersion {
			t.Fatal("MixedEngineVersion should default to false")
		}

		// A batch that spanned an upgrade keeps a sample version *and*
		// the mixed flag; the flag is what tells a reader the number is
		// confounded, so losing either half defeats the purpose.
		mixed, err := s.UpsertAnalysisCohortSnapshot(AnalysisCohortSnapshot{
			CohortID:           cohortID,
			BatchID:            "batch-mixed",
			Slug:               "2026-08-15-mixed",
			Status:             AnalysisSnapshotStatusCaptured,
			EngineVersion:      "v1.6.3",
			MixedEngineVersion: true,
		})
		if err != nil {
			t.Fatalf("upsert mixed: %v", err)
		}
		if mixed.EngineVersion != "v1.6.3" || !mixed.MixedEngineVersion {
			t.Fatalf("mixed snapshot: got version=%q mixed=%v want v1.6.3/true",
				mixed.EngineVersion, mixed.MixedEngineVersion)
		}

		// Pre-instrumentation snapshots carry no version at all.
		unknown, err := s.UpsertAnalysisCohortSnapshot(AnalysisCohortSnapshot{
			CohortID: cohortID,
			BatchID:  "batch-unknown",
			Slug:     "2026-05-05-unknown",
			Status:   AnalysisSnapshotStatusCaptured,
		})
		if err != nil {
			t.Fatalf("upsert unknown: %v", err)
		}
		if unknown.EngineVersion != "" {
			t.Fatalf("EngineVersion should stay empty when unknown, got %q", unknown.EngineVersion)
		}

		// Re-read through a different accessor to prove the columns are in
		// the shared column list, not just echoed back by the writer.
		reread, ok := s.GetAnalysisCohortSnapshotByBatch(cohortID, "batch-mixed")
		if !ok {
			t.Fatal("GetAnalysisCohortSnapshotByBatch: not found")
		}
		if reread.EngineVersion != "v1.6.3" || !reread.MixedEngineVersion {
			t.Fatalf("re-read: got version=%q mixed=%v want v1.6.3/true",
				reread.EngineVersion, reread.MixedEngineVersion)
		}
	})
}

func TestSQLJobStoreUpsertAnalysisCohortSnapshotValidates(t *testing.T) {
	forEachBackend(t, func(t *testing.T, s *SQLJobStore) {
		if _, err := s.UpsertAnalysisCohortSnapshot(AnalysisCohortSnapshot{}); err == nil {
			t.Fatal("expected validation error for empty snapshot")
		}
		if _, err := s.UpsertAnalysisCohortSnapshot(AnalysisCohortSnapshot{CohortID: 1}); err == nil {
			t.Fatal("expected validation error for missing batch_id")
		}
		if _, err := s.UpsertAnalysisCohortSnapshot(AnalysisCohortSnapshot{CohortID: 1, BatchID: "b"}); err == nil {
			t.Fatal("expected validation error for missing slug")
		}
	})
}

// TestSQLJobStoreSetAnalysisSnapshotMaterialization exercises the rematerialize
// progress setters that back the admin-UI progress bar. It walks the full
// lifecycle: pending (no timestamp) -> progress bump -> ready (timestamp
// stamped). It then asserts the subtle contract that calling the setter with a
// nil materializedAt (the pending/failed paths) leaves the previously stamped
// last_materialized_at intact, so a failed re-run still shows when the snapshot
// was last successfully rebuilt.
func TestSQLJobStoreSetAnalysisSnapshotMaterialization(t *testing.T) {
	forEachBackend(t, func(t *testing.T, s *SQLJobStore) {
		cohortID := seedCohortForSnapshotTest(t, s, "tld")
		created, err := s.UpsertAnalysisCohortSnapshot(AnalysisCohortSnapshot{
			CohortID: cohortID,
			BatchID:  "batch-1",
			Slug:     "2026-04-20",
			Status:   AnalysisSnapshotStatusCaptured,
			IsPublic: true,
		})
		if err != nil {
			t.Fatalf("seed snapshot: %v", err)
		}

		// Start: pending, no completion timestamp yet.
		if err := s.SetAnalysisSnapshotMaterialization(created.ID, AnalysisMaterializationPending, 0, 4, "", nil); err != nil {
			t.Fatalf("set pending: %v", err)
		}
		got, _ := s.GetAnalysisCohortSnapshot(created.ID)
		if got.MaterializationStatus != AnalysisMaterializationPending || got.MaterializationDone != 0 || got.MaterializationTotal != 4 {
			t.Fatalf("pending state = %q %d/%d, want pending 0/4",
				got.MaterializationStatus, got.MaterializationDone, got.MaterializationTotal)
		}
		if !got.LastMaterializedAt.IsZero() {
			t.Fatalf("last_materialized_at = %s, want zero while pending", got.LastMaterializedAt)
		}

		// Progress bump touches only the done counter.
		if err := s.SetAnalysisSnapshotMaterializationProgress(created.ID, 2); err != nil {
			t.Fatalf("bump progress: %v", err)
		}
		got, _ = s.GetAnalysisCohortSnapshot(created.ID)
		if got.MaterializationDone != 2 || got.MaterializationStatus != AnalysisMaterializationPending || got.MaterializationTotal != 4 {
			t.Fatalf("after bump = %q %d/%d, want pending 2/4",
				got.MaterializationStatus, got.MaterializationDone, got.MaterializationTotal)
		}

		// Ready stamps last_materialized_at.
		readyAt := time.Date(2026, 4, 21, 9, 30, 0, 0, time.UTC)
		if err := s.SetAnalysisSnapshotMaterialization(created.ID, AnalysisMaterializationReady, 4, 4, "", &readyAt); err != nil {
			t.Fatalf("set ready: %v", err)
		}
		got, _ = s.GetAnalysisCohortSnapshot(created.ID)
		if got.MaterializationStatus != AnalysisMaterializationReady || got.MaterializationDone != 4 {
			t.Fatalf("ready state = %q %d/%d, want ready 4/4",
				got.MaterializationStatus, got.MaterializationDone, got.MaterializationTotal)
		}
		if !got.LastMaterializedAt.Equal(readyAt) {
			t.Fatalf("last_materialized_at = %s, want %s", got.LastMaterializedAt, readyAt)
		}

		// A later failure (nil materializedAt) records the error but must
		// preserve the last successful materialization timestamp.
		if err := s.SetAnalysisSnapshotMaterialization(created.ID, AnalysisMaterializationFailed, 1, 4, "boom", nil); err != nil {
			t.Fatalf("set failed: %v", err)
		}
		got, _ = s.GetAnalysisCohortSnapshot(created.ID)
		if got.MaterializationStatus != AnalysisMaterializationFailed || got.LastMaterializationError != "boom" {
			t.Fatalf("failed state = %q err=%q, want failed err=boom",
				got.MaterializationStatus, got.LastMaterializationError)
		}
		if !got.LastMaterializedAt.Equal(readyAt) {
			t.Fatalf("last_materialized_at = %s after failure, want preserved %s", got.LastMaterializedAt, readyAt)
		}
	})
}

func TestSQLJobStoreAnalysisCohortSnapshotSlugUnique(t *testing.T) {
	forEachBackend(t, func(t *testing.T, s *SQLJobStore) {
		cohortID := seedCohortForSnapshotTest(t, s, "tld")

		if _, err := s.UpsertAnalysisCohortSnapshot(AnalysisCohortSnapshot{
			CohortID: cohortID,
			BatchID:  "batch-1",
			Slug:     "2026-04-20",
			Status:   AnalysisSnapshotStatusCaptured,
			IsPublic: true,
		}); err != nil {
			t.Fatalf("first snapshot: %v", err)
		}
		if _, err := s.UpsertAnalysisCohortSnapshot(AnalysisCohortSnapshot{
			CohortID: cohortID,
			BatchID:  "batch-2",
			Slug:     "2026-04-20",
			Status:   AnalysisSnapshotStatusCaptured,
			IsPublic: true,
		}); err == nil {
			t.Fatal("expected duplicate-slug error; got nil")
		}
	})
}

func TestSQLJobStoreListAnalysisCohortSnapshots(t *testing.T) {
	forEachBackend(t, func(t *testing.T, s *SQLJobStore) {
		cohortID := seedCohortForSnapshotTest(t, s, "tld")
		now := time.Now().UTC().Truncate(time.Microsecond)

		for i, slug := range []string{"2026-02-01", "2026-03-01", "2026-04-01"} {
			capturedAt := now.Add(time.Duration(i) * time.Hour)
			if _, err := s.UpsertAnalysisCohortSnapshot(AnalysisCohortSnapshot{
				CohortID:   cohortID,
				BatchID:    "batch-" + slug,
				Slug:       slug,
				Status:     AnalysisSnapshotStatusCaptured,
				IsPublic:   true,
				CapturedAt: capturedAt,
			}); err != nil {
				t.Fatalf("seed snapshot %s: %v", slug, err)
			}
		}

		// A second cohort's snapshots must not leak into the listing.
		otherCohortID := seedCohortForSnapshotTest(t, s, "gov")
		if _, err := s.UpsertAnalysisCohortSnapshot(AnalysisCohortSnapshot{
			CohortID:   otherCohortID,
			BatchID:    "batch-other",
			Slug:       "2026-04-01",
			Status:     AnalysisSnapshotStatusCaptured,
			IsPublic:   true,
			CapturedAt: now,
		}); err != nil {
			t.Fatalf("seed other cohort snapshot: %v", err)
		}

		list := s.ListAnalysisCohortSnapshots(cohortID)
		if len(list) != 3 {
			t.Fatalf("len list = %d, want 3", len(list))
		}
		if list[0].Slug != "2026-04-01" || list[2].Slug != "2026-02-01" {
			t.Fatalf("list order: got %q, %q, %q; want newest first",
				list[0].Slug, list[1].Slug, list[2].Slug)
		}
	})
}

func TestSQLJobStoreGetDefaultSnapshotForCohort(t *testing.T) {
	forEachBackend(t, func(t *testing.T, s *SQLJobStore) {
		cohortID := seedCohortForSnapshotTest(t, s, "tld")
		now := time.Now().UTC().Truncate(time.Microsecond)

		// No snapshots → no default.
		if _, ok := s.GetDefaultSnapshotForCohort(cohortID); ok {
			t.Fatal("expected no default snapshot when cohort is empty")
		}

		older, err := s.UpsertAnalysisCohortSnapshot(AnalysisCohortSnapshot{
			CohortID:   cohortID,
			BatchID:    "batch-old",
			Slug:       "2026-03-01",
			Status:     AnalysisSnapshotStatusCaptured,
			IsPublic:   true,
			CapturedAt: now,
		})
		if err != nil {
			t.Fatalf("older: %v", err)
		}
		newer, err := s.UpsertAnalysisCohortSnapshot(AnalysisCohortSnapshot{
			CohortID:   cohortID,
			BatchID:    "batch-new",
			Slug:       "2026-04-01",
			Status:     AnalysisSnapshotStatusCaptured,
			IsPublic:   true,
			CapturedAt: now.Add(time.Hour),
		})
		if err != nil {
			t.Fatalf("newer: %v", err)
		}

		// Default policy is auto_latest → newest captured+public wins.
		def, ok := s.GetDefaultSnapshotForCohort(cohortID)
		if !ok {
			t.Fatal("expected default under auto_latest")
		}
		if def.ID != newer.ID {
			t.Fatalf("auto_latest default: got id=%d want %d", def.ID, newer.ID)
		}

		// Non-public snapshots are excluded.
		if _, err := s.UpsertAnalysisCohortSnapshot(AnalysisCohortSnapshot{
			CohortID:   cohortID,
			BatchID:    "batch-private",
			Slug:       "2026-05-01-private",
			Status:     AnalysisSnapshotStatusCaptured,
			IsPublic:   false,
			CapturedAt: now.Add(2 * time.Hour),
		}); err != nil {
			t.Fatalf("private snapshot seed: %v", err)
		}
		def, ok = s.GetDefaultSnapshotForCohort(cohortID)
		if !ok || def.ID != newer.ID {
			t.Fatalf("auto_latest should skip private snapshots; got id=%d ok=%v", def.ID, ok)
		}

		// Pin to the older snapshot.
		pinnedID := older.ID
		if _, err := s.UpsertAnalysisCohort(AnalysisCohort{
			SourceType:            "tag",
			SourceTag:             "tld",
			DefaultSnapshotPolicy: DefaultSnapshotPolicyPinned,
			DefaultSnapshotID:     &pinnedID,
		}); err != nil {
			t.Fatalf("pin default: %v", err)
		}
		def, ok = s.GetDefaultSnapshotForCohort(cohortID)
		if !ok {
			t.Fatal("expected default under pinned policy")
		}
		if def.ID != older.ID {
			t.Fatalf("pinned default: got id=%d want %d", def.ID, older.ID)
		}

		// Dangling pin: pinned ID points at a snapshot that no longer
		// exists. Resolver must fall back to auto-latest so a stale
		// pointer left behind by a wipe does not hide every remaining
		// captured snapshot from the public path.
		ghostID := older.ID + newer.ID + 999
		if _, err := s.UpsertAnalysisCohort(AnalysisCohort{
			SourceType:            "tag",
			SourceTag:             "tld",
			DefaultSnapshotPolicy: DefaultSnapshotPolicyPinned,
			DefaultSnapshotID:     &ghostID,
		}); err != nil {
			t.Fatalf("dangling pin: %v", err)
		}
		def, ok = s.GetDefaultSnapshotForCohort(cohortID)
		if !ok {
			t.Fatal("dangling pin must fall back to auto-latest, not return false")
		}
		if def.ID != newer.ID {
			t.Fatalf("dangling pin fallback: got id=%d want %d (newest captured public)", def.ID, newer.ID)
		}
	})
}

// TestClearAnalysisCohortSnapshotsResetsPin pins down the regression that
// left an operator's cohort stuck on the no_snapshot empty state after
// rebuild: clearing the snapshot rows must also reset the cohort's
// pinned default_snapshot_id, otherwise GetDefaultSnapshotForCohort
// resolves to a row that no longer exists.
func TestClearAnalysisCohortSnapshotsResetsPin(t *testing.T) {
	forEachBackend(t, func(t *testing.T, s *SQLJobStore) {
		cohortID := seedCohortForSnapshotTest(t, s, "tld")
		now := time.Now().UTC().Truncate(time.Microsecond)

		snap, err := s.UpsertAnalysisCohortSnapshot(AnalysisCohortSnapshot{
			CohortID:   cohortID,
			BatchID:    "batch-pinned",
			Slug:       "2026-04-26",
			Status:     AnalysisSnapshotStatusCaptured,
			IsPublic:   true,
			CapturedAt: now,
		})
		if err != nil {
			t.Fatalf("seed snapshot: %v", err)
		}
		snapID := snap.ID
		if _, err := s.UpsertAnalysisCohort(AnalysisCohort{
			SourceType:            "tag",
			SourceTag:             "tld",
			DefaultSnapshotPolicy: DefaultSnapshotPolicyPinned,
			DefaultSnapshotID:     &snapID,
		}); err != nil {
			t.Fatalf("pin default: %v", err)
		}

		if err := s.ClearAnalysisCohortSnapshots(cohortID); err != nil {
			t.Fatalf("clear: %v", err)
		}

		cohort, ok := s.GetAnalysisCohort(cohortID)
		if !ok {
			t.Fatal("cohort should survive clear")
		}
		if cohort.DefaultSnapshotPolicy != DefaultSnapshotPolicyAutoLatest {
			t.Errorf("policy = %q, want auto_latest after clear", cohort.DefaultSnapshotPolicy)
		}
		if cohort.DefaultSnapshotID != nil {
			t.Errorf("DefaultSnapshotID = %d, want nil after clear", *cohort.DefaultSnapshotID)
		}
	})
}

// TestSQLJobStoreSetCohortDefaultSnapshot covers the shared pin helper used by
// both the manual "Set default" handler and the promote-on-capture path: it
// sets is_default on the target, clears every sibling, and flips the catalog to
// the pinned policy. Re-pinning a different snapshot moves the badge; re-pinning
// the same snapshot is a no-op.
func TestSQLJobStoreSetCohortDefaultSnapshot(t *testing.T) {
	forEachBackend(t, func(t *testing.T, s *SQLJobStore) {
		cohortID := seedCohortForSnapshotTest(t, s, "tld")
		now := time.Now().UTC().Truncate(time.Microsecond)

		snapA, err := s.UpsertAnalysisCohortSnapshot(AnalysisCohortSnapshot{
			CohortID: cohortID, BatchID: "batch-a", Slug: "2026-04-20-a",
			Status: AnalysisSnapshotStatusCaptured, IsPublic: true, CapturedAt: now,
		})
		if err != nil {
			t.Fatalf("seed snapshot A: %v", err)
		}
		snapB, err := s.UpsertAnalysisCohortSnapshot(AnalysisCohortSnapshot{
			CohortID: cohortID, BatchID: "batch-b", Slug: "2026-04-21-b",
			Status: AnalysisSnapshotStatusCaptured, IsPublic: true, CapturedAt: now,
		})
		if err != nil {
			t.Fatalf("seed snapshot B: %v", err)
		}

		assertPin := func(want int64, wantDefault, wantOther int64) {
			t.Helper()
			cohort, _ := s.GetAnalysisCohort(cohortID)
			if cohort.DefaultSnapshotPolicy != DefaultSnapshotPolicyPinned {
				t.Fatalf("policy = %q, want pinned", cohort.DefaultSnapshotPolicy)
			}
			if cohort.DefaultSnapshotID == nil || *cohort.DefaultSnapshotID != want {
				t.Fatalf("default_snapshot_id = %+v, want %d", cohort.DefaultSnapshotID, want)
			}
			def, _ := s.GetAnalysisCohortSnapshot(wantDefault)
			if !def.IsDefault {
				t.Fatalf("snapshot %d should be is_default", wantDefault)
			}
			other, _ := s.GetAnalysisCohortSnapshot(wantOther)
			if other.IsDefault {
				t.Fatalf("snapshot %d should not be is_default", wantOther)
			}
		}

		if err := s.SetCohortDefaultSnapshot(cohortID, snapA.ID); err != nil {
			t.Fatalf("pin A: %v", err)
		}
		assertPin(snapA.ID, snapA.ID, snapB.ID)

		// Moving the pin flips is_default off A and onto B.
		if err := s.SetCohortDefaultSnapshot(cohortID, snapB.ID); err != nil {
			t.Fatalf("pin B: %v", err)
		}
		assertPin(snapB.ID, snapB.ID, snapA.ID)

		// Re-pinning the already-default snapshot is idempotent.
		if err := s.SetCohortDefaultSnapshot(cohortID, snapB.ID); err != nil {
			t.Fatalf("re-pin B: %v", err)
		}
		assertPin(snapB.ID, snapB.ID, snapA.ID)
	})
}

func TestSQLJobStoreReplaceSnapshotOverview(t *testing.T) {
	forEachBackend(t, func(t *testing.T, s *SQLJobStore) {
		cohortID := seedCohortForSnapshotTest(t, s, "tld")
		snap, err := s.UpsertAnalysisCohortSnapshot(AnalysisCohortSnapshot{
			CohortID: cohortID,
			BatchID:  "batch-1",
			Slug:     "2026-04-20",
			Status:   AnalysisSnapshotStatusCaptured,
			IsPublic: true,
		})
		if err != nil {
			t.Fatalf("seed snapshot: %v", err)
		}

		initial := SnapshotOverviewV2{
			Totals: SnapshotOverviewTotals{DomainCount: 2},
			FactDistributions: map[string]PublicAnalysisFactDistribution{
				FactCategoryGrade: {
					Category: FactCategoryGrade,
					Buckets:  []PublicAnalysisFactBucket{{Key: "A", Count: 1}},
				},
				FactCategoryDNSSECPosture: {
					Category: FactCategoryDNSSECPosture,
					Buckets:  []PublicAnalysisFactBucket{{Key: FactKeyNSEC3, Count: 1}},
				},
				FactCategorySeverity: {
					Category: FactCategorySeverity,
					Buckets:  []PublicAnalysisFactBucket{{Key: "OK", Count: 2}},
				},
			},
		}
		if err := s.ReplaceSnapshotOverview(snap.ID, initial); err != nil {
			t.Fatalf("Replace initial: %v", err)
		}
		got, ok := s.GetSnapshotOverview(snap.ID)
		if !ok {
			t.Fatal("expected overview row after replace")
		}
		if got.Totals.DomainCount != 2 {
			t.Fatalf("DomainCount = %d, want 2", got.Totals.DomainCount)
		}
		grade := bucketsToCounts(got.FactDistributions[FactCategoryGrade].Buckets)
		posture := bucketsToCounts(got.FactDistributions[FactCategoryDNSSECPosture].Buckets)
		if grade["A"] != 1 || posture[FactKeyNSEC3] != 1 {
			t.Fatalf("payload mismatch: %+v", got)
		}

		// Replacement must atomically swap.
		replacement := SnapshotOverviewV2{
			FactDistributions: map[string]PublicAnalysisFactDistribution{
				FactCategoryDNSSECPosture: {
					Category: FactCategoryDNSSECPosture,
					Buckets:  []PublicAnalysisFactBucket{{Key: FactKeyNSEC3, Count: 2}},
				},
			},
		}
		if err := s.ReplaceSnapshotOverview(snap.ID, replacement); err != nil {
			t.Fatalf("Replace second: %v", err)
		}
		got, ok = s.GetSnapshotOverview(snap.ID)
		if !ok {
			t.Fatal("expected overview row after second replace")
		}
		postureAfter := bucketsToCounts(got.FactDistributions[FactCategoryDNSSECPosture].Buckets)
		if postureAfter[FactKeyNSEC3] != 2 {
			t.Fatalf("replacement payload not stored: %+v", got)
		}
		if _, leaked := got.FactDistributions[FactCategoryGrade]; leaked {
			t.Fatalf("old grade distribution leaked into replacement: %+v", got)
		}
	})
}

func TestSQLJobStoreBatchSnapshotIntentRoundTrip(t *testing.T) {
	forEachBackend(t, func(t *testing.T, s *SQLJobStore) {
		now := time.Now().UTC().Truncate(time.Microsecond)

		if err := s.CreateBatch(Batch{
			ID:             "batch-intent",
			Tag:            "tld",
			CreatedAt:      now,
			DomainCount:    10,
			Description:    "monthly",
			SnapshotIntent: true,
		}); err != nil {
			t.Fatalf("CreateBatch intent: %v", err)
		}
		got, ok := s.GetBatch("batch-intent")
		if !ok {
			t.Fatal("GetBatch intent: not found")
		}
		if !got.SnapshotIntent {
			t.Fatalf("SnapshotIntent round trip: got false, want true (%+v)", got)
		}

		if err := s.CreateBatch(Batch{
			ID:          "batch-no-intent",
			Tag:         "tld",
			CreatedAt:   now,
			DomainCount: 2,
		}); err != nil {
			t.Fatalf("CreateBatch no intent: %v", err)
		}
		got, ok = s.GetBatch("batch-no-intent")
		if !ok {
			t.Fatal("GetBatch no-intent: not found")
		}
		if got.SnapshotIntent {
			t.Fatalf("SnapshotIntent default: got true, want false (%+v)", got)
		}
	})
}

// testSnapshotVocabulary is a two-module test_levels table as capture stores it.
const testSnapshotVocabulary = `{"DNSSEC":{"DS02_NO_MATCHING_DNSKEY_RRSIG":"ERROR"},"ZONE":{"Z15_NO_CAA":"NOTICE"}}`

func TestSQLJobStoreSnapshotCarriesVocabularyProvenance(t *testing.T) {
	forEachBackend(t, func(t *testing.T, s *SQLJobStore) {
		cohortID := seedCohortForSnapshotTest(t, s, "tld")
		now := time.Now().UTC().Truncate(time.Microsecond)

		created, err := s.UpsertAnalysisCohortSnapshot(AnalysisCohortSnapshot{
			CohortID:          cohortID,
			BatchID:           "batch-vocab",
			Slug:              "2026-09-18",
			Status:            AnalysisSnapshotStatusCaptured,
			IsPublic:          true,
			Vocabulary:        testSnapshotVocabulary,
			ScoringConfigHash: "default",
			CapturedAt:        now,
		})
		if err != nil {
			t.Fatalf("insert: %v", err)
		}
		if created.Vocabulary != testSnapshotVocabulary {
			t.Fatalf("Vocabulary: got %q", created.Vocabulary)
		}
		if created.ScoringConfigHash != "default" {
			t.Fatalf("ScoringConfigHash: got %q want default", created.ScoringConfigHash)
		}

		read, ok := s.GetAnalysisCohortSnapshot(created.ID)
		if !ok {
			t.Fatal("expected the snapshot to be readable")
		}
		if read.Vocabulary != testSnapshotVocabulary || read.ScoringConfigHash != "default" {
			t.Fatalf("read back: vocabulary=%q hash=%q", read.Vocabulary, read.ScoringConfigHash)
		}

		updated, err := s.UpsertAnalysisCohortSnapshot(AnalysisCohortSnapshot{
			CohortID:          cohortID,
			BatchID:           "batch-vocab",
			Slug:              "2026-09-18",
			Status:            AnalysisSnapshotStatusCaptured,
			Vocabulary:        testSnapshotVocabulary,
			ScoringConfigHash: "9f2c",
		})
		if err != nil {
			t.Fatalf("update: %v", err)
		}
		if updated.Vocabulary != testSnapshotVocabulary || updated.ScoringConfigHash != "9f2c" {
			t.Fatalf("update: vocabulary=%q hash=%q", updated.Vocabulary, updated.ScoringConfigHash)
		}
	})
}

// The vocabulary is provenance stored on the snapshot row, so it has to
// outlive the runs it was read from.
func TestSQLJobStoreSnapshotVocabularySurvivesRunPurge(t *testing.T) {
	forEachBackend(t, func(t *testing.T, s *SQLJobStore) {
		cohortID := seedCohortForSnapshotTest(t, s, "tld")
		old := time.Now().UTC().Add(-48 * time.Hour).Truncate(time.Microsecond)

		job := createAndGraduate(t, s, Job{
			ID: "job-purged", Domain: "alpha.example", BatchID: "batch-purge",
			Status: JobQueued, CreatedAt: old,
			EffectiveProfile: `{"test_levels":{"ZONE":{"Z15_NO_CAA":"NOTICE"}}}`,
		}, nil)

		snap, err := s.UpsertAnalysisCohortSnapshot(AnalysisCohortSnapshot{
			CohortID:   cohortID,
			BatchID:    "batch-purge",
			Slug:       "2026-09-18-purge",
			Status:     AnalysisSnapshotStatusCaptured,
			IsPublic:   true,
			Vocabulary: testSnapshotVocabulary,
		})
		if err != nil {
			t.Fatalf("insert snapshot: %v", err)
		}

		if _, err := s.PurgeOlderThan(time.Now().UTC().Add(-24 * time.Hour)); err != nil {
			t.Fatalf("purge: %v", err)
		}
		if _, ok := s.GetRun(job.ID); ok {
			t.Fatal("expected the run to be purged")
		}
		read, ok := s.GetAnalysisCohortSnapshot(snap.ID)
		if !ok {
			t.Fatal("expected the snapshot to survive the purge")
		}
		if read.Vocabulary != testSnapshotVocabulary {
			t.Fatalf("Vocabulary after purge: got %q", read.Vocabulary)
		}
	})
}

// A rebuild restamps captured_at, so the batch run window orders the series.
func TestSQLJobStoreSnapshotOrderFollowsRunWindow(t *testing.T) {
	forEachBackend(t, func(t *testing.T, s *SQLJobStore) {
		cohortID := seedCohortForSnapshotTest(t, s, "tld")
		rebuiltAt := time.Date(2026, 9, 18, 21, 23, 4, 0, time.UTC)

		if _, err := s.UpsertAnalysisCohortSnapshot(AnalysisCohortSnapshot{
			CohortID:   cohortID,
			BatchID:    "batch-june",
			Slug:       "2026-06-03",
			Status:     AnalysisSnapshotStatusCaptured,
			IsPublic:   true,
			CapturedAt: rebuiltAt.Add(time.Second),
			FirstRunAt: time.Date(2026, 6, 3, 8, 55, 0, 0, time.UTC),
			LastRunAt:  time.Date(2026, 6, 3, 8, 59, 0, 0, time.UTC),
		}); err != nil {
			t.Fatalf("seed june snapshot: %v", err)
		}
		september, err := s.UpsertAnalysisCohortSnapshot(AnalysisCohortSnapshot{
			CohortID:   cohortID,
			BatchID:    "batch-september",
			Slug:       "2026-09-18",
			Status:     AnalysisSnapshotStatusCaptured,
			IsPublic:   true,
			CapturedAt: rebuiltAt,
			FirstRunAt: time.Date(2026, 9, 18, 9, 22, 0, 0, time.UTC),
			LastRunAt:  time.Date(2026, 9, 18, 9, 28, 0, 0, time.UTC),
		})
		if err != nil {
			t.Fatalf("seed september snapshot: %v", err)
		}

		list := s.ListAnalysisCohortSnapshots(cohortID)
		if len(list) != 2 {
			t.Fatalf("len list = %d, want 2", len(list))
		}
		if list[0].Slug != "2026-09-18" {
			t.Errorf("list order: got %q first, want 2026-09-18", list[0].Slug)
		}

		def, ok := s.GetDefaultSnapshotForCohort(cohortID)
		if !ok {
			t.Fatal("expected a default snapshot")
		}
		if def.ID != september.ID {
			t.Errorf("auto_latest default = %q, want 2026-09-18", def.Slug)
		}
	})
}
