// Package scoring computes a numeric quality score and letter grade for a
// completed DNS test run. It is a standalone package with no dependency on the
// server or engine packages and can be used by the server, CLI, and Nagios
// plugin alike.
package scoring

import (
	"encoding/json"
	"fmt"
	"os"
)

// Config holds all tunable parameters for the scoring engine.
// Call DefaultConfig to obtain a ready-to-use configuration.
type Config struct {
	// SeverityPenalties maps a severity level name to the point penalty
	// applied for each entry at that level.
	SeverityPenalties map[string]int `json:"severity_penalties"`

	// CategoryWeights maps a category name to its relative weight when
	// computing the aggregate score from per-category sub-scores.
	CategoryWeights map[string]float64 `json:"category_weights"`

	// ModuleCategories maps an engine module name (e.g. "DNSSEC") to a
	// scoring category name (e.g. "dnssec").
	ModuleCategories map[string]string `json:"module_categories"`

	// TagPenalties maps a specific tag name to a point penalty that overrides
	// the SeverityPenalties lookup for that tag. Use this to assign penalties
	// that are disproportionate to the tag's log level — for example to treat
	// a WARNING-level tag as more serious than other warnings.
	// Keys are matched case-insensitively.
	TagPenalties map[string]int `json:"tag_penalties,omitempty"`

	// GradeBands defines the letter grade thresholds in descending order.
	// The first band whose MinScore is ≤ the numeric score is used.
	GradeBands []GradeBand `json:"grade_bands"`

	// BonusCriteria controls which A+ bonus checks are evaluated.
	BonusCriteria BonusCriteriaConfig `json:"bonus_criteria"`
}

// GradeBand maps a minimum numeric score to a letter grade.
type GradeBand struct {
	Grade    string `json:"grade"`
	MinScore int    `json:"min_score"`
}

// BonusCriteriaConfig selects which bonus checks are evaluated.
// Disabled checks are omitted from the BonusResult entirely.
type BonusCriteriaConfig struct {
	NoWarningsOrErrors  bool `json:"no_warnings_or_errors"`
	DNSSECEnabled       bool `json:"dnssec_enabled"`
	StrongAlgorithm     bool `json:"strong_algorithm"`
	NSEC3NonOptout      bool `json:"nsec3_non_optout"`
	CDSCDNSKEYPublished bool `json:"cds_cdnskey_published"`
	IPv6AllNameservers  bool `json:"ipv6_all_nameservers"`
	ASDiversity         bool `json:"as_diversity"`
}

// LoadConfig reads a JSON scoring config file and returns the parsed Config.
// Fields absent from the file retain the values from DefaultConfig.
// Note: map fields (SeverityPenalties, CategoryWeights, ModuleCategories) are
// replaced entirely when present in the file, not merged with defaults.
func LoadConfig(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("reading scoring config %q: %w", path, err)
	}
	cfg := DefaultConfig()
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parsing scoring config %q: %w", path, err)
	}
	return cfg, nil
}

// DefaultConfig returns the recommended configuration suitable for public DNS
// infrastructure. Operators can adjust weights, penalties, or disable bonus
// criteria for private or split-horizon deployments.
func DefaultConfig() Config {
	return Config{
		SeverityPenalties: map[string]int{
			"NOTICE":   1,
			"WARNING":  5,
			"ERROR":    20,
			"CRITICAL": 0, // CRITICAL triggers an automatic F override, not a numeric penalty
		},
		CategoryWeights: map[string]float64{
			"dnssec":           1.5,
			"nameserver_health": 1.2,
			"connectivity":     1.0,
			"zone_consistency": 0.8,
		},
		ModuleCategories: map[string]string{
			"DNSSEC":      "dnssec",
			"NAMESERVER":  "nameserver_health",
			"BASIC":       "nameserver_health",
			"DELEGATION":  "nameserver_health",
			"CONNECTIVITY": "connectivity",
			"ADDRESS":     "connectivity",
			"CONSISTENCY": "zone_consistency",
			"ZONE":        "zone_consistency",
			"SYNTAX":      "zone_consistency",
		},
		TagPenalties: map[string]int{
			// DS07_NOT_SIGNED: zone has no DNSKEY records on any nameserver —
			// entirely unsigned. Treated like an ERROR regardless of its WARNING level.
			"DS07_NOT_SIGNED": 20,
			// DS07_NO_DS_FOR_SIGNED_ZONE: zone is signed but has no DS record at
			// the parent — breaks the chain of trust. Same severity as above.
			"DS07_NO_DS_FOR_SIGNED_ZONE": 20,
			// NO_IPV6_NS_CHILD / NO_IPV6_NS_DEL: zero nameservers have IPv6
			// addresses — the zone is entirely unreachable over IPv6. Both are
			// NOTICE (1 pt) by default but warrant the same weight as an ERROR.
			"NO_IPV6_NS_CHILD": 20,
			"NO_IPV6_NS_DEL":   20,
		},
		GradeBands: []GradeBand{
			{Grade: "A", MinScore: 90},
			{Grade: "B", MinScore: 75},
			{Grade: "C", MinScore: 60},
			{Grade: "D", MinScore: 40},
			{Grade: "F", MinScore: 0},
		},
		BonusCriteria: BonusCriteriaConfig{
			NoWarningsOrErrors:  true,
			DNSSECEnabled:       true,
			StrongAlgorithm:     true,
			NSEC3NonOptout:      true,
			CDSCDNSKEYPublished: true,
			IPv6AllNameservers:  true,
			ASDiversity:         true,
		},
	}
}
