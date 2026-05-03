package analysis

import (
	"sort"
	"strconv"
	"strings"

	serverpkg "codeberg.org/pawal/gonemaster/server"
)

// Category / key tokens are declared in the server package so extractor
// code here and the display registry on the server side share the wire
// token without a redeclaration drift risk.
const (
	factCategorySeverity        = serverpkg.FactCategorySeverity
	factCategoryDNSSECPosture   = serverpkg.FactCategoryDNSSECPosture
	factCategoryGrade           = serverpkg.FactCategoryGrade
	factCategoryDNSKEYAlgorithm = serverpkg.FactCategoryDNSKEYAlgorithm

	factKeySigned    = serverpkg.FactKeySigned
	factKeyUnsigned  = serverpkg.FactKeyUnsigned
	factKeyNSEC      = serverpkg.FactKeyNSEC
	factKeyNSEC3     = serverpkg.FactKeyNSEC3
	factKeyNSECMixed = serverpkg.FactKeyNSECMixed
)

// extractedDomainFact is one (category, key) fact to materialize for this
// run's domain. Extractors append to a slice and the projector serializes
// them into AnalysisRunDomainFact rows per cohort.
type extractedDomainFact struct {
	category string
	key      string
	valueNum *int64
}

// extractDomainFacts is the dispatcher for all domain-fact extractors. Each
// extractor returns its own slice; the dispatcher concatenates, dedupes on
// (category, key), and keeps the first value_num seen per key.
func extractDomainFacts(input RunInput) []extractedDomainFact {
	var out []extractedDomainFact
	out = append(out, extractSeverity(input)...)
	out = append(out, extractDNSSECPosture(input)...)
	out = append(out, extractDNSKEYAlgorithms(input)...)
	out = append(out, extractGrade(input)...)
	return dedupeDomainFacts(out)
}

// extractSeverity emits exactly one fact per run carrying the worst
// finding level the engine recorded. Empty worst_level collapses to
// "OK" so the bar covers the full cohort membership.
func extractSeverity(input RunInput) []extractedDomainFact {
	level := strings.ToUpper(strings.TrimSpace(input.Run.WorstLevel))
	if level == "" {
		level = "OK"
	}
	return []extractedDomainFact{{category: factCategorySeverity, key: level}}
}

// extractGrade emits one fact row per run carrying the letter grade the
// scoring engine assigned at graduation time. The grade set is not
// hard-coded because scoring is configurable - a cohort scored under a
// custom profile may produce labels outside the default A+/A/B/C/D/F
// set. The registry handles display; this extractor just records
// whatever label the run carries.
func extractGrade(input RunInput) []extractedDomainFact {
	if input.Run.Grade == nil {
		return nil
	}
	grade := strings.TrimSpace(*input.Run.Grade)
	if grade == "" {
		return nil
	}
	return []extractedDomainFact{{category: factCategoryGrade, key: grade}}
}

func dedupeDomainFacts(in []extractedDomainFact) []extractedDomainFact {
	type key struct {
		category, fact string
	}
	seen := map[key]int{}
	out := make([]extractedDomainFact, 0, len(in))
	for _, f := range in {
		k := key{f.category, f.key}
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = len(out)
		out = append(out, f)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].category != out[j].category {
			return out[i].category < out[j].category
		}
		return out[i].key < out[j].key
	})
	return out
}

// extractDNSKEYAlgorithms emits one fact per distinct DNSKEY algorithm the
// zone publishes. DNSSEC05 logs one entry per (keytag, algo) combination with
// tags DS05_ALGO_OK / DS05_ALGO_DEPRECATED / ... and the algo_num arg; we
// dedupe to the set of algorithms observed for this domain in this run.
// value_num carries the count of distinct keytags seen with that algorithm.
func extractDNSKEYAlgorithms(input RunInput) []extractedDomainFact {
	keytagsPerAlgo := map[int64]map[int64]struct{}{}
	for _, entry := range input.Entries {
		if !strings.HasPrefix(entry.Tag, "DS05_ALGO_") {
			continue
		}
		algo, ok := numericArg(entry.Args, "algo_num")
		if !ok {
			continue
		}
		keytags, exists := keytagsPerAlgo[algo]
		if !exists {
			keytags = map[int64]struct{}{}
			keytagsPerAlgo[algo] = keytags
		}
		if kt, ok := numericArg(entry.Args, "keytag"); ok {
			keytags[kt] = struct{}{}
		}
	}
	if len(keytagsPerAlgo) == 0 {
		return nil
	}
	out := make([]extractedDomainFact, 0, len(keytagsPerAlgo))
	for algo, keytags := range keytagsPerAlgo {
		count := int64(len(keytags))
		fact := extractedDomainFact{
			category: factCategoryDNSKEYAlgorithm,
			key:      strconv.FormatInt(algo, 10),
		}
		if count > 0 {
			fact.valueNum = &count
		}
		out = append(out, fact)
	}
	return out
}

// extractDNSSECPosture emits exactly one fact per run partitioning the
// cohort by DNSSEC state and denial-of-existence mode:
//
//   - "unsigned" - DS07_NOT_SIGNED present (short-circuits the rest of
//     the DNSSEC suite, so the verdict is authoritative).
//   - "mixed" - DS10_MIXED_NSEC_NSEC3, or both DS10_HAS_NSEC and
//     DS10_HAS_NSEC3 from different servers in the same run.
//   - "nsec" - only DS10_HAS_NSEC seen.
//   - "nsec3" - only DS10_HAS_NSEC3 seen.
//   - "signed" - DNSSEC ran but DS10 did not produce a denial-of-existence
//     signal (degraded run). Kept as a fallback so the bar still counts
//     the domain.
//
// When no DNSSEC signal was produced at all (engine never ran DNSSEC, or
// skipped on earlier failure) we emit nothing rather than claim a posture.
func extractDNSSECPosture(input RunInput) []extractedDomainFact {
	sawDNSSEC := false
	hasNSEC := false
	hasNSEC3 := false
	mixed := false
	for _, entry := range input.Entries {
		if entry.Tag == "DS07_NOT_SIGNED" {
			return []extractedDomainFact{{category: factCategoryDNSSECPosture, key: factKeyUnsigned}}
		}
		if strings.EqualFold(entry.Module, "DNSSEC") {
			sawDNSSEC = true
		}
		switch entry.Tag {
		case "DS10_HAS_NSEC":
			hasNSEC = true
		case "DS10_HAS_NSEC3":
			hasNSEC3 = true
		case "DS10_MIXED_NSEC_NSEC3":
			mixed = true
		}
	}
	if !sawDNSSEC {
		return nil
	}
	switch {
	case mixed, hasNSEC && hasNSEC3:
		return []extractedDomainFact{{category: factCategoryDNSSECPosture, key: factKeyNSECMixed}}
	case hasNSEC:
		return []extractedDomainFact{{category: factCategoryDNSSECPosture, key: factKeyNSEC}}
	case hasNSEC3:
		return []extractedDomainFact{{category: factCategoryDNSSECPosture, key: factKeyNSEC3}}
	}
	return []extractedDomainFact{{category: factCategoryDNSSECPosture, key: factKeySigned}}
}

// buildDomainFactRows materializes the extracted facts into store rows for
// one cohort. Factored so writePrepared stays readable and tests can cover
// the shape conversion independently.
func buildDomainFactRows(cohortID int64, runID string, domainID int64, facts []extractedDomainFact) []serverpkg.AnalysisRunDomainFact {
	rows := make([]serverpkg.AnalysisRunDomainFact, 0, len(facts))
	for _, f := range facts {
		rows = append(rows, serverpkg.AnalysisRunDomainFact{
			CohortID: cohortID,
			RunID:    runID,
			DomainID: domainID,
			Category: f.category,
			Key:      f.key,
			ValueNum: f.valueNum,
		})
	}
	return rows
}

// numericArg returns the numeric value at key, handling the int / int64 /
// float64 forms JSON decoding may produce, as well as the uint-widths the
// engine uses natively when tests construct entries in-process. Mirrors
// numericToInt64 but returns the second result inline for ergonomic use.
func numericArg(args map[string]any, key string) (int64, bool) {
	if args == nil {
		return 0, false
	}
	raw, ok := args[key]
	if !ok {
		return 0, false
	}
	switch v := raw.(type) {
	case int:
		return int64(v), true
	case int8:
		return int64(v), true
	case int16:
		return int64(v), true
	case int32:
		return int64(v), true
	case int64:
		return v, true
	case uint:
		return int64(v), true
	case uint8:
		return int64(v), true
	case uint16:
		return int64(v), true
	case uint32:
		return int64(v), true
	case uint64:
		return int64(v), true
	case float32:
		return int64(v), true
	case float64:
		return int64(v), true
	case string:
		if n, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64); err == nil {
			return n, true
		}
	}
	return 0, false
}
