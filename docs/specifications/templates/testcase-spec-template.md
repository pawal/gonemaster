# <Module><NN> (<moduleNN>)

Status: Draft | Reviewed | Final

## Purpose
- Describe what this testcase verifies.

## Inputs And Preconditions
- Required inputs:
  - `<input>`
- Preconditions:
  - `<condition>`
- Profile/config knobs that affect behavior:
  - `<profile key>`

## Algorithm And Decision Flow
1. <step>
2. <step>
3. <step>

## Emitted Tags (Possible Set)
| Tag | Level | Emitted when |
| --- | --- | --- |
| `<TAG_NAME>` | `<INFO|NOTICE|WARNING|ERROR|CRITICAL|DEBUG...>` | `<condition>` |

## Tag Arguments
| Tag | Argument key | Type | Meaning |
| --- | --- | --- | --- |
| `<TAG_NAME>` | `<arg>` | `<string|int|bool|...>` | `<description>` |

## Differences From Upstream
- Upstream reference: `<path or link>`
- Differences:
  - `<difference>`

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
