# DNSSEC24

Status: Final

## Purpose
- Verify the authenticated DNSSEC bootstrapping signals of RFC 9615 for a
  signed zone that has no DS at its parent.
- A child DNS operator copublishes the apex CDS and CDNSKEY RRsets of the zone
  at a signaling name under each nameserver hostname, in a signaling zone with
  a valid chain of trust from the root (RFC 9615 sections 3.1, 3.2, 4.1). A
  parental agent that validates these copies publishes the DS without the
  acceptance delay of RFC 8078 section 3.3. The testcase executes the four
  validation steps of RFC 9615 section 4.2 and reports every condition on
  which a parental agent aborts.
- DNSSEC15 to DNSSEC18 evaluate the apex records. This testcase evaluates the
  signaling records and their agreement with the apex.

## Preconditions And Inputs
- Preconditions:
  - A `zone.Zone` object is available.
- Required inputs:
  - The DS RRset of the zone from each nameserver of
    [`ParentNameservers`](../../nameserver-resolution.md#parentnameservers),
    or the fake DS data of an undelegated run. In a full run the responses are
    served from the per-nameserver query cache (DNSSEC07).
  - The apex CDS and CDNSKEY RRsets from each nameserver of
    [`GlueNameservers`](../../nameserver-resolution.md#gluenameservers),
    deduplicated by IP. In a full run the responses are served from the
    per-nameserver query cache (DNSSEC15).
  - The parent-side delegation `NS` names from
    [`GlueNames`](../../nameserver-resolution.md#gluenames).
  - The root server addresses from the root hints, and the root trust anchors:
    the IANA root DS records for key tags 20326 and 38696 (algorithm 8, digest
    type 2), embedded in the engine.
  - Per signaling name: `CDS`, `CDNSKEY`, `DS` and `DNSKEY` responses from the
    servers of each zone cut between the root and the signaling zone, all with
    DNSSEC enabled.
- Profile/config knobs that affect behavior:
  - `net.ipv4` and `net.ipv6`: disabled transports are skipped. Parent and
    apex nameservers are reported with transport debug tags; servers on a
    signaling path are left out without a tag.
  - `dnssec.signature_validity_skew`: clock-skew tolerance used by
    `verifyRRSIG`.

## Algorithm And Decision Flow
1. Emit `TEST_CASE_START`.
2. Parent DS view:
   - If a nameserver of the parent zone holds fake DS data for the zone name,
     emit `DS24_DELEGATION_SECURE` with those nameservers and `TEST_CASE_END`,
     then stop.
   - Otherwise, for each parent nameserver P: if the transport is disabled,
     emit `IPV4_DISABLED` or `IPV6_DISABLED` with `query_type` `DS` and skip P.
     Query `<zone> DS` at P with DNSSEC enabled (UDP, retry over TCP on `TC`).
     P holds a DS when the response is `AA` `NOERROR` and its answer section
     carries a DS record owned by the zone name.
   - At least one P holds a DS: emit `DS24_DELEGATION_SECURE` with those
     nameservers and `TEST_CASE_END`, then stop. A zone whose parent servers
     disagree counts as securely delegated; DNSSEC07 reports the disagreement.
3. Apex view: for each delegation nameserver X, deduplicated by IP:
   - If the transport is disabled, emit `IPV4_DISABLED` or `IPV6_DISABLED` with
     `query_type` `CDS` and `CDNSKEY`, and skip X.
   - Query `<zone> CDS` and `<zone> CDNSKEY` at X with DNSSEC enabled (UDP,
     retry over TCP on `TC`). X answers a type when the response is `AA`
     `NOERROR`; its RRset of that type is the set of answer-section records of
     that type owned by the zone name, possibly empty.
   - No X returned a non-empty CDS or CDNSKEY RRset: emit `DS24_NO_CDS_CDNSKEY`
     and `TEST_CASE_END`, then stop.
   - Every record of every returned RRset is an RFC 8078 section 4 delete
     record, recognised by algorithm 0 (CDS `0 0 0 00`, CDNSKEY `0 3 0 AA==`):
     emit
     `DS24_DELETE_REQUESTED` and `TEST_CASE_END`, then stop. A delete request
     with no DS to delete is not a bootstrapping request.
   - Some X answered neither type, or only one of them: emit
     `DS24_APEX_UNAVAILABLE` with those nameservers. The testcase continues.
4. Signaling domains: take the delegation `NS` names, lower-cased and
   deduplicated, and drop every name H for which `z.Name.IsInBailiwick(H)`
   holds (RFC 9615 section 4.1).
   - No name remains: emit `DS24_ONLY_IN_DOMAIN_NS` with every delegation
     `NS` name and `TEST_CASE_END`, then stop (RFC 9615 section 4.4).
5. For each remaining name H, in lexicographic order:
   1. The signaling name N is `_dsboot`, the labels of the zone name,
      `_signal`, then the labels of H. If the wire form of N exceeds 255
      octets (RFC 1035 section 3.1), emit `DS24_SIGNAL_NAME_TOO_LONG` and
      continue with the next name. No query is made.
   2. Locate N by the descent below. On `at cut`, emit
      `DS24_SIGNAL_AT_ZONE_CUT`. On `unreachable` at cut C, emit
      `DS24_SIGNAL_ZONE_UNREACHABLE` with `zone` C and the servers tried. On
      `absent`, record H as absent. In these cases no `DNSKEY` is queried.
   3. On `signal`, validate the chain of trust of the signaling zone Z by the
      verification below. The first fault from the root down ends the
      evaluation of H:
      - a cut without DS: `DS24_SIGNAL_ZONE_INSECURE` with `zone` that cut;
      - a DS, DNSKEY or RRSIG that fails to validate:
        `DS24_SIGNAL_CHAIN_BROKEN` with `zone` that cut;
      - no server of a cut answers its `DNSKEY` question:
        `DS24_SIGNAL_ZONE_UNREACHABLE` with `zone` that cut.
   4. For each non-empty signaling RRset, of type T: at least one RRSIG in the
      answer section covering T at N, with Z as Signer's Name, MUST verify
      against the DNSKEY RRset of Z, with `packetTime` of the response as the
      reference time. Otherwise emit `DS24_SIGNAL_UNSIGNED` with `query_type`
      T.
   5. For each type T in `CDS`, `CDNSKEY` without `DS24_SIGNAL_UNSIGNED`:
      compare the signaling RRset of T with the apex RRset of T of every
      delegation nameserver that answered T (RFC 9615 section 4.2 step 4).
      Comparison is by content: owner name and TTL are ignored, an empty RRset
      equals only an empty RRset. A difference from any of them: emit
      `DS24_SIGNAL_MISMATCH` with `query_type` T.
   6. No tag of steps 5.3 to 5.5 for H, and no indeterminate result: emit
      `DS24_SIGNAL_VALIDATED` with `zone` Z.
6. Aggregate the absent names:
   - At least one name reached `signal`: emit `DS24_SIGNAL_MISSING` for each
     absent name.
   - No name reached `signal`: emit one `DS24_NO_SIGNAL` with the absent names.
7. Every name of step 5 emitted `DS24_SIGNAL_VALIDATED` and
   `DS24_APEX_UNAVAILABLE` was not emitted: emit `DS24_BOOTSTRAP_READY`.
8. Emit `TEST_CASE_END`.

### Descent

A level is a zone cut C on the path to N, with the servers that serve C, the
DS RRset of C with its RRSIG records as the parent of C served them, and the
level of that parent. The root level has the root servers and no DS. Levels
are memoized by cut name for the whole testcase and shared between signaling
names.

Servers of a level: the `NS` names of the referral that created it, in
lexicographic order. The addresses of a name are the `A` and `AAAA` records
owned by it in the additional section of that referral, when the name lies at
or below the referring cut; otherwise they are resolved through the
recursor. Servers on a disabled transport are left out. The first address of
each name precedes the second address of any name. At most two servers are
asked per question.

Descent for N:

1. Start at the deepest memoized level whose cut is a proper ancestor of N; the
   root level when none is.
2. Query `N CDS` at the servers of the level in turn, with DNSSEC enabled (UDP,
   retry over TCP on `TC`). Classify the response:
   - Referral (not `AA`, `NOERROR`, empty answer section, an `NS` RRset in the
     authority section) with owner C:
     - C equals N: `at cut`. RFC 9615 section 4.1 forbids a zone cut at a
       signaling name.
     - C strictly below the current cut and a proper ancestor of N: the level
       of C is created from the referral, unless memoized, and the descent
       continues there at step 2. The DS RRset of C and the RRSIG records
       covering it are taken from the authority section.
     - Otherwise, the same cut, a cut above it or a name off the path: no
       answer.
   - `AA` `NXDOMAIN`: `absent`.
   - `AA` `NOERROR`: query `N CDNSKEY` at the same server. An `AA` `NXDOMAIN`
     gives an empty CDNSKEY RRset, an `AA` `NOERROR` its answer-section
     CDNSKEY records owned by N; any other response is no answer from this
     server. Both RRsets empty: `absent`. Otherwise `signal`.
   - Any other response, or none: no answer.
3. No server of the level gave an answer: the level is unreachable, and `N` is
   `unreachable` at its cut. The state is memoized: a later name below the same
   level is unreachable without a query.
4. The descent is bounded by the label count of N.

The signaling zone Z of a `signal` result is the Signer's Name of the first
answer-section RRSIG covering a non-empty signaling RRset, CDS first, when that
name is at or below the cut C of the answering level and an ancestor of N.
Otherwise Z is C.

- Z equals N: `at cut`.
- Z strictly below C: one server serves both C and Z, and no referral marked
  the cut. Query `Z DS` at the servers of C, with DNSSEC enabled:
  - A referral to a cut M strictly below C, at or above Z: the level of M is
    created from the referral; if M is Z the level of Z is found, otherwise
    the query repeats from M.
  - `AA` `NOERROR` with a DS RRset for Z: the level of Z is created with that
    DS RRset and its RRSIG records. When their Signer's Name P lies strictly
    between C and Z, the level of P is located the same way first and becomes
    the parent of Z. The servers of Z are the servers of C, the answering
    server first.
  - `AA` `NOERROR` without a DS RRset: the level of Z is created without DS.
  - `AA` `NXDOMAIN`: the level of Z is created as broken.
  - No answer from both servers: `unreachable` at C.

### Verification

Verification of a level, memoized per level, after its parent level verified
secure:

- Root level: query `. DNSKEY` at the root servers. A DNSKEY MUST match a root
  trust anchor by key tag, algorithm and digest, and that key MUST verify an
  RRSIG covering the DNSKEY RRset. Failure: broken at `.`.
- Any other level C (RFC 4035 section 5.2):
  - No DS RRset: insecure at C. The denial of existence of the DS is not
    verified.
  - An RRSIG covering the DS RRset, with the parent cut as Signer's Name, MUST
    verify against the DNSKEY RRset of the parent. Failure: broken at C.
  - Query `C DNSKEY` at the servers of C. No `AA` `NOERROR` response:
    unreachable at C. A DNSKEY MUST match a DS record of C by key tag,
    algorithm and digest, and that key MUST verify an RRSIG covering the
    DNSKEY RRset. Failure: broken at C.
- A level created as broken: broken at its cut.
- A signature whose algorithm the local verifier cannot process is
  indeterminate: the signaling name gets no tag of steps 5.3 to 5.6.

Signature checks use `packetTime` of the response carrying the signature.

{{% expand "Show diagram" %}}
```
parent DS: fake DS, or any parent NS with AA NOERROR and a DS at the zone
   yes -> DS24_DELEGATION_SECURE, end
apex: CDS and CDNSKEY per delegation NS (by IP)
   no non-empty RRset             -> DS24_NO_CDS_CDNSKEY, end
   delete records only            -> DS24_DELETE_REQUESTED, end
   NS without AA NOERROR for a type -> DS24_APEX_UNAVAILABLE, continue
names = delegation NS names not in the zone
   none -> DS24_ONLY_IN_DOMAIN_NS, end

for each name H:
   N = _dsboot.<zone>._signal.<H>
      > 255 octets -> DS24_SIGNAL_NAME_TOO_LONG
   descend from the deepest known cut above N, N CDS, two servers per cut
      referral to N        -> DS24_SIGNAL_AT_ZONE_CUT
      no usable answer     -> DS24_SIGNAL_ZONE_UNREACHABLE (zone)
      AA NXDOMAIN / both empty -> absent
      signal: Z = signer; Z below the cut -> Z DS at that cut
         Z == N            -> DS24_SIGNAL_AT_ZONE_CUT
         verify root anchors, DS, DNSKEY top down to Z
            no DS          -> DS24_SIGNAL_ZONE_INSECURE (zone)
            fails          -> DS24_SIGNAL_CHAIN_BROKEN (zone)
            no DNSKEY      -> DS24_SIGNAL_ZONE_UNREACHABLE (zone)
         per non-empty type: RRSIG by Z verifies, else DS24_SIGNAL_UNSIGNED
         per type: equals the apex RRset of every NS, else DS24_SIGNAL_MISMATCH
         no fault          -> DS24_SIGNAL_VALIDATED (zone=Z)

absent names: any signal -> DS24_SIGNAL_MISSING each; else DS24_NO_SIGNAL
all VALIDATED, no DS24_APEX_UNAVAILABLE -> DS24_BOOTSTRAP_READY
```
{{% /expand %}}

## Emitted Tags (Possible Set)
| Tag | Emitted when |
| --- | --- |
| `DS24_APEX_UNAVAILABLE` | A delegation nameserver gave no `AA` `NOERROR` response to the apex `CDS` or `CDNSKEY` question (RFC 9615 section 4.2 step 2). |
| `DS24_BOOTSTRAP_READY` | Every signaling name validated and every delegation nameserver answered both apex questions. |
| `DS24_DELEGATION_SECURE` | A parent nameserver returned a DS record for the zone, or fake DS data exists (RFC 9615 section 4.2 step 1). |
| `DS24_DELETE_REQUESTED` | Every apex CDS and CDNSKEY record is an RFC 8078 delete record. |
| `DS24_NO_CDS_CDNSKEY` | No delegation nameserver returned a non-empty apex CDS or CDNSKEY RRset. |
| `DS24_NO_SIGNAL` | No signaling name carries CDS or CDNSKEY records, and at least one is absent. |
| `DS24_ONLY_IN_DOMAIN_NS` | Every delegation nameserver name is at or below the zone name. |
| `DS24_SIGNAL_AT_ZONE_CUT` | A zone cut exists at the signaling name. |
| `DS24_SIGNAL_CHAIN_BROKEN` | A DS, DNSKEY or RRSIG on the path to the signaling zone fails to validate (RFC 9615 section 3.1). |
| `DS24_SIGNAL_MISMATCH` | The signaling RRset of a type differs from the apex RRset of that type on at least one delegation nameserver. |
| `DS24_SIGNAL_MISSING` | The signaling name of a nameserver is absent while another signaling name carries records (RFC 9615 section 4.1). |
| `DS24_SIGNAL_NAME_TOO_LONG` | The signaling name exceeds 255 octets in wire form. |
| `DS24_SIGNAL_UNSIGNED` | No RRSIG by the signaling zone verifies a non-empty signaling RRset (RFC 9615 section 4.1). |
| `DS24_SIGNAL_VALIDATED` | The signaling name validates from the root and its RRsets equal the apex RRsets. |
| `DS24_SIGNAL_ZONE_INSECURE` | A zone cut on the path to the signaling zone has no DS (RFC 9615 section 3.1). |
| `DS24_SIGNAL_ZONE_UNREACHABLE` | No server of a zone cut on the path gave a usable answer (RFC 9615 section 4.2 step 3). |
| `IPV4_DISABLED` | IPv4 transport is disabled for a parent (`DS`) or delegation (`CDS`, `CDNSKEY`) nameserver. |
| `IPV6_DISABLED` | IPv6 transport is disabled for a parent (`DS`) or delegation (`CDS`, `CDNSKEY`) nameserver. |
| `TEST_CASE_END` | Testcase completion marker is emitted. |
| `TEST_CASE_START` | Testcase start marker is emitted. |

## Tag Arguments
| Tag | Argument key | Type | Meaning |
| --- | --- | --- | --- |
| `DS24_APEX_UNAVAILABLE` | `servers` | `array<object>` | Delegation nameservers (`{ns,address}`) without an `AA` `NOERROR` response for at least one of the two types. |
| `DS24_DELEGATION_SECURE` | `servers` | `array<object>` | Parent nameservers (`{ns,address}`) that returned or hold a DS record. |
| `DS24_NO_SIGNAL` | `servers` | `array<object>` | Nameserver names (`{ns}`) whose signaling name is absent. |
| `DS24_ONLY_IN_DOMAIN_NS` | `servers` | `array<object>` | The delegation nameserver names (`{ns}`). |
| `DS24_SIGNAL_AT_ZONE_CUT` | `ns` | `string` | Nameserver name under whose signaling domain the name lies. |
| `DS24_SIGNAL_AT_ZONE_CUT` | `query_name` | `string` | Signaling name. |
| `DS24_SIGNAL_CHAIN_BROKEN` | `ns` | `string` | Nameserver name under whose signaling domain the name lies. |
| `DS24_SIGNAL_CHAIN_BROKEN` | `zone` | `string` | Zone cut where validation fails. |
| `DS24_SIGNAL_MISMATCH` | `ns` | `string` | Nameserver name under whose signaling domain the name lies. |
| `DS24_SIGNAL_MISMATCH` | `query_name` | `string` | Signaling name. |
| `DS24_SIGNAL_MISMATCH` | `query_type` | `string` | `CDS` or `CDNSKEY`. |
| `DS24_SIGNAL_MISSING` | `ns` | `string` | Nameserver name under whose signaling domain the name lies. |
| `DS24_SIGNAL_MISSING` | `query_name` | `string` | Signaling name. |
| `DS24_SIGNAL_NAME_TOO_LONG` | `ns` | `string` | Nameserver name under whose signaling domain the name lies. |
| `DS24_SIGNAL_NAME_TOO_LONG` | `query_name` | `string` | Signaling name. |
| `DS24_SIGNAL_UNSIGNED` | `ns` | `string` | Nameserver name under whose signaling domain the name lies. |
| `DS24_SIGNAL_UNSIGNED` | `query_name` | `string` | Signaling name. |
| `DS24_SIGNAL_UNSIGNED` | `query_type` | `string` | `CDS` or `CDNSKEY`. |
| `DS24_SIGNAL_UNSIGNED` | `zone` | `string` | Signaling zone. |
| `DS24_SIGNAL_VALIDATED` | `ns` | `string` | Nameserver name under whose signaling domain the name lies. |
| `DS24_SIGNAL_VALIDATED` | `query_name` | `string` | Signaling name. |
| `DS24_SIGNAL_VALIDATED` | `zone` | `string` | Signaling zone. |
| `DS24_SIGNAL_ZONE_INSECURE` | `ns` | `string` | Nameserver name under whose signaling domain the name lies. |
| `DS24_SIGNAL_ZONE_INSECURE` | `zone` | `string` | Zone cut without DS. |
| `DS24_SIGNAL_ZONE_UNREACHABLE` | `ns` | `string` | Nameserver name under whose signaling domain the name lies. |
| `DS24_SIGNAL_ZONE_UNREACHABLE` | `zone` | `string` | Zone cut whose servers gave no usable answer. |
| `DS24_SIGNAL_ZONE_UNREACHABLE` | `servers` | `array<object>` | Servers (`{ns,address}`) asked at that cut. |
| `IPV4_DISABLED` | `ns` | `string` | Nameserver identity skipped on IPv4. |
| `IPV4_DISABLED` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `IPV4_DISABLED` | `query_type` | `string` | rrtype skipped (`DS`, `CDS` or `CDNSKEY`). |
| `IPV6_DISABLED` | `ns` | `string` | Nameserver identity skipped on IPv6. |
| `IPV6_DISABLED` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `IPV6_DISABLED` | `query_type` | `string` | rrtype skipped (`DS`, `CDS` or `CDNSKEY`). |
| `TEST_CASE_END` | `testcase` | `string` | Testcase display name (`DNSSEC24`). |
| `TEST_CASE_START` | `testcase` | `string` | Testcase display name (`DNSSEC24`). |

`DS24_BOOTSTRAP_READY`, `DS24_DELETE_REQUESTED` and `DS24_NO_CDS_CDNSKEY`
carry no arguments.

## Severity Levels Per Tag
A parental agent aborts the procedure of RFC 9615 section 4.2 on every
WARNING-level condition. A zone that publishes no signal waits for the RFC 8078
section 3.3 acceptance delay, which `DS07_NO_DS_FOR_SIGNED_ZONE` already
reports.

| Tag | Level | Notes |
| --- | --- | --- |
| `DS24_APEX_UNAVAILABLE` | `WARNING` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS24_BOOTSTRAP_READY` | `INFO` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS24_DELEGATION_SECURE` | `INFO` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS24_DELETE_REQUESTED` | `INFO` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS24_NO_CDS_CDNSKEY` | `INFO` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS24_NO_SIGNAL` | `NOTICE` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS24_ONLY_IN_DOMAIN_NS` | `NOTICE` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS24_SIGNAL_AT_ZONE_CUT` | `WARNING` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS24_SIGNAL_CHAIN_BROKEN` | `WARNING` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS24_SIGNAL_MISMATCH` | `WARNING` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS24_SIGNAL_MISSING` | `WARNING` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS24_SIGNAL_NAME_TOO_LONG` | `NOTICE` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS24_SIGNAL_UNSIGNED` | `WARNING` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS24_SIGNAL_VALIDATED` | `INFO` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS24_SIGNAL_ZONE_INSECURE` | `WARNING` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS24_SIGNAL_ZONE_UNREACHABLE` | `WARNING` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `IPV4_DISABLED` | `DEBUG2` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `IPV6_DISABLED` | `DEBUG2` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `TEST_CASE_END` | `DEBUG` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `TEST_CASE_START` | `DEBUG` | Default from `share/profile.json` (`test_levels.DNSSEC`). |

Scoring takes the severity default in the `dnssec` dimension. No
`TagPenalties` override is defined.

## Differences From Upstream
- Upstream reference: no upstream equivalent. No upstream testcase reads an
  RFC 9615 signaling name.
- Potential upstream report:
  - `yes`
  - Upstream expected behavior: the bootstrapping signals of a signed zone
    without DS are validated as a parental agent validates them.
  - Gonemaster observed behavior: DNSSEC24 validates them.
  - evidence: `engine/test/dnssec/dnssec24_test.go`.
  - report status: `not filed`.

## Edge Cases And Limitations
- The root zone has no parent DS and publishes no CDS; it stops at
  `DS24_NO_CDS_CDNSKEY`. The DS view of a top-level domain comes from the root
  servers.
- A nameserver name equal to the zone name is in-domain.
- A wildcard in the signaling zone synthesizes signaling RRsets that validate
  as any other; the testcase does not distinguish them.
- Apex delete records alone stop at the gate. Mixed with other records they
  are content as any other, and DNSSEC16 and DNSSEC17 report the mix.
- A signaling name that does not exist MAY be answered `NODATA` with compact
  denial instead of `NXDOMAIN`; both are `absent`. After `NODATA` on `CDS` the
  `CDNSKEY` question is still asked.
- A `CNAME` at the signaling name is not followed; the name counts by the
  records owned by N.
- The denial of existence that marks a cut insecure is not verified. A cut
  without DS is insecure whether or not the denial would validate.
- A server whose address the non-global target guard refuses gives no
  answer, which can leave a cut unreachable.
- The signaling zones are probed from one vantage point. A lame anycast
  instance is reported as seen.
- An inconsistent apex RRset of one type yields `DS24_SIGNAL_MISMATCH` for that
  type at every signaling name, beside the DNSSEC15 inconsistency tags. Content
  is compared over every digest type; the RFC 9975 digest filter of DNSSEC15
  does not apply.
- A top-level domain passes the gate like any zone. Its parental agent is the
  root zone operator.

## Implementation Notes
Implementation-defined choices, none of them mandated by the protocol:

- Validation is an own walk from the root hints with the embedded IANA root
  trust anchors, not the AD bit of a resolver. The verdict does not depend on
  the host, and the finding names the zone cut that fails.
- The `DNSKEY` RRsets of the path are queried only for a signaling name that
  carries records. An absent signaling name costs one query per zone cut
  above it, and a name sharing the cuts of an earlier one costs one query.
- Signaling names are walked in sequence, sharing the level memo. A level
  found unreachable stays unreachable for the rest of the testcase.
- At most two servers are asked per question; the first address of each name
  precedes the second address of any name.
- The apex queries share the option set of DNSSEC15, and the parent `DS`
  queries that of DNSSEC07, so a full run serves them from the cache.
- `DS24_BOOTSTRAP_READY` is OK-tag gated on every signaling name validating.

## Evidence In Gonemaster
- Code paths:
  - `engine/test/dnssec/dnssec.go` (DNSSEC24 testcase function and signaling
    walker).
  - `engine/dsboot/dsboot.go` (signaling name, signaling hosts, delete
    records, content comparison).
  - `engine/dnssecutil/anchors.go` (root trust anchors).
- Related tests:
  - `engine/test/dnssec/dnssec24_test.go`.
  - `engine/dsboot/dsboot_test.go`.
- References:
  - RFC 9615 sections 3.1, 3.2, 4.1, 4.2, 4.4 (authenticated bootstrapping).
  - RFC 8078 sections 3 and 4 (acceptance policy, delete records).
  - RFC 7344 (CDS and CDNSKEY).
  - RFC 4035 section 5 (authenticating responses, chain of trust).
  - RFC 1035 section 3.1 (name length).
  - IANA root trust anchors, `https://data.iana.org/root-anchors/root-anchors.xml`.
