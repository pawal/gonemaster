# Consistency05

Status: Final

## Purpose
- Compare delegation glue addresses against child authoritative address data for in-domain nameservers.
- Compare not-in-domain glue addresses against recursive public lookup results.

## Preconditions And Inputs
- Preconditions:
  - A `zone.Zone` object is available.
- Required inputs:
  - Parent-side NS, A, and AAAA responses via `queryParentAll`.
  - Child-side nameserver names via [`AllNSNames`](../../nameserver-resolution.md#allnsnames).
  - Child-side nameserver servers via [`AllNameservers`](../../nameserver-resolution.md#allnameservers).
  - Recursive lookup results via `recurse` for not-in-domain checks and referral fallbacks.
- Profile/config knobs that affect behavior:
  - `net.ipv4` and `net.ipv6`: filter which child nameserver addresses are queried.

## Algorithm And Decision Flow
1. Emit `TEST_CASE_START`.
2. Query parent for child-zone NS records; collect unique child NS names.
3. For each child NS name, query parent for A and AAAA and collect glue items `owner/ip`.
4. Split parent glue into:
   - in-domain strict glue (`strictGlue`),
   - not-in-domain extended glue (`extendedGlue` grouped by NS name).
5. Build in-domain NS name set from [`AllNSNames`](../../nameserver-resolution.md#allnsnames), and in-domain child NS servers from [`AllNameservers`](../../nameserver-resolution.md#allnameservers) (respecting enabled IP versions).
6. If [`AllNameservers`](../../nameserver-resolution.md#allnameservers) yields no usable in-domain child NS servers:
   - Materialize child NS server endpoints from in-domain strict glue, respecting enabled IP versions.
   - Query those endpoints for the child-zone NS set and merge any in-domain names into the in-domain NS name set.
7. For each in-domain NS name:
   - Query every in-domain child NS server for A and AAAA with RD off (`getAddrRRs`).
   - `getAddrRRs` emits `NO_RESPONSE` on no response and `CHILD_NS_FAILED` on unusable non-referral/non-NXDOMAIN authoritative behavior.
   - Referral responses trigger recursive fallback lookup and use resulting answer data if available.
   - Otherwise accumulate child authoritative `owner/ip` pairs.
8. If no in-domain address lookup path was usable for any in-domain NS name, emit `CHILD_ZONE_LAME`, emit `TEST_CASE_END`, and return.
9. Compare in-domain sets:
   - Parent-only items -> emit `IN_BAILIWICK_ADDR_MISMATCH`.
   - Child-only items -> emit `EXTRA_ADDRESS_CHILD`.
10. For each not-in-domain NS name in extended glue:
   - Recurse A and AAAA, build child/public `owner/ip` set.
   - If any parent glue item for that name is missing from child/public set, emit `OUT_OF_BAILIWICK_ADDR_MISMATCH`.
11. If none of the three mismatch tags were emitted, emit `ADDRESSES_MATCH`.
12. Emit `TEST_CASE_END`.

### Parent Glue and Child Address Lookup (steps 2-7)

{{% expand "Show diagram" %}}
```
queryParentAll(z, "NS")
 +- collect distinct child NS names from NS answer records

For each child NS name (sorted):
   queryParentAll(name, "A");    collect (owner lower / addr) glue items
   queryParentAll(name, "AAAA"); collect (owner lower / addr) glue items

split parent glue by domain relation:
   z.Name.IsInBailiwick(nsName) -> strictGlue[owner/ip]
   otherwise                    -> extendedGlue[nsName] += "owner/ip"

build in-domain name set + NS server set:
   inBailiwickNames  = AllNSNames names filtered by IsInBailiwick
   inBailiwickServers = AllNameservers filtered by Net.IPv4 / Net.IPv6 enable
   inBailiwickServers empty AND strict glue available:
       fall back to strictGlue endpoints (filtered by Net.IPv4/IPv6)
       extend inBailiwickNames with names learned from those endpoints

For each in-domain NS name:
   for each in-domain child NS server:
     getAddrRRs(ns, name, "A")    \
     getAddrRRs(ns, name, "AAAA") /  per qtype:
        +- error / no resp.Msg          -> NO_RESPONSE (child ns), no records
        +- IsRedirect                   -> recurse(z, name, qtype); use answer
        +- AA + NOERROR                 -> use answer records
        +- AA + NXDOMAIN                -> no records (treated as success)
        +- otherwise                    -> CHILD_NS_FAILED (child ns), no records

     records found accumulate into childIBStrings (owner/ip set)
     a child NS attempt is "successful" if neither qtype emitted NO_RESPONSE
     a name's lookups "failed" only if every server failed for both qtypes
     overall "all failed" only if every in-domain name had all-failed lookups
```
{{% /expand %}}

### Domain-Relation Comparison and Final Emission (steps 8-12)

{{% expand "Show diagram" %}}
```
all in-domain lookups failed                -> CHILD_ZONE_LAME
                                                  emit TEST_CASE_END and return

compare in-domain sets:
  ibMismatch     = strictGlue keys NOT in childIBStrings
  ibExtraChild   = childIBStrings keys NOT in strictGlue
  ibMismatch non-empty   -> IN_BAILIWICK_ADDR_MISMATCH (parent_servers, zone_servers)
  ibExtraChild non-empty -> EXTRA_ADDRESS_CHILD (addresses)

not-in-domain (per nsName in extendedGlue, sorted):
  recurse(z, nsName, "A");    add answers to childOOB
  recurse(z, nsName, "AAAA"); add answers to childOOB
  for each parent glue string at nsName missing from childOOB:
     append to mismatchForGlue and oobMismatch
  mismatchForGlue non-empty
     -> OUT_OF_BAILIWICK_ADDR_MISMATCH (parent_servers, zone_servers)

ibMismatch empty AND ibExtraChild empty AND oobMismatch empty
                                          -> ADDRESSES_MATCH

emit TEST_CASE_END
```
{{% /expand %}}

## Emitted Tags (Possible Set)
| Tag | Emitted when |
| --- | --- |
| `ADDRESSES_MATCH` | No in-domain mismatch, no extra child address, and no not-in-domain mismatch were found. |
| `CHILD_NS_FAILED` | Child nameserver response for in-domain address lookup was unusable (non-AA/no referral/no accepted RCODE path). |
| `CHILD_ZONE_LAME` | Every in-domain address lookup path failed for all in-domain NS names. |
| `EXTRA_ADDRESS_CHILD` | Child authoritative in-domain address set contains addresses not present in strict glue. |
| `IN_BAILIWICK_ADDR_MISMATCH` | Strict in-domain glue contains addresses not found in child authoritative data. |
| `NO_RESPONSE` | A child nameserver did not return a response for an in-domain A/AAAA lookup. |
| `OUT_OF_BAILIWICK_ADDR_MISMATCH` | Not-in-domain glue contains addresses not found in recursive public A/AAAA results. |
| `TEST_CASE_END` | Testcase completion marker is emitted. |
| `TEST_CASE_START` | Testcase start marker is emitted. |

## Tag Arguments
| Tag | Argument key | Type | Meaning |
| --- | --- | --- | --- |
| `ADDRESSES_MATCH` | `-` | `-` | No arguments. |
| `CHILD_NS_FAILED` | `ns` | `string` | Child nameserver identity (`ns` name only; use `address` for IP) that failed authoritative child lookup requirements. |
| `CHILD_NS_FAILED` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `CHILD_ZONE_LAME` | `-` | `-` | No arguments. |
| `EXTRA_ADDRESS_CHILD` | `addresses` | `array<string>` | Structured `owner/ip` entries found only in child authoritative data. |
| `IN_BAILIWICK_ADDR_MISMATCH` | `parent_servers` | `array<object>` | Structured parent strict-glue endpoint list; each item is `{ "ns": "...", "address": "..." }`. |
| `IN_BAILIWICK_ADDR_MISMATCH` | `zone_servers` | `array<object>` | Structured child authoritative in-domain endpoint list; each item is `{ "ns": "...", "address": "..." }`. |
| `NO_RESPONSE` | `ns` | `string` | Child nameserver identity (`ns` name only; use `address` for IP) with no response. |
| `NO_RESPONSE` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `OUT_OF_BAILIWICK_ADDR_MISMATCH` | `parent_servers` | `array<object>` | Structured parent not-in-domain glue endpoint list for one NS name. |
| `OUT_OF_BAILIWICK_ADDR_MISMATCH` | `zone_servers` | `array<object>` | Structured recursively resolved endpoint list for the same NS name. |
| `TEST_CASE_END` | `testcase` | `string` | Testcase display name (`Consistency05`). |
| `TEST_CASE_START` | `testcase` | `string` | Testcase display name (`Consistency05`). |

## Severity Levels Per Tag
| Tag | Level | Notes |
| --- | --- | --- |
| `ADDRESSES_MATCH` | `INFO` | Default from `share/profile.json` (`test_levels.CONSISTENCY`). |
| `CHILD_NS_FAILED` | `DEBUG` | Default from `share/profile.json` (`test_levels.CONSISTENCY`). |
| `CHILD_ZONE_LAME` | `ERROR` | Default from `share/profile.json` (`test_levels.CONSISTENCY`). |
| `EXTRA_ADDRESS_CHILD` | `NOTICE` | Default from `share/profile.json` (`test_levels.CONSISTENCY`). |
| `IN_BAILIWICK_ADDR_MISMATCH` | `ERROR` | Default from `share/profile.json` (`test_levels.CONSISTENCY`). |
| `NO_RESPONSE` | `DEBUG` | Default from `share/profile.json` (`test_levels.CONSISTENCY`). |
| `OUT_OF_BAILIWICK_ADDR_MISMATCH` | `ERROR` | Default from `share/profile.json` (`test_levels.CONSISTENCY`). |
| `TEST_CASE_END` | `DEBUG` | Default from `share/profile.json` (`test_levels.CONSISTENCY`). |
| `TEST_CASE_START` | `DEBUG` | Default from `share/profile.json` (`test_levels.CONSISTENCY`). |

## Differences From Upstream
- Differences (Upstream vs Gonemaster):
  - Upstream: does not explicitly define this detail. Gonemaster: In-domain processing queries all discovered in-domain child servers and emits one `NO_RESPONSE` or `CHILD_NS_FAILED` entry per failing nameserver before final mismatch classification.
  - Upstream: does not explicitly define this detail. Gonemaster: Referral handling explicitly falls back to recursive lookup for the same qtype and owner.
  - Upstream: the short-circuit wording can be read per in-domain NS name. Gonemaster: `CHILD_ZONE_LAME` is emitted only when all in-domain address lookup paths fail, so disjoint parent/child NS sets can still be classified as address mismatches.
  - Upstream: does not explicitly define this detail. Gonemaster: If [`AllNameservers`](../../nameserver-resolution.md#allnameservers) cannot produce usable in-domain child NS endpoints, strict glue endpoints are used as a fallback for child-side address checks and child NS name discovery.
- Potential upstream report:
  - `no`

## Edge Cases And Limitations
- `CHILD_ZONE_LAME` short-circuits testcase execution and suppresses later mismatch checks when no usable in-domain address lookup path was found.
- Not-in-domain mismatch reporting is per NS name group; each emission includes full parent list for that group.
- Disabled IP versions affect child authoritative probes indirectly by filtering queried child servers.
