package main

import (
	"fmt"
	"net/url"
	"strings"
	"testing"

	"codeberg.org/pawal/gonemaster/internal/apitest"
)

func TestCohortReport(t *testing.T) {
	api := fakeAPI(t, apitest.ReportOpts(nil))
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
	api := fakeAPI(t, apitest.ReportOpts(&captured))
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
	api := fakeAPI(t, apitest.ReportOpts(nil))
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
	api := fakeAPI(t, apitest.ReportOpts(&captured))
	var out cohortReportOutput
	callTool(t, api, "cohort_report", map[string]any{
		"min_cluster": float64(5), "max_spread": float64(2),
	}, &out)
	if captured.Get("min_cluster") != "5" || captured.Get("max_spread") != "2" {
		t.Fatalf("cluster bounds not forwarded: %v", captured)
	}
}

func TestCohortReportNeedsTwoSnapshots(t *testing.T) {
	opts := apitest.ReportOpts(nil)
	single := apitest.SnapshotPair()
	single.Snapshots = single.Snapshots[:1]
	opts.AnalysisSnapshots = &single
	api := fakeAPI(t, opts)
	res := callTool(t, api, "cohort_report", map[string]any{}, nil)
	if !res.IsError {
		t.Fatalf("expected a tool error for a single-snapshot cohort")
	}
	if got := errorText(res); !strings.Contains(got, "fewer than two snapshots to compare") {
		t.Errorf("error = %q, want the snapshot count", got)
	}
}

func TestCohortReportNamesCohortsWithoutDefault(t *testing.T) {
	catalog := apitest.AnalysisCatalog{Cohorts: []apitest.AnalysisCohortView{
		{DatasetTag: "tld"}, {DatasetTag: apitest.ReportDatasetTag},
	}}
	api := fakeAPI(t, apitest.Opts{AnalysisCatalog: &catalog})
	res := callTool(t, api, "cohort_report", map[string]any{}, nil)
	if !res.IsError {
		t.Fatalf("expected a tool error without a default cohort")
	}
	if got := errorText(res); !strings.Contains(got, "dataset_tag is required; available: tld, kommuner") {
		t.Errorf("error = %q, want the cohort names", got)
	}
}

// An unreadable vocabulary must not read as an unchanged one.
func TestCohortReportUnknownVocabulary(t *testing.T) {
	opts := apitest.ReportOpts(nil)
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

func TestCohortReportErrors(t *testing.T) {
	withoutReport := apitest.ReportOpts(nil)
	withoutReport.AnalysisReport = nil

	cases := []struct {
		name string
		opts apitest.Opts
		args map[string]any
		want string
	}{
		{
			"no cohort", apitest.Opts{}, map[string]any{},
			"no public analysis cohort is available",
		},
		{
			"no baseline", apitest.ReportOpts(nil), map[string]any{"to": apitest.ReportFromSlug},
			"no snapshot precedes " + apitest.ReportFromSlug + " in cohort " + apitest.ReportDatasetTag,
		},
		{
			"report missing", withoutReport, map[string]any{},
			"get cohort report failed: not found (404)",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			api := fakeAPI(t, tc.opts)
			res := callTool(t, api, "cohort_report", tc.args, nil)
			if !res.IsError {
				t.Fatalf("expected a tool error")
			}
			if got := errorText(res); got != tc.want {
				t.Errorf("error = %q, want %q", got, tc.want)
			}
		})
	}
}

// The lists stay at maxReportRows however large the requested limit is.
func TestCohortReportClampsLimitToMax(t *testing.T) {
	opts := apitest.ReportOpts(nil)
	report := apitest.SampleReport()
	report.Domains = nil
	for i := range maxReportRows + 50 {
		score, delta := 100, -(i + 1)
		report.Domains = append(report.Domains, apitest.AnalysisReportDomain{
			Domain:     fmt.Sprintf("d%03d.example", i),
			FromScore:  &score,
			ScoreDelta: &delta,
			Category:   "real",
		})
	}
	report.DomainTotal = len(report.Domains)
	opts.AnalysisReport = &report

	var out cohortReportOutput
	callTool(t, fakeAPI(t, opts), "cohort_report", map[string]any{"limit": float64(maxReportRows + 300)}, &out)
	if len(out.Movers) != maxReportRows {
		t.Fatalf("movers = %d, want %d", len(out.Movers), maxReportRows)
	}
	if !out.Truncated {
		t.Errorf("expected truncated=true after the clamp cut the list")
	}
	if out.Movers[0].Domain != "d249.example" {
		t.Errorf("first mover = %q, want d249.example, the largest move", out.Movers[0].Domain)
	}
}
