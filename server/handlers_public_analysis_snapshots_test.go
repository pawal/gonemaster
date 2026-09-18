package server

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

// TestPublicAnalysisCohortDetailNoSnapshotState covers the empty
// cohort case: a cohort with no captured public snapshot returns
// status = "no_snapshot" so the UI can render a helpful panel.
func TestPublicAnalysisCohortDetailNoSnapshotState(t *testing.T) {
	forEachAnalysisFixture(t, func(t *testing.T, f *analysisFixture) {
		if _, err := f.store.UpsertAnalysisCohort(AnalysisCohort{
			SourceType:      "tag",
			SourceTag:       "tld",
			Label:           "TLD",
			AnalysisEnabled: true,
			PublicEnabled:   true,
			IsDefault:       true,
		}); err != nil {
			t.Fatalf("upsert cohort: %v", err)
		}

		resp := getPublic(t, f.srv, "/pub/api/v1/analysis/cohorts/tld")
		detail := mustJSON[PublicAnalysisCohortDetail](t, resp, http.StatusOK)
		if detail.Status != PublicAnalysisStatusNoSnapshot {
			t.Fatalf("Status = %q, want no_snapshot", detail.Status)
		}
		if detail.Snapshot != nil {
			t.Fatalf("expected Snapshot nil on no_snapshot response, got %+v", detail.Snapshot)
		}
	}, asDefaultCohort(), withoutSnapshot())
}

// TestPublicAnalysisCatalogCarriesDefaultSnapshot checks that the catalog
// exposes default_snapshot + snapshot_count for each selectable cohort.
func TestPublicAnalysisCatalogCarriesDefaultSnapshot(t *testing.T) {
	forEachAnalysisAPIFixture(t, func(t *testing.T, f *analysisFixture) {
		resp := getPublic(t, f.srv, "/pub/api/v1/analysis/catalog")
		payload := mustJSON[PublicAnalysisCatalogResponse](t, resp, http.StatusOK)
		if len(payload.Cohorts) != 1 {
			t.Fatalf("expected 1 cohort, got %d", len(payload.Cohorts))
		}
		view := payload.Cohorts[0]
		if view.DefaultSnapshot == nil {
			t.Fatal("expected default_snapshot to be present")
		}
		if view.DefaultSnapshot.Slug != f.snapshot.Slug {
			t.Fatalf("default_snapshot slug = %q, want %q", view.DefaultSnapshot.Slug, f.snapshot.Slug)
		}
		if view.SnapshotCount != 1 {
			t.Fatalf("snapshot_count = %d, want 1", view.SnapshotCount)
		}
	})
}

// TestPublicAnalysisSnapshotsListReturnsCapturedOnly pins the visibility
// rules: captured public snapshots surface, pending / retired /
// mixed-profile / non-public snapshots do not.
func TestPublicAnalysisSnapshotsListReturnsCapturedOnly(t *testing.T) {
	forEachAnalysisAPIFixture(t, func(t *testing.T, f *analysisFixture) {
		// Retired snapshot - must be hidden.
		if _, err := f.store.UpsertAnalysisCohortSnapshot(AnalysisCohortSnapshot{
			CohortID: f.cohort.ID,
			BatchID:  "batch-retired",
			Slug:     "2026-03-01-retired",
			Status:   AnalysisSnapshotStatusRetired,
			IsPublic: true,
		}); err != nil {
			t.Fatalf("seed retired: %v", err)
		}
		// Mixed-profile snapshot - must be hidden.
		if _, err := f.store.UpsertAnalysisCohortSnapshot(AnalysisCohortSnapshot{
			CohortID: f.cohort.ID,
			BatchID:  "batch-mixed",
			Slug:     "2026-03-15-mixed",
			Status:   AnalysisSnapshotStatusFailedMixedProfiles,
			IsPublic: false,
		}); err != nil {
			t.Fatalf("seed mixed: %v", err)
		}
		// Pending (not yet captured) snapshot - must be hidden.
		if _, err := f.store.UpsertAnalysisCohortSnapshot(AnalysisCohortSnapshot{
			CohortID: f.cohort.ID,
			BatchID:  "batch-pending",
			Slug:     "2026-04-25-pending",
			Status:   AnalysisSnapshotStatusPending,
			IsPublic: true,
		}); err != nil {
			t.Fatalf("seed pending: %v", err)
		}
		// Captured but is_public=0 - must be hidden.
		if _, err := f.store.UpsertAnalysisCohortSnapshot(AnalysisCohortSnapshot{
			CohortID:   f.cohort.ID,
			BatchID:    "batch-private",
			Slug:       "2026-04-05-private",
			Status:     AnalysisSnapshotStatusCaptured,
			IsPublic:   false,
			CapturedAt: time.Date(2026, 4, 5, 12, 0, 0, 0, time.UTC),
		}); err != nil {
			t.Fatalf("seed private: %v", err)
		}

		resp := getPublic(t, f.srv, "/pub/api/v1/analysis/cohorts/tld/snapshots")
		payload := mustJSON[PublicAnalysisSnapshotListResponse](t, resp, http.StatusOK)
		if len(payload.Snapshots) != 1 {
			t.Fatalf("expected 1 public snapshot, got %d: %+v", len(payload.Snapshots), payload.Snapshots)
		}
		if payload.Snapshots[0].Slug != f.snapshot.Slug {
			t.Fatalf("slug = %q, want %q", payload.Snapshots[0].Slug, f.snapshot.Slug)
		}
		if !payload.Snapshots[0].IsDefault {
			t.Fatal("expected fixture snapshot to be flagged is_default")
		}
		if payload.Snapshots[0].LastRunAt.IsZero() {
			t.Fatal("expected snapshot list to carry source run timing")
		}
	})
}

func TestPublicAnalysisTrendsUseSourceRunOrderAndMetadata(t *testing.T) {
	forEachAnalysisAPIFixture(t, func(t *testing.T, f *analysisFixture) {
		newSource := time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC)
		oldSource := time.Date(2026, 3, 26, 12, 0, 0, 0, time.UTC)

		f.snapshot.CapturedAt = time.Date(2026, 4, 24, 18, 14, 0, 0, time.UTC)
		f.snapshot.FirstRunAt = newSource
		f.snapshot.LastRunAt = newSource
		updated, err := f.store.UpsertAnalysisCohortSnapshot(f.snapshot)
		if err != nil {
			t.Fatalf("update fixture snapshot: %v", err)
		}
		f.snapshot = updated
		older := f.seedAlternateSnapshot("batch-old-source", "2026-03-26-old", time.Date(2026, 4, 24, 18, 19, 30, 0, time.UTC))
		older.FirstRunAt = oldSource
		older.LastRunAt = oldSource
		older, err = f.store.UpsertAnalysisCohortSnapshot(older)
		if err != nil {
			t.Fatalf("update older snapshot source time: %v", err)
		}

		for _, snap := range []AnalysisCohortSnapshot{f.snapshot, older} {
			if err := f.store.ReplaceSnapshotOverview(snap.ID, SnapshotOverviewV2{
				FactDistributions: map[string]PublicAnalysisFactDistribution{
					FactCategorySeverity: {
						Category: FactCategorySeverity,
						Buckets:  []PublicAnalysisFactBucket{{Key: "OK", Count: 1}},
					},
				},
			}); err != nil {
				t.Fatalf("seed overview for %s: %v", snap.Slug, err)
			}
		}

		resp := getPublic(t, f.srv, "/pub/api/v1/analysis/cohorts/tld/trends?category="+FactCategorySeverity)
		payload := mustJSON[PublicAnalysisTrendResponse](t, resp, http.StatusOK)
		if len(payload.Points) != 2 {
			t.Fatalf("expected 2 trend points, got %d: %+v", len(payload.Points), payload.Points)
		}
		if payload.Points[0].Slug != older.Slug || payload.Points[1].Slug != f.snapshot.Slug {
			t.Fatalf("trend order = [%s, %s], want source-date order [%s, %s]",
				payload.Points[0].Slug, payload.Points[1].Slug, older.Slug, f.snapshot.Slug)
		}
		if !payload.Points[0].LastRunAt.Equal(oldSource) {
			t.Fatalf("older point last_run_at = %s, want %s", payload.Points[0].LastRunAt, oldSource)
		}
		if payload.Points[1].Label != f.snapshot.Label {
			t.Fatalf("trend point label = %q, want %q", payload.Points[1].Label, f.snapshot.Label)
		}
	})
}

// TestPublicAnalysisTrendsCarryKeyMeta confirms /trends ships display
// metadata for every key seen, so the UI does not duplicate the
// factCategoryDisplays registry.
func TestPublicAnalysisTrendsCarryKeyMeta(t *testing.T) {
	forEachAnalysisAPIFixture(t, func(t *testing.T, f *analysisFixture) {
		if err := f.store.ReplaceSnapshotOverview(f.snapshot.ID, SnapshotOverviewV2{
			FactDistributions: map[string]PublicAnalysisFactDistribution{
				FactCategoryDNSKEYAlgorithm: {
					Category: FactCategoryDNSKEYAlgorithm,
					Buckets: []PublicAnalysisFactBucket{
						{Key: "8", Count: 3},
						{Key: "13", Count: 5},
					},
				},
			},
		}); err != nil {
			t.Fatalf("seed overview: %v", err)
		}

		resp := getPublic(t, f.srv, "/pub/api/v1/analysis/cohorts/tld/trends?category="+FactCategoryDNSKEYAlgorithm)
		payload := mustJSON[PublicAnalysisTrendResponse](t, resp, http.StatusOK)
		got8, ok := payload.KeyMeta["8"]
		if !ok {
			t.Fatalf("key_meta missing key %q: %+v", "8", payload.KeyMeta)
		}
		if got8.Label != "RSASHA256" {
			t.Fatalf("key_meta[8].label = %q, want %q", got8.Label, "RSASHA256")
		}
		if got8.Tone != "notice" {
			t.Fatalf("key_meta[8].tone = %q, want %q", got8.Tone, "notice")
		}
		if got8.Order != 8 {
			t.Fatalf("key_meta[8].order = %d, want %d", got8.Order, 8)
		}
		got13, ok := payload.KeyMeta["13"]
		if !ok {
			t.Fatalf("key_meta missing key %q: %+v", "13", payload.KeyMeta)
		}
		if got13.Label != "ECDSAP256SHA256" {
			t.Fatalf("key_meta[13].label = %q, want %q", got13.Label, "ECDSAP256SHA256")
		}
		if got13.Tone != "ok" {
			t.Fatalf("key_meta[13].tone = %q, want %q", got13.Tone, "ok")
		}
	})
}

// TestPublicAnalysisSnapshotDetailHiddenForRetired verifies that an
// explicit slug lookup of a retired or mixed-profile snapshot returns
// 404 - retired snapshots must not leak through the public path.
func TestPublicAnalysisSnapshotDetailHiddenForRetired(t *testing.T) {
	forEachAnalysisAPIFixture(t, func(t *testing.T, f *analysisFixture) {
		if _, err := f.store.UpsertAnalysisCohortSnapshot(AnalysisCohortSnapshot{
			CohortID: f.cohort.ID,
			BatchID:  "batch-retired",
			Slug:     "2026-03-01-retired",
			Status:   AnalysisSnapshotStatusRetired,
			IsPublic: true,
		}); err != nil {
			t.Fatalf("seed retired: %v", err)
		}
		resp := getPublic(t, f.srv, "/pub/api/v1/analysis/cohorts/tld/snapshots/2026-03-01-retired")
		wantStatus(t, resp, http.StatusNotFound)

		resp = getPublic(t, f.srv, f.publicURLForSnapshot("2026-03-01-retired", "overview"))
		wantStatus(t, resp, http.StatusNotFound)
	})
}

// TestPublicAnalysisSnapshotDetailHiddenForMixedProfile pins the mixed
// profile hiding rule on the detail endpoint.
func TestPublicAnalysisSnapshotDetailHiddenForMixedProfile(t *testing.T) {
	forEachAnalysisAPIFixture(t, func(t *testing.T, f *analysisFixture) {
		if _, err := f.store.UpsertAnalysisCohortSnapshot(AnalysisCohortSnapshot{
			CohortID: f.cohort.ID,
			BatchID:  "batch-mixed",
			Slug:     "2026-03-15-mixed",
			Status:   AnalysisSnapshotStatusFailedMixedProfiles,
			IsPublic: false,
		}); err != nil {
			t.Fatalf("seed mixed: %v", err)
		}
		resp := getPublic(t, f.srv, "/pub/api/v1/analysis/cohorts/tld/snapshots/2026-03-15-mixed")
		wantStatus(t, resp, http.StatusNotFound)
	})
}

// TestPublicAnalysisOverviewSlugInPath verifies that the overview
// handler returns the snapshot pinned by the URL path.
func TestPublicAnalysisOverviewSlugInPath(t *testing.T) {
	forEachAnalysisAPIFixture(t, func(t *testing.T, f *analysisFixture) {
		older := f.seedAlternateSnapshot("batch-old", "2026-04-01-older", time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC))

		resp := getPublic(t, f.srv, f.publicURL("overview"))
		payload := mustJSON[PublicAnalysisOverviewResponse](t, resp, http.StatusOK)
		if payload.Snapshot == nil || payload.Snapshot.Slug != f.snapshot.Slug {
			t.Fatalf("fixture slug: got %+v, want fixture", payload.Snapshot)
		}

		resp = getPublic(t, f.srv, f.publicURLForSnapshot("2026-04-01-older", "overview"))
		payload = mustJSON[PublicAnalysisOverviewResponse](t, resp, http.StatusOK)
		if payload.Snapshot == nil || payload.Snapshot.Slug != older.Slug {
			t.Fatalf("older slug: got %+v, want %q", payload.Snapshot, older.Slug)
		}
	})
}

// The public API reports whether the vocabulary is known, never the
// vocabulary itself, so a reader can tell a classifiable pair from one the
// report can only mark unknown.
func TestPublicAnalysisSnapshotReportsProvenanceAvailability(t *testing.T) {
	forEachAnalysisAPIFixture(t, func(t *testing.T, f *analysisFixture) {
		listPath := "/pub/api/v1/analysis/cohorts/tld/snapshots"
		detailPath := listPath + "/" + f.snapshot.Slug

		list := mustJSON[PublicAnalysisSnapshotListResponse](t, getPublic(t, f.srv, listPath), http.StatusOK)
		if len(list.Snapshots) != 1 {
			t.Fatalf("expected 1 public snapshot, got %d", len(list.Snapshots))
		}
		if list.Snapshots[0].VocabularyAvailable {
			t.Fatal("an unstamped snapshot must report vocabulary_available false")
		}
		detail := mustJSON[PublicAnalysisSnapshotDetail](t, getPublic(t, f.srv, detailPath), http.StatusOK)
		if detail.VocabularyAvailable || detail.ScoringConfigHash != "" {
			t.Fatalf("unstamped detail: available=%v hash=%q", detail.VocabularyAvailable, detail.ScoringConfigHash)
		}

		stamped := f.snapshot
		stamped.Vocabulary = `{"ZONE":{"Z15_NO_CAA":"NOTICE"}}`
		stamped.ScoringConfigHash = "default"
		if _, err := f.store.UpsertAnalysisCohortSnapshot(stamped); err != nil {
			t.Fatalf("stamp provenance: %v", err)
		}

		list = mustJSON[PublicAnalysisSnapshotListResponse](t, getPublic(t, f.srv, listPath), http.StatusOK)
		if !list.Snapshots[0].VocabularyAvailable {
			t.Fatal("a stamped snapshot must report vocabulary_available true")
		}
		if list.Snapshots[0].ScoringConfigHash != "default" {
			t.Fatalf("list scoring_config_hash = %q, want default", list.Snapshots[0].ScoringConfigHash)
		}
		detail = mustJSON[PublicAnalysisSnapshotDetail](t, getPublic(t, f.srv, detailPath), http.StatusOK)
		if !detail.VocabularyAvailable || detail.ScoringConfigHash != "default" {
			t.Fatalf("stamped detail: available=%v hash=%q", detail.VocabularyAvailable, detail.ScoringConfigHash)
		}

		// The blob itself stays server side.
		if strings.Contains(getPublic(t, f.srv, detailPath).Body.String(), "Z15_NO_CAA") {
			t.Fatal("the vocabulary blob must not reach the public response")
		}
	})
}
