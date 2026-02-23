# Syntax07 (syntax07)

Status: Draft

## Purpose
- Validate SOA `MNAME` hostname syntax using the same hostname validator as `syntax04`.

## Preconditions And Inputs
- Preconditions:
  - A `zone.Zone` object is available.
- Required inputs:
  - SOA response for the child zone apex.
  - `MNAME` value from the first SOA answer record.
- Profile/config knobs that affect behavior:
  - `net.ipv4` and `net.ipv6` can affect which transport `z.QueryOne` uses.

## Algorithm And Decision Flow
1. Emit `TEST_CASE_START`.
2. Execute `z.QueryOne` for SOA at zone apex.
3. If SOA answer exists, extract first SOA `Ns` (`MNAME`) and run `checkNameSyntax("MNAME", ...)`:
   - Emit `MNAME_NON_ALLOWED_CHARS` if disallowed characters are found.
   - Emit `MNAME_DISCOURAGED_DOUBLE_DASH` for non-ACE `--` in positions 3 and 4.
   - Emit `MNAME_NUMERIC_TLD` for all-numeric TLD.
   - Emit `MNAME_SYNTAX_OK` only if no issue tag was emitted.
4. If no SOA answer is available, emit `NO_RESPONSE_SOA_QUERY`.
5. Emit `TEST_CASE_END`.

## Emitted Tags (Possible Set)
| Tag | Emitted when |
| --- | --- |
| `MNAME_DISCOURAGED_DOUBLE_DASH` | MNAME has non-ACE `--` in positions 3 and 4. |
| `MNAME_NON_ALLOWED_CHARS` | MNAME has disallowed characters. |
| `MNAME_NUMERIC_TLD` | MNAME rightmost label is numeric-only. |
| `MNAME_SYNTAX_OK` | MNAME passes all implemented checks. |
| `NO_RESPONSE_SOA_QUERY` | No usable SOA answer is obtained. |
| `TEST_CASE_END` | Testcase completion marker is emitted. |
| `TEST_CASE_START` | Testcase start marker is emitted. |

## Tag Arguments
| Tag | Argument key | Type | Meaning |
| --- | --- | --- | --- |
| `MNAME_DISCOURAGED_DOUBLE_DASH` | `label` | `string` | Label with discouraged double dash pattern. |
| `MNAME_DISCOURAGED_DOUBLE_DASH` | `domain` | `string` | MNAME hostname under validation. |
| `MNAME_NON_ALLOWED_CHARS` | `domain` | `string` | MNAME hostname with disallowed characters. |
| `MNAME_NUMERIC_TLD` | `domain` | `string` | MNAME hostname under validation. |
| `MNAME_NUMERIC_TLD` | `tld` | `string` | Numeric TLD label. |
| `MNAME_SYNTAX_OK` | `domain` | `string` | MNAME hostname that passed checks. |
| `NO_RESPONSE_SOA_QUERY` | `-` | `-` | No arguments. |
| `TEST_CASE_END` | `testcase` | `string` | Testcase display name (`Syntax07`). |
| `TEST_CASE_START` | `testcase` | `string` | Testcase display name (`Syntax07`). |

## Severity Levels Per Tag
| Tag | Level | Notes |
| --- | --- | --- |
| `MNAME_DISCOURAGED_DOUBLE_DASH` | `WARNING` | Default from `share/profile.json` (`test_levels.SYNTAX`). |
| `MNAME_NON_ALLOWED_CHARS` | `WARNING` | Default from `share/profile.json` (`test_levels.SYNTAX`). |
| `MNAME_NUMERIC_TLD` | `WARNING` | Default from `share/profile.json` (`test_levels.SYNTAX`). |
| `MNAME_SYNTAX_OK` | `INFO` | Default from `share/profile.json` (`test_levels.SYNTAX`). |
| `NO_RESPONSE_SOA_QUERY` | `DEBUG` | Default from `share/profile.json` (`test_levels.SYNTAX`). |
| `TEST_CASE_END` | `DEBUG` | Default from `share/profile.json` (`test_levels.SYNTAX`). |
| `TEST_CASE_START` | `DEBUG` | Default from `share/profile.json` (`test_levels.SYNTAX`). |

## Differences From Upstream
- Upstream reference: [`syntax07.md`](../../upstream/tests/Syntax-TP/syntax07.md)
- Differences (Upstream vs Gonemaster):
  - Upstream text describes label and full-name length checks, but Gonemaster only checks character set, numeric TLD, and discouraged double dash.
- Potential upstream report:
  - `yes`
- If yes, include:
  - Upstream expected behavior: Explicit hostname length validation.
  - Gonemaster observed behavior: No length checks in shared syntax validator.
  - evidence: `docs/specifications/upstream/tests/Syntax-TP/syntax07.md`, `engine/test/syntax/syntax.go` (`checkNameSyntaxWithLogger`).
  - report status: `not filed`

## Edge Cases And Limitations
- If SOA answer has multiple records, only the first SOA is used.
- `NO_RESPONSE_SOA_QUERY` is emitted for both no response and responses lacking SOA answer.
- When running through `syntax.All`, this testcase is skipped if `syntax01` did not emit `ONLY_ALLOWED_CHARS`.
- When running through `syntax.All`, this testcase is skipped if `syntax05` emitted `NO_RESPONSE_SOA_QUERY`.

---

Copyright (c) Patrik Wallström  
Copyright (c) The Swedish Internet Foundation (https://internetstiftelsen.se/en/)  
Copyright (c) AFNIC (https://www.afnic.fr/en/)  
All rights reserved.  

Copyright belongs to external contributor where applicable.  

Creative Commons Attribution 4.0 International License applies. See https://creativecommons.org/licenses/by/4.0/ for the license conditions.
