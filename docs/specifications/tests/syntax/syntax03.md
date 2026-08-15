# Syntax03

Status: Final

## Purpose
- Validate that domain labels do not contain a double hyphen in positions 3 and 4, except ACE labels (`xn--...`).

## Preconditions And Inputs
- Preconditions:
  - A `zone.Zone` object is available.
- Required inputs:
  - Child zone name (`z.Name`).
- Profile/config knobs that affect behavior:
  - None inside this testcase.

## Algorithm And Decision Flow
1. Emit `TEST_CASE_START`.
2. For each label in the tested domain, evaluate `labelNotACEHasDoubleHyphen`.
3. Emit `DISCOURAGED_DOUBLE_DASH` for each label that matches.
4. If at least one label exists and no issues were emitted, emit `NO_DOUBLE_DASH`.
5. Emit `TEST_CASE_END`.

## Emitted Tags (Possible Set)
| Tag | Emitted when |
| --- | --- |
| `DISCOURAGED_DOUBLE_DASH` | A non-ACE label has `--` in positions 3 and 4. |
| `NO_DOUBLE_DASH` | Domain labels exist and no discouraged double dash is found. |
| `TEST_CASE_END` | Testcase completion marker is emitted. |
| `TEST_CASE_START` | Testcase start marker is emitted. |

## Tag Arguments
| Tag | Argument key | Type | Meaning |
| --- | --- | --- | --- |
| `DISCOURAGED_DOUBLE_DASH` | `label` | `string` | Label with discouraged double dash pattern. |
| `DISCOURAGED_DOUBLE_DASH` | `domain` | `string` | Tested domain name. |
| `NO_DOUBLE_DASH` | `domain` | `string` | Tested domain name. |
| `TEST_CASE_END` | `testcase` | `string` | Testcase display name (`Syntax03`). |
| `TEST_CASE_START` | `testcase` | `string` | Testcase display name (`Syntax03`). |

## Severity Levels Per Tag
| Tag | Level | Notes |
| --- | --- | --- |
| `DISCOURAGED_DOUBLE_DASH` | `WARNING` | Default from `share/profile.json` (`test_levels.SYNTAX`). |
| `NO_DOUBLE_DASH` | `INFO` | Default from `share/profile.json` (`test_levels.SYNTAX`). |
| `TEST_CASE_END` | `DEBUG` | Default from `share/profile.json` (`test_levels.SYNTAX`). |
| `TEST_CASE_START` | `DEBUG` | Default from `share/profile.json` (`test_levels.SYNTAX`). |

## Differences From Upstream
- Differences (Upstream vs Gonemaster):
  - Upstream: the "Test case identifier" heading reads `SYNTAX02`, a typo. Gonemaster: runs the testcase as `syntax03` and emits `Syntax03` markers; no behavioral difference follows from the typo.
- Potential upstream report:
  - `yes`
- If yes, include:
  - Upstream expected behavior: Identifier should match testcase ID `SYNTAX03`.
  - Gonemaster observed behavior: Testcase executes as `syntax03` and emits `Syntax03` markers.
  - evidence: `engine/test/syntax/syntax.go`.
  - report status: `not filed`

## Edge Cases And Limitations
- ACE labels beginning with `xn` are excluded from this warning, including `xn--...` labels.
- If the tested name has zero labels, `NO_DOUBLE_DASH` is not emitted.
