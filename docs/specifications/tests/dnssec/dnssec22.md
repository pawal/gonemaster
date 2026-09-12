# DNSSEC22

Status: Final

## Purpose
- Verify that the address records (`A` and `AAAA`) of every in-domain nameserver name of the zone validate under the chain of trust of the zone.
- An in-domain nameserver name (RFC 9499 section 7) is an `NS` target at or
  below the apex of the zone under test. Its address records are served by the
  zone itself or by a zone delegated below it, so they are inside the
  authentication chain the zone owns.
- A validating resolver that refreshes such an address authoritatively
  receives SERVFAIL when the records are bogus and stops using that
  nameserver. No other testcase reads a signature below the apex: the address
  records of in-domain names are resolved with DO=0 and glue is accepted, so a
  bogus name is reported as resolvable.

## Preconditions And Inputs
- Preconditions:
  - A `zone.Zone` object is available.
  - At least one in-domain nameserver name exists. The root zone has an empty
    name set by rule, see Edge Cases And Limitations.
  - The zone is secure: at least one child nameserver returns an apex DNSKEY
    RRset and at least one parent nameserver returns a DS RRset for the zone
    name.
- Required inputs:
  - Child nameserver name/IP items from
    [`DelegationNameservers`](../../nameserver-resolution.md#delegationnameservers)
    and [`ZoneNameservers`](../../nameserver-resolution.md#zonenameservers),
    deduplicated by IP.
  - The parent-side delegation `NS` names and the child apex `NS` names.
  - Apex DNSKEY RRset of the zone from a child nameserver, DS RRset of the zone
    from a parent nameserver. In a full run both are served from the
    per-nameserver query cache (DNSSEC10 for DNSKEY, DNSSEC07 and DNSSEC21 for
    DS).
  - Per child nameserver IP: `A` responses (and `AAAA` where required) for each
    in-domain name, and `DS` and `DNSKEY` responses for each probed zone cut,
    all with DNSSEC enabled.
- Profile/config knobs that affect behavior:
  - `net.ipv4` and `net.ipv6`: disabled transports are skipped with transport
    debug tags.
  - `resolver.defaults.parallel`: per-nameserver parallel execution fanout.
  - `dnssec.signature_validity_skew`: clock-skew tolerance used by
    `verifyRRSIG`.

## Algorithm And Decision Flow
1. Emit `TEST_CASE_START`.
2. Build the name set: the union of the parent-side delegation `NS` names and
   the child apex `NS` names, keeping every name N for which
   `z.Name.IsInBailiwick(N)` holds, deduplicated case-insensitively. The apex
   itself is kept when it appears as an `NS` target. For the root zone the name
   set is empty by rule.
   - Empty name set: emit `DS22_NO_IN_DOMAIN_NS` and `TEST_CASE_END`, then
     stop. No query is made.
3. Gate on zone security:
   - If no child nameserver returns an apex DNSKEY RRset, or no parent
     nameserver returns a DS RRset for the zone name, emit
     `DS22_ZONE_NOT_SECURE` and `TEST_CASE_END`, then stop. Nothing below an
     unsigned zone or an island of security can be bogus.
4. For each unique child nameserver IP X (parallelized):
   - If the transport is disabled, emit `IPV4_DISABLED` or `IPV6_DISABLED` with
     `query_type` `A`, and skip X.
   - Initialise the per-server state: an empty referral set and an empty zone
     cut memo.
   - Process the names of the name set in order. For each name N:
     1. If N is at or below a name in the referral set, skip N. X has stated it
        does not serve that subtree.
     2. Query `N A` at X with DNSSEC enabled (UDP, retry over TCP on `TC`).
     3. Classify the response and select the covering RRSIG records:
        - No response, or a response whose RCODE is neither `NOERROR` nor
          `NXDOMAIN`: no finding for N.
        - Referral (not `AA`, empty answer section, `NS` RRset in the authority
          section): add the owner name of that `NS` RRset to the referral set;
          no finding for N.
        - `AA` `NXDOMAIN`: no finding for N. Delegation and nameserver
          testcases own a nameserver name that does not exist.
        - `CNAME` in the answer section: no finding for N. RFC 2181 section
          10.3 forbids an alias at a nameserver name and Delegation05 reports
          it.
        - `AA` `NOERROR` with an `A` RRset for N: the covering records are the
          answer-section RRSIG records covering type `A` at N.
        - `AA` `NOERROR` without an `A` RRset (NODATA): query `N AAAA` at X
          with DNSSEC enabled. On an `AA` `NOERROR` answer with an `AAAA`
          RRset, the covering records are the answer-section RRSIG records
          covering type `AAAA` at N. On any other outcome, the covering records
          are the authority-section RRSIG records covering `NSEC` or `NSEC3` in
          the `A` response.
        - The signer S is the Signer's Name of the covering RRSIG records. With
          no covering RRSIG record, S is undefined.
     4. Determine the expected signer E. The walk order is N, then each proper
        ancestor of N strictly below the zone apex, upward, then the apex:
        - If S equals the zone name, E is the zone and no query is made. X
          holds no zone cut at or below N.
        - Otherwise the walk starts at S when S is a member of the walk order,
          and at N otherwise. X answers from its most specific matching zone,
          so a zone cut between N and S would have answered instead and is not
          probed.
        - E is the first name M in the walk, from the start position upward,
          whose `cutStatus(X, M)` is `secure`, `insecure` or `broken`. If no
          such M exists, E is the zone.
     5. Classify N on X, in this order:
        - `cutStatus(X, E)` is `insecure`: `DS22_NS_ADDRESS_INSECURE`.
          Validators accept the data unsigned.
        - `cutStatus(X, E)` is `broken`: `DS22_NS_ADDRESS_CHAIN_BROKEN` with
          `signer` E.
        - S undefined: `DS22_NS_ADDRESS_UNSIGNED`.
        - S equals E: verify each covering RRSIG against the DNSKEY RRset of E,
          which is the apex DNSKEY RRset when E is the zone and the DNSKEY
          RRset retained by `cutStatus` otherwise, with `packetTime` of the
          response as the reference time.
          - A signature whose algorithm the local verifier cannot process
            (`dnssecutil.AlgorithmSupported` false, or
            `RSAExponentBeyondLocalVerifier` true) is indeterminate and yields
            no finding.
          - Expiration in the past: `DS22_NS_ADDRESS_RRSIG_EXPIRED` with
            `keytag`.
          - Inception in the future, no DNSKEY with that keytag, or
            verification failure:
            `DS22_NS_ADDRESS_RRSIG_NOT_VALID_BY_DNSKEY` with `signer` and
            `keytag`.
          - At least one covering RRSIG verifies: N validates on X, no finding.
        - S is a member of the walk order strictly below E and `cutStatus(X, S)`
          is `not a cut`: `DS22_NS_ADDRESS_ORPHAN_ZONE` with `signer` S. X
          serves S as a zone apex while the enclosing zone proves no delegation
          there.
        - S is a member of the walk order strictly below E and `cutStatus(X, S)`
          is `indeterminate`: no finding. X stated nothing about S as a zone
          cut, so neither the orphan nor the signer conclusion is available.
        - Otherwise: `DS22_NS_ADDRESS_RRSIG_NOT_VALID_BY_DNSKEY` with `signer`
          S and the `keytag` of the first covering RRSIG. The signature cannot
          verify under the chain of trust of the zone.
5. Aggregate across nameservers: one emission per tag and per distinct
   combination of its arguments other than `servers`, with the matching
   `servers` merged and sorted.
6. If no ERROR-level `DS22_*` tag was emitted and at least one address RRset
   validated, emit `DS22_NS_ADDRESS_VALIDATES` with the nameservers that
   validated at least one address RRset.
7. Emit `TEST_CASE_END`.

### Zone Cut Status

`cutStatus(X, M)` is the statement of nameserver X about the name M as a zone
cut. It queries `M DS` at X with DNSSEC enabled (UDP, retry over TCP on `TC`)
and returns one of `secure`, `insecure`, `broken`, `not a cut` or
`indeterminate`:

- No response, a response that is not `AA`, or a referral: `indeterminate`. X
  does not serve the parent side of M.
- `AA` `NOERROR` with a DS RRset at M:
  - The signer P of the DS-covering RRSIG MUST be a proper ancestor of M at or
    below the zone apex, otherwise `broken`.
  - The DNSKEY RRset of P is the apex DNSKEY RRset when P is the zone;
    otherwise `cutStatus(X, P)` MUST be `secure` and supplies it, and any other
    status yields `broken`. The recursion is bounded by the labels between M
    and the zone apex.
  - The DS RRSIG MUST verify against a DNSKEY of P, otherwise `broken`.
  - `M DNSKEY` is queried at X. The response MUST be `AA` `NOERROR` with a
    DNSKEY RRset containing a key that matches a DS by keytag, algorithm and
    digest, and the DNSKEY RRset MUST carry an RRSIG that verifies under that
    key, otherwise `broken`.
  - All conditions hold: `secure`, retaining the DNSKEY RRset of M.
  - A signature whose algorithm the local verifier cannot process is
    `indeterminate`, not `broken`.
- `AA` `NOERROR` without a DS RRset (NODATA):
  - Locate the NSEC record whose owner is M, or the NSEC3 record whose owner
    matches the hash of M computed with the salt and iteration count of that
    record.
  - Type bitmap with the NS bit and without the DS bit: `insecure`.
  - Type bitmap with both the NS bit and the DS bit: the bitmap contradicts the
    NODATA, `indeterminate`.
  - Type bitmap without the NS bit: `not a cut`. RFC 5155 section 8.9 and RFC
    4035 section 5.2 make the NS bit the statement of the parent zone that a
    zone cut exists at the name.
  - No NSEC and no NSEC3 record: the enclosing zone is unsigned, which is valid
    only below an insecure delegation. Return `not a cut` and let the walk
    decide from an ancestor.
- `AA` `NXDOMAIN`: `not a cut`.

Every result is memoized per (X, M), `indeterminate` included, and reused by
every name below M on X.

### Per-NS Name Validation And Aggregation (steps 2-7)

{{% expand "Show diagram" %}}
```
names = in-domain(delegation NS names ++ child apex NS names); dedupe
   names empty (root zone included) -> DS22_NO_IN_DOMAIN_NS
                                       emit TEST_CASE_END and stop

gate: apex DNSKEY from any child NS AND DS from any parent NS
   not both                         -> DS22_ZONE_NOT_SECURE
                                       emit TEST_CASE_END and stop

For each unique child NS IP X (parallel; fan-out = resolver.defaults.parallel):

   transport disabled -> IPV4_DISABLED / IPV6_DISABLED (query_type A), skip X
   refersFor = {}; cuts = {}

   For each in-domain name N:
      N at or below a name in refersFor  -> skip N
      query N A at X, DNSSEC=on (TC -> retry over TCP)
       +- no resp / RCODE not NOERROR|NXDOMAIN -> no finding
       +- referral       -> refersFor += authority NS owner; no finding
       +- AA NXDOMAIN    -> no finding
       +- CNAME          -> no finding (Delegation05)
       +- AA answer      -> sigs = RRSIG(A) in answer
       +- AA NODATA      -> query N AAAA at X, DNSSEC=on
                              AA answer -> sigs = RRSIG(AAAA) in answer
                              otherwise -> sigs = RRSIG(NSEC|NSEC3) in the
                                           authority of the A response
      S = Signer's Name of sigs (undefined when sigs is empty)

      walk = N, ancestors of N below the apex, apex
      S == apex                         -> E = apex (no query)
      otherwise, from S in walk (else N) upward:
         first M with cutStatus(X, M) in {secure, insecure, broken} -> E = M
         none                                                      -> E = apex

      cutStatus(E) insecure   -> DS22_NS_ADDRESS_INSECURE (ns, servers)
      cutStatus(E) broken     -> DS22_NS_ADDRESS_CHAIN_BROKEN (ns, signer=E)
      S undefined             -> DS22_NS_ADDRESS_UNSIGNED (ns, servers)
      S == E                  -> verify sigs against DNSKEY(E) at packetTime
            unsupported algorithm   -> no finding
            expired                 -> DS22_NS_ADDRESS_RRSIG_EXPIRED
            not yet valid / no key
            / bad signature         -> DS22_NS_ADDRESS_RRSIG_NOT_VALID_BY_DNSKEY
            one verifies            -> N validates on X
      S in walk below E, cutStatus(X, S) == not a cut
                              -> DS22_NS_ADDRESS_ORPHAN_ZONE (ns, signer=S)
      S in walk below E, cutStatus(X, S) == indeterminate -> no finding
      otherwise               -> DS22_NS_ADDRESS_RRSIG_NOT_VALID_BY_DNSKEY

Aggregate:
   per tag and per distinct non-servers argument set -> merge and sort servers
   no ERROR-level DS22 tag AND at least one RRset validated
      -> DS22_NS_ADDRESS_VALIDATES (servers)

emit TEST_CASE_END
```
{{% /expand %}}

## Emitted Tags (Possible Set)
| Tag | Emitted when |
| --- | --- |
| `DS22_NO_IN_DOMAIN_NS` | No nameserver name of the zone is in-domain, the root zone included. |
| `DS22_NS_ADDRESS_CHAIN_BROKEN` | The expected signer is a zone cut below the zone apex on the same nameserver, and its DS matches no DNSKEY, or its DNSKEY or DS RRSIG does not verify. |
| `DS22_NS_ADDRESS_INSECURE` | The name lies below an insecure delegation served by the same nameserver. |
| `DS22_NS_ADDRESS_ORPHAN_ZONE` | The address records are signed by a name the nameserver serves as a zone apex, while the NSEC or NSEC3 record matching that name in the enclosing zone carries no NS bit, so no zone cut is proven. |
| `DS22_NS_ADDRESS_RRSIG_EXPIRED` | The expiration of the RRSIG covering the address records is in the past. |
| `DS22_NS_ADDRESS_RRSIG_NOT_VALID_BY_DNSKEY` | The RRSIG covering the address records does not verify against the DNSKEY RRset of the zone that must sign the name: bad signature, inception in the future, no DNSKEY with the keytag, or a signer that is neither the expected zone nor an orphan apex. |
| `DS22_NS_ADDRESS_UNSIGNED` | The address records, or the NODATA proof standing for them, carry no RRSIG and no insecure delegation lies between the name and the zone apex. |
| `DS22_NS_ADDRESS_VALIDATES` | At least one in-domain address RRset validated and no ERROR-level `DS22_*` tag was emitted. |
| `DS22_ZONE_NOT_SECURE` | No child nameserver returned an apex DNSKEY RRset, or no parent nameserver returned a DS RRset for the zone. |
| `IPV4_DISABLED` | IPv4 transport is disabled for a queried nameserver (`A`). |
| `IPV6_DISABLED` | IPv6 transport is disabled for a queried nameserver (`A`). |
| `TEST_CASE_END` | Testcase completion marker is emitted. |
| `TEST_CASE_START` | Testcase start marker is emitted. |

## Tag Arguments
| Tag | Argument key | Type | Meaning |
| --- | --- | --- | --- |
| `DS22_NS_ADDRESS_CHAIN_BROKEN` | `ns` | `string` | In-domain nameserver name whose address records were evaluated. |
| `DS22_NS_ADDRESS_CHAIN_BROKEN` | `signer` | `string` | Zone cut below the apex whose chain of trust is broken. |
| `DS22_NS_ADDRESS_CHAIN_BROKEN` | `servers` | `array<object>` | Structured nameserver identities (`{ns,address}` object) that showed the broken chain. |
| `DS22_NS_ADDRESS_INSECURE` | `ns` | `string` | In-domain nameserver name whose address records were evaluated. |
| `DS22_NS_ADDRESS_INSECURE` | `servers` | `array<object>` | Structured nameserver identities (`{ns,address}` object) serving the name below an insecure delegation. |
| `DS22_NS_ADDRESS_ORPHAN_ZONE` | `ns` | `string` | In-domain nameserver name whose address records were evaluated. |
| `DS22_NS_ADDRESS_ORPHAN_ZONE` | `signer` | `string` | Signer's Name of the RRSIG covering the address records, served as a zone apex without a proven delegation. |
| `DS22_NS_ADDRESS_ORPHAN_ZONE` | `servers` | `array<object>` | Structured nameserver identities (`{ns,address}` object) exhibiting the orphan zone. |
| `DS22_NS_ADDRESS_RRSIG_EXPIRED` | `ns` | `string` | In-domain nameserver name whose address records were evaluated. |
| `DS22_NS_ADDRESS_RRSIG_EXPIRED` | `keytag` | `int` | Keytag of the expired RRSIG. |
| `DS22_NS_ADDRESS_RRSIG_EXPIRED` | `servers` | `array<object>` | Structured nameserver identities (`{ns,address}` object) that returned the expired RRSIG. |
| `DS22_NS_ADDRESS_RRSIG_NOT_VALID_BY_DNSKEY` | `ns` | `string` | In-domain nameserver name whose address records were evaluated. |
| `DS22_NS_ADDRESS_RRSIG_NOT_VALID_BY_DNSKEY` | `signer` | `string` | Signer's Name of the failing RRSIG. |
| `DS22_NS_ADDRESS_RRSIG_NOT_VALID_BY_DNSKEY` | `keytag` | `int` | Keytag of the failing RRSIG. |
| `DS22_NS_ADDRESS_RRSIG_NOT_VALID_BY_DNSKEY` | `servers` | `array<object>` | Structured nameserver identities (`{ns,address}` object) that returned the failing RRSIG. |
| `DS22_NS_ADDRESS_UNSIGNED` | `ns` | `string` | In-domain nameserver name whose address records were evaluated. |
| `DS22_NS_ADDRESS_UNSIGNED` | `servers` | `array<object>` | Structured nameserver identities (`{ns,address}` object) that returned the unsigned records. |
| `DS22_NS_ADDRESS_VALIDATES` | `servers` | `array<object>` | Structured nameserver identities (`{ns,address}` object) that validated at least one in-domain address RRset. |
| `IPV4_DISABLED` | `ns` | `string` | Nameserver identity (`ns` name only; use `address` for IP) skipped on IPv4. |
| `IPV4_DISABLED` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `IPV4_DISABLED` | `query_type` | `string` | rrtype skipped (`A`). |
| `IPV6_DISABLED` | `ns` | `string` | Nameserver identity (`ns` name only; use `address` for IP) skipped on IPv6. |
| `IPV6_DISABLED` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `IPV6_DISABLED` | `query_type` | `string` | rrtype skipped (`A`). |
| `TEST_CASE_END` | `testcase` | `string` | Testcase display name (`DNSSEC22`). |
| `TEST_CASE_START` | `testcase` | `string` | Testcase display name (`DNSSEC22`). |

`DS22_NO_IN_DOMAIN_NS` and `DS22_ZONE_NOT_SECURE` carry no arguments.

## Severity Levels Per Tag
The five failure tags describe a name that validating resolvers answer with
SERVFAIL, the outcome `CAN_NOT_BE_RESOLVED` (ERROR) describes for a name with
no address at all. The level follows the fault, not the residual reachability
that glue gives the zone under test.

| Tag | Level | Notes |
| --- | --- | --- |
| `DS22_NO_IN_DOMAIN_NS` | `INFO` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS22_NS_ADDRESS_CHAIN_BROKEN` | `ERROR` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS22_NS_ADDRESS_INSECURE` | `INFO` | Data below an insecure delegation is accepted by validators. |
| `DS22_NS_ADDRESS_ORPHAN_ZONE` | `ERROR` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS22_NS_ADDRESS_RRSIG_EXPIRED` | `ERROR` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS22_NS_ADDRESS_RRSIG_NOT_VALID_BY_DNSKEY` | `ERROR` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS22_NS_ADDRESS_UNSIGNED` | `ERROR` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS22_NS_ADDRESS_VALIDATES` | `INFO` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS22_ZONE_NOT_SECURE` | `INFO` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `IPV4_DISABLED` | `DEBUG` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `IPV6_DISABLED` | `DEBUG` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `TEST_CASE_END` | `DEBUG` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `TEST_CASE_START` | `DEBUG` | Default from `share/profile.json` (`test_levels.DNSSEC`). |

Scoring takes the severity default in the `dnssec` dimension. No
`TagPenalties` override is defined.

## Differences From Upstream
- Upstream reference: no upstream Zonemaster equivalent. DNSSEC22 is a new
  testcase unique to gonemaster, and no upstream testcase validates a signature
  below the apex of the zone under test.
- Potential upstream report:
  - `yes`
  - Upstream expected behavior: an in-domain nameserver name whose address
    records are bogus under the chain of trust of the zone under test is
    reported.
  - Gonemaster observed behavior: DNSSEC22 reports it; the address records of
    in-domain nameserver names are otherwise resolved with DO=0 and glue is
    accepted, in gonemaster as upstream.
  - evidence: `engine/test/dnssec/dnssec22_test.go`.
  - report status: `not filed`.

## Edge Cases And Limitations
- The root zone has an empty name set and emits `DS22_NO_IN_DOMAIN_NS` without
  a query. Every name is subordinate to the root, so the in-domain rule alone
  admits all thirteen root server names. They live in `root-servers.net`, an
  unsigned zone under `net`: a root server serves that zone authoritatively but
  answers `root-servers.net DS` with a referral, so every zone cut walk from a
  root server ends `indeterminate` and no finding is possible.
- A nameserver that answers a referral for a name serves neither that name nor
  its subtree, and the whole subtree is skipped on that nameserver without a
  finding. The delegated zone is a test target of its own. This is the shape of
  `de`, whose nameserver names live in the delegated zone `nic.de`: DNSSEC22
  sees only referrals there and validates none of the three names.
- An undelegated zone and a correctly delegated zone whose signer omitted the
  NS bit from the matching NSEC or NSEC3 record are indistinguishable from the
  parent side when one nameserver holds both zones: an `NS` query for the name
  is answered from the child, never with a parent-side delegation, so the type
  bitmap is the only statement available about what the parent holds. Both land
  in `DS22_NS_ADDRESS_ORPHAN_ZONE`, whose message names the missing NS bit as
  the evidence. Validators reject the name in both cases.
- `A` classifies a name and `AAAA` is queried only when there is no `A`, so a
  signature fault confined to the `AAAA` RRset of a name whose `A` RRset is
  healthy is not detected. Signers re-sign a zone as a unit, so the failure
  classes this testcase covers all show on `A`.
- `AAAA` is never queried for a name already at fault: no `DS22_*` tag carries
  a per-type argument, so a second observation could reach no emission.
- Several nameserver names sharing one IP are evaluated once per name; the
  `servers` argument carries the `{ns,address}` endpoints.
- A NODATA response is evaluated for the signer and the signature validity of
  the NSEC or NSEC3 records it carries, not for the logical completeness of the
  denial. DNSSEC10 owns denial completeness.
- Wildcard-synthesised address records need no special handling:
  `dnssecutil.VerifyRRSIG` derives the wildcard owner from the RRSIG label
  count.
- A `CNAME` at a nameserver name is skipped. RFC 2181 section 10.3 forbids an
  alias there and Delegation05 reports it; following the alias would validate a
  name in another zone.
- Undelegated runs (`hasFakeAddresses` is true) use the fake addresses. The
  zone cut walk still works, since DS and DNSKEY come from the same fake
  nameservers.
- A signature whose algorithm the local verifier cannot process is
  indeterminate and yields no finding, at the leaf and in the zone cut walk
  alike.
- A zone cut whose `DS` question draws no answer on a nameserver stays
  `indeterminate` for every name below it on that nameserver and yields no
  finding there. Other nameservers are unaffected.
- Running `--testcase dnssec22` alone makes the DNSKEY and DS queries of the
  gate itself; nothing is assumed cached.

## Implementation Notes
Implementation-defined choices, none of them mandated by the protocol:

- The NSEC and NSEC3 records read for zone cut status are not verified
  cryptographically. They come from a nameserver that also serves the child
  data, and the finding is a structural contradiction between two answers of
  that same nameserver. DNSSEC10 owns NSEC and NSEC3 signature checks.
- Child nameservers are deduplicated by IP, and one task runs per unique IP.
- `A` is queried first and settles the name; `AAAA` follows only an
  authoritative NODATA on `A`.
- The referral set and the zone cut memo are per nameserver. The names of one
  nameserver are processed in sequence, so no two names share a partial memo
  entry. `indeterminate` is memoized like every other status.
- The zone cut walk is bounded by the labels between the name and the zone
  apex; no ancestor of the zone under test is derived.
- `DS22_NS_ADDRESS_VALIDATES` is OK-tag gated: it is emitted only when no
  ERROR-level `DS22_*` tag was emitted.
- The reference time for signature validity is `packetTime(resp)`, the
  timestamp of the response carrying the signature, as in the other verifying
  DNSSEC testcases.
- The name set is the delegation. The SOA MNAME is not evaluated, although an
  in-domain MNAME carries the same failure.

## Evidence In Gonemaster
- Code paths:
  - `engine/test/dnssec/dnssec.go` (DNSSEC22 testcase function and zone cut
    walker).
- Related tests:
  - `engine/test/dnssec/dnssec22_test.go`.
- References:
  - RFC 1034, RFC 1035 (zone cuts, delegation).
  - RFC 2181 section 10.3 (no alias at a nameserver name).
  - RFC 4033, RFC 4034, RFC 4035 (DNSSEC, NSEC type bitmap, authenticating a
    referral and a negative response).
  - RFC 5155 (NSEC3, opt-out, proving the absence of a DS).
  - RFC 9471 (glue for in-domain nameserver names).
  - RFC 9499 section 7 (in-domain, sibling domain, zone cut) and section 10
    (signed zone, insecure delegation, island of security, and the secure,
    insecure, bogus and indeterminate validation states).
