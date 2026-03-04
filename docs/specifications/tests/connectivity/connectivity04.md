# Connectivity04 (connectivity04)

Status: Final

## Purpose
- Evaluate prefix diversity of nameserver IP addresses by IP family.
- Report same-prefix groups, unique-prefix members, and single-prefix concentration.

## Preconditions And Inputs
- Preconditions:
  - A `zone.Zone` object is available.
  - A recursor is available on the zone object.
- Required inputs:
  - Nameserver name/address sets from `methodsv2.GetDelNSNamesAndIPs` and `methodsv2.GetZoneNSNamesAndIPs`.
  - ASN/prefix lookup results per unique IP address from `asnlookup.GetWithPrefix`.
- Profile/config knobs that affect behavior:
  - `resolver.defaults.parallel`: parallel prefix lookup fan-out.
  - `asn_db.style` and `asn_db.sources`: choose lookup backend/source list.

## Algorithm And Decision Flow
1. Emit `TEST_CASE_START`.
2. Collect delegation and zone NS items, concatenate, and keep addressed entries only.
3. Build an ordered unique lookup list by IP address (IPv4 and IPv6 tracked separately), preserving first-seen item per unique IP.
4. For each unique IP (parallelized):
   - Run ASN/prefix lookup (`lookupASN`).
   - If result code is `ERROR_ASN_DATABASE`, emit `CN04_ERROR_PREFIX_DATABASE` (`ns_ip`) and stop processing for that IP.
   - If result code is `EMPTY_ASN_SET`, emit `CN04_EMPTY_PREFIX_SET` (`ns_ip`) and stop processing for that IP.
   - If raw lookup text exists, emit `CN04_ASN_INFOS_RAW` (`ns_ip`, `data`).
   - If prefix exists, emit `CN04_ASN_INFOS_ANNOUNCE_IN` (`ns_ip`, `prefixes`) and store the `(prefix -> ns item)` relation for that IP family.
5. For each family (IPv4 then IPv6) with stored prefixes:
   - For each prefix group with 2 or more members, emit `CN04_IPV<4|6>_SAME_PREFIX` (`prefixes`, `servers`).
   - Collect members from prefix groups with exactly 1 member and emit `CN04_IPV<4|6>_DIFFERENT_PREFIX` with combined `servers` if non-empty.
   - If exactly one prefix exists for the family and all processed family IPs mapped to that prefix, emit `CN04_IPV<4|6>_SINGLE_PREFIX`.
6. Emit `TEST_CASE_END`.

## Emitted Tags (Possible Set)
| Tag | Emitted when |
| --- | --- |
| `CN04_ASN_INFOS_ANNOUNCE_IN` | Prefix lookup returned a containing prefix for an IP. |
| `CN04_ASN_INFOS_RAW` | Prefix lookup returned raw backend data text for an IP. |
| `CN04_EMPTY_PREFIX_SET` | Prefix backend returned no mapping for an IP. |
| `CN04_ERROR_PREFIX_DATABASE` | Prefix lookup backend failed for an IP. |
| `CN04_IPV4_DIFFERENT_PREFIX` | IPv4 has one or more single-member prefix groups. |
| `CN04_IPV4_SAME_PREFIX` | At least one IPv4 prefix group contains 2+ nameserver items. |
| `CN04_IPV4_SINGLE_PREFIX` | Every processed IPv4 IP with successful prefix lookup maps to one identical prefix and no processed IPv4 IP is missing prefix data. |
| `CN04_IPV6_DIFFERENT_PREFIX` | IPv6 has one or more single-member prefix groups. |
| `CN04_IPV6_SAME_PREFIX` | At least one IPv6 prefix group contains 2+ nameserver items. |
| `CN04_IPV6_SINGLE_PREFIX` | Every processed IPv6 IP with successful prefix lookup maps to one identical prefix and no processed IPv6 IP is missing prefix data. |
| `TEST_CASE_END` | Testcase completion marker is emitted. |
| `TEST_CASE_START` | Testcase start marker is emitted. |

## Tag Arguments
| Tag | Argument key | Type | Meaning |
| --- | --- | --- | --- |
| `CN04_ASN_INFOS_ANNOUNCE_IN` | `ns_ip` | `string` | IP address looked up. |
| `CN04_ASN_INFOS_ANNOUNCE_IN` | `prefixes` | `array<string>` | Structured prefix list announcing the IP (single-item list per entry). |
| `CN04_ASN_INFOS_RAW` | `ns_ip` | `string` | IP address looked up. |
| `CN04_ASN_INFOS_RAW` | `data` | `string` | Raw backend response data string. |
| `CN04_EMPTY_PREFIX_SET` | `ns_ip` | `string` | IP address with empty prefix lookup result. |
| `CN04_ERROR_PREFIX_DATABASE` | `ns_ip` | `string` | IP address where prefix lookup backend failed. |
| `CN04_IPV4_DIFFERENT_PREFIX` | `servers` | `array<object>` | Structured single-member IPv4 prefix-group items (`{ns,address}` object). |
| `CN04_IPV4_SAME_PREFIX` | `prefixes` | `array<string>` | Structured shared IPv4 prefix list (single-item list per entry). |
| `CN04_IPV4_SAME_PREFIX` | `servers` | `array<object>` | Structured nameserver items (`{ns,address}` object) in that prefix group. |
| `CN04_IPV4_SINGLE_PREFIX` | `-` | `-` | No arguments. |
| `CN04_IPV6_DIFFERENT_PREFIX` | `servers` | `array<object>` | Structured single-member IPv6 prefix-group items (`{ns,address}` object). |
| `CN04_IPV6_SAME_PREFIX` | `prefixes` | `array<string>` | Structured shared IPv6 prefix list (single-item list per entry). |
| `CN04_IPV6_SAME_PREFIX` | `servers` | `array<object>` | Structured nameserver items (`{ns,address}` object) in that prefix group. |
| `CN04_IPV6_SINGLE_PREFIX` | `-` | `-` | No arguments. |
| `TEST_CASE_END` | `testcase` | `string` | Testcase display name (`Connectivity04`). |
| `TEST_CASE_START` | `testcase` | `string` | Testcase display name (`Connectivity04`). |

## Severity Levels Per Tag
| Tag | Level | Notes |
| --- | --- | --- |
| `CN04_ASN_INFOS_ANNOUNCE_IN` | `DEBUG` | Default from `share/profile.json` (`test_levels.CONNECTIVITY`). |
| `CN04_ASN_INFOS_RAW` | `DEBUG` | Default from `share/profile.json` (`test_levels.CONNECTIVITY`). |
| `CN04_EMPTY_PREFIX_SET` | `NOTICE` | Default from `share/profile.json` (`test_levels.CONNECTIVITY`). |
| `CN04_ERROR_PREFIX_DATABASE` | `NOTICE` | Default from `share/profile.json` (`test_levels.CONNECTIVITY`). |
| `CN04_IPV4_DIFFERENT_PREFIX` | `INFO` | Default from `share/profile.json` (`test_levels.CONNECTIVITY`). |
| `CN04_IPV4_SAME_PREFIX` | `NOTICE` | Default from `share/profile.json` (`test_levels.CONNECTIVITY`). |
| `CN04_IPV4_SINGLE_PREFIX` | `WARNING` | Default from `share/profile.json` (`test_levels.CONNECTIVITY`). |
| `CN04_IPV6_DIFFERENT_PREFIX` | `INFO` | Default from `share/profile.json` (`test_levels.CONNECTIVITY`). |
| `CN04_IPV6_SAME_PREFIX` | `NOTICE` | Default from `share/profile.json` (`test_levels.CONNECTIVITY`). |
| `CN04_IPV6_SINGLE_PREFIX` | `WARNING` | Default from `share/profile.json` (`test_levels.CONNECTIVITY`). |
| `TEST_CASE_END` | `DEBUG` | Default from `share/profile.json` (`test_levels.CONNECTIVITY`). |
| `TEST_CASE_START` | `DEBUG` | Default from `share/profile.json` (`test_levels.CONNECTIVITY`). |

## Differences From Upstream
- Upstream reference: [`connectivity04.md`](../../upstream/tests/Connectivity-TP/connectivity04.md)
- Differences (Upstream vs Gonemaster):
  - Upstream: does not explicitly define this detail. Gonemaster: emits additional debug observability tags (`CN04_ASN_INFOS_RAW`, `CN04_ASN_INFOS_ANNOUNCE_IN`).
  - Upstream: does not explicitly define this detail. Gonemaster: Multiple nameserver names sharing the same IP are collapsed to one first-seen nameserver name before prefix grouping.
- Potential upstream report:
  - `yes`
- If yes, include:
  - Upstream expected behavior: Prefix grouping is described over nameserver name-and-IP members collected from Methodsv2 sets.
  - Gonemaster observed behavior: Prefix grouping deduplicates by IP before grouping, so alternate names sharing the same IP are not independently represented.
  - evidence: `docs/specifications/upstream/tests/Connectivity-TP/connectivity04.md`, `engine/test/connectivity/connectivity.go` (`Connectivity04` dedup via `processed[version][ip.String()]`).
  - report status: `not filed`

## Edge Cases And Limitations
- If no addressed NS items are found, only testcase start/end tags are emitted.
- `CN04_IPV<4|6>_SINGLE_PREFIX` is not emitted when any processed IP of that family has empty/error prefix lookup.
- Per-IP prefix lookup tasks run in parallel, but emitted entry ordering is deterministic relative to input order.
