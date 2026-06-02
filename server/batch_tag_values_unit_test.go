package server

import (
	"reflect"
	"testing"
)

func TestArgValues(t *testing.T) {
	// A scalar string is returned as-is; a number (float64, as JSON decodes it)
	// renders without a fractional part; a list is unpacked element by element;
	// a missing arg yields nothing.
	if got := argValues(map[string]any{"nsid": "1.fra.pch"}, "nsid"); !reflect.DeepEqual(got, []string{"1.fra.pch"}) {
		t.Errorf("scalar string = %v, want [1.fra.pch]", got)
	}
	if got := argValues(map[string]any{"asn": float64(13335)}, "asn"); !reflect.DeepEqual(got, []string{"13335"}) {
		t.Errorf("scalar number = %v, want [13335]", got)
	}
	list := argValues(map[string]any{"nameservers": []any{"a", "b", "c"}}, "nameservers")
	if !reflect.DeepEqual(list, []string{"a", "b", "c"}) {
		t.Errorf("list arg = %v, want [a b c]", list)
	}
	if got := argValues(map[string]any{"nsid": "x"}, "version"); got != nil {
		t.Errorf("missing arg should yield nil, got %v", got)
	}
	if got := argValues(map[string]any{"v": 1.5}, "v"); !reflect.DeepEqual(got, []string{"1.5"}) {
		t.Errorf("fractional number = %v, want [1.5]", got)
	}
}

func TestAggregateTagValuesCountRanking(t *testing.T) {
	// Three runs carry values A, B, A. A appears in two runs, B in one, so A
	// ranks first by count and its samples list both contributing domains.
	runs := []valueRun{
		{domain: "alpha", values: []string{"A"}},
		{domain: "beta", values: []string{"B"}},
		{domain: "gamma", values: []string{"A"}},
	}
	got := aggregateTagValues(runs, 1, 50, false)
	want := []TagValueRollup{
		{Value: "A", Count: 2, SampleDomains: []string{"alpha", "gamma"}},
		{Value: "B", Count: 1, SampleDomains: []string{"beta"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("count ranking mismatch:\n got %+v\nwant %+v", got, want)
	}

	// min_count=2 drops B.
	if only := aggregateTagValues(runs, 2, 50, false); len(only) != 1 || only[0].Value != "A" {
		t.Fatalf("min_count=2 should leave only A, got %+v", only)
	}
}

func TestAggregateTagValuesListNoDoubleCount(t *testing.T) {
	// One run carrying the same value twice (e.g. a list with duplicates)
	// counts that value once for the run.
	runs := []valueRun{
		{domain: "dup", values: []string{"x", "x", "y"}},
	}
	got := aggregateTagValues(runs, 1, 50, false)
	byVal := map[string]TagValueRollup{}
	for _, v := range got {
		byVal[v.Value] = v
	}
	if v := byVal["x"]; v.Count != 1 {
		t.Errorf("x should count the run once, got %+v", v)
	}
	if v := byVal["y"]; v.Count != 1 {
		t.Errorf("y should count the run once, got %+v", v)
	}
}

func TestAggregateTagValuesWeightByScore(t *testing.T) {
	// A serves three domains (100, 90, 80 -> mean 90); B serves two (70, 60 ->
	// mean 65). With weighting, A ranks first by mean score and samples are
	// ordered by score descending.
	runs := []valueRun{
		{domain: "d100", score: 100, hasScore: true, values: []string{"A"}},
		{domain: "d90", score: 90, hasScore: true, values: []string{"A"}},
		{domain: "d80", score: 80, hasScore: true, values: []string{"A"}},
		{domain: "d70", score: 70, hasScore: true, values: []string{"B"}},
		{domain: "d60", score: 60, hasScore: true, values: []string{"B"}},
	}
	got := aggregateTagValues(runs, 1, 50, true)
	if len(got) != 2 {
		t.Fatalf("expected two values, got %+v", got)
	}
	if got[0].Value != "A" || got[0].AvgScore == nil || *got[0].AvgScore != 90 {
		t.Errorf("first = %+v, want A with avg 90", got[0])
	}
	if got[1].Value != "B" || got[1].AvgScore == nil || *got[1].AvgScore != 65 {
		t.Errorf("second = %+v, want B with avg 65", got[1])
	}
	if !reflect.DeepEqual(got[0].SampleDomains, []string{"d100", "d90", "d80"}) {
		t.Errorf("A samples = %v, want score-descending order", got[0].SampleDomains)
	}
}

func TestAggregateTagValuesTieBreakAndLimit(t *testing.T) {
	// Equal counts tie-break on value ascending; limit truncates the output.
	runs := []valueRun{
		{domain: "d1", values: []string{"bbb"}},
		{domain: "d2", values: []string{"aaa"}},
		{domain: "d3", values: []string{"ccc"}},
	}
	got := aggregateTagValues(runs, 1, 50, false)
	gotVals := []string{got[0].Value, got[1].Value, got[2].Value}
	if want := []string{"aaa", "bbb", "ccc"}; !reflect.DeepEqual(gotVals, want) {
		t.Errorf("tie-break order = %v, want %v", gotVals, want)
	}
	if limited := aggregateTagValues(runs, 1, 2, false); len(limited) != 2 {
		t.Errorf("limit=2 should return 2 rows, got %d", len(limited))
	}
}

func TestAggregateTagValuesSampleCap(t *testing.T) {
	// sample_domains caps at 10 even when more domains contribute the value.
	runs := make([]valueRun, 0, 12)
	for i := 0; i < 12; i++ {
		runs = append(runs, valueRun{domain: string(rune('a' + i)), values: []string{"v"}})
	}
	got := aggregateTagValues(runs, 1, 50, false)
	if len(got) != 1 {
		t.Fatalf("expected one value, got %d", len(got))
	}
	if n := len(got[0].SampleDomains); n != 10 {
		t.Fatalf("sample_domains should cap at 10, got %d", n)
	}
}
