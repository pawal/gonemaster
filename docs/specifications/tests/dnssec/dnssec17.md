# DNSSEC17 (dnssec17)

Status: Final

## Purpose
- Validate CDNSKEY RRsets against DNSKEY data and CDNSKEY signatures, including delete semantics and signature/keytag consistency checks.

## Preconditions And Inputs
- Preconditions:
  - A `zone.Zone` object is available.
- Required inputs:
  - Child nameserver sets from `methods.Method4` and `methods.Method5`.
  - CDNSKEY and DNSKEY responses with DNSSEC enabled.
  - CDNSKEY and DNSKEY answer-section RRSIG records.
- Profile/config knobs that affect behavior:
  - `net.ipv4` and `net.ipv6`: disabled transports are skipped with transport debug tags.
  - `resolver.defaults.parallel`: parallel nameserver query and validation fanout.

## Algorithm And Decision Flow
1. Emit `TEST_CASE_START`.
2. Build child nameserver set from Method4+Method5 and deduplicate by IP.
3. For each unique nameserver IP (parallelized):
   - If transport is disabled, emit `IPV4_DISABLED` or `IPV6_DISABLED` for rrtypes `CDNSKEY` and `DNSKEY` and skip.
   - Query `CDNSKEY` with DNSSEC enabled; require authoritative `NOERROR` and at least one CDNSKEY record to continue.
   - Collect CDNSKEY records and answer-section RRSIG records.
   - Query `DNSKEY` with DNSSEC enabled; collect DNSKEY and answer-section RRSIG records when authoritative `NOERROR` with DNSKEY answers.
4. If no nameserver produced CDNSKEY records, emit no DS17 findings.
5. For each nameserver with CDNSKEY records (parallelized validation):
   - Detect delete semantics:
     - Mixed delete/non-delete CDNSKEY => `DS17_MIXED_DELETE_CDNSKEY`.
     - Only delete CDNSKEY => `DS17_DELETE_CDNSKEY`.
     - In both delete cases, remaining CDNSKEY validations are skipped for that nameserver.
   - If no DNSKEY RRset for nameserver => `DS17_CDNSKEY_WITHOUT_DNSKEY`.
   - For each non-delete CDNSKEY:
     - If zone bit unset => `DS17_CDNSKEY_IS_NON_ZONE` and skip remaining checks for this CDNSKEY.
     - If SEP bit unset => `DS17_CDNSKEY_IS_NON_SEP`.
     - If no matching DNSKEY keytag => `DS17_CDNSKEY_MATCHES_NO_DNSKEY`.
     - Else:
       - If DNSKEY RRSIG set lacks CDNSKEY keytag => `DS17_DNSKEY_NOT_SIGNED_BY_CDNSKEY`.
       - If CDNSKEY RRSIG set lacks CDNSKEY keytag => `DS17_CDNSKEY_NOT_SIGNED_BY_CDNSKEY`.
   - If CDNSKEY RRset has no RRSIG records => `DS17_CDNSKEY_UNSIGNED`.
   - Else for each CDNSKEY RRSIG:
     - If keytag has no matching DNSKEY => `DS17_CDNSKEY_SIGNED_BY_UNKNOWN_DNSKEY`.
     - Else if cryptographic verification fails for all matching DNSKEY records => `DS17_CDNSKEY_INVALID_RRSIG`.
6. Emit accumulated DS17 findings grouped by keytag or nameserver list as applicable.
7. Emit `TEST_CASE_END`.

## Emitted Tags (Possible Set)
| Tag | Emitted when |
| --- | --- |
| `DS17_CDNSKEY_INVALID_RRSIG` | CDNSKEY RRSIG keytag matches DNSKEY keytag(s), but signature verification fails. |
| `DS17_CDNSKEY_IS_NON_SEP` | CDNSKEY record has SEP bit unset. |
| `DS17_CDNSKEY_IS_NON_ZONE` | CDNSKEY record has zone bit unset. |
| `DS17_CDNSKEY_MATCHES_NO_DNSKEY` | CDNSKEY keytag matches no DNSKEY keytag. |
| `DS17_CDNSKEY_NOT_SIGNED_BY_CDNSKEY` | CDNSKEY RRset has no RRSIG with CDNSKEY keytag. |
| `DS17_CDNSKEY_SIGNED_BY_UNKNOWN_DNSKEY` | CDNSKEY RRset has RRSIG keytag not present in DNSKEY RRset. |
| `DS17_CDNSKEY_UNSIGNED` | CDNSKEY RRset has no RRSIG records. |
| `DS17_CDNSKEY_WITHOUT_DNSKEY` | CDNSKEY RRset exists but DNSKEY RRset is missing. |
| `DS17_DELETE_CDNSKEY` | Nameserver CDNSKEY RRset consists only of delete CDNSKEY record(s). |
| `DS17_DNSKEY_NOT_SIGNED_BY_CDNSKEY` | DNSKEY RRset has no RRSIG with CDNSKEY keytag. |
| `DS17_MIXED_DELETE_CDNSKEY` | Delete CDNSKEY record is mixed with non-delete CDNSKEY record(s). |
| `IPV4_DISABLED` | IPv4 transport is disabled for a queried nameserver (`CDNSKEY`, `DNSKEY`). |
| `IPV6_DISABLED` | IPv6 transport is disabled for a queried nameserver (`CDNSKEY`, `DNSKEY`). |
| `TEST_CASE_END` | Testcase completion marker is emitted. |
| `TEST_CASE_START` | Testcase start marker is emitted. |

## Tag Arguments
| Tag | Argument key | Type | Meaning |
| --- | --- | --- | --- |
| `DS17_CDNSKEY_INVALID_RRSIG` | `keytag` | `int` | RRSIG keytag with invalid signature. |
| `DS17_CDNSKEY_INVALID_RRSIG` | `ns_ip_list` | `string` | Semicolon-delimited child nameserver IP list. |
| `DS17_CDNSKEY_IS_NON_SEP` | `keytag` | `int` | CDNSKEY keytag with SEP bit unset. |
| `DS17_CDNSKEY_IS_NON_SEP` | `ns_ip_list` | `string` | Semicolon-delimited child nameserver IP list. |
| `DS17_CDNSKEY_IS_NON_ZONE` | `keytag` | `int` | CDNSKEY keytag with zone bit unset. |
| `DS17_CDNSKEY_IS_NON_ZONE` | `ns_ip_list` | `string` | Semicolon-delimited child nameserver IP list. |
| `DS17_CDNSKEY_MATCHES_NO_DNSKEY` | `keytag` | `int` | CDNSKEY keytag not found in DNSKEY RRset. |
| `DS17_CDNSKEY_MATCHES_NO_DNSKEY` | `ns_ip_list` | `string` | Semicolon-delimited child nameserver IP list. |
| `DS17_CDNSKEY_NOT_SIGNED_BY_CDNSKEY` | `keytag` | `int` | CDNSKEY keytag missing from CDNSKEY RRset RRSIG keytags. |
| `DS17_CDNSKEY_NOT_SIGNED_BY_CDNSKEY` | `ns_ip_list` | `string` | Semicolon-delimited child nameserver IP list. |
| `DS17_CDNSKEY_SIGNED_BY_UNKNOWN_DNSKEY` | `keytag` | `int` | CDNSKEY RRSIG keytag with no DNSKEY match. |
| `DS17_CDNSKEY_SIGNED_BY_UNKNOWN_DNSKEY` | `ns_ip_list` | `string` | Semicolon-delimited child nameserver IP list. |
| `DS17_CDNSKEY_UNSIGNED` | `ns_ip_list` | `string` | Semicolon-delimited child nameserver IP list. |
| `DS17_CDNSKEY_WITHOUT_DNSKEY` | `ns_ip_list` | `string` | Semicolon-delimited child nameserver IP list. |
| `DS17_DELETE_CDNSKEY` | `ns_ip_list` | `string` | Semicolon-delimited child nameserver IP list. |
| `DS17_DNSKEY_NOT_SIGNED_BY_CDNSKEY` | `keytag` | `int` | CDNSKEY keytag missing from DNSKEY RRset RRSIG keytags. |
| `DS17_DNSKEY_NOT_SIGNED_BY_CDNSKEY` | `ns_ip_list` | `string` | Semicolon-delimited child nameserver IP list. |
| `DS17_MIXED_DELETE_CDNSKEY` | `ns_ip_list` | `string` | Semicolon-delimited child nameserver IP list. |
| `IPV4_DISABLED` | `ns` | `string` | Nameserver identity (`name/ip`) skipped on IPv4. |
| `IPV4_DISABLED` | `rrtype` | `string` | rrtype skipped (`CDNSKEY` or `DNSKEY`). |
| `IPV6_DISABLED` | `ns` | `string` | Nameserver identity (`name/ip`) skipped on IPv6. |
| `IPV6_DISABLED` | `rrtype` | `string` | rrtype skipped (`CDNSKEY` or `DNSKEY`). |
| `TEST_CASE_END` | `testcase` | `string` | Testcase display name (`DNSSEC17`). |
| `TEST_CASE_START` | `testcase` | `string` | Testcase display name (`DNSSEC17`). |

## Severity Levels Per Tag
| Tag | Level | Notes |
| --- | --- | --- |
| `DS17_CDNSKEY_INVALID_RRSIG` | `ERROR` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS17_CDNSKEY_IS_NON_SEP` | `NOTICE` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS17_CDNSKEY_IS_NON_ZONE` | `ERROR` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS17_CDNSKEY_MATCHES_NO_DNSKEY` | `WARNING` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS17_CDNSKEY_NOT_SIGNED_BY_CDNSKEY` | `NOTICE` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS17_CDNSKEY_SIGNED_BY_UNKNOWN_DNSKEY` | `ERROR` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS17_CDNSKEY_UNSIGNED` | `ERROR` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS17_CDNSKEY_WITHOUT_DNSKEY` | `ERROR` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS17_DELETE_CDNSKEY` | `INFO` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS17_DNSKEY_NOT_SIGNED_BY_CDNSKEY` | `WARNING` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS17_MIXED_DELETE_CDNSKEY` | `ERROR` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `IPV4_DISABLED` | `DEBUG` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `IPV6_DISABLED` | `DEBUG` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `TEST_CASE_END` | `DEBUG` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `TEST_CASE_START` | `DEBUG` | Default from `share/profile.json` (`test_levels.DNSSEC`). |

## Differences From Upstream
- Upstream reference: [`dnssec17.md`](../../upstream/tests/DNSSEC-TP/dnssec17.md)
- Differences (Upstream vs Gonemaster):
  - Upstream: describes this testcase as producing no output when no CDNSKEY is found. Gonemaster: emits only `TEST_CASE_START`, `TEST_CASE_END`, and, if any transport is disabled, `IPV4_DISABLED` and/or `IPV6_DISABLED` in that case.
  - Upstream: signature checks are described at RRset/signature level. Gonemaster: the checks for `DS17_DNSKEY_NOT_SIGNED_BY_CDNSKEY` and `DS17_CDNSKEY_NOT_SIGNED_BY_CDNSKEY` are implemented as keytag-presence checks in RRSIG sets, not full per-signature validation.
  - Upstream: does not explicitly specify testcase boundary and transport-disabled debug emissions in this testcase summary. Gonemaster: emits `TEST_CASE_START`, `TEST_CASE_END`, `IPV4_DISABLED`, and `IPV6_DISABLED`.
- Potential upstream report:
  - `no`

## Edge Cases And Limitations
- Delete semantics short-circuit further CDNSKEY validation for that nameserver.
- Non-zone CDNSKEY records are flagged and then skipped from subsequent matching/signing checks for that record.
- Nameserver evaluation is deduplicated by IP.
