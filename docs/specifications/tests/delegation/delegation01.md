# Delegation01

Status: Final

## Purpose
- Validate that delegation-side and child-side nameserver sets meet minimum-count requirements overall and per IP family.
- Flag any in-bailiwick delegation NS name that the parent referral ships without the required A/AAAA glue.

## Preconditions And Inputs
- Preconditions:
  - A `zone.Zone` object is available.
- Required inputs:
  - Delegation names from [`z.GlueNames(ctx)`](../../nameserver-resolution.md#gluenames).
  - Child NS names from [`z.ApexNSNames(ctx)`](../../nameserver-resolution.md#apexnsnames).
  - Delegation nameserver addresses from [`GlueNameservers`](../../nameserver-resolution.md#gluenameservers).
  - Child nameserver addresses from [`ApexNameservers`](../../nameserver-resolution.md#apexnameservers).
  - Raw delegation items from [`DelegationNameservers`](../../nameserver-resolution.md#delegationnameservers), which carry in-bailiwick NS names without an address when the referral has no glue for them.
- Profile/config knobs that affect behavior:
  - No direct profile knob in this testcase.
  - Minimum required nameserver count is fixed by `constants.MinimumNumberOfNameservers`.

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
6. Get raw delegation items ([`DelegationNameservers`](../../nameserver-resolution.md#delegationnameservers)). For each in-bailiwick NS name that has no address in any item (the referral shipped no glue for it), emit `IN_BAILIWICK_GLUE_MISSING` (one per name, sorted). Out-of-bailiwick names are out of scope here.
7. Emit `TEST_CASE_END`.

### In-Bailiwick Glue Presence Check (step 6)

A delegation NS name that lies inside the zone (in-bailiwick) can only be reached if the parent referral carries its address as glue; otherwise a resolver holding only the referral cannot resolve the name (the address lives inside the very zone the name serves). The raw referral view from `DelegationNameservers` is used here rather than [`GlueNameservers`](../../nameserver-resolution.md#gluenameservers), because the latter resolves the name through the recursor and would mask the missing glue.

{{% expand "Show diagram" %}}
```
items = DelegationNameservers(z)
for each in-bailiwick NS name with no addressed item:
   IN_BAILIWICK_GLUE_MISSING (ns)
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
| `IN_BAILIWICK_GLUE_MISSING` | An in-bailiwick delegation NS name has no A/AAAA glue in the parent referral. |
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
| `IN_BAILIWICK_GLUE_MISSING` | `ns` | `string` | The in-bailiwick delegation NS name that lacks glue. |
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
| `IN_BAILIWICK_GLUE_MISSING` | `ERROR` | Default from `share/profile.json` (`test_levels.DELEGATION`). |
| `NOT_ENOUGH_IPV4_NS_CHILD` | `ERROR` | Default from `share/profile.json` (`test_levels.DELEGATION`). |
| `NOT_ENOUGH_IPV4_NS_DEL` | `ERROR` | Default from `share/profile.json` (`test_levels.DELEGATION`). |
| `NOT_ENOUGH_IPV6_NS_CHILD` | `ERROR` | Default from `share/profile.json` (`test_levels.DELEGATION`). |
| `NOT_ENOUGH_IPV6_NS_DEL` | `ERROR` | Default from `share/profile.json` (`test_levels.DELEGATION`). |
| `NOT_ENOUGH_NS_CHILD` | `ERROR` | Default from `share/profile.json` (`test_levels.DELEGATION`). |
| `NOT_ENOUGH_NS_DEL` | `ERROR` | Default from `share/profile.json` (`test_levels.DELEGATION`). |
| `NO_IPV4_NS_CHILD` | `WARNING` | Default from `share/profile.json` (`test_levels.DELEGATION`). |
| `NO_IPV4_NS_DEL` | `WARNING` | Default from `share/profile.json` (`test_levels.DELEGATION`). |
| `NO_IPV6_NS_CHILD` | `NOTICE` | Default from `share/profile.json` (`test_levels.DELEGATION`). |
| `NO_IPV6_NS_DEL` | `NOTICE` | Default from `share/profile.json` (`test_levels.DELEGATION`). |
| `TEST_CASE_END` | `DEBUG` | Default from `share/profile.json` (`test_levels.DELEGATION`). |
| `TEST_CASE_START` | `DEBUG` | Default from `share/profile.json` (`test_levels.DELEGATION`). |

## Differences From Upstream
- Differences (Upstream vs Gonemaster):
  - Upstream: describes delegation-side IPv4/IPv6 checks before child-side IPv4/IPv6 checks. Gonemaster: performs child-side family checks first, then delegation-side family checks.
  - Upstream: does not describe testcase boundary debug markers. Gonemaster: emits `TEST_CASE_START` and `TEST_CASE_END`.
  - Upstream: only counts NS names with at least one address in aggregate, so a single in-bailiwick name missing its glue is absorbed into the count and never flagged. Gonemaster: additionally flags each such name with `IN_BAILIWICK_GLUE_MISSING`.
- Potential upstream report:
  - `yes` (feature proposal: per-name in-bailiwick glue presence)

## Edge Cases And Limitations
- Per-family counts are by unique NS name, not by count of IP addresses.
- A single NS name can contribute to both IPv4 and IPv6 counts when it has both address families.
- Any method error aborts testcase execution and can prevent later tags from being emitted.
- The glue-presence check uses the raw parent referral (`DelegationNameservers`), not `GlueNameservers`, so a name resolvable only via the recursor still counts as missing glue.
- Out-of-bailiwick NS names are out of scope for the glue-presence check; their resolvability is covered by other testcases.
