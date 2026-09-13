# DNSSEC07

Status: Final

## Purpose
- Determine whether the child zone is signed (based on DNSKEY + covering RRSIG observations) and, for signed zones, whether parent-side DS data is present and consistent.
- Distinguish the two readings of a signed zone with no parent DS: a parent
  that is silent about the zone, which is an island of security, and a parent
  that proves under signature that no delegation exists at the name, which
  every validating resolver rejects.

## Preconditions And Inputs
- Preconditions:
  - A `zone.Zone` object is available.
- Required inputs:
  - Child nameserver name/IP items from [`DelegationNameservers`](../../nameserver-resolution.md#delegationnameservers) and [`ZoneNameservers`](../../nameserver-resolution.md#zonenameservers).
  - Parent nameservers from [`ParentNameservers`](../../nameserver-resolution.md#parentnameservers) (or undelegated fake-DS data path).
  - SOA, DNSKEY, and DS query responses.
  - For the no-delegation verdict only: the `DS` RRset of the parent zone at a
    grandparent nameserver, and the apex `DNSKEY` RRset of the parent zone.
- Profile/config knobs that affect behavior:
  - `net.ipv4` and `net.ipv6`: disabled transports are skipped with transport debug tags.
  - `resolver.defaults.parallel`: parallel child and parent query execution fanout.

## Algorithm And Decision Flow
1. Emit `TEST_CASE_START`.
2. Build child nameserver set from delegation+zone NS items (grouped by IP).
3. For each unique child nameserver IP (parallelized):
   - If transport is disabled, emit `IPV4_DISABLED` or `IPV6_DISABLED` for rrtypes `SOA`, `DNSKEY`, and `DS`, then skip.
   - Query `SOA`:
     - If no response, non-`NOERROR`, non-`AA`, or no SOA answer, classify nameserver as ignored child NS.
   - Query `DNSKEY` with DNSSEC enabled:
     - No response -> `No Response DNSKEY`.
     - Non-`AA` -> `No Auth DNSKEY`.
     - `RCODE != NOERROR` -> `Error RCODE DNSKEY`.
     - Else inspect answer for an `RRSIG` covering `DNSKEY`:
       - If found -> nameserver classified as signed response.
       - Else -> nameserver classified as no DNSKEY-signature response.
4. Build parent nameserver set.
5. Undelegated DS shortcut:
   - If parent-zone fake DS records exist for child, set DS-in-response set to `"-"` and skip parent DS querying.
6. If no signed child response exists, clear parent evaluation sets and skip DS parent checks.
7. Otherwise, for each unique parent nameserver IP (parallelized):
   - If transport is disabled, emit `IPV4_DISABLED` or `IPV6_DISABLED` for rrtype `DS`, then skip.
   - Query `DS` with DNSSEC enabled.
   - If response fails required shape (`NOERROR`, `OPT`, `DO`, `AA`), classify parent nameserver as ignored.
   - Else, if answer contains `RRSIG` covering `DS` at child owner name, classify as DS present.
   - Else classify as no DS, and record the parent's statement about the name
     as a delegation, read from the response already in hand:
     - The NSEC record whose owner is the zone name, or the NSEC3 record whose
       owner matches the hash of the zone name computed with the salt and
       iteration count of that record:
       - Type bitmap with the NS bit: `delegated`. The parent delegates the
         name and the delegation is insecure.
       - Type bitmap without the NS bit: `denied`. The parent proves no
         delegation exists at the name.
     - No NSEC and no NSEC3 record: `unproven`. The parent zone is unsigned, or
       its denial is incomplete, and states nothing about the name.
8. Emit child-side signing-state tags:
   - If the union of ignored/no-response/non-auth/unexpected-rcode child sets equals all child nameservers, emit `DS07_NOT_SIGNED`.
   - Emit detail tags for non-response, non-auth, unexpected-rcode groups.
   - Emit `DS07_SIGNED_ON_SERVER` for signed-response set.
   - Emit `DS07_NOT_SIGNED_ON_SERVER` for no-DNSKEY-signature set.
   - Emit `DS07_INCONSISTENT_SIGNED` if both signed and not-signed-on-server sets are non-empty.
   - Emit `DS07_SIGNED` if signed set non-empty and no-DNSKEY-signature set empty.
   - Emit `DS07_NOT_SIGNED` if signed set empty and no-DNSKEY-signature set non-empty.
9. Emit parent DS tags:
   - Emit `DS07_NO_DS_ON_PARENT_SERVER` for no-DS set, but only when DS-present set is also non-empty (per-server tag fires only for the inconsistent case; when every parent fails to return DS, the aggregate `DS07_NO_DS_FOR_SIGNED_ZONE` covers it).
   - Emit `DS07_DS_ON_PARENT_SERVER` for DS-present set.
   - Emit `DS07_INCONSISTENT_DS` if both no-DS and DS-present sets are non-empty.
   - If zone is considered signed (`signed response` non-empty and `no DNSKEY-signature` empty):
     - Emit `DS07_NO_DS_FOR_SIGNED_ZONE` when no-DS set non-empty and DS-present set empty.
     - Emit `DS07_DS_FOR_SIGNED_ZONE` when no-DS set empty and DS-present set non-empty.
10. No-delegation verdict. It is reached only when every condition below holds,
    in this order. The first three are read from responses already made, so the
    two queries of the fourth are issued only for a zone that is already
    inconsistent.
    1. `DS07_NO_DS_FOR_SIGNED_ZONE` was emitted: the child is signed and no
       parent nameserver returned a DS.
    2. At least one parent nameserver recorded `denied` in step 7.
    3. No parent nameserver recorded `delegated`. Parent nameservers that
       disagree are reported by `DS07_INCONSISTENT_DS` and reach no verdict
       here.
    4. The parent zone is anchored: query the parent zone's `DS` at a
       grandparent nameserver and the parent zone's apex `DNSKEY` at a parent
       nameserver, both with DNSSEC enabled. The DS RRset MUST carry an RRSIG,
       a DNSKEY of the parent MUST match a DS by keytag, algorithm and digest,
       and the RRSIG covering the NSEC or NSEC3 of step 7 MUST verify under a
       DNSKEY of the parent. A signature whose algorithm the local verifier
       cannot process yields no verdict.
    - All conditions hold: emit `DS07_PARENT_PROVES_NO_DELEGATION` with
      `parent` and the `servers` that proved it.
11. Emit `TEST_CASE_END`.

### Child Signing-State Classification (steps 2-3, 8)

{{% expand "Show diagram" %}}
```
child set = GetDelNSNamesAndIPs ++ GetZoneNSNamesAndIPs; group by IP

For each unique child NS IP (parallel; fan-out = resolver.defaults.parallel):

   transport disabled for SOA/DNSKEY/DS -> IPV4_DISABLED / IPV6_DISABLED, skip
   query SOA at z.Name
    +- no resp / RCODE != NOERROR / !AA / no SOA -> ignored child NS

   query DNSKEY at z.Name, DNSSEC=on
    +- resp.Msg == nil               -> noResponseDNSKEY
    +- !AA                           -> nonAuthDNSKEY
    +- RCODE != NOERROR              -> errRcodeDNSKEY (rcode)
    +- RRSIG covering DNSKEY present -> signedOnServer
    +- otherwise                     -> notSignedOnServer

Child-side emissions:
   union(ignored, noResponseDNSKEY, nonAuthDNSKEY, errRcodeDNSKEY) == all child NS
     -> DS07_NOT_SIGNED (no args)

   noResponseDNSKEY  non-empty -> DS07_NO_RESPONSE_DNSKEY      (servers)
   nonAuthDNSKEY     non-empty -> DS07_NON_AUTH_RESPONSE_DNSKEY (servers)
   errRcodeDNSKEY    non-empty -> DS07_UNEXP_RCODE_RESP_DNSKEY  (servers, rcode)
   signedOnServer    non-empty -> DS07_SIGNED_ON_SERVER         (servers)
   notSignedOnServer non-empty -> DS07_NOT_SIGNED_ON_SERVER     (servers)
   both signedOnServer AND notSignedOnServer non-empty
                               -> DS07_INCONSISTENT_SIGNED      (no args)
   signedOnServer non-empty AND notSignedOnServer empty
                               -> DS07_SIGNED                   (no args)
   signedOnServer empty AND notSignedOnServer non-empty
                               -> DS07_NOT_SIGNED               (no args)
```
{{% /expand %}}

### Parent DS and Final Aggregation (steps 4-9)

{{% expand "Show diagram" %}}
```
parent set = ParentNameservers; group by IP

undelegated DS shortcut:
   any parent NS has FakeDSRecords for z
     -> dsInResponse += "-"; skip parent DS queries

no signedOnServer NS observed
     -> clear parent evaluation sets; skip parent DS phase

Otherwise, for each unique parent NS IP (parallel):

   transport disabled for DS -> IPV4_DISABLED / IPV6_DISABLED, skip
   query DS at z.Name, DNSSEC=on
    +- resp.Msg == nil / RCODE != NOERROR / no EDNS / !DO / !AA -> ignored parent
    +- answer has RRSIG covering DS at z.Name                    -> dsInResponse
    +- otherwise                                                 -> noDSInResponse

Parent-side emissions:
   noDSInResponse non-empty AND dsInResponse non-empty
                              -> DS07_NO_DS_ON_PARENT_SERVER (servers = noDSInResponse)
                              (suppressed when every parent fails)
   dsInResponse non-empty     -> DS07_DS_ON_PARENT_SERVER    (servers)
   both non-empty             -> DS07_INCONSISTENT_DS        (no args)

Signed-zone aggregate (signedOnServer non-empty AND notSignedOnServer empty):
   noDSInResponse non-empty AND dsInResponse empty
                              -> DS07_NO_DS_FOR_SIGNED_ZONE  (no args)
   noDSInResponse empty AND dsInResponse non-empty
                              -> DS07_DS_FOR_SIGNED_ZONE     (no args)

emit TEST_CASE_END
```
{{% /expand %}}

## Emitted Tags (Possible Set)
| Tag | Emitted when |
| --- | --- |
| `DS07_DS_FOR_SIGNED_ZONE` | Zone is considered signed and parent DS-present set is non-empty while no-DS set is empty. |
| `DS07_DS_ON_PARENT_SERVER` | At least one parent nameserver is classified as DS-present. |
| `DS07_INCONSISTENT_DS` | Both parent DS-present and parent no-DS sets are non-empty. |
| `DS07_INCONSISTENT_SIGNED` | Both child signed-response and child no-DNSKEY-signature sets are non-empty. |
| `DS07_NON_AUTH_RESPONSE_DNSKEY` | Child nameservers returned DNSKEY responses without AA. |
| `DS07_NOT_SIGNED` | Zone is determined not signed by child-evaluation logic. |
| `DS07_NOT_SIGNED_ON_SERVER` | Child nameservers whose DNSKEY response carries no RRSIG covering DNSKEY, either because the DNSKEY RRset is absent or because it is unsigned. |
| `DS07_NO_DS_ON_PARENT_SERVER` | At least one parent nameserver returned no DS-signature evidence and at least one other parent nameserver did - i.e., the parent is inconsistent. Suppressed when every parent fails. |
| `DS07_NO_DS_FOR_SIGNED_ZONE` | Zone is considered signed but no parent DS-present evidence exists. |
| `DS07_NO_RESPONSE_DNSKEY` | Child nameservers did not respond to DNSKEY query. |
| `DS07_PARENT_PROVES_NO_DELEGATION` | The child zone is signed, no parent nameserver has a DS, the NSEC or NSEC3 record matching the zone name in the parent carries no NS bit, and the parent zone is anchored by a DS at its own parent. |
| `DS07_SIGNED` | Zone is determined signed by child-evaluation logic. |
| `DS07_SIGNED_ON_SERVER` | Child nameservers returned DNSKEY-covering RRSIG evidence. |
| `DS07_UNEXP_RCODE_RESP_DNSKEY` | Child nameservers returned unexpected DNSKEY query RCODE. |
| `IPV4_DISABLED` | IPv4 transport is disabled for child/parent queries in this testcase. |
| `IPV6_DISABLED` | IPv6 transport is disabled for child/parent queries in this testcase. |
| `TEST_CASE_END` | Testcase completion marker is emitted. |
| `TEST_CASE_START` | Testcase start marker is emitted. |

## Tag Arguments
| Tag | Argument key | Type | Meaning |
| --- | --- | --- | --- |
| `DS07_DS_FOR_SIGNED_ZONE` | `-` | `-` | No arguments. |
| `DS07_DS_ON_PARENT_SERVER` | `servers` | `array<object>` | Structured parent nameserver identities (`{ns,address}` object) with DS-signature evidence, or `-` in undelegated fake-DS path. |
| `DS07_INCONSISTENT_DS` | `-` | `-` | No arguments. |
| `DS07_INCONSISTENT_SIGNED` | `-` | `-` | No arguments. |
| `DS07_NON_AUTH_RESPONSE_DNSKEY` | `servers` | `array<object>` | Structured child nameserver identities (`{ns,address}` object) returning non-AA DNSKEY responses. |
| `DS07_NOT_SIGNED` | `-` | `-` | No arguments. |
| `DS07_NOT_SIGNED_ON_SERVER` | `servers` | `array<object>` | Structured child nameserver identities (`{ns,address}` object) whose DNSKEY response carries no RRSIG covering DNSKEY, either because the DNSKEY RRset is absent or because it is unsigned. |
| `DS07_NO_DS_ON_PARENT_SERVER` | `servers` | `array<object>` | Structured parent nameserver identities (`{ns,address}` object) with no DS-signature evidence. |
| `DS07_NO_DS_FOR_SIGNED_ZONE` | `-` | `-` | No arguments. |
| `DS07_NO_RESPONSE_DNSKEY` | `servers` | `array<object>` | Structured child nameserver identities (`{ns,address}` object) with no DNSKEY response. |
| `DS07_PARENT_PROVES_NO_DELEGATION` | `parent` | `string` | Parent zone whose signed denial proves no delegation exists at the zone name. |
| `DS07_PARENT_PROVES_NO_DELEGATION` | `servers` | `array<object>` | Structured parent nameserver identities (`{ns,address}` object) that returned the denial. |
| `DS07_SIGNED` | `-` | `-` | No arguments. |
| `DS07_SIGNED_ON_SERVER` | `servers` | `array<object>` | Structured child nameserver identities (`{ns,address}` object) with DNSKEY-signature evidence. |
| `DS07_UNEXP_RCODE_RESP_DNSKEY` | `servers` | `array<object>` | Structured child nameserver identities (`{ns,address}` object) returning this unexpected RCODE. |
| `DS07_UNEXP_RCODE_RESP_DNSKEY` | `rcode` | `string` | DNSKEY response RCODE mnemonic. |
| `IPV4_DISABLED` | `ns` | `string` | Nameserver identity (`ns` name only; use `address` for IP) skipped on IPv4. |
| `IPV4_DISABLED` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `IPV4_DISABLED` | `rrtype` | `string` | rrtype skipped (`SOA`, `DNSKEY`, or `DS`). |
| `IPV6_DISABLED` | `ns` | `string` | Nameserver identity (`ns` name only; use `address` for IP) skipped on IPv6. |
| `IPV6_DISABLED` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `IPV6_DISABLED` | `rrtype` | `string` | rrtype skipped (`SOA`, `DNSKEY`, or `DS`). |
| `TEST_CASE_END` | `testcase` | `string` | Testcase display name (`DNSSEC07`). |
| `TEST_CASE_START` | `testcase` | `string` | Testcase display name (`DNSSEC07`). |

## Severity Levels Per Tag
| Tag | Level | Notes |
| --- | --- | --- |
| `DS07_DS_FOR_SIGNED_ZONE` | `INFO` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS07_DS_ON_PARENT_SERVER` | `INFO` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS07_INCONSISTENT_DS` | `ERROR` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS07_INCONSISTENT_SIGNED` | `ERROR` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS07_NON_AUTH_RESPONSE_DNSKEY` | `WARNING` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS07_NOT_SIGNED` | `WARNING` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS07_NOT_SIGNED_ON_SERVER` | `WARNING` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS07_NO_DS_ON_PARENT_SERVER` | `WARNING` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS07_NO_DS_FOR_SIGNED_ZONE` | `WARNING` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS07_NO_RESPONSE_DNSKEY` | `WARNING` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS07_PARENT_PROVES_NO_DELEGATION` | `ERROR` | The zone and every name in it are rejected by validating resolvers. `DS07_NO_DS_FOR_SIGNED_ZONE` keeps its WARNING and is emitted alongside. |
| `DS07_SIGNED` | `INFO` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS07_SIGNED_ON_SERVER` | `INFO` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS07_UNEXP_RCODE_RESP_DNSKEY` | `WARNING` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `IPV4_DISABLED` | `DEBUG` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `IPV6_DISABLED` | `DEBUG` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `TEST_CASE_END` | `DEBUG` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `TEST_CASE_START` | `DEBUG` | Default from `share/profile.json` (`test_levels.DNSSEC`). |

## Differences From Upstream
- Differences (Upstream vs Gonemaster):
  - Upstream: summary text states that if no DNSKEY records are found then no messages are output. Gonemaster: emits not-signed findings (`DS07_NOT_SIGNED_ON_SERVER`, `DS07_NOT_SIGNED`) in that case when child responses are otherwise usable.
  - Upstream: parent DS-positive condition is described as requiring DS plus RRSIG covering DS. Gonemaster: parent DS-positive check is driven by presence of an `RRSIG` covering `DS` for child owner and does not explicitly assert DS RR presence in the same branch.
  - Upstream: the `DS07_NOT_SIGNED_ON_SERVER` message states that the servers respond with no DNSKEY. Gonemaster: the message states that the DNSKEY RRset is not signed, because the classification is RRSIG-based and also fires for zones that publish DNSKEYs (`DIV-DS11-UNSIGNED-DNSKEY`, reported together with dnssec11).
  - Upstream: does not explicitly specify testcase boundary and per-query transport debug emissions in this testcase summary. Gonemaster: emits `TEST_CASE_START`, `TEST_CASE_END`, `IPV4_DISABLED`, and `IPV6_DISABLED`.
- Potential upstream report:
  - `no`

## Edge Cases And Limitations
- Ignored parent nameserver outcomes are tracked internally but have no dedicated DS07 output tag.
- Child transport-disabled path logs rrtypes `SOA`, `DNSKEY`, and `DS`, even though DS is only queried against parent nameservers in this testcase.
- Parent DS evaluation is fully skipped when no signed child response is observed.
- A DNSKEY RRset without a covering RRSIG is classified exactly like an absent DNSKEY RRset. The parent-DS consequence of that state is reported by DNSSEC11, which runs before the module short-circuit.
- The anchoring check of step 10 is one level. It proves that the parent zone is
  anchored by a DS at the grandparent; it does not walk to the root. A
  grandparent that is itself insecure would make the whole subtree insecure, and
  no resolver would reject the zone. The verdict is not reached when the
  grandparent cannot be resolved or its DS query yields no RRSIG.
- A parent that answers `AA` `NXDOMAIN` for the `DS` query also proves no
  delegation, but the response shape check of step 7 classifies it as ignored,
  so no verdict is reached. Basic01 owns a zone name that does not exist at the
  parent.
- An unsigned parent proves nothing about the name. Its `DS` NODATA carries no
  NSEC and no NSEC3, so step 7 records `unproven` and no verdict is reached.
- `DS07_NO_DS_FOR_SIGNED_ZONE` continues to fire whenever no parent nameserver
  has a DS. It says the zone is not anchored; the new tag says why.
