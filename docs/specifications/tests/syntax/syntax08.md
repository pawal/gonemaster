# Syntax08 (syntax08)

Status: Draft

## Purpose
- Validate syntax of MX exchange hostnames for the tested zone.

## Preconditions And Inputs
- Preconditions:
  - A `zone.Zone` object is available.
- Required inputs:
  - MX response for zone apex.
  - Deduplicated MX exchange targets from answer section.
- Profile/config knobs that affect behavior:
  - `resolver.defaults.parallel`: controls parallel per-MX-target syntax checks.
  - `net.ipv4` and `net.ipv6` can affect `z.QueryOne` transport availability.

## Algorithm And Decision Flow
1. Emit `TEST_CASE_START`.
2. Execute `z.QueryOne` for MX at zone apex.
3. If response is present:
   - Extract MX exchanges from answer section.
   - Deduplicate case-insensitively, preserving first-seen order.
   - For each unique exchange, run `checkNameSyntaxWithLogger("MX", target)` in parallel.
   - Emit one or more of: `MX_NON_ALLOWED_CHARS`, `MX_DISCOURAGED_DOUBLE_DASH`, `MX_NUMERIC_TLD`, or `MX_SYNTAX_OK`.
   - Emit `TEST_CASE_END` and return.
4. If no response message is available, emit `NO_RESPONSE_MX_QUERY`.
5. Emit `TEST_CASE_END`.

## Emitted Tags (Possible Set)
| Tag | Emitted when |
| --- | --- |
| `MX_DISCOURAGED_DOUBLE_DASH` | MX hostname has non-ACE `--` in positions 3 and 4. |
| `MX_NON_ALLOWED_CHARS` | MX hostname has disallowed characters. |
| `MX_NUMERIC_TLD` | MX hostname rightmost label is numeric-only. |
| `MX_SYNTAX_OK` | MX hostname passes all implemented checks. |
| `NO_RESPONSE_MX_QUERY` | No MX response message is obtained. |
| `TEST_CASE_END` | Testcase completion marker is emitted. |
| `TEST_CASE_START` | Testcase start marker is emitted. |

## Tag Arguments
| Tag | Argument key | Type | Meaning |
| --- | --- | --- | --- |
| `MX_DISCOURAGED_DOUBLE_DASH` | `label` | `string` | Label with discouraged double dash pattern. |
| `MX_DISCOURAGED_DOUBLE_DASH` | `domain` | `string` | MX hostname under validation. |
| `MX_NON_ALLOWED_CHARS` | `domain` | `string` | MX hostname with disallowed characters. |
| `MX_NUMERIC_TLD` | `domain` | `string` | MX hostname under validation. |
| `MX_NUMERIC_TLD` | `tld` | `string` | Numeric TLD label. |
| `MX_SYNTAX_OK` | `domain` | `string` | MX hostname that passed checks. |
| `NO_RESPONSE_MX_QUERY` | `-` | `-` | No arguments. |
| `TEST_CASE_END` | `testcase` | `string` | Testcase display name (`Syntax08`). |
| `TEST_CASE_START` | `testcase` | `string` | Testcase display name (`Syntax08`). |

## Severity Levels Per Tag
| Tag | Level | Notes |
| --- | --- | --- |
| `MX_DISCOURAGED_DOUBLE_DASH` | `WARNING` | Default from `share/profile.json` (`test_levels.SYNTAX`). |
| `MX_NON_ALLOWED_CHARS` | `WARNING` | Default from `share/profile.json` (`test_levels.SYNTAX`). |
| `MX_NUMERIC_TLD` | `WARNING` | Default from `share/profile.json` (`test_levels.SYNTAX`). |
| `MX_SYNTAX_OK` | `INFO` | Default from `share/profile.json` (`test_levels.SYNTAX`). |
| `NO_RESPONSE_MX_QUERY` | `DEBUG` | Default from `share/profile.json` (`test_levels.SYNTAX`). |
| `TEST_CASE_END` | `DEBUG` | Default from `share/profile.json` (`test_levels.SYNTAX`). |
| `TEST_CASE_START` | `DEBUG` | Default from `share/profile.json` (`test_levels.SYNTAX`). |

## Differences From Upstream
- Upstream reference: [`syntax08.md`](../../upstream/tests/Syntax-TP/syntax08.md)
- Differences (Upstream vs Gonemaster):
  - Upstream text describes hostname length checks, but Gonemaster only checks character set, numeric TLD, and discouraged double dash.
  - Upstream describes dependency on de-escaped output from `syntax05`; Gonemaster runs independently of `syntax05` output values.
- Potential upstream report:
  - `yes`
- If yes, include:
  - Upstream expected behavior: Explicit length checks and syntax05-linked dependency.
  - Gonemaster observed behavior: No length checks and no syntax05 data dependency.
  - evidence: `docs/specifications/upstream/tests/Syntax-TP/syntax08.md`, `engine/test/syntax/syntax.go` (`All`, `checkNameSyntaxWithLogger`, `Syntax08`).
  - report status: `not filed`

## Edge Cases And Limitations
- If MX response exists but has zero MX answers, no MX-specific tag is emitted (only testcase start/end).
- Duplicate MX targets are checked once (case-insensitive dedupe).
- When running through `syntax.All`, this testcase is skipped if `syntax01` did not emit `ONLY_ALLOWED_CHARS`.
