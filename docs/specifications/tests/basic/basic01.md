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
   - For a child below a TLD (two or more labels) the root servers are not the parent, so the root level is walked until one root server refers onwards or answers for the child itself, and the root NS names in the `NS` answers of the probed root servers are not resolved. A root server that fails is reported and the next one is tried.
   - If the walk then ends without a parent or without child evidence (no delegation, authoritative SOA or DNAME), it continues with the remaining root servers and re-queues every zone they add servers to, so the outcome is the one a walk through every root server gives.
   - Every zone of the walk is the child or an ancestor of it. The shallowest queued zone is walked first, so a referral never adds nameservers to a zone already walked. A zone is walked through every nameserver the referrals to it name and every nameserver its own `NS` answers name.
   - Each zone is visited at most once per root path. A visit beyond 2 × (labels of the child + 1) emits `LOOP_PROTECTION` and ends the walk; steps 6 to 9 then run on the observations gathered so far.
5. For each probed nameserver address:
   - Emit transport enable/disable tags (`IPV4_*`, `IPV6_*`) per rrtype (`SOA`, `NS`, `DNAME`) and skip queries on disabled transports.
   - Emit `B01_SERVER_ZONE_ERROR` when response requirements fail, with `reason` naming the first requirement the response fails and `rcode` holding its RCODE.
   - On `NXDOMAIN`+AA at an intermediate name (not the child itself), additionally probe SOA for the child name on the same nameserver. If that probe returns a referral whose NS records are owned by the child or by an ancestor of the child below the zone being walked, the nameserver is contradicting itself (it denies an ancestor name but still has a deeper delegation, violating RFC 8020 section 3.1). A referral for the child counts as a valid delegation observation (added to `delegationFound`). A referral for an ancestor of the child queues the nameservers it names, and the walk continues at that zone cut; the nameserver is not recorded as a parent. In both cases the offending `(NS, intermediate name)` pair is recorded for the diagnostic tag and the NS is not added to `aaNXDomain`.
6. Collect parent/delegation observations and emit:
   - `B01_PARENT_FOUND` for discovered parents.
   - `B01_PARENT_UNDETERMINED` when multiple parent candidates exist.
   - `B01_PARENT_NOT_FOUND` when none exists.
   - `B01_PARENT_NXDOMAIN_HIDES_DELEGATION` for each NS that returned NXDOMAIN at an intermediate name but also has a delegation at the child or at an ancestor of the child below that name's zone.
7. Emit child/delegation status:
   - `B01_CHILD_FOUND` when delegation or authoritative SOA evidence exists (including delegations recovered via the NXDOMAIN-contradiction probe).
   - `B01_INCONSISTENT_DELEGATION` for inconsistent parent-side results.
   - `B01_NO_CHILD` when child evidence is absent. `domain_super` is the parent zone when exactly one was found; when none was found, the deepest zone the walk was referred to or proved; in every other case the next-higher name.
8. Emit alias findings:
   - `B01_CHILD_IS_ALIAS` per detected DNAME target.
   - `B01_INCONSISTENT_ALIAS` when multiple alias targets are found.
9. Emit `TEST_CASE_END`.

### Mode Classification (steps 2-3)

{{% expand "Show diagram" %}}
```
z.Name
 +- "."                              -> B01_CHILD_FOUND
 |                                      B01_ROOT_HAS_NO_PARENT
 |                                      emit TEST_CASE_END and return
 +- Recursor.HasFakeAddresses(child) -> B01_CHILD_FOUND
 |                                      B01_PARENT_DISREGARDED
 |                                      emit TEST_CASE_END and return
 +- otherwise                        -> normal-mode traversal from root
```
{{% /expand %}}

### Per-Server Probe (step 5)

{{% expand "Show diagram" %}}
```
For each remaining label, shallowest first (from "." down toward child):
   +- zone visits > 2 * (labels of child + 1)   -> LOOP_PROTECTION, end the walk
   for each NS listed for that label, including NS appended during the pass:
     +- label is "." on the one root path (child has two or more labels)
        AND (a deeper label is queued OR parentFound non-empty)
                                                   -> stop probing root servers
     +- already handled (zone, addr)?              -> skip
     +- transport disabled for SOA/NS/DNAME        -> IPV4_DISABLED / IPV6_DISABLED, skip
     +- otherwise                                  -> IPV4_ENABLED / IPV6_ENABLED
                                                     |
                                                     v
        query SOA at zoneName
         +- error / no Msg / RCODE != NOERROR / !AA / !=1 SOA in answer / SOA owner != name
              -> B01_SERVER_ZONE_ERROR (query_type=SOA, reason, rcode), skip ns
        query NS  at zoneName
         +- error / no Msg / RCODE != NOERROR / !AA / no NS / NS owner != name
              -> B01_SERVER_ZONE_ERROR (query_type=NS, reason, rcode),  skip ns
         +- success
              -> extract NS names + A/AAAA glue (recurse to resolve missing glue),
                 append servers not yet listed for zoneName to its list, probed in this pass
              -> at "." on the one root path: drop the NS names, resolve nothing

   inner: prepend labels of child name to intermediate (parent walk)
     +- intermediate has all labels of child  -> stop inner loop

     query SOA at intermediate
      +- error / no Msg                      -> B01_SERVER_ZONE_ERROR, skip ns
      +- NOERROR + AA + 1 SOA at intermediate
      |    +- intermediate == child          -> parentFound, aaSOA
      |    +- else: query NS at intermediate
      |                +- invalid response   -> B01_SERVER_ZONE_ERROR, skip ns
      |                +- valid              -> extract NS+glue, enqueue, continue inner
      +- NXDOMAIN + AA
      |    +- intermediate == child          -> parentFound, aaNXDomain
      |    +- intermediate != child: probe SOA at child on same NS
      |         +- referral with NS for child -> parentFound, delegationFound,
      |         |                                nxdomainHidesDelegation
      |         |                                (NOT aaNXDomain)
      |         +- referral with NS for an ancestor of child below the walked zone
      |         |                             -> enqueue its NS, nxdomainHidesDelegation
      |         |                                (NOT parentFound, NOT aaNXDomain)
      |         +- otherwise                  -> parentFound, aaNXDomain
      +- IsRedirect with NS in authority for intermediate
      |    +- intermediate == child          -> parentFound, delegationFound
      |    +- else                           -> extract NS+glue, enqueue
      +- NOERROR + AA, intermediate == child
      |    +- CNAME in answer for child      -> parentFound, aaCNAME
      |    +- DNAME query: AA+NOERROR+1 DNAME-> parentFound, aaDname[target]
      |    +- otherwise                      -> parentFound, aaNodata
      +- !AA + NOERROR + NS in authority, answer only CNAME (and RRSIG) with a CNAME for child
      |                                      -> parentFound, cnameWithReferral
      +- any other shape                     -> B01_SERVER_ZONE_ERROR (query_type=SOA, reason, rcode)

   label queue empty on the one root path
   AND (parentFound empty OR delegationFound, aaSOA and aaDname all empty)
     -> leave the one root path, re-queue ".", probe the remaining root servers,
        re-queue every label they add servers to
```
{{% /expand %}}

### Outcome Aggregation (steps 6-9)

{{% expand "Show diagram" %}}
```
1. Parent
     parentFound non-empty:
       per (parent domain, ns set)         -> B01_PARENT_FOUND
       len(parentFound) > 1                -> B01_PARENT_UNDETERMINED (merged ns set)
     parentFound empty                     -> B01_PARENT_NOT_FOUND
     nxdomainHidesDelegation non-empty:
       per offending NS                    -> B01_PARENT_NXDOMAIN_HIDES_DELEGATION
                                              (ns, address, query_name, domain_child)

2. Child
     delegationFound non-empty OR aaSOA non-empty
       -> B01_CHILD_FOUND
          if not fake-addresses:
            for each parent observed in aaNXDomain / aaCNAME / cnameWithReferral
            / aaNodata / aaDname:
              -> B01_INCONSISTENT_DELEGATION (domain_parent, domain_child, servers)
     delegationFound empty AND aaSOA empty AND aaDname empty
       -> B01_NO_CHILD (domain_child, domain_super)
          domain_super: one parentFound zone -> that zone
                        no parentFound zone  -> the deepest zone the walk was
                                                referred to or proved, if any
                        otherwise            -> z.Name.NextHigher()

3. Alias
     aaDname non-empty:
       per DNAME target                    -> B01_CHILD_IS_ALIAS (domain_child, domain_target, servers)
       len(aaDname) > 1                    -> B01_INCONSISTENT_ALIAS (domain)

4. emit TEST_CASE_END
```
{{% /expand %}}

## Emitted Tags (Possible Set)
| Tag | Emitted when |
| --- | --- |
| `B01_CHILD_IS_ALIAS` | Child is identified as alias via DNAME evidence. |
| `B01_CHILD_FOUND` | Child existence is confirmed (root, undelegated, delegation, or authoritative SOA path). |
| `B01_INCONSISTENT_ALIAS` | More than one DNAME target was observed for child aliasing. |
| `B01_INCONSISTENT_DELEGATION` | Parent-side responses for child delegation are inconsistent. |
| `B01_NO_CHILD` | No delegation/SOA/DNAME evidence exists in normal mode (non-fake-address). Mutually exclusive with `B01_CHILD_IS_ALIAS`. |
| `B01_PARENT_DISREGARDED` | Fake-address (undelegated) mode is active, so parent search is skipped. |
| `B01_PARENT_FOUND` | At least one parent zone candidate is identified. |
| `B01_PARENT_NOT_FOUND` | No parent zone candidate was identified from any probed nameserver response. |
| `B01_PARENT_NXDOMAIN_HIDES_DELEGATION` | A nameserver on the path to the child returned authoritative NXDOMAIN for an intermediate empty non-terminal but also has a delegation at the child name or at an ancestor of it. The contradiction violates RFC 8020 section 3.1. The zone is still tested through the referral. |
| `B01_PARENT_UNDETERMINED` | Multiple parent zone candidates were identified. |
| `B01_ROOT_HAS_NO_PARENT` | Child zone is root (`.`). |
| `B01_SERVER_ZONE_ERROR` | SOA/NS response validation fails for a probed server/query name. |
| `CNAME_CHAIN_TOO_LONG` | CNAME chain across recursion hops while resolving a discovered NS A/AAAA exceeds `CNAMEMaxChainLength`. |
| `CNAME_TARGET_UNRESOLVED` | CNAME chain while resolving a discovered NS A/AAAA forms a loop, breaks, or fails qtype match (RFC 1034 section 5.2.2). |
| `CNAME_TOO_MANY_RECORDS` | A single answer for a discovered NS A/AAAA carries more than `CNAMEMaxRecords` distinct CNAME RRs. |
| `IPV4_DISABLED` | IPv4 transport is disabled for queried rrtype. |
| `IPV4_ENABLED` | IPv4 transport is enabled for queried rrtype. |
| `IPV6_DISABLED` | IPv6 transport is disabled for queried rrtype. |
| `IPV6_ENABLED` | IPv6 transport is enabled for queried rrtype. |
| `LOOP_PROTECTION` | The walk visits more zones than 2 × (labels of the child + 1). |
| `TEST_CASE_END` | Testcase completion marker is emitted. |
| `TEST_CASE_START` | Testcase start marker is emitted. |

## Tag Arguments
| Tag | Argument key | Type | Meaning |
| --- | --- | --- | --- |
| `B01_CHILD_IS_ALIAS` | `domain_child` | `string` | Child zone name. |
| `B01_CHILD_IS_ALIAS` | `domain_target` | `string` | Alias target name from DNAME. |
| `B01_CHILD_IS_ALIAS` | `servers` | `array<object>` | Structured nameserver list that returned the result. |
| `B01_CHILD_FOUND` | `domain` | `string` | Child zone name found. |
| `B01_INCONSISTENT_ALIAS` | `domain` | `string` | Child zone name with inconsistent alias targets. |
| `B01_INCONSISTENT_DELEGATION` | `domain_parent` | `string` | Parent zone candidate showing inconsistency. |
| `B01_INCONSISTENT_DELEGATION` | `domain_child` | `string` | Child zone name. |
| `B01_INCONSISTENT_DELEGATION` | `servers` | `array<object>` | Structured nameserver list tied to inconsistency. |
| `B01_NO_CHILD` | `domain_child` | `string` | Child zone name. |
| `B01_NO_CHILD` | `domain_super` | `string` | Zone suggested for testing: the parent zone when exactly one was found; when none was found, the deepest zone the walk was referred to or proved; in every other case the next-higher name. |
| `B01_PARENT_DISREGARDED` | `-` | `-` | No arguments. |
| `B01_PARENT_FOUND` | `domain` | `string` | Parent zone name candidate. |
| `B01_PARENT_FOUND` | `servers` | `array<object>` | Structured nameserver list returning parent evidence. |
| `B01_PARENT_NOT_FOUND` | `-` | `-` | No arguments. |
| `B01_PARENT_NXDOMAIN_HIDES_DELEGATION` | `ns` | `string` | Nameserver identity (`ns` name only; use `address` for IP). |
| `B01_PARENT_NXDOMAIN_HIDES_DELEGATION` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `B01_PARENT_NXDOMAIN_HIDES_DELEGATION` | `query_name` | `string` | Intermediate name where the nameserver returned NXDOMAIN. |
| `B01_PARENT_NXDOMAIN_HIDES_DELEGATION` | `domain_child` | `string` | Child zone name; the same NS refers a query for it to the child or to an ancestor of the child. |
| `B01_PARENT_UNDETERMINED` | `servers` | `array<object>` | Structured nameserver list across competing parents. |
| `B01_ROOT_HAS_NO_PARENT` | `-` | `-` | No arguments. |
| `B01_SERVER_ZONE_ERROR` | `query_name` | `string` | Queried owner name that failed validation. |
| `B01_SERVER_ZONE_ERROR` | `query_type` | `string` | Queried rrtype (`SOA` or `NS`). |
| `B01_SERVER_ZONE_ERROR` | `ns` | `string` | Nameserver identity (`ns` name only; use `address` for IP). |
| `B01_SERVER_ZONE_ERROR` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `B01_SERVER_ZONE_ERROR` | `reason` | `string` | First requirement the response fails: `no_response` (no response, or one that cannot be read), `rcode` (an RCODE other than `NOERROR`, or an authoritative `NXDOMAIN` for a zone of the walk), `referral` (a referral without the AA bit, naming no NS for the queried name), `not_authoritative` (any other response without the AA bit), `soa_records` (not exactly one SOA record in the answer section, or one of another owner), `ns_records` (no NS record, or an NS record of another owner). |
| `B01_SERVER_ZONE_ERROR` | `rcode` | `string` | RCODE mnemonic of the response; absent when `reason` is `no_response`. |
| `CNAME_CHAIN_TOO_LONG` | `query_name` | `string` | The qname whose CNAME chain exceeded the depth bound. |
| `CNAME_TARGET_UNRESOLVED` | `query_name` | `string` | The qname whose CNAME target could not be resolved. |
| `CNAME_TARGET_UNRESOLVED` | `cname_target` | `string` | The last attempted CNAME target. |
| `CNAME_TOO_MANY_RECORDS` | `query_name` | `string` | The qname whose answer carried too many CNAME RRs. |
| `IPV4_DISABLED` | `ns` | `string` | Nameserver identity (`ns` name only; use `address` for IP). |
| `IPV4_DISABLED` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `IPV4_DISABLED` | `query_type` | `string` | Query type skipped due to transport disable. |
| `IPV4_ENABLED` | `ns` | `string` | Nameserver identity (`ns` name only; use `address` for IP). |
| `IPV4_ENABLED` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `IPV4_ENABLED` | `query_type` | `string` | Query type sent over enabled transport. |
| `IPV6_DISABLED` | `ns` | `string` | Nameserver identity (`ns` name only; use `address` for IP). |
| `IPV6_DISABLED` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `IPV6_DISABLED` | `query_type` | `string` | Query type skipped due to transport disable. |
| `IPV6_ENABLED` | `ns` | `string` | Nameserver identity (`ns` name only; use `address` for IP). |
| `IPV6_ENABLED` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `IPV6_ENABLED` | `query_type` | `string` | Query type sent over enabled transport. |
| `LOOP_PROTECTION` | `caller` | `string` | Internal caller name that hit loop protection. |
| `LOOP_PROTECTION` | `child_zone_name` | `string` | Child zone name under test. |
| `LOOP_PROTECTION` | `zone_name` | `string` | Zone the walk was about to visit. |
| `LOOP_PROTECTION` | `intermediate_query_name` | `string` | Name of the next query, the apex of `zone_name`. |
| `TEST_CASE_END` | `testcase` | `string` | Testcase display name (`Basic01`). |
| `TEST_CASE_START` | `testcase` | `string` | Testcase display name (`Basic01`). |

## Severity Levels Per Tag
| Tag | Level | Notes |
| --- | --- | --- |
| `B01_CHILD_IS_ALIAS` | `NOTICE` | Default from `share/profile.json`. |
| `B01_CHILD_FOUND` | `INFO` | Default from `share/profile.json`. |
| `B01_INCONSISTENT_ALIAS` | `ERROR` | Default from `share/profile.json`. |
| `B01_INCONSISTENT_DELEGATION` | `ERROR` | Default from `share/profile.json`. |
| `B01_NO_CHILD` | `ERROR` | Default from `share/profile.json`. |
| `B01_PARENT_DISREGARDED` | `INFO` | Default from `share/profile.json`. |
| `B01_PARENT_FOUND` | `INFO` | Default from `share/profile.json`. |
| `B01_PARENT_NOT_FOUND` | `WARNING` | Default from `share/profile.json`. |
| `B01_PARENT_NXDOMAIN_HIDES_DELEGATION` | `ERROR` | Default from `share/profile.json`. |
| `B01_PARENT_UNDETERMINED` | `WARNING` | Default from `share/profile.json`. |
| `B01_ROOT_HAS_NO_PARENT` | `INFO` | Default from `share/profile.json`. |
| `B01_SERVER_ZONE_ERROR` | `DEBUG` | Default from `share/profile.json`. |
| `CNAME_CHAIN_TOO_LONG` | `ERROR` | Default from `share/profile.json`. |
| `CNAME_TARGET_UNRESOLVED` | `ERROR` | Default from `share/profile.json`. |
| `CNAME_TOO_MANY_RECORDS` | `ERROR` | Default from `share/profile.json`. |
| `IPV4_DISABLED` | `DEBUG2` | Default from `share/profile.json`. |
| `IPV4_ENABLED` | `DEBUG2` | Default from `share/profile.json`. |
| `IPV6_DISABLED` | `DEBUG2` | Default from `share/profile.json`. |
| `IPV6_ENABLED` | `DEBUG2` | Default from `share/profile.json`. |
| `LOOP_PROTECTION` | `DEBUG2` | Default from `share/profile.json`. |
| `TEST_CASE_END` | `DEBUG` | Default from `share/profile.json`. |
| `TEST_CASE_START` | `DEBUG` | Default from `share/profile.json`. |

## Differences From Upstream
- Upstream reference: `docs/public/specifications/tests/Basic-TP/basic01.md` (develop, 2026-11-23), and the open specification change zonemaster#1488.
- Differences (Upstream vs Gonemaster):
  - Upstream: the summary lists no transport, CNAME or loop-protection tags. Gonemaster: emits `IPV4_*`, `IPV6_*`, `CNAME_*` for the address lookup of an NS name, and `LOOP_PROTECTION`.
  - Upstream: probes every root server for every child. Gonemaster: for a child below a TLD, follows one working root server and walks the others only when that path finds no parent or no child, so `B01_SERVER_ZONE_ERROR` is reported only for root servers that were probed.
  - Upstream: `B01_SERVER_ZONE_ERROR` carries the query and the nameserver. Gonemaster: adds `reason` and `rcode`.
  - Upstream: no diagnostic for parent nameservers that violate RFC 8020 by returning NXDOMAIN at an intermediate empty non-terminal while still delegating a deeper child; the child is treated as nonexistent. Gonemaster: emits `B01_PARENT_NXDOMAIN_HIDES_DELEGATION`, treats the directly-observed referral as delegation evidence, emits `B01_CHILD_FOUND`, and lets the rest of the test suite run. zonemaster#1488 adds `B01_UNEXPECTED_NXDOMAIN` for the same fault (`DIV-B01-RFC8020-NXDOMAIN`).
  - Upstream (step 9.2): one `B01_INCONSISTENT_DELEGATION` with the combined server set. Gonemaster: one per parent zone that has servers answering without a delegation.
  - Upstream (step 10): `domain_super` is the child with its first label removed. Gonemaster: the parent zone when exactly one was found, otherwise the deepest zone the walk reached, otherwise the next-higher name (`DIV-B01-DOMAIN-SUPER`).
  - Upstream (step 10): `B01_NO_CHILD` is emitted whenever neither a delegation nor an authoritative SOA was found, beside `B01_CHILD_IS_ALIAS` for a DNAME. Gonemaster: emits no `B01_NO_CHILD` when a DNAME was found (`DIV-B01-DNAME-NO-CHILD`).
  - Upstream: follows every NS name and address. Gonemaster: cuts the NS names of one response at 32 and the addresses of one name at 16, and a run queries at most 512 addresses; a cut logs `LIMIT_REACHED`.
- Potential upstream report:
  - `yes`
- If yes, include:
  - Upstream expected behavior: a child below an empty non-terminal for which a parent nameserver returns an authoritative NXDOMAIN is reported as nonexistent.
  - Gonemaster observed behavior: the child is found through the referral the same nameserver returns for it, and `B01_PARENT_NXDOMAIN_HIDES_DELEGATION` names the nameserver.
  - evidence: `engine/test/basic/basic01_test.go` (`TestBasic01IntermediateNXDomain`, `TestBasic01AncestorNXDomainHidesParentDelegation`).
  - report status: `filed` (zonemaster#1484, opened and closed upstream; specification change in zonemaster#1488, open)

## Implementation Notes

The following behaviors are implementation choices, not mandated by RFC 1034 or RFC 1035:

- **Traversal strategy**: The testcase probes iteratively from root servers using SOA, NS, and DNAME queries, extending the intermediate name toward the child zone at each step.  The DNS protocol specifies the resolution model but does not define how a testcase tool should walk the hierarchy.
- **One root path below a TLD**: Every root server is probed only when the roots are the parent (a TLD child) or when one root path finds nothing.  Root server faults are a root or TLD issue, not one the owner of a deeper zone can act on.
- **Sorted `servers` arguments**: Nameserver lists in tag arguments are sorted by name and address. Deterministic ordering makes output reproducible and is not a protocol requirement.
- **Loop protection threshold**: The walk stops after 2 × (labels of the child + 1) zone visits and emits `LOOP_PROTECTION`. A correct walk stays within that bound; the guard protects against a queueing fault. No DNS standard defines the bound.
- **Fan-out bounds**: At each zone of the walk, the NS names of one NS response and the addresses of each name are cut after the bounds `ns_names_per_zone` (32) and `addresses_per_ns` (16) of [Fan-out Limits](../../nameserver-resolution.md#fan-out-limits), in sorted order; a cut logs `LIMIT_REACHED`. RFC 1034 section 5.3.3 requires the work to be bounded and names no value.

## Edge Cases And Limitations
- A missing recursor causes testcase execution error before completion.
- When `LOOP_PROTECTION` ends the walk, the outcome rests on the zones visited before it; a child the walk did not reach is reported with `B01_NO_CHILD`.
- A zone of the walk with more than 32 NS names in one response, or an NS name with more than 16 addresses, is walked through the first names and addresses in sorted order only; servers past the cut are neither probed nor reported.
- With fake addresses the walk does not run, so child existence is not judged here: the testcase emits `B01_CHILD_FOUND` and `B01_PARENT_DISREGARDED` only.
- With one parent, or with none, `domain_super` is the closest enclosing zone the walk reached, not a name derived from the child. Below a missing intermediate name, below an intermediate name a nameserver fails to answer for, and for a reverse name under `ip6.arpa` (RFC 3596 section 2.5), the next-higher name is not a zone. With several parent candidates `domain_super` is the next-higher name.
- When a parent nameserver returns authoritative NXDOMAIN at an intermediate empty non-terminal (RFC 8020 violation) but also has a delegation at the child, the per-label SOA walk would normally stop on that NS without ever reaching the child. Basic01 issues one extra SOA probe at the child name; if that probe returns a referral for the child, the directly-observed delegation is treated as valid evidence (added to `delegationFound`, so `B01_CHILD_FOUND` fires and downstream test cases run against the real child nameservers), and `B01_PARENT_NXDOMAIN_HIDES_DELEGATION` is emitted to flag the contradiction. If the probe returns a referral for a zone between the walked zone and the child, the walk continues at that zone's nameservers and the same tag is emitted; no further query is sent to the contradicting nameserver. Ordinary recursive resolvers reach the data the same way because they typically query only the child name and follow the referral directly.
