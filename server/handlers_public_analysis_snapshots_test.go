package server

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"
)

// TestPublicAnalysisCohortDetailNoSnapshotState covers the empty
// cohort case: a cohort with no captured public snapshot returns
// status = "no_snapshot" so the UI can render a helpful panel.
func TestPublicAnalysisCohortDetailNoSnapshotState(t *testing.T) {
	db, store, srv := newIsolatedAnalysisTestServer(t)
	_ = db
	if _, err := store.UpsertAnalysisCohort(AnalysisCohort{
		SourceType:      "tag",
		SourceTag:       "tld",
		Label:           "TLD",
		AnalysisEnabled: true,
		PublicEnabled:   true,
		IsDefault:       true,
	}); err != nil {
		t.Fatalf("upsert cohort: %v", err)
	}

	resp := getPublic(t, srv, "/pub/api/v1/analysis/cohorts/tld")
	if resp.Code != http.StatusOK {
		t.Fatalf("cohort detail: got %d, want 200: %s", resp.Code, resp.Body)
	}
	var detail PublicAnalysisCohortDetail
	if err := json.NewDecoder(resp.Body).Decode(&detail); err != nil {
		t.Fatalf("decode cohort detail: %v", err)
	}
	if detail.Status != PublicAnalysisStatusNoSnapshot {
		t.Fatalf("Status = %q, want no_snapshot", detail.Status)
	}
	if detail.Snapshot != nil {
		t.Fatalf("expected Snapshot nil on no_snapshot response, got %+v", detail.Snapshot)
	}
}

// TestPublicAnalysisCatalogCarriesDefaultSnapshot checks that the catalog
// exposes default_snapshot + snapshot_count for each selectable cohort.
func TestPublicAnalysisCatalogCarriesDefaultSnapshot(t *testing.T) {
	f := newAnalysisAPITestFixture(t)
	resp := getPublic(t, f.srv, "/pub/api/v1/analysis/catalog")
	if resp.Code != http.StatusOK {
		t.Fatalf("catalog: got %d, want 200: %s", resp.Code, resp.Body)
	}
	var payload PublicAnalysisCatalogResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode catalog: %v", err)
	}
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
}

// TestPublicAnalysisSnapshotsListReturnsCapturedOnly pins the visibility
// rules: captured public snapshots surface, pending / retired /
// mixed-profile / non-public snapshots do not.
func TestPublicAnalysisSnapshotsListReturnsCapturedOnly(t *testing.T) {
	f := newAnalysisAPITestFixture(t)
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
	if resp.Code != http.StatusOK {
		t.Fatalf("snapshots: got %d, want 200: %s", resp.Code, resp.Body)
	}
	var payload PublicAnalysisSnapshotListResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode snapshots: %v", err)
	}
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
}

func TestPublicAnalysisTrendsUseSourceRunOrderAndMetadata(t *testing.T) {
	f := newAnalysisAPITestFixture(t)
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
	if resp.Code != http.StatusOK {
		t.Fatalf("trends: got %d, want 200: %s", resp.Code, resp.Body)
	}
	var payload PublicAnalysisTrendResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode trends: %v", err)
	}
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
}

// TestPublicAnalysisSnapshotDetailHiddenForRetired verifies that an
// explicit slug lookup of a retired or mixed-profile snapshot returns
// 404 - retired snapshots must not leak through the public path.
func TestPublicAnalysisSnapshotDetailHiddenForRetired(t *testing.T) {
	f := newAnalysisAPITestFixture(t)
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
	if resp.Code != http.StatusNotFound {
		t.Fatalf("retired slug: got %d, want 404: %s", resp.Code, resp.Body)
	}

	resp = getPublic(t, f.srv, f.publicURLForSnapshot("2026-03-01-retired", "overview"))
	if resp.Code != http.StatusNotFound {
		t.Fatalf("retired path-segmented: got %d, want 404: %s", resp.Code, resp.Body)
	}
}

// TestPublicAnalysisSnapshotDetailHiddenForMixedProfile pins the mixed
// profile hiding rule on the detail endpoint.
func TestPublicAnalysisSnapshotDetailHiddenForMixedProfile(t *testing.T) {
	f := newAnalysisAPITestFixture(t)
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
	if resp.Code != http.StatusNotFound {
		t.Fatalf("mixed slug: got %d, want 404: %s", resp.Code, resp.Body)
	}
}

// TestPublicAnalysisOverviewSlugInPath verifies that the overview
// handler returns the snapshot pinned by the URL path.
func TestPublicAnalysisOverviewSlugInPath(t *testing.T) {
	f := newAnalysisAPITestFixture(t)
	older := f.seedAlternateSnapshot("batch-old", "2026-04-01-older", time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC))

	resp := getPublic(t, f.srv, f.publicURL("overview"))
	if resp.Code != http.StatusOK {
		t.Fatalf("overview: got %d: %s", resp.Code, resp.Body)
	}
	var payload PublicAnalysisOverviewResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if payload.Snapshot == nil || payload.Snapshot.Slug != f.snapshot.Slug {
		t.Fatalf("fixture slug: got %+v, want fixture", payload.Snapshot)
	}

	resp = getPublic(t, f.srv, f.publicURLForSnapshot("2026-04-01-older", "overview"))
	if resp.Code != http.StatusOK {
		t.Fatalf("older overview: got %d: %s", resp.Code, resp.Body)
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode older: %v", err)
	}
	if payload.Snapshot == nil || payload.Snapshot.Slug != older.Slug {
		t.Fatalf("older slug: got %+v, want %q", payload.Snapshot, older.Slug)
	}
}

// newIsolatedAnalysisTestServer is newAnalysisAPITestFixture without the
// auto-seeded batch + snapshot. Used by tests that need a cohort with
// zero snapshots so the no_snapshot branch is exercised.
func newIsolatedAnalysisTestServer(t *testing.T) (interface{}, *SQLJobStore, *Server) {
	t.Helper()
	f := newAnalysisAPITestFixture(t)
	// Strip the fixture snapshot so the cohort looks empty on the read
	// path. Cleanest: delete the snapshot + batch rows manually.
	if _, err := f.store.db.Exec(`DELETE FROM analysis_cohort_snapshots WHERE cohort_id = ?`, f.cohort.ID); err != nil {
		t.Fatalf("strip fixture snapshot: %v", err)
	}
	if _, err := f.store.db.Exec(`DELETE FROM batches WHERE id = ?`, f.batchID); err != nil {
		t.Fatalf("strip fixture batch: %v", err)
	}
	return nil, f.store, f.srv
}
