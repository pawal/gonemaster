# Nameserver06 (nameserver06)

Status: Final

## Purpose
- Verify that NS names discovered from delegation/child data can be resolved to at least one IP address.

## Preconditions And Inputs
- Preconditions:
  - A `zone.Zone` object is available.
- Required inputs:
  - NS names from `methods.Method2` and `methods.Method3`.
  - Resolved nameserver data from `methods.Method4and5`.
- Profile/config knobs that affect behavior:
  - No direct profile knob in this testcase.

## Algorithm And Decision Flow
1. Emit `TEST_CASE_START`.
2. Read names from `Method2` (delegation) and `Method3` (child), lowercase each name, and build unique union `allNames`.
3. Read resolved nameserver list from `Method4and5`, lowercase each nameserver name, and build set `withIP`.
4. Build sorted `withoutIP` list for names in `allNames` that are not present in `withIP`.
5. Emit one of:
   - `CAN_NOT_BE_RESOLVED` when `withoutIP` is non-empty and `withIP` is non-empty.
   - `NO_RESOLUTION` when `withIP` is empty.
   - `CAN_BE_RESOLVED` otherwise.
6. Emit `TEST_CASE_END`.

## Emitted Tags (Possible Set)
| Tag | Emitted when |
| --- | --- |
| `CAN_BE_RESOLVED` | All discovered names are resolved under current method outputs. |
| `CAN_NOT_BE_RESOLVED` | At least one discovered nameserver name failed to resolve and at least one resolved successfully. |
| `NO_RESOLUTION` | No resolved nameserver entries were available. |
| `TEST_CASE_END` | Testcase completion marker is emitted. |
| `TEST_CASE_START` | Testcase start marker is emitted. |

## Tag Arguments
| Tag | Argument key | Type | Meaning |
| --- | --- | --- | --- |
| `CAN_BE_RESOLVED` | `-` | `-` | No arguments. |
| `CAN_NOT_BE_RESOLVED` | `servers` | `array<object>` | Structured unresolved nameserver names as `{ns}` items (lowercase, sorted). |
| `NO_RESOLUTION` | `names` | `string` | Comma-delimited unresolved NS names (lowercase, sorted). |
| `TEST_CASE_END` | `testcase` | `string` | Testcase display name (`Nameserver06`). |
| `TEST_CASE_START` | `testcase` | `string` | Testcase display name (`Nameserver06`). |

## Severity Levels Per Tag
| Tag | Level | Notes |
| --- | --- | --- |
| `CAN_BE_RESOLVED` | `INFO` | Default from `share/profile.json` (`test_levels.NAMESERVER`). |
| `CAN_NOT_BE_RESOLVED` | `ERROR` | Default from `share/profile.json` (`test_levels.NAMESERVER`). |
| `NO_RESOLUTION` | `ERROR` | Default from `share/profile.json` (`test_levels.NAMESERVER`). |
| `TEST_CASE_END` | `DEBUG` | Default from `share/profile.json` (`test_levels.NAMESERVER`). |
| `TEST_CASE_START` | `DEBUG` | Default from `share/profile.json` (`test_levels.NAMESERVER`). |

## Differences From Upstream
- Upstream reference: [`nameserver06.md`](../../upstream/tests/Nameserver-TP/nameserver06.md)
- Differences (Upstream vs Gonemaster):
  - Upstream: describes failure when any nameserver name does not resolve. Gonemaster: distinguishes partial failure (`CAN_NOT_BE_RESOLVED`) from complete failure (`NO_RESOLUTION`) and also emits explicit success (`CAN_BE_RESOLVED`).
  - Upstream: special requirements mention transport-disable handling. Gonemaster: has no direct transport-disabled branch in this testcase; it uses resolved data produced by methods.
  - Upstream: does not explicitly describe testcase boundary markers. Gonemaster: emits `TEST_CASE_START` and `TEST_CASE_END`.
- Potential upstream report:
  - `no`

## Edge Cases And Limitations
- Name comparisons are case-insensitive (all names lowercased before set operations).
- If both `allNames` and `withIP` are empty, `NO_RESOLUTION` is emitted with empty `names`.
- Resolution quality depends entirely on upstream method outputs (`Method2`, `Method3`, `Method4and5`).
