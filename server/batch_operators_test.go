package server

import (
	"reflect"
	"testing"
)

func TestNSParent(t *testing.T) {
	// The NS-parent rule is "drop the leftmost label". A two-label name
	// collapses to its single trailing label, a one-label name is returned
	// unchanged, and case plus any trailing root dot are normalised away so
	// that DNS1.NIC.EXAMPLE. and dns1.nic.example map to the same parent.
	cases := map[string]string{
		"dns1.nic.yamaxun":   "nic.yamaxun",
		"a.gtld-servers.net": "gtld-servers.net",
		"a0.nic.example":     "nic.example",
		"ns.example":         "example",
		"DNS1.NIC.EXAMPLE.":  "nic.example",
		"example":            "example",
		"":                   "",
		"   ":                "",
	}
	for in, want := range cases {
		if got := nsParent(in); got != want {
			t.Errorf("nsParent(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestASNsFromEntry(t *testing.T) {
	// JSON unmarshals numbers into float64, so the list tags carry an "asns"
	// slice of float64 and the scalar tags carry a single float64 "asn".
	// Tags outside the Connectivity03 ASN set contribute nothing.
	list := asnsFromEntry("IPV4_DIFFERENT_ASN", map[string]any{"asns": []any{float64(13335), float64(15169)}})
	if want := []string{"13335", "15169"}; !reflect.DeepEqual(list, want) {
		t.Errorf("list tag asns = %v, want %v", list, want)
	}

	scalar := asnsFromEntry("IPV6_ONE_ASN", map[string]any{"asn": float64(64500)})
	if want := []string{"64500"}; !reflect.DeepEqual(scalar, want) {
		t.Errorf("scalar tag asn = %v, want %v", scalar, want)
	}

	if got := asnsFromEntry("B01_CHILD_FOUND", map[string]any{"asn": float64(1)}); got != nil {
		t.Errorf("non-ASN tag should yield nil, got %v", got)
	}
	if got := asnsFromEntry("IPV4_SAME_ASN", map[string]any{}); got != nil {
		t.Errorf("missing args should yield nil, got %v", got)
	}
}

func TestAggregateOperatorsRankingAndSamples(t *testing.T) {
	// Five domains across two operators: A serves the top three scores
	// (100, 90, 80 -> avg 90) and B serves the bottom two (70, 60 -> avg 65).
	// A must rank first (higher mean), and each operator's sample_domains must
	// be ordered by score descending.
	const a, b = "64500", "64501"
	runs := []operatorRun{
		{domain: "d100", score: 100, keys: []string{a}},
		{domain: "d90", score: 90, keys: []string{a}},
		{domain: "d80", score: 80, keys: []string{a}},
		{domain: "d70", score: 70, keys: []string{b}},
		{domain: "d60", score: 60, keys: []string{b}},
	}

	got := aggregateOperators(runs, 1, 10)
	want := []OperatorRollup{
		{Key: a, DomainCount: 3, AvgScore: 90, SampleDomains: []string{"d100", "d90", "d80"}},
		{Key: b, DomainCount: 2, AvgScore: 65, SampleDomains: []string{"d70", "d60"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("aggregateOperators ranking mismatch:\n got %+v\nwant %+v", got, want)
	}

	// min_count=3 drops B, which only has two domains.
	only := aggregateOperators(runs, 3, 10)
	if len(only) != 1 || only[0].Key != a {
		t.Fatalf("min_count=3 should leave only A, got %+v", only)
	}
}

func TestAggregateOperatorsUnionNoDoubleCount(t *testing.T) {
	// A single run may surface the same ASN more than once (for example the
	// same AS in both the v4 and v6 union). The aggregate must attribute the
	// run's score to that ASN exactly once, so domain_count stays 1, not 2.
	runs := []operatorRun{
		{domain: "dup", score: 80, keys: []string{"13335", "13335", "15169"}},
	}
	got := aggregateOperators(runs, 1, 10)
	byKey := map[string]OperatorRollup{}
	for _, op := range got {
		byKey[op.Key] = op
	}
	if op := byKey["13335"]; op.DomainCount != 1 || op.AvgScore != 80 {
		t.Errorf("AS13335 should count the run once at score 80, got %+v", op)
	}
	if op := byKey["15169"]; op.DomainCount != 1 {
		t.Errorf("AS15169 should count the run once, got %+v", op)
	}
}

func TestAggregateOperatorsTieBreakAndLimit(t *testing.T) {
	// Equal mean scores tie-break on domain_count descending, then key ascending.
	// "many" (two domains) outranks both single-domain keys, and between the
	// single-domain keys "aaa" precedes "bbb" lexically.
	runs := []operatorRun{
		{domain: "d1", score: 50, keys: []string{"many"}},
		{domain: "d2", score: 50, keys: []string{"many"}},
		{domain: "d3", score: 50, keys: []string{"bbb"}},
		{domain: "d4", score: 50, keys: []string{"aaa"}},
	}
	got := aggregateOperators(runs, 1, 10)
	gotKeys := []string{got[0].Key, got[1].Key, got[2].Key}
	if want := []string{"many", "aaa", "bbb"}; !reflect.DeepEqual(gotKeys, want) {
		t.Errorf("tie-break order = %v, want %v", gotKeys, want)
	}

	// limit truncates the ranked output.
	if limited := aggregateOperators(runs, 1, 2); len(limited) != 2 {
		t.Errorf("limit=2 should return 2 rows, got %d", len(limited))
	}
}

func TestAggregateOperatorsSampleCap(t *testing.T) {
	// sample_domains is capped at 10 entries even when more domains contribute,
	// and the retained sample is the ten highest-scoring domains.
	runs := make([]operatorRun, 0, 12)
	for i := 0; i < 12; i++ {
		runs = append(runs, operatorRun{
			domain: string(rune('a' + i)),
			score:  i, // ascending scores; highest are kept
			keys:   []string{"op"},
		})
	}
	got := aggregateOperators(runs, 1, 10)
	if len(got) != 1 {
		t.Fatalf("expected one operator, got %d", len(got))
	}
	if n := len(got[0].SampleDomains); n != 10 {
		t.Fatalf("sample_domains should cap at 10, got %d", n)
	}
	// Highest score (11 -> 'l') must lead; lowest two (0,1) are dropped.
	if got[0].SampleDomains[0] != "l" {
		t.Errorf("top sample = %q, want %q", got[0].SampleDomains[0], "l")
	}
}
