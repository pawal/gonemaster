# Delegation02

Status: Final

## Purpose
- Detect nameserver IP-address reuse within delegation data, within child data, and across the combined delegation+child addressed NS set.

## Preconditions And Inputs
- Preconditions:
  - A `zone.Zone` object is available.
- Required inputs:
  - Delegation addressed NS from [`GlueNameservers`](../../nameserver-resolution.md#gluenameservers).
  - Child addressed NS from [`ApexNameservers`](../../nameserver-resolution.md#apexnameservers).
- Profile/config knobs that affect behavior:
  - No direct profile knob in this testcase.

## Algorithm And Decision Flow
1. Emit `TEST_CASE_START`.
2. Read delegation addressed NS ([`GlueNameservers`](../../nameserver-resolution.md#gluenameservers)) and child addressed NS ([`ApexNameservers`](../../nameserver-resolution.md#apexnameservers)).
3. Run duplicate-IP detection for delegation addressed NS:
   - For each IP used by two or more different NS names, emit `DEL_NS_SAME_IP`.
   - Else, when delegation list is non-empty, emit `DEL_DISTINCT_NS_IP`.
4. Run duplicate-IP detection for child addressed NS:
   - For each IP used by two or more different NS names, emit `CHILD_NS_SAME_IP`.
   - Else, when child list is non-empty, emit `CHILD_DISTINCT_NS_IP`.
5. Run duplicate-IP detection for the combined delegation+child addressed NS list:
   - For each IP used by two or more different NS names, emit `SAME_IP_ADDRESS`.
   - Else, when combined list is non-empty, emit `DISTINCT_IP_ADDRESS`.
6. Emit `TEST_CASE_END`.

### Three-Set Duplicate-IP Detection (steps 3-5)

{{% expand "Show diagram" %}}
```
delNS    = GlueNameservers
childNS  = ApexNameservers
combined = delNS ++ childNS

findDupNS(nsList, duplicateTag, distinctTag):
   group nsList by address; nsList first deduped by "name/ip"
   any address with >= 2 distinct NS names
     -> for each such address (sorted): emit duplicateTag (servers, address)
   no duplicate emitted AND nsList non-empty
     -> emit distinctTag (no args)

Run three times:
   findDupNS(delNS,    DEL_NS_SAME_IP,    DEL_DISTINCT_NS_IP)
   findDupNS(childNS,  CHILD_NS_SAME_IP,  CHILD_DISTINCT_NS_IP)
   findDupNS(combined, SAME_IP_ADDRESS,   DISTINCT_IP_ADDRESS)

emit TEST_CASE_END
```
{{% /expand %}}

## Emitted Tags (Possible Set)
| Tag | Emitted when |
| --- | --- |
| `CHILD_DISTINCT_NS_IP` | Child addressed NS list is non-empty and has no repeated IP across different NS names. |
| `CHILD_NS_SAME_IP` | Child addressed NS includes an IP shared by two or more different NS names. |
| `DEL_DISTINCT_NS_IP` | Delegation addressed NS list is non-empty and has no repeated IP across different NS names. |
| `DEL_NS_SAME_IP` | Delegation addressed NS includes an IP shared by two or more different NS names. |
| `DISTINCT_IP_ADDRESS` | Combined delegation+child addressed NS list is non-empty and has no repeated IP across different NS names. |
| `SAME_IP_ADDRESS` | Combined delegation+child addressed NS includes an IP shared by two or more different NS names. |
| `TEST_CASE_END` | Testcase completion marker is emitted. |
| `TEST_CASE_START` | Testcase start marker is emitted. |

## Tag Arguments
| Tag | Argument key | Type | Meaning |
| --- | --- | --- | --- |
| `CHILD_DISTINCT_NS_IP` | `-` | `-` | No arguments. |
| `CHILD_NS_SAME_IP` | `servers` | `array<object>` | Structured nameserver names as `{ns}` items sharing `ns_ip`. |
| `CHILD_NS_SAME_IP` | `ns_ip` | `string` | Reused IP address. |
| `DEL_DISTINCT_NS_IP` | `-` | `-` | No arguments. |
| `DEL_NS_SAME_IP` | `servers` | `array<object>` | Structured nameserver names as `{ns}` items sharing `ns_ip`. |
| `DEL_NS_SAME_IP` | `ns_ip` | `string` | Reused IP address. |
| `DISTINCT_IP_ADDRESS` | `-` | `-` | No arguments. |
| `SAME_IP_ADDRESS` | `servers` | `array<object>` | Structured nameserver names as `{ns}` items sharing `ns_ip`. |
| `SAME_IP_ADDRESS` | `ns_ip` | `string` | Reused IP address. |
| `TEST_CASE_END` | `testcase` | `string` | Testcase display name (`Delegation02`). |
| `TEST_CASE_START` | `testcase` | `string` | Testcase display name (`Delegation02`). |

## Severity Levels Per Tag
| Tag | Level | Notes |
| --- | --- | --- |
| `CHILD_DISTINCT_NS_IP` | `INFO` | Default from `share/profile.json` (`test_levels.DELEGATION`). |
| `CHILD_NS_SAME_IP` | `ERROR` | Default from `share/profile.json` (`test_levels.DELEGATION`). |
| `DEL_DISTINCT_NS_IP` | `INFO` | Default from `share/profile.json` (`test_levels.DELEGATION`). |
| `DEL_NS_SAME_IP` | `ERROR` | Default from `share/profile.json` (`test_levels.DELEGATION`). |
| `DISTINCT_IP_ADDRESS` | `INFO` | Default from `share/profile.json` (`test_levels.DELEGATION`). |
| `SAME_IP_ADDRESS` | `ERROR` | Default from `share/profile.json` (`test_levels.DELEGATION`). |
| `TEST_CASE_END` | `DEBUG` | Default from `share/profile.json` (`test_levels.DELEGATION`). |
| `TEST_CASE_START` | `DEBUG` | Default from `share/profile.json` (`test_levels.DELEGATION`). |

## Differences From Upstream
- Differences (Upstream vs Gonemaster):
  - Upstream: specifies duplicate-IP evaluation for delegation side and child side only. Gonemaster: adds a third evaluation over the combined delegation+child set, with `SAME_IP_ADDRESS` or `DISTINCT_IP_ADDRESS`.
  - Upstream: does not describe testcase boundary debug markers. Gonemaster: emits `TEST_CASE_START` and `TEST_CASE_END`.
- Potential upstream report:
  - `yes`
- If yes, include:
  - Upstream expected behavior: Scope is delegation-side and child-side duplicate detection.
  - Gonemaster observed behavior: Adds combined-scope duplicate detection and extra tags.
  - evidence: `docs/specifications/upstream/tests/Delegation-TP/delegation02.md`, `engine/test/delegation/delegation.go`
  - report status: `not filed`

## Edge Cases And Limitations
- Exact duplicate `name/ip` entries are deduplicated before grouping by IP.
- If an evaluated input list is empty, neither the duplicate tag nor the distinct tag is emitted for that list.
