package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"codeberg.org/pawal/gonemaster/engine"
)

func TestTagDetailReadsFromViewTable(t *testing.T) {
	f := newAnalysisAPITestFixture(t)
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
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", resp.Code, resp.Body)
	}
	var got PublicAnalysisTagDetail
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
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
}

func TestTagDetail404OnUnknownTag(t *testing.T) {
	f := newAnalysisAPITestFixture(t)
	ts := time.Date(2026, 4, 26, 10, 0, 0, 0, time.UTC)
	f.seedGraduatedRun("alpha.example", ts, []engine.LogEntry{
		{Module: "DNSSEC", Testcase: "dnssec07", Tag: "DS07_NOT_SIGNED", Level: "ERROR"},
	})

	resp := getPublic(t, f.srv, f.publicURL("tags/UNKNOWN_TAG"))
	if resp.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.Code)
	}
}

func TestTagDetailFloorExcludesInfoTags(t *testing.T) {
	f := newAnalysisAPITestFixture(t)
	ts := time.Date(2026, 4, 26, 10, 0, 0, 0, time.UTC)
	// Default floor is NOTICE, so an INFO-only tag must not have a row.
	f.seedGraduatedRun("alpha.example", ts, []engine.LogEntry{
		{Module: "BASIC", Testcase: "basic01", Tag: "MODULE_OK", Level: "INFO"},
	})

	resp := getPublic(t, f.srv, f.publicURL("tags/MODULE_OK"))
	if resp.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (INFO tags must not get a detail page)", resp.Code)
	}
}

func TestTagDetailFloorHonorsConfigOverride(t *testing.T) {
	f := newAnalysisAPITestFixture(t)
	// Tighten the floor to WARNING; NOTICE-only tags should now drop too.
	f.store.SetTagViewMinLevel("WARNING")

	ts := time.Date(2026, 4, 26, 10, 0, 0, 0, time.UTC)
	f.seedGraduatedRun("alpha.example", ts, []engine.LogEntry{
		{Module: "BASIC", Testcase: "basic01", Tag: "B01_OK", Level: "NOTICE"},
		{Module: "DNSSEC", Testcase: "dnssec07", Tag: "DS07_NOT_SIGNED", Level: "ERROR"},
	})

	resp := getPublic(t, f.srv, f.publicURL("tags/B01_OK"))
	if resp.Code != http.StatusNotFound {
		t.Fatalf("NOTICE tag with floor=WARNING: status = %d, want 404", resp.Code)
	}
	resp = getPublic(t, f.srv, f.publicURL("tags/DS07_NOT_SIGNED"))
	if resp.Code != http.StatusOK {
		t.Fatalf("ERROR tag with floor=WARNING: status = %d, want 200", resp.Code)
	}
}

func TestTagListingReadsFromViewTable(t *testing.T) {
	f := newAnalysisAPITestFixture(t)
	ts := time.Date(2026, 4, 26, 10, 0, 0, 0, time.UTC)
	f.seedGraduatedRun("alpha.example", ts, []engine.LogEntry{
		{Module: "DNSSEC", Testcase: "dnssec07", Tag: "DS07_NOT_SIGNED", Level: "ERROR"},
		{Module: "BASIC", Testcase: "basic01", Tag: "B01_OK", Level: "NOTICE"},
		{Module: "BASIC", Testcase: "basic01", Tag: "MODULE_OK", Level: "INFO"},
	})

	resp := getPublic(t, f.srv, f.publicURL("tags"))
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", resp.Code, resp.Body)
	}
	var got PublicAnalysisListResponse[PublicAnalysisTagView]
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
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
}

func TestTagListingMinLevelTightens(t *testing.T) {
	f := newAnalysisAPITestFixture(t)
	ts := time.Date(2026, 4, 26, 10, 0, 0, 0, time.UTC)
	f.seedGraduatedRun("alpha.example", ts, []engine.LogEntry{
		{Module: "DNSSEC", Testcase: "dnssec07", Tag: "DS07_NOT_SIGNED", Level: "ERROR"},
		{Module: "BASIC", Testcase: "basic01", Tag: "B01_OK", Level: "NOTICE"},
	})

	resp := getPublic(t, f.srv, f.publicURL("tags?min_level=WARNING"))
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d", resp.Code)
	}
	var got PublicAnalysisListResponse[PublicAnalysisTagView]
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
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
}

func TestTagDetailCacheHeadersExplicitSnapshot(t *testing.T) {
	f := newAnalysisAPITestFixture(t)
	ts := time.Date(2026, 4, 26, 10, 0, 0, 0, time.UTC)
	f.seedGraduatedRun("alpha.example", ts, []engine.LogEntry{
		{Module: "DNSSEC", Testcase: "dnssec07", Tag: "DS07_NOT_SIGNED", Level: "ERROR"},
	})

	resp := getPublic(t, f.srv, f.publicURL("tags/DS07_NOT_SIGNED"))
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d", resp.Code)
	}
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
}

func TestComputeSnapshotEntityViewsBuildsTagViews(t *testing.T) {
	s := testStoreForBackend(t, testBackends(t)[0])
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
}
