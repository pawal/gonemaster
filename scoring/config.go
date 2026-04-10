// Package scoring computes a numeric quality score and letter grade for a
// completed DNS test run. It is a standalone package with no dependency on the
// server or engine packages and can be used by the server, CLI, and Nagios
// plugin alike.
package scoring

// Config holds all tunable parameters for the scoring engine.
// Call DefaultConfig to obtain a ready-to-use configuration.
type Config struct {
	// SeverityPenalties maps a severity level name to the point penalty
	// applied for each entry at that level.
	SeverityPenalties map[string]int

	// CategoryWeights maps a category name to its relative weight when
	// computing the aggregate score from per-category sub-scores.
	CategoryWeights map[string]float64

	// ModuleCategories maps an engine module name (e.g. "DNSSEC") to a
	// scoring category name (e.g. "dnssec").
	ModuleCategories map[string]string

	// GradeBands defines the letter grade thresholds in descending order.
	// The first band whose MinScore is ≤ the numeric score is used.
	GradeBands []GradeBand

	// BonusCriteria controls which A+ bonus checks are evaluated.
	BonusCriteria BonusCriteriaConfig
}

// GradeBand maps a minimum numeric score to a letter grade.
type GradeBand struct {
	Grade    string
	MinScore int
}

// BonusCriteriaConfig selects which bonus checks are evaluated.
// Disabled checks are omitted from the BonusResult entirely.
type BonusCriteriaConfig struct {
	NoWarningsOrErrors   bool
	DNSSECEnabled        bool
	StrongAlgorithm      bool
	NSEC3NonOptout       bool
	CDSCDNSKEYPublished  bool
	IPv6AllNameservers   bool
	ASDiversity          bool
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
