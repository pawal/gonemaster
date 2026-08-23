package server

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"codeberg.org/pawal/gonemaster/engine"
)

func TestTagDetailReadsFromViewTable(t *testing.T) {
	forEachAnalysisAPIFixture(t, func(t *testing.T, f *analysisFixture) {
		ts := time.Date(2026, 4, 26, 10, 0, 0, 0, time.UTC)
		f.seedGraduatedRun("alpha.example", ts, []engine.LogEntry{
			{Module: "DNSSEC", Testcase: "dnssec07", Tag: "DS07_NOT_SIGNED", Level: "ERROR"},
		})
		f.seedGraduatedRun("beta.example", ts, []engine.LogEntry{
			{Module: "DNSSEC", Testcase: "dnssec07", Tag: "DS07_NOT_SIGNED", Level: "ERROR"},
		})

		// The legacy detail handler used to load every entry of every run;
		// the new path should not need the entries table at all. Wipe it to
		// prove the rewrite serves the view rows.
		if _, err := f.store.db.Exec("DELETE FROM entries"); err != nil {
			t.Fatalf("clear entries: %v", err)
		}

		resp := getPublic(t, f.srv, f.publicURL("tags/DS07_NOT_SIGNED"))
		got := mustJSON[PublicAnalysisTagDetail](t, resp, http.StatusOK)
		if got.Tag != "DS07_NOT_SIGNED" {
			t.Errorf("Tag = %q, want DS07_NOT_SIGNED", got.Tag)
		}
		if got.Level != "ERROR" {
			t.Errorf("Level = %q, want ERROR", got.Level)
		}
		if got.DomainCount != 2 {
			t.Errorf("DomainCount = %d, want 2", got.DomainCount)
		}
		if len(got.Domains) != 2 || got.Domains[0] != "alpha.example" || got.Domains[1] != "beta.example" {
			t.Errorf("Domains = %v, want sorted [alpha.example beta.example]", got.Domains)
		}
	})
}

func TestTagDetail404OnUnknownTag(t *testing.T) {
	forEachAnalysisAPIFixture(t, func(t *testing.T, f *analysisFixture) {
		ts := time.Date(2026, 4, 26, 10, 0, 0, 0, time.UTC)
		f.seedGraduatedRun("alpha.example", ts, []engine.LogEntry{
			{Module: "DNSSEC", Testcase: "dnssec07", Tag: "DS07_NOT_SIGNED", Level: "ERROR"},
		})

		resp := getPublic(t, f.srv, f.publicURL("tags/UNKNOWN_TAG"))
		wantStatus(t, resp, http.StatusNotFound)
	})
}

func TestTagDetailFloorExcludesInfoTags(t *testing.T) {
	forEachAnalysisAPIFixture(t, func(t *testing.T, f *analysisFixture) {
		ts := time.Date(2026, 4, 26, 10, 0, 0, 0, time.UTC)
		// Default floor is NOTICE, so an INFO-only tag must not have a row.
		f.seedGraduatedRun("alpha.example", ts, []engine.LogEntry{
			{Module: "BASIC", Testcase: "basic01", Tag: "MODULE_OK", Level: "INFO"},
		})

		resp := getPublic(t, f.srv, f.publicURL("tags/MODULE_OK"))
		wantStatus(t, resp, http.StatusNotFound)
	})
}

func TestTagDetailFloorHonorsConfigOverride(t *testing.T) {
	forEachAnalysisAPIFixture(t, func(t *testing.T, f *analysisFixture) {
		// Tighten the floor to WARNING; NOTICE-only tags should now drop too.
		f.store.SetTagViewMinLevel("WARNING")

		ts := time.Date(2026, 4, 26, 10, 0, 0, 0, time.UTC)
		f.seedGraduatedRun("alpha.example", ts, []engine.LogEntry{
			{Module: "BASIC", Testcase: "basic01", Tag: "B01_OK", Level: "NOTICE"},
			{Module: "DNSSEC", Testcase: "dnssec07", Tag: "DS07_NOT_SIGNED", Level: "ERROR"},
		})

		resp := getPublic(t, f.srv, f.publicURL("tags/B01_OK"))
		wantStatus(t, resp, http.StatusNotFound)
		resp = getPublic(t, f.srv, f.publicURL("tags/DS07_NOT_SIGNED"))
		wantStatus(t, resp, http.StatusOK)
	})
}

func TestTagListingReadsFromViewTable(t *testing.T) {
	forEachAnalysisAPIFixture(t, func(t *testing.T, f *analysisFixture) {
		ts := time.Date(2026, 4, 26, 10, 0, 0, 0, time.UTC)
		f.seedGraduatedRun("alpha.example", ts, []engine.LogEntry{
			{Module: "DNSSEC", Testcase: "dnssec07", Tag: "DS07_NOT_SIGNED", Level: "ERROR"},
			{Module: "BASIC", Testcase: "basic01", Tag: "B01_OK", Level: "NOTICE"},
			{Module: "BASIC", Testcase: "basic01", Tag: "MODULE_OK", Level: "INFO"},
		})

		resp := getPublic(t, f.srv, f.publicURL("tags"))
		got := mustJSON[PublicAnalysisListResponse[PublicAnalysisTagView]](t, resp, http.StatusOK)
		tags := map[string]PublicAnalysisTagView{}
		for _, v := range got.Items {
			tags[v.Tag] = v
		}
		if _, ok := tags["MODULE_OK"]; ok {
			t.Errorf("INFO tag MODULE_OK should not appear (capture-time floor is NOTICE)")
		}
		if _, ok := tags["B01_OK"]; !ok {
			t.Errorf("NOTICE tag B01_OK should appear under default floor; got %+v", tags)
		}
		if _, ok := tags["DS07_NOT_SIGNED"]; !ok {
			t.Errorf("ERROR tag DS07_NOT_SIGNED should appear; got %+v", tags)
		}
	})
}

func TestTagListingMinLevelTightens(t *testing.T) {
	forEachAnalysisAPIFixture(t, func(t *testing.T, f *analysisFixture) {
		ts := time.Date(2026, 4, 26, 10, 0, 0, 0, time.UTC)
		f.seedGraduatedRun("alpha.example", ts, []engine.LogEntry{
			{Module: "DNSSEC", Testcase: "dnssec07", Tag: "DS07_NOT_SIGNED", Level: "ERROR"},
			{Module: "BASIC", Testcase: "basic01", Tag: "B01_OK", Level: "NOTICE"},
		})

		resp := getPublic(t, f.srv, f.publicURL("tags?min_level=WARNING"))
		got := mustJSON[PublicAnalysisListResponse[PublicAnalysisTagView]](t, resp, http.StatusOK)
		tags := map[string]PublicAnalysisTagView{}
		for _, v := range got.Items {
			tags[v.Tag] = v
		}
		if _, ok := tags["B01_OK"]; ok {
			t.Errorf("min_level=WARNING should drop NOTICE tags; got %+v", tags)
		}
		if _, ok := tags["DS07_NOT_SIGNED"]; !ok {
			t.Errorf("min_level=WARNING should keep ERROR; got %+v", tags)
		}
	})
}

func TestTagDetailCacheHeadersExplicitSnapshot(t *testing.T) {
	forEachAnalysisAPIFixture(t, func(t *testing.T, f *analysisFixture) {
		ts := time.Date(2026, 4, 26, 10, 0, 0, 0, time.UTC)
		f.seedGraduatedRun("alpha.example", ts, []engine.LogEntry{
			{Module: "DNSSEC", Testcase: "dnssec07", Tag: "DS07_NOT_SIGNED", Level: "ERROR"},
		})

		resp := getPublic(t, f.srv, f.publicURL("tags/DS07_NOT_SIGNED"))
		wantStatus(t, resp, http.StatusOK)
		// Snapshots can be rebuilt under the same slug, so the response must not be
		// immutable; it revalidates via the ETag, which changes on rebuild.
		if cc := resp.Header().Get("Cache-Control"); strings.Contains(cc, "immutable") {
			t.Errorf("explicit-snapshot Cache-Control = %q, must not be immutable", cc)
		}
		if resp.Header().Get("ETag") == "" {
			t.Error("explicit-snapshot response missing ETag for revalidation")
		}
		if etag := resp.Header().Get("ETag"); etag == "" {
			t.Error("explicit-snapshot must set ETag")
		}
	})
}

func TestComputeSnapshotEntityViewsBuildsTagViews(t *testing.T) {
	forEachBackend(t, func(t *testing.T, s *SQLJobStore) {
		cohortID, _ := snapshotViewFixture(t, s)

		// Add a tag summary directly so the fixture has tag data - the
		// existing fixture only exercises endpoints/asns.
		if err := s.ReplaceAnalysisRunTagSummaries(cohortID, "run-a", []AnalysisRunTagSummary{
			{CohortID: cohortID, RunID: "run-a", DomainID: 1, Tag: "DS07_NOT_SIGNED",
				Module: "DNSSEC", Testcase: "dnssec07", Level: "ERROR", OccurrenceCount: 1},
			{CohortID: cohortID, RunID: "run-a", DomainID: 1, Tag: "MODULE_OK",
				Module: "BASIC", Testcase: "basic01", Level: "INFO", OccurrenceCount: 1},
		}); err != nil {
			t.Fatalf("seed tag summaries: %v", err)
		}

		views, err := s.ComputeSnapshotEntityViews(cohortID, "batch-x", "")
		if err != nil {
			t.Fatalf("compute: %v", err)
		}
		byTag := map[string]AnalysisSnapshotTagView{}
		for _, v := range views.Tags {
			byTag[v.Tag] = v
		}
		if _, ok := byTag["DS07_NOT_SIGNED"]; !ok {
			t.Errorf("ERROR tag missing from view: %+v", views.Tags)
		}
		if _, ok := byTag["MODULE_OK"]; ok {
			t.Errorf("INFO tag MODULE_OK should be filtered out by the default floor; got %+v", views.Tags)
		}
	})
}
