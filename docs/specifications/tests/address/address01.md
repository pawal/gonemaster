# Address01 (address01)

Status: Draft

## Purpose
- Classify authoritative nameserver IP addresses as globally reachable, documentation, local-use, or otherwise not globally reachable.
- Detect the case where no nameserver address data is available.

## Preconditions And Inputs
- Preconditions:
  - A `zone.Zone` object is available.
  - A recursor is available on the zone object for Methodsv2 lookups.
- Required inputs:
  - Delegation nameserver names and addresses from `GetDelNSNamesAndIPs`.
  - Zone-apex nameserver names and addresses from `GetZoneNSNamesAndIPs`.
  - Special-purpose address registry data via `constants.FindSpecialAddress`.
- Profile/config knobs that affect behavior:
  - `net.ipv4` and `net.ipv6` may affect address discovery in Methodsv2 lookups.

## Algorithm And Decision Flow
1. Emit `TEST_CASE_START`.
2. Query nameserver names/addresses using `GetDelNSNamesAndIPs` and `GetZoneNSNamesAndIPs`.
3. Merge the two result sets, keep only entries with an address, and deduplicate by lowercase `name/ip`.
4. If no addressed nameserver remains, emit `A01_NO_NAME_SERVERS_FOUND`, emit `TEST_CASE_END`, and return.
5. Group items by IP address, then iterate IP groups in lexicographic IP string order.
6. For each IP group, classify by special-address category:
   - Documentation category -> add all `name/ip` pairs in group to documentation set.
   - Local-use categories (`Private-Use`, `Loopback`, `Link Local`, `Link-Local`, `Unique-Local`, `Shared Address Space`) -> add to local-use set.
   - Any other non-globally-reachable special range -> add to not-globally-reachable set.
   - Otherwise -> add to globally-reachable set.
7. If globally-reachable set is non-empty, emit `A01_GLOBALLY_REACHABLE_ADDR` with `ns_list`; else emit `A01_NO_GLOBALLY_REACHABLE_ADDR`.
8. Emit `A01_DOCUMENTATION_ADDR` if documentation set is non-empty.
9. Emit `A01_LOCAL_USE_ADDR` if local-use set is non-empty.
10. Emit `A01_ADDR_NOT_GLOBALLY_REACHABLE` if not-globally-reachable set is non-empty.
11. Emit `TEST_CASE_END`.

## Emitted Tags (Possible Set)
| Tag | Emitted when |
| --- | --- |
| `A01_ADDR_NOT_GLOBALLY_REACHABLE` | At least one nameserver IP is in a special-purpose range that is not globally reachable and not classified as documentation/local-use. |
| `A01_DOCUMENTATION_ADDR` | At least one nameserver IP is in a documentation-only range. |
| `A01_GLOBALLY_REACHABLE_ADDR` | At least one nameserver IP is classified as globally reachable. |
| `A01_LOCAL_USE_ADDR` | At least one nameserver IP is in a local-use category. |
| `A01_NO_GLOBALLY_REACHABLE_ADDR` | No nameserver IP is classified as globally reachable. |
| `A01_NO_NAME_SERVERS_FOUND` | No nameserver with an address was discovered from delegation+zone sources. |
| `TEST_CASE_END` | Testcase completion marker is emitted. |
| `TEST_CASE_START` | Testcase start marker is emitted. |

## Tag Arguments
| Tag | Argument key | Type | Meaning |
| --- | --- | --- | --- |
| `A01_ADDR_NOT_GLOBALLY_REACHABLE` | `ns_list` | `string` | Semicolon-delimited `name/ip` pairs in the non-globally-reachable set. |
| `A01_DOCUMENTATION_ADDR` | `ns_list` | `string` | Semicolon-delimited `name/ip` pairs in documentation ranges. |
| `A01_GLOBALLY_REACHABLE_ADDR` | `ns_list` | `string` | Semicolon-delimited `name/ip` pairs in globally reachable ranges. |
| `A01_LOCAL_USE_ADDR` | `ns_list` | `string` | Semicolon-delimited `name/ip` pairs in local-use ranges. |
| `A01_NO_GLOBALLY_REACHABLE_ADDR` | `-` | `-` | No arguments. |
| `A01_NO_NAME_SERVERS_FOUND` | `-` | `-` | No arguments. |
| `TEST_CASE_END` | `testcase` | `string` | Testcase display name (`Address01`). |
| `TEST_CASE_START` | `testcase` | `string` | Testcase display name (`Address01`). |

## Severity Levels Per Tag
| Tag | Level | Notes |
| --- | --- | --- |
| `A01_ADDR_NOT_GLOBALLY_REACHABLE` | `ERROR` | Default from `share/profile.json` (`test_levels.ADDRESS`). |
| `A01_DOCUMENTATION_ADDR` | `ERROR` | Default from `share/profile.json` (`test_levels.ADDRESS`). |
| `A01_GLOBALLY_REACHABLE_ADDR` | `INFO` | Default from `share/profile.json` (`test_levels.ADDRESS`). |
| `A01_LOCAL_USE_ADDR` | `ERROR` | Default from `share/profile.json` (`test_levels.ADDRESS`). |
| `A01_NO_GLOBALLY_REACHABLE_ADDR` | `ERROR` | Default from `share/profile.json` (`test_levels.ADDRESS`). |
| `A01_NO_NAME_SERVERS_FOUND` | `CRITICAL` | Default from `share/profile.json` (`test_levels.ADDRESS`). |
| `TEST_CASE_END` | `DEBUG` | Default from `share/profile.json` (`test_levels.ADDRESS`). |
| `TEST_CASE_START` | `DEBUG` | Default from `share/profile.json` (`test_levels.ADDRESS`). |

## Differences From Upstream
- Upstream reference: [`address01.md`](../../upstream/tests/Address-TP/address01.md)
- Differences (Upstream vs Gonemaster):
  - Upstream: does not define a strict order for category tag emission. Gonemaster: emits `A01_GLOBALLY_REACHABLE_ADDR`/`A01_NO_GLOBALLY_REACHABLE_ADDR` before other category tags.
  - Upstream: does not require a specific `ns_list` presentation order. Gonemaster: deduplicates and lexicographically sorts `ns_list` values before emission.
- Potential upstream report:
  - `no`

## Edge Cases And Limitations
- The testcase ignores nameserver entries that lack resolved IP addresses.
- If all discovered addresses are reserved/non-global, `A01_NO_GLOBALLY_REACHABLE_ADDR` can be emitted together with one or more category error tags.
- Multiple nameserver names sharing one IP are preserved as distinct `name/ip` entries in output lists.

---

Copyright (c) Patrik Wallström  
Copyright (c) The Swedish Internet Foundation (https://internetstiftelsen.se/en/)  
Copyright (c) AFNIC (https://www.afnic.fr/en/)  
All rights reserved.  

Copyright belongs to external contributor where applicable.  

Creative Commons Attribution 4.0 International License applies. See https://creativecommons.org/licenses/by/4.0/ for the license conditions.
