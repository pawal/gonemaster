package server

import (
	"testing"
)

func TestBuildFactDistributionsGroupsByCategoryAndCounts(t *testing.T) {
	v := int64(1)
	facts := []AnalysisRunDomainFact{
		// Two domains sign with algo 13.
		{CohortID: 1, RunID: "r1", DomainID: 10, Category: FactCategoryDNSKEYAlgorithm, Key: "13", ValueNum: &v},
		{CohortID: 1, RunID: "r2", DomainID: 11, Category: FactCategoryDNSKEYAlgorithm, Key: "13", ValueNum: &v},
		// One of them also publishes algo 8 (dual-algo).
		{CohortID: 1, RunID: "r1", DomainID: 10, Category: FactCategoryDNSKEYAlgorithm, Key: "8", ValueNum: &v},
		// Domain 11 is signed, domain 12 is unsigned.
		{CohortID: 1, RunID: "r2", DomainID: 11, Category: FactCategorySigned, Key: FactKeySigned},
		{CohortID: 1, RunID: "r3", DomainID: 12, Category: FactCategorySigned, Key: FactKeyUnsigned},
	}
	out := buildFactDistributions(facts)

	signedDist, ok := out[FactCategorySigned]
	if !ok {
		t.Fatalf("expected %q distribution, got %+v", FactCategorySigned, out)
	}
	if signedDist.Label != "DNSSEC posture" {
		t.Fatalf("unexpected signed label: %q", signedDist.Label)
	}
	if len(signedDist.Buckets) != 2 {
		t.Fatalf("expected 2 signed buckets, got %+v", signedDist.Buckets)
	}
	// Signed must come before Unsigned by KeyOrder.
	if signedDist.Buckets[0].Key != FactKeySigned {
		t.Fatalf("expected signed first, got %+v", signedDist.Buckets)
	}
	if signedDist.Buckets[0].Count != 1 || signedDist.Buckets[1].Count != 1 {
		t.Fatalf("expected 1:1 signed/unsigned, got %+v", signedDist.Buckets)
	}
	if signedDist.Buckets[1].Tone != "warning" {
		t.Fatalf("expected unsigned tone=warning, got %q", signedDist.Buckets[1].Tone)
	}

	algoDist, ok := out[FactCategoryDNSKEYAlgorithm]
	if !ok {
		t.Fatalf("expected dnskey_algo distribution, got %+v", out)
	}
	if len(algoDist.Buckets) != 2 {
		t.Fatalf("expected 2 algo buckets, got %+v", algoDist.Buckets)
	}
	// Numeric ordering: algo 8 before algo 13.
	if algoDist.Buckets[0].Key != "8" || algoDist.Buckets[1].Key != "13" {
		t.Fatalf("expected numeric ordering, got %+v", algoDist.Buckets)
	}
	if algoDist.Buckets[0].Count != 1 {
		t.Fatalf("algo=8 should count 1 domain, got %d", algoDist.Buckets[0].Count)
	}
	if algoDist.Buckets[1].Count != 2 {
		t.Fatalf("algo=13 should count 2 domains, got %d", algoDist.Buckets[1].Count)
	}
	if algoDist.Buckets[0].Label != "RSASHA256" || algoDist.Buckets[1].Label != "ECDSAP256SHA256" {
		t.Fatalf("unexpected labels: %+v", algoDist.Buckets)
	}
}

func TestBuildFactDistributionsUnknownCategoryGetsSafeDefaults(t *testing.T) {
	facts := []AnalysisRunDomainFact{
		{CohortID: 1, RunID: "r1", DomainID: 1, Category: "unregistered", Key: "x"},
	}
	out := buildFactDistributions(facts)
	got, ok := out["unregistered"]
	if !ok {
		t.Fatalf("expected unregistered category to pass through, got %+v", out)
	}
	if got.Label != "unregistered" {
		t.Fatalf("fallback label should be category, got %q", got.Label)
	}
	if len(got.Buckets) != 1 || got.Buckets[0].Label != "x" || got.Buckets[0].Tone != "neutral" {
		t.Fatalf("unexpected fallback bucket: %+v", got.Buckets)
	}
}

func TestBuildFactDistributionsEmpty(t *testing.T) {
	if out := buildFactDistributions(nil); out != nil {
		t.Fatalf("empty facts should return nil, got %+v", out)
	}
}

func TestGradeDistributionKnownLettersUseSeverityTones(t *testing.T) {
	facts := []AnalysisRunDomainFact{
		{CohortID: 1, RunID: "r1", DomainID: 1, Category: FactCategoryGrade, Key: "A+"},
		{CohortID: 1, RunID: "r2", DomainID: 2, Category: FactCategoryGrade, Key: "A"},
		{CohortID: 1, RunID: "r3", DomainID: 3, Category: FactCategoryGrade, Key: "B"},
		{CohortID: 1, RunID: "r4", DomainID: 4, Category: FactCategoryGrade, Key: "C"},
		{CohortID: 1, RunID: "r5", DomainID: 5, Category: FactCategoryGrade, Key: "D"},
		{CohortID: 1, RunID: "r6", DomainID: 6, Category: FactCategoryGrade, Key: "F"},
	}
	dist, ok := buildFactDistributions(facts)[FactCategoryGrade]
	if !ok {
		t.Fatalf("expected grade distribution, got %+v", buildFactDistributions(facts))
	}
	if len(dist.Buckets) != 6 {
		t.Fatalf("expected 6 buckets, got %+v", dist.Buckets)
	}
	want := []struct {
		key, tone string
	}{
		{"A+", "ok"},
		{"A", "ok"},
		{"B", "notice"},
		{"C", "warning"},
		{"D", "error"},
		{"F", "critical"},
	}
	for i, w := range want {
		got := dist.Buckets[i]
		if got.Key != w.key || got.Tone != w.tone {
			t.Fatalf("bucket %d: got %+v, want key=%q tone=%q", i, got, w.key, w.tone)
		}
	}
}

func TestGradeDistributionUnknownLabelFallsBackToNeutral(t *testing.T) {
	facts := []AnalysisRunDomainFact{
		{CohortID: 1, RunID: "r1", DomainID: 1, Category: FactCategoryGrade, Key: "Gold"},
		{CohortID: 1, RunID: "r2", DomainID: 2, Category: FactCategoryGrade, Key: "A"},
	}
	dist := buildFactDistributions(facts)[FactCategoryGrade]
	byKey := map[string]PublicAnalysisFactBucket{}
	for _, b := range dist.Buckets {
		byKey[b.Key] = b
	}
	if byKey["Gold"].Tone != "neutral" {
		t.Fatalf("custom grade should be neutral, got %q", byKey["Gold"].Tone)
	}
	if byKey["A"].Tone != "ok" {
		t.Fatalf("default grade should keep its tone, got %q", byKey["A"].Tone)
	}
	// Unknown grades sort after known ones, but A comes first.
	if dist.Buckets[0].Key != "A" {
		t.Fatalf("expected A first, got %+v", dist.Buckets)
	}
}

func TestDNSKEYAlgorithmKeyLabelFallback(t *testing.T) {
	if got := dnskeyAlgorithmKeyLabel("999"); got != "ALGO 999" {
		t.Fatalf("unknown algo number should render as ALGO <n>, got %q", got)
	}
	if got := dnskeyAlgorithmKeyLabel("not-numeric"); got != "not-numeric" {
		t.Fatalf("non-numeric key should pass through, got %q", got)
	}
}
