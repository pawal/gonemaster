# DNSSEC09 (dnssec09)

Status: Draft

## Purpose
- Verify that SOA responses are signed and that SOA RRSIG records are time-valid, algorithm-supported, and cryptographically matched by DNSKEY records from the same nameserver.

## Preconditions And Inputs
- Preconditions:
  - A `zone.Zone` object is available.
- Required inputs:
  - Child nameserver sets from `methods.Method4` and `methods.Method5`.
  - DNSKEY and SOA responses with DNSSEC enabled from child nameservers.
- Profile/config knobs that affect behavior:
  - `net.ipv4` and `net.ipv6`: disabled transports are skipped with transport debug tags.
  - `resolver.defaults.parallel`: parallel nameserver execution fanout.

## Algorithm And Decision Flow
1. Emit `TEST_CASE_START`.
2. Build nameserver set from Method4+Method5 and deduplicate by IP.
3. For each unique nameserver IP (parallelized):
   - If transport is disabled, emit `IPV4_DISABLED` or `IPV6_DISABLED` for rrtype `DNSKEY` and skip.
   - Query `DNSKEY` at child apex with DNSSEC enabled.
   - Skip nameserver if response is absent, non-`NOERROR`, non-`AA`, or has no apex DNSKEY records.
   - Query `SOA` at child apex with DNSSEC enabled over UDP (`UseVC=false`).
   - Skip nameserver if response is absent, non-`NOERROR`, non-`AA`, or has no apex SOA records.
   - If answer section has no `RRSIG`, mark nameserver for `DS09_MISSING_RRSIG_IN_RESPONSE`.
   - Else, for each answer-section RRSIG:
     - If inception is after test time, mark `DS09_SOA_RRSIG_NOT_YET_VALID` by keytag.
     - Else if expiration is before test time, mark `DS09_SOA_RRSIG_EXPIRED` by keytag.
     - Else if algorithm is unsupported, mark `DS09_ALGO_NOT_SUPPORTED_BY_ZM` by keytag+algo.
     - Else find matching DNSKEY records by keytag:
       - If none found, mark `DS09_NO_MATCHING_DNSKEY`.
       - Else verify signature over SOA RRset; if no candidate validates, mark `DS09_RRSIG_NOT_VALID_BY_DNSKEY`.
4. Emit accumulated DS09 findings grouped by keytag/algo as applicable.
5. Emit `TEST_CASE_END`.

## Emitted Tags (Possible Set)
| Tag | Emitted when |
| --- | --- |
| `DS09_ALGO_NOT_SUPPORTED_BY_ZM` | RRSIG verification requires an unsupported algorithm. |
| `DS09_MISSING_RRSIG_IN_RESPONSE` | SOA response has no answer-section RRSIG records. |
| `DS09_NO_MATCHING_DNSKEY` | RRSIG keytag has no matching DNSKEY keytag. |
| `DS09_RRSIG_NOT_VALID_BY_DNSKEY` | Matching DNSKEY candidates exist but none verify the SOA signature. |
| `DS09_SOA_RRSIG_EXPIRED` | SOA-related RRSIG expiration is before test time. |
| `DS09_SOA_RRSIG_NOT_YET_VALID` | SOA-related RRSIG inception is after test time. |
| `IPV4_DISABLED` | IPv4 transport is disabled for a queried nameserver (`DNSKEY`). |
| `IPV6_DISABLED` | IPv6 transport is disabled for a queried nameserver (`DNSKEY`). |
| `TEST_CASE_END` | Testcase completion marker is emitted. |
| `TEST_CASE_START` | Testcase start marker is emitted. |

## Tag Arguments
| Tag | Argument key | Type | Meaning |
| --- | --- | --- | --- |
| `DS09_ALGO_NOT_SUPPORTED_BY_ZM` | `keytag` | `int` | RRSIG keytag associated with unsupported algorithm. |
| `DS09_ALGO_NOT_SUPPORTED_BY_ZM` | `algo_num` | `int` | Unsupported DNSSEC algorithm number. |
| `DS09_ALGO_NOT_SUPPORTED_BY_ZM` | `algo_mnemo` | `string` | Unsupported DNSSEC algorithm mnemonic. |
| `DS09_ALGO_NOT_SUPPORTED_BY_ZM` | `ns_ip_list` | `string` | Semicolon-delimited child nameserver IP list. |
| `DS09_MISSING_RRSIG_IN_RESPONSE` | `ns_ip_list` | `string` | Semicolon-delimited child nameserver IP list lacking SOA RRSIG. |
| `DS09_NO_MATCHING_DNSKEY` | `keytag` | `int` | RRSIG keytag with no matching DNSKEY keytag. |
| `DS09_NO_MATCHING_DNSKEY` | `ns_ip_list` | `string` | Semicolon-delimited child nameserver IP list. |
| `DS09_RRSIG_NOT_VALID_BY_DNSKEY` | `keytag` | `int` | RRSIG keytag that failed DNSKEY verification. |
| `DS09_RRSIG_NOT_VALID_BY_DNSKEY` | `ns_ip_list` | `string` | Semicolon-delimited child nameserver IP list. |
| `DS09_SOA_RRSIG_EXPIRED` | `keytag` | `int` | RRSIG keytag with expired validity window. |
| `DS09_SOA_RRSIG_EXPIRED` | `ns_ip_list` | `string` | Semicolon-delimited child nameserver IP list. |
| `DS09_SOA_RRSIG_NOT_YET_VALID` | `keytag` | `int` | RRSIG keytag with not-yet-valid validity window. |
| `DS09_SOA_RRSIG_NOT_YET_VALID` | `ns_ip_list` | `string` | Semicolon-delimited child nameserver IP list. |
| `IPV4_DISABLED` | `ns` | `string` | Nameserver identity (`name/ip`) skipped on IPv4. |
| `IPV4_DISABLED` | `rrtype` | `string` | rrtype skipped (`DNSKEY`). |
| `IPV6_DISABLED` | `ns` | `string` | Nameserver identity (`name/ip`) skipped on IPv6. |
| `IPV6_DISABLED` | `rrtype` | `string` | rrtype skipped (`DNSKEY`). |
| `TEST_CASE_END` | `testcase` | `string` | Testcase display name (`DNSSEC09`). |
| `TEST_CASE_START` | `testcase` | `string` | Testcase display name (`DNSSEC09`). |

## Severity Levels Per Tag
| Tag | Level | Notes |
| --- | --- | --- |
| `DS09_ALGO_NOT_SUPPORTED_BY_ZM` | `NOTICE` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS09_MISSING_RRSIG_IN_RESPONSE` | `ERROR` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS09_NO_MATCHING_DNSKEY` | `ERROR` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS09_RRSIG_NOT_VALID_BY_DNSKEY` | `ERROR` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS09_SOA_RRSIG_EXPIRED` | `ERROR` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS09_SOA_RRSIG_NOT_YET_VALID` | `ERROR` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `IPV4_DISABLED` | `DEBUG` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `IPV6_DISABLED` | `DEBUG` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `TEST_CASE_END` | `DEBUG` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `TEST_CASE_START` | `DEBUG` | Default from `share/profile.json` (`test_levels.DNSSEC`). |

## Differences From Upstream
- Upstream reference: [`dnssec09.md`](../../upstream/tests/DNSSEC-TP/dnssec09.md)
- Differences (Upstream vs Gonemaster):
  - Upstream: procedure text describes evaluating SOA RRSIG records. Gonemaster: iterates all answer-section RRSIG records and does not explicitly filter by `TypeCovered == SOA` before DS09 classification.
  - Upstream: describes validity checks against test execution time. Gonemaster: uses the DNSKEY response packet timestamp (`packetTime(dnskeyResp)`) as the reference time.
  - Upstream: does not explicitly specify testcase boundary and per-query transport debug emissions in this testcase summary. Gonemaster: emits `TEST_CASE_START`, `TEST_CASE_END`, `IPV4_DISABLED`, and `IPV6_DISABLED`.
- Potential upstream report:
  - `no`

## Edge Cases And Limitations
- Nameserver evaluation is deduplicated by IP; repeated names on one IP share one DS09 outcome.
- Nameservers failing response-shape checks (`Msg`, `NOERROR`, `AA`, apex owner match) are skipped for DS09 findings.
- If no usable DNSKEY records are found for a nameserver, SOA signing checks are skipped for that nameserver.

---

Copyright (c) Patrik Wallström  
Copyright (c) The Swedish Internet Foundation (https://internetstiftelsen.se/en/)  
Copyright (c) AFNIC (https://www.afnic.fr/en/)  
All rights reserved.  

Copyright belongs to external contributor where applicable.  

Creative Commons Attribution 4.0 International License applies. See https://creativecommons.org/licenses/by/4.0/ for the license conditions.
