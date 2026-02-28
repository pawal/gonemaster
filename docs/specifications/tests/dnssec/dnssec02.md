# DNSSEC02 (dnssec02)

Status: Final

## Purpose
- Verify that DS records found at the parent delegation match usable DNSKEYs in the child zone and that matching DNSKEYs can validate DNSKEY RRset signatures.

## Preconditions And Inputs
- Preconditions:
  - A `zone.Zone` object is available.
- Required inputs:
  - Parent nameserver names/IPs from `methodsv2.GetParentNSNamesAndIPs`.
  - Child nameserver sets from `methods.Method4` and `methods.Method5`.
  - Parent DS responses and child DNSKEY/RRSIG responses.
- Profile/config knobs that affect behavior:
  - `net.ipv4` and `net.ipv6`: disabled transports are skipped with transport debug tags.
  - `resolver.defaults.parallel`: parallel parent and child query execution fanout.

## Algorithm And Decision Flow
1. Emit `TEST_CASE_START`.
2. Collect DS records from parent nameservers:
   - Query each unique parent nameserver IP in parallel.
   - If transport is disabled, emit `IPV4_DISABLED` or `IPV6_DISABLED` for rrtype `DS` and skip.
   - Accept response only if DNSSEC response shape passes (`NOERROR`, `OPT`, `DO`, `AA`) and at least one DS record matches child zone owner name.
   - Add unique DS RDATA values to DS record set.
3. If DS record set is empty, emit `TEST_CASE_END` and stop.
4. Build child nameserver set from Method4+Method5 and deduplicate by IP.
5. For each unique child nameserver IP (parallelized):
   - If transport is disabled, emit `IPV4_DISABLED` or `IPV6_DISABLED` for rrtype `DNSKEY` and skip.
   - Query DNSKEY with DNSSEC enabled.
   - Require `NOERROR`, `OPT`, `DO`, `AA`, and at least one DNSKEY at child apex; otherwise skip.
   - Mark nameserver as responding.
   - For each DS record:
     - Find DNSKEY candidates by keytag and select a matching candidate (digest-checked when digest type is supported).
     - If no DNSKEY by keytag exists, add keytag/ns to `DS02_NO_DNSKEY_FOR_DS`.
     - If DNSKEY exists but DS digest check fails, add keytag/ns to `DS02_NO_MATCH_DS_DNSKEY`.
     - If DNSKEY has no ZONE flag, add keytag/ns to `DS02_DNSKEY_NOT_FOR_ZONE_SIGNING` and stop processing that DS.
     - If DNSKEY has no SEP flag, add keytag/ns to `DS02_DNSKEY_NOT_SEP`.
     - Track DNSKEY as DS-matching candidate for signature checks.
   - For each DS-matching DNSKEY:
     - Find DNSKEY-covering RRSIG by keytag and verify against DNSKEY.
     - Missing/invalid match contributes `DS02_NO_MATCHING_DNSKEY_RRSIG`.
     - Unsupported verification algorithm contributes `DS02_ALGO_NOT_SUPPORTED_BY_ZM`.
     - Signature verification failure contributes `DS02_RRSIG_NOT_VALID_BY_DNSKEY`.
     - Successful verification marks nameserver as having an RRSIG match for DS.
6. Emit accumulated per-keytag findings (`DS02_*`) with merged `ns_ip_list`.
7. Emit per-nameserver summary:
   - `DS02_NO_VALID_DNSKEY_FOR_ANY_DS` for responding nameservers with no DS-matching DNSKEY.
   - Else `DS02_DNSKEY_NOT_SIGNED_BY_ANY_DS` for responding nameservers with DS-matching DNSKEY but no validating RRSIG from those keys.
8. Emit `TEST_CASE_END`.

## Emitted Tags (Possible Set)
| Tag | Emitted when |
| --- | --- |
| `DS02_ALGO_NOT_SUPPORTED_BY_ZM` | DNSKEY RRSIG verification requires an unsupported algorithm for this build/runtime. |
| `DS02_DNSKEY_NOT_FOR_ZONE_SIGNING` | DS-matching DNSKEY is found but lacks ZONE flag. |
| `DS02_DNSKEY_NOT_SEP` | DS-matching DNSKEY is found but lacks SEP flag. |
| `DS02_DNSKEY_NOT_SIGNED_BY_ANY_DS` | Nameserver has DS-matching DNSKEY(s), but no validating DNSKEY RRSIG from those keys. |
| `DS02_NO_DNSKEY_FOR_DS` | No DNSKEY with matching keytag exists for DS record. |
| `DS02_NO_MATCHING_DNSKEY_RRSIG` | No valid DNSKEY-covering RRSIG could be matched to a DS-matching DNSKEY. |
| `DS02_NO_MATCH_DS_DNSKEY` | DNSKEY keytag match exists but DS digest/algorithm does not match DNSKEY data. |
| `DS02_NO_VALID_DNSKEY_FOR_ANY_DS` | Responding child nameserver has no valid DS-matching DNSKEY for any DS. |
| `DS02_RRSIG_NOT_VALID_BY_DNSKEY` | Candidate DNSKEY RRSIG was present but failed verification with matching DNSKEY. |
| `IPV4_DISABLED` | IPv4 transport is disabled for a queried nameserver (`DS` or `DNSKEY`). |
| `IPV6_DISABLED` | IPv6 transport is disabled for a queried nameserver (`DS` or `DNSKEY`). |
| `TEST_CASE_END` | Testcase completion marker is emitted. |
| `TEST_CASE_START` | Testcase start marker is emitted. |

## Tag Arguments
| Tag | Argument key | Type | Meaning |
| --- | --- | --- | --- |
| `DS02_ALGO_NOT_SUPPORTED_BY_ZM` | `keytag` | `int` | DNSKEY keytag associated with unsupported signature algorithm. |
| `DS02_ALGO_NOT_SUPPORTED_BY_ZM` | `algo_num` | `int` | DNSSEC algorithm number that verification cannot process. |
| `DS02_ALGO_NOT_SUPPORTED_BY_ZM` | `algo_mnemo` | `string` | DNSSEC algorithm mnemonic string. |
| `DS02_ALGO_NOT_SUPPORTED_BY_ZM` | `ns_ip_list` | `string` | Semicolon-delimited child nameserver IP list. |
| `DS02_DNSKEY_NOT_FOR_ZONE_SIGNING` | `keytag` | `int` | DS/DNSKEY keytag lacking ZONE bit. |
| `DS02_DNSKEY_NOT_FOR_ZONE_SIGNING` | `ns_ip_list` | `string` | Semicolon-delimited child nameserver IP list. |
| `DS02_DNSKEY_NOT_SEP` | `keytag` | `int` | DS/DNSKEY keytag lacking SEP bit. |
| `DS02_DNSKEY_NOT_SEP` | `ns_ip_list` | `string` | Semicolon-delimited child nameserver IP list. |
| `DS02_DNSKEY_NOT_SIGNED_BY_ANY_DS` | `ns_ip_list` | `string` | Semicolon-delimited child nameserver IP list. |
| `DS02_NO_DNSKEY_FOR_DS` | `keytag` | `int` | DS keytag for which no DNSKEY was found. |
| `DS02_NO_DNSKEY_FOR_DS` | `ns_ip_list` | `string` | Semicolon-delimited child nameserver IP list. |
| `DS02_NO_MATCHING_DNSKEY_RRSIG` | `keytag` | `int` | DS-matching DNSKEY keytag lacking a validating DNSKEY RRSIG. |
| `DS02_NO_MATCHING_DNSKEY_RRSIG` | `ns_ip_list` | `string` | Semicolon-delimited child nameserver IP list. |
| `DS02_NO_MATCH_DS_DNSKEY` | `keytag` | `int` | DS keytag whose DS digest/algorithm did not match DNSKEY data. |
| `DS02_NO_MATCH_DS_DNSKEY` | `ns_ip_list` | `string` | Semicolon-delimited child nameserver IP list. |
| `DS02_NO_VALID_DNSKEY_FOR_ANY_DS` | `ns_ip_list` | `string` | Semicolon-delimited child nameserver IP list. |
| `DS02_RRSIG_NOT_VALID_BY_DNSKEY` | `keytag` | `int` | Keytag from DNSKEY RRSIG verification failure. |
| `DS02_RRSIG_NOT_VALID_BY_DNSKEY` | `ns_ip_list` | `string` | Semicolon-delimited child nameserver IP list. |
| `IPV4_DISABLED` | `ns` | `string` | Nameserver identity (`name/ip`) skipped on IPv4. |
| `IPV4_DISABLED` | `rrtype` | `string` | rrtype skipped (`DS` or `DNSKEY`). |
| `IPV6_DISABLED` | `ns` | `string` | Nameserver identity (`name/ip`) skipped on IPv6. |
| `IPV6_DISABLED` | `rrtype` | `string` | rrtype skipped (`DS` or `DNSKEY`). |
| `TEST_CASE_END` | `testcase` | `string` | Testcase display name (`DNSSEC02`). |
| `TEST_CASE_START` | `testcase` | `string` | Testcase display name (`DNSSEC02`). |

## Severity Levels Per Tag
| Tag | Level | Notes |
| --- | --- | --- |
| `DS02_ALGO_NOT_SUPPORTED_BY_ZM` | `NOTICE` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS02_DNSKEY_NOT_FOR_ZONE_SIGNING` | `ERROR` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS02_DNSKEY_NOT_SEP` | `NOTICE` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS02_DNSKEY_NOT_SIGNED_BY_ANY_DS` | `ERROR` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS02_NO_DNSKEY_FOR_DS` | `WARNING` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS02_NO_MATCHING_DNSKEY_RRSIG` | `WARNING` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS02_NO_MATCH_DS_DNSKEY` | `ERROR` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS02_NO_VALID_DNSKEY_FOR_ANY_DS` | `ERROR` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS02_RRSIG_NOT_VALID_BY_DNSKEY` | `ERROR` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `IPV4_DISABLED` | `DEBUG` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `IPV6_DISABLED` | `DEBUG` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `TEST_CASE_END` | `DEBUG` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `TEST_CASE_START` | `DEBUG` | Default from `share/profile.json` (`test_levels.DNSSEC`). |

## Differences From Upstream
- Upstream reference: [`dnssec02.md`](../../upstream/tests/DNSSEC-TP/dnssec02.md)
- Differences (Upstream vs Gonemaster):
  - Upstream: explicitly describes a dedicated undelegated DS input branch in testcase flow. Gonemaster: `DNSSEC02` implementation uses parent DS discovery path directly and has no separate testcase-local undelegated DS branch.
  - Upstream: does not explicitly specify testcase boundary and per-query transport debug emissions in this testcase summary. Gonemaster: emits `TEST_CASE_START`, `TEST_CASE_END`, `IPV4_DISABLED`, and `IPV6_DISABLED`.
- Potential upstream report:
  - `no`

## Edge Cases And Limitations
- If parent DS discovery yields no DS records, testcase stops after boundary tags and emits no DS02 findings.
- Child nameservers are deduplicated by IP before DNSKEY checks, so repeated names on one IP collapse into one probe context.
- `DS02_NO_VALID_DNSKEY_FOR_ANY_DS` and `DS02_DNSKEY_NOT_SIGNED_BY_ANY_DS` are mutually exclusive by implementation (`else if` branch).
