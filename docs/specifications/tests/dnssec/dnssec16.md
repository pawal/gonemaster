# DNSSEC16 (dnssec16)

Status: Draft

## Purpose
- Validate CDS RRsets against DNSKEY data and CDS signatures, including delete semantics and signature/keytag consistency checks.

## Preconditions And Inputs
- Preconditions:
  - A `zone.Zone` object is available.
- Required inputs:
  - Child nameserver sets from `methods.Method4` and `methods.Method5`.
  - CDS and DNSKEY responses with DNSSEC enabled.
  - CDS and DNSKEY answer-section RRSIG records.
- Profile/config knobs that affect behavior:
  - `net.ipv4` and `net.ipv6`: disabled transports are skipped with transport debug tags.
  - `resolver.defaults.parallel`: parallel nameserver query and validation fanout.

## Algorithm And Decision Flow
1. Emit `TEST_CASE_START`.
2. Build child nameserver set from Method4+Method5 and deduplicate by IP.
3. For each unique nameserver IP (parallelized):
   - If transport is disabled, emit `IPV4_DISABLED` or `IPV6_DISABLED` for rrtypes `CDS` and `DNSKEY` and skip.
   - Query `CDS` with DNSSEC enabled; require authoritative `NOERROR` and at least one CDS record to continue.
   - Collect CDS records and answer-section RRSIG records.
   - Query `DNSKEY` with DNSSEC enabled; collect DNSKEY and answer-section RRSIG records when authoritative `NOERROR` with DNSKEY answers.
4. If no nameserver produced CDS records, emit no DS16 findings.
5. For each nameserver with CDS records (parallelized validation):
   - Detect delete semantics:
     - Mixed delete/non-delete CDS => `DS16_MIXED_DELETE_CDS`.
     - Only delete CDS => `DS16_DELETE_CDS`.
     - In both delete cases, remaining CDS validations are skipped for that nameserver.
   - If no DNSKEY RRset for nameserver => `DS16_CDS_WITHOUT_DNSKEY`.
   - For each non-delete CDS:
     - If no matching DNSKEY keytag => `DS16_CDS_MATCHES_NO_DNSKEY`.
     - Else if any matching DNSKEY has zone bit unset => `DS16_CDS_MATCHES_NON_ZONE_DNSKEY`.
     - Else:
       - If DNSKEY RRSIG set lacks CDS keytag => `DS16_DNSKEY_NOT_SIGNED_BY_CDS`.
       - If CDS RRSIG set lacks CDS keytag => `DS16_CDS_NOT_SIGNED_BY_CDS`.
       - If any matching DNSKEY has SEP bit unset => `DS16_CDS_MATCHES_NON_SEP_DNSKEY`.
   - If CDS RRset has no RRSIG records => `DS16_CDS_UNSIGNED`.
   - Else for each CDS RRSIG:
     - If keytag has no matching DNSKEY => `DS16_CDS_SIGNED_BY_UNKNOWN_DNSKEY`.
     - Else if cryptographic verification fails for all matching DNSKEY records => `DS16_CDS_INVALID_RRSIG`.
6. Emit accumulated DS16 findings grouped by keytag or nameserver list as applicable.
7. Emit `TEST_CASE_END`.

## Emitted Tags (Possible Set)
| Tag | Emitted when |
| --- | --- |
| `DS16_CDS_INVALID_RRSIG` | CDS RRSIG keytag matches DNSKEY keytag(s), but signature verification fails. |
| `DS16_CDS_MATCHES_NON_SEP_DNSKEY` | CDS points to a DNSKEY with SEP bit unset. |
| `DS16_CDS_MATCHES_NON_ZONE_DNSKEY` | CDS points to a DNSKEY with zone bit unset. |
| `DS16_CDS_MATCHES_NO_DNSKEY` | CDS keytag matches no DNSKEY keytag. |
| `DS16_CDS_NOT_SIGNED_BY_CDS` | CDS RRset has no RRSIG with CDS keytag. |
| `DS16_CDS_SIGNED_BY_UNKNOWN_DNSKEY` | CDS RRset has RRSIG keytag not present in DNSKEY RRset. |
| `DS16_CDS_UNSIGNED` | CDS RRset has no RRSIG records. |
| `DS16_CDS_WITHOUT_DNSKEY` | CDS RRset exists but DNSKEY RRset is missing. |
| `DS16_DELETE_CDS` | Nameserver CDS RRset consists only of delete CDS record(s). |
| `DS16_DNSKEY_NOT_SIGNED_BY_CDS` | DNSKEY RRset has no RRSIG with CDS keytag. |
| `DS16_MIXED_DELETE_CDS` | Delete CDS record is mixed with non-delete CDS record(s). |
| `IPV4_DISABLED` | IPv4 transport is disabled for a queried nameserver (`CDS`, `DNSKEY`). |
| `IPV6_DISABLED` | IPv6 transport is disabled for a queried nameserver (`CDS`, `DNSKEY`). |
| `TEST_CASE_END` | Testcase completion marker is emitted. |
| `TEST_CASE_START` | Testcase start marker is emitted. |

## Tag Arguments
| Tag | Argument key | Type | Meaning |
| --- | --- | --- | --- |
| `DS16_CDS_INVALID_RRSIG` | `keytag` | `int` | RRSIG keytag with invalid signature. |
| `DS16_CDS_INVALID_RRSIG` | `ns_ip_list` | `string` | Semicolon-delimited child nameserver IP list. |
| `DS16_CDS_MATCHES_NON_SEP_DNSKEY` | `keytag` | `int` | CDS keytag referencing non-SEP DNSKEY. |
| `DS16_CDS_MATCHES_NON_SEP_DNSKEY` | `ns_ip_list` | `string` | Semicolon-delimited child nameserver IP list. |
| `DS16_CDS_MATCHES_NON_ZONE_DNSKEY` | `keytag` | `int` | CDS keytag referencing non-zone DNSKEY. |
| `DS16_CDS_MATCHES_NON_ZONE_DNSKEY` | `ns_ip_list` | `string` | Semicolon-delimited child nameserver IP list. |
| `DS16_CDS_MATCHES_NO_DNSKEY` | `keytag` | `int` | CDS keytag not found in DNSKEY RRset. |
| `DS16_CDS_MATCHES_NO_DNSKEY` | `ns_ip_list` | `string` | Semicolon-delimited child nameserver IP list. |
| `DS16_CDS_NOT_SIGNED_BY_CDS` | `keytag` | `int` | CDS keytag missing from CDS RRset RRSIG keytags. |
| `DS16_CDS_NOT_SIGNED_BY_CDS` | `ns_ip_list` | `string` | Semicolon-delimited child nameserver IP list. |
| `DS16_CDS_SIGNED_BY_UNKNOWN_DNSKEY` | `keytag` | `int` | CDS RRSIG keytag with no DNSKEY match. |
| `DS16_CDS_SIGNED_BY_UNKNOWN_DNSKEY` | `ns_ip_list` | `string` | Semicolon-delimited child nameserver IP list. |
| `DS16_CDS_UNSIGNED` | `ns_ip_list` | `string` | Semicolon-delimited child nameserver IP list. |
| `DS16_CDS_WITHOUT_DNSKEY` | `ns_ip_list` | `string` | Semicolon-delimited child nameserver IP list. |
| `DS16_DELETE_CDS` | `ns_ip_list` | `string` | Semicolon-delimited child nameserver IP list. |
| `DS16_DNSKEY_NOT_SIGNED_BY_CDS` | `keytag` | `int` | CDS keytag missing from DNSKEY RRset RRSIG keytags. |
| `DS16_DNSKEY_NOT_SIGNED_BY_CDS` | `ns_ip_list` | `string` | Semicolon-delimited child nameserver IP list. |
| `DS16_MIXED_DELETE_CDS` | `ns_ip_list` | `string` | Semicolon-delimited child nameserver IP list. |
| `IPV4_DISABLED` | `ns` | `string` | Nameserver identity (`name/ip`) skipped on IPv4. |
| `IPV4_DISABLED` | `rrtype` | `string` | rrtype skipped (`CDS` or `DNSKEY`). |
| `IPV6_DISABLED` | `ns` | `string` | Nameserver identity (`name/ip`) skipped on IPv6. |
| `IPV6_DISABLED` | `rrtype` | `string` | rrtype skipped (`CDS` or `DNSKEY`). |
| `TEST_CASE_END` | `testcase` | `string` | Testcase display name (`DNSSEC16`). |
| `TEST_CASE_START` | `testcase` | `string` | Testcase display name (`DNSSEC16`). |

## Severity Levels Per Tag
| Tag | Level | Notes |
| --- | --- | --- |
| `DS16_CDS_INVALID_RRSIG` | `ERROR` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS16_CDS_MATCHES_NON_SEP_DNSKEY` | `NOTICE` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS16_CDS_MATCHES_NON_ZONE_DNSKEY` | `ERROR` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS16_CDS_MATCHES_NO_DNSKEY` | `WARNING` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS16_CDS_NOT_SIGNED_BY_CDS` | `NOTICE` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS16_CDS_SIGNED_BY_UNKNOWN_DNSKEY` | `ERROR` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS16_CDS_UNSIGNED` | `ERROR` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS16_CDS_WITHOUT_DNSKEY` | `ERROR` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS16_DELETE_CDS` | `INFO` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS16_DNSKEY_NOT_SIGNED_BY_CDS` | `WARNING` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS16_MIXED_DELETE_CDS` | `ERROR` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `IPV4_DISABLED` | `DEBUG` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `IPV6_DISABLED` | `DEBUG` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `TEST_CASE_END` | `DEBUG` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `TEST_CASE_START` | `DEBUG` | Default from `share/profile.json` (`test_levels.DNSSEC`). |

## Differences From Upstream
- Upstream reference: [`dnssec16.md`](../../upstream/tests/DNSSEC-TP/dnssec16.md)
- Differences (Upstream vs Gonemaster):
  - Upstream: describes this testcase as producing no output when no CDS is found. Gonemaster: emits only `TEST_CASE_START`, `TEST_CASE_END`, and, if any transport is disabled, `IPV4_DISABLED` and/or `IPV6_DISABLED` in that case.
  - Upstream: signature checks are described at RRset/signature level. Gonemaster: the checks for `DS16_DNSKEY_NOT_SIGNED_BY_CDS` and `DS16_CDS_NOT_SIGNED_BY_CDS` are implemented as keytag-presence checks in RRSIG sets, not full per-signature validation.
  - Upstream: does not explicitly specify testcase boundary and transport-disabled debug emissions in this testcase summary. Gonemaster: emits `TEST_CASE_START`, `TEST_CASE_END`, `IPV4_DISABLED`, and `IPV6_DISABLED`.
- Potential upstream report:
  - `no`

## Edge Cases And Limitations
- Delete semantics short-circuit further CDS validation for that nameserver.
- `DS16_CDS_MATCHES_NON_ZONE_DNSKEY` is emitted if any matching-keytag DNSKEY has zone bit unset.
- Nameserver evaluation is deduplicated by IP.

---

Copyright (c) Patrik Wallström  
Copyright (c) The Swedish Internet Foundation (https://internetstiftelsen.se/en/)  
Copyright (c) AFNIC (https://www.afnic.fr/en/)  
All rights reserved.  

Copyright belongs to external contributor where applicable.  

Creative Commons Attribution 4.0 International License applies. See https://creativecommons.org/licenses/by/4.0/ for the license conditions.
