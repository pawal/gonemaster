package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// ── Public API addressing ────────────────────────────────────────────────────

// publicAPIBase maps the admin API base onto the public API base.
func publicAPIBase(base string) string {
	trimmed := strings.TrimRight(base, "/")
	if strings.HasSuffix(trimmed, "/api/v1") {
		return strings.TrimSuffix(trimmed, "/api/v1") + "/pub/api/v1"
	}
	return trimmed + "/pub/api/v1"
}

// getPublic performs a GET against the public API, which takes no token.
func (c *apiClient) getPublic(ctx context.Context, path string, out any) error {
	return c.doJSONURL(ctx, http.MethodGet, publicAPIBase(c.baseURL)+path, nil, out)
}

// ── Response types ───────────────────────────────────────────────────────────

type cohortView struct {
	DatasetTag    string `json:"dataset_tag"`
	Label         string `json:"label"`
	IsDefault     bool   `json:"is_default"`
	SnapshotCount int    `json:"snapshot_count"`
}

type analysisCatalog struct {
	DefaultTag string       `json:"default_tag,omitempty"`
	Cohorts    []cohortView `json:"cohorts"`
}

type snapshotEntry struct {
	Slug        string `json:"slug"`
	Label       string `json:"label,omitempty"`
	CapturedAt  string `json:"captured_at"`
	DomainCount int    `json:"domain_count"`
}

type snapshotList struct {
	DatasetTag string          `json:"dataset_tag"`
	Snapshots  []snapshotEntry `json:"snapshots"`
}

type vocabularyEntry struct {
	Tag    string `json:"tag"`
	Module string `json:"module,omitempty"`
	Level  string `json:"level,omitempty"`
}

type vocabularyChange struct {
	Tag       string `json:"tag"`
	Module    string `json:"module,omitempty"`
	FromLevel string `json:"from_level"`
	ToLevel   string `json:"to_level"`
}

type vocabularyDelta struct {
	FromAvailable bool               `json:"from_available"`
	ToAvailable   bool               `json:"to_available"`
	FromTagCount  int                `json:"from_tag_count"`
	ToTagCount    int                `json:"to_tag_count"`
	Added         []vocabularyEntry  `json:"added"`
	Removed       []vocabularyEntry  `json:"removed"`
	LevelChanged  []vocabularyChange `json:"level_changed"`
}

type reportSide struct {
	Slug              string `json:"slug"`
	Label             string `json:"label,omitempty"`
	CapturedAt        string `json:"captured_at"`
	EngineVersion     string `json:"engine_version,omitempty"`
	ProfileName       string `json:"profile_name,omitempty"`
	ScoringConfigHash string `json:"scoring_config_hash,omitempty"`
	TagViewMinLevel   string `json:"tag_view_min_level,omitempty"`
	DomainCount       int    `json:"domain_count"`
}

type reportEngineDelta struct {
	FromEngineVersion string `json:"from_engine_version,omitempty"`
	ToEngineVersion   string `json:"to_engine_version,omitempty"`
	Crossed           bool   `json:"crossed_engine_versions"`
	Unknown           bool   `json:"engine_version_unknown,omitempty"`
}

type reportHeader struct {
	From                 reportSide        `json:"from"`
	To                   reportSide        `json:"to"`
	Engine               reportEngineDelta `json:"engine"`
	Vocabulary           vocabularyDelta   `json:"vocabulary"`
	ScoringConfigChanged string            `json:"scoring_config_changed"`
	TagFloor             string            `json:"tag_floor,omitempty"`
}

type reportTotals struct {
	FromDomainCount  int            `json:"from_domain_count"`
	ToDomainCount    int            `json:"to_domain_count"`
	BothDomainCount  int            `json:"both_domain_count"`
	Added            int            `json:"added"`
	Removed          int            `json:"removed"`
	IdenticalScore   int            `json:"identical_score"`
	Improved         int            `json:"improved"`
	Regressed        int            `json:"regressed"`
	FromMeanScore    *float64       `json:"from_mean_score,omitempty"`
	ToMeanScore      *float64       `json:"to_mean_score,omitempty"`
	FromGrades       map[string]int `json:"from_grades,omitempty"`
	ToGrades         map[string]int `json:"to_grades,omitempty"`
	DomainCategories map[string]int `json:"domain_categories,omitempty"`
}

type reportTagEntry struct {
	Tag             string `json:"tag"`
	Module          string `json:"module,omitempty"`
	Testcase        string `json:"testcase,omitempty"`
	FromLevel       string `json:"from_level,omitempty"`
	ToLevel         string `json:"to_level,omitempty"`
	FromDomainCount int    `json:"from_domain_count"`
	ToDomainCount   int    `json:"to_domain_count"`
	DomainDelta     int    `json:"domain_delta"`
	Classification  string `json:"classification"`
}

type reportTags struct {
	Appeared     []reportTagEntry `json:"appeared"`
	Cleared      []reportTagEntry `json:"cleared"`
	LevelChanged []reportTagEntry `json:"level_changed"`
}

type reportTagChange struct {
	Tag            string `json:"tag"`
	Module         string `json:"module,omitempty"`
	Testcase       string `json:"testcase,omitempty"`
	FromLevel      string `json:"from_level,omitempty"`
	ToLevel        string `json:"to_level,omitempty"`
	Classification string `json:"classification"`
}

type reportDomain struct {
	Domain           string            `json:"domain"`
	FromScore        *int              `json:"from_score,omitempty"`
	ToScore          *int              `json:"to_score,omitempty"`
	ScoreDelta       *int              `json:"score_delta,omitempty"`
	FromGrade        string            `json:"from_grade,omitempty"`
	ToGrade          string            `json:"to_grade,omitempty"`
	GradeChanged     bool              `json:"grade_changed"`
	Category         string            `json:"category"`
	ExplainedDelta   int               `json:"explained_delta"`
	UnexplainedDelta int               `json:"unexplained_delta"`
	Appeared         []reportTagChange `json:"appeared"`
	Cleared          []reportTagChange `json:"cleared"`
	LevelChanged     []reportTagChange `json:"level_changed"`
}

type reportClusterDimension struct {
	Dimension    string `json:"dimension"`
	Value        string `json:"value"`
	Label        string `json:"label,omitempty"`
	TotalDomains int    `json:"total_domains"`
}

type reportCluster struct {
	Dimensions []reportClusterDimension `json:"dimensions"`
	Domains    []string                 `json:"domains"`
	Size       int                      `json:"size"`
	MinDelta   int                      `json:"min_delta"`
	MaxDelta   int                      `json:"max_delta"`
	Direction  string                   `json:"direction"`
}

type cohortReport struct {
	DatasetTag   string          `json:"dataset_tag"`
	FromSlug     string          `json:"from_slug"`
	ToSlug       string          `json:"to_slug"`
	MinCluster   int             `json:"min_cluster"`
	MaxSpread    int             `json:"max_spread"`
	Header       reportHeader    `json:"header"`
	Totals       reportTotals    `json:"totals"`
	Tags         reportTags      `json:"tags"`
	Domains      []reportDomain  `json:"domains"`
	Clusters     []reportCluster `json:"clusters"`
	DomainTotal  int             `json:"domain_total"`
	DomainLimit  int             `json:"domain_limit,omitempty"`
	DomainOffset int             `json:"domain_offset,omitempty"`
}

// ── Labels ───────────────────────────────────────────────────────────────────

var classificationLabels = map[string]string{
	"cohort_change":       "Cohort change",
	"new_in_engine":       "New in engine",
	"removed_from_engine": "Removed from engine",
	"level_reclassified":  "Severity reclassified",
	"unknown":             "Unknown",
}

var categoryLabels = map[string]string{
	"real":        "Real",
	"mixed":       "Mixed",
	"measurement": "Measurement",
	"unknown":     "Unknown",
}

// categoryOrder lists the causes from most to least actionable.
var categoryOrder = []string{"real", "mixed", "measurement", "unknown"}

func classificationLabel(name string) string {
	if label, ok := classificationLabels[name]; ok {
		return label
	}
	return name
}

func categoryLabel(name string) string {
	if label, ok := categoryLabels[name]; ok {
		return label
	}
	return name
}

// ── Fetching ─────────────────────────────────────────────────────────────────

// resolveReportCohort returns the requested dataset tag, or the catalog's
// default when none was given.
func resolveReportCohort(ctx context.Context, client *apiClient, datasetTag string) (string, error) {
	if datasetTag != "" {
		return datasetTag, nil
	}
	var catalog analysisCatalog
	if err := client.getPublic(ctx, "/analysis/catalog", &catalog); err != nil {
		return "", err
	}
	if catalog.DefaultTag != "" {
		return catalog.DefaultTag, nil
	}
	if len(catalog.Cohorts) == 1 {
		return catalog.Cohorts[0].DatasetTag, nil
	}
	names := make([]string, 0, len(catalog.Cohorts))
	for _, c := range catalog.Cohorts {
		names = append(names, c.DatasetTag)
	}
	if len(names) == 0 {
		return "", fmt.Errorf("no public analysis cohort is available")
	}
	return "", fmt.Errorf("name a cohort: %s", strings.Join(names, ", "))
}

// resolveReportPair fills a missing slug from the snapshot list: newest as
// the later side, the one before it as the baseline.
func resolveReportPair(ctx context.Context, client *apiClient, datasetTag, from, to string) (string, string, error) {
	if from != "" && to != "" {
		return from, to, nil
	}
	var list snapshotList
	path := "/analysis/cohorts/" + url.PathEscape(datasetTag) + "/snapshots"
	if err := client.getPublic(ctx, path, &list); err != nil {
		return "", "", err
	}
	if len(list.Snapshots) < 2 {
		return "", "", fmt.Errorf("cohort %s has fewer than two snapshots to compare", datasetTag)
	}
	if to == "" {
		to = list.Snapshots[0].Slug
	}
	if from != "" {
		return from, to, nil
	}
	for i, snap := range list.Snapshots {
		if snap.Slug == to && i+1 < len(list.Snapshots) {
			return list.Snapshots[i+1].Slug, to, nil
		}
	}
	return "", "", fmt.Errorf("no snapshot precedes %s in cohort %s", to, datasetTag)
}

func fetchCohortReport(ctx context.Context, client *apiClient, datasetTag, from, to string, minCluster, maxSpread, limit, offset int) (cohortReport, error) {
	q := url.Values{}
	q.Set("from", from)
	q.Set("to", to)
	if minCluster > 0 {
		q.Set("min_cluster", strconv.Itoa(minCluster))
	}
	if maxSpread > 0 {
		q.Set("max_spread", strconv.Itoa(maxSpread))
	}
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
	if offset > 0 {
		q.Set("offset", strconv.Itoa(offset))
	}
	path := "/analysis/cohorts/" + url.PathEscape(datasetTag) + "/report?" + q.Encode()
	var report cohortReport
	if err := client.getPublic(ctx, path, &report); err != nil {
		return cohortReport{}, err
	}
	return report, nil
}

// ── Markdown ─────────────────────────────────────────────────────────────────

func signedNumber(value int) string {
	if value > 0 {
		return "+" + strconv.Itoa(value)
	}
	return strconv.Itoa(value)
}

func signedDelta(value *int) string {
	if value == nil {
		return ""
	}
	return signedNumber(*value)
}

func optionalInt(value *int) string {
	if value == nil {
		return ""
	}
	return strconv.Itoa(*value)
}

func optionalFloat(value *float64) string {
	if value == nil {
		return ""
	}
	return strconv.FormatFloat(*value, 'f', 2, 64)
}

// markdownCell escapes the one character that breaks a table row.
func markdownCell(value string) string {
	return strings.ReplaceAll(value, "|", "\\|")
}

func markdownTable(headers []string, rows [][]string) []string {
	if len(rows) == 0 {
		return nil
	}
	lines := []string{
		"| " + strings.Join(headers, " | ") + " |",
		"|" + strings.Repeat(" --- |", len(headers)),
	}
	for _, row := range rows {
		cells := make([]string, len(row))
		for i, cell := range row {
			cells[i] = markdownCell(cell)
		}
		lines = append(lines, "| "+strings.Join(cells, " | ")+" |")
	}
	return append(lines, "")
}

func scoringLine(state string) string {
	switch state {
	case "true":
		return "Scoring configuration changed between the two snapshots."
	case "false":
		return "Scoring configuration unchanged."
	default:
		return "Scoring configuration provenance unknown; a score move cannot be fully attributed."
	}
}

func vocabularyLine(delta vocabularyDelta) string {
	if !delta.FromAvailable || !delta.ToAvailable {
		return "Tag vocabulary unknown on at least one side; no change can be attributed to the engine."
	}
	return fmt.Sprintf("Tag vocabulary %d to %d tags: %d added, %d removed, %d reclassified.",
		delta.FromTagCount, delta.ToTagCount, len(delta.Added), len(delta.Removed), len(delta.LevelChanged))
}

// clusterDimensions names the dimensions a cluster was detected over.
func clusterDimensions(cluster reportCluster) string {
	parts := make([]string, 0, len(cluster.Dimensions))
	for _, d := range cluster.Dimensions {
		value := d.Label
		if value == "" {
			value = d.Value
		}
		parts = append(parts, fmt.Sprintf("%s %s (%d of %d)", d.Dimension, value, cluster.Size, d.TotalDomains))
	}
	return strings.Join(parts, ", ")
}

func tagRows(entries []reportTagEntry, fromLevel bool) [][]string {
	rows := make([][]string, 0, len(entries))
	for _, e := range entries {
		level := e.ToLevel
		if fromLevel {
			level = e.FromLevel
		}
		rows = append(rows, []string{e.Tag, level, signedNumber(e.DomainDelta), classificationLabel(e.Classification)})
	}
	return rows
}

// reportMarkdown renders the report in the order the analysis UI shows it.
func reportMarkdown(report cohortReport) string {
	header, totals := report.Header, report.Totals
	var lines []string

	lines = append(lines,
		fmt.Sprintf("# %s: %s to %s", report.DatasetTag, report.FromSlug, report.ToSlug), "",
		fmt.Sprintf("From %s (engine %s, %d domains) to %s (engine %s, %d domains).",
			header.From.Slug, orUnknown(header.From.EngineVersion), header.From.DomainCount,
			header.To.Slug, orUnknown(header.To.EngineVersion), header.To.DomainCount), "",
		"- "+vocabularyLine(header.Vocabulary),
		"- "+scoringLine(header.ScoringConfigChanged))
	if header.TagFloor != "" {
		lines = append(lines, fmt.Sprintf("- Findings below %s are not covered by the per-domain lists.", header.TagFloor))
	}
	lines = append(lines, "", "## Totals", "")
	lines = append(lines, markdownTable([]string{"Metric", "From", "To"}, [][]string{
		{"Domains", strconv.Itoa(totals.FromDomainCount), strconv.Itoa(totals.ToDomainCount)},
		{"Mean score", optionalFloat(totals.FromMeanScore), optionalFloat(totals.ToMeanScore)},
	})...)
	lines = append(lines, fmt.Sprintf(
		"%d domains on both sides: %d scored identically, %d improved, %d regressed. %d added, %d removed.",
		totals.BothDomainCount, totals.IdenticalScore, totals.Improved, totals.Regressed,
		totals.Added, totals.Removed), "")

	var categoryRows [][]string
	for _, category := range categoryOrder {
		if count := totals.DomainCategories[category]; count > 0 {
			categoryRows = append(categoryRows, []string{categoryLabel(category), strconv.Itoa(count)})
		}
	}
	if len(categoryRows) > 0 {
		lines = append(lines, "## Movers by cause", "")
		lines = append(lines, markdownTable([]string{"Cause", "Domains"}, categoryRows)...)
	}

	if len(report.Clusters) > 0 {
		rows := make([][]string, 0, len(report.Clusters))
		for _, c := range report.Clusters {
			rows = append(rows, []string{
				clusterDimensions(c), strconv.Itoa(c.Size),
				signedNumber(c.MinDelta) + " to " + signedNumber(c.MaxDelta),
				strings.Join(c.Domains, ", "),
			})
		}
		lines = append(lines, "## Clusters", "")
		lines = append(lines, markdownTable([]string{"Dimensions", "Domains", "Score move", "Members"}, rows)...)
	}

	sections := []struct {
		title     string
		entries   []reportTagEntry
		fromLevel bool
	}{
		{"Tags appeared", report.Tags.Appeared, false},
		{"Tags cleared", report.Tags.Cleared, true},
		{"Tag severity changed", report.Tags.LevelChanged, false},
	}
	for _, section := range sections {
		if len(section.entries) == 0 {
			continue
		}
		lines = append(lines, "## "+section.title, "")
		lines = append(lines, markdownTable(
			[]string{"Tag", "Level", "Domains", "Classification"},
			tagRows(section.entries, section.fromLevel))...)
	}

	if len(report.Domains) > 0 {
		rows := make([][]string, 0, len(report.Domains))
		for _, d := range report.Domains {
			grade := d.ToGrade
			if d.GradeChanged {
				grade = d.FromGrade + " to " + d.ToGrade
			}
			rows = append(rows, []string{
				d.Domain,
				optionalInt(d.FromScore) + " to " + optionalInt(d.ToScore),
				signedDelta(d.ScoreDelta),
				signedNumber(d.ExplainedDelta),
				grade,
				categoryLabel(d.Category),
			})
		}
		lines = append(lines, "## Movers", "")
		lines = append(lines, markdownTable(
			[]string{"Domain", "Score", "Delta", "Explained", "Grade", "Cause"}, rows)...)
		if report.DomainTotal > len(report.Domains) {
			lines = append(lines, fmt.Sprintf("Showing %d of %d movers, from offset %d.",
				len(report.Domains), report.DomainTotal, report.DomainOffset), "")
		}
	}

	return strings.TrimRight(strings.Join(lines, "\n"), "\n") + "\n"
}

func orUnknown(value string) string {
	if value == "" {
		return "unknown"
	}
	return value
}

// ── Command ──────────────────────────────────────────────────────────────────

func runReport(ctx context.Context, client *apiClient, opts globalOptions, args []string, out io.Writer, errOut io.Writer) int {
	fs := flag.NewFlagSet("report", flag.ContinueOnError)
	fs.SetOutput(errOut)
	setSubcommandUsage(fs)
	var from, to string
	var minCluster, maxSpread, limit, offset int
	fs.StringVar(&from, "from", "", "Baseline snapshot slug (default: the snapshot before --to)")
	fs.StringVar(&to, "to", "", "Later snapshot slug (default: the newest snapshot)")
	fs.IntVar(&minCluster, "min-cluster", 0, "Domains a cluster needs (server default 3)")
	fs.IntVar(&maxSpread, "max-spread", 0, "Score spread a cluster allows (server default 3)")
	fs.IntVar(&limit, "limit", 0, "Max movers to list (server default 500)")
	fs.IntVar(&offset, "offset", 0, "First mover to list")
	fs.StringVar(&opts.format, "format", opts.format, "Output format: markdown (default), json")
	if err := parseWithReorderedFlags(fs, args); err != nil {
		return parseExit(err)
	}
	datasetTag := ""
	if rest := fs.Args(); len(rest) > 0 {
		datasetTag = strings.TrimSpace(rest[0])
	}
	datasetTag, err := resolveReportCohort(ctx, client, datasetTag)
	if err != nil {
		fmt.Fprintln(errOut, err.Error())
		return 2
	}
	from, to, err = resolveReportPair(ctx, client, datasetTag, strings.TrimSpace(from), strings.TrimSpace(to))
	if err != nil {
		fmt.Fprintln(errOut, err.Error())
		return 2
	}
	report, err := fetchCohortReport(ctx, client, datasetTag, from, to, minCluster, maxSpread, limit, offset)
	if err != nil {
		fmt.Fprintln(errOut, err.Error())
		return 2
	}
	if isJSONFormat(opts.format) {
		if err := writeOutput(out, opts.format, report); err != nil {
			fmt.Fprintln(errOut, err.Error())
			return 2
		}
		return 0
	}
	fmt.Fprint(out, reportMarkdown(report))
	return 0
}

// isJSONFormat reports whether the format asks for the raw response. The
// report's pretty rendering is Markdown, so markdown names it too.
func isJSONFormat(format string) bool {
	return format == "json" || format == "jsonl"
}

// ── cohorts ──────────────────────────────────────────────────────────────────

func runCohorts(ctx context.Context, client *apiClient, opts globalOptions, args []string, out io.Writer, errOut io.Writer) int {
	const subs = "list|snapshots"
	if len(args) == 0 {
		fmt.Fprintln(errOut, "cohorts subcommand is required: "+subs)
		return 2
	}
	if isHelpArg(args[0]) {
		return groupUsage(out, "cohorts", subs)
	}
	cmd := args[0]
	args = args[1:]
	switch cmd {
	case "list":
		return runCohortsList(ctx, client, opts, args, out, errOut)
	case "snapshots":
		return runCohortsSnapshots(ctx, client, opts, args, out, errOut)
	default:
		fmt.Fprintf(errOut, "Unknown cohorts command %q\n", cmd)
		return 2
	}
}

// runCohortsList names the cohorts a report can be run over.
func runCohortsList(ctx context.Context, client *apiClient, opts globalOptions, args []string, out io.Writer, errOut io.Writer) int {
	fs := flag.NewFlagSet("cohorts list", flag.ContinueOnError)
	fs.SetOutput(errOut)
	setSubcommandUsage(fs)
	if err := parseWithReorderedFlags(fs, args); err != nil {
		return parseExit(err)
	}
	var catalog analysisCatalog
	if err := client.getPublic(ctx, "/analysis/catalog", &catalog); err != nil {
		fmt.Fprintln(errOut, err.Error())
		return 2
	}
	if isJSONFormat(opts.format) {
		if err := writeOutput(out, opts.format, catalog); err != nil {
			fmt.Fprintln(errOut, err.Error())
			return 2
		}
		return 0
	}
	fmt.Fprintf(out, "Cohorts: %d\n", len(catalog.Cohorts))
	for _, c := range catalog.Cohorts {
		marker := ""
		if c.IsDefault || c.DatasetTag == catalog.DefaultTag {
			marker = "  (default)"
		}
		fmt.Fprintf(out, "  %-24s  %-24s  snapshots=%-4d%s\n",
			c.DatasetTag, c.Label, c.SnapshotCount, marker)
	}
	return 0
}

// runCohortsSnapshots lists one cohort's snapshot slugs, which are what the
// report compares.
func runCohortsSnapshots(ctx context.Context, client *apiClient, opts globalOptions, args []string, out io.Writer, errOut io.Writer) int {
	fs := flag.NewFlagSet("cohorts snapshots", flag.ContinueOnError)
	fs.SetOutput(errOut)
	setSubcommandUsage(fs)
	if err := parseWithReorderedFlags(fs, args); err != nil {
		return parseExit(err)
	}
	datasetTag := ""
	if rest := fs.Args(); len(rest) > 0 {
		datasetTag = strings.TrimSpace(rest[0])
	}
	datasetTag, err := resolveReportCohort(ctx, client, datasetTag)
	if err != nil {
		fmt.Fprintln(errOut, err.Error())
		return 2
	}
	var list snapshotList
	path := "/analysis/cohorts/" + url.PathEscape(datasetTag) + "/snapshots"
	if err := client.getPublic(ctx, path, &list); err != nil {
		fmt.Fprintln(errOut, err.Error())
		return 2
	}
	if isJSONFormat(opts.format) {
		if err := writeOutput(out, opts.format, list); err != nil {
			fmt.Fprintln(errOut, err.Error())
			return 2
		}
		return 0
	}
	fmt.Fprintf(out, "Snapshots in %s: %d\n", datasetTag, len(list.Snapshots))
	for _, snap := range list.Snapshots {
		captured := snap.CapturedAt
		if len(captured) > 10 {
			captured = captured[:10]
		}
		fmt.Fprintf(out, "  %-24s  %-10s  domains=%d\n", snap.Slug, captured, snap.DomainCount)
	}
	return 0
}
