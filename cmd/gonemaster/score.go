package main

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"codeberg.org/pawal/gonemaster/engine"
	"codeberg.org/pawal/gonemaster/scoring"
)

// levelOrder lists all log levels from least to most severe.
var levelOrder = []string{"DEBUG3", "DEBUG2", "DEBUG", "INFO", "NOTICE", "WARNING", "ERROR", "CRITICAL"}

// levelIndex returns the position of level in levelOrder (case-insensitive).
// Unknown levels map to len(levelOrder), treated as most severe.
func levelIndex(level string) int {
	for i, l := range levelOrder {
		if strings.EqualFold(l, level) {
			return i
		}
	}
	return len(levelOrder)
}

// lowerLevel returns whichever of a or b is less severe. Case-insensitive.
func lowerLevel(a, b string) string {
	if levelIndex(a) <= levelIndex(b) {
		return strings.ToUpper(a)
	}
	return strings.ToUpper(b)
}

// filterEntriesByLevel returns only entries whose Level is at or above minLevel.
func filterEntriesByLevel(entries []engine.LogEntry, minLevel string) []engine.LogEntry {
	threshold := levelIndex(minLevel)
	out := make([]engine.LogEntry, 0, len(entries))
	for _, e := range entries {
		if levelIndex(e.Level) >= threshold {
			out = append(out, e)
		}
	}
	return out
}

// computeScore converts engine log entries to scoring entries and calls
// scoring.Compute. Returns nil when entries is nil (no run performed).
func computeScore(domain string, entries []engine.LogEntry, cfg scoring.Config) *scoring.Result {
	if entries == nil {
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
