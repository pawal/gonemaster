// Command nsagg aggregates stored nameserver timings per address across the
// runs of one or more batches. Arguments are variant=batch-id; output is CSV.
// See the kit README for the columns and what they mean.
package main

import (
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

type runListItem struct {
	ID     string `json:"id"`
	Domain string `json:"domain"`
	Status string `json:"status"`
}

type runList struct {
	Items []runListItem `json:"items"`
	Total int           `json:"total"`
}

type nameserverTiming struct {
	Nameserver   string  `json:"nameserver"`
	Address      string  `json:"address"`
	MedianMS     float64 `json:"median_ms"`
	MaxMS        float64 `json:"max_ms"`
	Count        int     `json:"count"`
	Status       string  `json:"status"`
	TimeoutCount int     `json:"timeout_count"`
	RefusedCount int     `json:"refused_count"`
}

type runResult struct {
	NameserverTimings []nameserverTiming `json:"nameserver_timings"`
}

// addrStats accumulates one address across a variant. Medians are averaged,
// not pooled: the stored row carries only the per-run median.
type addrStats struct {
	nameserver   string
	address      string
	runs         int
	queries      int
	timeouts     int
	refused      int
	medianSum    float64
	medianRuns   int
	maxMS        float64
	unreachable  int
	runsWithFail int
	runsEngaged  int
}

// add folds one run's row into the totals. engage replays a backoff trigger:
// a run counts as engaged when the address answered and still burned that
// many timeout budgets. Per run, never summed.
func (s *addrStats) add(t nameserverTiming, engage int) {
	s.runs++
	s.queries += t.Count
	s.timeouts += t.TimeoutCount
	s.refused += t.RefusedCount
	if t.Count > 0 {
		s.medianSum += t.MedianMS
		s.medianRuns++
	}
	if t.MaxMS > s.maxMS {
		s.maxMS = t.MaxMS
	}
	if t.Status == "unreachable" {
		s.unreachable++
	}
	if t.TimeoutCount > 0 || t.RefusedCount > 0 {
		s.runsWithFail++
	}
	if engage > 0 && t.Count > 0 && t.TimeoutCount >= engage {
		s.runsEngaged++
	}
}

func main() {
	baseURL := flag.String("base-url", "http://127.0.0.1:18080", "Server base URL")
	limit := flag.Int("limit", 500, "Runs fetched per page (server caps this at 500)")
	timeout := flag.Duration("timeout", 60*time.Second, "HTTP timeout per request")
	minQueries := flag.Int("min-queries", 0, "Only print addresses we sent at least this many queries to (answers plus timeouts)")
	engage := flag.Int("engage-threshold", 0, "Replay the detector: count runs where an address answered and still burned this many timeout budgets (0 = off)")
	flag.Parse()

	pairs := flag.Args()
	if len(pairs) == 0 {
		fmt.Fprintln(os.Stderr, "usage: nsagg [--base-url URL] variant=batch-id [variant=batch-id ...]")
		os.Exit(2)
	}

	client := &http.Client{Timeout: *timeout}
	api := strings.TrimSuffix(*baseURL, "/") + "/api/v1"

	w := csv.NewWriter(os.Stdout)
	header := []string{
		"variant", "nameserver", "address", "runs", "queries", "timeouts", "refused",
		"timeout_rate", "refused_rate", "avg_median_ms", "max_ms", "unreachable_runs", "runs_with_failures",
		"runs_engaged",
	}
	if err := w.Write(header); err != nil {
		fail(err)
	}

	for _, pair := range pairs {
		variant, batchID, ok := strings.Cut(pair, "=")
		if !ok {
			fail(fmt.Errorf("argument %q is not variant=batch-id", pair))
		}
		stats, err := aggregateBatch(client, api, batchID, *limit, *engage)
		if err != nil {
			fail(fmt.Errorf("variant %s: %w", variant, err))
		}
		keys := make([]string, 0, len(stats))
		for k := range stats {
			keys = append(keys, k)
		}
		// Worst first: the addresses carrying the most failed queries are
		// the only ones the characterization cares about.
		sort.Slice(keys, func(i, j int) bool {
			a, b := stats[keys[i]], stats[keys[j]]
			fa, fb := a.timeouts+a.refused, b.timeouts+b.refused
			if fa != fb {
				return fa > fb
			}
			return keys[i] < keys[j]
		})
		for _, k := range keys {
			s := stats[k]
			// Filter on attempts, not answers: an address that timed out in
			// every run has zero answers and is exactly the row to keep.
			attempts := s.queries + s.timeouts
			if attempts < *minQueries {
				continue
			}
			row := []string{
				variant,
				s.nameserver,
				s.address,
				strconv.Itoa(s.runs),
				strconv.Itoa(s.queries),
				strconv.Itoa(s.timeouts),
				strconv.Itoa(s.refused),
				ratio(s.timeouts, attempts),
				ratio(s.refused, attempts),
				avg(s.medianSum, s.medianRuns),
				fmt.Sprintf("%.2f", s.maxMS),
				strconv.Itoa(s.unreachable),
				strconv.Itoa(s.runsWithFail),
				strconv.Itoa(s.runsEngaged),
			}
			if err := w.Write(row); err != nil {
				fail(err)
			}
		}
		w.Flush()
		if err := w.Error(); err != nil {
			fail(err)
		}
	}
}

func aggregateBatch(client *http.Client, api, batchID string, limit, engage int) (map[string]*addrStats, error) {
	items, err := listBatchRuns(client, api, batchID, limit)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("batch %s has no runs", batchID)
	}

	stats := map[string]*addrStats{}
	for _, item := range items {
		var result runResult
		if err := getJSON(client, api+"/runs/"+url.PathEscape(item.ID)+"/result", &result); err != nil {
			fmt.Fprintf(os.Stderr, "warning: run %s (%s): %v\n", item.ID, item.Domain, err)
			continue
		}
		for _, t := range result.NameserverTimings {
			key := t.Nameserver + "/" + t.Address
			s, ok := stats[key]
			if !ok {
				s = &addrStats{nameserver: t.Nameserver, address: t.Address}
				stats[key] = s
			}
			s.add(t, engage)
		}
	}
	return stats, nil
}

// listBatchRuns pages through a batch. The server caps limit at 500, so a
// larger batch must be walked with offset or its tail is silently dropped.
func listBatchRuns(client *http.Client, api, batchID string, pageSize int) ([]runListItem, error) {
	if pageSize <= 0 || pageSize > 500 {
		pageSize = 500
	}
	var items []runListItem
	for offset := 0; ; offset += pageSize {
		q := url.Values{}
		q.Set("batch", batchID)
		q.Set("limit", strconv.Itoa(pageSize))
		q.Set("offset", strconv.Itoa(offset))
		var page runList
		if err := getJSON(client, api+"/runs?"+q.Encode(), &page); err != nil {
			return nil, fmt.Errorf("list runs: %w", err)
		}
		items = append(items, page.Items...)
		if len(page.Items) < pageSize || len(items) >= page.Total {
			return items, nil
		}
	}
}

func getJSON(client *http.Client, endpoint string, out any) error {
	resp, err := client.Get(endpoint)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: %s", endpoint, resp.Status)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func ratio(n, total int) string {
	if total <= 0 {
		return "0"
	}
	return fmt.Sprintf("%.6f", float64(n)/float64(total))
}

func avg(sum float64, n int) string {
	if n <= 0 {
		return "0"
	}
	return fmt.Sprintf("%.2f", sum/float64(n))
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "nsagg:", err)
	os.Exit(1)
}
