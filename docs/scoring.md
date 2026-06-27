# Scoring

Gonemaster assigns each completed test run a numeric quality score (0–100)
and a letter grade (A+ through F). The scoring engine evaluates the same log
entries produced by every test run, and no extra DNS queries are needed. Scoring
is a pure server-side computation that works identically in the server, CLI,
and Nagios plugin.

The model is inspired by SSL Labs grading for TLS: a domain with zero issues,
full DNSSEC with strong algorithms, IPv6 on all nameservers, and nameserver
diversity earns an A+. Failures cascade the grade downward by severity and
category.

## How scoring works

Each run starts at **100 points**. Every log entry at NOTICE level or above
incurs a penalty determined by two factors:

1. **Entry penalty** - looked up first by tag name (`tag_penalties`), then by
   severity level (`severity_penalties`). A tag penalty of 0 suppresses the
   entry entirely.
2. **Category weight** - a multiplier based on which scoring category the entry
   belongs to.

The aggregate score is the weighted average of per-category sub-scores:

```
category_score = max(0, 100 - sum_of_penalties_in_category)
aggregate      = sum(category_score * weight) / sum(weight)
```

### Severity penalties

| Level    | Default penalty | Notes                                      |
|----------|-----------------|--------------------------------------------|
| INFO     | 0               | No penalty                                 |
| NOTICE   | 1               | Cosmetic - accumulates slowly              |
| WARNING  | 5               | Real concern - a few warnings hurt         |
| ERROR    | 20              | Serious - one error drops a full grade band|
| CRITICAL | 0 (special)     | Triggers automatic F override (see below)  |

### CRITICAL override

Any run containing one or more CRITICAL entries is capped at **score ≤ 10**
(grade F), regardless of the computed numeric score. A non-functional domain
cannot earn a passing grade. The per-category breakdown is still computed
normally so operators can see where other issues are. The cap of 10 (rather
than 0) preserves a small numeric gap between critical failure and a domain
that simply scores poorly.

### Tag-level penalty overrides

Some tags warrant a penalty disproportionate to their log level. The
`tag_penalties` map takes precedence over `severity_penalties`:

| Tag                          | Level   | Override | Rationale                          |
|------------------------------|---------|----------|------------------------------------|
| `DS07_NOT_SIGNED`            | WARNING | **20**   | Zone entirely unsigned             |
| `DS07_NO_DS_FOR_SIGNED_ZONE` | WARNING | **20**   | Chain of trust broken              |
| `NO_IPV6_NS_CHILD`          | NOTICE  | **20**   | Zone unreachable over IPv6         |
| `NO_IPV6_NS_DEL`            | NOTICE  | **20**   | Delegation has no IPv6 addresses   |
| `N15_SOFTWARE_VERSION`       | NOTICE  | **0**    | Cosmetic - zone works correctly    |
| `N16_HAS_NSID`               | NOTICE  | **0**    | Informational - explicit NSID reply|
| `N18_NO_EXTENDED_ERROR`      | INFO    | **0**    | Diagnostic - EDE enrichment (RFC 8914)|
| `N18_EXTENDED_ERROR_REPORTED`| NOTICE  | **0**    | Diagnostic - EDE enrichment (RFC 8914)|
| `N18_SERVER_ERROR_REPORTED`  | WARNING | **0**    | Diagnostic - explained by other tests |
| `N18_RESOLVER_BEHAVIOR_REPORTED`| WARNING | **0**  | Diagnostic - gates A+ only             |
| `N18_FILTERED_RESPONSE`      | WARNING | **0**    | Diagnostic - gates A+ only             |
| `N18_NO_RESPONSE`            | WARNING | **0**    | Diagnostic - reachability via other tests|

Setting a tag penalty to 0 suppresses it entirely.

## Scoring categories

Each log entry's module maps to one of four scoring categories:

| Category             | Modules                            | Default weight |
|----------------------|------------------------------------|----------------|
| **DNSSEC**           | DNSSEC                             | 1.5            |
| **Nameserver Health**| NAMESERVER, BASIC, DELEGATION      | 1.2            |
| **Connectivity**     | CONNECTIVITY, ADDRESS              | 1.0            |
| **Zone Consistency** | CONSISTENCY, ZONE, SYNTAX          | 0.8            |

DNSSEC is weighted highest because cryptographic misconfigurations represent
the most serious operational risk. Zone consistency is weighted lowest because
minor timer or syntax issues rarely affect resolution.

Entries from unknown modules are silently ignored.

## Grade bands

| Grade | Numeric range | Meaning                          |
|-------|---------------|----------------------------------|
| A+    | 100 + bonus   | Perfect, with best practices     |
| A     | 90–100        | Excellent, minor notices at most |
| B     | 75–89         | Good, some warnings              |
| C     | 60–74         | Fair, significant issues         |
| D     | 40–59         | Poor, multiple errors            |
| F     | 0–39          | Failing, critical problems       |

## A+ bonus criteria

A score of 100 earns an **A** by default. To reach **A+**, the domain must
also satisfy all applicable bonus criteria, evaluated from existing entry tags:

| Criterion                  | Condition                                               |
|----------------------------|---------------------------------------------------------|
| No warnings or errors      | Only NOTICE or below                                    |
| DNSSEC enabled             | At least one validated DNSKEY present                   |
| Strong algorithm           | All DNSKEY algorithms are ECDSA or EdDSA (13/14/15/16)  |
| NSEC3 non-opt-out          | If NSEC3 is used, opt-out flag is not set               |
| CDS/CDNSKEY published      | Domain publishes CDS and/or CDNSKEY records             |
| IPv6 all nameservers       | Every NS has at least one AAAA and responds             |
| AS diversity               | Nameservers span at least 2 autonomous systems          |

### Context-aware evaluation

Criteria that are **not applicable** to a domain are treated as **satisfied**:

- **CDS/CDNSKEY** is not evaluated for TLDs (whose parent typically does not
  consume CDS/CDNSKEY). The criterion is marked `null` (not applicable).
- **NSEC3 non-opt-out** returns `null` for TLDs where opt-out is standard.

Criteria whose relevant test was **skipped by profile** are treated as
**not met** - the operator chose not to test it, so the criterion cannot be
confirmed.

## Per-category sub-scores

The API returns a breakdown per category so the UI can show where points were
lost:

```json
{
  "score": 72,
  "grade": "C",
  "categories": {
    "dnssec":            { "score": 55, "penalties": 45, "entry_count": 3 },
    "nameserver_health": { "score": 85, "penalties": 15, "entry_count": 2 },
    "connectivity":      { "score": 90, "penalties": 10, "entry_count": 1 },
    "zone_consistency":  { "score": 100, "penalties": 0,  "entry_count": 0 }
  },
  "bonus": {
    "eligible": false,
    "criteria": {
      "dnssec_enabled": true,
      "strong_algorithm": true,
      "nsec3_non_optout": true,
      "cds_cdnskey_published": null,
      "ipv6_all_nameservers": true,
      "as_diversity": true
    }
  }
}
```

Categories that receive no penalizing entries score 100. This means profiles
that skip certain modules do not penalize the domain for the missing tests -
the skipped categories simply score perfectly.

## Disabled IP stacks

When a profile disables an IP stack (e.g. `ipv4_disabled` or
`ipv6_disabled`), the scoring engine detects this from the entry tags and
reports it in the `disabled_stacks` array. The score is partial: tests for
the disabled stack were not run, so the score may be higher than a full run
would produce. The UI displays a notice when `disabled_stacks` is non-empty.

## Undelegated test runs

Undelegated (pre-delegation) test runs are scored identically to delegated
runs. The scoring engine does not distinguish between them - it evaluates
whatever entries the engine produces. Some tests may produce different
results for undelegated zones (e.g. delegation-related checks), which affects
the score accordingly.

## Known limitations

**Profile affects score.** Scores are only meaningfully comparable across
runs that use the same profile. A profile that disables DNSSEC tests
eliminates all DNSSEC-category penalties, which can significantly inflate the
score. When comparing scores, ensure the profile is consistent.

**Zero-entry runs.** A run that produces zero entries (possible with very
restrictive profiles) receives score 100, grade A. Bonus criteria cannot be
evaluated without entries, so A+ is not awarded.

**Config changes affect historical scores.** The result endpoints recompute
scores on every read using the current scoring config. If you change weights
or penalties, API responses for historical runs will reflect the new config.
The score and grade stored in the database at graduation time are not
updated retroactively, so list views may show stale values until the result
is accessed.

## Configuration

The scoring engine uses sensible defaults with no configuration required. To
customize, create a JSON file and pass it via the `--scoring-config` CLI flag
or the `scoring_config_path` server config key.

### Scoring config file

```json
{
  "severity_penalties": {
    "NOTICE":   1,
    "WARNING":  5,
    "ERROR":    20,
    "CRITICAL": 0
  },
  "category_weights": {
    "dnssec":            1.5,
    "nameserver_health": 1.2,
    "connectivity":      1.0,
    "zone_consistency":  0.8
  },
  "module_categories": {
    "DNSSEC":       "dnssec",
    "NAMESERVER":   "nameserver_health",
    "BASIC":        "nameserver_health",
    "DELEGATION":   "nameserver_health",
    "CONNECTIVITY": "connectivity",
    "ADDRESS":      "connectivity",
    "CONSISTENCY":  "zone_consistency",
    "ZONE":         "zone_consistency",
    "SYNTAX":       "zone_consistency"
  },
  "tag_penalties": {
    "DS07_NOT_SIGNED":            20,
    "DS07_NO_DS_FOR_SIGNED_ZONE": 20,
    "NO_IPV6_NS_CHILD":           20,
    "NO_IPV6_NS_DEL":             20,
    "N15_SOFTWARE_VERSION":        0,
    "N16_HAS_NSID":                0,
    "N18_NO_EXTENDED_ERROR":          0,
    "N18_EXTENDED_ERROR_REPORTED":    0,
    "N18_SERVER_ERROR_REPORTED":      0,
    "N18_RESOLVER_BEHAVIOR_REPORTED": 0,
    "N18_FILTERED_RESPONSE":          0,
    "N18_NO_RESPONSE":                0
  },
  "grade_bands": [
    { "grade": "A", "min_score": 90 },
    { "grade": "B", "min_score": 75 },
    { "grade": "C", "min_score": 60 },
    { "grade": "D", "min_score": 40 },
    { "grade": "F", "min_score": 0 }
  ],
  "bonus_criteria": {
    "no_warnings_or_errors":  true,
    "dnssec_enabled":         true,
    "strong_algorithm":       true,
    "nsec3_non_optout":       true,
    "cds_cdnskey_published":  true,
    "ipv6_all_nameservers":   true,
    "as_diversity":           true
  }
}
```

Fields absent from the file retain their default values. Map fields
(`severity_penalties`, `category_weights`, `module_categories`,
`tag_penalties`) are replaced entirely when present - they are not merged
with defaults.

### Disabling categories

Set a category weight to 0 to exclude it entirely. For example, to ignore
zone consistency:

```json
{
  "category_weights": {
    "dnssec":            1.5,
    "nameserver_health": 1.2,
    "connectivity":      1.0,
    "zone_consistency":  0.0
  }
}
```

### Disabling bonus criteria

Set individual criteria to `false` to exclude them from A+ evaluation:

```json
{
  "bonus_criteria": {
    "no_warnings_or_errors":  true,
    "dnssec_enabled":         true,
    "strong_algorithm":       true,
    "nsec3_non_optout":       true,
    "cds_cdnskey_published":  false,
    "ipv6_all_nameservers":   false,
    "as_diversity":           true
  }
}
```

### Private infrastructure

For private DNS infrastructure (no public IPv6, split-horizon, etc.),
consider:

- Disabling `ipv6_all_nameservers` bonus criterion
- Reducing the `connectivity` category weight
- Setting tag penalties for IPv6-related tags to 0
- Disabling `as_diversity` if nameservers are intentionally co-located

## Server settings

Two server config flags control scoring visibility in the admin and public
UIs:

| Setting              | Default | Effect                                    |
|----------------------|---------|-------------------------------------------|
| `show_score_admin`   | `true`  | Show/hide scoring in admin interface      |
| `show_score_public`  | `true`  | Show/hide scoring in public results page  |

Both flags can be toggled at runtime via the Server Settings tab in the admin
UI. When disabled, the API omits the `score` field from result responses and
the UI hides all scoring elements.

The public UI uses a **fail-safe default**: scoring is hidden until the
server confirms it is enabled. If the feature flag endpoint is unreachable,
scoring remains hidden.

## API fields

### Result endpoints

`GET /api/v1/jobs/{id}/result`, `GET /api/v1/runs/{id}/result`,
`GET /pub/api/v1/jobs/{publicID}/result`

The `score` field is included when scoring is enabled:

| Field                        | Type              | Description                                |
|------------------------------|-------------------|--------------------------------------------|
| `score.score`                | `int`             | Aggregate score 0–100                      |
| `score.grade`                | `string`          | Letter grade: A+, A, B, C, D, F            |
| `score.categories`           | `object`          | Per-category breakdown (see below)         |
| `score.bonus`                | `object`          | A+ eligibility and criteria outcomes       |
| `score.disabled_stacks`      | `string[]`        | IP stacks that were disabled in the profile|

Each category in `score.categories`:

| Field          | Type  | Description                            |
|----------------|-------|----------------------------------------|
| `score`        | `int` | Category sub-score 0–100              |
| `penalties`    | `int` | Total unweighted penalty points       |
| `entry_count`  | `int` | Number of penalizing entries          |

The `score.bonus` object:

| Field      | Type                   | Description                          |
|------------|------------------------|--------------------------------------|
| `eligible` | `bool`                 | True when grade is A+                |
| `criteria` | `map[string]*bool`     | Per-criterion outcome (see below)    |

Criterion values: `true` = met, `false` = not met (blocks A+),
`null` = not applicable (does not block A+).

### Run and domain list endpoints

`GET /api/v1/runs`, `GET /api/v1/domains`, `GET /api/v1/domains/{id}/runs`

| Field   | Type      | Description                      |
|---------|-----------|----------------------------------|
| `score` | `int?`    | Aggregate score (nullable)       |
| `grade` | `string?` | Letter grade (nullable)          |

### Feature flag endpoints

`GET /api/v1/features` (admin), `GET /pub/api/v1/info` (public)

| Field                | Type   | Description                       |
|----------------------|--------|-----------------------------------|
| `show_score_admin`   | `bool` | Whether admin scoring is enabled  |
| `show_score_public`  | `bool` | Whether public scoring is enabled |

### CSV export

`GET /api/v1/entries?format=csv` includes `score` and `grade` columns with
per-run values repeated on each entry row.
