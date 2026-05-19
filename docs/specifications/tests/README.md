# Gonemaster Testcase Specifications

This directory contains canonical testcase specifications for gonemaster.

Each testcase spec MUST be based on current implementation behavior and SHOULD
start from `docs/specifications/templates/testcase-spec-template.md`.

## Nameserver Resolution Vocabulary

Specs refer to several distinct views of the nameservers that serve a zone.
These are not interchangeable; testcases pick the view that matches what is
actually being verified.

See [Nameserver Resolution Reference](../nameserver-resolution.md) for the
canonical description of each function, with signatures and source
locations. Testcase spec files link symbol mentions back to that page.

## Normalization Rules

- One testcase specification per file.
- Canonical file location format:
  - `docs/specifications/tests/<module>/<testcase>.md`
  - Example: `docs/specifications/tests/zone/zone07.md`
- Specs describe gonemaster behavior, not desired future behavior.
- If upstream and implementation differ, the difference MUST be explicit.
- Every emitted tag documented for a testcase MUST be possible in code.

## Normative Language

Use RFC 2119 style keywords with their normal meanings:
- `MUST`, `MUST NOT` for required behavior.
- `SHOULD`, `SHOULD NOT` for strong recommendations.
- `MAY` for optional behavior.

Avoid vague terms like "usually", "often", or "normally" unless followed by
explicit conditions.

## Naming And Casing Policy

- Module identifier in spec metadata and paths: lowercase (`basic`, `zone`,
  `dnssec`).
- Testcase identifier in metadata and paths: lowercase with numeric suffix
  (`basic01`, `zone07`, `dnssec10`).
- Human-readable display names may use gonemaster style casing (`Basic01`,
  `Zone07`, `DNSSEC10`), but the canonical ID remains lowercase.
- Tag names MUST preserve exact emitted casing from implementation.

## Mandatory Sections Per Testcase

Every testcase spec MUST contain all sections below.

1. Purpose
2. Preconditions and inputs
3. Algorithm/decision flow
4. Emitted tags (all possible tags)
5. Tag argument keys and meaning
6. Severity level per tag
7. Differences from upstream
8. Edge cases / known limitations

## Section Requirements

### Purpose
- State exactly what the testcase validates.
- Keep scope bounded to one testcase.

### Preconditions And Inputs
- List required input data and environmental assumptions.
- List profile/config knobs that can change behavior.

### Algorithm/Decision Flow
- Describe branch conditions in deterministic order.
- For each branch, tie outcomes to emitted tags.

### Emitted Tags (All Possible Tags)
- Include every tag that can be emitted by current implementation.
- Include emission conditions precise enough for test assertions.

### Tag Argument Keys And Meaning
- Document argument key names exactly as emitted.
- Include expected type and semantic meaning for each key.

### Severity Level Per Tag
- Provide an explicit level mapping for every emitted tag.
- If level can vary by profile/rule, document default and override behavior.

### Differences From Upstream
- Link relevant upstream spec text.
- Explicitly document behavioral mismatch and rationale.
- Each difference bullet MUST label both sides explicitly using this pattern:
  `Upstream: ... Gonemaster: ...`.
- If mismatch appears report-worthy upstream, include tracking status/evidence.

### Edge Cases / Known Limitations
- Document known non-obvious behavior and boundary conditions.
- Document known gaps, unsupported paths, and ambiguous areas.

## Implementation-Defined vs Protocol-Defined Behaviors

DNS testcase behavior falls into two distinct categories:

- **Protocol-defined**: The behavior is mandated (or strongly implied) by an RFC
  or DNS standard.  Deviating from it constitutes a conformance failure.
- **Implementation-defined**: The behavior reflects a specific engineering choice
  made in gonemaster that is not mandated by the protocol.  A compliant
  alternative implementation could legitimately make different choices and still
  be correct.

Where the distinction is non-obvious, a testcase spec SHOULD include an
`## Implementation Notes` section that explicitly lists the implementation-defined
choices and, where helpful, contrasts them with the underlying protocol
requirement.

**Common categories of implementation-defined behavior in gonemaster:**

| Category | Examples |
| --- | --- |
| Parallel execution | `resolver.defaults.parallel` controls whether per-nameserver queries run sequentially or concurrently |
| Deduplication strategy | Group by `name/ip`, by IP only, or by composite key; first-seen-wins vs last-seen-wins |
| List ordering | Sorting `servers` values before joining; lexicographic ordering of serial keys |
| Argument formatting | Semicolon delimiter for `servers`, slash delimiter for PTR names |
| Reference time source | DNSKEY packet timestamp vs wall-clock `time.Now()` for RRSIG validity checks |
| OK-tag gating | Emitting a success tag only when no non-start diagnostic tag was emitted |
| Module orchestration | Testcase A runs only if testcase B emitted a specific tag |
| Loop protection | Fixed internal iteration limit with no protocol counterpart |

## Upstream Deviation Reporting

If a difference looks like a clear upstream spec/code issue, mark it in the
testcase spec as a potential upstream report with evidence and current report
status.
