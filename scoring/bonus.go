package scoring

import "strings"

// evaluateBonus checks A+ bonus criteria against the run's entries.
// score is the computed aggregate; A+ is only possible at score == 100.
func evaluateBonus(domain string, entries []Entry, score int, cfg BonusCriteriaConfig) BonusResult {
	// Build a set of all tags present in the run.
	tags := make(map[string]bool, len(entries))
	for _, e := range entries {
		tags[strings.ToUpper(e.Tag)] = true
	}

	criteria := make(map[string]*bool)

	if cfg.NoWarningsOrErrors {
		criteria["no_warnings_or_errors"] = boolPtr(noWarningsOrErrors(entries))
	}
	if cfg.DNSSECEnabled {
		criteria["dnssec_enabled"] = dnssecEnabled(tags)
	}
	if cfg.StrongAlgorithm {
		criteria["strong_algorithm"] = strongAlgorithm(tags)
	}
	if cfg.NSEC3NonOptout {
		criteria["nsec3_non_optout"] = nsec3NonOptout(tags)
	}
	if cfg.CDSCDNSKEYPublished {
		criteria["cds_cdnskey_published"] = cdsCDNSKEYPublished(domain, tags)
	}
	if cfg.IPv6AllNameservers {
		criteria["ipv6_all_nameservers"] = ipv6AllNameservers(tags)
	}
	if cfg.ASDiversity {
		criteria["as_diversity"] = asDiversity(tags)
	}

	// Eligible only at a perfect score with all applicable criteria met.
	eligible := score == 100 && allCriteriaMet(criteria)

	return BonusResult{
		Eligible: eligible,
		Criteria: criteria,
	}
}

// allCriteriaMet returns true when every criterion is either explicitly true
// or nil (not applicable). A nil value means the criterion does not apply to
// this domain (e.g. CDS/CDNSKEY for TLDs) and is treated as satisfied.
// A false value means either the criterion was checked and failed, or the
// relevant test was not run so the criterion cannot be confirmed.
func allCriteriaMet(criteria map[string]*bool) bool {
	for _, v := range criteria {
		if v == nil {
			// Not applicable - treated as satisfied.
			continue
		}
		if !*v {
			return false
		}
	}
	return true
}

// noWarningsOrErrors returns true when no entry is at WARNING or above.
func noWarningsOrErrors(entries []Entry) bool {
	for _, e := range entries {
		switch strings.ToUpper(e.Level) {
		case "WARNING", "ERROR", "CRITICAL":
			return false
		}
	}
	return true
}

// dnssecEnabled returns:
//   - true  when DS07_SIGNED is present (zone has a signed DS)
//   - false when DS07_NOT_SIGNED is present, or when the test was not run
//     (cannot confirm → treated as not met for A+ purposes)
func dnssecEnabled(tags map[string]bool) *bool {
	if tags["DS07_SIGNED"] {
		return boolPtr(true)
	}
	return boolPtr(false)
}

// strongAlgorithm returns:
//   - false when any deprecated or not-recommended algorithm tag is present,
//     or when DS05 was not run (cannot confirm → not met for A+)
//   - true  when DS05_ALGO_OK is present and no weak-algorithm tags exist
func strongAlgorithm(tags map[string]bool) *bool {
	weakTags := []string{
		"DS05_ALGO_DEPRECATED",
		"DS05_ALGO_NOT_RECOMMENDED",
		"DS01_DS_ALGO_DEPRECATED",
	}
	for _, t := range weakTags {
		if tags[t] {
			return boolPtr(false)
		}
	}
	if tags["DS05_ALGO_OK"] || tags["DS01_DS_ALGO_OK"] {
		return boolPtr(true)
	}
	return boolPtr(false)
}

// nsec3NonOptout returns:
//   - nil   when DS03_NSEC3_OPT_OUT_ENABLED_TLD is present (TLD using
//     opt-out is expected behaviour; criterion is not applicable)
//   - false when DS03_NSEC3_OPT_OUT_ENABLED_NON_TLD is present, or when
//     DS03 was not run (cannot confirm → not met for A+)
//   - true  when DS03_NSEC3_OPT_OUT_DISABLED or DS03_NO_NSEC3 is present
func nsec3NonOptout(tags map[string]bool) *bool {
	if tags["DS03_NSEC3_OPT_OUT_ENABLED_TLD"] {
		return nil // not applicable for TLDs
	}
	if tags["DS03_NSEC3_OPT_OUT_ENABLED_NON_TLD"] {
		return boolPtr(false)
	}
	if tags["DS03_NSEC3_OPT_OUT_DISABLED"] || tags["DS03_NO_NSEC3"] {
		return boolPtr(true)
	}
	return boolPtr(false)
}

// cdsCDNSKEYPublished returns:
//   - nil   for TLD zones (parent does not consume CDS/CDNSKEY; not applicable)
//   - true  when any DS15 "HAS_CDS" or "HAS_CDNSKEY" tag is present
//   - nil   when DS18_NO_CDS_CDNSKEY_BUT_ROLLOVER_EVIDENCE is present (on-demand
//     publication model; absence is intentional, should not block A+)
//   - false when DS15_NO_CDS_CDNSKEY is present, or when DS15 was not run
//     (cannot confirm → not met for A+)
func cdsCDNSKEYPublished(domain string, tags map[string]bool) *bool {
	if isTLDZone(domain) {
		return nil
	}
	positives := []string{
		"DS15_HAS_CDS_AND_CDNSKEY",
		"DS15_HAS_CDS_NO_CDNSKEY",
		"DS15_HAS_CDNSKEY_NO_CDS",
	}
	for _, t := range positives {
		if tags[t] {
			return boolPtr(true)
		}
	}
	// On-demand CDS/CDNSKEY publication (e.g. Knot DNS mid-rollover): the operator
	// withdrew CDS/CDNSKEY after the parent updated its DS, but other rollover
	// signals confirm the zone is correctly managed.  Treat as not-applicable.
	if tags["DS18_NO_CDS_CDNSKEY_BUT_ROLLOVER_EVIDENCE"] {
		return nil
	}
	return boolPtr(false)
}

// ipv6AllNameservers returns:
//   - false when IPV6_DISABLED or CN01_IPV6_DISABLED is present (IPv6 disabled
//     in the profile for some or all nameservers)
//   - true  when IPv6 ASN tags are present (implies NSes have IPv6 addresses
//     and ASN lookups succeeded) and no IPv6-disabled tags are present
//   - nil   when the available tags are insufficient to determine the outcome;
//     since the engine does not emit per-nameserver IPv6 success tags,
//     nil means "could not confirm" and is treated as not applicable
//     rather than as a hard failure - operators may disable this
//     criterion in environments where IPv6 is not available
func ipv6AllNameservers(tags map[string]bool) *bool {
	if tags["IPV6_DISABLED"] || tags["CN01_IPV6_DISABLED"] {
		return boolPtr(false)
	}
	// Presence of an IPv6 ASN tag implies at least some NSes were reached over
	// IPv6. This is the best available indicator; a more precise check would
	// require per-nameserver IPv6 success tags which the engine does not emit.
	hasIPv6ASN := tags["IPV6_ONE_ASN"] || tags["IPV6_SAME_ASN"] || tags["IPV6_DIFFERENT_ASN"]
	if hasIPv6ASN {
		return boolPtr(true)
	}
	// Cannot determine: return nil so that domains tested without connectivity03
	// are not penalised, while still failing when IPv6 is explicitly disabled.
	return nil
}

// asDiversity returns:
//   - true  when IPV4_DIFFERENT_ASN or IPV6_DIFFERENT_ASN is present
//   - false when only ONE_ASN or SAME_ASN tags are present (non-diverse), or
//     when AS lookup was not run (cannot confirm → not met for A+)
func asDiversity(tags map[string]bool) *bool {
	if tags["IPV4_DIFFERENT_ASN"] || tags["IPV6_DIFFERENT_ASN"] {
		return boolPtr(true)
	}
	hasIPv4Single := tags["IPV4_ONE_ASN"] || tags["IPV4_SAME_ASN"]
	hasIPv6Single := tags["IPV6_ONE_ASN"] || tags["IPV6_SAME_ASN"]
	if hasIPv4Single || hasIPv6Single {
		return boolPtr(false)
	}
	return boolPtr(false)
}

func boolPtr(b bool) *bool { return &b }
