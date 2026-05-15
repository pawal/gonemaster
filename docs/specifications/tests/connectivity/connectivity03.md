# Connectivity03

Status: Final

## Purpose
- Evaluate ASN diversity of authoritative nameserver IP addresses.
- Classify IPv4 and IPv6 ASN diversity independently.

## Preconditions And Inputs
- Preconditions:
  - A `zone.Zone` object is available.
  - A recursor is available on the zone object.
- Required inputs:
  - Nameserver addresses from `methods.Method4and5`.
  - ASN lookup results per unique IP address from `asnlookup.GetWithPrefix`.
- Profile/config knobs that affect behavior:
  - `resolver.defaults.parallel`: parallel ASN lookup fan-out.
  - `asn_db.style` and `asn_db.sources`: choose lookup backend/source list.

## Algorithm And Decision Flow
1. Emit `TEST_CASE_START`.
2. Resolve nameserver list from `Method4and5`.
3. Split into unique IPv4 and unique IPv6 IP lists (deduplicated by IP string).
4. For each unique IP (parallelized by family):
   - Run ASN lookup (`lookupASN`).
   - If result code is `ERROR_ASN_DATABASE` or `EMPTY_ASN_SET`, emit that tag with `ns_ip` and stop processing for that IP.
   - If raw lookup text exists, emit `ASN_INFOS_RAW` (`ns_ip`, `data`).
   - If ASN list exists, emit `ASN_INFOS_ANNOUNCE_BY` (`ns_ip`, `asns`) and store ASN set for diversity classification.
   - If prefix exists, emit `ASN_INFOS_ANNOUNCE_IN` (`ns_ip`, `prefixes`).
5. For IPv4 stored ASN data:
   - If no ASN values were stored, emit no IPv4 diversity summary tag.
   - If exactly one unique ASN exists, emit `IPV4_ONE_ASN`.
   - Else if all IPv4 IPs with ASN data share the same ASN-set signature, emit `IPV4_SAME_ASN`.
   - Else emit `IPV4_DIFFERENT_ASN`.
6. Repeat step 5 for IPv6 (`IPV6_ONE_ASN`, `IPV6_SAME_ASN`, `IPV6_DIFFERENT_ASN`).
7. Emit `TEST_CASE_END`.

### Per-IP ASN Lookup (step 4)

{{% expand "Show diagram" %}}
{{< mermaid >}}
stateDiagram-v2
    [*] --> resolve
    resolve : resolve NS list
    resolve --> split
    split : split by IP family
    split --> lookup
    lookup : per-IP ASN lookup (parallel)
    lookup --> code
    code : result code
    code --> dbErr : db error
    code --> emptySet : empty
    code --> ok : lookup ok
    dbErr : asn-db-error tag
    emptySet : empty-asn-set tag
    ok : observability tags
    ok --> stored
    stored : ASNs stored per family
    dbErr --> [*]
    emptySet --> [*]
    stored --> [*]
{{< /mermaid >}}
{{% /expand %}}

### Per-Family Diversity Classification (steps 5-6)

{{% expand "Show diagram" %}}
{{< mermaid >}}
stateDiagram-v2
    [*] --> v4Class
    v4Class : IPv4 stored ASN data
    v4Class --> v4None : no ASNs stored
    v4Class --> v4One : exactly one ASN
    v4Class --> v4Same : same ASN signature
    v4Class --> v4Diff : different signatures
    v4None : no IPv4 diversity tag
    v4One : ipv4-one-asn tag
    v4Same : ipv4-same-asn tag
    v4Diff : ipv4-different-asn tag
    v4None --> v6Class
    v4One --> v6Class
    v4Same --> v6Class
    v4Diff --> v6Class
    v6Class : IPv6 stored ASN data
    v6Class --> v6None : no ASNs stored
    v6Class --> v6One : exactly one ASN
    v6Class --> v6Same : same ASN signature
    v6Class --> v6Diff : different signatures
    v6None : no IPv6 diversity tag
    v6One : ipv6-one-asn tag
    v6Same : ipv6-same-asn tag
    v6Diff : ipv6-different-asn tag
    v6None --> done
    v6One --> done
    v6Same --> done
    v6Diff --> done
    done : emit test-case-end
    done --> [*]
{{< /mermaid >}}
{{% /expand %}}

## Emitted Tags (Possible Set)
| Tag | Emitted when |
| --- | --- |
| `ASN_INFOS_ANNOUNCE_BY` | ASN lookup returned one or more ASNs for an IP. |
| `ASN_INFOS_ANNOUNCE_IN` | ASN lookup returned a containing prefix for an IP. |
| `ASN_INFOS_RAW` | ASN lookup returned raw backend data text for an IP. |
| `EMPTY_ASN_SET` | ASN backend returned no ASN mapping for an IP. |
| `ERROR_ASN_DATABASE` | ASN lookup backend failed for an IP. |
| `IPV4_DIFFERENT_ASN` | IPv4 ASN data contains more than one distinct ASN-set signature. |
| `IPV4_ONE_ASN` | IPv4 ASN data collapses to exactly one ASN. |
| `IPV4_SAME_ASN` | IPv4 ASN data contains multiple ASNs but every IPv4 IP maps to the same ASN set. |
| `IPV6_DIFFERENT_ASN` | IPv6 ASN data contains more than one distinct ASN-set signature. |
| `IPV6_ONE_ASN` | IPv6 ASN data collapses to exactly one ASN. |
| `IPV6_SAME_ASN` | IPv6 ASN data contains multiple ASNs but every IPv6 IP maps to the same ASN set. |
| `TEST_CASE_END` | Testcase completion marker is emitted. |
| `TEST_CASE_START` | Testcase start marker is emitted. |

## Tag Arguments
| Tag | Argument key | Type | Meaning |
| --- | --- | --- | --- |
| `ASN_INFOS_ANNOUNCE_BY` | `ns_ip` | `string` | IP address looked up. |
| `ASN_INFOS_ANNOUNCE_BY` | `asns` | `array<int>` | Structured ASN list reported for the IP. |
| `ASN_INFOS_ANNOUNCE_IN` | `ns_ip` | `string` | IP address looked up. |
| `ASN_INFOS_ANNOUNCE_IN` | `prefixes` | `array<string>` | Structured prefix list announcing the IP (single-item list per entry). |
| `ASN_INFOS_RAW` | `ns_ip` | `string` | IP address looked up. |
| `ASN_INFOS_RAW` | `data` | `string` | Raw backend response data string. |
| `EMPTY_ASN_SET` | `ns_ip` | `string` | IP address with empty ASN lookup result. |
| `ERROR_ASN_DATABASE` | `ns_ip` | `string` | IP address where ASN lookup backend failed. |
| `IPV4_DIFFERENT_ASN` | `asns` | `array<int>` | Structured unique IPv4 ASN values. |
| `IPV4_ONE_ASN` | `asn` | `int` | Single unique IPv4 ASN value. |
| `IPV4_SAME_ASN` | `asns` | `array<int>` | Structured ASN values shared across IPv4 IPs. |
| `IPV6_DIFFERENT_ASN` | `asns` | `array<int>` | Structured unique IPv6 ASN values. |
| `IPV6_ONE_ASN` | `asn` | `int` | Single unique IPv6 ASN value. |
| `IPV6_SAME_ASN` | `asns` | `array<int>` | Structured ASN values shared across IPv6 IPs. |
| `TEST_CASE_END` | `testcase` | `string` | Testcase display name (`Connectivity03`). |
| `TEST_CASE_START` | `testcase` | `string` | Testcase display name (`Connectivity03`). |

## Severity Levels Per Tag
| Tag | Level | Notes |
| --- | --- | --- |
| `ASN_INFOS_ANNOUNCE_BY` | `DEBUG` | Default from `share/profile.json` (`test_levels.CONNECTIVITY`). |
| `ASN_INFOS_ANNOUNCE_IN` | `DEBUG` | Default from `share/profile.json` (`test_levels.CONNECTIVITY`). |
| `ASN_INFOS_RAW` | `DEBUG` | Default from `share/profile.json` (`test_levels.CONNECTIVITY`). |
| `EMPTY_ASN_SET` | `NOTICE` | Default from `share/profile.json` (`test_levels.CONNECTIVITY`). |
| `ERROR_ASN_DATABASE` | `NOTICE` | Default from `share/profile.json` (`test_levels.CONNECTIVITY`). |
| `IPV4_DIFFERENT_ASN` | `INFO` | Default from `share/profile.json` (`test_levels.CONNECTIVITY`). |
| `IPV4_ONE_ASN` | `WARNING` | Default from `share/profile.json` (`test_levels.CONNECTIVITY`). |
| `IPV4_SAME_ASN` | `NOTICE` | Default from `share/profile.json` (`test_levels.CONNECTIVITY`). |
| `IPV6_DIFFERENT_ASN` | `INFO` | Default from `share/profile.json` (`test_levels.CONNECTIVITY`). |
| `IPV6_ONE_ASN` | `WARNING` | Default from `share/profile.json` (`test_levels.CONNECTIVITY`). |
| `IPV6_SAME_ASN` | `NOTICE` | Default from `share/profile.json` (`test_levels.CONNECTIVITY`). |
| `TEST_CASE_END` | `DEBUG` | Default from `share/profile.json` (`test_levels.CONNECTIVITY`). |
| `TEST_CASE_START` | `DEBUG` | Default from `share/profile.json` (`test_levels.CONNECTIVITY`). |

## Differences From Upstream
- Upstream reference: [`connectivity03.md`](../../upstream/tests/Connectivity-TP/connectivity03.md)
- Differences (Upstream vs Gonemaster):
  - Upstream: does not explicitly define this detail. Gonemaster: emits additional per-IP debug observability tags (`ASN_INFOS_RAW`, `ASN_INFOS_ANNOUNCE_BY`, `ASN_INFOS_ANNOUNCE_IN`).
  - Upstream: does not explicitly define this detail. Gonemaster: Diversity summary tags are computed from IPs that returned ASN data; IPs with `EMPTY_ASN_SET`/`ERROR_ASN_DATABASE` do not contribute ASN values to summary classification.
- Potential upstream report:
  - `no`

## Edge Cases And Limitations
- If no IPs exist for a family, that family emits no diversity summary tag.
- If all lookups in a family fail or return empty ASN sets, that family emits only error/empty tags and no diversity summary tag.
- Per-IP ASN lookup tasks run in parallel, but emitted entry ordering is deterministic relative to input order.
