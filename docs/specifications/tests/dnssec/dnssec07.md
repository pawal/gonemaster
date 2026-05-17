# DNSSEC07

Status: Final

## Purpose
- Determine whether the child zone is signed (based on DNSKEY + covering RRSIG observations) and, for signed zones, whether parent-side DS data is present and consistent.

## Preconditions And Inputs
- Preconditions:
  - A `zone.Zone` object is available.
- Required inputs:
  - Child nameserver name/IP items from `methodsv2.GetDelNSNamesAndIPs` and `methodsv2.GetZoneNSNamesAndIPs`.
  - Parent nameservers from `methodsv2.GetParentNSNamesAndIPs` (or undelegated fake-DS data path).
  - SOA, DNSKEY, and DS query responses.
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
   - Else classify as no DS.
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
10. Emit `TEST_CASE_END`.

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
parent set = methodsv2.GetParentNSNamesAndIPs; group by IP

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
| `DS07_NOT_SIGNED_ON_SERVER` | Child nameservers returned responses without DNSKEY-covering RRSIG evidence. |
| `DS07_NO_DS_ON_PARENT_SERVER` | At least one parent nameserver returned no DS-signature evidence and at least one other parent nameserver did - i.e., the parent is inconsistent. Suppressed when every parent fails. |
| `DS07_NO_DS_FOR_SIGNED_ZONE` | Zone is considered signed but no parent DS-present evidence exists. |
| `DS07_NO_RESPONSE_DNSKEY` | Child nameservers did not respond to DNSKEY query. |
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
| `DS07_NOT_SIGNED_ON_SERVER` | `servers` | `array<object>` | Structured child nameserver identities (`{ns,address}` object) without DNSKEY-signature evidence. |
| `DS07_NO_DS_ON_PARENT_SERVER` | `servers` | `array<object>` | Structured parent nameserver identities (`{ns,address}` object) with no DS-signature evidence. |
| `DS07_NO_DS_FOR_SIGNED_ZONE` | `-` | `-` | No arguments. |
| `DS07_NO_RESPONSE_DNSKEY` | `servers` | `array<object>` | Structured child nameserver identities (`{ns,address}` object) with no DNSKEY response. |
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
| `DS07_SIGNED` | `INFO` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS07_SIGNED_ON_SERVER` | `INFO` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS07_UNEXP_RCODE_RESP_DNSKEY` | `WARNING` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `IPV4_DISABLED` | `DEBUG` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `IPV6_DISABLED` | `DEBUG` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `TEST_CASE_END` | `DEBUG` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `TEST_CASE_START` | `DEBUG` | Default from `share/profile.json` (`test_levels.DNSSEC`). |

## Differences From Upstream
- Upstream reference: [`dnssec07.md`](../../upstream/tests/DNSSEC-TP/dnssec07.md)
- Differences (Upstream vs Gonemaster):
  - Upstream: summary text states that if no DNSKEY records are found then no messages are output. Gonemaster: emits not-signed findings (`DS07_NOT_SIGNED_ON_SERVER`, `DS07_NOT_SIGNED`) in that case when child responses are otherwise usable.
  - Upstream: parent DS-positive condition is described as requiring DS plus RRSIG covering DS. Gonemaster: parent DS-positive check is driven by presence of an `RRSIG` covering `DS` for child owner and does not explicitly assert DS RR presence in the same branch.
  - Upstream: does not explicitly specify testcase boundary and per-query transport debug emissions in this testcase summary. Gonemaster: emits `TEST_CASE_START`, `TEST_CASE_END`, `IPV4_DISABLED`, and `IPV6_DISABLED`.
- Potential upstream report:
  - `no`

## Edge Cases And Limitations
- Ignored parent nameserver outcomes are tracked internally but have no dedicated DS07 output tag.
- Child transport-disabled path logs rrtypes `SOA`, `DNSKEY`, and `DS`, even though DS is only queried against parent nameservers in this testcase.
- Parent DS evaluation is fully skipped when no signed child response is observed.
