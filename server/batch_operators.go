package server

import (
	"math"
	"sort"
	"strconv"
	"strings"
)

// OperatorRollup is one ranked operator row in a batch operator aggregation.
type OperatorRollup struct {
	Key           string   `json:"key"`
	DomainCount   int      `json:"domain_count"`
	AvgScore      float64  `json:"avg_score"`
	SampleDomains []string `json:"sample_domains"`
}

// BatchOperatorsResponse is the body of GET /batches/{id}/operators.
type BatchOperatorsResponse struct {
	BatchID   string           `json:"batch_id"`
	GroupBy   string           `json:"group_by"`
	MinCount  int              `json:"min_count"`
	Operators []OperatorRollup `json:"operators"`
}

// Connectivity03 testcase and the tags that carry ASN data in their args.
const connectivityASNTestcase = "connectivity03"

// asnListTags carry an "asns" list arg; asnScalarTags carry a single "asn" arg.
var (
	asnListTags = map[string]bool{
		"IPV4_DIFFERENT_ASN": true,
		"IPV4_SAME_ASN":      true,
		"IPV6_DIFFERENT_ASN": true,
		"IPV6_SAME_ASN":      true,
	}
	asnScalarTags = map[string]bool{
		"IPV4_ONE_ASN": true,
		"IPV6_ONE_ASN": true,
	}
)

// nsParent drops the leftmost label: dns1.nic.example -> nic.example.
func nsParent(host string) string {
	h := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".")
	if h == "" {
		return ""
	}
	if i := strings.IndexByte(h, '.'); i >= 0 && i+1 < len(h) {
		return h[i+1:]
	}
	return h
}

// asnsFromEntry pulls ASN identifiers out of one Connectivity03 entry's args.
func asnsFromEntry(tag string, args map[string]any) []string {
	switch {
	case asnListTags[tag]:
		raw, ok := args["asns"].([]any)
		if !ok {
			return nil
		}
		out := make([]string, 0, len(raw))
		for _, v := range raw {
			if s := asnToString(v); s != "" {
				out = append(out, s)
			}
		}
		return out
	case asnScalarTags[tag]:
		if s := asnToString(args["asn"]); s != "" {
			return []string{s}
		}
	}
	return nil
}

func asnToString(v any) string {
	switch n := v.(type) {
	case float64:
		return strconv.FormatInt(int64(n), 10)
	case int:
		return strconv.Itoa(n)
	case int64:
		return strconv.FormatInt(n, 10)
	case string:
		return strings.TrimSpace(n)
	}
	return ""
}

// operatorRun is a run reduced to its score and operator keys.
type operatorRun struct {
	domain string
	score  int
	keys   []string
}

type operatorSample struct {
	domain string
	score  int
}

// aggregateOperators rolls runs up per key, filters by minCount, ranks by
// avg_score (then domain_count, then key), and truncates to limit.
func aggregateOperators(runs []operatorRun, minCount, limit int) []OperatorRollup {
	type acc struct {
		count   int
		sum     int
		samples []operatorSample
	}
	groups := map[string]*acc{}
	for _, r := range runs {
		seen := map[string]bool{}
		for _, key := range r.keys {
			if key == "" || seen[key] {
				continue
			}
			seen[key] = true
			a := groups[key]
			if a == nil {
				a = &acc{}
				groups[key] = a
			}
			a.count++
			a.sum += r.score
			a.samples = append(a.samples, operatorSample{domain: r.domain, score: r.score})
		}
	}

	out := make([]OperatorRollup, 0, len(groups))
	for key, a := range groups {
		if a.count < minCount {
			continue
		}
		sort.Slice(a.samples, func(i, j int) bool {
			if a.samples[i].score != a.samples[j].score {
				return a.samples[i].score > a.samples[j].score
			}
			return a.samples[i].domain < a.samples[j].domain
		})
		n := min(len(a.samples), 10)
		domains := make([]string, n)
		for i := 0; i < n; i++ {
			domains[i] = a.samples[i].domain
		}
		out = append(out, OperatorRollup{
			Key:           key,
			DomainCount:   a.count,
			AvgScore:      float64(a.sum) / float64(a.count),
			SampleDomains: domains,
		})
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].AvgScore != out[j].AvgScore {
			return out[i].AvgScore > out[j].AvgScore
		}
		if out[i].DomainCount != out[j].DomainCount {
			return out[i].DomainCount > out[j].DomainCount
		}
		return out[i].Key < out[j].Key
	})
	for i := range out {
		out[i].AvgScore = math.Round(out[i].AvgScore*10) / 10
	}
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

// batchOperators rolls up a batch's runs by groupBy ("ns_parent" or "asn").
func (s *Server) batchOperators(batchID, groupBy string, minCount, limit int) []OperatorRollup {
	const fetchLimit = 1_000_000
	runs := s.store.ListRuns(RunFilter{BatchID: batchID, Limit: fetchLimit}).Items

	rows := make([]operatorRun, 0, len(runs))
	switch groupBy {
	case "ns_parent":
		for _, run := range runs {
			if run.Score == nil {
				continue
			}
			seen := map[string]bool{}
			keys := make([]string, 0, len(run.NameserverTimings))
			for _, t := range run.NameserverTimings {
				if p := nsParent(t.Nameserver); p != "" && !seen[p] {
					seen[p] = true
					keys = append(keys, p)
				}
			}
			rows = append(rows, operatorRun{domain: run.Domain, score: *run.Score, keys: keys})
		}
	case "asn":
		entries := s.store.QueryEntries(EntryFilter{BatchID: batchID, Testcase: connectivityASNTestcase, Limit: fetchLimit}).Items
		asnByRun := map[string]map[string]bool{}
		for _, e := range entries {
			for _, asn := range asnsFromEntry(e.Tag, e.Args) {
				set := asnByRun[e.RunID]
				if set == nil {
					set = map[string]bool{}
					asnByRun[e.RunID] = set
				}
				set[asn] = true
			}
		}
		for _, run := range runs {
			if run.Score == nil {
				continue
			}
			set := asnByRun[run.ID]
			keys := make([]string, 0, len(set))
			for asn := range set {
				keys = append(keys, asn)
			}
			rows = append(rows, operatorRun{domain: run.Domain, score: *run.Score, keys: keys})
		}
	}

	return aggregateOperators(rows, minCount, limit)
}
