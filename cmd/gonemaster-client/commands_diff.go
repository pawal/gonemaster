package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

// levelRank mirrors the engine's severity ordering. An unranked level would
// tie with every other unranked one and look like a severity change.
var levelRank = map[string]int{
	"DEBUG3":   1,
	"DEBUG2":   2,
	"DEBUG":    3,
	"INFO":     4,
	"NOTICE":   5,
	"WARNING":  6,
	"ERROR":    7,
	"CRITICAL": 8,
}

type tagInfo struct {
	Module string `json:"module,omitempty"`
	Level  string `json:"level,omitempty"`
}

type tagDelta struct {
	Tag       string `json:"tag"`
	Module    string `json:"module,omitempty"`
	Level     string `json:"level,omitempty"`
	FromLevel string `json:"from_level,omitempty"`
	ToLevel   string `json:"to_level,omitempty"`
}

type runDiffOutput struct {
	RunA    string     `json:"run_a"`
	RunB    string     `json:"run_b"`
	DomainA string     `json:"domain_a,omitempty"`
	DomainB string     `json:"domain_b,omitempty"`
	GradeA  string     `json:"grade_a,omitempty"`
	GradeB  string     `json:"grade_b,omitempty"`
	Added   []tagDelta `json:"added"`
	Removed []tagDelta `json:"removed"`
	Changed []tagDelta `json:"changed"`
}

// Identical reports whether every tag kept its worst level.
func (d runDiffOutput) Identical() bool {
	return len(d.Added) == 0 && len(d.Removed) == 0 && len(d.Changed) == 0
}

// worstLevelByTag keeps the highest-severity level seen per tag, matching
// the MCP run_diff tool.
func worstLevelByTag(result jobResult) map[string]tagInfo {
	m := map[string]tagInfo{}
	if result.Raw == nil {
		return m
	}
	for _, e := range result.Raw.Entries {
		cur, ok := m[e.Tag]
		if !ok || levelRank[e.Level] > levelRank[cur.Level] {
			m[e.Tag] = tagInfo{Module: e.Module, Level: e.Level}
		}
	}
	return m
}

func diffTagMaps(a, b map[string]tagInfo) (added, removed, changed []tagDelta) {
	added, removed, changed = []tagDelta{}, []tagDelta{}, []tagDelta{}
	for tag, ib := range b {
		ia, ok := a[tag]
		if !ok {
			added = append(added, tagDelta{Tag: tag, Module: ib.Module, Level: ib.Level})
		} else if ia.Level != ib.Level {
			changed = append(changed, tagDelta{Tag: tag, Module: ib.Module, FromLevel: ia.Level, ToLevel: ib.Level})
		}
	}
	for tag, ia := range a {
		if _, ok := b[tag]; !ok {
			removed = append(removed, tagDelta{Tag: tag, Module: ia.Module, Level: ia.Level})
		}
	}
	sortDeltas(added)
	sortDeltas(removed)
	sortDeltas(changed)
	return added, removed, changed
}

func sortDeltas(d []tagDelta) {
	sort.Slice(d, func(i, j int) bool { return d[i].Tag < d[j].Tag })
}

func runRunsDiff(ctx context.Context, client *apiClient, opts globalOptions, args []string, out io.Writer, errOut io.Writer) int {
	fs := flag.NewFlagSet("runs diff", flag.ContinueOnError)
	fs.SetOutput(errOut)
	setSubcommandUsage(fs)
	var quiet bool
	fs.BoolVar(&quiet, "quiet", false, "Report through the exit status only")
	if err := parseWithReorderedFlags(fs, args); err != nil {
		return parseExit(err)
	}
	rest := fs.Args()
	if len(rest) < 2 {
		fmt.Fprintln(errOut, "two run IDs are required: runs diff <run-a> <run-b>")
		return 2
	}
	runA := strings.TrimSpace(rest[0])
	runB := strings.TrimSpace(rest[1])

	diff, err := fetchRunDiff(ctx, client, opts, runA, runB)
	if err != nil {
		fmt.Fprintln(errOut, err.Error())
		return 2
	}

	if quiet {
		if diff.Identical() {
			return 0
		}
		return 1
	}
	if opts.format != "pretty" {
		if err := writeOutput(out, opts.format, diff); err != nil {
			fmt.Fprintln(errOut, err.Error())
			return 2
		}
		if diff.Identical() {
			return 0
		}
		return 1
	}
	writeRunDiffPretty(out, diff)
	if diff.Identical() {
		return 0
	}
	return 1
}

type batchDiffDomain struct {
	Domain  string     `json:"domain"`
	RunA    string     `json:"run_a"`
	RunB    string     `json:"run_b"`
	GradeA  string     `json:"grade_a,omitempty"`
	GradeB  string     `json:"grade_b,omitempty"`
	Added   []tagDelta `json:"added"`
	Removed []tagDelta `json:"removed"`
	Changed []tagDelta `json:"changed"`
}

type batchDiffOutput struct {
	BatchA string `json:"batch_a"`
	BatchB string `json:"batch_b"`
	// Domains in both batches; the rest are reported apart, not dropped.
	Compared     int               `json:"compared"`
	Identical    int               `json:"identical"`
	Differing    int               `json:"differing"`
	OnlyInA      []string          `json:"only_in_a,omitempty"`
	OnlyInB      []string          `json:"only_in_b,omitempty"`
	Unusable     []string          `json:"unusable,omitempty"`
	TagAdded     map[string]int    `json:"tag_added,omitempty"`
	TagRemoved   map[string]int    `json:"tag_removed,omitempty"`
	TagChanged   map[string]int    `json:"tag_changed,omitempty"`
	DomainDeltas []batchDiffDomain `json:"domain_deltas,omitempty"`
}

// Clean reports agreement over a complete comparison. An unmatched or
// unusable domain is missing data, not agreement.
func (d batchDiffOutput) Clean() bool {
	return d.Differing == 0 && len(d.OnlyInA) == 0 && len(d.OnlyInB) == 0 && len(d.Unusable) == 0
}

func runBatchesDiff(ctx context.Context, client *apiClient, opts globalOptions, args []string, out io.Writer, errOut io.Writer) int {
	fs := flag.NewFlagSet("batches diff", flag.ContinueOnError)
	fs.SetOutput(errOut)
	setSubcommandUsage(fs)
	var quiet, perDomain bool
	var limit int
	fs.BoolVar(&quiet, "quiet", false, "Report through the exit status only")
	fs.BoolVar(&perDomain, "per-domain", false, "Include per-domain deltas")
	fs.IntVar(&limit, "limit", 5000, "Max runs per batch")
	if err := parseWithReorderedFlags(fs, args); err != nil {
		return parseExit(err)
	}
	rest := fs.Args()
	if len(rest) < 2 {
		fmt.Fprintln(errOut, "two batch IDs are required: batches diff <batch-a> <batch-b>")
		return 2
	}

	diff, err := fetchBatchDiff(ctx, client, opts, strings.TrimSpace(rest[0]), strings.TrimSpace(rest[1]), limit, perDomain)
	if err != nil {
		fmt.Fprintln(errOut, err.Error())
		return 2
	}

	if quiet {
		if diff.Clean() {
			return 0
		}
		return 1
	}
	if opts.format != "pretty" {
		if err := writeOutput(out, opts.format, diff); err != nil {
			fmt.Fprintln(errOut, err.Error())
			return 2
		}
	} else {
		writeBatchDiffPretty(out, diff)
	}
	if diff.Clean() {
		return 0
	}
	return 1
}

func fetchBatchDiff(ctx context.Context, client *apiClient, opts globalOptions, batchA, batchB string, limit int, perDomain bool) (batchDiffOutput, error) {
	runsA, err := listBatchRuns(ctx, client, batchA, limit)
	if err != nil {
		return batchDiffOutput{}, fmt.Errorf("batch %s: %w", batchA, err)
	}
	runsB, err := listBatchRuns(ctx, client, batchB, limit)
	if err != nil {
		return batchDiffOutput{}, fmt.Errorf("batch %s: %w", batchB, err)
	}

	diff := batchDiffOutput{
		BatchA:     batchA,
		BatchB:     batchB,
		TagAdded:   map[string]int{},
		TagRemoved: map[string]int{},
		TagChanged: map[string]int{},
	}
	for domain := range runsA {
		if _, ok := runsB[domain]; !ok {
			diff.OnlyInA = append(diff.OnlyInA, domain)
		}
	}
	for domain := range runsB {
		if _, ok := runsA[domain]; !ok {
			diff.OnlyInB = append(diff.OnlyInB, domain)
		}
	}
	sort.Strings(diff.OnlyInA)
	sort.Strings(diff.OnlyInB)

	domains := make([]string, 0, len(runsA))
	for domain := range runsA {
		if _, ok := runsB[domain]; ok {
			domains = append(domains, domain)
		}
	}
	sort.Strings(domains)

	for _, domain := range domains {
		idA, idB := runsA[domain], runsB[domain]
		// An unfetchable or entry-less run supports no comparison, and
		// counting it identical would let a broken arm pass the gate.
		resultA, errA := fetchRunResult(ctx, client, opts, idA)
		resultB, errB := fetchRunResult(ctx, client, opts, idB)
		if errA != nil || errB != nil || !comparableResult(resultA) || !comparableResult(resultB) {
			diff.Unusable = append(diff.Unusable, domain)
			continue
		}
		added, removed, changed := diffTagMaps(worstLevelByTag(resultA), worstLevelByTag(resultB))
		diff.Compared++
		if len(added) == 0 && len(removed) == 0 && len(changed) == 0 {
			diff.Identical++
			continue
		}
		diff.Differing++
		for _, d := range added {
			diff.TagAdded[d.Tag]++
		}
		for _, d := range removed {
			diff.TagRemoved[d.Tag]++
		}
		for _, d := range changed {
			diff.TagChanged[d.Tag]++
		}
		if !perDomain {
			continue
		}
		entry := batchDiffDomain{Domain: domain, RunA: idA, RunB: idB, Added: added, Removed: removed, Changed: changed}
		if resultA.Score != nil {
			entry.GradeA = resultA.Score.Grade
		}
		if resultB.Score != nil {
			entry.GradeB = resultB.Score.Grade
		}
		diff.DomainDeltas = append(diff.DomainDeltas, entry)
	}
	return diff, nil
}

// comparableResult reports whether a run can carry a tag set; no entries
// means it failed or was purged, not that it found nothing.
func comparableResult(result jobResult) bool {
	return result.Raw != nil && len(result.Raw.Entries) > 0
}

// listBatchRuns maps domain to run ID for one batch. The runs endpoint caps
// limit at 500, so larger batches must be paged.
func listBatchRuns(ctx context.Context, client *apiClient, batchID string, limit int) (map[string]string, error) {
	const pageSize = 500
	if limit <= 0 {
		return nil, fmt.Errorf("limit must be positive, got %d", limit)
	}
	out := map[string]string{}
	total := 0
	for offset := 0; offset < limit; offset += pageSize {
		size := min(pageSize, limit-offset)
		q := url.Values{}
		q.Set("batch", batchID)
		q.Set("limit", strconv.Itoa(size))
		q.Set("offset", strconv.Itoa(offset))
		var page runList
		if err := client.doJSON(ctx, http.MethodGet, "/runs?"+q.Encode(), nil, &page); err != nil {
			return nil, err
		}
		total = page.Total
		for _, item := range page.Items {
			// Newest first, so the first run for a domain wins.
			if _, ok := out[item.Domain]; !ok {
				out[item.Domain] = item.ID
			}
		}
		if len(page.Items) < size {
			break
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no runs found")
	}
	// Truncating would shrink the comparison and read as agreement.
	if total > limit {
		return nil, fmt.Errorf("batch holds %d runs, above the --limit of %d; raise --limit", total, limit)
	}
	return out, nil
}

func writeBatchDiffPretty(out io.Writer, diff batchDiffOutput) {
	fmt.Fprintf(out, "Batch diff: %s -> %s\n", diff.BatchA, diff.BatchB)
	fmt.Fprintf(out, "  Domains compared: %d (identical %d, differing %d)\n", diff.Compared, diff.Identical, diff.Differing)
	if len(diff.OnlyInA) > 0 {
		fmt.Fprintf(out, "  Only in A: %d\n", len(diff.OnlyInA))
	}
	if len(diff.OnlyInB) > 0 {
		fmt.Fprintf(out, "  Only in B: %d\n", len(diff.OnlyInB))
	}
	if len(diff.Unusable) > 0 {
		fmt.Fprintf(out, "  Not comparable: %d\n", len(diff.Unusable))
	}
	writeTagCounts(out, "  + appeared in B", diff.TagAdded)
	writeTagCounts(out, "  - cleared in B", diff.TagRemoved)
	writeTagCounts(out, "  ~ changed severity", diff.TagChanged)
	for _, d := range diff.DomainDeltas {
		fmt.Fprintf(out, "  %s: added=%d removed=%d changed=%d\n", d.Domain, len(d.Added), len(d.Removed), len(d.Changed))
	}
}

func writeTagCounts(out io.Writer, label string, counts map[string]int) {
	if len(counts) == 0 {
		return
	}
	type row struct {
		tag string
		n   int
	}
	rows := make([]row, 0, len(counts))
	for tag, n := range counts {
		rows = append(rows, row{tag, n})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].n != rows[j].n {
			return rows[i].n > rows[j].n
		}
		return rows[i].tag < rows[j].tag
	})
	fmt.Fprintln(out, label+":")
	for _, r := range rows {
		fmt.Fprintf(out, "      %-44s %d domains\n", r.tag, r.n)
	}
}

func fetchRunDiff(ctx context.Context, client *apiClient, opts globalOptions, runA, runB string) (runDiffOutput, error) {
	resultA, domainA, err := fetchRunForDiff(ctx, client, opts, runA)
	if err != nil {
		return runDiffOutput{}, fmt.Errorf("run %s: %w", runA, err)
	}
	resultB, domainB, err := fetchRunForDiff(ctx, client, opts, runB)
	if err != nil {
		return runDiffOutput{}, fmt.Errorf("run %s: %w", runB, err)
	}

	diff := runDiffOutput{RunA: runA, RunB: runB, DomainA: domainA, DomainB: domainB}
	if resultA.Score != nil {
		diff.GradeA = resultA.Score.Grade
	}
	if resultB.Score != nil {
		diff.GradeB = resultB.Score.Grade
	}
	diff.Added, diff.Removed, diff.Changed = diffTagMaps(worstLevelByTag(resultA), worstLevelByTag(resultB))
	return diff, nil
}

func fetchRunForDiff(ctx context.Context, client *apiClient, opts globalOptions, runID string) (jobResult, string, error) {
	var record runRecord
	if err := client.doJSON(ctx, http.MethodGet, "/runs/"+url.PathEscape(runID), nil, &record); err != nil {
		return jobResult{}, "", err
	}
	result, err := fetchRunResult(ctx, client, opts, runID)
	if err != nil {
		return jobResult{}, "", err
	}
	return result, record.Domain, nil
}

// fetchRunResult skips the metadata request; the batch path already has the
// domain from the listing.
func fetchRunResult(ctx context.Context, client *apiClient, opts globalOptions, runID string) (jobResult, error) {
	path := "/runs/" + url.PathEscape(runID) + "/result"
	if v := strings.TrimSpace(opts.locale); v != "" {
		path += "?locale=" + url.QueryEscape(v)
	}
	var result jobResult
	if err := client.doJSON(ctx, http.MethodGet, path, nil, &result); err != nil {
		return jobResult{}, err
	}
	return result, nil
}

func writeRunDiffPretty(out io.Writer, diff runDiffOutput) {
	fmt.Fprintf(out, "Run diff: %s -> %s\n", diff.RunA, diff.RunB)
	if diff.DomainA != "" || diff.DomainB != "" {
		if diff.DomainA == diff.DomainB {
			fmt.Fprintf(out, "  Domain: %s\n", diff.DomainA)
		} else {
			fmt.Fprintf(out, "  Domains: %s -> %s\n", diff.DomainA, diff.DomainB)
		}
	}
	if diff.GradeA != "" || diff.GradeB != "" {
		fmt.Fprintf(out, "  Grade: %s -> %s\n", orDash(diff.GradeA), orDash(diff.GradeB))
	}
	if diff.Identical() {
		fmt.Fprintln(out, "  No tag or severity changes.")
		return
	}
	for _, d := range diff.Added {
		fmt.Fprintf(out, "  + %-40s %-10s %s\n", d.Tag, d.Level, d.Module)
	}
	for _, d := range diff.Removed {
		fmt.Fprintf(out, "  - %-40s %-10s %s\n", d.Tag, d.Level, d.Module)
	}
	for _, d := range diff.Changed {
		fmt.Fprintf(out, "  ~ %-40s %s -> %s  %s\n", d.Tag, d.FromLevel, d.ToLevel, d.Module)
	}
	fmt.Fprintf(out, "  added=%d removed=%d changed=%d\n", len(diff.Added), len(diff.Removed), len(diff.Changed))
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
