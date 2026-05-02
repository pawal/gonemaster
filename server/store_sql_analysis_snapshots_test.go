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
	for _, b := range testBackends(t) {
		t.Run(b.name, func(t *testing.T) {
			s := testStoreForBackend(t, b)
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
}

func TestSQLJobStoreUpsertAnalysisCohortSnapshotValidates(t *testing.T) {
	s := testStoreForBackend(t, testBackends(t)[0])
	if _, err := s.UpsertAnalysisCohortSnapshot(AnalysisCohortSnapshot{}); err == nil {
		t.Fatal("expected validation error for empty snapshot")
	}
	if _, err := s.UpsertAnalysisCohortSnapshot(AnalysisCohortSnapshot{CohortID: 1}); err == nil {
		t.Fatal("expected validation error for missing batch_id")
	}
	if _, err := s.UpsertAnalysisCohortSnapshot(AnalysisCohortSnapshot{CohortID: 1, BatchID: "b"}); err == nil {
		t.Fatal("expected validation error for missing slug")
	}
}

func TestSQLJobStoreAnalysisCohortSnapshotSlugUnique(t *testing.T) {
	s := testStoreForBackend(t, testBackends(t)[0])
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
}

func TestSQLJobStoreListAnalysisCohortSnapshots(t *testing.T) {
	for _, b := range testBackends(t) {
		t.Run(b.name, func(t *testing.T) {
			s := testStoreForBackend(t, b)
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
}

func TestSQLJobStoreGetDefaultSnapshotForCohort(t *testing.T) {
	for _, b := range testBackends(t) {
		t.Run(b.name, func(t *testing.T) {
			s := testStoreForBackend(t, b)
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
}

// TestClearAnalysisCohortSnapshotsResetsPin pins down the regression that
// left an operator's cohort stuck on the no_snapshot empty state after
// rebuild: clearing the snapshot rows must also reset the cohort's
// pinned default_snapshot_id, otherwise GetDefaultSnapshotForCohort
// resolves to a row that no longer exists.
func TestClearAnalysisCohortSnapshotsResetsPin(t *testing.T) {
	for _, b := range testBackends(t) {
		t.Run(b.name, func(t *testing.T) {
			s := testStoreForBackend(t, b)
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
}

func TestSQLJobStoreReplaceSnapshotOverview(t *testing.T) {
	for _, b := range testBackends(t) {
		t.Run(b.name, func(t *testing.T) {
			s := testStoreForBackend(t, b)
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
}

func TestSQLJobStoreBatchSnapshotIntentRoundTrip(t *testing.T) {
	for _, b := range testBackends(t) {
		t.Run(b.name, func(t *testing.T) {
			s := testStoreForBackend(t, b)
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
}
