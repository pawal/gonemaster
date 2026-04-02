package nameserver

import (
	"math"
	"sort"
	"time"
)

// NSTimingStats holds computed statistics for a single nameserver's query times.
type NSTimingStats struct {
	Max    float64
	Min    float64
	Avg    float64
	Stddev float64
	Median float64
	Total  float64
	Count  int
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
