package server

import (
	"math"
	"sort"
	"strconv"
	"strings"
)

// TagValueRollup is one ranked row of a tag-argument value aggregation.
type TagValueRollup struct {
	Value         string   `json:"value"`
	Count         int      `json:"count"`
	AvgScore      *float64 `json:"avg_score,omitempty"`
	SampleDomains []string `json:"sample_domains"`
}

// BatchTagValuesResponse is the body of GET /batches/{id}/tag-values.
type BatchTagValuesResponse struct {
	BatchID       string           `json:"batch_id"`
	Tag           string           `json:"tag"`
	Arg           string           `json:"arg"`
	MinCount      int              `json:"min_count"`
	WeightByScore bool             `json:"weight_by_score,omitempty"`
	Values        []TagValueRollup `json:"values"`
}

// argValues extracts arg from one entry's args; list args are unpacked.
func argValues(args map[string]any, arg string) []string {
	v, ok := args[arg]
	if !ok {
		return nil
	}
	if list, ok := v.([]any); ok {
		out := make([]string, 0, len(list))
		for _, e := range list {
			if s := argValueToString(e); s != "" {
				out = append(out, s)
			}
		}
		return out
	}
	if s := argValueToString(v); s != "" {
		return []string{s}
	}
	return nil
}

// argValueToString renders a scalar arg; whole-number floats lose the fraction.
func argValueToString(v any) string {
	switch n := v.(type) {
	case string:
		return strings.TrimSpace(n)
	case float64:
		if n == math.Trunc(n) && !math.IsInf(n, 0) {
			return strconv.FormatInt(int64(n), 10)
		}
		return strconv.FormatFloat(n, 'g', -1, 64)
	case int:
		return strconv.Itoa(n)
	case int64:
		return strconv.FormatInt(n, 10)
	}
	return ""
}

// valueRun is a run reduced to its score and the arg values it carries.
type valueRun struct {
	domain   string
	score    int
	hasScore bool
	values   []string
}

type tagValueSample struct {
	domain string
	score  int
}

// aggregateTagValues groups runs by value, filters by minCount, ranks, truncates.
func aggregateTagValues(runs []valueRun, minCount, limit int, weightByScore bool) []TagValueRollup {
	type acc struct {
		count   int
		sum     int
		scored  int
		samples []tagValueSample
	}
	groups := map[string]*acc{}
	for _, r := range runs {
		seen := map[string]bool{}
		for _, v := range r.values {
			if v == "" || seen[v] {
				continue
			}
			seen[v] = true
			a := groups[v]
			if a == nil {
				a = &acc{}
				groups[v] = a
			}
			a.count++
			if r.hasScore {
				a.sum += r.score
				a.scored++
			}
			a.samples = append(a.samples, tagValueSample{domain: r.domain, score: r.score})
		}
	}

	out := make([]TagValueRollup, 0, len(groups))
	for value, a := range groups {
		if a.count < minCount {
			continue
		}
		// Highest score first when weighting, else domain order.
		sort.Slice(a.samples, func(i, j int) bool {
			if weightByScore && a.samples[i].score != a.samples[j].score {
				return a.samples[i].score > a.samples[j].score
			}
			return a.samples[i].domain < a.samples[j].domain
		})
		n := min(len(a.samples), 10)
		domains := make([]string, n)
		for i := 0; i < n; i++ {
			domains[i] = a.samples[i].domain
		}
		row := TagValueRollup{Value: value, Count: a.count, SampleDomains: domains}
		if weightByScore {
			avg := 0.0
			if a.scored > 0 {
				avg = math.Round(float64(a.sum)/float64(a.scored)*10) / 10
			}
			row.AvgScore = &avg
		}
		out = append(out, row)
	}

	sort.Slice(out, func(i, j int) bool {
		if weightByScore {
			if ai, aj := derefScore(out[i].AvgScore), derefScore(out[j].AvgScore); ai != aj {
				return ai > aj
			}
		}
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Value < out[j].Value
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

func derefScore(p *float64) float64 {
	if p == nil {
		return 0
	}
	return *p
}

// batchTagValues rolls up the values of arg within tag across a batch's runs.
func (s *Server) batchTagValues(batchID, tag, arg string, minCount, limit int, weightByScore bool) []TagValueRollup {
	const fetchLimit = 1_000_000
	runs := s.store.ListRuns(RunFilter{BatchID: batchID, Limit: fetchLimit}).Items
	domainByRun := make(map[string]string, len(runs))
	scoreByRun := make(map[string]*int, len(runs))
	for _, run := range runs {
		domainByRun[run.ID] = run.Domain
		scoreByRun[run.ID] = run.Score
	}

	entries := s.store.QueryEntries(EntryFilter{BatchID: batchID, EntryTag: tag, Limit: fetchLimit}).Items
	valuesByRun := map[string][]string{}
	for _, e := range entries {
		if vs := argValues(e.Args, arg); len(vs) > 0 {
			valuesByRun[e.RunID] = append(valuesByRun[e.RunID], vs...)
		}
	}

	rows := make([]valueRun, 0, len(valuesByRun))
	for runID, vals := range valuesByRun {
		vr := valueRun{domain: domainByRun[runID], values: vals}
		if score := scoreByRun[runID]; score != nil {
			vr.score = *score
			vr.hasScore = true
		}
		rows = append(rows, vr)
	}
	return aggregateTagValues(rows, minCount, limit, weightByScore)
}
