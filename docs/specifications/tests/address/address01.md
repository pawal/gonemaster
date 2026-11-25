# Address01

Status: Final

## Purpose
- Classify authoritative nameserver IP addresses as IPv4-embedded, globally reachable, documentation, local-use, or otherwise not globally reachable.
- Flag nameserver addresses that embed an IPv4 address in IPv6 form. RFC 10001 section 4.1: authoritative DNS servers "MUST NOT use IPv4-embedded addresses [RFC6052] (including IPv4-Mapped IPv6 addresses and deprecated IPv4-compatible addresses [RFC4291]) for receiving queries."
- Detect the case where no nameserver address data is available.

## Preconditions And Inputs
- Preconditions:
  - A `zone.Zone` object is available.
  - A recursor is available on the zone object for Methodsv2 lookups.
- Required inputs:
  - Delegation nameserver names and addresses from `GetDelNSNamesAndIPs`.
  - Zone-apex nameserver names and addresses from `GetZoneNSNamesAndIPs`.
  - IPv4-embedded prefixes via `constants.IsIPv4Embedded`.
  - Special-purpose address registry data (RFC 6890 section 2.2) via `constants.FindSpecialAddress`.
- Profile/config knobs that affect behavior:
  - `net.ipv4` and `net.ipv6` may affect address discovery in Methodsv2 lookups.

## Algorithm And Decision Flow
1. Emit `TEST_CASE_START`.
2. Query nameserver names/addresses using `GetDelNSNamesAndIPs` and `GetZoneNSNamesAndIPs`.
3. Merge the two result sets, keep only entries with an address, and deduplicate by lowercase `name/ip`.
4. If no addressed nameserver remains, emit `A01_NO_NAME_SERVERS_FOUND`, emit `TEST_CASE_END`, and return.
5. Group items by IP address, then iterate IP groups in lexicographic IP string order.
6. For each IP group, test the address with `constants.IsIPv4Embedded` first. A match adds all `name/ip` pairs in the group to the embedded set and skips the registry classification. The embedded forms are:
   - IPv4-mapped `::ffff:0:0/96` (RFC 4291 section 2.5.5.2),
   - IPv4-compatible `::/96`, other than `::` and `::1` (RFC 4291 section 2.5.5.1, deprecated),
   - NAT64 well-known prefix `64:ff9b::/96` (RFC 6052 section 2.1),
   - local-use IPv4/IPv6 translation prefix `64:ff9b:1::/48` (RFC 8215).
7. Otherwise classify the IP group by the category of the most specific special-purpose block that contains the address, that is the longest matching prefix; a broader block defers to a more specific entry (RFC 6890 sections 2.2.2 and 2.2.3):
   - Documentation category -> add all `name/ip` pairs in group to documentation set.
   - Local-use categories (`Private-Use`, `Loopback`, `Link Local`, `Link-Local`, `Unique-Local`, `Shared Address Space`) -> add to local-use set.
   - Any other non-globally-reachable special range -> add to not-globally-reachable set.
   - Otherwise -> add to globally-reachable set.
8. If globally-reachable set is non-empty, emit `A01_GLOBALLY_REACHABLE_ADDR` with `servers`; else emit `A01_NO_GLOBALLY_REACHABLE_ADDR`.
9. Emit `A01_DOCUMENTATION_ADDR` if documentation set is non-empty.
10. Emit `A01_LOCAL_USE_ADDR` if local-use set is non-empty.
11. Emit `A01_ADDR_NOT_GLOBALLY_REACHABLE` if not-globally-reachable set is non-empty.
12. Emit `A01_IPV4_EMBEDDED_IPV6_ADDR` if embedded set is non-empty.
13. Emit `TEST_CASE_END`.

### Address Collection and Classification (steps 2-7)

{{% expand "Show diagram" %}}
```
gather NS items (GetDelNSNamesAndIPs + GetZoneNSNamesAndIPs)
 |
 +- per item: keep only if item.HasAddress
 +- dedupe by lower-case "name/ip" string
 |
 v
addressed NS count
 +- zero    -> emit A01_NO_NAME_SERVERS_FOUND
 |              emit TEST_CASE_END and return
 +- nonzero -> group by Address.String(); iterate IP keys in lex order
                 |
                 v
              IsIPv4Embedded(ip)  (on group[0])
                +- match -> ipv4Embedded
                +- no match:
                     FindSpecialAddress(ip)  (longest prefix, on group[0])
                       +- no special-purpose block         -> globallyReachable
                       +- block.Name contains "Documentation" -> documentationAddr
                       +- isLocalUseCategory(block.Name)   -> localUseAddr
                       |     ("Private-Use", "Loopback",
                       |      "Link Local", "Link-Local",
                       |      "Unique-Local", "Shared Address Space")
                       +- !IsGloballyReachable(block)      -> notGloballyReachable
                       +- otherwise                        -> globallyReachable
```
{{% /expand %}}

### Aggregation and Final Emission (steps 8-13)

{{% expand "Show diagram" %}}
```
After classification, emit in fixed order:

1. globallyReachable
     non-empty -> A01_GLOBALLY_REACHABLE_ADDR     (servers)
     empty     -> A01_NO_GLOBALLY_REACHABLE_ADDR  (no args)

2. documentationAddr non-empty    -> A01_DOCUMENTATION_ADDR          (servers)
3. localUseAddr non-empty         -> A01_LOCAL_USE_ADDR              (servers)
4. notGloballyReachable non-empty -> A01_ADDR_NOT_GLOBALLY_REACHABLE (servers)
5. ipv4Embedded non-empty         -> A01_IPV4_EMBEDDED_IPV6_ADDR     (servers)

6. emit TEST_CASE_END
```
{{% /expand %}}

## Emitted Tags (Possible Set)
| Tag | Emitted when |
| --- | --- |
| `A01_ADDR_NOT_GLOBALLY_REACHABLE` | At least one nameserver IP is in a special-purpose range that is not globally reachable (RFC 8190 section 2.1) and not classified as embedded, documentation, or local-use. |
| `A01_DOCUMENTATION_ADDR` | At least one nameserver IP is in a documentation-only range (RFC 5737 section 4, RFC 3849 section 3, RFC 9637 section 4). |
| `A01_GLOBALLY_REACHABLE_ADDR` | At least one nameserver IP is classified as globally reachable. |
| `A01_IPV4_EMBEDDED_IPV6_ADDR` | At least one nameserver IP is an IPv4-mapped, IPv4-compatible, or NAT64 translation address (RFC 10001 section 4.1). |
| `A01_LOCAL_USE_ADDR` | At least one nameserver IP is in a local-use category, none of which is globally reachable (RFC 8190 section 2.1). |
| `A01_NO_GLOBALLY_REACHABLE_ADDR` | No nameserver IP is classified as globally reachable. |
| `A01_NO_NAME_SERVERS_FOUND` | No nameserver with an address was discovered from delegation+zone sources. |
| `CNAME_CHAIN_TOO_LONG` | A discovered NS hostname's CNAME chain exceeds `CNAMEMaxChainLength` while resolving its address. |
| `CNAME_TARGET_UNRESOLVED` | A discovered NS hostname's CNAME chain forms a loop, breaks, or fails to resolve to an address (RFC 1034 section 5.2.2). |
| `CNAME_TOO_MANY_RECORDS` | A single answer while resolving a discovered NS hostname carries more than `CNAMEMaxRecords` distinct CNAME RRs. |
| `TEST_CASE_END` | Testcase completion marker is emitted. |
| `TEST_CASE_START` | Testcase start marker is emitted. |

## Tag Arguments
| Tag | Argument key | Type | Meaning |
| --- | --- | --- | --- |
| `A01_ADDR_NOT_GLOBALLY_REACHABLE` | `servers` | `array<object>` | Structured `{ns,address}` object pairs in the non-globally-reachable set. |
| `A01_DOCUMENTATION_ADDR` | `servers` | `array<object>` | Structured `{ns,address}` object pairs in documentation ranges. |
| `A01_GLOBALLY_REACHABLE_ADDR` | `servers` | `array<object>` | Structured `{ns,address}` object pairs in globally reachable ranges. |
| `A01_IPV4_EMBEDDED_IPV6_ADDR` | `servers` | `array<object>` | Structured `{ns,address}` object pairs with IPv4-embedded addresses. |
| `A01_LOCAL_USE_ADDR` | `servers` | `array<object>` | Structured `{ns,address}` object pairs in local-use ranges. |
| `A01_NO_GLOBALLY_REACHABLE_ADDR` | `-` | `-` | No arguments. |
| `A01_NO_NAME_SERVERS_FOUND` | `-` | `-` | No arguments. |
| `CNAME_CHAIN_TOO_LONG` | `query_name` | `string` | The NS hostname whose CNAME chain exceeded the depth bound. |
| `CNAME_TARGET_UNRESOLVED` | `query_name` | `string` | The NS hostname whose CNAME target could not be resolved. |
| `CNAME_TARGET_UNRESOLVED` | `cname_target` | `string` | The last attempted CNAME target. |
| `CNAME_TOO_MANY_RECORDS` | `query_name` | `string` | The NS hostname whose answer carried too many CNAME RRs. |
| `TEST_CASE_END` | `testcase` | `string` | Testcase display name (`Address01`). |
| `TEST_CASE_START` | `testcase` | `string` | Testcase display name (`Address01`). |

## Severity Levels Per Tag
| Tag | Level | Notes |
| --- | --- | --- |
| `A01_ADDR_NOT_GLOBALLY_REACHABLE` | `ERROR` | Default from `share/profile.json` (`test_levels.ADDRESS`). |
| `A01_DOCUMENTATION_ADDR` | `ERROR` | Default from `share/profile.json` (`test_levels.ADDRESS`). |
| `A01_GLOBALLY_REACHABLE_ADDR` | `INFO` | Default from `share/profile.json` (`test_levels.ADDRESS`). |
| `A01_IPV4_EMBEDDED_IPV6_ADDR` | `ERROR` | Default from `share/profile.json` (`test_levels.ADDRESS`). RFC 10001 section 4.1 MUST NOT. |
| `A01_LOCAL_USE_ADDR` | `ERROR` | Default from `share/profile.json` (`test_levels.ADDRESS`). |
| `A01_NO_GLOBALLY_REACHABLE_ADDR` | `ERROR` | Default from `share/profile.json` (`test_levels.ADDRESS`). |
| `A01_NO_NAME_SERVERS_FOUND` | `CRITICAL` | Default from `share/profile.json` (`test_levels.ADDRESS`). |
| `CNAME_CHAIN_TOO_LONG` | `ERROR` | Default from `share/profile.json` (`test_levels.ADDRESS`). |
| `CNAME_TARGET_UNRESOLVED` | `ERROR` | Default from `share/profile.json` (`test_levels.ADDRESS`). |
| `CNAME_TOO_MANY_RECORDS` | `ERROR` | Default from `share/profile.json` (`test_levels.ADDRESS`). |
| `TEST_CASE_END` | `DEBUG` | Default from `share/profile.json` (`test_levels.ADDRESS`). |
| `TEST_CASE_START` | `DEBUG` | Default from `share/profile.json` (`test_levels.ADDRESS`). |

## Differences From Upstream
- Differences (Upstream vs Gonemaster):
  - Upstream: does not define a strict order for category tag emission. Gonemaster: emits `A01_GLOBALLY_REACHABLE_ADDR`/`A01_NO_GLOBALLY_REACHABLE_ADDR` before other category tags.
  - Upstream: does not require a specific `servers` presentation order. Gonemaster: deduplicates and lexicographically sorts `servers` values before emission.
  - Upstream: classifies every address by the special-purpose registries alone, so `64:ff9b::/96` (registered as globally reachable) and `::/96` (no registry entry) count as globally reachable, while `::ffff:0:0/96` and `64:ff9b:1::/48` count as not globally reachable. Gonemaster: claims all four for `A01_IPV4_EMBEDDED_IPV6_ADDR` before the registry lookup (RFC 10001 section 4.1).
- Potential upstream report:
  - `yes` (feature proposal: IPv4-embedded nameserver addresses per RFC 10001 section 4.1; not filed)

## Edge Cases And Limitations
- The testcase ignores nameserver entries that lack resolved IP addresses.
- If all discovered addresses are reserved/non-global, `A01_NO_GLOBALLY_REACHABLE_ADDR` can be emitted together with one or more category error tags.
- An embedded address is never in the globally reachable set, so a zone whose addresses are all embedded also emits `A01_NO_GLOBALLY_REACHABLE_ADDR`.
- Each address gets one diagnosis. An `::ffff:0:0/96` or `64:ff9b:1::/48` address, which the registry marks not globally reachable, emits `A01_IPV4_EMBEDDED_IPV6_ADDR` and not `A01_ADDR_NOT_GLOBALLY_REACHABLE`.
- The address items keep the IPv6 form of an IPv4-mapped AAAA record; Delegation01 unmaps the same address and counts it as IPv4.
- NAT64 network-specific prefixes (RFC 6052 section 2.2) are indistinguishable from ordinary global unicast addresses without knowledge of the operator's network and are not detected.
- Multiple nameserver names sharing one IP are preserved as distinct `name/ip` entries in output lists.
- An address in a block nested inside a broader one, such as AS112-v6 (`2001:4:112::/48`) inside IETF Protocol Assignments (`2001::/23`), is classified by the nested block and its globally reachable flag.
