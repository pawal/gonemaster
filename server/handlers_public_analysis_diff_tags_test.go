package server

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"codeberg.org/pawal/gonemaster/engine"
)

// A tag only in "to" appeared, only in "from" cleared, in both with a
// different worst level changed level, and in both with the same level (case
// insensitive) is in no bucket. Buckets sort by the larger domain count
// descending, then tag, so map iteration order cannot leak into the API.
func TestDiffTagViews(t *testing.T) {
	from := []AnalysisSnapshotTagView{
		{Tag: "DS07_NOT_SIGNED", Module: "DNSSEC", Testcase: "dnssec07", Level: "ERROR", DomainCount: 5},
		{Tag: "DROPPED_TAG", Module: "DELEGATION", Testcase: "delegation01", Level: "WARNING", DomainCount: 3},
		{Tag: "LEVELUP", Module: "CONSISTENCY", Testcase: "consistency01", Level: "NOTICE", DomainCount: 4},
		{Tag: "ONLY_CASE", Level: "error", DomainCount: 1},
	}
	to := []AnalysisSnapshotTagView{
		{Tag: "DS07_NOT_SIGNED", Module: "DNSSEC", Testcase: "dnssec07", Level: "ERROR", DomainCount: 12},
		{Tag: "NEW_TAG", Module: "DELEGATION", Testcase: "delegation02", Level: "WARNING", DomainCount: 8},
		{Tag: "BIG_NEW_TAG", Module: "DNSSEC", Testcase: "dnssec10", Level: "ERROR", DomainCount: 20},
		{Tag: "LEVELUP", Module: "CONSISTENCY", Testcase: "consistency01", Level: "ERROR", DomainCount: 6},
		{Tag: "ONLY_CASE", Level: "ERROR", DomainCount: 1},
	}

	appeared, cleared, levelChanged := diffTagViews(from, to)

	// Appeared: BIG_NEW_TAG (20) sorts before NEW_TAG (8) by impact.
	if len(appeared) != 2 {
		t.Fatalf("appeared = %d entries, want 2: %+v", len(appeared), appeared)
	}
	if appeared[0].Tag != "BIG_NEW_TAG" || appeared[1].Tag != "NEW_TAG" {
		t.Errorf("appeared order = [%s %s], want [BIG_NEW_TAG NEW_TAG]", appeared[0].Tag, appeared[1].Tag)
	}
	if appeared[1].ToDomainCount != 8 || appeared[1].FromDomainCount != 0 || appeared[1].DomainDelta != 8 {
		t.Errorf("NEW_TAG counts = from %d to %d delta %d, want from 0 to 8 delta 8",
			appeared[1].FromDomainCount, appeared[1].ToDomainCount, appeared[1].DomainDelta)
	}
	if appeared[1].FromLevel != "" || appeared[1].ToLevel != "WARNING" {
		t.Errorf("NEW_TAG levels = from %q to %q, want from '' to WARNING", appeared[1].FromLevel, appeared[1].ToLevel)
	}

	// Cleared: only DROPPED_TAG, with a negative delta.
	if len(cleared) != 1 || cleared[0].Tag != "DROPPED_TAG" {
		t.Fatalf("cleared = %+v, want [DROPPED_TAG]", cleared)
	}
	if cleared[0].FromDomainCount != 3 || cleared[0].ToDomainCount != 0 || cleared[0].DomainDelta != -3 {
		t.Errorf("DROPPED_TAG counts = from %d to %d delta %d, want from 3 to 0 delta -3",
			cleared[0].FromDomainCount, cleared[0].ToDomainCount, cleared[0].DomainDelta)
	}
	if cleared[0].ToLevel != "" || cleared[0].FromLevel != "WARNING" {
		t.Errorf("DROPPED_TAG levels = from %q to %q, want from WARNING to ''", cleared[0].FromLevel, cleared[0].ToLevel)
	}

	// Level-changed: only LEVELUP (NOTICE -> ERROR). DS07 kept its ERROR
	// level and ONLY_CASE differs only by case, so neither appears here.
	if len(levelChanged) != 1 || levelChanged[0].Tag != "LEVELUP" {
		t.Fatalf("levelChanged = %+v, want [LEVELUP]", levelChanged)
	}
	lc := levelChanged[0]
	if lc.FromLevel != "NOTICE" || lc.ToLevel != "ERROR" {
		t.Errorf("LEVELUP levels = from %q to %q, want NOTICE -> ERROR", lc.FromLevel, lc.ToLevel)
	}
	if lc.FromDomainCount != 4 || lc.ToDomainCount != 6 || lc.DomainDelta != 2 {
		t.Errorf("LEVELUP counts = from %d to %d delta %d, want from 4 to 6 delta 2",
			lc.FromDomainCount, lc.ToDomainCount, lc.DomainDelta)
	}
	if lc.Module != "CONSISTENCY" || lc.Testcase != "consistency01" {
		t.Errorf("LEVELUP module/testcase = %q/%q, want CONSISTENCY/consistency01", lc.Module, lc.Testcase)
	}
}

// TestDiffTagViewsEmpty proves empty inputs yield empty (non-nil) slices so
// the JSON response always carries [] rather than null.
func TestDiffTagViewsEmpty(t *testing.T) {
	appeared, cleared, levelChanged := diffTagViews(nil, nil)
	if appeared == nil || cleared == nil || levelChanged == nil {
		t.Fatalf("expected non-nil empty slices, got %v %v %v", appeared, cleared, levelChanged)
	}
	if len(appeared)+len(cleared)+len(levelChanged) != 0 {
		t.Errorf("expected all empty, got appeared=%d cleared=%d level=%d",
			len(appeared), len(cleared), len(levelChanged))
	}
}

// TestDiffGranularityTagsClassifiesTags exercises the full HTTP path: two
// captured snapshots whose runs carry deliberately different finding tags,
// diffed with ?granularity=tags. The older snapshot has DS08_MISSING
// (cleared) and SOA_SERIAL at NOTICE; the newer one drops DS08, raises
// SOA_SERIAL to ERROR (level-changed), keeps DS07 unchanged, and introduces
// NS_FEW (appeared).
func TestDiffGranularityTagsClassifiesTags(t *testing.T) {
	forEachAnalysisAPIFixture(t, func(t *testing.T, f *analysisFixture) {
		t1 := time.Date(2026, 4, 17, 12, 0, 0, 0, time.UTC)
		t2 := t1.Add(24 * time.Hour)

		older := f.seedAlternateSnapshot("batch-old", "2026-04-17-old", t1.Add(-time.Hour))
		f.seedGraduatedRunInBatch(older.BatchID, "a.example", t1, []engine.LogEntry{
			{Module: "DNSSEC", Testcase: "dnssec07", Tag: "DS07_NOT_SIGNED", Level: "ERROR"},
		})
		f.seedGraduatedRunInBatch(older.BatchID, "b.example", t1, []engine.LogEntry{
			{Module: "DNSSEC", Testcase: "dnssec08", Tag: "DS08_MISSING", Level: "WARNING"},
		})
		f.seedGraduatedRunInBatch(older.BatchID, "c.example", t1, []engine.LogEntry{
			{Module: "CONSISTENCY", Testcase: "consistency01", Tag: "SOA_SERIAL", Level: "NOTICE"},
		})

		f.seedGraduatedRunInBatch(f.batchID, "a.example", t2, []engine.LogEntry{
			{Module: "DNSSEC", Testcase: "dnssec07", Tag: "DS07_NOT_SIGNED", Level: "ERROR"},
		})
		f.seedGraduatedRunInBatch(f.batchID, "c.example", t2, []engine.LogEntry{
			{Module: "CONSISTENCY", Testcase: "consistency01", Tag: "SOA_SERIAL", Level: "ERROR"},
		})
		f.seedGraduatedRunInBatch(f.batchID, "d.example", t2, []engine.LogEntry{
			{Module: "DELEGATION", Testcase: "delegation01", Tag: "NS_FEW", Level: "WARNING"},
		})
		f.refreshSnapshotViews(older.BatchID)
		f.refreshSnapshotViews(f.batchID)

		url := "/pub/api/v1/analysis/cohorts/tld/diff?granularity=tags&from=" + older.Slug + "&to=" + f.snapshot.Slug
		resp := getPublic(t, f.srv, url)
		if resp.Code != http.StatusOK {
			t.Fatalf("status = %d, body = %s", resp.Code, resp.Body)
		}
		var got PublicAnalysisTagDiffResponse
		if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if got.Granularity != "tags" {
			t.Errorf("Granularity = %q, want tags", got.Granularity)
		}
		if !hasTag(got.Appeared, "NS_FEW") {
			t.Errorf("Appeared should include NS_FEW, got %+v", got.Appeared)
		}
		if hasTag(got.Appeared, "DS07_NOT_SIGNED") || hasTag(got.Appeared, "SOA_SERIAL") {
			t.Errorf("Appeared must not include unchanged/level-changed tags, got %+v", got.Appeared)
		}
		if !hasTag(got.Cleared, "DS08_MISSING") {
			t.Errorf("Cleared should include DS08_MISSING, got %+v", got.Cleared)
		}
		soa, ok := findTag(got.LevelChanged, "SOA_SERIAL")
		if !ok {
			t.Fatalf("LevelChanged should include SOA_SERIAL, got %+v", got.LevelChanged)
		}
		if soa.FromLevel != "NOTICE" || soa.ToLevel != "ERROR" {
			t.Errorf("SOA_SERIAL levels = %s -> %s, want NOTICE -> ERROR", soa.FromLevel, soa.ToLevel)
		}
		// DS07 is present in both snapshots at the same level: it must not leak
		// into any of the three change buckets.
		if hasTag(got.Cleared, "DS07_NOT_SIGNED") || hasTag(got.LevelChanged, "DS07_NOT_SIGNED") {
			t.Errorf("unchanged DS07_NOT_SIGNED leaked into a change bucket")
		}
	})
}

// TestDiffGranularityInvalidReturns400 rejects an unknown granularity so the
// UI can rely on either a documented shape or a clear error.
func TestDiffGranularityInvalidReturns400(t *testing.T) {
	forEachAnalysisAPIFixture(t, func(t *testing.T, f *analysisFixture) {
		t1 := time.Date(2026, 4, 17, 12, 0, 0, 0, time.UTC)
		older := f.seedAlternateSnapshot("batch-old", "2026-04-17-old", t1.Add(-time.Hour))
		f.seedGraduatedRunInBatch(older.BatchID, "a.example", t1, nil)
		f.seedGraduatedRunInBatch(f.batchID, "a.example", t1.Add(time.Hour), nil)
		f.refreshSnapshotViews(older.BatchID)
		f.refreshSnapshotViews(f.batchID)

		url := "/pub/api/v1/analysis/cohorts/tld/diff?granularity=bogus&from=" + older.Slug + "&to=" + f.snapshot.Slug
		resp := getPublic(t, f.srv, url)
		if resp.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400; body = %s", resp.Code, resp.Body)
		}
	})
}

func hasTag(entries []PublicAnalysisTagDiffEntry, tag string) bool {
	_, ok := findTag(entries, tag)
	return ok
}

func findTag(entries []PublicAnalysisTagDiffEntry, tag string) (PublicAnalysisTagDiffEntry, bool) {
	for _, e := range entries {
		if e.Tag == tag {
			return e, true
		}
	}
	return PublicAnalysisTagDiffEntry{}, false
}
