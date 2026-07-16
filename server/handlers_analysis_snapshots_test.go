package server

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

// adminSnapshotFixture bundles a SQL-backed server with a seeded cohort
// and one captured public snapshot. Admin snapshot tests operate against
// this fixture so CSRF semantics and route resolution run end-to-end.
type adminSnapshotFixture struct {
	t        *testing.T
	srv      *Server
	store    *SQLJobStore
	cohort   AnalysisCohort
	snapshot AnalysisCohortSnapshot
}

func newAdminSnapshotFixture(t *testing.T) *adminSnapshotFixture {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	configurePool(db, "sqlite")
	t.Cleanup(func() { _ = db.Close() })
	dialect := sqliteDialect{}
	if err := runMigrations(db, dialect); err != nil {
		t.Fatalf("runMigrations: %v", err)
	}
	store := NewSQLJobStore(db, dialect)
	srv := newServer(DefaultConfig(), store, NewInMemoryQueue())
	cohort, err := store.UpsertAnalysisCohort(AnalysisCohort{
		SourceType:      "tag",
		SourceTag:       "tld",
		Label:           "TLD",
		AnalysisEnabled: true,
		PublicEnabled:   true,
	})
	if err != nil {
		t.Fatalf("upsert cohort: %v", err)
	}
	now := time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC)
	if err := store.CreateBatch(Batch{
		ID:             "batch-admin",
		Tag:            "tld",
		CreatedAt:      now,
		SnapshotIntent: true,
	}); err != nil {
		t.Fatalf("create batch: %v", err)
	}
	insertTestRun(t, store, Run{
		ID:         "run-admin-1",
		DomainID:   1,
		Domain:     "example.test",
		BatchID:    "batch-admin",
		Status:     JobSucceeded,
		CreatedAt:  now,
		StartedAt:  now,
		FinishedAt: now,
	})
	snap, err := store.UpsertAnalysisCohortSnapshot(AnalysisCohortSnapshot{
		CohortID:   cohort.ID,
		BatchID:    "batch-admin",
		Slug:       "2026-04-20-fixture",
		Label:      "Fixture",
		CapturedAt: now,
		Status:     AnalysisSnapshotStatusCaptured,
		IsPublic:   true,
	})
	if err != nil {
		t.Fatalf("upsert snapshot: %v", err)
	}
	return &adminSnapshotFixture{t: t, srv: srv, store: store, cohort: cohort, snapshot: snap}
}

func (f *adminSnapshotFixture) call(method, path, body string) *httptest.ResponseRecorder {
	f.t.Helper()
	var buf *bytes.Buffer
	if body == "" {
		buf = bytes.NewBuffer(nil)
	} else {
		buf = bytes.NewBufferString(body)
	}
	req := httptest.NewRequest(method, path, buf)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	resp := httptest.NewRecorder()
	f.srv.Handler().ServeHTTP(resp, req)
	return resp
}

func (f *adminSnapshotFixture) callWithOrigin(method, path, origin, body string) *httptest.ResponseRecorder {
	f.t.Helper()
	var buf *bytes.Buffer
	if body == "" {
		buf = bytes.NewBuffer(nil)
	} else {
		buf = bytes.NewBufferString(body)
	}
	req := httptest.NewRequest(method, path, buf)
	req.Host = "example.com"
	req.Header.Set("Origin", origin)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	resp := httptest.NewRecorder()
	f.srv.Handler().ServeHTTP(resp, req)
	return resp
}

func (f *adminSnapshotFixture) snapshotByID(id int64) (AnalysisCohortSnapshot, bool) {
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
	f := newAdminSnapshotFixture(t)
	path := fmt.Sprintf("/api/v1/analysis/cohorts/%d/snapshots/%s", f.cohort.ID, f.snapshot.Slug)
	resp := f.call(http.MethodPost, path, `{"slug":"2026-04-20-relabeled","label":"April 2026","description":"Monthly rollover"}`)
	if resp.Code != http.StatusOK {
		t.Fatalf("patch: got %d, want 200: %s", resp.Code, resp.Body)
	}
	var view AdminAnalysisSnapshotView
	if err := json.NewDecoder(resp.Body).Decode(&view); err != nil {
		t.Fatalf("decode: %v", err)
	}
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
}

// TestAdminSnapshotPatchRejectsSlugCollision verifies slug uniqueness.
func TestAdminSnapshotPatchRejectsSlugCollision(t *testing.T) {
	f := newAdminSnapshotFixture(t)
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
	if resp.Code != http.StatusConflict {
		t.Fatalf("expected 409 slug collision, got %d: %s", resp.Code, resp.Body)
	}
}

// TestAdminSnapshotPatchSetDefaultPinsCohort covers is_default=true: the
// cohort's default_snapshot_policy flips to pinned and default_snapshot_id
// points at this snapshot. Other snapshots have their is_default cleared.
func TestAdminSnapshotPatchSetDefaultPinsCohort(t *testing.T) {
	f := newAdminSnapshotFixture(t)
	path := fmt.Sprintf("/api/v1/analysis/cohorts/%d/snapshots/%s", f.cohort.ID, f.snapshot.Slug)
	resp := f.call(http.MethodPost, path, `{"is_default":true}`)
	if resp.Code != http.StatusOK {
		t.Fatalf("patch: got %d, want 200: %s", resp.Code, resp.Body)
	}
	cohort, _ := f.store.GetAnalysisCohort(f.cohort.ID)
	if cohort.DefaultSnapshotPolicy != DefaultSnapshotPolicyPinned {
		t.Fatalf("policy = %q, want pinned", cohort.DefaultSnapshotPolicy)
	}
	if cohort.DefaultSnapshotID == nil || *cohort.DefaultSnapshotID != f.snapshot.ID {
		t.Fatalf("default_snapshot_id = %+v, want %d", cohort.DefaultSnapshotID, f.snapshot.ID)
	}

	// Unpin via is_default=false restores auto_latest.
	resp = f.call(http.MethodPost, path, `{"is_default":false}`)
	if resp.Code != http.StatusOK {
		t.Fatalf("unpin: got %d, want 200: %s", resp.Code, resp.Body)
	}
	cohort, _ = f.store.GetAnalysisCohort(f.cohort.ID)
	if cohort.DefaultSnapshotPolicy != DefaultSnapshotPolicyAutoLatest {
		t.Fatalf("policy after unpin = %q, want auto_latest", cohort.DefaultSnapshotPolicy)
	}
	if cohort.DefaultSnapshotID != nil {
		t.Fatalf("expected default_snapshot_id to be cleared, got %v", cohort.DefaultSnapshotID)
	}
}

// TestAdminSnapshotRetireSoftDeletes verifies the default DELETE path:
// snapshot stays in the table with status=retired and is_public=false.
func TestAdminSnapshotRetireSoftDeletes(t *testing.T) {
	f := newAdminSnapshotFixture(t)
	path := fmt.Sprintf("/api/v1/analysis/cohorts/%d/snapshots/%s", f.cohort.ID, f.snapshot.Slug)
	resp := f.call(http.MethodDelete, path, "")
	if resp.Code != http.StatusNoContent {
		t.Fatalf("retire: got %d, want 204: %s", resp.Code, resp.Body)
	}
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
}

// TestAdminSnapshotPurgeHardDeletes verifies the ?purge=true query
// parameter: the snapshot row and its overview view are removed.
func TestAdminSnapshotPurgeHardDeletes(t *testing.T) {
	f := newAdminSnapshotFixture(t)
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
	if resp.Code != http.StatusNoContent {
		t.Fatalf("purge: got %d, want 204: %s", resp.Code, resp.Body)
	}
	if _, found := f.store.GetAnalysisCohortSnapshotBySlug(f.cohort.ID, f.snapshot.Slug); found {
		t.Fatal("expected snapshot row to be hard-deleted")
	}
	if _, ok := f.store.GetSnapshotOverview(f.snapshot.ID); ok {
		t.Fatal("expected overview view row to be purged")
	}
}

// TestAdminSnapshotRetireUnpinsCohort covers the corner case: retiring
// the pinned default snapshot must revert the cohort to auto_latest
// so the cohort doesn't end up with a dangling default pointer.
func TestAdminSnapshotRetireUnpinsCohort(t *testing.T) {
	f := newAdminSnapshotFixture(t)
	// Pin the fixture snapshot first.
	path := fmt.Sprintf("/api/v1/analysis/cohorts/%d/snapshots/%s", f.cohort.ID, f.snapshot.Slug)
	if resp := f.call(http.MethodPost, path, `{"is_default":true}`); resp.Code != http.StatusOK {
		t.Fatalf("pin: got %d: %s", resp.Code, resp.Body)
	}
	// Retire it.
	if resp := f.call(http.MethodDelete, path, ""); resp.Code != http.StatusNoContent {
		t.Fatalf("retire: got %d: %s", resp.Code, resp.Body)
	}
	cohort, _ := f.store.GetAnalysisCohort(f.cohort.ID)
	if cohort.DefaultSnapshotPolicy != DefaultSnapshotPolicyAutoLatest {
		t.Fatalf("policy = %q, want auto_latest after retiring pinned snapshot", cohort.DefaultSnapshotPolicy)
	}
	if cohort.DefaultSnapshotID != nil {
		t.Fatalf("expected default_snapshot_id cleared, got %v", cohort.DefaultSnapshotID)
	}
}

// waitForMaterialization polls the snapshot row until its materialization
// status reaches want, returning the final row. The rematerialize endpoint
// dispatches the rebuild in a goroutine, so tests must wait for completion
// rather than reading the row immediately after the 202.
func (f *adminSnapshotFixture) waitForMaterialization(id int64, want string) (AnalysisCohortSnapshot, bool) {
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
	f := newAdminSnapshotFixture(t)
	before, ok := f.snapshotByID(f.snapshot.ID)
	if !ok {
		t.Fatal("fixture snapshot not readable")
	}
	path := fmt.Sprintf("/api/v1/analysis/cohorts/%d/snapshots/%s/rematerialize", f.cohort.ID, f.snapshot.Slug)
	resp := f.call(http.MethodPost, path, "")
	if resp.Code != http.StatusAccepted {
		t.Fatalf("rematerialize: got %d, want 202: %s", resp.Code, resp.Body)
	}
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
}

// TestAdminSnapshotRematerializeRejectsPurgedSource verifies that once
// the source runs for a snapshot's batch are gone (purge has happened),
// the rematerialize endpoint returns 409 source_runs_purged instead of
// silently producing empty views.
func TestAdminSnapshotRematerializeRejectsPurgedSource(t *testing.T) {
	f := newAdminSnapshotFixture(t)
	if _, err := f.store.db.Exec("DELETE FROM runs WHERE batch_id = ?", "batch-admin"); err != nil {
		t.Fatalf("delete runs: %v", err)
	}
	path := fmt.Sprintf("/api/v1/analysis/cohorts/%d/snapshots/%s/rematerialize", f.cohort.ID, f.snapshot.Slug)
	resp := f.call(http.MethodPost, path, "")
	if resp.Code != http.StatusConflict {
		t.Fatalf("got %d, want 409: %s", resp.Code, resp.Body)
	}
	if !strings.Contains(resp.Body.String(), "source_runs_purged") {
		t.Fatalf("expected source_runs_purged error code, got %s", resp.Body)
	}
}

// TestAdminSnapshotListExposesSourceRunsAvailable verifies the snapshot
// list reports source_runs_available per snapshot so the admin UI can
// disable the rematerialize button on snapshots whose runs have been
// purged.
func TestAdminSnapshotListExposesSourceRunsAvailable(t *testing.T) {
	f := newAdminSnapshotFixture(t)
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
	if resp.Code != http.StatusOK {
		t.Fatalf("list: got %d, want 200: %s", resp.Code, resp.Body)
	}
	var list []AdminAnalysisSnapshotView
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		t.Fatalf("decode: %v", err)
	}
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
}

// TestAdminSnapshotCSRFRejectsMismatchedOrigin verifies that mutating
// verbs enforce CSRF when a browser Origin is present. The POST patch
// path is a representative sample; every mutating endpoint goes
// through the same enforceCSRF gate.
func TestAdminSnapshotCSRFRejectsMismatchedOrigin(t *testing.T) {
	f := newAdminSnapshotFixture(t)
	path := fmt.Sprintf("/api/v1/analysis/cohorts/%d/snapshots/%s", f.cohort.ID, f.snapshot.Slug)
	resp := f.callWithOrigin(http.MethodPost, path, "https://evil.example", `{"label":"injected"}`)
	if resp.Code != http.StatusForbidden {
		t.Fatalf("expected 403 on cross-origin POST, got %d: %s", resp.Code, resp.Body)
	}
	if !strings.Contains(resp.Body.String(), "csrf_origin_mismatch") {
		t.Fatalf("expected csrf_origin_mismatch error, got %s", resp.Body)
	}
	// Same origin still works.
	resp = f.callWithOrigin(http.MethodPost, path, "http://example.com", `{"label":"ok"}`)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200 on matching-origin POST, got %d: %s", resp.Code, resp.Body)
	}
}

// TestAdminSnapshotNotFound covers 404 for unknown cohort and unknown
// slug on every verb.
func TestAdminSnapshotNotFound(t *testing.T) {
	f := newAdminSnapshotFixture(t)
	// Unknown cohort id.
	for _, method := range []string{http.MethodPost, http.MethodDelete} {
		resp := f.call(method, "/api/v1/analysis/cohorts/99999/snapshots/whatever", "{}")
		if resp.Code != http.StatusNotFound {
			t.Fatalf("%s unknown cohort: got %d: %s", method, resp.Code, resp.Body)
		}
	}
	// Unknown slug on known cohort.
	path := fmt.Sprintf("/api/v1/analysis/cohorts/%d/snapshots/missing", f.cohort.ID)
	for _, method := range []string{http.MethodPost, http.MethodDelete} {
		resp := f.call(method, path, "{}")
		if resp.Code != http.StatusNotFound {
			t.Fatalf("%s missing slug: got %d: %s", method, resp.Code, resp.Body)
		}
	}
	// Rematerialize a missing slug.
	resp := f.call(http.MethodPost, path+"/rematerialize", "")
	if resp.Code != http.StatusNotFound {
		t.Fatalf("rematerialize missing slug: got %d: %s", resp.Code, resp.Body)
	}
}

// TestAdminSnapshotListReturnsAllStatuses covers the admin list endpoint
// added in Phase 6: unlike the public list, it includes retired and
// failed_mixed_profiles rows so the cohort panel can manage them.
func TestAdminSnapshotListReturnsAllStatuses(t *testing.T) {
	f := newAdminSnapshotFixture(t)
	// Seed a retired snapshot in addition to the fixture's captured one.
	if _, err := f.store.UpsertAnalysisCohortSnapshot(AnalysisCohortSnapshot{
		CohortID: f.cohort.ID, BatchID: "batch-retired", Slug: "2026-03-01-retired",
		Status: AnalysisSnapshotStatusRetired, IsPublic: false,
	}); err != nil {
		t.Fatalf("seed retired: %v", err)
	}
	path := fmt.Sprintf("/api/v1/analysis/cohorts/%d/snapshots", f.cohort.ID)
	resp := f.call(http.MethodGet, path, "")
	if resp.Code != http.StatusOK {
		t.Fatalf("list: got %d, want 200: %s", resp.Code, resp.Body)
	}
	var list []AdminAnalysisSnapshotView
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 snapshots (captured + retired), got %d", len(list))
	}
}

// TestAdminSnapshotRestoreViaStatus covers the status=captured restore
// path added in Phase 6 so retired snapshots can be un-retired without a
// separate endpoint.
func TestAdminSnapshotRestoreViaStatus(t *testing.T) {
	f := newAdminSnapshotFixture(t)
	// Retire the fixture snapshot first.
	path := fmt.Sprintf("/api/v1/analysis/cohorts/%d/snapshots/%s", f.cohort.ID, f.snapshot.Slug)
	if resp := f.call(http.MethodDelete, path, ""); resp.Code != http.StatusNoContent {
		t.Fatalf("retire: got %d: %s", resp.Code, resp.Body)
	}
	// Restore via status=captured.
	resp := f.call(http.MethodPost, path, `{"status":"captured","is_public":true}`)
	if resp.Code != http.StatusOK {
		t.Fatalf("restore: got %d: %s", resp.Code, resp.Body)
	}
	snap, _ := f.store.GetAnalysisCohortSnapshotBySlug(f.cohort.ID, f.snapshot.Slug)
	if snap.Status != AnalysisSnapshotStatusCaptured {
		t.Fatalf("Status after restore = %q, want captured", snap.Status)
	}
	if !snap.IsPublic {
		t.Fatal("restored snapshot must be public again")
	}
}

// TestAdminSnapshotPatchRejectsInvalidStatus pins the status whitelist
// so lifecycle states like "pending" aren't admin-writable.
func TestAdminSnapshotPatchRejectsInvalidStatus(t *testing.T) {
	f := newAdminSnapshotFixture(t)
	path := fmt.Sprintf("/api/v1/analysis/cohorts/%d/snapshots/%s", f.cohort.ID, f.snapshot.Slug)
	resp := f.call(http.MethodPost, path, `{"status":"pending"}`)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid status, got %d: %s", resp.Code, resp.Body)
	}
}

// TestAdminSnapshotMethodNotAllowed pins GET/PUT as not allowed on the
// snapshot resource - the shape is POST/DELETE only.
func TestAdminSnapshotMethodNotAllowed(t *testing.T) {
	f := newAdminSnapshotFixture(t)
	path := fmt.Sprintf("/api/v1/analysis/cohorts/%d/snapshots/%s", f.cohort.ID, f.snapshot.Slug)
	resp := f.call(http.MethodPut, path, `{}`)
	if resp.Code != http.StatusMethodNotAllowed {
		t.Fatalf("PUT: got %d, want 405: %s", resp.Code, resp.Body)
	}
	resp = f.call(http.MethodGet, path, "")
	if resp.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET: got %d, want 405: %s", resp.Code, resp.Body)
	}
}
