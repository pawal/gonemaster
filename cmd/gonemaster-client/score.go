package main

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"codeberg.org/pawal/gonemaster/scoring"
)

// scoringOptions carries the resolved scoring settings for a command invocation.
type scoringOptions struct {
	enabled bool
	cfg     scoring.Config
}

// parseScoringOptions resolves --score, --no-score, and --scoring-config into
// a scoringOptions value. --scoring-config implies --score. --no-score wins
// over both --score and --scoring-config.
func parseScoringOptions(scoreFlag, noScoreFlag bool, configPath string) (scoringOptions, error) {
	if noScoreFlag {
		return scoringOptions{enabled: false, cfg: scoring.DefaultConfig()}, nil
	}
	cfg := scoring.DefaultConfig()
	if configPath != "" {
		loaded, err := scoring.LoadConfig(configPath)
		if err != nil {
			return scoringOptions{}, err
		}
		cfg = loaded
		return scoringOptions{enabled: true, cfg: cfg}, nil
	}
	return scoringOptions{enabled: scoreFlag, cfg: cfg}, nil
}

// computeScoreFromResult converts the raw entries in a jobResult into scoring
// entries and calls scoring.Compute. Returns nil when no raw entries are
// available (e.g. summary-only results).
func computeScoreFromResult(result jobResult, domain string, cfg scoring.Config) *scoring.Result {
	if result.Raw == nil || len(result.Raw.Entries) == 0 {
		return nil
	}
	entries := make([]scoring.Entry, 0, len(result.Raw.Entries))
	for _, e := range result.Raw.Entries {
		entries = append(entries, scoring.Entry{
			Module: e.Module,
			Tag:    e.Tag,
			Level:  e.Level,
		})
	}
	r := scoring.Compute(domain, entries, cfg)
	return &r
}

// printScorePretty prints a compact scoring summary to out.
func printScorePretty(out io.Writer, r *scoring.Result) {
	if r == nil {
		fmt.Fprintln(out, "  Score: N/A (no entries data)")
		return
	}

	// Header line: score and grade, with partial-score note if stacks disabled.
	partial := ""
	if len(r.DisabledStacks) > 0 {
		partial = fmt.Sprintf("  [partial: %s disabled]", strings.Join(r.DisabledStacks, ", "))
	}
	fmt.Fprintf(out, "  Score: %d (%s)%s\n", r.Score, r.Grade, partial)

	// Per-category breakdown in a stable order.
	catOrder := []string{"dnssec", "nameserver_health", "connectivity", "zone_consistency"}
	printed := map[string]bool{}
	for _, cat := range catOrder {
		res, ok := r.Categories[cat]
		if !ok {
			continue
		}
		printed[cat] = true
		if !res.Tested {
			fmt.Fprintf(out, "    %-20s   -  (not tested)\n", cat+":")
		} else {
			suffix := penaltySuffix(res.EntryCount)
			fmt.Fprintf(out, "    %-20s %3d  (%d %s)\n", cat+":", res.Score, res.EntryCount, suffix)
		}
	}
	// Any categories not in the default order (custom configs).
	extra := []string{}
	for cat := range r.Categories {
		if !printed[cat] {
			extra = append(extra, cat)
		}
	}
	sort.Strings(extra)
	for _, cat := range extra {
		res := r.Categories[cat]
		if !res.Tested {
			fmt.Fprintf(out, "    %-20s   -  (not tested)\n", cat+":")
		} else {
			suffix := penaltySuffix(res.EntryCount)
			fmt.Fprintf(out, "    %-20s %3d  (%d %s)\n", cat+":", res.Score, res.EntryCount, suffix)
		}
	}

	// A+ criteria note when score is 100 but A+ was not awarded.
	if r.Score == 100 && r.Grade == "A" {
		notMet := []string{}
		for name, v := range r.Bonus.Criteria {
			if v != nil && !*v {
				notMet = append(notMet, name)
			}
		}
		if len(notMet) > 0 {
			sort.Strings(notMet)
			fmt.Fprintf(out, "  A+ not achieved: %s\n", strings.Join(notMet, ", "))
		}
	}
}

func penaltySuffix(count int) string {
	if count == 1 {
		return "penalty"
	}
	return "penalties"
}
