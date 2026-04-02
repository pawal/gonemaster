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

func writeNSTimes(out io.Writer, timings map[string][]time.Duration) error {
	if len(timings) == 0 {
		return nil
	}

	type entry struct {
		key   string
		stats nameserver.NSTimingStats
	}

	entries := make([]entry, 0, len(timings))
	for key, times := range timings {
		entries = append(entries, entry{
			key:   key,
			stats: nameserver.ComputeTimingStats(times),
		})
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].key < entries[j].key
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
		s := e.stats
		line := fmt.Sprintf("%*s  %*.2f %*.2f %*.2f %*.2f %*.2f %*.2f %*d",
			nameWidth, e.key,
			nsTimesNumberWidth, s.Max,
			nsTimesNumberWidth, s.Min,
			nsTimesNumberWidth, s.Avg,
			nsTimesNumberWidth, s.Stddev,
			nsTimesNumberWidth, s.Median,
			nsTimesNumberWidth+1, s.Total,
			nsTimesNumberWidth+1, s.Count)
		if _, err := fmt.Fprintln(out, line); err != nil {
			return err
		}
		grandTotal += s.Total
		grandCount += s.Count
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
