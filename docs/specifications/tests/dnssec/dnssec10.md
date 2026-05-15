# DNSSEC10

Status: Final

## Purpose
- Verify that signed child-zone nameservers consistently provide NSEC or NSEC3 denial-of-existence material (including signatures and owner/type-shape checks) when querying for apex `NSEC` and `NSEC3PARAM`.

## Preconditions And Inputs
- Preconditions:
  - A `zone.Zone` object is available.
- Required inputs:
  - Child nameserver name/IP items from `methodsv2.GetDelNSNamesAndIPs` and `methodsv2.GetZoneNSNamesAndIPs`.
  - DNSKEY, NSEC, and NSEC3PARAM query responses (DNSSEC enabled).
- Profile/config knobs that affect behavior:
  - `net.ipv4` and `net.ipv6`: disabled transports are skipped with transport debug tags.
  - `resolver.defaults.parallel`: per-nameserver parallel execution fanout.

## Algorithm And Decision Flow
1. Emit `TEST_CASE_START`.
2. Build child nameserver set from delegation+zone NS items, grouped by IP.
3. For each unique nameserver IP (parallelized):
   - If transport is disabled, emit `IPV4_DISABLED` or `IPV6_DISABLED` for rrtypes `DNSKEY`, `NSEC`, and `NSEC3PARAM`, mark nameserver ignored, and skip.
   - Query `DNSKEY` (DNSSEC enabled); if response is absent/non-`NOERROR`/non-`AA`, mark ignored and skip.
   - If no apex DNSKEY records are returned, classify nameserver as `without DNSKEY` and skip remaining DS10 checks.
   - Otherwise classify nameserver as `with DNSKEY` and keep DNSKEY set for signature checks.
4. Run `NSEC` query processing:
   - Response-shape failure => `NSEC query response error` set.
   - Non-empty answer with NSEC records => NSEC-in-answer path (multi-record, apex-owner checks).
   - Non-empty answer without NSEC => erroneous-answer set.
   - Empty answer with NSEC3 in authority => NSEC3-NODATA path (SOA presence/owner checks, NSEC3 owner/type-list checks, signature presence and verification checks).
   - Empty answer with NSEC in authority (no NSEC3) => NSEC-NODATA path for RFC 4470 / RFC 9824 white-lies / minimally covering NSEC / compact denial of existence implementations (SOA presence/owner checks, NSEC count/apex-owner checks, signature presence and verification checks; type-list validation is skipped because the synthesized bitmap intentionally excludes the queried type). Treated as NSEC evidence equivalent to NSEC-in-answer for consistency checks.
5. Run `NSEC3PARAM` query processing:
   - Response-shape failure => `NSEC3PARAM query response error` set.
   - Non-empty answer with NSEC3PARAM => NSEC3PARAM-in-answer path (apex-owner check applied to every NSEC3PARAM RR in the RRset; multiple NSEC3PARAM RRs are permitted to accommodate parameter rollover).
   - Non-empty answer without NSEC3PARAM => erroneous-answer set.
   - Empty answer with NSEC in authority => NSEC-NODATA path (SOA presence/owner checks, NSEC owner/type-list checks, signature presence and verification checks).
6. During NSEC/NSEC3 signature verification:
   - Track per-keytag conditions: no matching DNSKEY, expired, not-yet-valid, verify error.
   - Track successful verification per nameserver.
   - Track unsupported algorithm situations as `DS10_ALGO_NOT_SUPPORTED_BY_ZM`.
7. Aggregate outcomes across nameservers and emit structural consistency tags:
   - `DS10_ERR_MULT_*`, `DS10_INCONSISTENT_*`, `DS10_MIXED_NSEC_NSEC3`, `DS10_HAS_NSEC`, `DS10_HAS_NSEC3`.
8. Emit detailed content/signature tags for NSEC and NSEC3/NSEC3PARAM observations.
9. Emit DNSSEC-presence summary tags:
   - `DS10_ZONE_NO_DNSSEC` when only no-DNSKEY responders were observed.
   - `DS10_SERVER_NO_DNSSEC` when mixed DNSKEY/no-DNSKEY responders were observed.
10. Emit `DS10_EXPECTED_NSEC_NSEC3_MISSING` for nameservers with DNSKEY that produced neither expected NSEC nor expected NSEC3 evidence sets.
11. Emit `TEST_CASE_END`.

### Per-Nameserver DNSKEY Classification (step 3)

{{% expand "Show diagram" %}}
{{< mermaid >}}
stateDiagram-v2
    [*] --> transport
    transport : transport check
    transport --> ignored : disabled
    transport --> queryDNSKEY : enabled
    queryDNSKEY : query DNSKEY
    queryDNSKEY --> ignored : bad response
    queryDNSKEY --> noDNSKEY : no apex DNSKEY
    queryDNSKEY --> withDNSKEY : apex DNSKEY present
    ignored : mark ignored
    noDNSKEY : without DNSKEY
    withDNSKEY : with DNSKEY
    ignored --> [*]
    noDNSKEY --> [*]
    withDNSKEY --> nsecProc
    nsecProc : NSEC/NSEC3PARAM checks
    nsecProc --> [*]
{{< /mermaid >}}
{{% /expand %}}

### NSEC Query Processing (step 4)

{{% expand "Show diagram" %}}
{{< mermaid >}}
stateDiagram-v2
    [*] --> querying
    querying : query NSEC at apex
    querying --> respErr : shape failure
    querying --> nonEmpty : non-empty answer
    querying --> empty : empty answer
    nonEmpty --> nsecAns : NSEC in answer
    nonEmpty --> errAns : no NSEC in answer
    empty --> nsec3Nodata : NSEC3 only
    empty --> nsecNodata : NSEC only
    empty --> noEvid : neither
    respErr : response error
    nsecAns : NSEC-in-answer path
    errAns : erroneous answer
    nsec3Nodata : NSEC3-NODATA path
    nsecNodata : NSEC-NODATA path
    noEvid : no evidence
    respErr --> [*]
    nsecAns --> [*]
    errAns --> [*]
    nsec3Nodata --> [*]
    nsecNodata --> [*]
    noEvid --> [*]
{{< /mermaid >}}
{{% /expand %}}

### NSEC3PARAM Query Processing (step 5)

{{% expand "Show diagram" %}}
{{< mermaid >}}
stateDiagram-v2
    [*] --> querying
    querying : query NSEC3PARAM at apex
    querying --> respErr : shape failure
    querying --> nonEmpty : non-empty answer
    querying --> empty : empty answer
    nonEmpty --> nsec3pAns : NSEC3PARAM in answer
    nonEmpty --> errAns : no NSEC3PARAM in answer
    empty --> nsecNodata : NSEC in authority
    empty --> noEvid : neither
    respErr : response error
    nsec3pAns : NSEC3PARAM path
    errAns : erroneous answer
    nsecNodata : NSEC-NODATA path
    noEvid : no evidence
    respErr --> [*]
    nsec3pAns --> [*]
    errAns --> [*]
    nsecNodata --> [*]
    noEvid --> [*]
{{< /mermaid >}}
{{% /expand %}}

### Aggregation and Final Emission (steps 6-11)

{{% expand "Show diagram" %}}
{{< mermaid >}}
stateDiagram-v2
    [*] --> sigTags
    sigTags : signature condition tags
    sigTags --> consTags
    consTags : structural consistency tags
    consTags --> shapeTags
    shapeTags : content and shape tags
    shapeTags --> dkeyCheck
    dkeyCheck : DNSKEY across NSes?
    dkeyCheck --> allDNSKEY : all have DNSKEY
    dkeyCheck --> noDNSKEY : none have DNSKEY
    dkeyCheck --> mixedDNSKEY : mixed
    noDNSKEY --> zoneTag
    zoneTag : DS10_ZONE_NO_DNSSEC
    mixedDNSKEY --> srvTag
    srvTag : DS10_SERVER_NO_DNSSEC
    allDNSKEY --> missing
    zoneTag --> missing
    srvTag --> missing
    missing : NSEC/NSEC3 missing
    missing --> [*]
{{< /mermaid >}}
{{% /expand %}}

## Emitted Tags (Possible Set)
| Tag | Emitted when |
| --- | --- |
| `DS10_ALGO_NOT_SUPPORTED_BY_ZM` | Signature verification required an unsupported algorithm. |
| `DS10_ERR_MULT_NSEC` | More than one NSEC record was observed where one was expected. |
| `DS10_ERR_MULT_NSEC3` | More than one NSEC3 record was observed where one was expected. |
| `DS10_EXPECTED_NSEC_NSEC3_MISSING` | Nameserver had DNSKEY support but did not provide expected NSEC/NSEC3 evidence. |
| `DS10_HAS_NSEC` | Zone behavior is consistently NSEC-only for observed nameservers. |
| `DS10_HAS_NSEC3` | Zone behavior is consistently NSEC3-only for observed nameservers. |
| `DS10_INCONSISTENT_NSEC` | NSEC evidence is inconsistent across nameservers. |
| `DS10_INCONSISTENT_NSEC3` | NSEC3 evidence is inconsistent across nameservers. |
| `DS10_INCONSISTENT_NSEC_NSEC3` | At least one nameserver uses NSEC-only and at least one uses NSEC3-only, with no nameserver exhibiting both simultaneously. |
| `DS10_MIXED_NSEC_NSEC3` | At least one nameserver shows both NSEC and NSEC3 behavior. |
| `DS10_NSEC3PARAM_GIVES_ERR_ANSWER` | NSEC3PARAM query had unexpected non-empty answer content. |
| `DS10_NSEC3PARAM_MISMATCHES_APEX` | NSEC3PARAM owner name did not match zone apex. |
| `DS10_NSEC3PARAM_QUERY_RESPONSE_ERR` | NSEC3PARAM query had no usable authoritative `NOERROR` response. |
| `DS10_NSEC3_ERR_TYPE_LIST` | NSEC3 type bitmap failed mandatory/forbidden checks. |
| `DS10_NSEC3_MISMATCHES_APEX` | NSEC3 owner hash/name did not match expected apex semantics. |
| `DS10_NSEC3_MISSING_SIGNATURE` | NSEC3 RRset had no matching RRSIG coverage. |
| `DS10_NSEC3_NODATA_MISSING_SOA` | NSEC3 NODATA authority response lacked SOA. |
| `DS10_NSEC3_NODATA_WRONG_SOA` | NSEC3 NODATA authority response had SOA with wrong owner. |
| `DS10_NSEC3_NO_VERIFIED_SIGNATURE` | NSEC3 signatures existed but none verified for affected nameservers. |
| `DS10_NSEC3_RRSIG_EXPIRED` | NSEC3 RRSIG expired for given keytag. |
| `DS10_NSEC3_RRSIG_NOT_YET_VALID` | NSEC3 RRSIG not yet valid for given keytag. |
| `DS10_NSEC3_RRSIG_NO_DNSKEY` | NSEC3 RRSIG keytag had no matching DNSKEY. |
| `DS10_NSEC3_RRSIG_VERIFY_ERROR` | NSEC3 RRSIG verification failed for given keytag. |
| `DS10_NSEC_ERR_TYPE_LIST` | NSEC type bitmap failed mandatory/forbidden checks. |
| `DS10_NSEC_GIVES_ERR_ANSWER` | NSEC query had unexpected non-empty answer content. |
| `DS10_NSEC_MISMATCHES_APEX` | NSEC owner name did not match zone apex. |
| `DS10_NSEC_MISSING_SIGNATURE` | NSEC RRset had no matching RRSIG coverage. |
| `DS10_NSEC_NODATA_MISSING_SOA` | NSEC NODATA authority response lacked SOA. |
| `DS10_NSEC_NODATA_WRONG_SOA` | NSEC NODATA authority response had SOA with wrong owner. |
| `DS10_NSEC_NO_VERIFIED_SIGNATURE` | NSEC signatures existed but none verified for affected nameservers. |
| `DS10_NSEC_QUERY_RESPONSE_ERR` | NSEC query had no usable authoritative `NOERROR` response. |
| `DS10_NSEC_RRSIG_EXPIRED` | NSEC RRSIG expired for given keytag. |
| `DS10_NSEC_RRSIG_NOT_YET_VALID` | NSEC RRSIG not yet valid for given keytag. |
| `DS10_NSEC_RRSIG_NO_DNSKEY` | NSEC RRSIG keytag had no matching DNSKEY. |
| `DS10_NSEC_RRSIG_VERIFY_ERROR` | NSEC RRSIG verification failed for given keytag. |
| `DS10_SERVER_NO_DNSSEC` | At least one nameserver returned a usable DNSKEY and at least one nameserver returned no usable DNSKEY. |
| `DS10_ZONE_NO_DNSSEC` | No nameserver returned usable DNSKEY while at least one returned no DNSKEY. |
| `IPV4_DISABLED` | IPv4 transport is disabled for a queried nameserver/rrtype. |
| `IPV6_DISABLED` | IPv6 transport is disabled for a queried nameserver/rrtype. |
| `TEST_CASE_END` | Testcase completion marker is emitted. |
| `TEST_CASE_START` | Testcase start marker is emitted. |

## Tag Arguments
| Tag | Argument key | Type | Meaning |
| --- | --- | --- | --- |
| `DS10_ALGO_NOT_SUPPORTED_BY_ZM` | `keytag`, `algo_num`, `algo_mnemo`, `addresses` | `int`, `int`, `string`, `string` | Unsupported algorithm details and semicolon-delimited nameserver identity/IP list from verification path. |
| `DS10_ERR_MULT_NSEC` | `servers` | `array<object>` | Structured nameserver identities (`{ns,address}` object). |
| `DS10_ERR_MULT_NSEC3` | `servers` | `array<object>` | Structured nameserver identities (`{ns,address}` object). |
| `DS10_EXPECTED_NSEC_NSEC3_MISSING` | `servers` | `array<object>` | Structured nameserver identities (`{ns,address}` object). |
| `DS10_HAS_NSEC` | `servers` | `array<object>` | Structured nameserver identities (`{ns,address}` object). |
| `DS10_HAS_NSEC3` | `servers` | `array<object>` | Structured nameserver identities (`{ns,address}` object). |
| `DS10_INCONSISTENT_NSEC` | `servers` | `array<object>` | Structured nameserver identities (`{ns,address}` object). |
| `DS10_INCONSISTENT_NSEC3` | `servers` | `array<object>` | Structured nameserver identities (`{ns,address}` object). |
| `DS10_INCONSISTENT_NSEC_NSEC3` | `servers` | `array<object>` | Structured nameserver identities (`{ns,address}` object). |
| `DS10_MIXED_NSEC_NSEC3` | `servers` | `array<object>` | Structured nameserver identities (`{ns,address}` object). |
| `DS10_NSEC3PARAM_GIVES_ERR_ANSWER` | `servers` | `array<object>` | Structured nameserver identities (`{ns,address}` object). |
| `DS10_NSEC3PARAM_MISMATCHES_APEX` | `servers` | `array<object>` | Structured nameserver identities (`{ns,address}` object). |
| `DS10_NSEC3PARAM_QUERY_RESPONSE_ERR` | `servers` | `array<object>` | Structured nameserver identities (`{ns,address}` object). |
| `DS10_NSEC3_ERR_TYPE_LIST` | `servers` | `array<object>` | Structured nameserver identities (`{ns,address}` object). |
| `DS10_NSEC3_MISMATCHES_APEX` | `servers` | `array<object>` | Structured nameserver identities (`{ns,address}` object). |
| `DS10_NSEC3_MISSING_SIGNATURE` | `servers` | `array<object>` | Structured nameserver identities (`{ns,address}` object). |
| `DS10_NSEC3_NODATA_MISSING_SOA` | `servers` | `array<object>` | Structured nameserver identities (`{ns,address}` object). |
| `DS10_NSEC3_NODATA_WRONG_SOA` | `domain`, `servers` | `string`, `string` | Wrong SOA owner domain and affected nameserver identities (`name/ip`). |
| `DS10_NSEC3_NO_VERIFIED_SIGNATURE` | `servers` | `array<object>` | Structured nameserver identities (`{ns,address}` object) lacking any verified NSEC3 signature. |
| `DS10_NSEC3_RRSIG_EXPIRED` | `keytag`, `servers` | `int`, `string` | Keytag and affected nameserver identities (`name/ip`). |
| `DS10_NSEC3_RRSIG_NOT_YET_VALID` | `keytag`, `servers` | `int`, `string` | Keytag and affected nameserver identities (`name/ip`). |
| `DS10_NSEC3_RRSIG_NO_DNSKEY` | `keytag`, `servers` | `int`, `string` | Keytag and affected nameserver identities (`name/ip`). |
| `DS10_NSEC3_RRSIG_VERIFY_ERROR` | `keytag`, `servers` | `int`, `string` | Keytag and affected nameserver identities (`name/ip`). |
| `DS10_NSEC_ERR_TYPE_LIST` | `servers` | `array<object>` | Structured nameserver identities (`{ns,address}` object). |
| `DS10_NSEC_GIVES_ERR_ANSWER` | `servers` | `array<object>` | Structured nameserver identities (`{ns,address}` object). |
| `DS10_NSEC_MISMATCHES_APEX` | `servers` | `array<object>` | Structured nameserver identities (`{ns,address}` object). |
| `DS10_NSEC_MISSING_SIGNATURE` | `servers` | `array<object>` | Structured nameserver identities (`{ns,address}` object). |
| `DS10_NSEC_NODATA_MISSING_SOA` | `servers` | `array<object>` | Structured nameserver identities (`{ns,address}` object). |
| `DS10_NSEC_NODATA_WRONG_SOA` | `domain`, `servers` | `string`, `string` | Wrong SOA owner domain and affected nameserver identities (`name/ip`). |
| `DS10_NSEC_NO_VERIFIED_SIGNATURE` | `servers` | `array<object>` | Structured nameserver identities (`{ns,address}` object) lacking any verified NSEC signature. |
| `DS10_NSEC_QUERY_RESPONSE_ERR` | `servers` | `array<object>` | Structured nameserver identities (`{ns,address}` object). |
| `DS10_NSEC_RRSIG_EXPIRED` | `keytag`, `servers` | `int`, `string` | Keytag and affected nameserver identities (`name/ip`). |
| `DS10_NSEC_RRSIG_NOT_YET_VALID` | `keytag`, `servers` | `int`, `string` | Keytag and affected nameserver identities (`name/ip`). |
| `DS10_NSEC_RRSIG_NO_DNSKEY` | `keytag`, `servers` | `int`, `string` | Keytag and affected nameserver identities (`name/ip`). |
| `DS10_NSEC_RRSIG_VERIFY_ERROR` | `keytag`, `servers` | `int`, `string` | Keytag and affected nameserver identities (`name/ip`). |
| `DS10_SERVER_NO_DNSSEC` | `servers` | `array<object>` | Structured nameserver identities (`{ns,address}` object) without DNSKEY among mixed responders. |
| `DS10_ZONE_NO_DNSSEC` | `servers` | `array<object>` | Structured nameserver identities (`{ns,address}` object) without DNSKEY when zone appears unsigned. |
| `IPV4_DISABLED` | `ns`, `rrtype` | `string`, `string` | Disabled nameserver identity (`name/ip`) and skipped rrtype (`DNSKEY`, `NSEC`, `NSEC3PARAM`). |
| `IPV6_DISABLED` | `ns`, `rrtype` | `string`, `string` | Disabled nameserver identity (`name/ip`) and skipped rrtype (`DNSKEY`, `NSEC`, `NSEC3PARAM`). |
| `TEST_CASE_END` | `testcase` | `string` | Testcase display name (`DNSSEC10`). |
| `TEST_CASE_START` | `testcase` | `string` | Testcase display name (`DNSSEC10`). |

## Severity Levels Per Tag
| Tag | Level | Notes |
| --- | --- | --- |
| `DS10_ALGO_NOT_SUPPORTED_BY_ZM` | `NOTICE` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS10_ERR_MULT_NSEC` | `ERROR` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS10_ERR_MULT_NSEC3` | `ERROR` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS10_EXPECTED_NSEC_NSEC3_MISSING` | `ERROR` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS10_HAS_NSEC` | `INFO` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS10_HAS_NSEC3` | `INFO` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS10_INCONSISTENT_NSEC` | `ERROR` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS10_INCONSISTENT_NSEC3` | `ERROR` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS10_INCONSISTENT_NSEC_NSEC3` | `ERROR` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS10_MIXED_NSEC_NSEC3` | `ERROR` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS10_NSEC3PARAM_GIVES_ERR_ANSWER` | `ERROR` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS10_NSEC3PARAM_MISMATCHES_APEX` | `ERROR` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS10_NSEC3PARAM_QUERY_RESPONSE_ERR` | `ERROR` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS10_NSEC3_ERR_TYPE_LIST` | `ERROR` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS10_NSEC3_MISMATCHES_APEX` | `ERROR` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS10_NSEC3_MISSING_SIGNATURE` | `ERROR` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS10_NSEC3_NODATA_MISSING_SOA` | `ERROR` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS10_NSEC3_NODATA_WRONG_SOA` | `ERROR` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS10_NSEC3_NO_VERIFIED_SIGNATURE` | `ERROR` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS10_NSEC3_RRSIG_EXPIRED` | `ERROR` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS10_NSEC3_RRSIG_NOT_YET_VALID` | `ERROR` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS10_NSEC3_RRSIG_NO_DNSKEY` | `WARNING` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS10_NSEC3_RRSIG_VERIFY_ERROR` | `ERROR` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS10_NSEC_ERR_TYPE_LIST` | `ERROR` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS10_NSEC_GIVES_ERR_ANSWER` | `ERROR` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS10_NSEC_MISMATCHES_APEX` | `ERROR` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS10_NSEC_MISSING_SIGNATURE` | `ERROR` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS10_NSEC_NODATA_MISSING_SOA` | `ERROR` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS10_NSEC_NODATA_WRONG_SOA` | `ERROR` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS10_NSEC_NO_VERIFIED_SIGNATURE` | `ERROR` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS10_NSEC_QUERY_RESPONSE_ERR` | `ERROR` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS10_NSEC_RRSIG_EXPIRED` | `ERROR` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS10_NSEC_RRSIG_NOT_YET_VALID` | `ERROR` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS10_NSEC_RRSIG_NO_DNSKEY` | `WARNING` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS10_NSEC_RRSIG_VERIFY_ERROR` | `ERROR` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS10_SERVER_NO_DNSSEC` | `ERROR` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS10_ZONE_NO_DNSSEC` | `NOTICE` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `IPV4_DISABLED` | `DEBUG` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `IPV6_DISABLED` | `DEBUG` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `TEST_CASE_END` | `DEBUG` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `TEST_CASE_START` | `DEBUG` | Default from `share/profile.json` (`test_levels.DNSSEC`). |

## Differences From Upstream
- Upstream reference: [`dnssec10.md`](../../upstream/tests/DNSSEC-TP/dnssec10.md)
- Differences (Upstream vs Gonemaster):
  - Upstream: summary for `DS10_INCONSISTENT_NSEC_NSEC3` describes two separate lists (`ns_list_nsec`, `ns_list_nsec3`). Gonemaster: emits a single combined `servers` argument.
  - Upstream: most DS10 tags are documented with `servers`. Gonemaster: `DS10_ALGO_NOT_SUPPORTED_BY_ZM` uses `addresses` while other DS10 tags use `servers`.
  - Upstream: does not explicitly specify testcase boundary and per-query transport debug emissions in this testcase summary. Gonemaster: emits `TEST_CASE_START`, `TEST_CASE_END`, `IPV4_DISABLED`, and `IPV6_DISABLED`.
  - Upstream: does not handle the case where the NSEC query returns a NODATA response with NSEC in the authority section (RFC 4470 white-lies / minimally covering NSEC / RFC 9824 compact denial of existence, as used by e.g. AWS Route 53, Cloudflare, NS1). This causes a false positive `DS10_INCONSISTENT_NSEC` for zones using on-line signing. Gonemaster: treats NSEC-in-authority on the NSEC query as NSEC evidence equivalent to NSEC-in-answer for consistency checks, and skips type-list validation on the synthesized NSEC record (whose bitmap intentionally excludes the queried type).
  - `DS10_ERR_MULT_NSEC3PARAM` is not emitted (the tag has been removed). RFC 5155 does not constrain the cardinality of the apex `NSEC3PARAM` RRset and a zone may legitimately publish more than one `NSEC3PARAM` RR during a parameter (salt/iterations) rollover. Gonemaster follows the upstream removal: the apex-owner check (`DS10_NSEC3PARAM_MISMATCHES_APEX`) is applied to every NSEC3PARAM RR in the answer instead of only the first. Apex-hash verification for the NSEC3 chain itself uses each NSEC3 record's own salt/iterations, so a multi-chain rollover is already validated correctly without a cardinality check.
- Potential upstream report:
  - `yes` -- the upstream specification lacks a branch for NSEC-in-authority on the NSEC query, causing false `DS10_INCONSISTENT_NSEC` for RFC 4470 / RFC 9824 implementations (see [zonemaster/zonemaster#1424](https://github.com/zonemaster/zonemaster/issues/1424)).

## Implementation Notes

The following behaviors are implementation choices, not mandated by RFC 4034/4035/5155/4470/9824:

- **Reference time source**: RRSIG validity checks use wall-clock time (`time.Now().UTC()`) as the reference "now".  RFC 4034 requires checking whether signatures are currently valid; using wall-clock time rather than packet timestamps (as `dnssec04` does) is an implementation choice appropriate for aggregate multi-nameserver analysis where a single consistent reference point is preferred.
- **Deduplication by IP**: The nameserver set is built by IP address; delegation and zone NS entries sharing the same IP are merged.  First-seen nameserver identity string (`name/ip`) is used in output arguments.  The protocol does not specify how to handle NS records for the same IP from different sources.
- **`servers` vs `addresses` argument name**: Most DS10 tags use `servers` (nameserver identity strings) while `DS10_ALGO_NOT_SUPPORTED_BY_ZM` uses `addresses` (raw IPs from the signature verification path).  This asymmetry is an implementation-defined output format.
- **RFC 4470 / RFC 9824 white-lies NSEC handling**: When the NSEC query returns a NODATA response with NSEC in the authority section (rather than NSEC in the answer section), this is recognized as NSEC evidence from an on-line signing / minimally covering NSEC (RFC 4470) or compact denial of existence (RFC 9824) implementation.  RFC 9824 Section 7.2 explicitly permits dynamically generated NSEC records for owner names that don't exist or are ENTs, relaxing the RFC 4035 constraint.  The synthesized NSEC record's type bitmap intentionally excludes the queried type (NSEC) and may include normally-forbidden types (NSEC3PARAM), so type-list validation is skipped for this record.  All other checks (SOA, count, apex owner, signature) are performed normally.  For consistency purposes this evidence is treated identically to NSEC-in-answer.

## Edge Cases And Limitations
- Nameserver processing is deduplicated by IP; all DS10 output tags except `DS10_ALGO_NOT_SUPPORTED_BY_ZM` report `servers` as nameserver identity strings (`name/ip`) rather than raw IPs.
- `DS10_NSEC_NO_VERIFIED_SIGNATURE` and `DS10_NSEC3_NO_VERIFIED_SIGNATURE` are suppressed per nameserver when at least one signature verifies for that nameserver.
- Signature validity checks use testcase wall-clock time (`time.Now().UTC()`), not packet capture timestamps.
