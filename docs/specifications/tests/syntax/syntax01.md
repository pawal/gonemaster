# Syntax01 (syntax01)

Status: Final

## Purpose
- Validate that the tested domain name contains only allowed DNS hostname characters.

## Preconditions And Inputs
- Preconditions:
  - A `zone.Zone` object is available.
- Required inputs:
  - Child zone name (`z.Name`).
- Profile/config knobs that affect behavior:
  - None inside this testcase.

## Algorithm And Decision Flow
1. Emit `TEST_CASE_START`.
2. Evaluate all labels in the child zone name with `nameHasOnlyLegalCharacters`.
3. If all labels pass, emit `ONLY_ALLOWED_CHARS`; otherwise emit `NON_ALLOWED_CHARS`.
4. Emit `TEST_CASE_END`.

## Emitted Tags (Possible Set)
| Tag | Emitted when |
| --- | --- |
| `ONLY_ALLOWED_CHARS` | All domain labels contain only `A-Z`, `a-z`, `0-9`, and `-`. |
| `NON_ALLOWED_CHARS` | At least one domain label contains a disallowed character. |
| `TEST_CASE_END` | Testcase completion marker is emitted. |
| `TEST_CASE_START` | Testcase start marker is emitted. |

## Tag Arguments
| Tag | Argument key | Type | Meaning |
| --- | --- | --- | --- |
| `ONLY_ALLOWED_CHARS` | `domain` | `string` | Tested domain name. |
| `NON_ALLOWED_CHARS` | `domain` | `string` | Tested domain name. |
| `TEST_CASE_END` | `testcase` | `string` | Testcase display name (`Syntax01`). |
| `TEST_CASE_START` | `testcase` | `string` | Testcase display name (`Syntax01`). |

## Severity Levels Per Tag
| Tag | Level | Notes |
| --- | --- | --- |
| `ONLY_ALLOWED_CHARS` | `INFO` | Default from `share/profile.json` (`test_levels.SYNTAX`). |
| `NON_ALLOWED_CHARS` | `ERROR` | Default from `share/profile.json` (`test_levels.SYNTAX`). |
| `TEST_CASE_END` | `DEBUG` | Default from `share/profile.json` (`test_levels.SYNTAX`). |
| `TEST_CASE_START` | `DEBUG` | Default from `share/profile.json` (`test_levels.SYNTAX`). |

## Differences From Upstream
- Upstream reference: [`syntax01.md`](../../upstream/tests/Syntax-TP/syntax01.md)
- Differences (Upstream vs Gonemaster):
  - Upstream: No material behavioral difference identified. Gonemaster: Matches upstream behavior for this testcase.
- Potential upstream report:
  - `no`

## Edge Cases And Limitations
- For the root name (`.`), label iteration is empty and the testcase emits `ONLY_ALLOWED_CHARS`.
- This testcase does not validate label length or full-name length; it only validates character set.
