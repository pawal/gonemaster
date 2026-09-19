package main

import (
	"net/url"
	"testing"

	"codeberg.org/pawal/gonemaster/internal/apitest"
)

// reportOpts is the fake server carrying the shared report fixture.
func reportOpts(captured *url.Values) apitest.Opts {
	catalog := apitest.AnalysisCatalog{
		DefaultTag: apitest.ReportDatasetTag,
		Cohorts:    []apitest.AnalysisCohortView{{DatasetTag: apitest.ReportDatasetTag, IsDefault: true}},
	}
	snapshots := apitest.SnapshotPair()
	report := apitest.SampleReport()
	return apitest.Opts{
		AnalysisCatalog:     &catalog,
		AnalysisSnapshots:   &snapshots,
		AnalysisReport:      &report,
		AnalysisReportQuery: captured,
	}
}

func TestCohortReport(t *testing.T) {
	api := fakeAPI(t, reportOpts(nil))
	var out cohortReportOutput
	res := callTool(t, api, "cohort_report", map[string]any{
		"dataset_tag": apitest.ReportDatasetTag,
		"from":        apitest.ReportFromSlug,
		"to":          apitest.ReportToSlug,
	}, &out)
	if res.IsError {
		t.Fatalf("unexpected tool error: %s", errorText(res))
	}
	if out.DatasetTag != apitest.ReportDatasetTag || out.FromSlug != apitest.ReportFromSlug || out.ToSlug != apitest.ReportToSlug {
		t.Fatalf("pair wrong: %+v", out)
	}
	p := out.Provenance
	if !p.VocabularyKnown || p.TagsAddedToEngine != 1 || p.TagsReclassified != 1 || p.ScoringConfigChanged != "false" {
		t.Errorf("provenance wrong: %+v", p)
	}
	if p.FromEngineVersion != "1.7.0" || p.ToEngineVersion != "1.7.10" || p.TagFloor != "NOTICE" {
		t.Errorf("engine provenance wrong: %+v", p)
	}
	if out.Totals.Regressed != 1 || out.Totals.DomainCategories["real"] != 1 {
		t.Errorf("totals wrong: %+v", out.Totals)
	}
	// The engine-driven tag leads: it moved two domains, the cohort tag one.
	if len(out.TagsAppeared) != 2 || out.TagsAppeared[0].Tag != apitest.ReportEngineTag ||
		out.TagsAppeared[0].Classification != "new_in_engine" {
		t.Fatalf("appeared tags wrong: %+v", out.TagsAppeared)
	}
	if out.TagsAppeared[1].Classification != "cohort_change" {
		t.Errorf("cohort tag misclassified: %+v", out.TagsAppeared[1])
	}
	if len(out.TagsCleared) != 1 || len(out.TagsLevelChanged) != 1 {
		t.Errorf("tag lists wrong: %+v %+v", out.TagsCleared, out.TagsLevelChanged)
	}
	// Movers rank by the size of the move, so the regression leads.
	if len(out.Movers) != 2 || out.Movers[0].Domain != apitest.ReportRealDomain || out.Movers[0].Category != "real" {
		t.Fatalf("movers wrong: %+v", out.Movers)
	}
	if len(out.Movers[0].Appeared) != 1 || out.Movers[0].Appeared[0] != apitest.ReportCohortTag+" (cohort_change)" {
		t.Errorf("mover tags wrong: %+v", out.Movers[0].Appeared)
	}
	if len(out.Clusters) != 1 || out.Clusters[0].Size != 3 ||
		out.Clusters[0].Dimensions[0] != "nameserver="+apitest.ReportClusterHost+" (3 of 9)" {
		t.Fatalf("clusters wrong: %+v", out.Clusters)
	}
	if out.Truncated {
		t.Errorf("nothing was cut, truncated should be false")
	}
}

// With no arguments the tool compares the two newest snapshots of the
// default cohort.
func TestCohortReportDefaultsToNewestPair(t *testing.T) {
	var captured url.Values
	api := fakeAPI(t, reportOpts(&captured))
	var out cohortReportOutput
	res := callTool(t, api, "cohort_report", map[string]any{}, &out)
	if res.IsError {
		t.Fatalf("unexpected tool error: %s", errorText(res))
	}
	if captured.Get("from") != apitest.ReportFromSlug || captured.Get("to") != apitest.ReportToSlug {
		t.Fatalf("resolved pair wrong: %v", captured)
	}
}

func TestCohortReportTruncatesToLimit(t *testing.T) {
	api := fakeAPI(t, reportOpts(nil))
	var out cohortReportOutput
	callTool(t, api, "cohort_report", map[string]any{"limit": float64(1)}, &out)
	if len(out.TagsAppeared) != 1 || len(out.Movers) != 1 {
		t.Fatalf("limit not applied: %d tags, %d movers", len(out.TagsAppeared), len(out.Movers))
	}
	if !out.Truncated {
		t.Errorf("expected truncated=true after cutting a list")
	}
}

func TestCohortReportForwardsClusterBounds(t *testing.T) {
	var captured url.Values
	api := fakeAPI(t, reportOpts(&captured))
	var out cohortReportOutput
	callTool(t, api, "cohort_report", map[string]any{
		"min_cluster": float64(5), "max_spread": float64(2),
	}, &out)
	if captured.Get("min_cluster") != "5" || captured.Get("max_spread") != "2" {
		t.Fatalf("cluster bounds not forwarded: %v", captured)
	}
}

func TestCohortReportNeedsTwoSnapshots(t *testing.T) {
	opts := reportOpts(nil)
	single := apitest.SnapshotPair()
	single.Snapshots = single.Snapshots[:1]
	opts.AnalysisSnapshots = &single
	api := fakeAPI(t, opts)
	res := callTool(t, api, "cohort_report", map[string]any{}, nil)
	if !res.IsError {
		t.Fatalf("expected a tool error for a single-snapshot cohort")
	}
	if got := errorText(res); got == "" {
		t.Errorf("expected an explanation, got none")
	}
}

// An unreadable vocabulary must not read as an unchanged one.
func TestCohortReportUnknownVocabulary(t *testing.T) {
	opts := reportOpts(nil)
	report := apitest.SampleReport()
	report.Header.Vocabulary.ToAvailable = false
	report.Header.ScoringConfigChanged = "unknown"
	opts.AnalysisReport = &report
	api := fakeAPI(t, opts)
	var out cohortReportOutput
	callTool(t, api, "cohort_report", map[string]any{}, &out)
	if out.Provenance.VocabularyKnown {
		t.Errorf("vocabulary should be unknown: %+v", out.Provenance)
	}
	if out.Provenance.ScoringConfigChanged != "unknown" {
		t.Errorf("scoring state should propagate as unknown: %+v", out.Provenance)
	}
}

func TestPublicBaseURL(t *testing.T) {
	cases := map[string]string{
		"http://localhost:8080/api/v1":  "http://localhost:8080/pub/api/v1",
		"http://localhost:8080/api/v1/": "http://localhost:8080/pub/api/v1",
		"https://example.com":           "https://example.com/pub/api/v1",
	}
	for in, want := range cases {
		if got := publicBaseURL(in); got != want {
			t.Errorf("publicBaseURL(%q) = %q, want %q", in, got, want)
		}
	}
}
