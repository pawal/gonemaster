# Basic01

Status: Final

## Purpose
- Determine whether the child zone exists and whether a parent zone can be identified from iterative authoritative responses.
- Detect inconsistent delegation or alias outcomes while traversing from the root toward the child zone.

## Preconditions And Inputs
- Preconditions:
  - A `zone.Zone` is provided with a non-empty name.
  - A recursor is available on the zone object.
- Required inputs:
  - Child zone name (`z.Name`).
  - Root server set from `Recursor.RootServers()`.
- Profile/config knobs that affect behavior:
  - `net.ipv4`: enables or disables IPv4 queries.
  - `net.ipv6`: enables or disables IPv6 queries.
  - Undelegated mode via fake addresses (`Recursor.HasFakeAddresses(child)`).

## Algorithm And Decision Flow
1. Emit `TEST_CASE_START`.
2. If child is root (`.`), emit `B01_CHILD_FOUND` and `B01_ROOT_HAS_NO_PARENT`, then emit `TEST_CASE_END` and return.
3. If child has fake addresses (undelegated context), emit `B01_CHILD_FOUND` and `B01_PARENT_DISREGARDED`, then emit `TEST_CASE_END` and return.
4. Start from root servers and iteratively probe with SOA/NS (and DNAME when needed), extending the intermediate name toward the child.
5. For each probed nameserver address:
   - Emit transport enable/disable tags (`IPV4_*`, `IPV6_*`) per rrtype (`SOA`, `NS`, `DNAME`) and skip queries on disabled transports.
   - Emit `B01_SERVER_ZONE_ERROR` when response requirements fail.
6. Collect parent/delegation observations and emit:
   - `B01_PARENT_FOUND` for discovered parents.
   - `B01_PARENT_UNDETERMINED` when multiple parent candidates exist.
   - `B01_PARENT_NOT_FOUND` when none exists.
7. Emit child/delegation status:
   - `B01_CHILD_FOUND` when delegation or authoritative SOA evidence exists.
   - `B01_INCONSISTENT_DELEGATION` for inconsistent parent-side results.
   - `B01_NO_CHILD` (normal mode) or `B01_CHILD_NOT_EXIST` (fake-address mode) when child evidence is absent.
8. Emit alias findings:
   - `B01_CHILD_IS_ALIAS` per detected DNAME target.
   - `B01_INCONSISTENT_ALIAS` when multiple alias targets are found.
9. Emit `TEST_CASE_END`.

### Mode Classification (steps 2-3)

{{% expand "Show diagram" %}}
{{< mermaid >}}
stateDiagram-v2
    [*] --> classify
    classify : determine mode
    classify --> rootCase : child is root
    classify --> fakeCase : fake addresses
    classify --> traverse : normal mode
    rootCase : root-zone tags
    fakeCase : fake-address tags
    traverse : iterative parent discovery
    rootCase --> [*]
    fakeCase --> [*]
    traverse --> [*]
{{< /mermaid >}}
{{% /expand %}}

### Per-Server Probe (step 5)

{{% expand "Show diagram" %}}
{{< mermaid >}}
stateDiagram-v2
    [*] --> nextHop
    nextHop : pick next NS address
    nextHop --> loopCheck
    loopCheck : iteration count
    loopCheck --> loopHit : over threshold
    loopCheck --> transport : under threshold
    loopHit : loop-protection tag
    transport : transport check per rrtype
    transport --> tagDis : disabled
    transport --> tagEn : enabled
    tagDis : transport-disabled tag
    tagEn : transport-enabled tag
    tagEn --> queryRR
    queryRR : SOA/NS/DNAME query
    queryRR --> respCheck
    respCheck : response validation
    respCheck --> tagErr : invalid
    respCheck --> extract : valid
    tagErr : server-zone-error tag
    extract : delegation and alias data
    loopHit --> [*]
    tagDis --> [*]
    tagErr --> [*]
    extract --> [*]
{{< /mermaid >}}
{{% /expand %}}

### Outcome Aggregation (steps 6-9)

{{% expand "Show diagram" %}}
{{< mermaid >}}
stateDiagram-v2
    [*] --> parentE
    parentE : parent outcome
    parentE --> pOne : one candidate
    parentE --> pMulti : multiple candidates
    parentE --> pNone : no candidate
    pOne : parent-found tag
    pMulti : parent-undetermined tag
    pNone : parent-not-found tag
    pOne --> childE
    pMulti --> childE
    pNone --> childE
    childE : child outcome
    childE --> cFound : delegation or SOA
    childE --> cIncon : inconsistent
    childE --> cAbsent : no evidence
    cFound : child-found tag
    cIncon : inconsistent-delegation tag
    cAbsent : no-child or not-exist tag
    cFound --> aliasE
    cIncon --> aliasE
    cAbsent --> aliasE
    aliasE : alias outcome
    aliasE --> aOne : single DNAME
    aliasE --> aMulti : multiple DNAMEs
    aliasE --> aNone : none
    aOne : child-is-alias tag
    aMulti : inconsistent-alias tag
    aNone : no alias tag
    aOne --> done
    aMulti --> done
    aNone --> done
    done : emit test-case-end
    done --> [*]
{{< /mermaid >}}
{{% /expand %}}

## Emitted Tags (Possible Set)
| Tag | Emitted when |
| --- | --- |
| `B01_CHILD_NOT_EXIST` | No delegation/SOA evidence exists and child is in fake-address mode. |
| `B01_CHILD_IS_ALIAS` | Child is identified as alias via DNAME evidence. |
| `B01_CHILD_FOUND` | Child existence is confirmed (root, undelegated, delegation, or authoritative SOA path). |
| `B01_INCONSISTENT_ALIAS` | More than one DNAME target was observed for child aliasing. |
| `B01_INCONSISTENT_DELEGATION` | Parent-side responses for child delegation are inconsistent. |
| `B01_NO_CHILD` | No delegation/SOA evidence exists in normal mode (non-fake-address). |
| `B01_PARENT_DISREGARDED` | Fake-address (undelegated) mode is active, so parent search is skipped. |
| `B01_PARENT_FOUND` | At least one parent zone candidate is identified. |
| `B01_PARENT_NOT_FOUND` | No parent zone candidate was identified from any probed nameserver response. |
| `B01_PARENT_UNDETERMINED` | Multiple parent zone candidates were identified. |
| `B01_ROOT_HAS_NO_PARENT` | Child zone is root (`.`). |
| `B01_SERVER_ZONE_ERROR` | SOA/NS response validation fails for a probed server/query name. |
| `IPV4_DISABLED` | IPv4 transport is disabled for queried rrtype. |
| `IPV4_ENABLED` | IPv4 transport is enabled for queried rrtype. |
| `IPV6_DISABLED` | IPv6 transport is disabled for queried rrtype. |
| `IPV6_ENABLED` | IPv6 transport is enabled for queried rrtype. |
| `LOOP_PROTECTION` | Internal traversal loop protection threshold is hit. |
| `TEST_CASE_END` | Testcase completion marker is emitted. |
| `TEST_CASE_START` | Testcase start marker is emitted. |

## Tag Arguments
| Tag | Argument key | Type | Meaning |
| --- | --- | --- | --- |
| `B01_CHILD_NOT_EXIST` | `domain` | `string` | Child zone name. |
| `B01_CHILD_IS_ALIAS` | `domain_child` | `string` | Child zone name. |
| `B01_CHILD_IS_ALIAS` | `domain_target` | `string` | Alias target name from DNAME. |
| `B01_CHILD_IS_ALIAS` | `servers` | `array<object>` | Structured nameserver list that returned the result. |
| `B01_CHILD_FOUND` | `domain` | `string` | Child zone name found. |
| `B01_INCONSISTENT_ALIAS` | `domain` | `string` | Child zone name with inconsistent alias targets. |
| `B01_INCONSISTENT_DELEGATION` | `domain_parent` | `string` | Parent zone candidate showing inconsistency. |
| `B01_INCONSISTENT_DELEGATION` | `domain_child` | `string` | Child zone name. |
| `B01_INCONSISTENT_DELEGATION` | `servers` | `array<object>` | Structured nameserver list tied to inconsistency. |
| `B01_NO_CHILD` | `domain_child` | `string` | Child zone name. |
| `B01_NO_CHILD` | `domain_super` | `string` | Next-higher domain suggested for testing. |
| `B01_PARENT_DISREGARDED` | `-` | `-` | No arguments. |
| `B01_PARENT_FOUND` | `domain` | `string` | Parent zone name candidate. |
| `B01_PARENT_FOUND` | `servers` | `array<object>` | Structured nameserver list returning parent evidence. |
| `B01_PARENT_NOT_FOUND` | `-` | `-` | No arguments. |
| `B01_PARENT_UNDETERMINED` | `servers` | `array<object>` | Structured nameserver list across competing parents. |
| `B01_ROOT_HAS_NO_PARENT` | `-` | `-` | No arguments. |
| `B01_SERVER_ZONE_ERROR` | `query_name` | `string` | Queried owner name that failed validation. |
| `B01_SERVER_ZONE_ERROR` | `rrtype` | `string` | Queried rrtype (`SOA` or `NS`). |
| `B01_SERVER_ZONE_ERROR` | `ns` | `string` | Nameserver identity (`ns` name only; use `address` for IP). |
| `B01_SERVER_ZONE_ERROR` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `IPV4_DISABLED` | `ns` | `string` | Nameserver identity (`ns` name only; use `address` for IP). |
| `IPV4_DISABLED` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `IPV4_DISABLED` | `rrtype` | `string` | rrtype skipped due to transport disable. |
| `IPV4_ENABLED` | `ns` | `string` | Nameserver identity (`ns` name only; use `address` for IP). |
| `IPV4_ENABLED` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `IPV4_ENABLED` | `rrtype` | `string` | rrtype queried over enabled transport. |
| `IPV6_DISABLED` | `ns` | `string` | Nameserver identity (`ns` name only; use `address` for IP). |
| `IPV6_DISABLED` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `IPV6_DISABLED` | `rrtype` | `string` | rrtype skipped due to transport disable. |
| `IPV6_ENABLED` | `ns` | `string` | Nameserver identity (`ns` name only; use `address` for IP). |
| `IPV6_ENABLED` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `IPV6_ENABLED` | `rrtype` | `string` | rrtype queried over enabled transport. |
| `LOOP_PROTECTION` | `caller` | `string` | Internal caller name that hit loop protection. |
| `LOOP_PROTECTION` | `child_zone_name` | `string` | Child zone name under test. |
| `LOOP_PROTECTION` | `zone_name` | `string` | Current loop zone name state. |
| `LOOP_PROTECTION` | `intermediate_query_name` | `string` | Intermediate query name at stop point. |
| `TEST_CASE_END` | `testcase` | `string` | Testcase display name (`Basic01`). |
| `TEST_CASE_START` | `testcase` | `string` | Testcase display name (`Basic01`). |

## Severity Levels Per Tag
| Tag | Level | Notes |
| --- | --- | --- |
| `B01_CHILD_NOT_EXIST` | `INFO` | Default from `share/profile.json`. |
| `B01_CHILD_IS_ALIAS` | `NOTICE` | Default from `share/profile.json`. |
| `B01_CHILD_FOUND` | `INFO` | Default from `share/profile.json`. |
| `B01_INCONSISTENT_ALIAS` | `ERROR` | Default from `share/profile.json`. |
| `B01_INCONSISTENT_DELEGATION` | `ERROR` | Default from `share/profile.json`. |
| `B01_NO_CHILD` | `ERROR` | Default from `share/profile.json`. |
| `B01_PARENT_DISREGARDED` | `INFO` | Default from `share/profile.json`. |
| `B01_PARENT_FOUND` | `INFO` | Default from `share/profile.json`. |
| `B01_PARENT_NOT_FOUND` | `WARNING` | Default from `share/profile.json`. |
| `B01_PARENT_UNDETERMINED` | `WARNING` | Default from `share/profile.json`. |
| `B01_ROOT_HAS_NO_PARENT` | `INFO` | Default from `share/profile.json`. |
| `B01_SERVER_ZONE_ERROR` | `DEBUG` | Default from `share/profile.json`. |
| `IPV4_DISABLED` | `DEBUG` | Default from `share/profile.json`. |
| `IPV4_ENABLED` | `DEBUG` | Default from `share/profile.json`. |
| `IPV6_DISABLED` | `DEBUG` | Default from `share/profile.json`. |
| `IPV6_ENABLED` | `DEBUG` | Default from `share/profile.json`. |
| `LOOP_PROTECTION` | `DEBUG2` | Default from `share/profile.json`. |
| `TEST_CASE_END` | `DEBUG` | Default from `share/profile.json`. |
| `TEST_CASE_START` | `DEBUG` | Default from `share/profile.json`. |

## Differences From Upstream
- Upstream reference: [`basic01.md`](../../upstream/tests/Basic-TP/basic01.md)
- Differences (Upstream vs Gonemaster):
  - Upstream: documents `B01_NO_CHILD` for the non-existing-child outcome. Gonemaster: also emits `B01_CHILD_NOT_EXIST` in fake-address mode.
  - Upstream: testcase summary does not list transport debug tags or `LOOP_PROTECTION`. Gonemaster: emits `IPV4_*`, `IPV6_*`, and `LOOP_PROTECTION`.
- Potential upstream report:
  - `yes`
- If yes, include:
  - Upstream expected behavior: Child-not-exist path is described with `B01_NO_CHILD` only, and summary does not include transport or loop-protection tags.
  - Gonemaster observed behavior: `B01_CHILD_NOT_EXIST`, transport tags, and `LOOP_PROTECTION` are possible emissions.
  - evidence: `engine/test/basic/basic.go` (`Basic01`, `ipDisabledMessage`, `ipEnabledMessage`).
  - report status: `not filed`

## Implementation Notes

The following behaviors are implementation choices, not mandated by RFC 1034/1035:

- **Traversal strategy**: The testcase probes iteratively from root servers using SOA, NS, and DNAME queries, extending the intermediate name toward the child zone at each step.  The DNS protocol specifies the resolution model but does not define how a testcase tool should walk the hierarchy.
- **Sorted `servers` arguments**: Nameserver lists passed in tag arguments are sorted before joining with `;`.  Deterministic ordering simplifies reproducible output but is not a protocol requirement.
- **Loop protection threshold**: Traversal stops at a fixed internal limit and emits `LOOP_PROTECTION`.  No DNS standard defines a specific iteration bound; the limit is a defensive implementation choice.

## Edge Cases And Limitations
- A missing recursor causes testcase execution error before completion.
- Loop-protection fallback is defensive; when triggered it logs `LOOP_PROTECTION` and terminates the testcase early.
- Child existence outcomes differ between normal and fake-address modes (`B01_NO_CHILD` vs `B01_CHILD_NOT_EXIST`).
- Nameserver list argument order is deterministic because lists are sorted before join.
