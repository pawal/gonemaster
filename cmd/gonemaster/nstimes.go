package main

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"codeberg.org/pawal/gonemaster/engine/nameserver"
)

const (
	nsTimesNameWidth   = 50
	nsTimesNumberWidth = 10
)

func writeNSTimes(out io.Writer, timings map[string][]time.Duration, timeouts map[string]int) error {
	if len(timings) == 0 && len(timeouts) == 0 {
		return nil
	}

	type entry struct {
		key         string
		stats       nameserver.NSTimingStats
		unreachable bool
	}

	entries := make([]entry, 0, len(timings)+len(timeouts))
	for key, times := range timings {
		entries = append(entries, entry{
			key:   key,
			stats: nameserver.ComputeTimingStats(times),
		})
	}
	// Keys that only ever timed out have no response-time samples; list them
	// as unreachable so a dead server stays visible without its waited budget
	// showing up as a response time.
	for key := range timeouts {
		if _, ok := timings[key]; ok {
			continue
		}
		entries = append(entries, entry{key: key, unreachable: true})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].key != entries[j].key {
			return entries[i].key < entries[j].key
		}
		return entries[i].stats.Median < entries[j].stats.Median
	})

	nameWidth := nsTimesNameWidth
	for _, e := range entries {
		if len(e.key) > nameWidth {
			nameWidth = len(e.key)
		}
	}

	divider := strings.Repeat("=", nameWidth) + "  " +
		strings.Repeat("=", nsTimesNumberWidth) + " " +
		strings.Repeat("=", nsTimesNumberWidth) + " " +
		strings.Repeat("=", nsTimesNumberWidth) + " " +
		strings.Repeat("=", nsTimesNumberWidth) + " " +
		strings.Repeat("=", nsTimesNumberWidth) + " " +
		strings.Repeat("=", nsTimesNumberWidth+1) + " " +
		strings.Repeat("=", nsTimesNumberWidth+1)

	header := fmt.Sprintf("%*s  %*s %*s %*s %*s %*s %*s %*s",
		nameWidth, "Name servers",
		nsTimesNumberWidth, "Max",
		nsTimesNumberWidth, "Min",
		nsTimesNumberWidth, "Avg",
		nsTimesNumberWidth, "Stddev",
		nsTimesNumberWidth, "Median",
		nsTimesNumberWidth+1, "Total",
		nsTimesNumberWidth+1, "Count")

	if _, err := fmt.Fprintln(out, header); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(out, divider); err != nil {
		return err
	}

	var grandTotal float64
	var grandCount int

	for _, e := range entries {
		var line string
		if e.unreachable {
			const dash = "-"
			line = fmt.Sprintf("%*s  %*s %*s %*s %*s %*s %*s %*s",
				nameWidth, e.key,
				nsTimesNumberWidth, dash,
				nsTimesNumberWidth, dash,
				nsTimesNumberWidth, dash,
				nsTimesNumberWidth, dash,
				nsTimesNumberWidth, dash,
				nsTimesNumberWidth+1, dash,
				nsTimesNumberWidth+1, dash)
		} else {
			s := e.stats
			line = fmt.Sprintf("%*s  %*.2f %*.2f %*.2f %*.2f %*.2f %*.2f %*d",
				nameWidth, e.key,
				nsTimesNumberWidth, s.Max,
				nsTimesNumberWidth, s.Min,
				nsTimesNumberWidth, s.Avg,
				nsTimesNumberWidth, s.Stddev,
				nsTimesNumberWidth, s.Median,
				nsTimesNumberWidth+1, s.Total,
				nsTimesNumberWidth+1, s.Count)
			grandTotal += s.Total
			grandCount += s.Count
		}
		if _, err := fmt.Fprintln(out, line); err != nil {
			return err
		}
	}

	if _, err := fmt.Fprintln(out, divider); err != nil {
		return err
	}
	summary := fmt.Sprintf("%*s  %*s %*s %*s %*s %*s %*.2f %*d",
		nameWidth, "Grand total",
		nsTimesNumberWidth, "",
		nsTimesNumberWidth, "",
		nsTimesNumberWidth, "",
		nsTimesNumberWidth, "",
		nsTimesNumberWidth, "",
		nsTimesNumberWidth+1, grandTotal,
		nsTimesNumberWidth+1, grandCount)
	if _, err := fmt.Fprintln(out, summary); err != nil {
		return err
	}
	return nil
}
