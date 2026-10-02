# DNSSEC23

Status: Final

## Purpose
- Verify that the NSEC or NSEC3 records a nameserver returns in one denial of
  existence response are consistent with each other, and that the NSEC3 chain
  serving the proof is published in the NSEC3PARAM RRset at the apex.
- A chain in which the interval of one record covers the owner name of
  another record asserts that a name both does not exist and exists. Every
  signature verifies and every record is well formed, so the zone passes the
  signature checks of DNSSEC10 while a validator that evaluates the proof
  rejects the response.
- DNSSEC10 checks the apex NODATA proof returned for `NSEC` and `NSEC3PARAM`
  queries, which carries exactly one NSEC3 record. DNSSEC23 checks the denial
  of a name that does not exist, which is the only response in a run that
  carries more than one denial record.

## Preconditions And Inputs
- Preconditions:
  - A `zone.Zone` object is available.
  - The nameserver under evaluation returns an apex DNSKEY RRset. A
    nameserver without one is skipped; DNSSEC07 and DNSSEC10 report it.
- Required inputs:
  - Child nameserver name/IP items from
    [`DelegationNameservers`](../../nameserver-resolution.md#delegationnameservers)
    and [`ZoneNameservers`](../../nameserver-resolution.md#zonenameservers),
    deduplicated by IP.
  - Per nameserver IP: the apex `DNSKEY` response, the `A` response for the
    probe name, and, for an NSEC3 proof, the apex `NSEC3PARAM` response, all
    with DNSSEC enabled. In a full run the `DNSKEY` and `NSEC3PARAM` responses
    are served from the per-nameserver query cache filled by DNSSEC10.
- Profile/config knobs that affect behavior:
  - `net.ipv4` and `net.ipv6`: disabled transports are skipped with transport
    debug tags.
  - `resolver.defaults.parallel`: per-nameserver parallel execution fanout.

## Algorithm And Decision Flow
1. Emit `TEST_CASE_START`.
2. The probe name P is the label `xn--gonemaster-dnssec23` prepended to the
   zone name. The label is not a valid A-label: Punycode decoding of it fails,
   so no registry accepts it and no signer has hashed it into a chain. The
   label is constant so that a saved cache replays.
3. For each unique child nameserver IP X (parallelized):
   1. If the transport is disabled, emit `IPV4_DISABLED` or `IPV6_DISABLED`
      with `query_type` `A`, and skip X.
   2. Query the apex `DNSKEY` at X with DNSSEC enabled. On no response, a
      response that is not `AA` `NOERROR`, or a response without a DNSKEY
      RRset at the apex, skip X. No finding is made.
   3. Query `P A` at X with DNSSEC enabled (UDP, retry over TCP on `TC`).
      Skip X with no finding on no response, a response that is not `AA`, a
      response with the `TC` flag set, or an RCODE other than `NOERROR` and
      `NXDOMAIN`. An `AA` `NOERROR` response is a wildcard expansion or a
      NODATA at an existing name; it carries a proof and is evaluated as an
      `NXDOMAIN` response is.
   4. Collect the proof from the authority section: the NSEC3 records whose
      owner name is exactly one label below the zone name, and the NSEC
      records whose owner name is at or below the zone name. Other records
      are ignored.
   5. Neither set has a member: `DS23_NO_DENIAL_PROOF` on X. Steps 3.6 and
      3.7 are not reached.
   6. NSEC3 records are present:
      1. R3. Collect the distinct (hash algorithm, iterations, salt) triples
         of the records. More than one triple: `DS23_NSEC3_MIXED_PARAMETERS`
         on X. Steps 3.6.2, 3.6.3 and 3.6.5 are skipped, since records from
         different chains have no common order.
      2. R2. For each unordered pair of records with distinct owner labels and
         an equal Next Hashed Owner Name, compared case-insensitively:
         `DS23_NSEC3_DUPLICATE_NEXT` on X, with `owner` the owner name of the
         record earlier in hash order, `covered` the owner name of the other
         record, and `next` the shared value.
      3. R1. For each ordered pair of records (A, B), A and B distinct and the
         pair not reported in step 3.6.2: if the owner label of B lies strictly
         inside the interval of A, `DS23_NSEC3_RANGES_OVERLAP` on X, with
         `owner` the owner name of A, `next` the Next Hashed Owner Name of A
         and `covered` the owner name of B.
      4. Query the apex `NSEC3PARAM` at X with DNSSEC enabled. On no response
         or a response that is not `AA` `NOERROR`, steps 3.6.5 and 3.6.6 are
         skipped. Otherwise take the NSEC3PARAM RRset at the apex from the
         answer section.
      5. R4. No NSEC3PARAM record carries the triple of the proof:
         `DS23_NSEC3_CHAIN_NOT_PUBLISHED` on X. An empty NSEC3PARAM RRset
         satisfies this condition.
      6. R5. More than one NSEC3PARAM record: `DS23_MULTIPLE_NSEC3PARAM` on X.
   7. NSEC records are present: R1 as in step 3.6.3 over the NSEC records,
      with the Next Domain Name as the end of the interval and canonical name
      order (RFC 4034 section 6.1) as the order:
      `DS23_NSEC_RANGES_OVERLAP` on X. NSEC and NSEC3 records in the same
      response are each evaluated with their own kind; DNSSEC10 reports the
      mixture.
   8. A proof that raised none of `DS23_NO_DENIAL_PROOF`,
      `DS23_NSEC3_MIXED_PARAMETERS`, `DS23_NSEC3_DUPLICATE_NEXT`,
      `DS23_NSEC3_RANGES_OVERLAP`, `DS23_NSEC3_CHAIN_NOT_PUBLISHED` and
      `DS23_NSEC_RANGES_OVERLAP` is consistent on X.
4. Aggregate across nameservers: one emission per tag and per distinct
   combination of its arguments other than `servers`, with the matching
   `servers` merged and sorted. Emissions are ordered by tag, then `owner`,
   then `covered`, then `next`.
5. If no ERROR-level `DS23_*` tag was emitted and at least one proof was
   consistent, emit `DS23_DENIAL_PROOF_CONSISTENT` with the nameservers whose
   proof was consistent.
6. Emit `TEST_CASE_END`.

### Intervals And Order

The interval of a record with owner O and next field N is the set of names
strictly between O and N in the order of the record's kind. When N is not
greater than O, the record is the wrap point of the chain (RFC 5155 section
7.1 step 7, RFC 4034 section 4.1.1): its interval is every name greater than
O together with every name less than N. A name equal to O or to N is not
inside the interval. When N equals O the chain has one record and the
interval is every other name.

- NSEC3 records are ordered by their hashed owner label, the first label of
  the owner name, compared bytewise after lowercasing. Base32hex preserves the
  order of the hash values, so this is hash order (RFC 5155 section 7.1 step
  5). The Next Hashed Owner Name is a label in the same encoding.
- NSEC records are ordered by canonical name order, which `dns.CompareName`
  implements.

### Per-NS Proof Evaluation And Aggregation (steps 3-6)

{{% expand "Show diagram" %}}
```
P = xn--gonemaster-dnssec23 . zone

For each unique child NS IP X (parallel; fan-out = resolver.defaults.parallel):

   transport disabled -> IPV4_DISABLED / IPV6_DISABLED (query_type A), skip X
   apex DNSKEY at X, DNSSEC=on
    +- no resp / not AA NOERROR / no DNSKEY -> skip X
   P A at X, DNSSEC=on (TC -> retry over TCP)
    +- no resp / not AA / TC / RCODE not NOERROR|NXDOMAIN -> skip X
   nsec3 = authority NSEC3 owned one label below zone
   nsec  = authority NSEC owned at or below zone
    +- both empty                          -> DS23_NO_DENIAL_PROOF
    +- nsec3 present
         triples > 1                       -> DS23_NSEC3_MIXED_PARAMETERS
                                              (R1, R2, R4 skipped)
         equal next, distinct owners       -> DS23_NSEC3_DUPLICATE_NEXT
                                              (owner, covered, next)
         owner inside another interval,
           pair not a duplicate            -> DS23_NSEC3_RANGES_OVERLAP
                                              (owner, next, covered)
         apex NSEC3PARAM at X, DNSSEC=on
          +- no resp / not AA NOERROR      -> R4, R5 skipped
          +- no record with the triple     -> DS23_NSEC3_CHAIN_NOT_PUBLISHED
          +- more than one record          -> DS23_MULTIPLE_NSEC3PARAM
    +- nsec present
         owner inside another interval     -> DS23_NSEC_RANGES_OVERLAP
                                              (owner, next, covered)
   no error tag on X                       -> consistent on X

aggregate: one emission per tag and argument set, servers merged and sorted
no ERROR DS23 tag and a consistent proof  -> DS23_DENIAL_PROOF_CONSISTENT
```
{{% /expand %}}

## Emitted Tags (Possible Set)
| Tag | Emitted when |
| --- | --- |
| `DS23_DENIAL_PROOF_CONSISTENT` | No ERROR-level `DS23_*` tag was emitted and at least one nameserver served a proof that passed every rule. |
| `DS23_MULTIPLE_NSEC3PARAM` | The apex NSEC3PARAM RRset on a nameserver has more than one record. |
| `DS23_NO_DENIAL_PROOF` | A nameserver that serves an apex DNSKEY RRset answered the probe authoritatively with neither NSEC nor NSEC3 records. |
| `DS23_NSEC3_CHAIN_NOT_PUBLISHED` | No NSEC3PARAM record at the apex carries the hash algorithm, iterations and salt of the NSEC3 records in the proof. |
| `DS23_NSEC3_DUPLICATE_NEXT` | Two NSEC3 records in one response have distinct owner names and the same Next Hashed Owner Name. |
| `DS23_NSEC3_MIXED_PARAMETERS` | The NSEC3 records in one response do not share hash algorithm, iterations and salt. |
| `DS23_NSEC3_RANGES_OVERLAP` | The owner name of one NSEC3 record lies strictly inside the interval of another, and the pair is not reported as a duplicate next field. |
| `DS23_NSEC_RANGES_OVERLAP` | The owner name of one NSEC record lies strictly inside the interval of another. |
| `IPV4_DISABLED` | IPv4 transport is disabled for a queried nameserver (`A`). |
| `IPV6_DISABLED` | IPv6 transport is disabled for a queried nameserver (`A`). |
| `TEST_CASE_END` | Testcase end marker is emitted. |
| `TEST_CASE_START` | Testcase start marker is emitted. |

## Tag Arguments
| Tag | Argument key | Type | Meaning |
| --- | --- | --- | --- |
| `DS23_DENIAL_PROOF_CONSISTENT` | `servers` | `array<object>` | Nameservers whose proof passed every rule (`{ns,address}` objects). |
| `DS23_MULTIPLE_NSEC3PARAM` | `servers` | `array<object>` | Nameservers whose apex NSEC3PARAM RRset has more than one record. |
| `DS23_NO_DENIAL_PROOF` | `servers` | `array<object>` | Nameservers that answered the probe without NSEC or NSEC3 records. |
| `DS23_NSEC3_CHAIN_NOT_PUBLISHED` | `servers` | `array<object>` | Nameservers whose proof comes from a chain absent from the apex NSEC3PARAM RRset. |
| `DS23_NSEC3_DUPLICATE_NEXT` | `owner` | `string` | Owner name of the record earlier in hash order, lowercased. |
| `DS23_NSEC3_DUPLICATE_NEXT` | `covered` | `string` | Owner name of the other record, lowercased. |
| `DS23_NSEC3_DUPLICATE_NEXT` | `next` | `string` | The shared Next Hashed Owner Name, base32hex as the record presents it. |
| `DS23_NSEC3_DUPLICATE_NEXT` | `servers` | `array<object>` | Nameservers serving the pair. |
| `DS23_NSEC3_MIXED_PARAMETERS` | `servers` | `array<object>` | Nameservers whose proof mixes parameters. |
| `DS23_NSEC3_RANGES_OVERLAP` | `owner` | `string` | Owner name of the covering record, lowercased. |
| `DS23_NSEC3_RANGES_OVERLAP` | `next` | `string` | Next Hashed Owner Name of the covering record, base32hex as the record presents it. |
| `DS23_NSEC3_RANGES_OVERLAP` | `covered` | `string` | Owner name of the record inside the interval, lowercased. |
| `DS23_NSEC3_RANGES_OVERLAP` | `servers` | `array<object>` | Nameservers serving the pair. |
| `DS23_NSEC_RANGES_OVERLAP` | `owner` | `string` | Owner name of the covering record, lowercased. |
| `DS23_NSEC_RANGES_OVERLAP` | `next` | `string` | Next Domain Name of the covering record, lowercased. |
| `DS23_NSEC_RANGES_OVERLAP` | `covered` | `string` | Owner name of the record inside the interval, lowercased. |
| `DS23_NSEC_RANGES_OVERLAP` | `servers` | `array<object>` | Nameservers serving the pair. |
| `IPV4_DISABLED` | `ns` | `string` | Nameserver identity (`ns` name only; use `address` for IP) skipped on IPv4. |
| `IPV4_DISABLED` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `IPV4_DISABLED` | `query_type` | `string` | rrtype skipped (`A`). |
| `IPV6_DISABLED` | `ns` | `string` | Nameserver identity (`ns` name only; use `address` for IP) skipped on IPv6. |
| `IPV6_DISABLED` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `IPV6_DISABLED` | `query_type` | `string` | rrtype skipped (`A`). |
| `TEST_CASE_END` | `testcase` | `string` | Testcase display name (`DNSSEC23`). |
| `TEST_CASE_START` | `testcase` | `string` | Testcase display name (`DNSSEC23`). |

## Severity Levels Per Tag
| Tag | Level | Notes |
| --- | --- | --- |
| `DS23_DENIAL_PROOF_CONSISTENT` | `INFO` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS23_MULTIPLE_NSEC3PARAM` | `NOTICE` | Default from `share/profile.json` (`test_levels.DNSSEC`). RFC 5155 section 7.3 permits more than one chain; this is the state during a parameter rollover. |
| `DS23_NO_DENIAL_PROOF` | `ERROR` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS23_NSEC3_CHAIN_NOT_PUBLISHED` | `ERROR` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS23_NSEC3_DUPLICATE_NEXT` | `ERROR` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS23_NSEC3_MIXED_PARAMETERS` | `ERROR` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS23_NSEC3_RANGES_OVERLAP` | `ERROR` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS23_NSEC_RANGES_OVERLAP` | `ERROR` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `IPV4_DISABLED` | `DEBUG` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `IPV6_DISABLED` | `DEBUG` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `TEST_CASE_END` | `DEBUG` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `TEST_CASE_START` | `DEBUG` | Default from `share/profile.json` (`test_levels.DNSSEC`). |

## Differences From Upstream
- Upstream reference: no upstream Zonemaster equivalent. DNSSEC23 is a new
  testcase unique to gonemaster. Upstream DNSSEC10 queries the apex only, so
  no upstream testcase holds a response with more than one denial record.
- Potential upstream report:
  - `yes`
  - Upstream expected behavior: a negative response whose NSEC3 records
    cover each other's owner names is reported.
  - Gonemaster observed behavior: DNSSEC23 reports it; the apex NODATA proof
    DNSSEC10 reads carries one record and cannot show the fault, in gonemaster
    as upstream.
  - evidence: `engine/test/dnssec/dnssec23_test.go`.
  - report status: `not filed`.

## Edge Cases And Limitations
- Opt-out needs no special case. An opt-out interval may cover names that
  exist, but those names have no NSEC3 record (RFC 5155 section 7.1), so no
  owner name in the response lies inside it.
- Minimally covering NSEC records (RFC 4470) and compact denial of existence
  (RFC 9824) synthesize one or two records around the queried name and the
  wildcard, whose intervals are disjoint. The rules find nothing and the proof
  is consistent. DNSSEC10 reports the shape as
  `DS10_NONSTANDARD_NSEC_RESPONSE`; DNSSEC23 does not report it again.
- A wildcard at the apex turns the probe response into a wildcard expansion.
  The authority section still carries the record proving that no closer name
  exists (RFC 4035 section 3.1.3.3, RFC 5155 section 7.2.6) and is evaluated.
- A NODATA response at an existing name, which arises only when the zone
  holds the probe name, carries one record per kind and is consistent.
- A referral for the probe name, a non-authoritative answer, or an error RCODE
  yields no finding. Other testcases report a lame or broken nameserver.
- `DS23_NO_DENIAL_PROOF` and the DNSSEC10 tags for a missing apex proof can
  both fire on one nameserver that strips denial records. They read two
  different responses.
- A chain with one record has a next field equal to its owner. There is no
  other owner to compare and the proof is consistent.
- A chain using a hash algorithm the engine does not implement is still
  checked: no rule computes a hash.
- `DS23_NSEC3_CHAIN_NOT_PUBLISHED` fires during a transition to NSEC3 when a
  nameserver serves the new chain before the NSEC3PARAM RRset is added, which
  RFC 5155 section 10.4 step 3 forbids, and during a transition away from a
  chain when the NSEC3PARAM record is removed before the server stops serving
  the chain.
- Records are not verified cryptographically and signatures are not read.
  DNSSEC10 owns NSEC and NSEC3 signature checks.
- Several nameserver names sharing one IP are evaluated once; the `servers`
  argument carries the `{ns,address}` endpoints.
- Running `--testcase dnssec23` alone makes the `DNSKEY` and `NSEC3PARAM`
  queries itself; nothing is assumed cached.
- Undelegated runs (`hasFakeAddresses` is true) use the fake addresses.

## Implementation Notes
Implementation-defined choices, none of them mandated by the protocol:

- The probe label `xn--gonemaster-dnssec23` and the query type `A`. Any
  non-existent name with any type draws the same proof.
- Child nameservers are deduplicated by IP, and one task runs per unique IP.
- A duplicate next field suppresses the overlap finding for the same pair, so
  one fault yields one finding.
- Mixed parameters suppress the pairwise rules and the published chain rule
  for the response.
- The OK tag is gated on the absence of every ERROR-level `DS23_*` tag across
  all nameservers, not per nameserver.

## Evidence In Gonemaster
- Code paths:
  - `engine/test/dnssec/dnssec.go` (DNSSEC23 testcase function and interval
    helpers).
- Related tests:
  - `engine/test/dnssec/dnssec23_test.go`.
- References:
  - RFC 4034 section 4.1.1 (NSEC Next Domain Name and the last record of the
    zone) and section 6.1 (canonical name order).
  - RFC 4035 section 3.1.3 (denial of existence in authoritative responses).
  - RFC 4470 (minimally covering NSEC records).
  - RFC 5155 section 7.1 (NSEC3 chain construction), 7.2 (one chain per
    response), 7.3 (NSEC3PARAM as the chain indication), 7.5 (dynamic update)
    and 10.4 (transition order).
  - RFC 9824 (compact denial of existence).
