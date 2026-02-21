# DNSSEC13 (dnssec13)

Status: Draft

## Purpose
- Verify that each DNSKEY algorithm observed in the DNSKEY RRset also appears in RRSIG records for DNSKEY, SOA, and NS answer flows at child nameservers.

## Preconditions And Inputs
- Preconditions:
  - A `zone.Zone` object is available.
- Required inputs:
  - Child nameserver sets from `methods.Method4` and `methods.Method5`.
  - DNSKEY, SOA, and NS query responses with DNSSEC enabled from child nameservers.
- Profile/config knobs that affect behavior:
  - `net.ipv4` and `net.ipv6`: disabled transports are skipped with transport debug tags.
  - `resolver.defaults.parallel`: parallel nameserver execution fanout.

## Algorithm And Decision Flow
1. Emit `TEST_CASE_START`.
2. Build nameserver set from Method4+Method5, deduplicate by IP.
3. For each unique nameserver IP (parallelized):
   - If transport is disabled, emit `IPV4_DISABLED` or `IPV6_DISABLED` for rrtypes `DNSKEY`, `SOA`, and `NS`, then skip.
   - Query `DNSKEY`, `SOA`, and `NS` at child apex with DNSSEC enabled.
   - For each query type:
     - Require response message, `RCODE=NOERROR`, `AA=true`, at least one answer record of the queried type, and at least one answer-section RRSIG; otherwise skip this query type.
     - On the `DNSKEY` query, collect DNSKEY algorithms from returned DNSKEY records.
     - For each collected DNSKEY algorithm, check whether any answer-section RRSIG has that algorithm.
     - If no matching RRSIG algorithm is found, record a missing-signature-algorithm finding for this query type.
4. Emit `DS13_ALGO_NOT_SIGNED_DNSKEY`, `DS13_ALGO_NOT_SIGNED_SOA`, and/or `DS13_ALGO_NOT_SIGNED_NS` grouped by algorithm with merged nameserver IP lists.
5. Emit `TEST_CASE_END`.

## Emitted Tags (Possible Set)
| Tag | Emitted when |
| --- | --- |
| `DS13_ALGO_NOT_SIGNED_DNSKEY` | DNSKEY RRset answer lacks RRSIG algorithm coverage for at least one DNSKEY algorithm. |
| `DS13_ALGO_NOT_SIGNED_NS` | NS RRset answer lacks RRSIG algorithm coverage for at least one DNSKEY algorithm. |
| `DS13_ALGO_NOT_SIGNED_SOA` | SOA RRset answer lacks RRSIG algorithm coverage for at least one DNSKEY algorithm. |
| `IPV4_DISABLED` | IPv4 transport is disabled for a queried nameserver/rrtype. |
| `IPV6_DISABLED` | IPv6 transport is disabled for a queried nameserver/rrtype. |
| `TEST_CASE_END` | Testcase completion marker is emitted. |
| `TEST_CASE_START` | Testcase start marker is emitted. |

## Tag Arguments
| Tag | Argument key | Type | Meaning |
| --- | --- | --- | --- |
| `DS13_ALGO_NOT_SIGNED_DNSKEY` | `ns_ip_list` | `string` | Semicolon-delimited child nameserver IP list. |
| `DS13_ALGO_NOT_SIGNED_DNSKEY` | `algo_num` | `int` | DNSKEY algorithm number missing in RRSIG coverage. |
| `DS13_ALGO_NOT_SIGNED_DNSKEY` | `algo_mnemo` | `string` | DNSKEY algorithm mnemonic missing in RRSIG coverage. |
| `DS13_ALGO_NOT_SIGNED_NS` | `ns_ip_list` | `string` | Semicolon-delimited child nameserver IP list. |
| `DS13_ALGO_NOT_SIGNED_NS` | `algo_num` | `int` | DNSKEY algorithm number missing in RRSIG coverage. |
| `DS13_ALGO_NOT_SIGNED_NS` | `algo_mnemo` | `string` | DNSKEY algorithm mnemonic missing in RRSIG coverage. |
| `DS13_ALGO_NOT_SIGNED_SOA` | `ns_ip_list` | `string` | Semicolon-delimited child nameserver IP list. |
| `DS13_ALGO_NOT_SIGNED_SOA` | `algo_num` | `int` | DNSKEY algorithm number missing in RRSIG coverage. |
| `DS13_ALGO_NOT_SIGNED_SOA` | `algo_mnemo` | `string` | DNSKEY algorithm mnemonic missing in RRSIG coverage. |
| `IPV4_DISABLED` | `ns` | `string` | Nameserver identity (`name/ip`) skipped on IPv4. |
| `IPV4_DISABLED` | `rrtype` | `string` | rrtype skipped (`DNSKEY`, `SOA`, or `NS`). |
| `IPV6_DISABLED` | `ns` | `string` | Nameserver identity (`name/ip`) skipped on IPv6. |
| `IPV6_DISABLED` | `rrtype` | `string` | rrtype skipped (`DNSKEY`, `SOA`, or `NS`). |
| `TEST_CASE_END` | `testcase` | `string` | Testcase display name (`DNSSEC13`). |
| `TEST_CASE_START` | `testcase` | `string` | Testcase display name (`DNSSEC13`). |

## Severity Levels Per Tag
| Tag | Level | Notes |
| --- | --- | --- |
| `DS13_ALGO_NOT_SIGNED_DNSKEY` | `WARNING` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS13_ALGO_NOT_SIGNED_NS` | `WARNING` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS13_ALGO_NOT_SIGNED_SOA` | `WARNING` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `IPV4_DISABLED` | `DEBUG` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `IPV6_DISABLED` | `DEBUG` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `TEST_CASE_END` | `DEBUG` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `TEST_CASE_START` | `DEBUG` | Default from `share/profile.json` (`test_levels.DNSSEC`). |

## Differences From Upstream
- Upstream reference: [`dnssec13.md`](../../upstream/tests/DNSSEC-TP/dnssec13.md)
- Differences (Upstream vs Gonemaster):
  - Upstream: describes checking RRSIG records for each queried RRset. Gonemaster: for each query type, checks answer-section RRSIG algorithms without explicitly filtering on `TypeCovered` for that query type.
  - Upstream: does not explicitly specify testcase boundary and per-query transport debug emissions in this testcase summary. Gonemaster: emits `TEST_CASE_START`, `TEST_CASE_END`, `IPV4_DISABLED`, and `IPV6_DISABLED`.
- Potential upstream report:
  - `no`

## Edge Cases And Limitations
- DNSKEY algorithms are discovered from DNSKEY responses only; if no usable DNSKEY answer exists, DS13 emits no algorithm findings.
- Nameserver evaluation is deduplicated by IP; repeated names on one IP share one DS13 outcome.
- Query-type checks are independent; a nameserver can emit findings for SOA/NS while DNSKEY-side checks are skipped, or vice versa, based on response shape.
