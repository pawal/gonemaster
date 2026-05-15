# Syntax02

Status: Final

## Purpose
- Validate that no domain label starts or ends with a hyphen (`-`).

## Preconditions And Inputs
- Preconditions:
  - A `zone.Zone` object is available.
- Required inputs:
  - Child zone name (`z.Name`).
- Profile/config knobs that affect behavior:
  - None inside this testcase.

## Algorithm And Decision Flow
1. Emit `TEST_CASE_START`.
2. For each label in the tested domain:
   - If the label starts with `-`, emit `INITIAL_HYPHEN`.
   - If the label ends with `-`, emit `TERMINAL_HYPHEN`.
3. If at least one label exists and no issues were emitted, emit `NO_ENDING_HYPHENS`.
4. Emit `TEST_CASE_END`.

## Emitted Tags (Possible Set)
| Tag | Emitted when |
| --- | --- |
| `INITIAL_HYPHEN` | A label starts with `-`. |
| `TERMINAL_HYPHEN` | A label ends with `-`. |
| `NO_ENDING_HYPHENS` | Domain labels exist and none starts/ends with `-`. |
| `TEST_CASE_END` | Testcase completion marker is emitted. |
| `TEST_CASE_START` | Testcase start marker is emitted. |

## Tag Arguments
| Tag | Argument key | Type | Meaning |
| --- | --- | --- | --- |
| `INITIAL_HYPHEN` | `label` | `string` | Label that starts with `-`. |
| `INITIAL_HYPHEN` | `domain` | `string` | Tested domain name. |
| `TERMINAL_HYPHEN` | `label` | `string` | Label that ends with `-`. |
| `TERMINAL_HYPHEN` | `domain` | `string` | Tested domain name. |
| `NO_ENDING_HYPHENS` | `domain` | `string` | Tested domain name. |
| `TEST_CASE_END` | `testcase` | `string` | Testcase display name (`Syntax02`). |
| `TEST_CASE_START` | `testcase` | `string` | Testcase display name (`Syntax02`). |

## Severity Levels Per Tag
| Tag | Level | Notes |
| --- | --- | --- |
| `INITIAL_HYPHEN` | `ERROR` | Default from `share/profile.json` (`test_levels.SYNTAX`). |
| `TERMINAL_HYPHEN` | `ERROR` | Default from `share/profile.json` (`test_levels.SYNTAX`). |
| `NO_ENDING_HYPHENS` | `INFO` | Default from `share/profile.json` (`test_levels.SYNTAX`). |
| `TEST_CASE_END` | `DEBUG` | Default from `share/profile.json` (`test_levels.SYNTAX`). |
| `TEST_CASE_START` | `DEBUG` | Default from `share/profile.json` (`test_levels.SYNTAX`). |

## Differences From Upstream
- Upstream reference: [`syntax02.md`](../../upstream/tests/Syntax-TP/syntax02.md)
- Differences (Upstream vs Gonemaster):
  - Upstream: No material behavioral difference identified. Gonemaster: Matches upstream behavior for this testcase.
- Potential upstream report:
  - `no`

## Edge Cases And Limitations
- A single label can emit both `INITIAL_HYPHEN` and `TERMINAL_HYPHEN`.
- If the tested name has zero labels, `NO_ENDING_HYPHENS` is not emitted.
