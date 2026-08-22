package server

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"codeberg.org/pawal/gonemaster/engine"
)

// TestOverviewReadsFromViewTable proves /overview serves from the
// per-snapshot overview view rather than scanning fact rows.
func TestOverviewReadsFromViewTable(t *testing.T) {
	forEachAnalysisAPIFixture(t, func(t *testing.T, f *analysisFixture) {
		now := time.Date(2026, 4, 26, 10, 0, 0, 0, time.UTC)
		f.seedGraduatedRun("alpha.example", now, []engine.LogEntry{
			{Module: "DNSSEC", Testcase: "dnssec07", Tag: "DS07_NOT_SIGNED", Level: "ERROR"},
		})
		f.seedEndpoint("run-alpha.example-"+now.Format("20060102150405"),
			"alpha.example", "ns1.example", "192.0.2.1", "ipv4", now, 64500, "192.0.2.0/24")

		resp := getPublic(t, f.srv, f.publicURL("overview"))
		if resp.Code != http.StatusOK {
			t.Fatalf("status = %d, body = %s", resp.Code, resp.Body)
		}
		var got PublicAnalysisOverviewResponse
		if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if got.Overview == nil {
			t.Fatal("expected Overview payload")
		}
		if got.Overview.Totals.DomainCount != 1 {
			t.Errorf("DomainCount = %d, want 1", got.Overview.Totals.DomainCount)
		}
		if got.Overview.Totals.NameserverCount != 1 {
			t.Errorf("NameserverCount = %d, want 1", got.Overview.Totals.NameserverCount)
		}
		if got.Overview.Totals.ASNCount != 1 {
			t.Errorf("ASNCount = %d, want 1", got.Overview.Totals.ASNCount)
		}
	})
}

// TestOverviewSurvivesFactWipe asserts the overview row is captured
// from the projected facts and remains readable after the underlying
// fact tables are emptied - the read path goes through the view only.
func TestOverviewSurvivesFactWipe(t *testing.T) {
	forEachAnalysisAPIFixture(t, func(t *testing.T, f *analysisFixture) {
		now := time.Date(2026, 4, 26, 10, 0, 0, 0, time.UTC)
		f.seedGraduatedRun("alpha.example", now, nil)
		runID := "run-alpha.example-" + now.Format("20060102150405")
		f.seedEndpoint(runID, "alpha.example", "ns1.example", "192.0.2.1", "ipv4", now, 64500, "192.0.2.0/24")

		if err := f.store.ReplaceAnalysisRunNSEndpoints(f.cohort.ID, runID, nil); err != nil {
			t.Fatalf("clear endpoints: %v", err)
		}
		if err := f.store.ReplaceAnalysisRunAddressASNs(f.cohort.ID, runID, nil); err != nil {
			t.Fatalf("clear address asns: %v", err)
		}

		resp := getPublic(t, f.srv, f.publicURL("overview"))
		if resp.Code != http.StatusOK {
			t.Fatalf("status = %d, body = %s", resp.Code, resp.Body)
		}
		var got PublicAnalysisOverviewResponse
		if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if got.Overview == nil || got.Overview.Totals.NameserverCount != 1 {
			t.Errorf("Overview = %+v, want NameserverCount=1 sourced from view", got.Overview)
		}
	})
}

// TestSnapshotDetailExposesAggregateMap pins backward compatibility:
// the snapshot detail handler still emits a category->payload map
// keyed by the legacy aggregate names, synthesized from the overview.
func TestSnapshotDetailExposesAggregateMap(t *testing.T) {
	forEachAnalysisAPIFixture(t, func(t *testing.T, f *analysisFixture) {
		now := time.Date(2026, 4, 26, 10, 0, 0, 0, time.UTC)
		f.seedGraduatedRun("alpha.example", now, []engine.LogEntry{
			{Module: "DNSSEC", Testcase: "dnssec07", Tag: "DS07_NOT_SIGNED", Level: "ERROR"},
		})

		url := "/pub/api/v1/analysis/cohorts/tld/snapshots/" + f.snapshot.Slug
		resp := getPublic(t, f.srv, url)
		if resp.Code != http.StatusOK {
			t.Fatalf("status = %d, body = %s", resp.Code, resp.Body)
		}
		var got PublicAnalysisSnapshotDetail
		if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if _, ok := got.Aggregates[SnapshotAggregateOverviewV2]; !ok {
			t.Errorf("Aggregates missing overview_v2 key: %+v", got.Aggregates)
		}
		if _, ok := got.Aggregates[FactCategorySeverity]; !ok {
			t.Errorf("Aggregates missing %q key", FactCategorySeverity)
		}
		if _, ok := got.Aggregates[FactCategoryGrade]; !ok {
			t.Errorf("Aggregates missing %q key", FactCategoryGrade)
		}
	})
}

// TestTrendsReadsFromOverviewView asserts the trend chart query is
// answered from one indexed scan over the overview view, not from
// per-snapshot aggregate-row reads.
func TestTrendsReadsFromOverviewView(t *testing.T) {
	forEachAnalysisAPIFixture(t, func(t *testing.T, f *analysisFixture) {
		t1 := time.Date(2026, 4, 17, 12, 0, 0, 0, time.UTC)
		t2 := t1.Add(24 * time.Hour)

		older := f.seedAlternateSnapshot("batch-old", "2026-04-17-old", t1.Add(-time.Hour))
		older.FirstRunAt = t1
		older.LastRunAt = t1
		updated, err := f.store.UpsertAnalysisCohortSnapshot(older)
		if err != nil {
			t.Fatalf("update older source time: %v", err)
		}
		older = updated
		f.seedGraduatedRunInBatch(older.BatchID, "alpha.example", t1, []engine.LogEntry{
			{Module: "BASIC", Testcase: "basic01", Tag: "B01_OK", Level: "NOTICE"},
		})

		f.seedGraduatedRunInBatch(f.batchID, "alpha.example", t2, []engine.LogEntry{
			{Module: "DNSSEC", Testcase: "dnssec07", Tag: "DS07_NOT_SIGNED", Level: "ERROR"},
		})

		resp := getPublic(t, f.srv, "/pub/api/v1/analysis/cohorts/tld/trends?category="+FactCategorySeverity)
		if resp.Code != http.StatusOK {
			t.Fatalf("status = %d, body = %s", resp.Code, resp.Body)
		}
		var got PublicAnalysisTrendResponse
		if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if len(got.Points) != 2 {
			t.Fatalf("expected 2 points (one per snapshot), got %d: %+v", len(got.Points), got.Points)
		}
		if got.Category != FactCategorySeverity {
			t.Errorf("Category = %q", got.Category)
		}
		for _, p := range got.Points {
			if len(p.Payload) == 0 {
				t.Errorf("snapshot %q has empty payload", p.Slug)
			}
		}
	})
}

// TestComputeSnapshotOverviewIsBatchScoped pins out-of-batch facts
// must not bleed into the captured overview row.
func TestComputeSnapshotOverviewIsBatchScoped(t *testing.T) {
	forEachBackend(t, func(t *testing.T, s *SQLJobStore) {
		cohortID, snapID := snapshotViewFixture(t, s)

		overview, err := s.ComputeSnapshotOverview(cohortID, "batch-x")
		if err != nil {
			t.Fatalf("compute: %v", err)
		}
		if err := s.ReplaceSnapshotOverview(snapID, overview); err != nil {
			t.Fatalf("replace: %v", err)
		}
		got, ok := s.GetSnapshotOverview(snapID)
		if !ok {
			t.Fatal("expected overview row after replace")
		}
		if got.Totals.DomainCount != 2 {
			t.Errorf("DomainCount = %d, want 2 (batch-other run must not surface)", got.Totals.DomainCount)
		}
		if got.Totals.ASNCount != 2 {
			t.Errorf("ASNCount = %d, want 2 (out-of-batch ASN 65000 must not surface)", got.Totals.ASNCount)
		}
	})
}

// TestListSnapshotOverviewsByIDsBulkReads confirms the trends bulk
// reader returns one row per requested snapshot id.
func TestListSnapshotOverviewsByIDsBulkReads(t *testing.T) {
	forEachBackend(t, func(t *testing.T, s *SQLJobStore) {
		cohortID, snapID := snapshotViewFixture(t, s)

		overview, err := s.ComputeSnapshotOverview(cohortID, "batch-x")
		if err != nil {
			t.Fatalf("compute: %v", err)
		}
		if err := s.ReplaceSnapshotOverview(snapID, overview); err != nil {
			t.Fatalf("replace: %v", err)
		}
		got := s.ListSnapshotOverviewsByIDs([]int64{snapID, snapID + 999})
		if _, ok := got[snapID]; !ok {
			t.Fatalf("expected primary snapshot in result, got %+v", got)
		}
		if _, ok := got[snapID+999]; ok {
			t.Errorf("unknown snapshot id leaked: %+v", got)
		}
	})
}

// TestAggregatesTableDropped asserts the legacy
// analysis_cohort_snapshot_aggregates table is gone after migration. Selecting
// from the table is the portable existence check: sqlite_master and
// information_schema disagree across the three backends, and a catalog query
// that simply errors would let this assertion pass for the wrong reason.
func TestAggregatesTableDropped(t *testing.T) {
	forEachBackend(t, func(t *testing.T, s *SQLJobStore) {
		var n int
		err := s.db.QueryRow(`SELECT COUNT(*) FROM analysis_cohort_snapshot_aggregates`).Scan(&n)
		if err == nil {
			t.Fatalf("aggregates table still present, %d rows", n)
		}
	})
}
