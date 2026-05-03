package scoring

import "strings"

// Entry is the minimal representation of a test log entry required for
// scoring. The server maps its own Entry type to this before calling Compute.
type Entry struct {
	Module string
	Tag    string
	Level  string
}

// Result holds the complete scoring output for a run.
type Result struct {
	// Score is the aggregate numeric score in the range [0, 100].
	Score int `json:"score"`

	// Grade is the letter grade corresponding to Score ("A+", "A" … "F").
	Grade string `json:"grade"`

	// Categories contains per-category sub-scores and penalty details.
	Categories map[string]CategoryResult `json:"categories"`

	// Bonus contains the A+ eligibility result and per-criterion outcomes.
	Bonus BonusResult `json:"bonus"`

	// DisabledStacks lists IP stacks that were disabled in the test profile
	// (e.g. "ipv4", "ipv6"). When non-empty the score is partial: tests for
	// the disabled stack were not run, so penalties from those tests are
	// absent and the score may be higher than a full run would produce.
	DisabledStacks []string `json:"disabled_stacks,omitempty"`
}

// CategoryResult holds the scoring breakdown for a single category.
type CategoryResult struct {
	// Score is the category sub-score in [0, 100].
	Score int `json:"score"`

	// Penalties is the total unweighted point penalty accumulated for this category.
	Penalties int `json:"penalties"`

	// EntryCount is the number of entries that contributed a penalty.
	EntryCount int `json:"entry_count"`

	// Tested is true when the category received at least one entry from any
	// module mapped to it, regardless of severity level. When false, the
	// category's tests did not run (typically because a critical failure
	// aborted the test run early) and the score of 0 reflects "not tested"
	// rather than "tested and broken".
	Tested bool `json:"tested"`
}

// BonusResult holds the A+ evaluation.
type BonusResult struct {
	// Eligible is true when the grade is A+ (score 100 and all applicable
	// bonus criteria are met).
	Eligible bool `json:"eligible"`

	// Criteria maps each bonus criterion name to its outcome:
	//   true  – criterion met
	//   false – criterion not met (blocks A+)
	//   nil   – not applicable or could not be determined from the entries
	// Criteria that are not applicable count as satisfied and do not block A+.
	Criteria map[string]*bool `json:"criteria"`
}

// Compute scores a completed run. domain is the zone name (e.g. "example.se"
// or "se") and is used for context-aware bonus checks. entries are the log
// entries produced by the test run. cfg should normally come from
// DefaultConfig, optionally with operator overrides applied.
func Compute(domain string, entries []Entry, cfg Config) Result {
	// Build per-category penalty sums and entry counts.
	type catAccum struct {
		penalties    int
		entryCount   int
		totalEntries int  // all entries regardless of level (tracks whether category was tested)
		hasCritical  bool // true if any CRITICAL entry landed in this category
	}
	cats := make(map[string]*catAccum)
	for cat := range cfg.CategoryWeights {
		cats[cat] = &catAccum{}
	}

	hasCritical := false
	ipv4Disabled := false
	ipv6Disabled := false

	for _, e := range entries {
		switch strings.ToUpper(e.Tag) {
		case "IPV4_DISABLED", "CN01_IPV4_DISABLED":
			ipv4Disabled = true
		case "IPV6_DISABLED", "CN01_IPV6_DISABLED":
			ipv6Disabled = true
		}
		isCritical := strings.ToUpper(e.Level) == "CRITICAL"
		if isCritical {
			hasCritical = true
		}
		cat, ok := cfg.ModuleCategories[strings.ToUpper(e.Module)]
		if !ok {
			continue
		}
		accum := cats[cat]
		accum.totalEntries++
		if isCritical {
			accum.hasCritical = true
		}
		// Tag-specific overrides take precedence over the severity table.
		penalty, ok := cfg.TagPenalties[strings.ToUpper(e.Tag)]
		if !ok {
			penalty, ok = cfg.SeverityPenalties[strings.ToUpper(e.Level)]
		}
		if !ok || penalty == 0 {
			continue
		}
		accum.penalties += penalty
		accum.entryCount++
	}

	// Compute per-category sub-scores and the weighted aggregate.
	categories := make(map[string]CategoryResult, len(cfg.CategoryWeights))
	var weightedSum float64
	var totalWeight float64

	for cat, weight := range cfg.CategoryWeights {
		accum := cats[cat]
		tested := accum.totalEntries > 0

		subScore := 100 - accum.penalties
		if subScore < 0 {
			subScore = 0
		}

		// When any CRITICAL entry is present in the run, the domain is
		// fundamentally broken (non-existent, no delegation, etc.). All
		// category scores are set to 0: untested categories were never
		// evaluated, and tested categories produced results that are
		// meaningless in the context of a non-functional domain.
		if hasCritical {
			subScore = 0
		}

		categories[cat] = CategoryResult{
			Score:      subScore,
			Penalties:  accum.penalties,
			EntryCount: accum.entryCount,
			Tested:     tested,
		}
		weightedSum += float64(subScore) * weight
		totalWeight += weight
	}

	var aggregate int
	if totalWeight > 0 {
		aggregate = int(weightedSum / totalWeight)
	}
	if aggregate > 100 {
		aggregate = 100
	}
	if aggregate < 0 {
		aggregate = 0
	}

	// CRITICAL override: any CRITICAL entry forces an F.
	if hasCritical && aggregate > 10 {
		aggregate = 10
	}

	grade := scoreToGrade(aggregate, cfg.GradeBands)

	bonus := evaluateBonus(domain, entries, aggregate, cfg.BonusCriteria)
	if bonus.Eligible {
		grade = "A+"
	}

	var disabledStacks []string
	if ipv4Disabled {
		disabledStacks = append(disabledStacks, "ipv4")
	}
	if ipv6Disabled {
		disabledStacks = append(disabledStacks, "ipv6")
	}

	return Result{
		Score:          aggregate,
		Grade:          grade,
		Categories:     categories,
		Bonus:          bonus,
		DisabledStacks: disabledStacks,
	}
}

// scoreToGrade maps a numeric score to a letter grade using the configured
// bands. The bands must be ordered from highest to lowest MinScore.
// Returns "F" if no band matches (should not happen with a well-formed config).
func scoreToGrade(score int, bands []GradeBand) string {
	for _, band := range bands {
		if score >= band.MinScore {
			return band.Grade
		}
	}
	return "F"
}

// isTLDZone returns true when domain is a TLD - i.e. it has no dots after
// stripping a trailing dot (e.g. "se", "com", "se.").
func isTLDZone(domain string) bool {
	d := strings.TrimSuffix(domain, ".")
	return !strings.Contains(d, ".")
}
