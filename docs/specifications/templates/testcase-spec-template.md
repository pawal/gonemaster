# <Module><NN> (<moduleNN>)

Status: Draft | Reviewed | Final

## Purpose
- Describe what this testcase verifies.

## Preconditions And Inputs
- Preconditions:
  - `<condition>`
- Required inputs:
  - `<input>`
- Profile/config knobs that affect behavior:
  - `<profile key>`

## Algorithm And Decision Flow
1. <step>
2. <step>
3. <step>

## Emitted Tags (Possible Set)
| Tag | Emitted when |
| --- | --- |
| `<TAG_NAME>` | `<condition>` |

## Tag Arguments
| Tag | Argument key | Type | Meaning |
| --- | --- | --- | --- |
| `<TAG_NAME>` | `<arg>` | `<string|int|bool|...>` | `<description>` |

## Severity Levels Per Tag
| Tag | Level | Notes |
| --- | --- | --- |
| `<TAG_NAME>` | `<INFO|NOTICE|WARNING|ERROR|CRITICAL|DEBUG...>` | `<default/override behavior>` |

## Differences From Upstream
- Upstream reference: `<path or link>`
- Differences (Upstream vs Gonemaster):
  - `Upstream: <what upstream spec text says>. Gonemaster: <what Gonemaster implementation does>.`
- Potential upstream report:
  - `yes|no`
- If yes, include:
  - Upstream expected behavior: `<text>`
  - Gonemaster observed behavior: `<text>`
  - evidence: `<code path or test>`
  - report status: `<not filed|filed|fixed upstream>`

## Edge Cases And Limitations
- `<edge case>`
- `<known limitation>`

## Evidence In Gonemaster
- Code paths:
  - `engine/test/<module>/<file>.go`
- Related tests:
  - `engine/test/<module>/<file>_test.go`

## Open Questions
- `<question>`
