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
		// Domain 11 publishes NSEC3, domain 12 is unsigned.
		{CohortID: 1, RunID: "r2", DomainID: 11, Category: FactCategoryDNSSECPosture, Key: FactKeyNSEC3},
		{CohortID: 1, RunID: "r3", DomainID: 12, Category: FactCategoryDNSSECPosture, Key: FactKeyUnsigned},
	}
	out := buildFactDistributions(facts)

	postureDist, ok := out[FactCategoryDNSSECPosture]
	if !ok {
		t.Fatalf("expected %q distribution, got %+v", FactCategoryDNSSECPosture, out)
	}
	if postureDist.Label != "DNSSEC posture" {
		t.Fatalf("unexpected posture label: %q", postureDist.Label)
	}
	if len(postureDist.Buckets) != 2 {
		t.Fatalf("expected 2 posture buckets, got %+v", postureDist.Buckets)
	}
	// Unsigned (order=0) comes before nsec3 (order=3).
	if postureDist.Buckets[0].Key != FactKeyUnsigned || postureDist.Buckets[1].Key != FactKeyNSEC3 {
		t.Fatalf("expected unsigned,nsec3 ordering, got %+v", postureDist.Buckets)
	}
	if postureDist.Buckets[0].Count != 1 || postureDist.Buckets[1].Count != 1 {
		t.Fatalf("expected 1:1 unsigned/nsec3, got %+v", postureDist.Buckets)
	}
	if postureDist.Buckets[0].Tone != "warning" {
		t.Fatalf("expected unsigned tone=warning, got %q", postureDist.Buckets[0].Tone)
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

// TestDNSKEYAlgorithmWeaknessRankOrdersByClassThenNumber pins the ordering
// the weakest-algorithm bar and the domain-list sort both depend on: the
// tone class dominates, the algorithm number only separates peers inside a
// class, and an algorithm the tone table does not know ranks below every
// usable class because no validator can use it.
func TestDNSKEYAlgorithmWeaknessRankOrdersByClassThenNumber(t *testing.T) {
	// Weakest first: unassigned (no class), SHA-1 family (error),
	// RSASHA1-NSEC3 (warning), RSA/SHA-256 (notice), curves (ok).
	weakestFirst := []string{"200", "1", "5", "7", "8", "10", "13", "15"}
	for i := 1; i < len(weakestFirst); i++ {
		prev := DNSKEYAlgorithmWeaknessRank(weakestFirst[i-1])
		cur := DNSKEYAlgorithmWeaknessRank(weakestFirst[i])
		if prev >= cur {
			t.Errorf("rank(%s)=%d must be below rank(%s)=%d",
				weakestFirst[i-1], prev, weakestFirst[i], cur)
		}
	}

	// Same class, so only the number separates them.
	if DNSKEYAlgorithmWeaknessRank("8") >= DNSKEYAlgorithmWeaknessRank("10") {
		t.Error("within one class the lower algorithm number must rank first")
	}

	// A non-numeric key cannot be ranked; it sorts last rather than
	// masquerading as the weakest algorithm in the cohort.
	if DNSKEYAlgorithmWeaknessRank("bogus") <= DNSKEYAlgorithmWeaknessRank("15") {
		t.Error("a non-numeric key must sort after every real algorithm")
	}
}

// TestNewFactCategoriesAreRegistered guards the registry wiring: an
// extractor that emits a category with no display entry renders as a raw
// token with neutral tones on the overview.
func TestNewFactCategoriesAreRegistered(t *testing.T) {
	for _, category := range []string{FactCategoryIPv6Coverage, FactCategoryDNSKEYAlgoWeakest} {
		display, ok := factCategoryDisplays[category]
		if !ok {
			t.Fatalf("category %q has no display entry", category)
		}
		if display.Label == "" || display.KeyLabel == nil || display.KeyTone == nil || display.KeyOrder == nil {
			t.Errorf("category %q display entry is incomplete: %+v", category, display)
		}
	}
}

// TestIPv6CoverageBucketsReadWorstFirst pins the coverage bar's tones and
// order so "none" leads the bar in red and "full" closes it in green.
func TestIPv6CoverageBucketsReadWorstFirst(t *testing.T) {
	cases := []struct {
		key   string
		tone  string
		order int
	}{
		{FactKeyCoverageNone, "error", 0},
		{FactKeyCoveragePartial, "warning", 1},
		{FactKeyCoverageFull, "ok", 2},
	}
	for _, c := range cases {
		if got := coverageKeyTone(c.key); got != c.tone {
			t.Errorf("tone(%s) = %q, want %q", c.key, got, c.tone)
		}
		if got := coverageKeyOrder(c.key); got != c.order {
			t.Errorf("order(%s) = %d, want %d", c.key, got, c.order)
		}
		if got := coverageKeyLabel(c.key); got == "" || got == c.key {
			t.Errorf("label(%s) = %q, want a human label", c.key, got)
		}
	}
	if got := coverageKeyTone("unknown"); got != "neutral" {
		t.Errorf("unknown coverage key tone = %q, want neutral", got)
	}
}
