# Delegation07

Status: Final

## Purpose
- Compare parent-side and child-side NS name sets and report mismatches.

## Preconditions And Inputs
- Preconditions:
  - A `zone.Zone` object is available.
- Required inputs:
  - Parent-side NS names from `methods.Method2`.
  - Child-side NS names from `methods.Method3`.
- Profile/config knobs that affect behavior:
  - No direct profile knob in this testcase.

## Algorithm And Decision Flow
1. Emit `TEST_CASE_START`.
2. Read parent names from `Method2` and child names from `Method3`.
3. Build `nameCounts` map by adding `+1` per parent name and `-1` per child name.
4. Partition names:
   - `sameNames` for `count == 0`.
   - `extraParent` for `count > 0`.
   - `extraChild` for `count < 0`.
5. Sort `sameNames`, `extraParent`, and `extraChild`.
6. Emit `EXTRA_NAME_PARENT` when `extraParent` is non-empty.
7. Emit `EXTRA_NAME_CHILD` when `extraChild` is non-empty.
8. Emit `NAMES_MATCH` with `sameNames` only when both extra lists are empty.
9. Emit `TOTAL_NAME_MISMATCH` when `sameNames` is empty.
10. Emit `TEST_CASE_END`.

### Parent-Child Name Set Comparison (steps 2-10)

{{% expand "Show diagram" %}}
{{< mermaid >}}
stateDiagram-v2
    [*] --> setup
    setup : build nameCounts
    setup --> p1
    p1 : parent extras?
    p1 --> emitP : yes
    p1 --> p2 : no
    emitP : extra-name-parent tag
    emitP --> p2
    p2 : child extras?
    p2 --> emitC : yes
    p2 --> p3 : no
    emitC : extra-name-child tag
    emitC --> p3
    p3 : both empty?
    p3 --> emitM : yes
    p3 --> p4 : no
    emitM : names-match tag
    emitM --> p4
    p4 : sameNames empty?
    p4 --> emitMis : yes
    p4 --> done : no
    emitMis : total-mismatch tag
    emitMis --> done
    done : emit test-case-end
    done --> [*]
{{< /mermaid >}}
{{% /expand %}}

## Emitted Tags (Possible Set)
| Tag | Emitted when |
| --- | --- |
| `EXTRA_NAME_CHILD` | One or more NS names are present only on child side (or appear more times on child side). |
| `EXTRA_NAME_PARENT` | One or more NS names are present only on parent side (or appear more times on parent side). |
| `NAMES_MATCH` | Parent and child name collections fully match under current counting logic. |
| `TEST_CASE_END` | Testcase completion marker is emitted. |
| `TEST_CASE_START` | Testcase start marker is emitted. |
| `TOTAL_NAME_MISMATCH` | Parent and child share no names under current counting logic. |

## Tag Arguments
| Tag | Argument key | Type | Meaning |
| --- | --- | --- | --- |
| `EXTRA_NAME_CHILD` | `extra` | `string` | Semicolon-delimited sorted child-only names. |
| `EXTRA_NAME_PARENT` | `extra` | `string` | Semicolon-delimited sorted parent-only names. |
| `NAMES_MATCH` | `names` | `string` | Semicolon-delimited sorted matched names. |
| `TEST_CASE_END` | `testcase` | `string` | Testcase display name (`Delegation07`). |
| `TEST_CASE_START` | `testcase` | `string` | Testcase display name (`Delegation07`). |
| `TOTAL_NAME_MISMATCH` | `glue` | `string` | Semicolon-delimited sorted parent-only names. |
| `TOTAL_NAME_MISMATCH` | `child` | `string` | Semicolon-delimited sorted child-only names. |

## Severity Levels Per Tag
| Tag | Level | Notes |
| --- | --- | --- |
| `EXTRA_NAME_CHILD` | `NOTICE` | Default from `share/profile.json` (`test_levels.DELEGATION`). |
| `EXTRA_NAME_PARENT` | `ERROR` | Default from `share/profile.json` (`test_levels.DELEGATION`). |
| `NAMES_MATCH` | `INFO` | Default from `share/profile.json` (`test_levels.DELEGATION`). |
| `TEST_CASE_END` | `DEBUG` | Default from `share/profile.json` (`test_levels.DELEGATION`). |
| `TEST_CASE_START` | `DEBUG` | Default from `share/profile.json` (`test_levels.DELEGATION`). |
| `TOTAL_NAME_MISMATCH` | `ERROR` | Default from `share/profile.json` (`test_levels.DELEGATION`). |

## Differences From Upstream
- Upstream reference: [`delegation07.md`](../../upstream/tests/Delegation-TP/delegation07.md)
- Differences (Upstream vs Gonemaster):
  - Upstream: describes a parent-to-child containment check for NS names. Gonemaster: performs two-way comparison and emits both parent-extra and child-extra findings.
  - Upstream: describes set comparison semantics. Gonemaster: uses count-based comparison (`+1/-1`) so repeated names can affect classification.
  - Upstream: does not describe testcase boundary debug markers. Gonemaster: emits `TEST_CASE_START` and `TEST_CASE_END`.
- Potential upstream report:
  - `yes`
- If yes, include:
  - Upstream expected behavior: Parent-side NS set membership in child-side NS set is the decisive condition.
  - Gonemaster observed behavior: Symmetric, count-aware comparison with additional tags for child extras and total mismatch.
  - evidence: `docs/specifications/upstream/tests/Delegation-TP/delegation07.md`, `engine/test/delegation/delegation.go`
  - report status: `not filed`

## Edge Cases And Limitations
- `TOTAL_NAME_MISMATCH` can be emitted together with `EXTRA_NAME_PARENT` and `EXTRA_NAME_CHILD`.
- When both parent and child name inputs are empty, `NAMES_MATCH` is emitted with an empty `names` value.
- Repeated occurrences of the same name can produce mismatch findings even when unique-set membership appears equal.
