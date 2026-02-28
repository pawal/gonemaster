# DNSSEC10 (dnssec10)

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
5. Run `NSEC3PARAM` query processing:
   - Response-shape failure => `NSEC3PARAM query response error` set.
   - Non-empty answer with NSEC3PARAM => NSEC3PARAM-in-answer path (multi-record, apex-owner checks).
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

## Emitted Tags (Possible Set)
| Tag | Emitted when |
| --- | --- |
| `DS10_ALGO_NOT_SUPPORTED_BY_ZM` | Signature verification required an unsupported algorithm. |
| `DS10_ERR_MULT_NSEC` | More than one NSEC record was observed where one was expected. |
| `DS10_ERR_MULT_NSEC3` | More than one NSEC3 record was observed where one was expected. |
| `DS10_ERR_MULT_NSEC3PARAM` | More than one NSEC3PARAM record was observed where one was expected. |
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
| `DS10_ALGO_NOT_SUPPORTED_BY_ZM` | `keytag`, `algo_num`, `algo_mnemo`, `ns_ip_list` | `int`, `int`, `string`, `string` | Unsupported algorithm details and semicolon-delimited nameserver identity/IP list from verification path. |
| `DS10_ERR_MULT_NSEC` | `ns_list` | `string` | Semicolon-delimited nameserver identities (`name/ip`). |
| `DS10_ERR_MULT_NSEC3` | `ns_list` | `string` | Semicolon-delimited nameserver identities (`name/ip`). |
| `DS10_ERR_MULT_NSEC3PARAM` | `ns_list` | `string` | Semicolon-delimited nameserver identities (`name/ip`). |
| `DS10_EXPECTED_NSEC_NSEC3_MISSING` | `ns_list` | `string` | Semicolon-delimited nameserver identities (`name/ip`). |
| `DS10_HAS_NSEC` | `ns_list` | `string` | Semicolon-delimited nameserver identities (`name/ip`). |
| `DS10_HAS_NSEC3` | `ns_list` | `string` | Semicolon-delimited nameserver identities (`name/ip`). |
| `DS10_INCONSISTENT_NSEC` | `ns_list` | `string` | Semicolon-delimited nameserver identities (`name/ip`). |
| `DS10_INCONSISTENT_NSEC3` | `ns_list` | `string` | Semicolon-delimited nameserver identities (`name/ip`). |
| `DS10_INCONSISTENT_NSEC_NSEC3` | `ns_list` | `string` | Semicolon-delimited nameserver identities (`name/ip`). |
| `DS10_MIXED_NSEC_NSEC3` | `ns_list` | `string` | Semicolon-delimited nameserver identities (`name/ip`). |
| `DS10_NSEC3PARAM_GIVES_ERR_ANSWER` | `ns_list` | `string` | Semicolon-delimited nameserver identities (`name/ip`). |
| `DS10_NSEC3PARAM_MISMATCHES_APEX` | `ns_list` | `string` | Semicolon-delimited nameserver identities (`name/ip`). |
| `DS10_NSEC3PARAM_QUERY_RESPONSE_ERR` | `ns_list` | `string` | Semicolon-delimited nameserver identities (`name/ip`). |
| `DS10_NSEC3_ERR_TYPE_LIST` | `ns_list` | `string` | Semicolon-delimited nameserver identities (`name/ip`). |
| `DS10_NSEC3_MISMATCHES_APEX` | `ns_list` | `string` | Semicolon-delimited nameserver identities (`name/ip`). |
| `DS10_NSEC3_MISSING_SIGNATURE` | `ns_list` | `string` | Semicolon-delimited nameserver identities (`name/ip`). |
| `DS10_NSEC3_NODATA_MISSING_SOA` | `ns_list` | `string` | Semicolon-delimited nameserver identities (`name/ip`). |
| `DS10_NSEC3_NODATA_WRONG_SOA` | `domain`, `ns_list` | `string`, `string` | Wrong SOA owner domain and affected nameserver identities (`name/ip`). |
| `DS10_NSEC3_NO_VERIFIED_SIGNATURE` | `ns_list` | `string` | Semicolon-delimited nameserver identities (`name/ip`) lacking any verified NSEC3 signature. |
| `DS10_NSEC3_RRSIG_EXPIRED` | `keytag`, `ns_list` | `int`, `string` | Keytag and affected nameserver identities (`name/ip`). |
| `DS10_NSEC3_RRSIG_NOT_YET_VALID` | `keytag`, `ns_list` | `int`, `string` | Keytag and affected nameserver identities (`name/ip`). |
| `DS10_NSEC3_RRSIG_NO_DNSKEY` | `keytag`, `ns_list` | `int`, `string` | Keytag and affected nameserver identities (`name/ip`). |
| `DS10_NSEC3_RRSIG_VERIFY_ERROR` | `keytag`, `ns_list` | `int`, `string` | Keytag and affected nameserver identities (`name/ip`). |
| `DS10_NSEC_ERR_TYPE_LIST` | `ns_list` | `string` | Semicolon-delimited nameserver identities (`name/ip`). |
| `DS10_NSEC_GIVES_ERR_ANSWER` | `ns_list` | `string` | Semicolon-delimited nameserver identities (`name/ip`). |
| `DS10_NSEC_MISMATCHES_APEX` | `ns_list` | `string` | Semicolon-delimited nameserver identities (`name/ip`). |
| `DS10_NSEC_MISSING_SIGNATURE` | `ns_list` | `string` | Semicolon-delimited nameserver identities (`name/ip`). |
| `DS10_NSEC_NODATA_MISSING_SOA` | `ns_list` | `string` | Semicolon-delimited nameserver identities (`name/ip`). |
| `DS10_NSEC_NODATA_WRONG_SOA` | `domain`, `ns_list` | `string`, `string` | Wrong SOA owner domain and affected nameserver identities (`name/ip`). |
| `DS10_NSEC_NO_VERIFIED_SIGNATURE` | `ns_list` | `string` | Semicolon-delimited nameserver identities (`name/ip`) lacking any verified NSEC signature. |
| `DS10_NSEC_QUERY_RESPONSE_ERR` | `ns_list` | `string` | Semicolon-delimited nameserver identities (`name/ip`). |
| `DS10_NSEC_RRSIG_EXPIRED` | `keytag`, `ns_list` | `int`, `string` | Keytag and affected nameserver identities (`name/ip`). |
| `DS10_NSEC_RRSIG_NOT_YET_VALID` | `keytag`, `ns_list` | `int`, `string` | Keytag and affected nameserver identities (`name/ip`). |
| `DS10_NSEC_RRSIG_NO_DNSKEY` | `keytag`, `ns_list` | `int`, `string` | Keytag and affected nameserver identities (`name/ip`). |
| `DS10_NSEC_RRSIG_VERIFY_ERROR` | `keytag`, `ns_list` | `int`, `string` | Keytag and affected nameserver identities (`name/ip`). |
| `DS10_SERVER_NO_DNSSEC` | `ns_list` | `string` | Semicolon-delimited nameserver identities (`name/ip`) without DNSKEY among mixed responders. |
| `DS10_ZONE_NO_DNSSEC` | `ns_list` | `string` | Semicolon-delimited nameserver identities (`name/ip`) without DNSKEY when zone appears unsigned. |
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
| `DS10_ERR_MULT_NSEC3PARAM` | `ERROR` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
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
  - Upstream: summary for `DS10_INCONSISTENT_NSEC_NSEC3` describes two separate lists (`ns_list_nsec`, `ns_list_nsec3`). Gonemaster: emits a single combined `ns_list` argument.
  - Upstream: most DS10 tags are documented with `ns_list`. Gonemaster: `DS10_ALGO_NOT_SUPPORTED_BY_ZM` uses `ns_ip_list` while other DS10 tags use `ns_list`.
  - Upstream: does not explicitly specify testcase boundary and per-query transport debug emissions in this testcase summary. Gonemaster: emits `TEST_CASE_START`, `TEST_CASE_END`, `IPV4_DISABLED`, and `IPV6_DISABLED`.
- Potential upstream report:
  - `no`

## Implementation Notes

The following behaviors are implementation choices, not mandated by RFC 4034/4035/5155:

- **Reference time source**: RRSIG validity checks use wall-clock time (`time.Now().UTC()`) as the reference "now".  RFC 4034 requires checking whether signatures are currently valid; using wall-clock time rather than packet timestamps (as `dnssec04` does) is an implementation choice appropriate for aggregate multi-nameserver analysis where a single consistent reference point is preferred.
- **Deduplication by IP**: The nameserver set is built by IP address; delegation and zone NS entries sharing the same IP are merged.  First-seen nameserver identity string (`name/ip`) is used in output arguments.  The protocol does not specify how to handle NS records for the same IP from different sources.
- **`ns_list` vs `ns_ip_list` argument name**: Most DS10 tags use `ns_list` (nameserver identity strings) while `DS10_ALGO_NOT_SUPPORTED_BY_ZM` uses `ns_ip_list` (raw IPs from the signature verification path).  This asymmetry is an implementation-defined output format.

## Edge Cases And Limitations
- Nameserver processing is deduplicated by IP; all DS10 output tags except `DS10_ALGO_NOT_SUPPORTED_BY_ZM` report `ns_list` as nameserver identity strings (`name/ip`) rather than raw IPs.
- `DS10_NSEC_NO_VERIFIED_SIGNATURE` and `DS10_NSEC3_NO_VERIFIED_SIGNATURE` are suppressed per nameserver when at least one signature verifies for that nameserver.
- Signature validity checks use testcase wall-clock time (`time.Now().UTC()`), not packet capture timestamps.
