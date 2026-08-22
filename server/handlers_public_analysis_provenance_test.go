package server

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"
)

// stampEngineVersion sets provenance on an already-seeded snapshot and
// returns the reloaded row.
func stampEngineVersion(t *testing.T, f *analysisFixture, snap AnalysisCohortSnapshot, version string, mixed bool) AnalysisCohortSnapshot {
	t.Helper()
	snap.EngineVersion = version
	snap.MixedEngineVersion = mixed
	updated, err := f.store.UpsertAnalysisCohortSnapshot(snap)
	if err != nil {
		t.Fatalf("stamp engine version on %s: %v", snap.Slug, err)
	}
	return updated
}

func TestPublicAnalysisSnapshotListCarriesEngineVersion(t *testing.T) {
	forEachAnalysisAPIFixture(t, func(t *testing.T, f *analysisFixture) {
		f.snapshot = stampEngineVersion(t, f, f.snapshot, "v1.6.6", false)

		resp := getPublic(t, f.srv, "/pub/api/v1/analysis/cohorts/tld/snapshots")
		if resp.Code != http.StatusOK {
			t.Fatalf("snapshots: got %d, want 200: %s", resp.Code, resp.Body)
		}
		var payload PublicAnalysisSnapshotListResponse
		if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
			t.Fatalf("decode snapshots: %v", err)
		}
		if len(payload.Snapshots) != 1 {
			t.Fatalf("expected 1 snapshot, got %d", len(payload.Snapshots))
		}
		if payload.Snapshots[0].EngineVersion != "v1.6.6" {
			t.Fatalf("engine_version = %q, want v1.6.6", payload.Snapshots[0].EngineVersion)
		}
		if payload.Snapshots[0].MixedEngineVersion {
			t.Fatal("mixed_engine_version should be false for a single-version snapshot")
		}
	})
}

func TestPublicAnalysisSnapshotListReportsMixedEngineVersion(t *testing.T) {
	forEachAnalysisAPIFixture(t, func(t *testing.T, f *analysisFixture) {
		f.snapshot = stampEngineVersion(t, f, f.snapshot, "v1.6.3", true)

		resp := getPublic(t, f.srv, "/pub/api/v1/analysis/cohorts/tld/snapshots")
		var payload PublicAnalysisSnapshotListResponse
		if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
			t.Fatalf("decode snapshots: %v", err)
		}
		if !payload.Snapshots[0].MixedEngineVersion {
			t.Fatal("expected mixed_engine_version to survive to the public response")
		}
	})
}

// A snapshot whose provenance could not be recovered must omit the field
// rather than report some default, so a client can tell "unknown" apart from
// "known to be version X".
func TestPublicAnalysisSnapshotListOmitsUnknownEngineVersion(t *testing.T) {
	forEachAnalysisAPIFixture(t, func(t *testing.T, f *analysisFixture) {

		resp := getPublic(t, f.srv, "/pub/api/v1/analysis/cohorts/tld/snapshots")
		var raw struct {
			Snapshots []map[string]any `json:"snapshots"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
			t.Fatalf("decode snapshots: %v", err)
		}
		if len(raw.Snapshots) != 1 {
			t.Fatalf("expected 1 snapshot, got %d", len(raw.Snapshots))
		}
		if _, present := raw.Snapshots[0]["engine_version"]; present {
			t.Fatal("engine_version must be omitted entirely when unknown")
		}
	})
}

func TestPublicAnalysisTrendPointsCarryEngineVersion(t *testing.T) {
	forEachAnalysisAPIFixture(t, func(t *testing.T, f *analysisFixture) {
		f.snapshot = stampEngineVersion(t, f, f.snapshot, "v1.6.6", false)
		if err := f.store.ReplaceSnapshotOverview(f.snapshot.ID, SnapshotOverviewV2{
			FactDistributions: map[string]PublicAnalysisFactDistribution{
				FactCategorySeverity: {
					Category: FactCategorySeverity,
					Buckets:  []PublicAnalysisFactBucket{{Key: "OK", Count: 1}},
				},
			},
		}); err != nil {
			t.Fatalf("seed overview: %v", err)
		}

		resp := getPublic(t, f.srv, "/pub/api/v1/analysis/cohorts/tld/trends?category="+FactCategorySeverity)
		if resp.Code != http.StatusOK {
			t.Fatalf("trends: got %d, want 200: %s", resp.Code, resp.Body)
		}
		var payload PublicAnalysisTrendResponse
		if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
			t.Fatalf("decode trends: %v", err)
		}
		if len(payload.Points) != 1 {
			t.Fatalf("expected 1 trend point, got %d", len(payload.Points))
		}
		if payload.Points[0].EngineVersion != "v1.6.6" {
			t.Fatalf("point engine_version = %q, want v1.6.6", payload.Points[0].EngineVersion)
		}
	})
}

// seedDiffPair returns (older, newer) snapshots stamped with the given
// versions, both carrying the domain views a diff needs.
func seedDiffPair(t *testing.T, f *analysisFixture, fromVersion, toVersion string) (AnalysisCohortSnapshot, AnalysisCohortSnapshot) {
	t.Helper()
	older := f.seedAlternateSnapshot("batch-prev", "2026-07-31-prev",
		time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC))
	older = stampEngineVersion(t, f, older, fromVersion, false)
	newer := stampEngineVersion(t, f, f.snapshot, toVersion, false)
	return older, newer
}

// The whole reason provenance exists: a diff that crosses an engine upgrade
// has to say so, because tag churn across that boundary may be new engine
// capability rather than the cohort changing.
func TestPublicAnalysisDiffFlagsCrossedEngineVersions(t *testing.T) {
	forEachAnalysisAPIFixture(t, func(t *testing.T, f *analysisFixture) {
		older, newer := seedDiffPair(t, f, "v1.6.3", "v1.6.6")

		resp := getPublic(t, f.srv, "/pub/api/v1/analysis/cohorts/tld/diff?from="+older.Slug+"&to="+newer.Slug)
		if resp.Code != http.StatusOK {
			t.Fatalf("diff: got %d, want 200: %s", resp.Code, resp.Body)
		}
		var payload PublicAnalysisDiffResponse
		if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
			t.Fatalf("decode diff: %v", err)
		}
		if !payload.Engine.Crossed {
			t.Fatal("expected crossed_engine_versions=true across an upgrade")
		}
		if payload.Engine.Unknown {
			t.Fatal("both sides are stamped, so the delta must not be unknown")
		}
		if payload.Engine.FromEngineVersion != "v1.6.3" || payload.Engine.ToEngineVersion != "v1.6.6" {
			t.Fatalf("engine delta = %+v, want v1.6.3 -> v1.6.6", payload.Engine)
		}
	})
}

// The complementary case, and the one that makes a step trustworthy: same
// engine on both sides means the change is the cohort's, not the tooling's.
func TestPublicAnalysisDiffReportsSameEngineVersion(t *testing.T) {
	forEachAnalysisAPIFixture(t, func(t *testing.T, f *analysisFixture) {
		older, newer := seedDiffPair(t, f, "v1.6.3", "v1.6.3")

		resp := getPublic(t, f.srv, "/pub/api/v1/analysis/cohorts/tld/diff?from="+older.Slug+"&to="+newer.Slug)
		var payload PublicAnalysisDiffResponse
		if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
			t.Fatalf("decode diff: %v", err)
		}
		if payload.Engine.Crossed {
			t.Fatal("same engine version on both sides must not be flagged as crossed")
		}
		if payload.Engine.Unknown {
			t.Fatal("both sides are stamped, so the delta must not be unknown")
		}
	})
}

// When one side has no recoverable version, "not crossed" would be a lie -
// we simply do not know. Unknown has to be reported distinctly.
func TestPublicAnalysisDiffReportsUnknownEngineVersion(t *testing.T) {
	forEachAnalysisAPIFixture(t, func(t *testing.T, f *analysisFixture) {
		older := f.seedAlternateSnapshot("batch-prev", "2026-05-05-prev",
			time.Date(2026, 5, 5, 12, 0, 0, 0, time.UTC))
		newer := stampEngineVersion(t, f, f.snapshot, "v1.6.6", false)

		resp := getPublic(t, f.srv, "/pub/api/v1/analysis/cohorts/tld/diff?from="+older.Slug+"&to="+newer.Slug)
		var payload PublicAnalysisDiffResponse
		if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
			t.Fatalf("decode diff: %v", err)
		}
		if !payload.Engine.Unknown {
			t.Fatal("expected engine_version_unknown=true when one side is unstamped")
		}
		if payload.Engine.Crossed {
			t.Fatal("crossed must stay false when the comparison cannot be made")
		}
	})
}

func TestPublicAnalysisTagDiffCarriesEngineDelta(t *testing.T) {
	forEachAnalysisAPIFixture(t, func(t *testing.T, f *analysisFixture) {
		older, newer := seedDiffPair(t, f, "v1.6.3", "v1.6.6")

		url := "/pub/api/v1/analysis/cohorts/tld/diff?granularity=tags&from=" + older.Slug + "&to=" + newer.Slug
		resp := getPublic(t, f.srv, url)
		if resp.Code != http.StatusOK {
			t.Fatalf("tag diff: got %d, want 200: %s", resp.Code, resp.Body)
		}
		var payload PublicAnalysisTagDiffResponse
		if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
			t.Fatalf("decode tag diff: %v", err)
		}
		if !payload.Engine.Crossed {
			t.Fatal("the tag diff is where engine capability shows up; it must carry the delta")
		}
	})
}
