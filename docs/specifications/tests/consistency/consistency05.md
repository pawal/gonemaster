# Consistency05

Status: Final

## Purpose
- Compare the delegation NS name set served by each responding parent nameserver.
- Compare delegation glue addresses against child authoritative address data for in-domain nameservers.
- Compare not-in-domain glue addresses against recursive public lookup results.

## Preconditions And Inputs
- Preconditions:
  - A `zone.Zone` object is available.
- Required inputs:
  - Parent-side NS responses via `queryParentAll`; NS records and glue A and AAAA records are read only from responses that qualify as usable referrals (step 3).
  - Child-side nameserver names via [`AllNSNames`](../../nameserver-resolution.md#allnsnames).
  - Child-side nameserver servers via [`AllNameservers`](../../nameserver-resolution.md#allnameservers).
  - Recursive lookup results via `recurse` for not-in-domain checks and referral fallbacks.
- Profile/config knobs that affect behavior:
  - `net.ipv4` and `net.ipv6`: filter which child nameserver addresses are queried.

## Algorithm And Decision Flow
1. Emit `TEST_CASE_START`.
2. Query parent for child-zone NS records.
3. Classify each parent response. A response is a **usable referral** when all of the following hold:
   - TC is clear,
   - RCODE is NOERROR,
   - the answer section is empty,
   - the authority section holds NS records owned by the child zone.

   A response failing any condition contributes neither NS names nor glue. AA is not consulted, since parents that set AA on referrals are still serving a referral.
4. Collect unique child NS names from the authority section of the usable referrals.
5. Read glue items `owner/ip` from the additional section of each usable referral, keeping only owners present in the authority NS set of that same response. Glue is never taken from separate address queries to the parent, so a parent that answers out-of-domain names directly cannot contribute bogus glue.
6. Group usable referrals by the delegation NS name set they carry:
   - The per-parent key is the sorted set of lowercased NS names from that response's authority section. Glue addresses and TTLs are not part of the key.
   - A parent server that does not respond, or whose response is not a usable referral, contributes no set and is excluded from the comparison.
   - With more than one distinct set, emit `MULTIPLE_DELEGATION_NS_SET` with the distinct-set count and `ns_names`, the NS names not served by every parent (the union of all sets minus their intersection), and one `DELEGATION_NS_SET` per distinct set naming the set elements and the parent servers that served it.
   - With zero or one distinct set, emit nothing. More than one distinct set implies a non-empty `ns_names`, since sets that differ must differ by at least one name.
7. Split parent glue into:
   - in-domain strict glue (`strictGlue`),
   - not-in-domain extended glue (`extendedGlue` grouped by NS name).
8. Build in-domain NS name set from [`AllNSNames`](../../nameserver-resolution.md#allnsnames), and in-domain child NS servers from [`AllNameservers`](../../nameserver-resolution.md#allnameservers) (respecting enabled IP versions).
9. If [`AllNameservers`](../../nameserver-resolution.md#allnameservers) yields no usable in-domain child NS servers:
   - Materialize child NS server endpoints from in-domain strict glue, respecting enabled IP versions.
   - Query those endpoints for the child-zone NS set and merge any in-domain names into the in-domain NS name set.
10. For each in-domain NS name:
   - Query every in-domain child NS server for A and AAAA with RD off (`getAddrRRs`).
   - `getAddrRRs` emits `NO_RESPONSE` on no response and `CHILD_NS_FAILED` on unusable non-referral/non-NXDOMAIN authoritative behavior.
   - Referral responses trigger recursive fallback lookup and use resulting answer data if available.
   - Otherwise accumulate child authoritative `owner/ip` pairs.
11. If no in-domain address lookup path was usable for any in-domain NS name, emit `CHILD_ZONE_LAME`, emit `TEST_CASE_END`, and return.
12. Compare in-domain glue against child address data, per NS name. Only names carrying at least one glue address take part; a name with no glue anywhere in the union is not compared, and is reported as missing glue by Delegation01 instead. For each such name, in sorted order:
   - Child serves no address record for the name -> emit `MISSING_ADDRESS_CHILD` with `ns`, and compare nothing further for that name.
   - Otherwise, glue addresses the child does not serve -> emit `IN_DOMAIN_ADDR_MISMATCH` with `ns`, `parent_servers` holding only those unconfirmed glue addresses, and `zone_servers` holding the child addresses for that name.
   - Independently, child addresses absent from that name's glue accumulate into the aggregate `EXTRA_ADDRESS_CHILD`. A name may therefore produce both an in-domain mismatch and a contribution to the extra-address list.
13. For each not-in-domain NS name in extended glue:
   - Recurse A and AAAA, build child/public `owner/ip` set.
   - If any parent glue item for that name is missing from child/public set, emit `NOT_IN_DOMAIN_ADDR_MISMATCH`.
14. If no address fault was found, emit `ADDRESSES_MATCH`. A delegation carrying no glue at all reaches this point with nothing to disagree about and is reported as matching. The delegation NS-set tags from step 6 do not affect this guard.
15. Emit `TEST_CASE_END`.

### Parent Glue and Child Address Lookup (steps 2-10)

{{% expand "Show diagram" %}}
```
queryParentAll(z, "NS")
 +- usable referral test, per response:
      resp.Msg != nil AND NOT resp.TC() AND rcode NOERROR
      AND answer section empty
      AND NS records owned by z in the authority section
      (AA is not consulted)
 +- collect distinct child NS names from the authority section
      of usable referrals
 +- read glue (A / AAAA) from the additional section of the same
      usable referral, keeping only owners in that response's
      authority NS set
      -> (owner lower / addr) glue items

group usable referrals by delegation NS name set (per response):
   resp that is not a usable referral -> not part of any set
   key = sorted lowercased NS names joined by ";" (no glue, no TTL)
   setServers[key] += resp.AnswerFrom (first-seen key order kept)
   >1 distinct key -> ns_names = union(keys) minus intersection(keys)
                      MULTIPLE_DELEGATION_NS_SET (count, ns_names)
                      DELEGATION_NS_SET per key (ns_set_servers, servers)
   0 or 1 distinct key -> no emission

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

### Domain-Relation Comparison and Final Emission (steps 11-15)

{{% expand "Show diagram" %}}
```
all in-domain lookups failed                -> CHILD_ZONE_LAME
                                                  emit TEST_CASE_END and return

compare in-domain glue per NS name (sorted, only names with glue):
  glueAddrs  = strict glue addresses for the name
  childAddrs = child authoritative addresses for the name
  childAddrs empty -> MISSING_ADDRESS_CHILD (ns); next name
  unconfirmed = glueAddrs NOT in childAddrs
     non-empty -> IN_DOMAIN_ADDR_MISMATCH (ns, parent_servers=unconfirmed,
                                              zone_servers=childAddrs)
  ibExtraChild += childAddrs NOT in glueAddrs
  ibExtraChild non-empty -> EXTRA_ADDRESS_CHILD (addresses)

  names with no glue at all are skipped: trimming cannot be told from a
  parent that never had glue, and Delegation01 reports missing glue

not-in-domain (per nsName in extendedGlue, sorted):
  recurse(z, nsName, "A");    add answers to childOOB
  recurse(z, nsName, "AAAA"); add answers to childOOB
  for each parent glue string at nsName missing from childOOB:
     append to mismatchForGlue and oobMismatch
  mismatchForGlue non-empty
     -> NOT_IN_DOMAIN_ADDR_MISMATCH (parent_servers, zone_servers)

no address fault emitted                  -> ADDRESSES_MATCH

emit TEST_CASE_END
```
{{% /expand %}}

## Emitted Tags (Possible Set)
| Tag | Emitted when |
| --- | --- |
| `ADDRESSES_MATCH` | No address fault was found. A glueless delegation has nothing to disagree about and reaches this tag. |
| `CHILD_NS_FAILED` | Child nameserver response for in-domain address lookup was unusable (non-AA/no referral/no accepted RCODE path). |
| `CHILD_ZONE_LAME` | Every in-domain address lookup path failed for all in-domain NS names. |
| `DELEGATION_NS_SET` | One distinct delegation NS name set (with the parent servers serving it), emitted per set when parents disagree. |
| `EXTRA_ADDRESS_CHILD` | For names that have glue, the child serves addresses not present in that name's glue. |
| `IN_DOMAIN_ADDR_MISMATCH` | An in-domain name's glue contains addresses the child does not serve, while the child serves at least one address for it. Emitted once per affected name. |
| `MISSING_ADDRESS_CHILD` | An in-domain name has glue in the delegation but the child zone serves no address record for it. Emitted once per affected name. |
| `MULTIPLE_DELEGATION_NS_SET` | Responding parent nameservers serve more than one distinct delegation NS name set. |
| `NO_RESPONSE` | A child nameserver did not return a response for an in-domain A/AAAA lookup. |
| `NOT_IN_DOMAIN_ADDR_MISMATCH` | Not-in-domain glue contains addresses not found in recursive public A/AAAA results. |
| `TEST_CASE_END` | Testcase completion marker is emitted. |
| `TEST_CASE_START` | Testcase start marker is emitted. |

## Tag Arguments
| Tag | Argument key | Type | Meaning |
| --- | --- | --- | --- |
| `ADDRESSES_MATCH` | `-` | `-` | No arguments. |
| `CHILD_NS_FAILED` | `ns` | `string` | Child nameserver identity (`ns` name only; use `address` for IP) that failed authoritative child lookup requirements. |
| `CHILD_NS_FAILED` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `CHILD_ZONE_LAME` | `-` | `-` | No arguments. |
| `DELEGATION_NS_SET` | `ns_set_servers` | `array<object>` | Structured delegation NS-set elements, one `{ "ns": "..." }` per NS name in the set. |
| `DELEGATION_NS_SET` | `servers` | `array<object>` | Structured parent server endpoints that served this set. |
| `EXTRA_ADDRESS_CHILD` | `addresses` | `array<string>` | Structured `owner/ip` entries found only in child authoritative data. |
| `IN_DOMAIN_ADDR_MISMATCH` | `ns` | `string` | The in-domain nameserver name this mismatch belongs to. |
| `IN_DOMAIN_ADDR_MISMATCH` | `parent_servers` | `array<object>` | Structured list of this name's glue addresses that the child does not serve; each item is `{ "ns": "...", "address": "..." }`. |
| `IN_DOMAIN_ADDR_MISMATCH` | `zone_servers` | `array<object>` | Structured list of the child authoritative addresses for the same name; each item is `{ "ns": "...", "address": "..." }`. |
| `MISSING_ADDRESS_CHILD` | `ns` | `string` | The in-domain nameserver name that has glue but no address record in the child zone. |
| `MULTIPLE_DELEGATION_NS_SET` | `count` | `int` | Number of distinct delegation NS name sets observed. |
| `MULTIPLE_DELEGATION_NS_SET` | `ns_names` | `array<string>` | Sorted NS names not served by every parent: the union of the observed sets minus their intersection. Never empty when the tag is emitted. |
| `NO_RESPONSE` | `ns` | `string` | Child nameserver identity (`ns` name only; use `address` for IP) with no response. |
| `NO_RESPONSE` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `NOT_IN_DOMAIN_ADDR_MISMATCH` | `parent_servers` | `array<object>` | Structured parent not-in-domain glue endpoint list for one NS name. |
| `NOT_IN_DOMAIN_ADDR_MISMATCH` | `zone_servers` | `array<object>` | Structured recursively resolved endpoint list for the same NS name. |
| `TEST_CASE_END` | `testcase` | `string` | Testcase display name (`Consistency05`). |
| `TEST_CASE_START` | `testcase` | `string` | Testcase display name (`Consistency05`). |

## Severity Levels Per Tag
| Tag | Level | Notes |
| --- | --- | --- |
| `ADDRESSES_MATCH` | `INFO` | Default from `share/profile.json` (`test_levels.CONSISTENCY`). |
| `CHILD_NS_FAILED` | `DEBUG` | Default from `share/profile.json` (`test_levels.CONSISTENCY`). |
| `CHILD_ZONE_LAME` | `ERROR` | Default from `share/profile.json` (`test_levels.CONSISTENCY`). |
| `DELEGATION_NS_SET` | `INFO` | Default from `share/profile.json` (`test_levels.CONSISTENCY`). |
| `EXTRA_ADDRESS_CHILD` | `NOTICE` | Default from `share/profile.json` (`test_levels.CONSISTENCY`). |
| `IN_DOMAIN_ADDR_MISMATCH` | `ERROR` | Default from `share/profile.json` (`test_levels.CONSISTENCY`). |
| `MISSING_ADDRESS_CHILD` | `NOTICE` | Default from `share/profile.json` (`test_levels.CONSISTENCY`). |
| `MULTIPLE_DELEGATION_NS_SET` | `WARNING` | Default from `share/profile.json` (`test_levels.CONSISTENCY`). |
| `NO_RESPONSE` | `DEBUG` | Default from `share/profile.json` (`test_levels.CONSISTENCY`). |
| `NOT_IN_DOMAIN_ADDR_MISMATCH` | `ERROR` | Default from `share/profile.json` (`test_levels.CONSISTENCY`). |
| `TEST_CASE_END` | `DEBUG` | Default from `share/profile.json` (`test_levels.CONSISTENCY`). |
| `TEST_CASE_START` | `DEBUG` | Default from `share/profile.json` (`test_levels.CONSISTENCY`). |

## Differences From Upstream
- Differences (Upstream vs Gonemaster):
  - Upstream: names the mismatch tags `IN_BAILIWICK_ADDR_MISMATCH` and `OUT_OF_BAILIWICK_ADDR_MISMATCH`. Gonemaster: uses RFC 9499 terminology, `IN_DOMAIN_ADDR_MISMATCH` and `NOT_IN_DOMAIN_ADDR_MISMATCH`; the upstream names stay accepted as profile aliases.
  - Upstream: outputs `IN_BAILIWICK_ADDR_MISMATCH` (`ERROR`) per unconfirmed glue address, including when the child serves no address at all for the name. Gonemaster: reports once per NS name, and splits the serves-nothing case out as `MISSING_ADDRESS_CHILD` (`NOTICE`).
  - Upstream: outputs `EXTRA_ADDRESS_CHILD` per child address missing from glue, for every in-domain name including names with no glue. Gonemaster: aggregates them into one `EXTRA_ADDRESS_CHILD` and compares only names that carry glue, because trimmed glue cannot be told from glue that never existed; Delegation01 reports glue missing from the delegation.
  - Upstream: emits `CHILD_ZONE_LAME` when every server fails for one in-domain NS name. Gonemaster: emits it only when every in-domain NS name failed on every server, so disjoint parent/child NS sets can still be classified as address mismatches.
  - Upstream: builds the child-side address server set from parent glue and child address records together. Gonemaster: uses [`AllNameservers`](../../nameserver-resolution.md#allnameservers), and falls back to strict-glue endpoints only when that yields no usable endpoint.
  - Upstream: follows a referral with a recursive lookup only when it points into a sub-zone of the child zone. Gonemaster: applies that fallback to any referral response.
  - Upstream: does not compare delegations between parent nameservers. Gonemaster: groups responding parents by the delegation NS name set they serve and reports `MULTIPLE_DELEGATION_NS_SET` with one `DELEGATION_NS_SET` per set, reusing the NS responses already collected for glue (no extra queries).
- Potential upstream report:
  - `no`

## Implementation Notes

The delegation comparison is bounded by what the protocol lets a server omit without signalling it. Four rules apply, and they differ by section and response type:

- **RFC 9471 section 3.1**: a referral MUST carry all available glue for in-domain names, or set TC=1 when it does not fit.
- **RFC 9471 section 3.2**: sibling-domain glue SHOULD be included; when it does not fit the server MAY set TC=1 but is not obliged to.
- **RFC 9609 section 4.2**: a priming response is an answer and not a referral, so RFC 9471 does not apply; addresses may be omitted from its additional section with no expectation that TC is set.
- **RFC 2181 section 9**: a response with TC=1 may carry the partial RRset that did not fit, so its body must not be consumed; the query is retried over a transport that fits the whole answer.

Deployed parents do omit in-domain referral glue with TC clear, contrary to RFC 9471 section 3.1, so neither lawful trimming nor a compliant TC signal can be assumed. Two consequences follow:

- **Cross-parent equality uses the authority section only**. An incomplete NS RRset can reach the client only together with TC=1, so a difference observed between TC-clear authority sections is real. Glue may arrive incomplete with no signal at all, so it cannot key a cross-parent equality check; glue faults are instead reported from the union of glue across all parents, where omission can only hide a fault and never invent one.
- **TC-set responses are skipped by this testcase**, not only by the transport. Truncated responses are normally replaced by a TCP retry (`resolver.defaults.fallback`), but with fallback disabled the truncated UDP response is returned as is, and per RFC 2181 section 9 its partial authority section would otherwise fabricate a set difference.

The same asymmetry governs the address comparison:

- **Glue is compared as a union across all parents**, so a name's glue is the set of addresses any parent supplied for it. Trimming can only shrink that union, never add to it, so it can hide a fault but cannot invent one.
- **Names with no glue in the union are not compared at all.** A parent that never had glue for a name cannot be told apart from parents that all trimmed the same name, so comparing such a name would report trimming as a fault. Glue absent from the delegation is reported once per name by Delegation01's `IN_DOMAIN_GLUE_MISSING`.
- **A missing address record is not a wrong one.** Glue that the child zone does not confirm misdirects resolvers and stays an ERROR. A glued name for which the child serves no address at all still resolves while the glue is served, and is reported separately at NOTICE.

## Edge Cases And Limitations
- Glue is taken only from the additional section of a usable referral, so a parent name server that answers out-of-domain names (for example via a catch-all or wildcard zone) does not inject spurious glue and cannot trigger a false `NOT_IN_DOMAIN_ADDR_MISMATCH`.
- A parent server that does not respond, or whose response is not a usable referral, is not part of any delegation NS set; non-response can never produce `MULTIPLE_DELEGATION_NS_SET`. With fewer than two usable referrals the delegation comparison is skipped entirely.
- The delegation NS-set comparison is silent when all responding parents agree; there is no positive confirmation tag, and `ADDRESSES_MATCH` remains governed only by the address comparisons.
- Glue trimmed from the additional section, whether lawfully or not, does not split the delegation, because the per-parent key holds NS names only.
- A parent that is also authoritative for the child zone answers from the answer section instead of referring. `arpa` is the live case: the root servers serve it directly. Such responses are not usable referrals and contribute no delegation set and no glue.
- The root zone is its own parent, so a test of `.` sends priming queries. Per RFC 9609 the responses are answers and not referrals, so they are not usable referrals: a test of the root produces no delegation NS-set finding.
- Undelegated tests produce synthetic, identical delegations, so the delegation NS-set comparison stays silent there.
- `CHILD_ZONE_LAME` short-circuits testcase execution and suppresses later mismatch checks when no usable in-domain address lookup path was found.
- In-domain mismatch reporting is per NS name: a zone with several names carrying wrong glue produces one `IN_DOMAIN_ADDR_MISMATCH` per name, each naming only that name's unconfirmed addresses.
- A single name can produce both `IN_DOMAIN_ADDR_MISMATCH` and a contribution to `EXTRA_ADDRESS_CHILD`, when its glue and its child addresses each hold entries the other lacks.
- A zone whose parents supply no usable glue, including the root zone and a delegation whose nameservers are all not-in-domain, has nothing to compare and reports `ADDRESSES_MATCH`.
- Not-in-domain mismatch reporting is per NS name group; each emission includes full parent list for that group.
- Disabled IP versions affect child authoritative probes indirectly by filtering queried child servers.
