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
// parameter: the snapshot row and its aggregates are removed entirely.
func TestAdminSnapshotPurgeHardDeletes(t *testing.T) {
	f := newAdminSnapshotFixture(t)
	if err := f.store.ReplaceSnapshotAggregates(f.snapshot.ID, []AnalysisCohortSnapshotAggregate{
		{SnapshotID: f.snapshot.ID, Category: SnapshotAggregateSigned, PayloadJSON: `{"signed":1}`},
	}); err != nil {
		t.Fatalf("seed aggregates: %v", err)
	}
	path := fmt.Sprintf("/api/v1/analysis/cohorts/%d/snapshots/%s?purge=true", f.cohort.ID, f.snapshot.Slug)
	resp := f.call(http.MethodDelete, path, "")
	if resp.Code != http.StatusNoContent {
		t.Fatalf("purge: got %d, want 204: %s", resp.Code, resp.Body)
	}
	if _, found := f.store.GetAnalysisCohortSnapshotBySlug(f.cohort.ID, f.snapshot.Slug); found {
		t.Fatal("expected snapshot row to be hard-deleted")
	}
	if aggs := f.store.ListSnapshotAggregates(f.snapshot.ID); len(aggs) != 0 {
		t.Fatalf("expected aggregates to be purged, got %d", len(aggs))
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

// TestAdminSnapshotRematerialize verifies the rematerialize action
// recomputes aggregates and bumps CapturedAt.
func TestAdminSnapshotRematerialize(t *testing.T) {
	f := newAdminSnapshotFixture(t)
	originalCaptured := f.snapshot.CapturedAt
	path := fmt.Sprintf("/api/v1/analysis/cohorts/%d/snapshots/%s/rematerialize", f.cohort.ID, f.snapshot.Slug)
	resp := f.call(http.MethodPost, path, "")
	if resp.Code != http.StatusOK {
		t.Fatalf("rematerialize: got %d, want 200: %s", resp.Code, resp.Body)
	}
	snap, found := f.snapshotByID(f.snapshot.ID)
	if !found {
		t.Fatal("snapshot missing after rematerialize")
	}
	if !snap.CapturedAt.After(originalCaptured) {
		t.Fatalf("CapturedAt = %s, want advanced from %s", snap.CapturedAt, originalCaptured)
	}
	aggs := f.store.ListSnapshotAggregates(snap.ID)
	if len(aggs) == 0 {
		t.Fatal("expected aggregates to be written by rematerialize")
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

// TestAdminSnapshotMethodNotAllowed pins GET/PUT as not allowed on the
// snapshot resource — the shape is POST/DELETE only.
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
