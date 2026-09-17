package server

import (
	"sort"
	"strconv"
)

// Fact-category identifiers materialized by the projector and rendered on
// the overview. Kept as string constants here so the projector (which
// imports this package) and the display registry below agree on the
// wire-format tokens. Adding a new statistic means adding one constant
// plus one entry in factCategoryDisplay below.
const (
	FactCategorySeverity          = "severity"
	FactCategoryDNSSECPosture     = "dnssec_posture"
	FactCategoryGrade             = "grade"
	FactCategoryDNSKEYAlgorithm   = "dnskey_algo"
	FactCategoryIPv6Coverage      = "ipv6_coverage"
	FactCategoryDNSKEYAlgoWeakest = "dnskey_algo_weakest"

	FactKeySigned    = "signed"
	FactKeyUnsigned  = "unsigned"
	FactKeyNSEC      = "nsec"
	FactKeyNSEC3     = "nsec3"
	FactKeyNSECMixed = "mixed"

	FactKeyCoverageFull    = "full"
	FactKeyCoveragePartial = "partial"
	FactKeyCoverageNone    = "none"
)

// factCategoryDisplay carries the UI-side metadata for one category:
// human-readable label, short description, default bar tone, and per-key
// helpers. None of this is stored in the database; all of it lives next
// to the projector extractor so changing a label is a code change
// reviewed like any other.
type factCategoryDisplay struct {
	Label       string
	Description string
	// Order controls the section order on the overview. Lower first.
	Order int
	// KeyLabel maps the stable key token to the human label shown on a
	// bar segment (e.g. "8" -> "RSASHA256"). Return the raw key when no
	// friendly label is known.
	KeyLabel func(key string) string
	// KeyTone maps a key to one of the tone tokens the UI understands
	// ("ok", "notice", "warning", "error", "critical", "neutral").
	KeyTone func(key string) string
	// KeyOrder returns a sort index within the category. Lower first.
	KeyOrder func(key string) int
}

// factCategoryDisplays is the registry consumed by the cohort detail
// handler to build fact_distributions response payloads.
var factCategoryDisplays = map[string]factCategoryDisplay{
	FactCategorySeverity: {
		Label:       "Domain health",
		Description: "Each domain counted by the worst severity in its latest run.",
		Order:       5,
		KeyLabel:    severityKeyLabel,
		KeyTone:     severityKeyTone,
		KeyOrder:    severityKeyOrder,
	},
	FactCategoryDNSSECPosture: {
		Label:       "DNSSEC posture",
		Description: "Whether domains are signed and which denial-of-existence mode they use.",
		Order:       10,
		KeyLabel:    dnssecPostureKeyLabel,
		KeyTone:     dnssecPostureKeyTone,
		KeyOrder:    dnssecPostureKeyOrder,
	},
	FactCategoryGrade: {
		Label: "Grade distribution",
		// Grades come from the scoring config that was active when the
		// run graduated, so the set of labels can vary across runs if a
		// cohort was scored under different configs. The display falls
		// back to neutral tone and lexical ordering for unknown labels.
		Description: "Letter grade assigned by the scoring engine at run time.",
		Order:       15,
		KeyLabel:    gradeKeyLabel,
		KeyTone:     gradeKeyTone,
		KeyOrder:    gradeKeyOrder,
	},
	FactCategoryDNSKEYAlgorithm: {
		Label:       "DNSKEY algorithms",
		Description: "Signing algorithms published by the cohort's signed domains.",
		Order:       20,
		KeyLabel:    dnskeyAlgorithmKeyLabel,
		KeyTone:     dnskeyAlgorithmKeyTone,
		KeyOrder:    dnskeyAlgorithmKeyOrder,
	},
	FactCategoryIPv6Coverage: {
		Label:       "IPv6 coverage",
		Description: "How many of a domain's nameservers publish an IPv6 address.",
		Order:       12,
		KeyLabel:    coverageKeyLabel,
		KeyTone:     coverageKeyTone,
		KeyOrder:    coverageKeyOrder,
	},
	FactCategoryDNSKEYAlgoWeakest: {
		Label: "Weakest signing algorithm",
		// A validator accepts any algorithm it supports, so a zone is only
		// as strong as the weakest one it publishes. One bucket per signed
		// domain, unlike the dnskey_algo bar.
		Description: "The weakest signing algorithm each signed domain publishes.",
		Order:       22,
		KeyLabel:    dnskeyAlgorithmKeyLabel,
		KeyTone:     dnskeyAlgorithmKeyTone,
		KeyOrder:    DNSKEYAlgorithmWeaknessRank,
	},
}

func coverageKeyLabel(key string) string {
	switch key {
	case FactKeyCoverageFull:
		return "All nameservers"
	case FactKeyCoveragePartial:
		return "Some nameservers"
	case FactKeyCoverageNone:
		return "None"
	}
	return key
}

func coverageKeyTone(key string) string {
	switch key {
	case FactKeyCoverageFull:
		return "ok"
	case FactKeyCoveragePartial:
		return "warning"
	case FactKeyCoverageNone:
		return "error"
	}
	return "neutral"
}

func coverageKeyOrder(key string) int {
	switch key {
	case FactKeyCoverageNone:
		return 0
	case FactKeyCoveragePartial:
		return 1
	case FactKeyCoverageFull:
		return 2
	}
	return 99
}

// severityLabels mirrors the worst_level set the projector emits per run.
// Keys are uppercase to match the engine's level vocabulary.
var severityLabels = map[string]string{
	"OK":       "OK",
	"NOTICE":   "Notice",
	"WARNING":  "Warning",
	"ERROR":    "Error",
	"CRITICAL": "Critical",
}

var severityTones = map[string]string{
	"OK":       "ok",
	"NOTICE":   "notice",
	"WARNING":  "warning",
	"ERROR":    "error",
	"CRITICAL": "critical",
}

var severityOrder = map[string]int{
	"OK":       0,
	"NOTICE":   1,
	"WARNING":  2,
	"ERROR":    3,
	"CRITICAL": 4,
}

func severityKeyLabel(key string) string {
	if label, ok := severityLabels[key]; ok {
		return label
	}
	return key
}

func severityKeyTone(key string) string {
	if tone, ok := severityTones[key]; ok {
		return tone
	}
	return "neutral"
}

func severityKeyOrder(key string) int {
	if n, ok := severityOrder[key]; ok {
		return n
	}
	return 99
}

func dnssecPostureKeyLabel(key string) string {
	switch key {
	case FactKeyUnsigned:
		return "Unsigned"
	case FactKeySigned:
		return "Signed"
	case FactKeyNSEC:
		return "NSEC"
	case FactKeyNSEC3:
		return "NSEC3"
	case FactKeyNSECMixed:
		return "Mixed NSEC/NSEC3"
	}
	return key
}

// Tones distinguish all five posture states so the chart is legible.
// Unsigned and mixed (mid-rollover) flag as warning. Signed zones use
// ok/notice to separate the NSEC variants: nsec3 (modern, preferred)
// gets the same green ok as a broadly-signed zone; nsec (older) gets
// notice (blue) so it reads as a distinct, still-valid category.
func dnssecPostureKeyTone(key string) string {
	switch key {
	case FactKeyUnsigned, FactKeyNSECMixed:
		return "warning"
	case FactKeySigned, FactKeyNSEC3:
		return "ok"
	case FactKeyNSEC:
		return "notice"
	}
	return "neutral"
}

// dnssecPostureKeys is the closed set of posture buckets, in display order.
var dnssecPostureKeys = []string{
	FactKeyUnsigned, FactKeySigned, FactKeyNSEC, FactKeyNSEC3, FactKeyNSECMixed,
}

// isValidDNSSECPostureKey reports whether key names a posture bucket.
func isValidDNSSECPostureKey(key string) bool {
	for _, k := range dnssecPostureKeys {
		if k == key {
			return true
		}
	}
	return false
}

// dnssecPostureDisplay returns the label and tone for a posture key.
func dnssecPostureDisplay(key string) (label, tone string) {
	if key == "" {
		return "", ""
	}
	return dnssecPostureKeyLabel(key), dnssecPostureKeyTone(key)
}

func dnssecPostureKeyOrder(key string) int {
	switch key {
	case FactKeyUnsigned:
		return 0
	case FactKeySigned:
		return 1
	case FactKeyNSEC:
		return 2
	case FactKeyNSEC3:
		return 3
	case FactKeyNSECMixed:
		return 4
	}
	return 99
}

// dnskeyAlgorithmMnemonics mirrors the engine's algoProperties table for
// the algorithms a TLD cohort is realistically going to see. Kept local
// so this package does not pull in engine imports for one string table.
// Unknown algo numbers render as "ALGO <n>".
var dnskeyAlgorithmMnemonics = map[int]string{
	1:  "RSAMD5",
	3:  "DSA",
	5:  "RSASHA1",
	6:  "DSA-NSEC3-SHA1",
	7:  "RSASHA1-NSEC3-SHA1",
	8:  "RSASHA256",
	10: "RSASHA512",
	12: "ECC-GOST",
	13: "ECDSAP256SHA256",
	14: "ECDSAP384SHA384",
	15: "ED25519",
	16: "ED448",
	17: "SM2SM3",
	18: "MLDSA44",
	23: "ECC-GOST12",
}

// dnskeyAlgorithmTones colors each algorithm by current best-practice:
// modern curves green, SHA-256 RSA blue (acceptable), SHA-1 family red
// (deprecated), unknown/private neutral. Regional national-standard curves
// are sound but not the default choice, so they get the acceptable tone.
var dnskeyAlgorithmTones = map[int]string{
	1:  "error",
	3:  "error",
	5:  "error",
	6:  "error",
	7:  "warning",
	8:  "notice",
	10: "notice",
	12: "warning",
	13: "ok",
	14: "ok",
	15: "ok",
	16: "ok",
	17: "notice",
	18: "ok",
	23: "notice",
}

func dnskeyAlgorithmKeyLabel(key string) string {
	n, err := strconv.Atoi(key)
	if err != nil {
		return key
	}
	if label, ok := dnskeyAlgorithmMnemonics[n]; ok {
		return label
	}
	return "ALGO " + key
}

func dnskeyAlgorithmKeyTone(key string) string {
	n, err := strconv.Atoi(key)
	if err != nil {
		return "neutral"
	}
	if tone, ok := dnskeyAlgorithmTones[n]; ok {
		return tone
	}
	return "neutral"
}

func dnskeyAlgorithmKeyOrder(key string) int {
	n, err := strconv.Atoi(key)
	if err != nil {
		return 1 << 30
	}
	return n
}

// dnskeyAlgorithmClassRank orders the tone classes weakest first. Derived
// from dnskeyAlgorithmTones so there is no second policy table to keep in
// sync; an algorithm with no tone entry (private, unassigned) gets class 0
// because no validator can use it.
var dnskeyAlgorithmClassRank = map[string]int{
	"error":   1,
	"warning": 2,
	"notice":  3,
	"ok":      4,
}

// DNSKEYAlgorithmWeaknessRank ranks a DNSKEY algorithm number weakest
// first: tone class, then algorithm number within the class. Shared by the
// weakest-algorithm bar order, the projector's per-zone pick, and the
// domain-list sort.
func DNSKEYAlgorithmWeaknessRank(key string) int {
	n, err := strconv.Atoi(key)
	if err != nil {
		return 1 << 30
	}
	return dnskeyAlgorithmClassRank[dnskeyAlgorithmTones[n]]*1000 + n
}

// gradeTones maps the default scoring config's letter grades to bar
// tones. A+ and A are ok (green), B is notice, C is warning, D is error,
// F is critical. Custom scoring configs that emit different labels
// fall back to neutral.
var gradeTones = map[string]string{
	"A+": "ok",
	"A":  "ok",
	"B":  "notice",
	"C":  "warning",
	"D":  "error",
	"F":  "critical",
}

// gradeOrder pins A+/A/B/C/D/F in the expected display order. Unknown
// grades sort lexically after the known set.
var gradeOrder = map[string]int{
	"A+": 0,
	"A":  1,
	"B":  2,
	"C":  3,
	"D":  4,
	"F":  5,
}

func gradeKeyLabel(key string) string { return key }

func gradeKeyTone(key string) string {
	if tone, ok := gradeTones[key]; ok {
		return tone
	}
	return "neutral"
}

func gradeKeyOrder(key string) int {
	if n, ok := gradeOrder[key]; ok {
		return n
	}
	return 1 << 30
}

// PublicAnalysisFactBucket is one (key, count) bar segment inside a
// category distribution on the overview response.
type PublicAnalysisFactBucket struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Tone  string `json:"tone"`
	Count int    `json:"count"`
	Order int    `json:"order"`
}

// PublicAnalysisFactDistribution is one category's payload inside
// fact_distributions on the cohort detail response.
type PublicAnalysisFactDistribution struct {
	Category    string                     `json:"category"`
	Label       string                     `json:"label"`
	Description string                     `json:"description,omitempty"`
	Order       int                        `json:"order"`
	Buckets     []PublicAnalysisFactBucket `json:"buckets"`
}

// factDistributionFromCounts wraps a per-key count map in the rich
// display shape the overview tab consumes. Used at capture time when
// the per-category counts are already in scope.
func factDistributionFromCounts(category string, counts map[string]int) PublicAnalysisFactDistribution {
	display, known := factCategoryDisplays[category]
	buckets := make([]PublicAnalysisFactBucket, 0, len(counts))
	for key, count := range counts {
		label := key
		tone := "neutral"
		order := 1 << 30
		if known {
			if display.KeyLabel != nil {
				label = display.KeyLabel(key)
			}
			if display.KeyTone != nil {
				tone = display.KeyTone(key)
			}
			if display.KeyOrder != nil {
				order = display.KeyOrder(key)
			}
		}
		buckets = append(buckets, PublicAnalysisFactBucket{
			Key:   key,
			Label: label,
			Tone:  tone,
			Count: count,
			Order: order,
		})
	}
	sort.Slice(buckets, func(i, j int) bool {
		if buckets[i].Order != buckets[j].Order {
			return buckets[i].Order < buckets[j].Order
		}
		return buckets[i].Key < buckets[j].Key
	})
	out := PublicAnalysisFactDistribution{Category: category, Buckets: buckets}
	if known {
		out.Label = display.Label
		out.Description = display.Description
		out.Order = display.Order
	} else {
		out.Label = category
		out.Order = 1 << 30
	}
	return out
}

// buildFactDistributions aggregates the cached domain-fact rows into
// per-category bar data suitable for the public overview. Counts are
// distinct domains per key - a domain can appear in multiple buckets
// within a category (e.g. a zone publishing two DNSKEY algorithms).
func buildFactDistributions(facts []AnalysisRunDomainFact) map[string]PublicAnalysisFactDistribution {
	if len(facts) == 0 {
		return nil
	}
	type bucketKey struct {
		category, key string
	}
	domainsByBucket := map[bucketKey]map[int64]struct{}{}
	for _, f := range facts {
		k := bucketKey{category: f.Category, key: f.Key}
		set, ok := domainsByBucket[k]
		if !ok {
			set = map[int64]struct{}{}
			domainsByBucket[k] = set
		}
		set[f.DomainID] = struct{}{}
	}
	byCategory := map[string][]PublicAnalysisFactBucket{}
	for k, domains := range domainsByBucket {
		display, known := factCategoryDisplays[k.category]
		label := k.key
		tone := "neutral"
		order := 1 << 30
		if known {
			if display.KeyLabel != nil {
				label = display.KeyLabel(k.key)
			}
			if display.KeyTone != nil {
				tone = display.KeyTone(k.key)
			}
			if display.KeyOrder != nil {
				order = display.KeyOrder(k.key)
			}
		}
		byCategory[k.category] = append(byCategory[k.category], PublicAnalysisFactBucket{
			Key:   k.key,
			Label: label,
			Tone:  tone,
			Count: len(domains),
			Order: order,
		})
	}
	out := make(map[string]PublicAnalysisFactDistribution, len(byCategory))
	for category, buckets := range byCategory {
		sort.Slice(buckets, func(i, j int) bool {
			if buckets[i].Order != buckets[j].Order {
				return buckets[i].Order < buckets[j].Order
			}
			return buckets[i].Key < buckets[j].Key
		})
		display, known := factCategoryDisplays[category]
		entry := PublicAnalysisFactDistribution{
			Category: category,
			Buckets:  buckets,
		}
		if known {
			entry.Label = display.Label
			entry.Description = display.Description
			entry.Order = display.Order
		} else {
			entry.Label = category
			entry.Order = 1 << 30
		}
		out[category] = entry
	}
	return out
}
