package main

import (
	"encoding/json"
	"net/url"
	"strings"
	"testing"

	"codeberg.org/pawal/gonemaster/cmd/internal/clitest"
	"codeberg.org/pawal/gonemaster/internal/apitest"
)

// reportFixture stubs the public analysis routes with the shared report.
func reportFixture(t *testing.T, captured *url.Values) {
	t.Helper()
	catalog := apitest.AnalysisCatalog{
		DefaultTag: apitest.ReportDatasetTag,
		Cohorts:    []apitest.AnalysisCohortView{{DatasetTag: apitest.ReportDatasetTag, Label: "Kommuner", IsDefault: true}},
	}
	snapshots := apitest.SnapshotPair()
	report := apitest.SampleReport()
	apitest.StubFake(t, &newHTTPClient, apitest.Opts{
		AnalysisCatalog:     &catalog,
		AnalysisSnapshots:   &snapshots,
		AnalysisReport:      &report,
		AnalysisReportQuery: captured,
	})
}

func TestReportMarkdown(t *testing.T) {
	reportFixture(t, nil)
	res := clitest.Run(t, run, "report", apitest.ReportDatasetTag, "--from", apitest.ReportFromSlug, "--to", apitest.ReportToSlug)
	res.RequireCode(t, 0)
	res.RequireOutContains(t,
		"# kommuner: 2026-06 to 2026-09",
		"engine 1.7.0",
		"Tag vocabulary 570 to 571 tags: 1 added, 0 removed, 1 reclassified.",
		"Scoring configuration unchanged.",
		"Findings below NOTICE are not covered",
		"## Totals",
		"| Mean score | 91.07 | 91.21 |",
		"3 domains on both sides: 1 scored identically, 1 improved, 1 regressed.",
		"## Movers by cause",
		"| Real | 1 |",
		"## Clusters",
		"nameserver ns1.example.net (3 of 9)",
		"## Tags appeared",
		"| "+apitest.ReportEngineTag+" | WARNING | +2 | New in engine |",
		"| "+apitest.ReportCohortTag+" | ERROR | +1 | Cohort change |",
		"## Tags cleared",
		"| N15_SOFTWARE_VERSION | NOTICE | -1 | Removed from engine |",
		"## Tag severity changed",
		"| N11_NO_RESPONSE | WARNING | 0 | Severity reclassified |",
		"## Movers",
		"| "+apitest.ReportRealDomain+" | 88 to 76 | -12 | -12 | B to D | Real |",
		"| "+apitest.ReportMeasDomain+" | 82 to 90 | +8 | +8 | A | Measurement |",
	)
}

func TestReportJSON(t *testing.T) {
	reportFixture(t, nil)
	res := clitest.Run(t, run, "--format", "json", "report", apitest.ReportDatasetTag,
		"--from", apitest.ReportFromSlug, "--to", apitest.ReportToSlug)
	res.RequireCode(t, 0)
	var got cohortReport
	if err := json.Unmarshal([]byte(res.Out), &got); err != nil {
		t.Fatalf("decode report: %v", err)
	}
	if got.DatasetTag != apitest.ReportDatasetTag || len(got.Domains) != 2 || len(got.Clusters) != 1 {
		t.Fatalf("report decoded wrong: %+v", got)
	}
	if got.Domains[0].Category != "real" || got.Tags.Appeared[0].Classification != "new_in_engine" {
		t.Errorf("classifications lost in decode: %+v", got.Domains[0])
	}
}

// With no slugs the client compares the two newest snapshots.
func TestReportDefaultsToNewestPair(t *testing.T) {
	var captured url.Values
	reportFixture(t, &captured)
	res := clitest.Run(t, run, "report")
	res.RequireCode(t, 0)
	if captured.Get("from") != apitest.ReportFromSlug || captured.Get("to") != apitest.ReportToSlug {
		t.Fatalf("resolved pair wrong: %v", captured)
	}
}

// Only --to given: the snapshot before it becomes the baseline.
func TestReportResolvesBaselineForTo(t *testing.T) {
	var captured url.Values
	reportFixture(t, &captured)
	res := clitest.Run(t, run, "report", "--to", apitest.ReportToSlug)
	res.RequireCode(t, 0)
	if captured.Get("from") != apitest.ReportFromSlug {
		t.Fatalf("baseline wrong: %v", captured)
	}
}

func TestReportForwardsClusterBounds(t *testing.T) {
	var captured url.Values
	reportFixture(t, &captured)
	res := clitest.Run(t, run, "report", "--from", apitest.ReportFromSlug, "--to", apitest.ReportToSlug,
		"--min-cluster", "5", "--max-spread", "2")
	res.RequireCode(t, 0)
	if captured.Get("min_cluster") != "5" || captured.Get("max_spread") != "2" {
		t.Fatalf("cluster bounds not forwarded: %v", captured)
	}
}

func TestCohortsSnapshots(t *testing.T) {
	reportFixture(t, nil)
	res := clitest.Run(t, run, "cohorts", "snapshots")
	res.RequireCode(t, 0)
	res.RequireOutContains(t, "Snapshots in kommuner: 2", apitest.ReportToSlug, apitest.ReportFromSlug)
}

func TestCohortsList(t *testing.T) {
	catalog := apitest.AnalysisCatalog{
		DefaultTag: "tld",
		Cohorts: []apitest.AnalysisCohortView{
			{DatasetTag: "tld", Label: "TLDs", IsDefault: true, SnapshotCount: 13},
			{DatasetTag: apitest.ReportDatasetTag, Label: "Kommuner", SnapshotCount: 2},
		},
	}
	apitest.StubFake(t, &newHTTPClient, apitest.Opts{AnalysisCatalog: &catalog})
	res := clitest.Run(t, run, "cohorts", "list")
	res.RequireCode(t, 0)
	res.RequireOutContains(t, "Cohorts: 2", "tld", "TLDs", "snapshots=13", "(default)", "Kommuner")
}

// The listing is the discovery path, so it must not need a resolvable
// default cohort of its own.
func TestCohortsListWithoutDefault(t *testing.T) {
	catalog := apitest.AnalysisCatalog{Cohorts: []apitest.AnalysisCohortView{
		{DatasetTag: "tld"}, {DatasetTag: apitest.ReportDatasetTag},
	}}
	apitest.StubFake(t, &newHTTPClient, apitest.Opts{AnalysisCatalog: &catalog})
	res := clitest.Run(t, run, "cohorts", "list")
	res.RequireCode(t, 0)
	res.RequireOutContains(t, "Cohorts: 2")
}

func TestCohortsListJSON(t *testing.T) {
	catalog := apitest.AnalysisCatalog{DefaultTag: "tld", Cohorts: []apitest.AnalysisCohortView{
		{DatasetTag: "tld", Label: "TLDs", IsDefault: true, SnapshotCount: 13},
	}}
	apitest.StubFake(t, &newHTTPClient, apitest.Opts{AnalysisCatalog: &catalog})
	res := clitest.Run(t, run, "--format", "json", "cohorts", "list")
	res.RequireCode(t, 0)
	var got analysisCatalog
	if err := json.Unmarshal([]byte(res.Out), &got); err != nil {
		t.Fatalf("decode catalog: %v", err)
	}
	if got.DefaultTag != "tld" || len(got.Cohorts) != 1 || got.Cohorts[0].SnapshotCount != 13 {
		t.Fatalf("catalog decoded wrong: %+v", got)
	}
}

func TestReportRejectsSingleSnapshotCohort(t *testing.T) {
	catalog := apitest.AnalysisCatalog{DefaultTag: apitest.ReportDatasetTag}
	snapshots := apitest.SnapshotPair()
	snapshots.Snapshots = snapshots.Snapshots[:1]
	apitest.StubFake(t, &newHTTPClient, apitest.Opts{AnalysisCatalog: &catalog, AnalysisSnapshots: &snapshots})
	res := clitest.Run(t, run, "report")
	res.RequireCode(t, 2)
	res.RequireErrContains(t, "fewer than two snapshots")
}

func TestReportWithoutCohortNames(t *testing.T) {
	catalog := apitest.AnalysisCatalog{Cohorts: []apitest.AnalysisCohortView{
		{DatasetTag: "tld"}, {DatasetTag: "kommuner"},
	}}
	apitest.StubFake(t, &newHTTPClient, apitest.Opts{AnalysisCatalog: &catalog})
	res := clitest.Run(t, run, "report")
	res.RequireCode(t, 2)
	res.RequireErrContains(t, "name a cohort: tld, kommuner")
}

func TestPublicAPIBase(t *testing.T) {
	cases := map[string]string{
		"http://localhost:8080/api/v1":  "http://localhost:8080/pub/api/v1",
		"http://localhost:8080/api/v1/": "http://localhost:8080/pub/api/v1",
		"https://example.com":           "https://example.com/pub/api/v1",
	}
	for in, want := range cases {
		if got := publicAPIBase(in); got != want {
			t.Errorf("publicAPIBase(%q) = %q, want %q", in, got, want)
		}
	}
}

// An unknown vocabulary must never read as an unchanged one.
func TestReportVocabularyUnknownLine(t *testing.T) {
	report := apitest.SampleReport()
	report.Header.Vocabulary.FromAvailable = false
	report.Header.ScoringConfigChanged = "unknown"
	apitest.StubFake(t, &newHTTPClient, apitest.Opts{AnalysisReport: &report})
	res := clitest.Run(t, run, "report", apitest.ReportDatasetTag, "--from", apitest.ReportFromSlug, "--to", apitest.ReportToSlug)
	res.RequireCode(t, 0)
	res.RequireOutContains(t,
		"Tag vocabulary unknown on at least one side",
		"Scoring configuration provenance unknown",
	)
}

// --format takes markdown by name, and works in the subcommand position.
func TestReportFormatNames(t *testing.T) {
	for _, format := range []string{"markdown", "pretty"} {
		reportFixture(t, nil)
		res := clitest.Run(t, run, "report", "--format", format)
		res.RequireCode(t, 0)
		res.RequireOutContains(t, "# kommuner: 2026-06 to 2026-09")
	}
	reportFixture(t, nil)
	res := clitest.Run(t, run, "report", "--format", "json")
	res.RequireCode(t, 0)
	var got cohortReport
	if err := json.Unmarshal([]byte(res.Out), &got); err != nil {
		t.Fatalf("decode report: %v", err)
	}
	if got.DatasetTag != apitest.ReportDatasetTag {
		t.Errorf("subcommand --format json did not emit the response: %+v", got)
	}
}

// The global --format still reaches the command when it is given first.
func TestReportGlobalFormatMarkdown(t *testing.T) {
	reportFixture(t, nil)
	res := clitest.Run(t, run, "--format", "markdown", "report")
	res.RequireCode(t, 0)
	res.RequireOutContains(t, "# kommuner: 2026-06 to 2026-09")
}

func TestReportForwardsPaging(t *testing.T) {
	var captured url.Values
	reportFixture(t, &captured)
	res := clitest.Run(t, run, "report", "--limit", "50", "--offset", "100")
	res.RequireCode(t, 0)
	if captured.Get("limit") != "50" || captured.Get("offset") != "100" {
		t.Fatalf("paging not forwarded: %v", captured)
	}
}

// A partial page says so; a whole one stays silent.
func TestReportMarksPartialMoverPage(t *testing.T) {
	reportFixture(t, nil)
	whole := clitest.Run(t, run, "report")
	whole.RequireCode(t, 0)
	if strings.Contains(whole.Out, "Showing") {
		t.Errorf("whole page must not claim a partial list:\n%s", whole.Out)
	}

	report := apitest.SampleReport()
	report.DomainTotal = 290
	apitest.StubFake(t, &newHTTPClient, apitest.Opts{AnalysisReport: &report})
	res := clitest.Run(t, run, "report", apitest.ReportDatasetTag,
		"--from", apitest.ReportFromSlug, "--to", apitest.ReportToSlug)
	res.RequireCode(t, 0)
	res.RequireOutContains(t, "Showing 2 of 290 movers, from offset 0.")
}
