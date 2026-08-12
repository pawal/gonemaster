package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
)

// levelRank orders severities so a tag seen at several levels within one run
// collapses to its worst.
var levelRank = map[string]int{
	"DEBUG":    1,
	"INFO":     2,
	"NOTICE":   3,
	"WARNING":  4,
	"ERROR":    5,
	"CRITICAL": 6,
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

// Identical reports whether the two runs produced the same worst level for
// every tag. This is the zero-delta gate the measurement plans require.
func (d runDiffOutput) Identical() bool {
	return len(d.Added) == 0 && len(d.Removed) == 0 && len(d.Changed) == 0
}

// worstLevelByTag collapses a run's entries to one entry per tag, keeping the
// highest-severity level seen for that tag. Same logic as the MCP run_diff
// tool, so the two agree on what "changed" means.
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
	fs.BoolVar(&quiet, "quiet", false, "Print nothing; exit 1 when the runs differ")
	if err := parseWithReorderedFlags(fs, args); err != nil {
		return 2
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
	path := "/runs/" + url.PathEscape(runID) + "/result"
	if v := strings.TrimSpace(opts.locale); v != "" {
		path += "?locale=" + url.QueryEscape(v)
	}
	var result jobResult
	if err := client.doJSON(ctx, http.MethodGet, path, nil, &result); err != nil {
		return jobResult{}, "", err
	}
	return result, record.Domain, nil
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
