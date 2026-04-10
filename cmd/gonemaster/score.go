package main

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"codeberg.org/pawal/gonemaster/engine"
	"codeberg.org/pawal/gonemaster/scoring"
)

// computeScore converts engine log entries to scoring entries and calls
// scoring.Compute. Returns nil when entries is empty.
func computeScore(domain string, entries []engine.LogEntry, cfg scoring.Config) *scoring.Result {
	if len(entries) == 0 {
		return nil
	}
	se := make([]scoring.Entry, 0, len(entries))
	for _, e := range entries {
		se = append(se, scoring.Entry{
			Module: e.Module,
			Tag:    e.Tag,
			Level:  e.Level,
		})
	}
	r := scoring.Compute(domain, se, cfg)
	return &r
}

// printScore writes a compact scoring summary to out.
func printScore(out io.Writer, r *scoring.Result) {
	if r == nil {
		fmt.Fprintln(out, "Score: N/A (no entries)")
		return
	}

	partial := ""
	if len(r.DisabledStacks) > 0 {
		partial = fmt.Sprintf("  [partial: %s disabled]", strings.Join(r.DisabledStacks, ", "))
	}
	fmt.Fprintf(out, "Score: %d (%s)%s\n", r.Score, r.Grade, partial)

	catOrder := []string{"dnssec", "nameserver_health", "connectivity", "zone_consistency"}
	printed := map[string]bool{}
	for _, cat := range catOrder {
		res, ok := r.Categories[cat]
		if !ok {
			continue
		}
		printed[cat] = true
		suffix := "penalties"
		if res.EntryCount == 1 {
			suffix = "penalty"
		}
		fmt.Fprintf(out, "  %-20s %3d  (%d %s)\n", cat+":", res.Score, res.EntryCount, suffix)
	}
	extra := []string{}
	for cat := range r.Categories {
		if !printed[cat] {
			extra = append(extra, cat)
		}
	}
	sort.Strings(extra)
	for _, cat := range extra {
		res := r.Categories[cat]
		suffix := "penalties"
		if res.EntryCount == 1 {
			suffix = "penalty"
		}
		fmt.Fprintf(out, "  %-20s %3d  (%d %s)\n", cat+":", res.Score, res.EntryCount, suffix)
	}

	if r.Score == 100 && r.Grade == "A" {
		notMet := []string{}
		for name, v := range r.Bonus.Criteria {
			if v != nil && !*v {
				notMet = append(notMet, name)
			}
		}
		if len(notMet) > 0 {
			sort.Strings(notMet)
			fmt.Fprintf(out, "A+ not achieved: %s\n", strings.Join(notMet, ", "))
		}
	}
}
