# Syntax04 (syntax04)

Status: Draft

## Purpose
- Validate syntax of nameserver hostnames gathered from parent delegation and child apex NS sets.

## Preconditions And Inputs
- Preconditions:
  - A `zone.Zone` object is available.
- Required inputs:
  - Nameserver names from `methods.Method2` and `methods.Method3`.
- Profile/config knobs that affect behavior:
  - `resolver.defaults.parallel`: controls parallel per-nameserver hostname checks.

## Algorithm And Decision Flow
1. Emit `TEST_CASE_START`.
2. Load nameserver names from `Method2` and `Method3`.
3. Merge names into a deduplicated, case-insensitive set and sort keys for deterministic output.
4. For each unique name, run `checkNameSyntaxWithLogger("NAMESERVER", name)` (parallelizable):
   - Emit `NAMESERVER_NON_ALLOWED_CHARS` if any label has disallowed characters.
   - Emit `NAMESERVER_DISCOURAGED_DOUBLE_DASH` for non-ACE `--` in positions 3 and 4.
   - Emit `NAMESERVER_NUMERIC_TLD` for all-numeric TLD.
   - Emit `NAMESERVER_SYNTAX_OK` only if no prior issue tag was emitted for that name.
5. Emit `TEST_CASE_END`.

## Emitted Tags (Possible Set)
| Tag | Emitted when |
| --- | --- |
| `NAMESERVER_DISCOURAGED_DOUBLE_DASH` | A nameserver label has non-ACE `--` in positions 3 and 4. |
| `NAMESERVER_NON_ALLOWED_CHARS` | A nameserver label contains disallowed characters. |
| `NAMESERVER_NUMERIC_TLD` | Nameserver rightmost label is numeric-only. |
| `NAMESERVER_SYNTAX_OK` | Name passes all implemented checks. |
| `TEST_CASE_END` | Testcase completion marker is emitted. |
| `TEST_CASE_START` | Testcase start marker is emitted. |

## Tag Arguments
| Tag | Argument key | Type | Meaning |
| --- | --- | --- | --- |
| `NAMESERVER_DISCOURAGED_DOUBLE_DASH` | `label` | `string` | Label with discouraged double dash pattern. |
| `NAMESERVER_DISCOURAGED_DOUBLE_DASH` | `domain` | `string` | Nameserver hostname being checked. |
| `NAMESERVER_NON_ALLOWED_CHARS` | `domain` | `string` | Nameserver hostname with disallowed characters. |
| `NAMESERVER_NUMERIC_TLD` | `domain` | `string` | Nameserver hostname being checked. |
| `NAMESERVER_NUMERIC_TLD` | `tld` | `string` | Numeric TLD label. |
| `NAMESERVER_SYNTAX_OK` | `domain` | `string` | Nameserver hostname that passed checks. |
| `TEST_CASE_END` | `testcase` | `string` | Testcase display name (`Syntax04`). |
| `TEST_CASE_START` | `testcase` | `string` | Testcase display name (`Syntax04`). |

## Severity Levels Per Tag
| Tag | Level | Notes |
| --- | --- | --- |
| `NAMESERVER_DISCOURAGED_DOUBLE_DASH` | `WARNING` | Default from `share/profile.json` (`test_levels.SYNTAX`). |
| `NAMESERVER_NON_ALLOWED_CHARS` | `ERROR` | Default from `share/profile.json` (`test_levels.SYNTAX`). |
| `NAMESERVER_NUMERIC_TLD` | `ERROR` | Default from `share/profile.json` (`test_levels.SYNTAX`). |
| `NAMESERVER_SYNTAX_OK` | `INFO` | Default from `share/profile.json` (`test_levels.SYNTAX`). |
| `TEST_CASE_END` | `DEBUG` | Default from `share/profile.json` (`test_levels.SYNTAX`). |
| `TEST_CASE_START` | `DEBUG` | Default from `share/profile.json` (`test_levels.SYNTAX`). |

## Differences From Upstream
- Upstream reference: [`syntax04.md`](../../upstream/tests/Syntax-TP/syntax04.md)
- Differences (Upstream vs Gonemaster):
  - Upstream text describes hostname-length checks (label <= 63, name <= 255), but Gonemaster only checks character set, numeric TLD, and discouraged double dash.
- Potential upstream report:
  - `yes`
- If yes, include:
  - Upstream expected behavior: Explicit length validation in ordered steps.
  - Gonemaster observed behavior: No length checks in `checkNameSyntaxWithLogger`.
  - evidence: `engine/test/syntax/syntax.go` (`checkNameSyntaxWithLogger`).
  - report status: `not filed`

## Edge Cases And Limitations
- Duplicate nameserver hostnames are evaluated once (case-insensitive).
- Output order is deterministic after parallel execution due sorted input and ordered runner merge.
- When running through `syntax.All`, this testcase is skipped if `syntax01` did not emit `ONLY_ALLOWED_CHARS`.
