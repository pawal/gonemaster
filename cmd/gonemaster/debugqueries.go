package main

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"codeberg.org/pawal/gonemaster/engine/querytrace"
)

const (
	qtNameWidth   = 50
	qtNumberWidth = 10
)

// writeQueryTrace prints the per-nameserver query-trace table, slowest first.
func writeQueryTrace(out io.Writer, c *querytrace.Collector) error {
	if c == nil {
		return nil
	}

	attempts, timeouts, nsCount := c.Totals()
	if attempts == 0 {
		_, err := fmt.Fprintln(out, "Query trace: no queries traced")
		return err
	}
	if _, err := fmt.Fprintf(out, "Query trace: %d attempts (%d timeouts) across %d nameservers\n", attempts, timeouts, nsCount); err != nil {
		return err
	}

	stats := c.Stats()

	nameWidth := qtNameWidth
	for _, s := range stats {
		if n := len(traceNSLabel(s)); n > nameWidth {
			nameWidth = n
		}
	}

	divider := strings.Repeat("=", nameWidth) + "  " +
		strings.Repeat("=", qtNumberWidth) + " " +
		strings.Repeat("=", qtNumberWidth) + " " +
		strings.Repeat("=", qtNumberWidth) + " " +
		strings.Repeat("=", qtNumberWidth+1)

	header := fmt.Sprintf("%*s  %*s %*s %*s %*s  %s",
		nameWidth, "Name servers",
		qtNumberWidth, "Attempts",
		qtNumberWidth, "Timeouts",
		qtNumberWidth, "Errors",
		qtNumberWidth+1, "Elapsed/ms",
		"Decisions")
	if _, err := fmt.Fprintln(out, header); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(out, divider); err != nil {
		return err
	}

	var grandElapsed float64
	var grandAttempts, grandTimeouts, grandErrors int
	for _, s := range stats {
		elapsedMS := float64(s.TotalElapsed) / float64(time.Millisecond)
		line := fmt.Sprintf("%*s  %*d %*d %*d %*.2f  %s",
			nameWidth, traceNSLabel(s),
			qtNumberWidth, s.Attempts,
			qtNumberWidth, s.Timeouts,
			qtNumberWidth, s.Errors,
			qtNumberWidth+1, elapsedMS,
			formatDecisions(s.Decisions))
		if _, err := fmt.Fprintln(out, line); err != nil {
			return err
		}
		grandElapsed += elapsedMS
		grandAttempts += s.Attempts
		grandTimeouts += s.Timeouts
		grandErrors += s.Errors
	}

	if _, err := fmt.Fprintln(out, divider); err != nil {
		return err
	}
	summary := fmt.Sprintf("%*s  %*d %*d %*d %*.2f",
		nameWidth, "Grand total",
		qtNumberWidth, grandAttempts,
		qtNumberWidth, grandTimeouts,
		qtNumberWidth, grandErrors,
		qtNumberWidth+1, grandElapsed)
	_, err := fmt.Fprintln(out, summary)
	return err
}

// traceNSLabel renders "name/addr" when the host name is known, else the address.
func traceNSLabel(s querytrace.NSStats) string {
	if s.Name != "" {
		return s.Name + "/" + s.Addr
	}
	return s.Addr
}

// formatDecisions renders the decision tally as "kind:count" pairs, sorted for
// stable output.
func formatDecisions(d map[querytrace.DecisionKind]int) string {
	if len(d) == 0 {
		return ""
	}
	parts := make([]string, 0, len(d))
	for kind, n := range d {
		parts = append(parts, fmt.Sprintf("%s:%d", kind, n))
	}
	sort.Strings(parts)
	return strings.Join(parts, " ")
}
