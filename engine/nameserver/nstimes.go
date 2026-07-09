package nameserver

import (
	"math"
	"sort"
	"strings"
	"time"
)

// NameserverTiming holds timing statistics for a single tested authoritative nameserver.
type NameserverTiming struct {
	Nameserver string  `json:"nameserver"`
	Address    string  `json:"address"`
	AvgMS      float64 `json:"avg_ms"`
	MinMS      float64 `json:"min_ms"`
	MaxMS      float64 `json:"max_ms"`
	MedianMS   float64 `json:"median_ms"`
	StddevMS   float64 `json:"stddev_ms"`
	Count      int     `json:"count"`
	// Empty on rows written before this field existed; treat as "ok".
	Status string `json:"status,omitempty"`
}

// Status values for NameserverTiming.
const (
	NameserverTimingStatusOK          = "ok"
	NameserverTimingStatusUnreachable = "unreachable"
	NameserverTimingStatusUnresolved  = "unresolved"
)

// TimingsFromQueryMap converts a raw query timing map (keyed as "name/address")
// into a slice of NameserverTiming sorted by nameserver name, address, then
// median query time ascending. Keys present only in timeouts (queries that
// timed out with no response) surface as unreachable rows so a dead server
// stays visible without its waited budget masquerading as a response time.
func TimingsFromQueryMap(queryTimings map[string][]time.Duration, timeouts map[string]int) []NameserverTiming {
	type entry struct {
		nameserver string
		address    string
		samples    []time.Duration
	}

	entries := make([]entry, 0, len(queryTimings))
	for key, samples := range queryTimings {
		name, address, ok := strings.Cut(key, "/")
		if !ok {
			continue
		}
		entries = append(entries, entry{nameserver: name, address: address, samples: samples})
	}

	out := make([]NameserverTiming, 0, len(entries))
	for _, e := range entries {
		stats := ComputeTimingStats(e.samples)
		if stats.Count == 0 {
			out = append(out, NameserverTiming{
				Nameserver: e.nameserver,
				Address:    e.address,
				Status:     NameserverTimingStatusUnreachable,
			})
			continue
		}
		out = append(out, NameserverTiming{
			Nameserver: e.nameserver,
			Address:    e.address,
			AvgMS:      stats.Avg,
			MinMS:      stats.Min,
			MaxMS:      stats.Max,
			MedianMS:   stats.Median,
			StddevMS:   stats.Stddev,
			Count:      stats.Count,
			Status:     NameserverTimingStatusOK,
		})
	}

	for key := range timeouts {
		if _, ok := queryTimings[key]; ok {
			continue
		}
		name, address, ok := strings.Cut(key, "/")
		if !ok {
			continue
		}
		out = append(out, NameserverTiming{
			Nameserver: name,
			Address:    address,
			Status:     NameserverTimingStatusUnreachable,
		})
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].Nameserver != out[j].Nameserver {
			return out[i].Nameserver < out[j].Nameserver
		}
		if out[i].Address != out[j].Address {
			return out[i].Address < out[j].Address
		}
		return out[i].MedianMS < out[j].MedianMS
	})

	return out
}

// NSTimingStats holds computed statistics for a single nameserver's query times.
type NSTimingStats struct {
	// Max is the largest observed query time in milliseconds.
	Max float64
	// Min is the smallest observed query time in milliseconds.
	Min float64
	// Avg is the arithmetic mean query time in milliseconds.
	Avg float64
	// Stddev is the standard deviation of query times in milliseconds.
	Stddev float64
	// Median is the median query time in milliseconds.
	Median float64
	// Total is the sum of all query times in milliseconds.
	Total float64
	// Count is the number of samples included in the statistics.
	Count int
}

// ComputeTimingStats calculates timing statistics from a slice of durations.
// Times are returned in milliseconds.
func ComputeTimingStats(times []time.Duration) NSTimingStats {
	n := len(times)
	if n == 0 {
		return NSTimingStats{}
	}

	ms := make([]float64, n)
	total := 0.0
	for i, d := range times {
		ms[i] = float64(d) / float64(time.Millisecond)
		total += ms[i]
	}

	sort.Float64s(ms)
	avg := total / float64(n)

	variance := 0.0
	for _, v := range ms {
		diff := v - avg
		variance += diff * diff
	}
	variance /= float64(n)

	var median float64
	if n%2 == 0 {
		median = (ms[n/2-1] + ms[n/2]) / 2
	} else {
		median = ms[n/2]
	}

	return NSTimingStats{
		Max:    ms[n-1],
		Min:    ms[0],
		Avg:    avg,
		Stddev: math.Sqrt(variance),
		Median: median,
		Total:  total,
		Count:  n,
	}
}
