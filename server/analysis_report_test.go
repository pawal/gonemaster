package server

import (
	"encoding/json"
	"fmt"
	"testing"

	"codeberg.org/pawal/gonemaster/scoring"
)

// vocabJSON marshals a module -> tag -> level table the way capture stores it.
func vocabJSON(t *testing.T, table map[string]map[string]string) string {
	t.Helper()
	raw, err := json.Marshal(table)
	if err != nil {
		t.Fatalf("marshal vocabulary: %v", err)
	}
	return string(raw)
}

// oneCategoryScoring keeps the weighted attribution arithmetic exact so a
// test asserts penalties, not rounding.
func oneCategoryScoring() scoring.Config {
	return scoring.Config{
		SeverityPenalties: map[string]int{"NOTICE": 1, "WARNING": 5, "ERROR": 20},
		CategoryWeights:   map[string]float64{"dnssec": 1},
		ModuleCategories:  map[string]string{"DNSSEC": "dnssec", "ZONE": "dnssec"},
	}
}

// A tag only in the newer vocabulary is engine-driven; one both sides level
// is a cohort change; a level move matching the re-level is reclassified and
// any other level move is a cohort change.
func TestReportTagClassification(t *testing.T) {
	from := parseReportVocabulary(vocabJSON(t, map[string]map[string]string{
		"DNSSEC":      {"DS08_EXPIRED": "WARNING", "DS02_MISMATCH": "WARNING"},
		"CONSISTENCY": {"DROPPED": "WARNING"},
	}))
	to := parseReportVocabulary(vocabJSON(t, map[string]map[string]string{
		"DNSSEC":     {"DS08_EXPIRED": "WARNING", "DS02_MISMATCH": "ERROR"},
		"ZONE":       {"Z15_NO_CAA": "NOTICE"},
		"NAMESERVER": {"N18_NO_RESPONSE": "NOTICE"},
	}))
	delta := diffReportVocabularies(from, to)

	if got := delta.classifyAppeared("Z15_NO_CAA"); got != ReportChangeNewInEngine {
		t.Errorf("Z15_NO_CAA appeared = %q, want %q", got, ReportChangeNewInEngine)
	}
	if got := delta.classifyAppeared("DS08_EXPIRED"); got != ReportChangeCohort {
		t.Errorf("DS08_EXPIRED appeared = %q, want %q", got, ReportChangeCohort)
	}
	if got := delta.classifyCleared("DROPPED"); got != ReportChangeRemovedFromEngine {
		t.Errorf("DROPPED cleared = %q, want %q", got, ReportChangeRemovedFromEngine)
	}
	if got := delta.classifyCleared("DS08_EXPIRED"); got != ReportChangeCohort {
		t.Errorf("DS08_EXPIRED cleared = %q, want %q", got, ReportChangeCohort)
	}
	if got := delta.classifyLevelMove("DS02_MISMATCH", "WARNING", "ERROR"); got != ReportChangeLevelReclassified {
		t.Errorf("DS02_MISMATCH relevel = %q, want %q", got, ReportChangeLevelReclassified)
	}
	// The vocabulary moved WARNING -> ERROR; a cohort-wide move the other
	// way is the cohort, not the engine.
	if got := delta.classifyLevelMove("DS02_MISMATCH", "ERROR", "WARNING"); got != ReportChangeCohort {
		t.Errorf("reverse relevel = %q, want %q", got, ReportChangeCohort)
	}
	if got := delta.classifyLevelMove("DS08_EXPIRED", "WARNING", "ERROR"); got != ReportChangeCohort {
		t.Errorf("unrelated level move = %q, want %q", got, ReportChangeCohort)
	}
}

// An unreadable vocabulary on either side classifies every change as
// unknown, never as a cohort change.
func TestReportTagClassificationUnknownVocabulary(t *testing.T) {
	known := parseReportVocabulary(vocabJSON(t, map[string]map[string]string{
		"DNSSEC": {"DS08_EXPIRED": "WARNING"},
	}))
	empty := parseReportVocabulary("")
	if empty.available {
		t.Fatalf("empty vocabulary must not report available")
	}
	for name, delta := range map[string]reportVocabularyDelta{
		"from missing": diffReportVocabularies(empty, known),
		"to missing":   diffReportVocabularies(known, empty),
		"both missing": diffReportVocabularies(empty, empty),
	} {
		if got := delta.classifyAppeared("ANY_TAG"); got != ReportChangeUnknown {
			t.Errorf("%s appeared = %q, want %q", name, got, ReportChangeUnknown)
		}
		if got := delta.classifyCleared("ANY_TAG"); got != ReportChangeUnknown {
			t.Errorf("%s cleared = %q, want %q", name, got, ReportChangeUnknown)
		}
		if got := delta.classifyLevelMove("ANY_TAG", "NOTICE", "ERROR"); got != ReportChangeUnknown {
			t.Errorf("%s level move = %q, want %q", name, got, ReportChangeUnknown)
		}
	}
}

// The vocabulary delta payload reports both sides' tag counts and the three
// change lists, so the header can state what the engine did.
func TestReportVocabularyDeltaPayload(t *testing.T) {
	from := parseReportVocabulary(vocabJSON(t, map[string]map[string]string{
		"DNSSEC": {"KEEP": "WARNING", "GONE": "NOTICE"},
	}))
	to := parseReportVocabulary(vocabJSON(t, map[string]map[string]string{
		"DNSSEC": {"KEEP": "ERROR", "FRESH": "NOTICE"},
	}))
	payload := diffReportVocabularies(from, to).payload
	if payload.FromTagCount != 2 || payload.ToTagCount != 2 {
		t.Errorf("tag counts = %d/%d, want 2/2", payload.FromTagCount, payload.ToTagCount)
	}
	if len(payload.Added) != 1 || payload.Added[0].Tag != "FRESH" {
		t.Errorf("Added = %+v, want [FRESH]", payload.Added)
	}
	if len(payload.Removed) != 1 || payload.Removed[0].Tag != "GONE" {
		t.Errorf("Removed = %+v, want [GONE]", payload.Removed)
	}
	if len(payload.LevelChanged) != 1 || payload.LevelChanged[0].Tag != "KEEP" {
		t.Errorf("LevelChanged = %+v, want [KEEP]", payload.LevelChanged)
	}
	if payload.LevelChanged[0].FromLevel != "WARNING" || payload.LevelChanged[0].ToLevel != "ERROR" {
		t.Errorf("KEEP levels = %s -> %s, want WARNING -> ERROR",
			payload.LevelChanged[0].FromLevel, payload.LevelChanged[0].ToLevel)
	}
}

func TestDomainCategoryBranches(t *testing.T) {
	cases := []struct {
		name      string
		available bool
		engine    int
		cohort    int
		want      string
	}{
		{"only engine tags", true, 2, 0, ReportCategoryMeasurement},
		{"only cohort tags", true, 0, 2, ReportCategoryReal},
		{"both", true, 1, 1, ReportCategoryMixed},
		{"no visible tag move", true, 0, 0, ReportCategoryUnknown},
		{"vocabulary unreadable", false, 3, 3, ReportCategoryUnknown},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := domainCategory(tc.available, tc.engine, tc.cohort); got != tc.want {
				t.Errorf("domainCategory = %q, want %q", got, tc.want)
			}
		})
	}
}

// A domain that carries a re-levelled tag without its own level moving is
// not measurement-affected: only its unrelated cohort change moved it.
func TestReportDomainRelevelledTagNotMeasurement(t *testing.T) {
	from := parseReportVocabulary(vocabJSON(t, map[string]map[string]string{
		"DNSSEC": {"DS02_MISMATCH": "WARNING", "DS08_EXPIRED": "WARNING"},
	}))
	to := parseReportVocabulary(vocabJSON(t, map[string]map[string]string{
		"DNSSEC": {"DS02_MISMATCH": "ERROR", "DS08_EXPIRED": "WARNING"},
	}))
	delta := diffReportVocabularies(from, to)

	fromByName := map[string]AnalysisSnapshotDomainView{
		"a.example": {
			DomainName: "a.example", Score: intPtr(90), Grade: "A",
			Tags: []DomainViewTag{{Tag: "DS02_MISMATCH", Module: "DNSSEC", Level: "ERROR"}},
		},
	}
	toByName := map[string]AnalysisSnapshotDomainView{
		"a.example": {
			DomainName: "a.example", Score: intPtr(70), Grade: "C",
			Tags: []DomainViewTag{
				{Tag: "DS02_MISMATCH", Module: "DNSSEC", Level: "ERROR"},
				{Tag: "DS08_EXPIRED", Module: "DNSSEC", Level: "WARNING"},
			},
		},
	}
	rows := buildReportDomains(fromByName, toByName, delta, oneCategoryScoring())
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	if rows[0].Category != ReportCategoryReal {
		t.Errorf("Category = %q, want %q", rows[0].Category, ReportCategoryReal)
	}
	if len(rows[0].LevelChanged) != 0 {
		t.Errorf("LevelChanged = %+v, want empty", rows[0].LevelChanged)
	}
	if len(rows[0].Appeared) != 1 || rows[0].Appeared[0].Classification != ReportChangeCohort {
		t.Errorf("Appeared = %+v, want one cohort_change", rows[0].Appeared)
	}
}

// A domain whose only movement is a tag the engine gained classifies as
// measurement, so a false regression reads as one.
func TestReportDomainMeasurementOnly(t *testing.T) {
	from := parseReportVocabulary(vocabJSON(t, map[string]map[string]string{
		"DNSSEC": {"DS08_EXPIRED": "WARNING"},
	}))
	to := parseReportVocabulary(vocabJSON(t, map[string]map[string]string{
		"DNSSEC": {"DS08_EXPIRED": "WARNING"},
		"ZONE":   {"Z15_NO_CAA": "NOTICE"},
	}))
	delta := diffReportVocabularies(from, to)
	fromByName := map[string]AnalysisSnapshotDomainView{
		"b.example": {DomainName: "b.example", Score: intPtr(100), Grade: "A"},
	}
	toByName := map[string]AnalysisSnapshotDomainView{
		"b.example": {
			DomainName: "b.example", Score: intPtr(99), Grade: "A",
			Tags: []DomainViewTag{{Tag: "Z15_NO_CAA", Module: "ZONE", Level: "NOTICE"}},
		},
	}
	rows := buildReportDomains(fromByName, toByName, delta, oneCategoryScoring())
	if len(rows) != 1 || rows[0].Category != ReportCategoryMeasurement {
		t.Fatalf("rows = %+v, want one measurement row", rows)
	}
	if rows[0].Appeared[0].Classification != ReportChangeNewInEngine {
		t.Errorf("Z15_NO_CAA = %q, want %q", rows[0].Appeared[0].Classification, ReportChangeNewInEngine)
	}
}

// Appeared and cleared penalties sum to the score delta, leaving nothing
// unexplained on a domain whose findings account for the whole move.
func TestExplainScoreDeltaMatchesFindings(t *testing.T) {
	cfg := oneCategoryScoring()
	appeared := []reportTagMove{{Tag: "DS08_EXPIRED", Module: "DNSSEC", ToLevel: "ERROR"}}
	cleared := []reportTagMove{{Tag: "DS10_BITMAP", Module: "DNSSEC", FromLevel: "WARNING"}}
	if got := explainScoreDelta(cfg, appeared, cleared, nil); got != -15 {
		t.Errorf("explained = %d, want -15", got)
	}
	changed := []reportTagMove{{Tag: "DS02_MISMATCH", Module: "DNSSEC", FromLevel: "NOTICE", ToLevel: "ERROR"}}
	if got := explainScoreDelta(cfg, nil, nil, changed); got != -19 {
		t.Errorf("explained level move = %d, want -19", got)
	}
}

// A TagPenalties override wins over the severity table, so a WARNING-level
// tag priced as an ERROR is attributed at its override.
func TestExplainScoreDeltaUsesTagPenalty(t *testing.T) {
	cfg := oneCategoryScoring()
	cfg.TagPenalties = map[string]int{"DS07_NOT_SIGNED": 20, "N15_SOFTWARE_VERSION": 0}
	appeared := []reportTagMove{
		{Tag: "DS07_NOT_SIGNED", Module: "DNSSEC", ToLevel: "WARNING"},
		{Tag: "N15_SOFTWARE_VERSION", Module: "DNSSEC", ToLevel: "NOTICE"},
	}
	if got := explainScoreDelta(cfg, appeared, nil, nil); got != -20 {
		t.Errorf("explained = %d, want -20", got)
	}
}

// A score move the findings do not account for leaves an unexplained
// remainder on the row.
func TestReportDomainUnexplainedDelta(t *testing.T) {
	vocab := vocabJSON(t, map[string]map[string]string{
		"DNSSEC": {"DS08_EXPIRED": "ERROR"},
	})
	delta := diffReportVocabularies(parseReportVocabulary(vocab), parseReportVocabulary(vocab))
	fromByName := map[string]AnalysisSnapshotDomainView{
		"c.example": {DomainName: "c.example", Score: intPtr(100)},
	}
	toByName := map[string]AnalysisSnapshotDomainView{
		"c.example": {
			DomainName: "c.example", Score: intPtr(90),
			Tags: []DomainViewTag{{Tag: "DS08_EXPIRED", Module: "DNSSEC", Level: "ERROR"}},
		},
	}
	rows := buildReportDomains(fromByName, toByName, delta, oneCategoryScoring())
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	if rows[0].ExplainedDelta != -20 {
		t.Errorf("ExplainedDelta = %d, want -20", rows[0].ExplainedDelta)
	}
	if rows[0].UnexplainedDelta != 10 {
		t.Errorf("UnexplainedDelta = %d, want 10", rows[0].UnexplainedDelta)
	}
}

// scoringConfigChanged never guesses: an unstamped side is unknown.
func TestScoringConfigChangedStates(t *testing.T) {
	cases := []struct {
		from, to, want string
	}{
		{"abc", "abc", ReportStateFalse},
		{"abc", "def", ReportStateTrue},
		{"", "def", ReportStateUnknown},
		{"abc", "", ReportStateUnknown},
	}
	for _, tc := range cases {
		got := scoringConfigChanged(
			AnalysisCohortSnapshot{ScoringConfigHash: tc.from},
			AnalysisCohortSnapshot{ScoringConfigHash: tc.to},
		)
		if got != tc.want {
			t.Errorf("scoringConfigChanged(%q, %q) = %q, want %q", tc.from, tc.to, got, tc.want)
		}
	}
}

func TestStricterFloor(t *testing.T) {
	if got := stricterFloor("NOTICE", "WARNING"); got != "WARNING" {
		t.Errorf("stricterFloor = %q, want WARNING", got)
	}
	if got := stricterFloor("ERROR", "NOTICE"); got != "ERROR" {
		t.Errorf("stricterFloor = %q, want ERROR", got)
	}
}

// clusterFixture builds n movers sharing one nameserver, one ASN and one
// software version, each with the given score delta.
func clusterFixture(prefix string, deltas []int) (map[string]AnalysisSnapshotDomainView, []PublicAnalysisReportDomain) {
	asn := int64(64500)
	views := map[string]AnalysisSnapshotDomainView{}
	rows := []PublicAnalysisReportDomain{}
	for i, delta := range deltas {
		name := fmt.Sprintf("%s%d.example", prefix, i)
		views[name] = AnalysisSnapshotDomainView{
			DomainName:      name,
			Nameservers:     []DomainViewNameserver{{Name: "ns.shared.example"}},
			Addresses:       []DomainViewAddress{{Address: "192.0.2.1", ASN: &asn, ASNLabel: "Shared AS", Prefix: "192.0.2.0/24"}},
			SoftwareVersion: "Authoritative Server 5.0.7",
		}
		d := delta
		rows = append(rows, PublicAnalysisReportDomain{Domain: name, ScoreDelta: &d})
	}
	return views, rows
}

// Nine domains sharing a nameserver, an ASN, a prefix and a software version
// and moving +8 to +9 collapse into one cluster naming every dimension.
func TestReportClustersCollapseDimensions(t *testing.T) {
	views, rows := clusterFixture("a", []int{8, 9, 8, 9, 8, 9, 8, 9, 8})
	clusters := buildReportClusters(views, rows, defaultReportMinCluster, defaultReportMaxSpread)
	if len(clusters) != 1 {
		t.Fatalf("clusters = %d, want 1: %+v", len(clusters), clusters)
	}
	c := clusters[0]
	if c.Size != 9 || c.MinDelta != 8 || c.MaxDelta != 9 || c.Direction != "improved" {
		t.Errorf("cluster = size %d, %d..%d, %s; want size 9, 8..9, improved",
			c.Size, c.MinDelta, c.MaxDelta, c.Direction)
	}
	dims := map[string]int{}
	for _, d := range c.Dimensions {
		dims[d.Dimension] = d.TotalDomains
	}
	for _, want := range []string{
		ReportDimensionNameserver, ReportDimensionASN, ReportDimensionPrefix, FactCategorySoftwareVersion,
	} {
		if _, ok := dims[want]; !ok {
			t.Errorf("cluster is missing dimension %q: %+v", want, c.Dimensions)
		}
	}
	if dims[ReportDimensionNameserver] != 9 {
		t.Errorf("nameserver total_domains = %d, want 9", dims[ReportDimensionNameserver])
	}
}

// The same nine domains spread over 40 points are not one movement.
func TestReportClustersRejectWideSpread(t *testing.T) {
	views, rows := clusterFixture("b", []int{1, 2, 3, 4, 5, 20, 30, 40, 41})
	clusters := buildReportClusters(views, rows, defaultReportMinCluster, defaultReportMaxSpread)
	if len(clusters) != 0 {
		t.Fatalf("clusters = %+v, want none", clusters)
	}
}

// A group that mixes improvement and regression is not a cluster.
func TestReportClustersRejectMixedDirection(t *testing.T) {
	views, rows := clusterFixture("c", []int{2, 2, -2})
	clusters := buildReportClusters(views, rows, defaultReportMinCluster, defaultReportMaxSpread)
	if len(clusters) != 0 {
		t.Fatalf("clusters = %+v, want none", clusters)
	}
}

// A group below min_cluster is dropped.
func TestReportClustersRespectMinCluster(t *testing.T) {
	views, rows := clusterFixture("d", []int{5, 5})
	if got := buildReportClusters(views, rows, defaultReportMinCluster, defaultReportMaxSpread); len(got) != 0 {
		t.Fatalf("clusters = %+v, want none at min 3", got)
	}
	if got := buildReportClusters(views, rows, 2, defaultReportMaxSpread); len(got) != 1 {
		t.Fatalf("clusters = %+v, want one at min 2", got)
	}
}

// Totals count membership, score movement and both sides' grade spread.
func TestBuildReportTotals(t *testing.T) {
	fromByName := map[string]AnalysisSnapshotDomainView{
		"same.example": {DomainName: "same.example", Score: intPtr(90), Grade: "A"},
		"down.example": {DomainName: "down.example", Score: intPtr(90), Grade: "A"},
		"gone.example": {DomainName: "gone.example", Score: intPtr(50), Grade: "D"},
	}
	toByName := map[string]AnalysisSnapshotDomainView{
		"same.example": {DomainName: "same.example", Score: intPtr(90), Grade: "A"},
		"down.example": {DomainName: "down.example", Score: intPtr(70), Grade: "C"},
		"new.example":  {DomainName: "new.example", Score: intPtr(100), Grade: "A"},
	}
	down := -20
	domains := []PublicAnalysisReportDomain{
		{Domain: "down.example", ScoreDelta: &down, Category: ReportCategoryReal},
	}
	totals := buildReportTotals(fromByName, toByName, domains)
	if totals.Added != 1 || totals.Removed != 1 || totals.BothDomainCount != 2 {
		t.Errorf("membership = added %d removed %d both %d, want 1/1/2",
			totals.Added, totals.Removed, totals.BothDomainCount)
	}
	if totals.Regressed != 1 || totals.Improved != 0 || totals.IdenticalScore != 1 {
		t.Errorf("movement = regressed %d improved %d identical %d, want 1/0/1",
			totals.Regressed, totals.Improved, totals.IdenticalScore)
	}
	if totals.FromMeanScore == nil || *totals.FromMeanScore != 76.67 {
		t.Errorf("FromMeanScore = %v, want 76.67", totals.FromMeanScore)
	}
	if totals.ToGrades["A"] != 2 || totals.ToGrades["C"] != 1 {
		t.Errorf("ToGrades = %v, want A:2 C:1", totals.ToGrades)
	}
	if totals.DomainCategories[ReportCategoryReal] != 1 {
		t.Errorf("DomainCategories = %v, want one real", totals.DomainCategories)
	}
}

// kommunerVocabularies returns the June and September tag vocabularies of the
// reference pair, cut down to the tags the fixture below exercises.
func kommunerVocabularies(t *testing.T) (string, string) {
	t.Helper()
	june := map[string]map[string]string{
		"ZONE": {
			"Z09_NO_RESPONSE_MX_QUERY": "WARNING",
		},
		"DNSSEC": {
			"DS20_NO_BITMAP":             "WARNING",
			"DS10_NSEC3_MISMATCHES_APEX": "WARNING",
			"DS08_DNSKEY_RRSIG_EXPIRED":  "ERROR",
		},
	}
	september := map[string]map[string]string{
		"ZONE": {
			"Z09_NO_RESPONSE_MX_QUERY": "WARNING",
			"Z15_NO_CAA":               "NOTICE",
		},
		"DNSSEC": {
			"DS20_NO_BITMAP":             "WARNING",
			"DS10_NSEC3_MISMATCHES_APEX": "WARNING",
			"DS08_DNSKEY_RRSIG_EXPIRED":  "ERROR",
		},
		"NAMESERVER": {
			"N18_NO_RESPONSE": "NOTICE",
		},
	}
	return vocabJSON(t, june), vocabJSON(t, september)
}

// The reference pair reproduces the hand analysis: two domains moved only by
// tags the engine gained, two by the cohort, the MX timeout is a cohort
// change on an unchanged vocabulary, and the nine domains sharing one
// version string form a single cluster.
func TestBuildAnalysisReportKommunerFixture(t *testing.T) {
	june, september := kommunerVocabularies(t)
	asn := int64(64500)

	fromDomains := []AnalysisSnapshotDomainView{
		{DomainName: "salem.se", Score: intPtr(100), Grade: "A",
			Nameservers: []DomainViewNameserver{{Name: "ns.salem.example"}}},
		{DomainName: "degerfors.se", Score: intPtr(100), Grade: "A",
			Nameservers: []DomainViewNameserver{{Name: "ns.degerfors.example"}}},
		{DomainName: "osteraker.se", Score: intPtr(85), Grade: "B",
			Nameservers: []DomainViewNameserver{{Name: "ns.osteraker.example"}}},
		{DomainName: "mala.se", Score: intPtr(95), Grade: "A",
			Nameservers: []DomainViewNameserver{{Name: "ns.mala.example"}}},
	}
	toDomains := []AnalysisSnapshotDomainView{
		{DomainName: "salem.se", Score: intPtr(99), Grade: "A",
			Nameservers: []DomainViewNameserver{{Name: "ns.salem.example"}},
			Tags:        []DomainViewTag{{Tag: "Z15_NO_CAA", Module: "ZONE", Level: "NOTICE"}}},
		{DomainName: "degerfors.se", Score: intPtr(99), Grade: "A",
			Nameservers: []DomainViewNameserver{{Name: "ns.degerfors.example"}},
			Tags:        []DomainViewTag{{Tag: "N18_NO_RESPONSE", Module: "NAMESERVER", Level: "NOTICE"}}},
		{DomainName: "osteraker.se", Score: intPtr(65), Grade: "D",
			Nameservers: []DomainViewNameserver{{Name: "ns.osteraker.example"}},
			Tags:        []DomainViewTag{{Tag: "DS08_DNSKEY_RRSIG_EXPIRED", Module: "DNSSEC", Level: "ERROR"}}},
		{DomainName: "mala.se", Score: intPtr(90), Grade: "A",
			Nameservers: []DomainViewNameserver{{Name: "ns.mala.example"}},
			Tags:        []DomainViewTag{{Tag: "Z09_NO_RESPONSE_MX_QUERY", Module: "ZONE", Level: "WARNING"}}},
	}

	// Nine domains on one operator clear both NSEC3 findings together.
	for i := range 9 {
		name := fmt.Sprintf("kommun%d.se", i)
		shared := []DomainViewNameserver{{Name: "ns1.powerdns.example"}}
		addrs := []DomainViewAddress{{Address: "192.0.2.1", ASN: &asn, ASNLabel: "Shared AS", Prefix: "192.0.2.0/24"}}
		fromDomains = append(fromDomains, AnalysisSnapshotDomainView{
			DomainName: name, Score: intPtr(90), Grade: "A",
			Nameservers: shared, Addresses: addrs,
			SoftwareVersion: "Authoritative Server 5.0.4",
			Tags: []DomainViewTag{
				{Tag: "DS20_NO_BITMAP", Module: "DNSSEC", Level: "WARNING"},
				{Tag: "DS10_NSEC3_MISMATCHES_APEX", Module: "DNSSEC", Level: "WARNING"},
			},
		})
		toDomains = append(toDomains, AnalysisSnapshotDomainView{
			DomainName: name, Score: intPtr(98 + i%2), Grade: "A",
			Nameservers: shared, Addresses: addrs,
			SoftwareVersion: "Authoritative Server 5.0.7",
		})
	}

	report := buildAnalysisReport(reportInput{
		DatasetTag: "kommuner",
		From: AnalysisCohortSnapshot{
			Slug: "2026-06-03", EngineVersion: "1.2.0",
			Vocabulary: june, ScoringConfigHash: "default", TagViewMinLevel: "NOTICE",
		},
		To: AnalysisCohortSnapshot{
			Slug: "2026-09-18", EngineVersion: "1.3.0",
			Vocabulary: september, ScoringConfigHash: "default", TagViewMinLevel: "NOTICE",
		},
		FromDomains: fromDomains,
		ToDomains:   toDomains,
		FromTags: []AnalysisSnapshotTagView{
			{Tag: "Z09_NO_RESPONSE_MX_QUERY", Module: "ZONE", Level: "WARNING", DomainCount: 3},
		},
		ToTags: []AnalysisSnapshotTagView{
			{Tag: "Z09_NO_RESPONSE_MX_QUERY", Module: "ZONE", Level: "WARNING", DomainCount: 19},
			{Tag: "Z15_NO_CAA", Module: "ZONE", Level: "NOTICE", DomainCount: 120},
		},
		Scoring:    oneCategoryScoring(),
		MinCluster: defaultReportMinCluster,
		MaxSpread:  defaultReportMaxSpread,
	})

	if !report.Header.Engine.Crossed {
		t.Errorf("engine versions must read as crossed: %+v", report.Header.Engine)
	}
	if report.Header.ScoringConfigChanged != ReportStateFalse {
		t.Errorf("ScoringConfigChanged = %q, want %q", report.Header.ScoringConfigChanged, ReportStateFalse)
	}
	if len(report.Header.Vocabulary.Added) != 2 {
		t.Errorf("vocabulary Added = %+v, want two tags", report.Header.Vocabulary.Added)
	}

	categories := map[string]string{}
	for _, row := range report.Domains {
		categories[row.Domain] = row.Category
	}
	for _, domain := range []string{"salem.se", "degerfors.se"} {
		if categories[domain] != ReportCategoryMeasurement {
			t.Errorf("%s = %q, want %q", domain, categories[domain], ReportCategoryMeasurement)
		}
	}
	for _, domain := range []string{"osteraker.se", "mala.se"} {
		if categories[domain] != ReportCategoryReal {
			t.Errorf("%s = %q, want %q", domain, categories[domain], ReportCategoryReal)
		}
	}

	// The MX timeout grew sixfold on a vocabulary that did not move, so it
	// is a cohort change; the engine version is what a reader must follow.
	mx, ok := findReportTag(report.Tags.LevelChanged, "Z09_NO_RESPONSE_MX_QUERY")
	if !ok {
		mx, ok = findReportTag(report.Tags.Appeared, "Z09_NO_RESPONSE_MX_QUERY")
	}
	if ok {
		t.Fatalf("Z09_NO_RESPONSE_MX_QUERY must not be an appeared or level-changed row: %+v", mx)
	}
	caa, ok := findReportTag(report.Tags.Appeared, "Z15_NO_CAA")
	if !ok || caa.Classification != ReportChangeNewInEngine {
		t.Errorf("Z15_NO_CAA = %+v, want %q", caa, ReportChangeNewInEngine)
	}

	if len(report.Clusters) != 1 {
		t.Fatalf("clusters = %d, want 1: %+v", len(report.Clusters), report.Clusters)
	}
	cluster := report.Clusters[0]
	if cluster.Size != 9 || cluster.Direction != "improved" {
		t.Errorf("cluster = size %d %s, want size 9 improved", cluster.Size, cluster.Direction)
	}
	dims := map[string]bool{}
	for _, d := range cluster.Dimensions {
		dims[d.Dimension] = true
	}
	for _, want := range []string{ReportDimensionNameserver, ReportDimensionASN, FactCategorySoftwareVersion} {
		if !dims[want] {
			t.Errorf("cluster is missing dimension %q: %+v", want, cluster.Dimensions)
		}
	}
	if report.Totals.Improved != 9 || report.Totals.Regressed != 4 {
		t.Errorf("movement = improved %d regressed %d, want 9/4",
			report.Totals.Improved, report.Totals.Regressed)
	}
}

func findReportTag(entries []PublicAnalysisReportTagEntry, tag string) (PublicAnalysisReportTagEntry, bool) {
	for _, e := range entries {
		if e.Tag == tag {
			return e, true
		}
	}
	return PublicAnalysisReportTagEntry{}, false
}

// A finding hidden only by the stricter of two tag view floors is not a
// cohort change. Both sides are compared at the floor they share.
func TestReportAlignsMismatchedTagFloors(t *testing.T) {
	vocab := vocabJSON(t, map[string]map[string]string{
		"NAMESERVER": {"N15_SOFTWARE_VERSION": "NOTICE"},
		"DNSSEC":     {"DS08_EXPIRED": "ERROR"},
	})
	report := buildAnalysisReport(reportInput{
		DatasetTag: "kommuner",
		From: AnalysisCohortSnapshot{
			Slug: "june", EngineVersion: "1.2.0", Vocabulary: vocab,
			ScoringConfigHash: "default", TagViewMinLevel: "NOTICE",
		},
		To: AnalysisCohortSnapshot{
			Slug: "sept", EngineVersion: "1.2.0", Vocabulary: vocab,
			ScoringConfigHash: "default", TagViewMinLevel: "WARNING",
		},
		FromDomains: []AnalysisSnapshotDomainView{{
			DomainName: "salem.se", Score: intPtr(99), Grade: "A",
			Tags: []DomainViewTag{
				{Tag: "N15_SOFTWARE_VERSION", Module: "NAMESERVER", Level: "NOTICE"},
				{Tag: "DS08_EXPIRED", Module: "DNSSEC", Level: "ERROR"},
			},
		}},
		// The NOTICE finding still holds but sits below the later floor.
		ToDomains: []AnalysisSnapshotDomainView{{
			DomainName: "salem.se", Score: intPtr(98), Grade: "A",
			Tags: []DomainViewTag{{Tag: "DS08_EXPIRED", Module: "DNSSEC", Level: "ERROR"}},
		}},
		FromTags: []AnalysisSnapshotTagView{
			{Tag: "N15_SOFTWARE_VERSION", Module: "NAMESERVER", Level: "NOTICE", DomainCount: 1},
			{Tag: "DS08_EXPIRED", Module: "DNSSEC", Level: "ERROR", DomainCount: 1},
		},
		ToTags: []AnalysisSnapshotTagView{
			{Tag: "DS08_EXPIRED", Module: "DNSSEC", Level: "ERROR", DomainCount: 1},
		},
		Scoring:    oneCategoryScoring(),
		MinCluster: defaultReportMinCluster,
		MaxSpread:  defaultReportMaxSpread,
	})

	if report.Header.TagFloor != "WARNING" {
		t.Fatalf("TagFloor = %q, want WARNING", report.Header.TagFloor)
	}
	if len(report.Tags.Cleared) != 0 {
		t.Errorf("cohort-wide cleared = %+v, want none below the floor", report.Tags.Cleared)
	}
	if len(report.Domains) != 1 {
		t.Fatalf("domains = %+v, want the one mover", report.Domains)
	}
	row := report.Domains[0]
	if len(row.Cleared) != 0 {
		t.Errorf("%s cleared = %+v, want none below the floor", row.Domain, row.Cleared)
	}
	// No finding moved above the floor, so the score move is unattributable.
	if row.Category != ReportCategoryUnknown {
		t.Errorf("%s category = %q, want %q", row.Domain, row.Category, ReportCategoryUnknown)
	}
}

// A shared floor leaves both sides exactly as captured.
func TestReportKeepsFindingsWhenFloorsAgree(t *testing.T) {
	vocab := vocabJSON(t, map[string]map[string]string{
		"NAMESERVER": {"N15_SOFTWARE_VERSION": "NOTICE"},
	})
	report := buildAnalysisReport(reportInput{
		DatasetTag: "kommuner",
		From: AnalysisCohortSnapshot{
			Slug: "june", EngineVersion: "1.2.0", Vocabulary: vocab,
			ScoringConfigHash: "default", TagViewMinLevel: "NOTICE",
		},
		To: AnalysisCohortSnapshot{
			Slug: "sept", EngineVersion: "1.2.0", Vocabulary: vocab,
			ScoringConfigHash: "default", TagViewMinLevel: "NOTICE",
		},
		FromDomains: []AnalysisSnapshotDomainView{{
			DomainName: "salem.se", Score: intPtr(99), Grade: "A",
			Tags: []DomainViewTag{{Tag: "N15_SOFTWARE_VERSION", Module: "NAMESERVER", Level: "NOTICE"}},
		}},
		ToDomains: []AnalysisSnapshotDomainView{{
			DomainName: "salem.se", Score: intPtr(100), Grade: "A",
		}},
		FromTags: []AnalysisSnapshotTagView{
			{Tag: "N15_SOFTWARE_VERSION", Module: "NAMESERVER", Level: "NOTICE", DomainCount: 1},
		},
		ToTags:     []AnalysisSnapshotTagView{},
		Scoring:    oneCategoryScoring(),
		MinCluster: defaultReportMinCluster,
		MaxSpread:  defaultReportMaxSpread,
	})

	if len(report.Tags.Cleared) != 1 || report.Tags.Cleared[0].Classification != ReportChangeCohort {
		t.Fatalf("cleared = %+v, want one cohort change", report.Tags.Cleared)
	}
	if len(report.Domains) != 1 || report.Domains[0].Category != ReportCategoryReal {
		t.Fatalf("domains = %+v, want one real mover", report.Domains)
	}
}
