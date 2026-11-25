# Delegation01

Status: Final

## Purpose
- Validate that delegation-side and child-side nameserver sets meet minimum-count requirements overall and per IP family.
- Apply the per-family minimum of RFC 10001 section 4.1: "at least two IPv4-reachable and two IPv6-reachable name servers MUST be configured for a zone. A single name server that is reachable over both IPv4 and IPv6 counts once per address family."
- Flag each nameserver name that resolves in one address family only (RFC 10001 section 4.1: all names used in NS RRs RECOMMENDED to have an A and AAAA record set).
- Flag any in-domain delegation NS name that the parent referral ships without glue, or without glue of a family the child serves for that name (RFC 10001 section 3.1, RFC 9471 section 3.1).

## Preconditions And Inputs
- Preconditions:
  - A `zone.Zone` object is available.
- Required inputs:
  - Delegation names from [`z.GlueNames(ctx)`](../../nameserver-resolution.md#gluenames).
  - Child NS names from [`z.ApexNSNames(ctx)`](../../nameserver-resolution.md#apexnsnames).
  - Delegation nameserver addresses from [`GlueNameservers`](../../nameserver-resolution.md#gluenameservers).
  - Child nameserver addresses from [`ApexNameservers`](../../nameserver-resolution.md#apexnameservers).
  - Raw delegation items from [`DelegationNameservers`](../../nameserver-resolution.md#delegationnameservers), which carry in-domain NS names without an address when the referral has no glue for them.
- Profile/config knobs that affect behavior:
  - No direct profile knob in this testcase.
  - Minimum required nameserver count is fixed by `constants.MinimumNumberOfNameservers` (two: RFC 1034 section 4.1 overall, RFC 10001 section 4.1 per IP family).
- No step sends a DNS query of its own; every step reads views that nameserver discovery has already resolved.

## Algorithm And Decision Flow
1. Emit `TEST_CASE_START`.
2. Get delegation NS names ([`z.GlueNames`](../../nameserver-resolution.md#gluenames)), sort names, and emit one of:
   - `ENOUGH_NS_DEL` when count is at least the minimum.
   - `NOT_ENOUGH_NS_DEL` otherwise.
3. Get child NS names ([`z.ApexNSNames`](../../nameserver-resolution.md#apexnsnames)), sort names, and emit one of:
   - `ENOUGH_NS_CHILD` when count is at least the minimum.
   - `NOT_ENOUGH_NS_CHILD` otherwise.
4. Get child addressed NS ([`ApexNameservers`](../../nameserver-resolution.md#apexnameservers)), split by IP family, count unique NS names per family, and emit per family:
   - IPv4: `ENOUGH_IPV4_NS_CHILD`, `NOT_ENOUGH_IPV4_NS_CHILD`, or `NO_IPV4_NS_CHILD`.
   - IPv6: `ENOUGH_IPV6_NS_CHILD`, `NOT_ENOUGH_IPV6_NS_CHILD`, or `NO_IPV6_NS_CHILD`.
5. Get delegation addressed NS ([`GlueNameservers`](../../nameserver-resolution.md#gluenameservers)), split by IP family, count unique NS names per family, and emit per family:
   - IPv4: `ENOUGH_IPV4_NS_DEL`, `NOT_ENOUGH_IPV4_NS_DEL`, or `NO_IPV4_NS_DEL`.
   - IPv6: `ENOUGH_IPV6_NS_DEL`, `NOT_ENOUGH_IPV6_NS_DEL`, or `NO_IPV6_NS_DEL`.
6. Over the union of the addressed NS from steps 4 and 5, collect the address families of each unique lowercased NS name. For each name, in sorted order:
   - IPv4 addresses only: emit `NS_NAME_NO_IPV6_ADDR`.
   - IPv6 addresses only: emit `NS_NAME_NO_IPV4_ADDR`.
   - Both families: no tag.
   A name without any address is not part of the union and gets no tag here.
7. Get raw delegation items ([`DelegationNameservers`](../../nameserver-resolution.md#delegationnameservers)). For each in-domain NS name, in sorted order, collect the glue families from its items and the child families from the step 4 view, then emit at most one of:
   - No item for the name has an address: `IN_DOMAIN_GLUE_MISSING`.
   - The glue has no IPv4 address and the child view holds an IPv4 address for the name: `IN_DOMAIN_GLUE_MISSING_IPV4`.
   - The glue has no IPv6 address and the child view holds an IPv6 address for the name: `IN_DOMAIN_GLUE_MISSING_IPV6`.
   - Otherwise no tag.
   Not-in-domain names are out of scope here.
8. Emit `TEST_CASE_END`.

### Per-Name Address Families (step 6)

RFC 10001 section 3.1 names "No A/AAAA records for NS names" as a cause of name space partitioning, and section 4.1 RECOMMENDS an A and an AAAA record set for every name used in NS RRs. Both sides are read, so a name counts as dual-stack when either view shows the missing family.

{{% expand "Show diagram" %}}
```
families = {}
for ns in GlueNameservers(z) + ApexNameservers(z):
   families[lower(ns.name)] += family(ns.address)

for name in sort(keys(families)):
   {IPv4}       -> NS_NAME_NO_IPV6_ADDR (ns)
   {IPv6}       -> NS_NAME_NO_IPV4_ADDR (ns)
   {IPv4, IPv6} -> no tag
```
{{% /expand %}}

### In-Domain Glue Checks (step 7)

A delegation NS name that lies inside the zone (in-domain, RFC 9499 section 7) can only be reached if the parent referral includes its address as glue; otherwise a resolver holding only the referral cannot resolve the name (the address lives inside the very zone the name serves). RFC 10001 section 3.1 requires glue of both families, and a resolver limited to one family needs glue of that family. The raw referral view from `DelegationNameservers` is used here rather than [`GlueNameservers`](../../nameserver-resolution.md#gluenameservers), because the latter resolves the name through the recursor and would mask the missing glue.

A per-family glue tag requires child-side evidence. RFC 10001 section 3.1 separates "Missing glue" (the parent lacks glue the child could supply) from "No A/AAAA records for NS names" (no record exists to glue). The step 4 view resolves each name from the root, so for an in-domain name it holds the child's own A and AAAA records; a family absent there is reported by step 6, not as missing glue.

{{% expand "Show diagram" %}}
```
items = DelegationNameservers(z)
child = families per name from ApexNameservers(z)   (step 4 view)

for each in-domain NS name, sorted:
   glue = families of the name's addressed items
   glue empty                          -> IN_DOMAIN_GLUE_MISSING      (ns)
   IPv4 not in glue, IPv4 in child     -> IN_DOMAIN_GLUE_MISSING_IPV4 (ns)
   IPv6 not in glue, IPv6 in child     -> IN_DOMAIN_GLUE_MISSING_IPV6 (ns)
   otherwise                           -> no tag
```
{{% /expand %}}

### Overall NS Count Checks (steps 2-3)

{{% expand "Show diagram" %}}
```
delegation NS names = sort(z.GlueNames)
   len >= constants.MinimumNumberOfNameservers -> ENOUGH_NS_DEL     (count, minimum, servers)
   otherwise                                   -> NOT_ENOUGH_NS_DEL (count, minimum, servers)

child NS names = sort(z.ApexNSNames)
   len >= constants.MinimumNumberOfNameservers -> ENOUGH_NS_CHILD     (count, minimum, servers)
   otherwise                                   -> NOT_ENOUGH_NS_CHILD (count, minimum, servers)
```
{{% /expand %}}

### Per-Family Addressed NS Checks (steps 4-5)

Same shape runs four times: child IPv4, child IPv6, delegation IPv4, delegation IPv6.

{{% expand "Show diagram" %}}
```
Per side <SIDE> in {CHILD (ApexNameservers), DEL (GlueNameservers)}:
  Per family <V> in {IPV4, IPV6}:
    count = unique-NS-name count whose address is in that family

    count == 0                                      -> NO_<V>_NS_<SIDE>
    0 < count <  MinimumNumberOfNameservers         -> NOT_ENOUGH_<V>_NS_<SIDE>
    count >= MinimumNumberOfNameservers             -> ENOUGH_<V>_NS_<SIDE>

  All tags carry (count, minimum, servers)
```
{{% /expand %}}

## Emitted Tags (Possible Set)
| Tag | Emitted when |
| --- | --- |
| `ENOUGH_IPV4_NS_CHILD` | Child side has at least minimum number of NS names with IPv4 addresses. |
| `ENOUGH_IPV4_NS_DEL` | Delegation side has at least minimum number of NS names with IPv4 addresses. |
| `ENOUGH_IPV6_NS_CHILD` | Child side has at least minimum number of NS names with IPv6 addresses. |
| `ENOUGH_IPV6_NS_DEL` | Delegation side has at least minimum number of NS names with IPv6 addresses. |
| `ENOUGH_NS_CHILD` | Child NS name count is at least minimum. |
| `ENOUGH_NS_DEL` | Delegation NS name count is at least minimum. |
| `IN_DOMAIN_GLUE_MISSING` | An in-domain delegation NS name has no A/AAAA glue in the parent referral (RFC 1034 section 4.2.1, RFC 9471 section 3.1). |
| `IN_DOMAIN_GLUE_MISSING_IPV4` | An in-domain delegation NS name has glue, none of it IPv4, while the child serves an IPv4 address for the name (RFC 10001 section 3.1). |
| `IN_DOMAIN_GLUE_MISSING_IPV6` | An in-domain delegation NS name has glue, none of it IPv6, while the child serves an IPv6 address for the name (RFC 10001 section 3.1). |
| `NOT_ENOUGH_IPV4_NS_CHILD` | Child side has IPv4-addressed NS names, but fewer than minimum. |
| `NOT_ENOUGH_IPV4_NS_DEL` | Delegation side has IPv4-addressed NS names, but fewer than minimum. |
| `NOT_ENOUGH_IPV6_NS_CHILD` | Child side has IPv6-addressed NS names, but fewer than minimum. |
| `NOT_ENOUGH_IPV6_NS_DEL` | Delegation side has IPv6-addressed NS names, but fewer than minimum. |
| `NOT_ENOUGH_NS_CHILD` | Child NS name count is fewer than minimum. |
| `NOT_ENOUGH_NS_DEL` | Delegation NS name count is fewer than minimum. |
| `NO_IPV4_NS_CHILD` | Child side has zero NS names with IPv4 addresses. |
| `NO_IPV4_NS_DEL` | Delegation side has zero NS names with IPv4 addresses. |
| `NO_IPV6_NS_CHILD` | Child side has zero NS names with IPv6 addresses. |
| `NO_IPV6_NS_DEL` | Delegation side has zero NS names with IPv6 addresses. |
| `NS_NAME_NO_IPV4_ADDR` | A nameserver name has IPv6 addresses and no IPv4 address in either view (RFC 10001 section 4.1). |
| `NS_NAME_NO_IPV6_ADDR` | A nameserver name has IPv4 addresses and no IPv6 address in either view (RFC 10001 section 4.1). |
| `TEST_CASE_END` | Testcase completion marker is emitted. |
| `TEST_CASE_START` | Testcase start marker is emitted. |

## Tag Arguments
| Tag | Argument key | Type | Meaning |
| --- | --- | --- | --- |
| `ENOUGH_IPV4_NS_CHILD` | `count` | `int` | Number of unique child NS names with IPv4 addresses. |
| `ENOUGH_IPV4_NS_CHILD` | `minimum` | `int` | Required minimum NS count. |
| `ENOUGH_IPV4_NS_CHILD` | `servers` | `array<object>` | Structured child `{ns,address}` object items considered for IPv4. |
| `ENOUGH_IPV4_NS_DEL` | `count` | `int` | Number of unique delegation NS names with IPv4 addresses. |
| `ENOUGH_IPV4_NS_DEL` | `minimum` | `int` | Required minimum NS count. |
| `ENOUGH_IPV4_NS_DEL` | `servers` | `array<object>` | Structured delegation `{ns,address}` object items considered for IPv4. |
| `ENOUGH_IPV6_NS_CHILD` | `count` | `int` | Number of unique child NS names with IPv6 addresses. |
| `ENOUGH_IPV6_NS_CHILD` | `minimum` | `int` | Required minimum NS count. |
| `ENOUGH_IPV6_NS_CHILD` | `servers` | `array<object>` | Structured child `{ns,address}` object items considered for IPv6. |
| `ENOUGH_IPV6_NS_DEL` | `count` | `int` | Number of unique delegation NS names with IPv6 addresses. |
| `ENOUGH_IPV6_NS_DEL` | `minimum` | `int` | Required minimum NS count. |
| `ENOUGH_IPV6_NS_DEL` | `servers` | `array<object>` | Structured delegation `{ns,address}` object items considered for IPv6. |
| `ENOUGH_NS_CHILD` | `count` | `int` | Number of child NS names. |
| `ENOUGH_NS_CHILD` | `minimum` | `int` | Required minimum NS count. |
| `ENOUGH_NS_CHILD` | `servers` | `array<object>` | Structured child nameserver names as `{ns}` items. |
| `ENOUGH_NS_DEL` | `count` | `int` | Number of delegation NS names. |
| `ENOUGH_NS_DEL` | `minimum` | `int` | Required minimum NS count. |
| `ENOUGH_NS_DEL` | `servers` | `array<object>` | Structured delegation nameserver names as `{ns}` items. |
| `IN_DOMAIN_GLUE_MISSING` | `ns` | `string` | The in-domain delegation NS name that lacks glue. |
| `IN_DOMAIN_GLUE_MISSING_IPV4` | `ns` | `string` | The in-domain delegation NS name that lacks IPv4 glue. |
| `IN_DOMAIN_GLUE_MISSING_IPV6` | `ns` | `string` | The in-domain delegation NS name that lacks IPv6 glue. |
| `NOT_ENOUGH_IPV4_NS_CHILD` | `count` | `int` | Number of unique child NS names with IPv4 addresses. |
| `NOT_ENOUGH_IPV4_NS_CHILD` | `minimum` | `int` | Required minimum NS count. |
| `NOT_ENOUGH_IPV4_NS_CHILD` | `servers` | `array<object>` | Structured child `{ns,address}` object items considered for IPv4. |
| `NOT_ENOUGH_IPV4_NS_DEL` | `count` | `int` | Number of unique delegation NS names with IPv4 addresses. |
| `NOT_ENOUGH_IPV4_NS_DEL` | `minimum` | `int` | Required minimum NS count. |
| `NOT_ENOUGH_IPV4_NS_DEL` | `servers` | `array<object>` | Structured delegation `{ns,address}` object items considered for IPv4. |
| `NOT_ENOUGH_IPV6_NS_CHILD` | `count` | `int` | Number of unique child NS names with IPv6 addresses. |
| `NOT_ENOUGH_IPV6_NS_CHILD` | `minimum` | `int` | Required minimum NS count. |
| `NOT_ENOUGH_IPV6_NS_CHILD` | `servers` | `array<object>` | Structured child `{ns,address}` object items considered for IPv6. |
| `NOT_ENOUGH_IPV6_NS_DEL` | `count` | `int` | Number of unique delegation NS names with IPv6 addresses. |
| `NOT_ENOUGH_IPV6_NS_DEL` | `minimum` | `int` | Required minimum NS count. |
| `NOT_ENOUGH_IPV6_NS_DEL` | `servers` | `array<object>` | Structured delegation `{ns,address}` object items considered for IPv6. |
| `NOT_ENOUGH_NS_CHILD` | `count` | `int` | Number of child NS names. |
| `NOT_ENOUGH_NS_CHILD` | `minimum` | `int` | Required minimum NS count. |
| `NOT_ENOUGH_NS_CHILD` | `servers` | `array<object>` | Structured child nameserver names as `{ns}` items. |
| `NOT_ENOUGH_NS_DEL` | `count` | `int` | Number of delegation NS names. |
| `NOT_ENOUGH_NS_DEL` | `minimum` | `int` | Required minimum NS count. |
| `NOT_ENOUGH_NS_DEL` | `servers` | `array<object>` | Structured delegation nameserver names as `{ns}` items. |
| `NO_IPV4_NS_CHILD` | `count` | `int` | Number of unique child NS names with IPv4 addresses (`0`). |
| `NO_IPV4_NS_CHILD` | `minimum` | `int` | Required minimum NS count. |
| `NO_IPV4_NS_CHILD` | `servers` | `array<object>` | Structured child `{ns,address}` object items considered for IPv4. |
| `NO_IPV4_NS_DEL` | `count` | `int` | Number of unique delegation NS names with IPv4 addresses (`0`). |
| `NO_IPV4_NS_DEL` | `minimum` | `int` | Required minimum NS count. |
| `NO_IPV4_NS_DEL` | `servers` | `array<object>` | Structured delegation `{ns,address}` object items considered for IPv4. |
| `NO_IPV6_NS_CHILD` | `count` | `int` | Number of unique child NS names with IPv6 addresses (`0`). |
| `NO_IPV6_NS_CHILD` | `minimum` | `int` | Required minimum NS count. |
| `NO_IPV6_NS_CHILD` | `servers` | `array<object>` | Structured child `{ns,address}` object items considered for IPv6. |
| `NO_IPV6_NS_DEL` | `count` | `int` | Number of unique delegation NS names with IPv6 addresses (`0`). |
| `NO_IPV6_NS_DEL` | `minimum` | `int` | Required minimum NS count. |
| `NO_IPV6_NS_DEL` | `servers` | `array<object>` | Structured delegation `{ns,address}` object items considered for IPv6. |
| `NS_NAME_NO_IPV4_ADDR` | `ns` | `string` | The nameserver name with no IPv4 address. |
| `NS_NAME_NO_IPV6_ADDR` | `ns` | `string` | The nameserver name with no IPv6 address. |
| `TEST_CASE_END` | `testcase` | `string` | Testcase display name (`Delegation01`). |
| `TEST_CASE_START` | `testcase` | `string` | Testcase display name (`Delegation01`). |

## Severity Levels Per Tag
| Tag | Level | Notes |
| --- | --- | --- |
| `ENOUGH_IPV4_NS_CHILD` | `INFO` | Default from `share/profile.json` (`test_levels.DELEGATION`). |
| `ENOUGH_IPV4_NS_DEL` | `INFO` | Default from `share/profile.json` (`test_levels.DELEGATION`). |
| `ENOUGH_IPV6_NS_CHILD` | `INFO` | Default from `share/profile.json` (`test_levels.DELEGATION`). |
| `ENOUGH_IPV6_NS_DEL` | `INFO` | Default from `share/profile.json` (`test_levels.DELEGATION`). |
| `ENOUGH_NS_CHILD` | `INFO` | Default from `share/profile.json` (`test_levels.DELEGATION`). |
| `ENOUGH_NS_DEL` | `INFO` | Default from `share/profile.json` (`test_levels.DELEGATION`). |
| `IN_DOMAIN_GLUE_MISSING` | `ERROR` | Default from `share/profile.json` (`test_levels.DELEGATION`). |
| `IN_DOMAIN_GLUE_MISSING_IPV4` | `WARNING` | Default from `share/profile.json` (`test_levels.DELEGATION`). The name stays reachable over IPv6. |
| `IN_DOMAIN_GLUE_MISSING_IPV6` | `WARNING` | Default from `share/profile.json` (`test_levels.DELEGATION`). The name stays reachable over IPv4. |
| `NOT_ENOUGH_IPV4_NS_CHILD` | `ERROR` | Default from `share/profile.json` (`test_levels.DELEGATION`). |
| `NOT_ENOUGH_IPV4_NS_DEL` | `ERROR` | Default from `share/profile.json` (`test_levels.DELEGATION`). |
| `NOT_ENOUGH_IPV6_NS_CHILD` | `ERROR` | Default from `share/profile.json` (`test_levels.DELEGATION`). |
| `NOT_ENOUGH_IPV6_NS_DEL` | `ERROR` | Default from `share/profile.json` (`test_levels.DELEGATION`). |
| `NOT_ENOUGH_NS_CHILD` | `ERROR` | Default from `share/profile.json` (`test_levels.DELEGATION`). |
| `NOT_ENOUGH_NS_DEL` | `ERROR` | Default from `share/profile.json` (`test_levels.DELEGATION`). |
| `NO_IPV4_NS_CHILD` | `ERROR` | Default from `share/profile.json` (`test_levels.DELEGATION`). RFC 10001 section 4.1 MUST, as for `NOT_ENOUGH_IPV4_NS_CHILD`. |
| `NO_IPV4_NS_DEL` | `ERROR` | Default from `share/profile.json` (`test_levels.DELEGATION`). RFC 10001 section 4.1 MUST, as for `NOT_ENOUGH_IPV4_NS_DEL`. |
| `NO_IPV6_NS_CHILD` | `ERROR` | Default from `share/profile.json` (`test_levels.DELEGATION`). RFC 10001 section 4.1 MUST, as for `NOT_ENOUGH_IPV6_NS_CHILD`. |
| `NO_IPV6_NS_DEL` | `ERROR` | Default from `share/profile.json` (`test_levels.DELEGATION`). RFC 10001 section 4.1 MUST, as for `NOT_ENOUGH_IPV6_NS_DEL`. |
| `NS_NAME_NO_IPV4_ADDR` | `NOTICE` | Default from `share/profile.json` (`test_levels.DELEGATION`). Zero penalty in the default scoring config. |
| `NS_NAME_NO_IPV6_ADDR` | `NOTICE` | Default from `share/profile.json` (`test_levels.DELEGATION`). Zero penalty in the default scoring config. |
| `TEST_CASE_END` | `DEBUG` | Default from `share/profile.json` (`test_levels.DELEGATION`). |
| `TEST_CASE_START` | `DEBUG` | Default from `share/profile.json` (`test_levels.DELEGATION`). |

## Differences From Upstream
- Differences (Upstream vs Gonemaster):
  - Upstream: describes delegation-side IPv4/IPv6 checks before child-side IPv4/IPv6 checks. Gonemaster: performs child-side family checks first, then delegation-side family checks.
  - Upstream: does not describe testcase boundary debug markers. Gonemaster: emits `TEST_CASE_START` and `TEST_CASE_END`.
  - Upstream: sets `NO_IPV4_NS_CHILD` and `NO_IPV4_NS_DEL` to `WARNING`, and `NO_IPV6_NS_CHILD` and `NO_IPV6_NS_DEL` to `NOTICE` in the released test plan, `WARNING` on the development branch since [zonemaster#1533](https://github.com/zonemaster/zonemaster/pull/1533), which also quotes RFC 10001 section 4.1 as the objective. Gonemaster: all four at `ERROR`, the level of the one-server `NOT_ENOUGH_*` tags, since zero servers in a family violates the same RFC 10001 section 4.1 MUST.
  - Upstream: only counts NS names with at least one address in aggregate, so a single in-domain name missing its glue is absorbed into the count and never flagged. Gonemaster: additionally flags each such name with `IN_DOMAIN_GLUE_MISSING`, and each name whose glue lacks a family the child serves with `IN_DOMAIN_GLUE_MISSING_IPV4` or `IN_DOMAIN_GLUE_MISSING_IPV6`.
  - Upstream: counts NS names per family and names no NS name that resolves in one family only. Gonemaster: flags each such name with `NS_NAME_NO_IPV4_ADDR` or `NS_NAME_NO_IPV6_ADDR`.
- Potential upstream report:
  - `yes` (feature proposal: per-name in-domain glue presence)
  - Severity reported as [zonemaster#1529](https://github.com/zonemaster/zonemaster/issues/1529); the remaining difference is tracked as `DIV-DEL01-IPV6-SEVERITY` in [known-behavior-divergences.md](../../known-behavior-divergences.md).

## Edge Cases And Limitations
- Per-family counts are by unique NS name, not by count of IP addresses.
- A single NS name can contribute to both IPv4 and IPv6 counts when it has both address families.
- Any method error aborts testcase execution and can prevent later tags from being emitted.
- An IPv4-mapped address (`::ffff:0:0/96`) in an AAAA record counts as IPv4 in every step: the nameserver views unmap it, and step 7 unmaps glue items the same way. A name whose only AAAA is mapped counts as IPv4-only. NAT64 (`64:ff9b::/96`, `64:ff9b:1::/48`) and IPv4-compatible (`::/96`) addresses count as IPv6 here; Address01 reports all four forms as `A01_IPV4_EMBEDDED_IPV6_ADDR`.
- The glue checks use the raw parent referral (`DelegationNameservers`), not `GlueNameservers`, so a name resolvable only via the recursor still counts as missing glue.
- When the parent answers with AA instead of a referral, `DelegationNameservers` fills the addresses through the recursor, so the glue checks there see the name's address records rather than referral glue.
- An in-domain name with no record of a family anywhere gets `NS_NAME_NO_IPV4_ADDR` or `NS_NAME_NO_IPV6_ADDR` and no per-family glue tag: there is no record to glue.
- A child that does not answer for an in-domain name gives no child-side evidence, so no per-family glue tag fires for it.
- Consistency05 reports a family the child serves beyond the glue in the aggregate `EXTRA_ADDRESS_CHILD`; the per-family glue tags name the same defect per NS name. Glue covering both families while the child lacks one is Consistency05's `IN_DOMAIN_ADDR_MISMATCH`, not a finding of this testcase.
- Not-in-domain NS names are out of scope for the glue checks; their resolvability is covered by other testcases.
- A name whose address lookup ends in NXDOMAIN has no address and gets no per-name tag.
- `ApexNameservers` is built from the first successful apex response, while `ApexNSNames` is the union over all child servers. A name only in that union has no address data and gets no per-name tag unless the delegation side supplies one.
