package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"codeberg.org/pawal/gonemaster/engine"
)

// seedGraduatedRun runs a job through GraduateJob so that entries and domain
// latest_* fields get populated, then upserts an analysis summary for the run.
func (f *analysisAPITestFixture) seedGraduatedRun(domainName string, finishedAt time.Time, entries []engine.LogEntry) Run {
	return f.seedGraduatedRunInBatch(f.batchID, domainName, finishedAt, entries)
}

// seedGraduatedRunInBatch is seedGraduatedRun with an explicit batch id,
// used by snapshot-scoping tests that need runs under a non-default
// batch so their facts live in a different snapshot.
func (f *analysisAPITestFixture) seedGraduatedRunInBatch(batchID, domainName string, finishedAt time.Time, entries []engine.LogEntry) Run {
	f.t.Helper()
	runID := "run-" + domainName + "-" + finishedAt.Format("20060102150405")
	now := finishedAt
	job := Job{
		ID:         runID,
		Domain:     domainName,
		BatchID:    batchID,
		Status:     JobSucceeded,
		CreatedAt:  now.Add(-time.Minute),
		StartedAt:  now.Add(-time.Minute),
		FinishedAt: now,
	}
	if _, err := f.store.Create(job); err != nil {
		f.t.Fatalf("create job: %v", err)
	}
	if err := f.store.GraduateJob(job, entries); err != nil {
		f.t.Fatalf("graduate job: %v", err)
	}
	run, _ := f.store.GetRun(runID)
	domain, _ := f.store.GetDomainByName(domainName)
	score := 85
	grade := "B"
	if err := f.store.UpsertAnalysisRunDomainSummary(AnalysisRunDomainSummary{
		CohortID: f.cohort.ID, RunID: runID, DomainID: domain.ID,
		Score: &score, Grade: &grade, WorstLevel: run.WorstLevel,
		NameserverCount: 1, EndpointCount: 1,
	}); err != nil {
		f.t.Fatalf("upsert summary: %v", err)
	}
	// Tag aggregates — mirror what the real projector persists so the
	// /tags + /testcases handlers (which now read from the materialized
	// tag_summary table) see the test's findings.
	type tagKey struct{ tag, testcase string }
	type bucket struct {
		module, level string
		count         int
	}
	buckets := map[tagKey]*bucket{}
	for _, e := range entries {
		tag := strings.TrimSpace(e.Tag)
		if tag == "" {
			continue
		}
		k := tagKey{tag: tag, testcase: e.Testcase}
		b, ok := buckets[k]
		if !ok {
			b = &bucket{module: e.Module, level: e.Level}
			buckets[k] = b
		}
		if severityRank(e.Level) > severityRank(b.level) {
			b.level = e.Level
		}
		b.count++
	}
	rows := make([]AnalysisRunTagSummary, 0, len(buckets))
	for k, b := range buckets {
		rows = append(rows, AnalysisRunTagSummary{
			CohortID: f.cohort.ID, RunID: runID, DomainID: domain.ID,
			Tag: k.tag, Module: b.module, Testcase: k.testcase,
			Level: b.level, OccurrenceCount: b.count,
		})
	}
	if err := f.store.ReplaceAnalysisRunTagSummaries(f.cohort.ID, runID, rows); err != nil {
		f.t.Fatalf("replace tag summaries: %v", err)
	}
	f.refreshSnapshotViews(batchID)
	return run
}

func TestPublicAnalysisTagsAggregates(t *testing.T) {
	f := newAnalysisAPITestFixture(t)
	ts := time.Date(2026, 4, 17, 12, 0, 0, 0, time.UTC)
	f.seedGraduatedRun("alpha.example", ts, []engine.LogEntry{
		{Module: "DNSSEC", Testcase: "dnssec01", Tag: "DS01_SIGNED", Level: "NOTICE"},
		{Module: "DNSSEC", Testcase: "dnssec07", Tag: "DS07_NOT_SIGNED", Level: "ERROR"},
	})
	f.seedGraduatedRun("beta.example", ts, []engine.LogEntry{
		{Module: "DNSSEC", Testcase: "dnssec07", Tag: "DS07_NOT_SIGNED", Level: "ERROR"},
	})
	f.seedGraduatedRun("gamma.example", ts, []engine.LogEntry{
		{Module: "BASIC", Testcase: "basic01", Tag: "B01_OK", Level: "NOTICE"},
	})

	resp := getPublic(t, f.srv, "/pub/api/v1/analysis/tags")
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body)
	}
	var got PublicAnalysisListResponse[PublicAnalysisTagView]
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Total != 3 {
		t.Fatalf("expected 3 unique tags, got %d", got.Total)
	}
	byTag := map[string]PublicAnalysisTagView{}
	for _, v := range got.Items {
		byTag[v.Tag] = v
	}
	if ds07 := byTag["DS07_NOT_SIGNED"]; ds07.DomainCount != 2 || ds07.OccurrenceCount != 2 || ds07.Level != "ERROR" {
		t.Fatalf("unexpected DS07 aggregation: %+v", ds07)
	}
	if ds01 := byTag["DS01_SIGNED"]; ds01.DomainCount != 1 || ds01.Level != "NOTICE" {
		t.Fatalf("unexpected DS01 aggregation: %+v", ds01)
	}
	if got.Items[0].Tag != "DS07_NOT_SIGNED" {
		t.Fatalf("expected top tag by domain_count to be DS07_NOT_SIGNED, got %q", got.Items[0].Tag)
	}
}

func TestPublicAnalysisTagsSearch(t *testing.T) {
	f := newAnalysisAPITestFixture(t)
	ts := time.Date(2026, 4, 17, 12, 0, 0, 0, time.UTC)
	f.seedGraduatedRun("alpha.example", ts, []engine.LogEntry{
		{Module: "DNSSEC", Tag: "DS07_NOT_SIGNED", Level: "ERROR"},
		{Module: "BASIC", Tag: "B01_OK", Level: "NOTICE"},
	})

	resp := getPublic(t, f.srv, "/pub/api/v1/analysis/tags?search=dnssec")
	var got PublicAnalysisListResponse[PublicAnalysisTagView]
	_ = json.NewDecoder(resp.Body).Decode(&got)
	if got.Total != 1 || got.Items[0].Tag != "DS07_NOT_SIGNED" {
		t.Fatalf("expected dnssec module filter to return one row, got %+v", got)
	}
}

func TestPublicAnalysisTagsMinLevel(t *testing.T) {
	f := newAnalysisAPITestFixture(t)
	ts := time.Date(2026, 4, 17, 12, 0, 0, 0, time.UTC)
	f.seedGraduatedRun("alpha.example", ts, []engine.LogEntry{
		{Module: "BASIC", Tag: "B01_OK", Level: "NOTICE"},
		{Module: "DELEGATION", Tag: "REFERRAL_SLOW", Level: "WARNING"},
		{Module: "DNSSEC", Tag: "DS07_NOT_SIGNED", Level: "ERROR"},
	})

	// min_level=WARNING drops NOTICE-only tags and keeps WARNING and ERROR.
	resp := getPublic(t, f.srv, "/pub/api/v1/analysis/tags?min_level=WARNING")
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body)
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
		t.Fatalf("min_level=WARNING should drop NOTICE tags, got %+v", tags)
	}
	if _, ok := tags["REFERRAL_SLOW"]; !ok {
		t.Fatalf("min_level=WARNING should keep WARNING tags, got %+v", tags)
	}
	if _, ok := tags["DS07_NOT_SIGNED"]; !ok {
		t.Fatalf("min_level=WARNING should keep ERROR tags, got %+v", tags)
	}

	// Invalid level is a 400.
	bad := getPublic(t, f.srv, "/pub/api/v1/analysis/tags?min_level=nonsense")
	if bad.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid min_level, got %d: %s", bad.Code, bad.Body)
	}
}

func TestPublicAnalysisTestcasesAggregates(t *testing.T) {
	f := newAnalysisAPITestFixture(t)
	ts := time.Date(2026, 4, 17, 12, 0, 0, 0, time.UTC)
	f.seedGraduatedRun("alpha.example", ts, []engine.LogEntry{
		{Module: "DNSSEC", Testcase: "dnssec07", Tag: "DS07_NOT_SIGNED", Level: "ERROR"},
		{Module: "DNSSEC", Testcase: "dnssec07", Tag: "DS07_DETAIL", Level: "NOTICE"},
	})
	f.seedGraduatedRun("beta.example", ts, []engine.LogEntry{
		{Module: "DNSSEC", Testcase: "dnssec07", Tag: "DS07_NOT_SIGNED", Level: "ERROR"},
		{Module: "BASIC", Testcase: "basic01", Tag: "B01_OK", Level: "NOTICE"},
	})

	resp := getPublic(t, f.srv, "/pub/api/v1/analysis/testcases")
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body)
	}
	var got PublicAnalysisListResponse[PublicAnalysisTestcaseView]
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Total != 2 {
		t.Fatalf("expected 2 testcases, got %d", got.Total)
	}
	byKey := map[string]PublicAnalysisTestcaseView{}
	for _, v := range got.Items {
		byKey[v.Module+"/"+v.Testcase] = v
	}
	tc := byKey["DNSSEC/dnssec07"]
	if tc.DomainCount != 2 {
		t.Fatalf("expected dnssec07 to cover 2 domains, got %d", tc.DomainCount)
	}
	if tc.WorstLevel != "ERROR" {
		t.Fatalf("expected dnssec07 worst level ERROR, got %q", tc.WorstLevel)
	}
	if tc.UniqueTags != 2 {
		t.Fatalf("expected dnssec07 2 unique tags, got %d", tc.UniqueTags)
	}
}

func TestPublicAnalysisFindingsRedactInternalIDs(t *testing.T) {
	f := newAnalysisAPITestFixture(t)
	ts := time.Date(2026, 4, 17, 12, 0, 0, 0, time.UTC)
	f.seedGraduatedRun("alpha.example", ts, []engine.LogEntry{
		{Module: "DNSSEC", Testcase: "dnssec07", Tag: "DS07_NOT_SIGNED", Level: "ERROR"},
	})

	for _, path := range []string{
		"/pub/api/v1/analysis/tags",
		"/pub/api/v1/analysis/testcases",
	} {
		resp := getPublic(t, f.srv, path)
		raw := resp.Body.String()
		for _, needle := range []string{
			`"run_id"`, `"domain_id"`, `"cohort_id"`, `"args_json"`, `"public_id"`,
		} {
			if strings.Contains(raw, needle) {
				t.Fatalf("%s should not expose %s: %s", path, needle, raw)
			}
		}
	}
}

func TestPublicAnalysisFindingsFailWithoutReadStore(t *testing.T) {
	srv := New(DefaultConfig())
	seedCohort(t, srv, AnalysisCohort{
		SourceType: "tag", SourceTag: "tld",
		AnalysisEnabled: true, PublicEnabled: true, IsDefault: true,
	})
	for _, path := range []string{
		"/pub/api/v1/analysis/tags",
		"/pub/api/v1/analysis/testcases",
	} {
		resp := getPublic(t, srv, path)
		if resp.Code != http.StatusServiceUnavailable {
			t.Fatalf("%s: expected 503 without read store, got %d", path, resp.Code)
		}
	}
}

func TestPublicAnalysisFindingListsLoadAllEntriesForRun(t *testing.T) {
	f := newAnalysisAPITestFixture(t)
	ts := time.Date(2026, 4, 17, 12, 0, 0, 0, time.UTC)
	entries := make([]engine.LogEntry, 0, 10050)
	for i := 0; i < 10050; i++ {
		entries = append(entries, engine.LogEntry{
			Module: "DNSSEC", Testcase: "bulk01", Tag: "BULK_TAG", Level: "ERROR",
		})
	}
	f.seedGraduatedRun("alpha.example", ts, entries)

	tagsResp := getPublic(t, f.srv, "/pub/api/v1/analysis/tags")
	if tagsResp.Code != http.StatusOK {
		t.Fatalf("expected 200 tags, got %d: %s", tagsResp.Code, tagsResp.Body)
	}
	var tags PublicAnalysisListResponse[PublicAnalysisTagView]
	if err := json.NewDecoder(tagsResp.Body).Decode(&tags); err != nil {
		t.Fatalf("decode tags: %v", err)
	}
	if len(tags.Items) != 1 || tags.Items[0].OccurrenceCount != 10050 {
		t.Fatalf("expected full BULK_TAG count, got %+v", tags)
	}

	testcasesResp := getPublic(t, f.srv, "/pub/api/v1/analysis/testcases")
	if testcasesResp.Code != http.StatusOK {
		t.Fatalf("expected 200 testcases, got %d: %s", testcasesResp.Code, testcasesResp.Body)
	}
	var testcases PublicAnalysisListResponse[PublicAnalysisTestcaseView]
	if err := json.NewDecoder(testcasesResp.Body).Decode(&testcases); err != nil {
		t.Fatalf("decode testcases: %v", err)
	}
	if len(testcases.Items) != 1 || testcases.Items[0].EntryCount != 10050 {
		t.Fatalf("expected full testcase count, got %+v", testcases)
	}
}
