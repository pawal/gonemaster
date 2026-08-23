package server

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func newAdminSnapshotFixture(t *testing.T) *analysisFixture {
	t.Helper()
	return newAnalysisFixture(t, withFixtureBatch("batch-admin"), withSeededRun("example.test"))
}

func (f *analysisFixture) call(method, path, body string, opts ...reqOpt) *httptest.ResponseRecorder {
	f.t.Helper()
	if body == "" {
		opts = append(opts, noContentType())
	}
	return doJSON(f.t, f.srv, method, path, body, opts...)
}

func (f *analysisFixture) snapshotByID(id int64) (AnalysisCohortSnapshot, bool) {
	// Walk the cohort's snapshots to avoid depending on a test-only
	// lookup-by-id helper; there are only a handful per test.
	for _, snap := range f.store.ListAnalysisCohortSnapshots(f.cohort.ID) {
		if snap.ID == id {
			return snap, true
		}
	}
	return AnalysisCohortSnapshot{}, false
}

// TestAdminSnapshotPatchLabelAndSlug covers the relabel + rename path.
func TestAdminSnapshotPatchLabelAndSlug(t *testing.T) {
	forEachAdminSnapshotFixture(t, func(t *testing.T, f *analysisFixture) {
		path := fmt.Sprintf("/api/v1/analysis/cohorts/%d/snapshots/%s", f.cohort.ID, f.snapshot.Slug)
		resp := f.call(http.MethodPost, path, `{"slug":"2026-04-20-relabeled","label":"April 2026","description":"Monthly rollover"}`)
		view := mustJSON[AdminAnalysisSnapshotView](t, resp, http.StatusOK)
		if view.Slug != "2026-04-20-relabeled" {
			t.Fatalf("Slug = %q, want 2026-04-20-relabeled", view.Slug)
		}
		if view.Label != "April 2026" {
			t.Fatalf("Label = %q, want April 2026", view.Label)
		}
		if view.Description != "Monthly rollover" {
			t.Fatalf("Description = %q", view.Description)
		}
		// Lookup by old slug must now miss.
		if _, found := f.store.GetAnalysisCohortSnapshotBySlug(f.cohort.ID, f.snapshot.Slug); found {
			t.Fatal("expected old slug to be gone after rename")
		}
	})
}

// TestAdminSnapshotPatchRejectsSlugCollision verifies slug uniqueness.
func TestAdminSnapshotPatchRejectsSlugCollision(t *testing.T) {
	forEachAdminSnapshotFixture(t, func(t *testing.T, f *analysisFixture) {
		// Seed a second snapshot whose slug the patch will collide with.
		if err := f.store.CreateBatch(Batch{
			ID: "batch-other", Tag: "tld", CreatedAt: time.Date(2026, 4, 10, 12, 0, 0, 0, time.UTC), SnapshotIntent: true,
		}); err != nil {
			t.Fatalf("seed other batch: %v", err)
		}
		if _, err := f.store.UpsertAnalysisCohortSnapshot(AnalysisCohortSnapshot{
			CohortID: f.cohort.ID, BatchID: "batch-other", Slug: "2026-04-10-other",
			Status: AnalysisSnapshotStatusCaptured, IsPublic: true,
		}); err != nil {
			t.Fatalf("seed other snapshot: %v", err)
		}
		path := fmt.Sprintf("/api/v1/analysis/cohorts/%d/snapshots/%s", f.cohort.ID, f.snapshot.Slug)
		resp := f.call(http.MethodPost, path, `{"slug":"2026-04-10-other"}`)
		wantStatus(t, resp, http.StatusConflict)
	})
}

// TestAdminSnapshotPatchSetDefaultPinsCohort covers is_default=true: the
// cohort's default_snapshot_policy flips to pinned and default_snapshot_id
// points at this snapshot. Other snapshots have their is_default cleared.
func TestAdminSnapshotPatchSetDefaultPinsCohort(t *testing.T) {
	forEachAdminSnapshotFixture(t, func(t *testing.T, f *analysisFixture) {
		path := fmt.Sprintf("/api/v1/analysis/cohorts/%d/snapshots/%s", f.cohort.ID, f.snapshot.Slug)
		resp := f.call(http.MethodPost, path, `{"is_default":true}`)
		wantStatus(t, resp, http.StatusOK)
		cohort, _ := f.store.GetAnalysisCohort(f.cohort.ID)
		if cohort.DefaultSnapshotPolicy != DefaultSnapshotPolicyPinned {
			t.Fatalf("policy = %q, want pinned", cohort.DefaultSnapshotPolicy)
		}
		if cohort.DefaultSnapshotID == nil || *cohort.DefaultSnapshotID != f.snapshot.ID {
			t.Fatalf("default_snapshot_id = %+v, want %d", cohort.DefaultSnapshotID, f.snapshot.ID)
		}

		// Unpin via is_default=false restores auto_latest.
		resp = f.call(http.MethodPost, path, `{"is_default":false}`)
		wantStatus(t, resp, http.StatusOK)
		cohort, _ = f.store.GetAnalysisCohort(f.cohort.ID)
		if cohort.DefaultSnapshotPolicy != DefaultSnapshotPolicyAutoLatest {
			t.Fatalf("policy after unpin = %q, want auto_latest", cohort.DefaultSnapshotPolicy)
		}
		if cohort.DefaultSnapshotID != nil {
			t.Fatalf("expected default_snapshot_id to be cleared, got %v", cohort.DefaultSnapshotID)
		}
	})
}

// TestAdminSnapshotRetireSoftDeletes verifies the default DELETE path:
// snapshot stays in the table with status=retired and is_public=false.
func TestAdminSnapshotRetireSoftDeletes(t *testing.T) {
	forEachAdminSnapshotFixture(t, func(t *testing.T, f *analysisFixture) {
		path := fmt.Sprintf("/api/v1/analysis/cohorts/%d/snapshots/%s", f.cohort.ID, f.snapshot.Slug)
		resp := f.call(http.MethodDelete, path, "")
		wantStatus(t, resp, http.StatusNoContent)
		snap, found := f.store.GetAnalysisCohortSnapshotBySlug(f.cohort.ID, f.snapshot.Slug)
		if !found {
			t.Fatal("expected retired snapshot row to remain; soft delete should not drop it")
		}
		if snap.Status != AnalysisSnapshotStatusRetired {
			t.Fatalf("Status = %q, want retired", snap.Status)
		}
		if snap.IsPublic {
			t.Fatal("retired snapshot must not remain public")
		}
	})
}

// TestAdminSnapshotPurgeHardDeletes verifies the ?purge=true query
// parameter: the snapshot row and its overview view are removed.
func TestAdminSnapshotPurgeHardDeletes(t *testing.T) {
	forEachAdminSnapshotFixture(t, func(t *testing.T, f *analysisFixture) {
		if err := f.store.ReplaceSnapshotOverview(f.snapshot.ID, SnapshotOverviewV2{
			FactDistributions: map[string]PublicAnalysisFactDistribution{
				FactCategoryDNSSECPosture: {
					Category: FactCategoryDNSSECPosture,
					Buckets:  []PublicAnalysisFactBucket{{Key: FactKeyNSEC3, Count: 1}},
				},
			},
		}); err != nil {
			t.Fatalf("seed overview: %v", err)
		}
		path := fmt.Sprintf("/api/v1/analysis/cohorts/%d/snapshots/%s?purge=true", f.cohort.ID, f.snapshot.Slug)
		resp := f.call(http.MethodDelete, path, "")
		wantStatus(t, resp, http.StatusNoContent)
		if _, found := f.store.GetAnalysisCohortSnapshotBySlug(f.cohort.ID, f.snapshot.Slug); found {
			t.Fatal("expected snapshot row to be hard-deleted")
		}
		if _, ok := f.store.GetSnapshotOverview(f.snapshot.ID); ok {
			t.Fatal("expected overview view row to be purged")
		}
	})
}

// TestAdminSnapshotRetireUnpinsCohort covers the corner case: retiring
// the pinned default snapshot must revert the cohort to auto_latest
// so the cohort doesn't end up with a dangling default pointer.
func TestAdminSnapshotRetireUnpinsCohort(t *testing.T) {
	forEachAdminSnapshotFixture(t, func(t *testing.T, f *analysisFixture) {
		// Pin the fixture snapshot first.
		path := fmt.Sprintf("/api/v1/analysis/cohorts/%d/snapshots/%s", f.cohort.ID, f.snapshot.Slug)
		wantStatus(t, f.call(http.MethodPost, path, `{"is_default":true}`), http.StatusOK)
		// Retire it.
		wantStatus(t, f.call(http.MethodDelete, path, ""), http.StatusNoContent)
		cohort, _ := f.store.GetAnalysisCohort(f.cohort.ID)
		if cohort.DefaultSnapshotPolicy != DefaultSnapshotPolicyAutoLatest {
			t.Fatalf("policy = %q, want auto_latest after retiring pinned snapshot", cohort.DefaultSnapshotPolicy)
		}
		if cohort.DefaultSnapshotID != nil {
			t.Fatalf("expected default_snapshot_id cleared, got %v", cohort.DefaultSnapshotID)
		}
	})
}

// waitForMaterialization polls the snapshot row until its materialization
// status reaches want, returning the final row. The rematerialize endpoint
// dispatches the rebuild in a goroutine, so tests must wait for completion
// rather than reading the row immediately after the 202. It stays on the
// real clock: the rebuild talks to the database outside any bubble.
func (f *analysisFixture) waitForMaterialization(id int64, want string) (AnalysisCohortSnapshot, bool) {
	f.t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		snap, found := f.snapshotByID(id)
		if found && snap.MaterializationStatus == want {
			return snap, true
		}
		if time.Now().After(deadline) {
			return snap, false
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// TestAdminSnapshotRematerialize verifies the rematerialize action rebuilds the
// overview view asynchronously, reports progress to completion, marks the
// snapshot ready, and - the ordering-fix contract - leaves captured_at intact.
// A rebuild recomputes derived views; it does not re-capture, so it must not
// bump captured_at and thereby reorder the newest-captured-first snapshot list.
func TestAdminSnapshotRematerialize(t *testing.T) {
	forEachAdminSnapshotFixture(t, func(t *testing.T, f *analysisFixture) {
		before, ok := f.snapshotByID(f.snapshot.ID)
		if !ok {
			t.Fatal("fixture snapshot not readable")
		}
		path := fmt.Sprintf("/api/v1/analysis/cohorts/%d/snapshots/%s/rematerialize", f.cohort.ID, f.snapshot.Slug)
		resp := f.call(http.MethodPost, path, "")
		wantStatus(t, resp, http.StatusAccepted)
		snap, ok := f.waitForMaterialization(f.snapshot.ID, AnalysisMaterializationReady)
		if !ok {
			t.Fatalf("snapshot did not reach ready: status=%q done=%d/%d",
				snap.MaterializationStatus, snap.MaterializationDone, snap.MaterializationTotal)
		}
		if snap.MaterializationTotal != rematerializePhases || snap.MaterializationDone != rematerializePhases {
			t.Fatalf("progress = %d/%d, want %d/%d", snap.MaterializationDone, snap.MaterializationTotal,
				rematerializePhases, rematerializePhases)
		}
		if snap.LastMaterializationError != "" {
			t.Fatalf("unexpected error on success: %q", snap.LastMaterializationError)
		}
		if snap.LastMaterializedAt.IsZero() {
			t.Fatal("last_materialized_at not stamped on ready")
		}
		// The ordering-fix contract: captured_at must be byte-for-byte unchanged.
		if !snap.CapturedAt.Equal(before.CapturedAt) {
			t.Fatalf("CapturedAt changed to %s, want unchanged %s", snap.CapturedAt, before.CapturedAt)
		}
		if _, ok := f.store.GetSnapshotOverview(snap.ID); !ok {
			t.Fatal("expected overview view row to be written by rematerialize")
		}
	})
}

// TestAdminSnapshotRematerializeRejectsPurgedSource verifies that once
// the source runs for a snapshot's batch are gone (purge has happened),
// the rematerialize endpoint returns 409 source_runs_purged instead of
// silently producing empty views.
func TestAdminSnapshotRematerializeRejectsPurgedSource(t *testing.T) {
	forEachAdminSnapshotFixture(t, func(t *testing.T, f *analysisFixture) {
		if _, err := f.store.db.Exec("DELETE FROM runs WHERE batch_id = "+f.store.ph(1), "batch-admin"); err != nil {
			t.Fatalf("delete runs: %v", err)
		}
		path := fmt.Sprintf("/api/v1/analysis/cohorts/%d/snapshots/%s/rematerialize", f.cohort.ID, f.snapshot.Slug)
		resp := f.call(http.MethodPost, path, "")
		wantStatus(t, resp, http.StatusConflict)
		if !strings.Contains(resp.Body.String(), "source_runs_purged") {
			t.Fatalf("expected source_runs_purged error code, got %s", resp.Body)
		}
	})
}

// TestAdminSnapshotListExposesSourceRunsAvailable verifies the snapshot
// list reports source_runs_available per snapshot so the admin UI can
// disable the rematerialize button on snapshots whose runs have been
// purged.
func TestAdminSnapshotListExposesSourceRunsAvailable(t *testing.T) {
	forEachAdminSnapshotFixture(t, func(t *testing.T, f *analysisFixture) {
		if err := f.store.CreateBatch(Batch{ID: "batch-orphan", Tag: "tld", CreatedAt: time.Now().UTC()}); err != nil {
			t.Fatalf("create orphan batch: %v", err)
		}
		if _, err := f.store.UpsertAnalysisCohortSnapshot(AnalysisCohortSnapshot{
			CohortID: f.cohort.ID, BatchID: "batch-orphan", Slug: "2026-04-21-orphan",
			Status: AnalysisSnapshotStatusCaptured, IsPublic: true,
		}); err != nil {
			t.Fatalf("seed orphan snapshot: %v", err)
		}
		path := fmt.Sprintf("/api/v1/analysis/cohorts/%d/snapshots", f.cohort.ID)
		resp := f.call(http.MethodGet, path, "")
		list := mustJSON[[]AdminAnalysisSnapshotView](t, resp, http.StatusOK)
		got := map[string]bool{}
		for _, snap := range list {
			got[snap.Slug] = snap.SourceRunsAvailable
		}
		if v, ok := got[f.snapshot.Slug]; !ok || !v {
			t.Fatalf("fixture snapshot source_runs_available = %v (ok=%v), want true", v, ok)
		}
		if v, ok := got["2026-04-21-orphan"]; !ok || v {
			t.Fatalf("orphan snapshot source_runs_available = %v (ok=%v), want false", v, ok)
		}
	})
}

// TestAdminSnapshotCSRFRejectsMismatchedOrigin verifies that mutating
// verbs enforce CSRF when a browser Origin is present. The POST patch
// path is a representative sample; every mutating endpoint goes
// through the same enforceCSRF gate.
func TestAdminSnapshotCSRFOriginMatrix(t *testing.T) {
	forEachAdminSnapshotFixture(t, func(t *testing.T, f *analysisFixture) {
		path := fmt.Sprintf("/api/v1/analysis/cohorts/%d/snapshots/%s", f.cohort.ID, f.snapshot.Slug)
		csrfOriginMatrix(t, http.StatusOK, func(t *testing.T, opts ...reqOpt) *httptest.ResponseRecorder {
			return f.call(http.MethodPost, path, `{"label":"relabel"}`,
				append([]reqOpt{withHost("example.com")}, opts...)...)
		})
	})
}

// TestAdminSnapshotNotFound covers 404 for unknown cohort and unknown
// slug on every verb.
func TestAdminSnapshotNotFound(t *testing.T) {
	forEachAdminSnapshotFixture(t, func(t *testing.T, f *analysisFixture) {
		// Unknown cohort id.
		for _, method := range []string{http.MethodPost, http.MethodDelete} {
			resp := f.call(method, "/api/v1/analysis/cohorts/99999/snapshots/whatever", "{}")
			wantStatus(t, resp, http.StatusNotFound)
		}
		// Unknown slug on known cohort.
		path := fmt.Sprintf("/api/v1/analysis/cohorts/%d/snapshots/missing", f.cohort.ID)
		for _, method := range []string{http.MethodPost, http.MethodDelete} {
			resp := f.call(method, path, "{}")
			wantStatus(t, resp, http.StatusNotFound)
		}
		// Rematerialize a missing slug.
		resp := f.call(http.MethodPost, path+"/rematerialize", "")
		wantStatus(t, resp, http.StatusNotFound)
	})
}

// TestAdminSnapshotListReturnsAllStatuses covers the admin list endpoint:
// unlike the public list, it includes retired and failed_mixed_profiles rows
// so the cohort panel can manage them.
func TestAdminSnapshotListReturnsAllStatuses(t *testing.T) {
	forEachAdminSnapshotFixture(t, func(t *testing.T, f *analysisFixture) {
		// Seed a retired snapshot in addition to the fixture's captured one.
		if _, err := f.store.UpsertAnalysisCohortSnapshot(AnalysisCohortSnapshot{
			CohortID: f.cohort.ID, BatchID: "batch-retired", Slug: "2026-03-01-retired",
			Status: AnalysisSnapshotStatusRetired, IsPublic: false,
		}); err != nil {
			t.Fatalf("seed retired: %v", err)
		}
		path := fmt.Sprintf("/api/v1/analysis/cohorts/%d/snapshots", f.cohort.ID)
		resp := f.call(http.MethodGet, path, "")
		list := mustJSON[[]AdminAnalysisSnapshotView](t, resp, http.StatusOK)
		if len(list) != 2 {
			t.Fatalf("expected 2 snapshots (captured + retired), got %d", len(list))
		}
	})
}

// TestAdminSnapshotRestoreViaStatus covers the status=captured restore path,
// which un-retires a snapshot without a separate endpoint.
func TestAdminSnapshotRestoreViaStatus(t *testing.T) {
	forEachAdminSnapshotFixture(t, func(t *testing.T, f *analysisFixture) {
		// Retire the fixture snapshot first.
		path := fmt.Sprintf("/api/v1/analysis/cohorts/%d/snapshots/%s", f.cohort.ID, f.snapshot.Slug)
		wantStatus(t, f.call(http.MethodDelete, path, ""), http.StatusNoContent)
		// Restore via status=captured.
		resp := f.call(http.MethodPost, path, `{"status":"captured","is_public":true}`)
		wantStatus(t, resp, http.StatusOK)
		snap, _ := f.store.GetAnalysisCohortSnapshotBySlug(f.cohort.ID, f.snapshot.Slug)
		if snap.Status != AnalysisSnapshotStatusCaptured {
			t.Fatalf("Status after restore = %q, want captured", snap.Status)
		}
		if !snap.IsPublic {
			t.Fatal("restored snapshot must be public again")
		}
	})
}

// TestAdminSnapshotPatchRejectsInvalidStatus pins the status whitelist
// so lifecycle states like "pending" aren't admin-writable.
func TestAdminSnapshotPatchRejectsInvalidStatus(t *testing.T) {
	forEachAdminSnapshotFixture(t, func(t *testing.T, f *analysisFixture) {
		path := fmt.Sprintf("/api/v1/analysis/cohorts/%d/snapshots/%s", f.cohort.ID, f.snapshot.Slug)
		resp := f.call(http.MethodPost, path, `{"status":"pending"}`)
		wantStatus(t, resp, http.StatusBadRequest)
	})
}

// TestAdminSnapshotMethodNotAllowed pins GET/PUT as not allowed on the
// snapshot resource - the shape is POST/DELETE only.
func TestAdminSnapshotMethodNotAllowed(t *testing.T) {
	forEachAdminSnapshotFixture(t, func(t *testing.T, f *analysisFixture) {
		path := fmt.Sprintf("/api/v1/analysis/cohorts/%d/snapshots/%s", f.cohort.ID, f.snapshot.Slug)
		resp := f.call(http.MethodPut, path, `{}`)
		wantStatus(t, resp, http.StatusMethodNotAllowed)
		resp = f.call(http.MethodGet, path, "")
		wantStatus(t, resp, http.StatusMethodNotAllowed)
	})
}
