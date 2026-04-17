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
	f.t.Helper()
	runID := "run-" + domainName + "-" + finishedAt.Format("20060102150405")
	now := finishedAt
	job := Job{
		ID:         runID,
		Domain:     domainName,
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
