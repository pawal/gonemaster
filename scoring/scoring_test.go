package scoring

import (
	"os"
	"path/filepath"
	"testing"
)

// cfg is the default config used across most tests.
var cfg = DefaultConfig()

// e constructs an Entry for test use.
func e(module, tag, level string) Entry {
	return Entry{Module: module, Tag: tag, Level: level}
}

// ---- Compute tests ----------------------------------------------------------

func TestCompute_PerfectScore(t *testing.T) {
	// No entries above INFO - should score 100 and grade A (not A+ without bonus).
	entries := []Entry{
		e("DNSSEC", "DS07_SIGNED", "INFO"),
		e("DNSSEC", "DS05_ALGO_OK", "INFO"),
	}
	r := Compute("example.se", entries, cfg)
	if r.Score != 100 {
		t.Errorf("expected score 100, got %d", r.Score)
	}
	if r.Grade != "A" && r.Grade != "A+" {
		t.Errorf("expected grade A or A+, got %s", r.Grade)
	}
}

func TestCompute_SingleWarning(t *testing.T) {
	entries := []Entry{
		e("DNSSEC", "DS04_RRSIG_EXPIRY_SOON", "WARNING"),
	}
	r := Compute("example.se", entries, cfg)
	// DNSSEC category: penalty 5, sub-score 95.
	// All other categories: 100.
	// Weighted avg with weights 1.5, 1.2, 1.0, 0.8 = 4.5 total.
	// (95*1.5 + 100*1.2 + 100*1.0 + 100*0.8) / 4.5 = (142.5+120+100+80)/4.5 = 442.5/4.5 = 98
	if r.Score != 98 {
		t.Errorf("expected score 98, got %d", r.Score)
	}
	if r.Grade != "A" {
		t.Errorf("expected grade A, got %s", r.Grade)
	}
	cat := r.Categories["dnssec"]
	if cat.Penalties != 5 {
		t.Errorf("expected dnssec penalties 5, got %d", cat.Penalties)
	}
	if cat.EntryCount != 1 {
		t.Errorf("expected dnssec entry_count 1, got %d", cat.EntryCount)
	}
}

func TestCompute_MultipleErrors_ScoreDrops(t *testing.T) {
	entries := []Entry{
		e("DNSSEC", "DS02_DNSKEY_NOT_SIGNED_BY_ANY_DS", "ERROR"),
		e("DNSSEC", "DS02_DNSKEY_NOT_SIGNED_BY_ANY_DS", "ERROR"),
		e("DNSSEC", "DS02_DNSKEY_NOT_SIGNED_BY_ANY_DS", "ERROR"),
	}
	r := Compute("example.se", entries, cfg)
	// DNSSEC: penalty 3*20=60, sub-score 40.
	// (40*1.5 + 100*1.2 + 100*1.0 + 100*0.8) / 4.5 = (60+120+100+80)/4.5 = 360/4.5 = 80
	if r.Score != 80 {
		t.Errorf("expected score 80, got %d", r.Score)
	}
	if r.Grade != "B" {
		t.Errorf("expected grade B, got %s", r.Grade)
	}
}

func TestCompute_ScoreFloorAtZero(t *testing.T) {
	// 10 ERRORs in the same category drives sub-score below 0.
	var entries []Entry
	for range 10 {
		entries = append(entries, e("DNSSEC", "DS_SOME_ERROR", "ERROR"))
	}
	r := Compute("example.se", entries, cfg)
	dnssec := r.Categories["dnssec"]
	if dnssec.Score != 0 {
		t.Errorf("expected dnssec sub-score 0, got %d", dnssec.Score)
	}
	if r.Score < 0 {
		t.Errorf("aggregate score should not be negative, got %d", r.Score)
	}
}

func TestCompute_CriticalForcesF(t *testing.T) {
	// One CRITICAL entry must force grade F regardless of everything else.
	entries := []Entry{
		e("BASIC", "B01_NO_PARENT", "CRITICAL"),
	}
	r := Compute("example.se", entries, cfg)
	if r.Grade != "F" {
		t.Errorf("expected grade F due to CRITICAL, got %s", r.Grade)
	}
	if r.Score > 10 {
		t.Errorf("score should be ≤10 when CRITICAL present, got %d", r.Score)
	}
	// The category containing the CRITICAL should score 0 and be marked tested.
	nh := r.Categories["nameserver_health"]
	if nh.Score != 0 {
		t.Errorf("expected nameserver_health score 0 (has CRITICAL), got %d", nh.Score)
	}
	if !nh.Tested {
		t.Error("expected nameserver_health tested=true (it received the CRITICAL entry)")
	}
	// Categories with no entries should be 0 and untested.
	for _, cat := range []string{"dnssec", "connectivity", "zone_consistency"} {
		cr := r.Categories[cat]
		if cr.Score != 0 {
			t.Errorf("expected %s score 0 (untested due to CRITICAL), got %d", cat, cr.Score)
		}
		if cr.Tested {
			t.Errorf("expected %s tested=false (no entries), got true", cat)
		}
	}
}

func TestCompute_CriticalWithOtherwiseGoodRun(t *testing.T) {
	// Even a run with mostly INFO entries drops to F if one CRITICAL exists.
	entries := []Entry{
		e("DNSSEC", "DS07_SIGNED", "INFO"),
		e("BASIC", "CRITICAL_FAILURE", "CRITICAL"),
	}
	r := Compute("example.se", entries, cfg)
	if r.Grade != "F" {
		t.Errorf("expected grade F, got %s", r.Grade)
	}
}

func TestCompute_CriticalAllCategoriesScoreZero(t *testing.T) {
	// A non-existent domain: BASIC CRITICAL + some entries from modules that
	// did run. ALL category scores must be 0 when CRITICAL is present -
	// individual scores are meaningless for a non-functional domain.
	entries := []Entry{
		e("BASIC", "B01_NO_PARENT", "CRITICAL"),
		e("DNSSEC", "DS07_NOT_SIGNED", "WARNING"),
	}
	r := Compute("example.se", entries, cfg)
	if r.Grade != "F" {
		t.Errorf("expected grade F, got %s", r.Grade)
	}
	if r.Score != 0 {
		t.Errorf("expected aggregate score 0, got %d", r.Score)
	}
	// Every category should be score 0.
	for cat, cr := range r.Categories {
		if cr.Score != 0 {
			t.Errorf("expected %s score 0 (CRITICAL present), got %d", cat, cr.Score)
		}
	}
	// Tested should reflect whether entries were received.
	if !r.Categories["nameserver_health"].Tested {
		t.Error("expected nameserver_health tested=true (received CRITICAL entry)")
	}
	if !r.Categories["dnssec"].Tested {
		t.Error("expected dnssec tested=true (received WARNING entry)")
	}
	for _, cat := range []string{"connectivity", "zone_consistency"} {
		if r.Categories[cat].Tested {
			t.Errorf("expected %s tested=false (no entries)", cat)
		}
	}
}

func TestCompute_NoCriticalUntestedCategoriesKeep100(t *testing.T) {
	// Without CRITICAL, categories with no penalty entries keep their
	// perfect score - this is the normal case where a category had no issues.
	entries := []Entry{
		e("DNSSEC", "DS04_RRSIG_EXPIRY_SOON", "WARNING"),
	}
	r := Compute("example.se", entries, cfg)
	// Other categories should still be 100 even though they have no entries,
	// because there is no CRITICAL to indicate an aborted run.
	for _, cat := range []string{"nameserver_health", "connectivity", "zone_consistency"} {
		cr := r.Categories[cat]
		if cr.Score != 100 {
			t.Errorf("expected %s score 100 (no CRITICAL, no penalties), got %d", cat, cr.Score)
		}
	}
}

func TestCompute_TestedFieldNormalRun(t *testing.T) {
	// In a normal run, Tested reflects whether the category received entries.
	entries := []Entry{
		e("DNSSEC", "DS07_SIGNED", "INFO"),
		e("NAMESERVER", "N15_SOFTWARE_VERSION", "NOTICE"),
	}
	r := Compute("example.se", entries, cfg)
	if !r.Categories["dnssec"].Tested {
		t.Error("expected dnssec tested=true")
	}
	if !r.Categories["nameserver_health"].Tested {
		t.Error("expected nameserver_health tested=true")
	}
	// connectivity and zone_consistency got no entries.
	if r.Categories["connectivity"].Tested {
		t.Error("expected connectivity tested=false (no entries)")
	}
	if r.Categories["zone_consistency"].Tested {
		t.Error("expected zone_consistency tested=false (no entries)")
	}
}

func TestCompute_GradeBands(t *testing.T) {
	cases := []struct {
		score int
		grade string
	}{
		{100, "A"},
		{95, "A"},
		{90, "A"},
		{89, "B"},
		{75, "B"},
		{74, "C"},
		{60, "C"},
		{59, "D"},
		{40, "D"},
		{39, "F"},
		{0, "F"},
	}
	for _, tc := range cases {
		got := scoreToGrade(tc.score, cfg.GradeBands)
		if got != tc.grade {
			t.Errorf("scoreToGrade(%d) = %q, want %q", tc.score, got, tc.grade)
		}
	}
}

func TestCompute_UnknownModuleIgnored(t *testing.T) {
	// Entries from unknown modules should not panic or affect the score.
	entries := []Entry{
		e("UNKNOWN_MODULE", "SOME_TAG", "ERROR"),
	}
	r := Compute("example.se", entries, cfg)
	if r.Score != 100 {
		t.Errorf("unknown module should not affect score, got %d", r.Score)
	}
}

func TestCompute_InfoAndDebugNopenalty(t *testing.T) {
	entries := []Entry{
		e("DNSSEC", "SOME_TAG", "INFO"),
		e("DNSSEC", "SOME_TAG", "DEBUG"),
		e("DNSSEC", "SOME_TAG", "DEBUG2"),
		e("DNSSEC", "SOME_TAG", "DEBUG3"),
	}
	r := Compute("example.se", entries, cfg)
	if r.Score != 100 {
		t.Errorf("INFO/DEBUG entries should carry no penalty, got score %d", r.Score)
	}
}

func TestCompute_ZeroEntries(t *testing.T) {
	// No entries at all: perfect score, grade A, bonus criteria not evaluable.
	r := Compute("example.se", nil, cfg)
	if r.Score != 100 {
		t.Errorf("expected score 100 for empty run, got %d", r.Score)
	}
	if r.Grade != "A" {
		t.Errorf("expected grade A for empty run, got %s", r.Grade)
	}
}

func TestCompute_CustomConfigZeroWeight(t *testing.T) {
	// Zeroing DNSSEC weight excludes it from the aggregate entirely.
	custom := cfgWith(func(c *Config) { c.CategoryWeights["dnssec"] = 0 })
	entries := []Entry{
		e("DNSSEC", "SOME_ERROR", "ERROR"),
		e("DNSSEC", "SOME_ERROR", "ERROR"),
		e("DNSSEC", "SOME_ERROR", "ERROR"),
	}
	r := Compute("example.se", entries, custom)
	// All other categories score 100; aggregate should be 100.
	if r.Score != 100 {
		t.Errorf("with zero DNSSEC weight, expected score 100, got %d", r.Score)
	}
}

// ---- Bonus criteria tests ---------------------------------------------------

// aPlusEntries meets every A+ criterion; cdsTags is the zone's CDS evidence.
func aPlusEntries(cdsTags ...string) []Entry {
	entries := []Entry{
		e("DNSSEC", "DS07_SIGNED", "INFO"),
		e("DNSSEC", "DS05_ALGO_OK", "INFO"),
		e("DNSSEC", "DS03_NSEC3_OPT_OUT_DISABLED", "INFO"),
		e("CONNECTIVITY", "IPV6_DIFFERENT_ASN", "INFO"),
		e("CONNECTIVITY", "IPV4_DIFFERENT_ASN", "INFO"),
	}
	for _, tag := range cdsTags {
		entries = append(entries, e("DNSSEC", tag, "INFO"))
	}
	return entries
}

func cfgWith(mutate func(*Config)) Config {
	c := DefaultConfig()
	mutate(&c)
	return c
}

const rolloverEvidence = "DS18_NO_CDS_CDNSKEY_BUT_ROLLOVER_EVIDENCE"

func TestBonusCDSCriterion(t *testing.T) {
	for _, tc := range []struct {
		name     string
		domain   string
		cdsTags  []string
		want     *bool
		grade    string
		eligible bool
	}{
		{"published", "example.se", []string{"DS15_HAS_CDS_AND_CDNSKEY"}, boolPtr(true), "A+", true},
		{"absent blocks A+", "example.se", []string{"DS15_NO_CDS_CDNSKEY"}, boolPtr(false), "A", false},
		{"not applicable to a TLD", "se", nil, nil, "A+", true},
		// On-demand publication (Knot DNS) must not read as false, or A+ is
		// unreachable.
		{"rollover evidence stands in", "example.se", []string{rolloverEvidence}, nil, "A+", true},
		// They should not coexist; the precedence keeps the criterion defined.
		{"DS15_HAS wins over rollover evidence", "example.se", []string{"DS15_HAS_CDS_AND_CDNSKEY", rolloverEvidence}, boolPtr(true), "A+", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := Compute(tc.domain, aPlusEntries(tc.cdsTags...), cfg)
			if r.Score != 100 {
				t.Fatalf("score = %d, want a perfect 100", r.Score)
			}
			got := r.Bonus.Criteria["cds_cdnskey_published"]
			switch {
			case tc.want == nil && got != nil:
				t.Errorf("cds_cdnskey_published = %v, want nil", *got)
			case tc.want != nil && got == nil:
				t.Errorf("cds_cdnskey_published = nil, want %v", *tc.want)
			case tc.want != nil && *got != *tc.want:
				t.Errorf("cds_cdnskey_published = %v, want %v", *got, *tc.want)
			}
			if r.Bonus.Eligible != tc.eligible {
				t.Errorf("eligible = %v, want %v", r.Bonus.Eligible, tc.eligible)
			}
			if r.Grade != tc.grade {
				t.Errorf("grade = %s, want %s", r.Grade, tc.grade)
			}
		})
	}
}

// An empty tag set means the module never ran: not-met, not not-applicable.
func TestBonusCriterionHelpers(t *testing.T) {
	for _, tc := range []struct {
		name string
		fn   func(map[string]bool) *bool
		tags []string
		want *bool
	}{
		// A TLD using opt-out is acceptable, so the criterion does not apply.
		{"nsec3 opt-out on a TLD", nsec3NonOptout, []string{"DS03_NSEC3_OPT_OUT_ENABLED_TLD"}, nil},
		{"nsec3 opt-out below a TLD", nsec3NonOptout, []string{"DS03_NSEC3_OPT_OUT_ENABLED_NON_TLD"}, boolPtr(false)},
		{"zone not signed", dnssecEnabled, []string{"DS07_NOT_SIGNED"}, boolPtr(false)},
		{"dnssec not run", dnssecEnabled, nil, boolPtr(false)},
		{"deprecated algorithm", strongAlgorithm, []string{"DS05_ALGO_DEPRECATED"}, boolPtr(false)},
		{"ipv6 disabled", ipv6AllNameservers, []string{"IPV6_DISABLED"}, boolPtr(false)},
		{"ipv6 disabled by CN01", ipv6AllNameservers, []string{"CN01_IPV6_DISABLED"}, boolPtr(false)},
		{"ipv4 addresses in different ASNs", asDiversity, []string{"IPV4_DIFFERENT_ASN"}, boolPtr(true)},
		{"ipv4 addresses in one ASN", asDiversity, []string{"IPV4_ONE_ASN"}, boolPtr(false)},
		{"AS lookup not run", asDiversity, nil, boolPtr(false)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tags := map[string]bool{}
			for _, tag := range tc.tags {
				tags[tag] = true
			}
			got := tc.fn(tags)
			switch {
			case tc.want == nil && got != nil:
				t.Fatalf("got %v, want nil", *got)
			case tc.want != nil && got == nil:
				t.Fatalf("got nil, want %v", *tc.want)
			case tc.want != nil && *got != *tc.want:
				t.Fatalf("got %v, want %v", *got, *tc.want)
			}
		})
	}
}

func TestBonus_TLDWithTrailingDot(t *testing.T) {
	// Trailing dot on TLD should still be detected correctly.
	if !isTLDZone("se.") {
		t.Error("expected isTLDZone(\"se.\") = true")
	}
	if isTLDZone("example.se.") {
		t.Error("expected isTLDZone(\"example.se.\") = false")
	}
}

func TestCompute_DisabledStacks(t *testing.T) {
	entries := []Entry{
		e("CONNECTIVITY", "IPV6_DISABLED", "INFO"),
		e("CONNECTIVITY", "CN01_IPV4_DISABLED", "INFO"),
	}
	r := Compute("example.se", entries, cfg)
	stacks := map[string]bool{}
	for _, s := range r.DisabledStacks {
		stacks[s] = true
	}
	if !stacks["ipv4"] {
		t.Error("expected ipv4 in DisabledStacks")
	}
	if !stacks["ipv6"] {
		t.Error("expected ipv6 in DisabledStacks")
	}
}

func TestCompute_NoDisabledStacks(t *testing.T) {
	r := Compute("example.se", nil, cfg)
	if len(r.DisabledStacks) != 0 {
		t.Errorf("expected no DisabledStacks, got %v", r.DisabledStacks)
	}
}

// ---- TagPenalties tests -----------------------------------------------------

func TestLoadConfig_MigratesRenamedTagPenalties(t *testing.T) {
	// A scoring configuration written before the bailiwick identifiers were
	// retired must keep overriding the same tags under their current names.
	// Stored configurations are full snapshots, so without this rewrite the
	// override would simply stop applying.
	path := filepath.Join(t.TempDir(), "scoring.json")
	body := `{"tag_penalties": {
		"IN_BAILIWICK_ADDR_MISMATCH": 40,
		"OUT_OF_BAILIWICK_ADDR_MISMATCH": 30,
		"IN_BAILIWICK_GLUE_MISSING": 25
	}}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	loaded, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	for tag, want := range map[string]int{
		"IN_DOMAIN_ADDR_MISMATCH":     40,
		"NOT_IN_DOMAIN_ADDR_MISMATCH": 30,
		"IN_DOMAIN_GLUE_MISSING":      25,
	} {
		if got := loaded.TagPenalties[tag]; got != want {
			t.Errorf("penalty for %s = %d, want %d", tag, got, want)
		}
	}
	for _, tag := range []string{
		"IN_BAILIWICK_ADDR_MISMATCH",
		"OUT_OF_BAILIWICK_ADDR_MISMATCH",
		"IN_BAILIWICK_GLUE_MISSING",
	} {
		if _, ok := loaded.TagPenalties[tag]; ok {
			t.Errorf("retired identifier %s survived the load", tag)
		}
	}
}

func TestLoadConfig_RenamedTagCurrentKeyWins(t *testing.T) {
	// A configuration carrying both spellings must keep the current one.
	path := filepath.Join(t.TempDir(), "scoring.json")
	body := `{"tag_penalties": {
		"IN_BAILIWICK_ADDR_MISMATCH": 40,
		"IN_DOMAIN_ADDR_MISMATCH": 7
	}}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	loaded, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if got := loaded.TagPenalties["IN_DOMAIN_ADDR_MISMATCH"]; got != 7 {
		t.Errorf("penalty for IN_DOMAIN_ADDR_MISMATCH = %d, want 7", got)
	}
}

func TestCompute_TagPenaltyScoreImpact(t *testing.T) {
	// DS07_NOT_SIGNED with 20-pt penalty: dnssec sub-score = 80.
	// Weighted: (80*1.5 + 100*1.2 + 100*1.0 + 100*0.8) / 4.5
	//         = (120 + 120 + 100 + 80) / 4.5 = 420/4.5 = 93
	entries := []Entry{
		e("DNSSEC", "DS07_NOT_SIGNED", "WARNING"),
	}
	r := Compute("example.se", entries, cfg)
	if r.Score != 93 {
		t.Errorf("expected score 93 with DS07_NOT_SIGNED penalty, got %d", r.Score)
	}
}

// Severity alone gives 5 points for WARNING and 1 for NOTICE.
func TestComputeTagPenalties(t *testing.T) {
	for _, tc := range []struct {
		name     string
		entry    Entry
		config   Config
		category string
		want     int
	}{
		{"not signed overrides its severity", e("DNSSEC", "DS07_NOT_SIGNED", "WARNING"), cfg, "dnssec", 20},
		{"no DS for a signed zone", e("DNSSEC", "DS07_NO_DS_FOR_SIGNED_ZONE", "WARNING"), cfg, "dnssec", 20},
		{"no IPv6 NS at the child", e("DELEGATION", "NO_IPV6_NS_CHILD", "NOTICE"), cfg, "nameserver_health", 20},
		{"no IPv6 NS in the delegation", e("DELEGATION", "NO_IPV6_NS_DEL", "NOTICE"), cfg, "nameserver_health", 20},
		{"not enough IPv6 NS keeps its severity", e("DELEGATION", "NOT_ENOUGH_IPV6_NS_CHILD", "ERROR"), cfg, "nameserver_health", 20},
		{"an unlisted tag keeps its severity", e("DNSSEC", "DS04_RRSIG_EXPIRY_SOON", "WARNING"), cfg, "dnssec", 5},
		{
			name:     "a zero override cancels the severity penalty",
			entry:    e("DNSSEC", "DS07_NOT_SIGNED", "WARNING"),
			config:   cfgWith(func(c *Config) { c.TagPenalties["DS07_NOT_SIGNED"] = 0 }),
			category: "dnssec",
			want:     0,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := Compute("example.se", []Entry{tc.entry}, tc.config)
			if got := r.Categories[tc.category].Penalties; got != tc.want {
				t.Errorf("%s penalties = %d, want %d", tc.category, got, tc.want)
			}
		})
	}
}

func TestComputeTagPenaltyScoreImpact(t *testing.T) {
	// DS07_NOT_SIGNED costs 20, so dnssec scores 80. Weighted:
	// (80*1.5 + 100*1.2 + 100*1.0 + 100*0.8) / 4.5 = 420/4.5 = 93.
	r := Compute("example.se", []Entry{e("DNSSEC", "DS07_NOT_SIGNED", "WARNING")}, cfg)
	if r.Score != 93 {
		t.Errorf("expected score 93 with DS07_NOT_SIGNED penalty, got %d", r.Score)
	}
}

// These tags report a fact, not a fault; a bare NOTICE would cost a point each.
func TestComputeScoreNeutralTags(t *testing.T) {
	for _, tc := range []struct {
		name     string
		domain   string
		module   string
		category string
		tags     []string
	}{
		{"software version", "example.se", "NAMESERVER", "nameserver_health", []string{"N15_SOFTWARE_VERSION", "N15_SOFTWARE_VERSION"}},
		{"NSID present", "example.se", "NAMESERVER", "nameserver_health", []string{"N16_HAS_NSID", "N16_HAS_NSID"}},
		{
			name: "unsupported RSA exponent", domain: "example.se",
			module: "DNSSEC", category: "dnssec",
			tags: []string{"DS02_RSA_EXPONENT_UNSUPPORTED", "DS08_RSA_EXPONENT_UNSUPPORTED", "DS09_RSA_EXPONENT_UNSUPPORTED"},
		},
		{"SPF macro target", "example.com", "ZONE", "zone_consistency", []string{"Z13_SPF_MACRO_TARGET", "Z13_SPF_MACRO_TARGET"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			entries := make([]Entry, len(tc.tags))
			for i, tag := range tc.tags {
				entries[i] = e(tc.module, tag, "NOTICE")
			}
			r := Compute(tc.domain, entries, cfg)
			if r.Score != 100 {
				t.Errorf("score = %d, want 100", r.Score)
			}
			if got := r.Categories[tc.category].Penalties; got != 0 {
				t.Errorf("%s penalties = %d, want 0", tc.category, got)
			}
		})
	}
}

func TestBonus_DisabledCriteriaNotInResult(t *testing.T) {
	custom := cfgWith(func(c *Config) {
		c.BonusCriteria.CDSCDNSKEYPublished = false
		c.BonusCriteria.IPv6AllNameservers = false
	})

	r := Compute("example.se", nil, custom)
	if _, ok := r.Bonus.Criteria["cds_cdnskey_published"]; ok {
		t.Error("disabled criterion should not appear in result")
	}
	if _, ok := r.Bonus.Criteria["ipv6_all_nameservers"]; ok {
		t.Error("disabled criterion should not appear in result")
	}
}

func TestSystemTagsAreScoreNeutral(t *testing.T) {
	// SYSTEM entries map to no scoring category.
	base := []Entry{{Module: "BASIC", Tag: "B01_OK", Level: "NOTICE"}}
	withGuard := append(append([]Entry{}, base...),
		Entry{Module: "System", Tag: "NON_GLOBAL_QUERY_BLOCKED", Level: "NOTICE"})

	rBase := Compute("example.se", base, cfg)
	rGuard := Compute("example.se", withGuard, cfg)
	if rBase.Score != rGuard.Score {
		t.Errorf("SYSTEM tag changed score: %d vs %d", rBase.Score, rGuard.Score)
	}
	if rBase.Grade != rGuard.Grade {
		t.Errorf("SYSTEM tag changed grade: %s vs %s", rBase.Grade, rGuard.Grade)
	}
}
