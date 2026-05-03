package scoring

import (
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
	// No entries above INFO — should score 100 and grade A (not A+ without bonus).
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
	// did run. ALL category scores must be 0 when CRITICAL is present —
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
	// perfect score — this is the normal case where a category had no issues.
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
	custom := DefaultConfig()
	custom.CategoryWeights["dnssec"] = 0
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

func TestBonus_APlus_AllCriteriaMet(t *testing.T) {
	entries := []Entry{
		// DNSSEC signed
		e("DNSSEC", "DS07_SIGNED", "INFO"),
		// Strong algorithm
		e("DNSSEC", "DS05_ALGO_OK", "INFO"),
		// NSEC3 opt-out disabled
		e("DNSSEC", "DS03_NSEC3_OPT_OUT_DISABLED", "INFO"),
		// CDS/CDNSKEY present
		e("DNSSEC", "DS15_HAS_CDS_AND_CDNSKEY", "INFO"),
		// IPv6 ASN found (implies IPv6 reachable)
		e("CONNECTIVITY", "IPV6_DIFFERENT_ASN", "INFO"),
		// AS diversity
		e("CONNECTIVITY", "IPV4_DIFFERENT_ASN", "INFO"),
	}
	r := Compute("example.se", entries, cfg)
	if r.Score != 100 {
		t.Fatalf("expected perfect score for A+ check, got %d", r.Score)
	}
	if !r.Bonus.Eligible {
		t.Error("expected A+ eligible")
	}
	if r.Grade != "A+" {
		t.Errorf("expected grade A+, got %s", r.Grade)
	}
}

func TestBonus_AGradeWhenMissingCriteria(t *testing.T) {
	// Perfect score but CDS/CDNSKEY missing.
	entries := []Entry{
		e("DNSSEC", "DS07_SIGNED", "INFO"),
		e("DNSSEC", "DS05_ALGO_OK", "INFO"),
		e("DNSSEC", "DS03_NSEC3_OPT_OUT_DISABLED", "INFO"),
		e("DNSSEC", "DS15_NO_CDS_CDNSKEY", "INFO"), // missing
		e("CONNECTIVITY", "IPV6_DIFFERENT_ASN", "INFO"),
		e("CONNECTIVITY", "IPV4_DIFFERENT_ASN", "INFO"),
	}
	r := Compute("example.se", entries, cfg)
	if r.Score != 100 {
		t.Fatalf("expected perfect score, got %d", r.Score)
	}
	if r.Bonus.Eligible {
		t.Error("should not be A+ eligible when CDS/CDNSKEY missing")
	}
	if r.Grade != "A" {
		t.Errorf("expected grade A, got %s", r.Grade)
	}
	v := r.Bonus.Criteria["cds_cdnskey_published"]
	if v == nil || *v {
		t.Error("expected cds_cdnskey_published = false")
	}
}

func TestBonus_CDSNotApplicableForTLD(t *testing.T) {
	// TLD zones: CDS/CDNSKEY criterion must be nil (not applicable).
	entries := []Entry{
		e("DNSSEC", "DS07_SIGNED", "INFO"),
		e("DNSSEC", "DS05_ALGO_OK", "INFO"),
		e("DNSSEC", "DS03_NSEC3_OPT_OUT_DISABLED", "INFO"),
		e("CONNECTIVITY", "IPV6_DIFFERENT_ASN", "INFO"),
		e("CONNECTIVITY", "IPV4_DIFFERENT_ASN", "INFO"),
	}
	r := Compute("se", entries, cfg) // TLD
	v := r.Bonus.Criteria["cds_cdnskey_published"]
	if v != nil {
		t.Errorf("expected cds_cdnskey_published nil for TLD, got %v", *v)
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

func TestBonus_NSEC3OptOutTLDIsNil(t *testing.T) {
	// DS03_NSEC3_OPT_OUT_ENABLED_TLD means TLD using opt-out, which is
	// acceptable: criterion should be nil (not applicable), not false.
	tags := map[string]bool{"DS03_NSEC3_OPT_OUT_ENABLED_TLD": true}
	v := nsec3NonOptout(tags)
	if v != nil {
		t.Errorf("expected nil for TLD opt-out tag, got %v", *v)
	}
}

func TestBonus_NSEC3OptOutNonTLDIsFalse(t *testing.T) {
	tags := map[string]bool{"DS03_NSEC3_OPT_OUT_ENABLED_NON_TLD": true}
	v := nsec3NonOptout(tags)
	if v == nil || *v {
		t.Error("expected false for non-TLD opt-out")
	}
}

func TestBonus_DNSSECNotSigned(t *testing.T) {
	tags := map[string]bool{"DS07_NOT_SIGNED": true}
	v := dnssecEnabled(tags)
	if v == nil || *v {
		t.Error("expected false when DS07_NOT_SIGNED present")
	}
}

func TestBonus_DNSSECNotRun(t *testing.T) {
	// Neither SIGNED nor NOT_SIGNED: test not run → treated as not met (false).
	tags := map[string]bool{}
	v := dnssecEnabled(tags)
	if v == nil || *v {
		t.Error("expected false when DNSSEC test not run")
	}
}

func TestBonus_StrongAlgorithmDeprecated(t *testing.T) {
	tags := map[string]bool{"DS05_ALGO_DEPRECATED": true}
	v := strongAlgorithm(tags)
	if v == nil || *v {
		t.Error("expected false for deprecated algorithm")
	}
}

func TestBonus_IPv6DisabledIsFalse(t *testing.T) {
	tags := map[string]bool{"IPV6_DISABLED": true}
	v := ipv6AllNameservers(tags)
	if v == nil || *v {
		t.Error("expected false when IPV6_DISABLED present")
	}
}

func TestBonus_IPv6CN01DisabledIsFalse(t *testing.T) {
	tags := map[string]bool{"CN01_IPV6_DISABLED": true}
	v := ipv6AllNameservers(tags)
	if v == nil || *v {
		t.Error("expected false when CN01_IPV6_DISABLED present")
	}
}

func TestBonus_ASDiversityTrue(t *testing.T) {
	tags := map[string]bool{"IPV4_DIFFERENT_ASN": true}
	v := asDiversity(tags)
	if v == nil || !*v {
		t.Error("expected true for IPV4_DIFFERENT_ASN")
	}
}

func TestBonus_ASDiversityFalse(t *testing.T) {
	tags := map[string]bool{"IPV4_ONE_ASN": true}
	v := asDiversity(tags)
	if v == nil || *v {
		t.Error("expected false for IPV4_ONE_ASN with no IPv6 diversity")
	}
}

func TestBonus_ASDiversityNotRun(t *testing.T) {
	// AS lookup not run → treated as not met (false).
	tags := map[string]bool{}
	v := asDiversity(tags)
	if v == nil || *v {
		t.Error("expected false when AS lookup not run")
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

func TestCompute_TagPenaltyOverridesSeverity(t *testing.T) {
	// DS07_NOT_SIGNED is WARNING (5 pts by severity) but gets 20 pts via TagPenalties.
	entries := []Entry{
		e("DNSSEC", "DS07_NOT_SIGNED", "WARNING"),
	}
	r := Compute("example.se", entries, cfg)
	cat := r.Categories["dnssec"]
	if cat.Penalties != 20 {
		t.Errorf("expected dnssec penalties 20 (tag override), got %d", cat.Penalties)
	}
}

func TestCompute_NoDSForSignedZonePenalty(t *testing.T) {
	// DS07_NO_DS_FOR_SIGNED_ZONE is WARNING but gets 20 pts via TagPenalties.
	entries := []Entry{
		e("DNSSEC", "DS07_NO_DS_FOR_SIGNED_ZONE", "WARNING"),
	}
	r := Compute("example.se", entries, cfg)
	cat := r.Categories["dnssec"]
	if cat.Penalties != 20 {
		t.Errorf("expected dnssec penalties 20 (tag override), got %d", cat.Penalties)
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

func TestCompute_TagPenaltyCanBeDisabledByZero(t *testing.T) {
	// Setting a tag penalty to 0 means no penalty even if severity would give one.
	custom := DefaultConfig()
	custom.TagPenalties["DS07_NOT_SIGNED"] = 0
	entries := []Entry{
		e("DNSSEC", "DS07_NOT_SIGNED", "WARNING"),
	}
	r := Compute("example.se", entries, custom)
	cat := r.Categories["dnssec"]
	if cat.Penalties != 0 {
		t.Errorf("expected 0 penalty when tag penalty set to 0, got %d", cat.Penalties)
	}
}

func TestCompute_NoIPv6NSChildPenalty(t *testing.T) {
	// NO_IPV6_NS_CHILD is NOTICE (1 pt) but must get 20 pts via TagPenalties.
	entries := []Entry{
		e("DELEGATION", "NO_IPV6_NS_CHILD", "NOTICE"),
	}
	r := Compute("example.se", entries, cfg)
	cat := r.Categories["nameserver_health"]
	if cat.Penalties != 20 {
		t.Errorf("expected nameserver_health penalties 20, got %d", cat.Penalties)
	}
}

func TestCompute_NoIPv6NSDelPenalty(t *testing.T) {
	entries := []Entry{
		e("DELEGATION", "NO_IPV6_NS_DEL", "NOTICE"),
	}
	r := Compute("example.se", entries, cfg)
	cat := r.Categories["nameserver_health"]
	if cat.Penalties != 20 {
		t.Errorf("expected nameserver_health penalties 20, got %d", cat.Penalties)
	}
}

func TestCompute_NotEnoughIPv6UsesErrorSeverity(t *testing.T) {
	// NOT_ENOUGH_IPV6_NS_CHILD is already ERROR (20 pts by severity); no override needed.
	entries := []Entry{
		e("DELEGATION", "NOT_ENOUGH_IPV6_NS_CHILD", "ERROR"),
	}
	r := Compute("example.se", entries, cfg)
	cat := r.Categories["nameserver_health"]
	if cat.Penalties != 20 {
		t.Errorf("expected nameserver_health penalties 20, got %d", cat.Penalties)
	}
}

func TestCompute_N15SoftwareVersionNopenalty(t *testing.T) {
	// N15_SOFTWARE_VERSION is NOTICE but must carry zero penalty — it is a
	// cosmetic privacy notice, not a zone health issue.
	entries := []Entry{
		e("NAMESERVER", "N15_SOFTWARE_VERSION", "NOTICE"),
		e("NAMESERVER", "N15_SOFTWARE_VERSION", "NOTICE"),
	}
	r := Compute("example.se", entries, cfg)
	if r.Score != 100 {
		t.Errorf("expected score 100 with N15_SOFTWARE_VERSION entries, got %d", r.Score)
	}
}

func TestCompute_N16HasNSIDNopenalty(t *testing.T) {
	// N16_HAS_NSID is NOTICE but should be informational only when a server
	// returns NSID in response to an explicit NSID query.
	entries := []Entry{
		e("NAMESERVER", "N16_HAS_NSID", "NOTICE"),
		e("NAMESERVER", "N16_HAS_NSID", "NOTICE"),
	}
	r := Compute("example.se", entries, cfg)
	if r.Score != 100 {
		t.Errorf("expected score 100 with N16_HAS_NSID entries, got %d", r.Score)
	}
	cat := r.Categories["nameserver_health"]
	if cat.Penalties != 0 {
		t.Errorf("expected nameserver_health penalties 0 for N16_HAS_NSID, got %d", cat.Penalties)
	}
}

func TestCompute_TagPenaltyNotAffectingOtherTags(t *testing.T) {
	// A regular WARNING tag should still use the severity penalty (5 pts).
	entries := []Entry{
		e("DNSSEC", "DS04_RRSIG_EXPIRY_SOON", "WARNING"),
	}
	r := Compute("example.se", entries, cfg)
	cat := r.Categories["dnssec"]
	if cat.Penalties != 5 {
		t.Errorf("expected severity-based penalty 5 for unlisted tag, got %d", cat.Penalties)
	}
}

func TestBonus_DisabledCriteriaNotInResult(t *testing.T) {
	custom := DefaultConfig()
	custom.BonusCriteria.CDSCDNSKEYPublished = false
	custom.BonusCriteria.IPv6AllNameservers = false

	r := Compute("example.se", nil, custom)
	if _, ok := r.Bonus.Criteria["cds_cdnskey_published"]; ok {
		t.Error("disabled criterion should not appear in result")
	}
	if _, ok := r.Bonus.Criteria["ipv6_all_nameservers"]; ok {
		t.Error("disabled criterion should not appear in result")
	}
}

// TestBonus_CDSRolloverEvidenceIsNil checks that DS18_NO_CDS_CDNSKEY_BUT_ROLLOVER_EVIDENCE
// causes the cds_cdnskey_published criterion to be nil (not-applicable, not false),
// so that on-demand CDS/CDNSKEY publication (e.g. Knot DNS) does not block A+.
func TestBonus_CDSRolloverEvidenceIsNil(t *testing.T) {
	entries := []Entry{
		e("DNSSEC", "DS07_SIGNED", "INFO"),
		e("DNSSEC", "DS05_ALGO_OK", "INFO"),
		e("DNSSEC", "DS03_NSEC3_OPT_OUT_DISABLED", "INFO"),
		// No DS15_HAS_* tag; rollover evidence seen instead.
		e("DNSSEC", "DS18_NO_CDS_CDNSKEY_BUT_ROLLOVER_EVIDENCE", "INFO"),
		e("CONNECTIVITY", "IPV6_DIFFERENT_ASN", "INFO"),
		e("CONNECTIVITY", "IPV4_DIFFERENT_ASN", "INFO"),
	}
	r := Compute("example.se", entries, cfg)
	v := r.Bonus.Criteria["cds_cdnskey_published"]
	if v != nil {
		t.Errorf("expected cds_cdnskey_published nil for rollover-evidence zone, got %v", *v)
	}
}

// TestBonus_APlusWithRolloverEvidence verifies a zone with rollover evidence but
// no CDS/CDNSKEY and a perfect score receives grade A+.
func TestBonus_APlusWithRolloverEvidence(t *testing.T) {
	entries := []Entry{
		e("DNSSEC", "DS07_SIGNED", "INFO"),
		e("DNSSEC", "DS05_ALGO_OK", "INFO"),
		e("DNSSEC", "DS03_NSEC3_OPT_OUT_DISABLED", "INFO"),
		e("DNSSEC", "DS18_NO_CDS_CDNSKEY_BUT_ROLLOVER_EVIDENCE", "INFO"),
		e("CONNECTIVITY", "IPV6_DIFFERENT_ASN", "INFO"),
		e("CONNECTIVITY", "IPV4_DIFFERENT_ASN", "INFO"),
	}
	r := Compute("example.se", entries, cfg)
	if r.Score != 100 {
		t.Fatalf("expected perfect score, got %d", r.Score)
	}
	if !r.Bonus.Eligible {
		t.Error("expected A+ eligible when rollover evidence substitutes CDS/CDNSKEY")
	}
	if r.Grade != "A+" {
		t.Errorf("expected grade A+, got %s", r.Grade)
	}
}

// TestBonus_NoCDSWithoutRolloverEvidence verifies that absence of both
// DS15_HAS_* and DS18_NO_CDS_CDNSKEY_BUT_ROLLOVER_EVIDENCE still blocks A+.
func TestBonus_NoCDSWithoutRolloverEvidence(t *testing.T) {
	entries := []Entry{
		e("DNSSEC", "DS07_SIGNED", "INFO"),
		e("DNSSEC", "DS05_ALGO_OK", "INFO"),
		e("DNSSEC", "DS03_NSEC3_OPT_OUT_DISABLED", "INFO"),
		e("DNSSEC", "DS15_NO_CDS_CDNSKEY", "INFO"),
		e("CONNECTIVITY", "IPV6_DIFFERENT_ASN", "INFO"),
		e("CONNECTIVITY", "IPV4_DIFFERENT_ASN", "INFO"),
	}
	r := Compute("example.se", entries, cfg)
	if r.Score != 100 {
		t.Fatalf("expected perfect score, got %d", r.Score)
	}
	if r.Bonus.Eligible {
		t.Error("should not be A+ eligible when no CDS/CDNSKEY and no rollover evidence")
	}
	v := r.Bonus.Criteria["cds_cdnskey_published"]
	if v == nil || *v {
		t.Error("expected cds_cdnskey_published = false")
	}
}

// TestBonus_DS15HasCDSWinsOverDS18RolloverEvidence locks the precedence: when a
// DS15_HAS_* tag is present, cds_cdnskey_published is true regardless of any
// DS18_NO_CDS_CDNSKEY_BUT_ROLLOVER_EVIDENCE marker (which would otherwise yield
// nil). The two should never coexist in practice, but the precedence keeps the
// criterion well-defined if they do.
func TestBonus_DS15HasCDSWinsOverDS18RolloverEvidence(t *testing.T) {
	entries := []Entry{
		e("DNSSEC", "DS07_SIGNED", "INFO"),
		e("DNSSEC", "DS05_ALGO_OK", "INFO"),
		e("DNSSEC", "DS03_NSEC3_OPT_OUT_DISABLED", "INFO"),
		e("DNSSEC", "DS15_HAS_CDS_AND_CDNSKEY", "INFO"),
		e("DNSSEC", "DS18_NO_CDS_CDNSKEY_BUT_ROLLOVER_EVIDENCE", "INFO"),
		e("CONNECTIVITY", "IPV6_DIFFERENT_ASN", "INFO"),
		e("CONNECTIVITY", "IPV4_DIFFERENT_ASN", "INFO"),
	}
	r := Compute("example.se", entries, cfg)
	v := r.Bonus.Criteria["cds_cdnskey_published"]
	if v == nil || !*v {
		t.Errorf("expected cds_cdnskey_published = true (DS15_HAS_* wins), got %v", v)
	}
}
