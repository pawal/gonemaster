# Consistency05 (consistency05)

Status: Draft

## Purpose
- Compare delegation glue addresses against child authoritative address data for in-bailiwick nameservers.
- Compare out-of-bailiwick glue addresses against recursive public lookup results.

## Preconditions And Inputs
- Preconditions:
  - A `zone.Zone` object is available.
- Required inputs:
  - Parent-side NS, A, and AAAA responses via `queryParentAll`.
  - Child-side nameserver names via `methods.Method2and3`.
  - Child-side nameserver servers via `methods.Method4and5`.
  - Recursive lookup results via `recurse` for out-of-bailiwick checks and referral fallbacks.
- Profile/config knobs that affect behavior:
  - `net.ipv4` and `net.ipv6`: filter which child nameserver addresses are queried.

## Algorithm And Decision Flow
1. Emit `TEST_CASE_START`.
2. Query parent for child-zone NS records; collect unique child NS names.
3. For each child NS name, query parent for A and AAAA and collect glue items `owner/ip`.
4. Split parent glue into:
   - in-bailiwick strict glue (`strictGlue`),
   - out-of-bailiwick extended glue (`extendedGlue` grouped by NS name).
5. Build in-bailiwick NS name set from Method2+Method3, and in-bailiwick child NS servers from Method4+Method5 (respecting enabled IP versions).
6. For each in-bailiwick NS name:
   - Query every in-bailiwick child NS server for A and AAAA with RD off (`getAddrRRs`).
   - `getAddrRRs` emits `NO_RESPONSE` on no response and `CHILD_NS_FAILED` on unusable non-referral/non-NXDOMAIN authoritative behavior.
   - Referral responses trigger recursive fallback lookup and use resulting answer data if available.
   - If all servers fail both A and AAAA for this NS name, emit `CHILD_ZONE_LAME`, emit `TEST_CASE_END`, and return.
   - Otherwise accumulate child authoritative `owner/ip` pairs.
7. Compare in-bailiwick sets:
   - Parent-only items -> emit `IN_BAILIWICK_ADDR_MISMATCH`.
   - Child-only items -> emit `EXTRA_ADDRESS_CHILD`.
8. For each out-of-bailiwick NS name in extended glue:
   - Recurse A and AAAA, build child/public `owner/ip` set.
   - If any parent glue item for that name is missing from child/public set, emit `OUT_OF_BAILIWICK_ADDR_MISMATCH`.
9. If none of the three mismatch tags were emitted, emit `ADDRESSES_MATCH`.
10. Emit `TEST_CASE_END`.

## Emitted Tags (Possible Set)
| Tag | Emitted when |
| --- | --- |
| `ADDRESSES_MATCH` | No in-bailiwick mismatch, no extra child address, and no out-of-bailiwick mismatch were found. |
| `CHILD_NS_FAILED` | Child nameserver response for in-bailiwick address lookup was unusable (non-AA/no referral/no accepted RCODE path). |
| `CHILD_ZONE_LAME` | For an in-bailiwick NS name, every queried child nameserver failed both A and AAAA lookup paths. |
| `EXTRA_ADDRESS_CHILD` | Child authoritative in-bailiwick address set contains addresses not present in strict glue. |
| `IN_BAILIWICK_ADDR_MISMATCH` | Strict in-bailiwick glue contains addresses not found in child authoritative data. |
| `NO_RESPONSE` | A child nameserver did not return a response for an in-bailiwick A/AAAA lookup. |
| `OUT_OF_BAILIWICK_ADDR_MISMATCH` | Out-of-bailiwick glue contains addresses not found in recursive public A/AAAA results. |
| `TEST_CASE_END` | Testcase completion marker is emitted. |
| `TEST_CASE_START` | Testcase start marker is emitted. |

## Tag Arguments
| Tag | Argument key | Type | Meaning |
| --- | --- | --- | --- |
| `ADDRESSES_MATCH` | `-` | `-` | No arguments. |
| `CHILD_NS_FAILED` | `ns` | `string` | Child nameserver identity (`name/ip`) that failed authoritative child lookup requirements. |
| `CHILD_ZONE_LAME` | `-` | `-` | No arguments. |
| `EXTRA_ADDRESS_CHILD` | `ns_ip_list` | `string` | Semicolon-delimited `owner/ip` entries found only in child authoritative data. |
| `IN_BAILIWICK_ADDR_MISMATCH` | `parent_addresses` | `string` | Semicolon-delimited strict glue `owner/ip` entries from parent. |
| `IN_BAILIWICK_ADDR_MISMATCH` | `zone_addresses` | `string` | Semicolon-delimited in-bailiwick child `owner/ip` entries. |
| `NO_RESPONSE` | `ns` | `string` | Child nameserver identity (`name/ip`) with no response. |
| `OUT_OF_BAILIWICK_ADDR_MISMATCH` | `parent_addresses` | `string` | Semicolon-delimited out-of-bailiwick glue addresses for one NS name. |
| `OUT_OF_BAILIWICK_ADDR_MISMATCH` | `zone_addresses` | `string` | Semicolon-delimited recursively resolved `owner/ip` entries for that NS name. |
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
- Upstream reference: [`consistency05.md`](../../upstream/tests/Consistency-TP/consistency05.md)
- Differences (Upstream vs Gonemaster):
  - Upstream: does not explicitly define this detail. Gonemaster: In-bailiwick processing queries all discovered in-bailiwick child servers and emits one `NO_RESPONSE` or `CHILD_NS_FAILED` entry per failing nameserver before final mismatch classification.
  - Upstream: does not explicitly define this detail. Gonemaster: Referral handling explicitly falls back to recursive lookup for the same qtype and owner.
- Potential upstream report:
  - `no`

## Edge Cases And Limitations
- `CHILD_ZONE_LAME` short-circuits testcase execution and suppresses later mismatch checks.
- Out-of-bailiwick mismatch reporting is per NS name group; each emission includes full parent list for that group.
- Disabled IP versions affect child authoritative probes indirectly by filtering queried child servers.

---

Copyright (c) Patrik Wallström  
Copyright (c) The Swedish Internet Foundation (https://internetstiftelsen.se/en/)  
Copyright (c) AFNIC (https://www.afnic.fr/en/)  
All rights reserved.  

Copyright belongs to external contributor where applicable.  

Creative Commons Attribution 4.0 International License applies. See https://creativecommons.org/licenses/by/4.0/ for the license conditions.
