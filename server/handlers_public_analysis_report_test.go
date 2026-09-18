package server

import (
	"net/http"
	"testing"
	"time"

	"codeberg.org/pawal/gonemaster/engine"
)

// stampProvenance records a snapshot's engine version, tag vocabulary and
// scoring identity the way capture does.
func (f *analysisFixture) stampProvenance(snap AnalysisCohortSnapshot, engineVersion, vocabulary, scoringHash string) {
	f.t.Helper()
	snap.EngineVersion = engineVersion
	snap.Vocabulary = vocabulary
	snap.ScoringConfigHash = scoringHash
	if _, err := f.store.UpsertAnalysisCohortSnapshot(snap); err != nil {
		f.t.Fatalf("stamp provenance on %q: %v", snap.Slug, err)
	}
}

// setRunScore overwrites a seeded run's summary score and grade, which the
// seeding helper fixes at 85/B.
func (f *analysisFixture) setRunScore(batchID, runID, domainName string, score int, grade string) {
	f.t.Helper()
	domain, ok := f.store.GetDomainByName(domainName)
	if !ok {
		f.t.Fatalf("seeded domain %q missing", domainName)
	}
	run, _ := f.store.GetRun(runID)
	scr, grd := score, grade
	if err := f.store.UpsertAnalysisRunDomainSummary(AnalysisRunDomainSummary{
		CohortID: f.cohort.ID, RunID: runID, DomainID: domain.ID,
		Score: &scr, Grade: &grd, WorstLevel: run.WorstLevel,
		NameserverCount: 1, EndpointCount: 1,
	}); err != nil {
		f.t.Fatalf("upsert summary: %v", err)
	}
	f.refreshSnapshotViews(batchID)
}

// reportFixtureVocabularies are two vocabularies one tag apart: the newer
// engine gained Z15_NO_CAA.
const (
	reportVocabOld = `{"DNSSEC":{"DS07_NOT_SIGNED":"ERROR"}}`
	reportVocabNew = `{"DNSSEC":{"DS07_NOT_SIGNED":"ERROR"},"ZONE":{"Z15_NO_CAA":"NOTICE"}}`
)

// seedReportPair builds two captured snapshots one engine version apart:
// a.example gains a tag the engine gained, b.example gains a tag both
// engines knew.
func seedReportPair(t *testing.T, f *analysisFixture) AnalysisCohortSnapshot {
	t.Helper()
	t1 := time.Date(2026, 4, 17, 12, 0, 0, 0, time.UTC)
	t2 := t1.Add(24 * time.Hour)
	older := f.seedAlternateSnapshot("batch-old", "2026-04-17-old", t1.Add(-time.Hour))

	oldA := f.seedGraduatedRunInBatch(older.BatchID, "a.example", t1, nil)
	oldB := f.seedGraduatedRunInBatch(older.BatchID, "b.example", t1, nil)
	newA := f.seedGraduatedRunInBatch(f.batchID, "a.example", t2, []engine.LogEntry{
		{Module: "ZONE", Testcase: "zone15", Tag: "Z15_NO_CAA", Level: "NOTICE"},
	})
	newB := f.seedGraduatedRunInBatch(f.batchID, "b.example", t2, []engine.LogEntry{
		{Module: "DNSSEC", Testcase: "dnssec07", Tag: "DS07_NOT_SIGNED", Level: "ERROR"},
	})

	f.setRunScore(older.BatchID, oldA.ID, "a.example", 100, "A")
	f.setRunScore(older.BatchID, oldB.ID, "b.example", 100, "A")
	f.setRunScore(f.batchID, newA.ID, "a.example", 99, "A")
	f.setRunScore(f.batchID, newB.ID, "b.example", 80, "B")

	f.stampProvenance(older, "1.2.0", reportVocabOld, "default")
	f.stampProvenance(f.snapshot, "1.3.0", reportVocabNew, "default")
	older, _ = f.store.GetAnalysisCohortSnapshotBySlug(f.cohort.ID, older.Slug)
	f.snapshot, _ = f.store.GetAnalysisCohortSnapshotBySlug(f.cohort.ID, f.snapshot.Slug)
	return older
}

func (f *analysisFixture) reportURL(fromSlug, query string) string {
	url := "/pub/api/v1/analysis/cohorts/" + f.cohort.SourceTag +
		"/report?from=" + fromSlug + "&to=" + f.snapshot.Slug
	if query != "" {
		url += "&" + query
	}
	return url
}

// The report separates a domain moved by a tag the engine gained from one
// moved by a tag both engines knew.
func TestPublicAnalysisReportClassifiesChanges(t *testing.T) {
	forEachAnalysisAPIFixture(t, func(t *testing.T, f *analysisFixture) {
		older := seedReportPair(t, f)
		resp := getPublic(t, f.srv, f.reportURL(older.Slug, ""))
		got := mustJSON[PublicAnalysisReportResponse](t, resp, http.StatusOK)

		if got.Header.From.EngineVersion != "1.2.0" || got.Header.To.EngineVersion != "1.3.0" {
			t.Errorf("engine versions = %s -> %s, want 1.2.0 -> 1.3.0",
				got.Header.From.EngineVersion, got.Header.To.EngineVersion)
		}
		if !got.Header.Vocabulary.FromAvailable || !got.Header.Vocabulary.ToAvailable {
			t.Errorf("vocabulary availability = %+v, want both sides available", got.Header.Vocabulary)
		}
		if len(got.Header.Vocabulary.Added) != 1 || got.Header.Vocabulary.Added[0].Tag != "Z15_NO_CAA" {
			t.Errorf("vocabulary Added = %+v, want [Z15_NO_CAA]", got.Header.Vocabulary.Added)
		}
		if got.Header.ScoringConfigChanged != ReportStateFalse {
			t.Errorf("ScoringConfigChanged = %q, want %q", got.Header.ScoringConfigChanged, ReportStateFalse)
		}
		if got.MinCluster != defaultReportMinCluster || got.MaxSpread != defaultReportMaxSpread {
			t.Errorf("bounds = %d/%d, want %d/%d",
				got.MinCluster, got.MaxSpread, defaultReportMinCluster, defaultReportMaxSpread)
		}

		caa, ok := findReportTag(got.Tags.Appeared, "Z15_NO_CAA")
		if !ok || caa.Classification != ReportChangeNewInEngine {
			t.Errorf("Z15_NO_CAA = %+v, want %q", caa, ReportChangeNewInEngine)
		}
		ds07, ok := findReportTag(got.Tags.Appeared, "DS07_NOT_SIGNED")
		if !ok || ds07.Classification != ReportChangeCohort {
			t.Errorf("DS07_NOT_SIGNED = %+v, want %q", ds07, ReportChangeCohort)
		}

		categories := map[string]string{}
		for _, row := range got.Domains {
			categories[row.Domain] = row.Category
		}
		if categories["a.example"] != ReportCategoryMeasurement {
			t.Errorf("a.example = %q, want %q", categories["a.example"], ReportCategoryMeasurement)
		}
		if categories["b.example"] != ReportCategoryReal {
			t.Errorf("b.example = %q, want %q", categories["b.example"], ReportCategoryReal)
		}
		if got.Totals.Regressed != 2 || got.Totals.BothDomainCount != 2 {
			t.Errorf("totals = regressed %d both %d, want 2/2", got.Totals.Regressed, got.Totals.BothDomainCount)
		}
	})
}

// A snapshot with no captured vocabulary makes every classification unknown
// rather than reading as a cohort change.
func TestPublicAnalysisReportUnknownWithoutVocabulary(t *testing.T) {
	forEachAnalysisAPIFixture(t, func(t *testing.T, f *analysisFixture) {
		older := seedReportPair(t, f)
		f.stampProvenance(older, "1.2.0", "", "")
		older, _ = f.store.GetAnalysisCohortSnapshotBySlug(f.cohort.ID, older.Slug)

		resp := getPublic(t, f.srv, f.reportURL(older.Slug, ""))
		got := mustJSON[PublicAnalysisReportResponse](t, resp, http.StatusOK)

		if got.Header.Vocabulary.FromAvailable {
			t.Errorf("from vocabulary must read unavailable: %+v", got.Header.Vocabulary)
		}
		if got.Header.ScoringConfigChanged != ReportStateUnknown {
			t.Errorf("ScoringConfigChanged = %q, want %q", got.Header.ScoringConfigChanged, ReportStateUnknown)
		}
		for _, e := range got.Tags.Appeared {
			if e.Classification != ReportChangeUnknown {
				t.Errorf("%s = %q, want %q", e.Tag, e.Classification, ReportChangeUnknown)
			}
		}
		for _, row := range got.Domains {
			if row.Category != ReportCategoryUnknown {
				t.Errorf("%s = %q, want %q", row.Domain, row.Category, ReportCategoryUnknown)
			}
		}
	})
}

// Both snapshots must be named and the cluster bounds must be in range.
func TestPublicAnalysisReportValidatesParameters(t *testing.T) {
	forEachAnalysisAPIFixture(t, func(t *testing.T, f *analysisFixture) {
		older := seedReportPair(t, f)
		base := "/pub/api/v1/analysis/cohorts/" + f.cohort.SourceTag + "/report"

		wantStatus(t, getPublic(t, f.srv, base), http.StatusBadRequest)
		wantStatus(t, getPublic(t, f.srv, base+"?from="+older.Slug), http.StatusBadRequest)
		wantStatus(t, getPublic(t, f.srv, f.reportURL(older.Slug, "min_cluster=0")), http.StatusBadRequest)
		wantStatus(t, getPublic(t, f.srv, f.reportURL(older.Slug, "min_cluster=abc")), http.StatusBadRequest)
		wantStatus(t, getPublic(t, f.srv, f.reportURL(older.Slug, "max_spread=0")), http.StatusBadRequest)
		wantStatus(t, getPublic(t, f.srv, f.reportURL(older.Slug, "max_spread=1000")), http.StatusBadRequest)
		wantStatus(t, getPublic(t, f.srv, f.reportURL("no-such-snapshot", "")), http.StatusNotFound)
		wantStatus(t, getPublic(t, f.srv, f.reportURL(older.Slug, "min_cluster=2&max_spread=5")), http.StatusOK)
	})
}

// The report carries the same revalidation headers as the other
// snapshot-scoped reads.
func TestPublicAnalysisReportCacheHeaders(t *testing.T) {
	forEachAnalysisAPIFixture(t, func(t *testing.T, f *analysisFixture) {
		older := seedReportPair(t, f)
		resp := getPublic(t, f.srv, f.reportURL(older.Slug, ""))
		wantStatus(t, resp, http.StatusOK)
		if got := resp.Header().Get("Cache-Control"); got != "no-cache, must-revalidate" {
			t.Errorf("Cache-Control = %q, want no-cache, must-revalidate", got)
		}
		if got := resp.Header().Get("ETag"); got != snapshotETag(f.snapshot) {
			t.Errorf("ETag = %q, want %q", got, snapshotETag(f.snapshot))
		}
	})
}

// A second read of the same pair is served from the cache, and a scoring
// configuration change drops it.
func TestAnalysisReportCacheEviction(t *testing.T) {
	cache := newAnalysisReportCache()
	builds := 0
	build := func() PublicAnalysisReportResponse {
		builds++
		return PublicAnalysisReportResponse{DatasetTag: "tld"}
	}
	cache.compute("k", build)
	cache.compute("k", build)
	if builds != 1 {
		t.Errorf("builds = %d, want 1", builds)
	}
	cache.reset()
	cache.compute("k", build)
	if builds != 2 {
		t.Errorf("builds after reset = %d, want 2", builds)
	}
	for i := range analysisReportCacheSize + 1 {
		cache.compute(string(rune('a'+i)), build)
	}
	if len(cache.entries) > analysisReportCacheSize {
		t.Errorf("cache holds %d entries, want at most %d", len(cache.entries), analysisReportCacheSize)
	}
}
