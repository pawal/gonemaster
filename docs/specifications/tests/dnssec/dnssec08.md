# DNSSEC08

Status: Final

## Purpose
- Verify that DNSKEY RRset signatures are present, time-valid, algorithm-supported, and cryptographically match DNSKEY records at child nameservers.

## Preconditions And Inputs
- Preconditions:
  - A `zone.Zone` object is available.
- Required inputs:
  - Child nameserver sets from [`GlueNameservers`](../../nameserver-resolution.md#gluenameservers) and [`ApexNameservers`](../../nameserver-resolution.md#apexnameservers).
  - DNSKEY query responses with DNSSEC enabled from child nameservers.
- Profile/config knobs that affect behavior:
  - `net.ipv4` and `net.ipv6`: disabled transports are skipped with transport debug tags.
  - `resolver.defaults.parallel`: parallel DNSKEY-query execution fanout.

## Algorithm And Decision Flow
1. Emit `TEST_CASE_START`.
2. Build nameserver set from the union of [`GlueNameservers`](../../nameserver-resolution.md#gluenameservers) and [`ApexNameservers`](../../nameserver-resolution.md#apexnameservers) (deduplicated by `ns.String()`), then deduplicate by IP.
3. For each unique nameserver IP (parallelized):
   - If transport is disabled, emit `IPV4_DISABLED` or `IPV6_DISABLED` for rrtype `DNSKEY` and skip.
   - Query child apex `DNSKEY` with DNSSEC enabled.
   - Require response message, `RCODE=NOERROR`, and `AA=true`; otherwise skip nameserver.
   - Require at least one apex DNSKEY answer record; otherwise skip nameserver.
   - If no `RRSIG` answer records, mark nameserver in `DNSKEY without RRSIG`.
   - Else evaluate each answer-section RRSIG record:
     - If inception is in future, mark `DNSKEY RRSIG not yet valid` by keytag.
     - Else if expiration is in past, mark `DNSKEY RRSIG expired` by keytag.
     - Else if signature algorithm is unsupported, mark `Algo Not Supported By ZM` by keytag+algo.
     - Else find matching DNSKEYs by keytag:
       - If none found, mark `No matching DNSKEY`.
       - Else verify signature against matching DNSKEY candidates:
         - If verification reports unsupported algorithm (`dns.ErrAlg`), mark `Algo Not Supported By ZM`.
         - Else if no candidate validates, mark `RRSIG not valid by DNSKEY`.
4. Emit accumulated findings:
   - `DS08_MISSING_RRSIG_IN_RESPONSE`
   - `DS08_DNSKEY_RRSIG_NOT_YET_VALID`
   - `DS08_DNSKEY_RRSIG_EXPIRED`
   - `DS08_NO_MATCHING_DNSKEY`
   - `DS08_RRSIG_NOT_VALID_BY_DNSKEY`
   - `DS08_ALGO_NOT_SUPPORTED_BY_ZM`
5. Collect nameserver IPs that had RRSIGs and emitted no DS08 failure; if non-empty emit `DS08_DNSKEY_RRSIG_VALID`.
6. Emit `TEST_CASE_END`.

### Per-NS DNSKEY RRSIG Verification (steps 2-6)

{{% expand "Show diagram" %}}
```
child set = GlueNameservers ++ ApexNameservers; dedupe by IP

For each unique child NS IP (parallel; fan-out = resolver.defaults.parallel):

   transport disabled for DNSKEY -> IPV4_DISABLED / IPV6_DISABLED, skip
   query DNSKEY at z.Name, DNSSEC=on
    +- resp.Msg == nil / RCODE != NOERROR / !AA  -> skip
    +- no apex DNSKEY in answer                  -> skip
    +- no RRSIG records in answer                -> missingRRSIG[ns]
    +- otherwise, for each RRSIG (per-sig keyed by sig.KeyTag):
         sig.Inception > packet now
                                                 -> rrsigNotYetValid[keytag][ns]
         sig.Expiration < packet now
                                                 -> rrsigExpired[keytag][ns]
         algorithm not supported in advance
                                                 -> algoNotSupportedByZM[keytag][algo][ns]
         no DNSKEY with that keytag in answer
                                                 -> noMatchingDNSKEY[keytag][ns]
         verify against matching DNSKEYs:
            err == ErrAlg
                                                 -> algoNotSupportedByZM[keytag][algo][ns]
            no candidate validates
                                                 -> rrsigNotValidByDNSKEY[keytag][ns]
            success
                                                 -> mark ns as having at least one valid RRSIG

Emit per category:
  missingRRSIG IPs                     -> DS08_MISSING_RRSIG_IN_RESPONSE  (addresses)
  per keytag rrsigNotYetValid          -> DS08_DNSKEY_RRSIG_NOT_YET_VALID (keytag, addresses)
  per keytag rrsigExpired              -> DS08_DNSKEY_RRSIG_EXPIRED       (keytag, addresses)
  per keytag noMatchingDNSKEY          -> DS08_NO_MATCHING_DNSKEY         (keytag, addresses)
  per keytag rrsigNotValidByDNSKEY     -> DS08_RRSIG_NOT_VALID_BY_DNSKEY  (keytag, addresses)
  per (keytag, algo) algoNotSupportedByZM
                                       -> DS08_ALGO_NOT_SUPPORTED_BY_ZM  (keytag, algo_num,
                                                                          algo_mnemo, addresses)

  IPs that had any RRSIG AND no DS08 failure
                                       -> DS08_DNSKEY_RRSIG_VALID         (addresses)

emit TEST_CASE_END
```
{{% /expand %}}

## Emitted Tags (Possible Set)
| Tag | Emitted when |
| --- | --- |
| `DS08_ALGO_NOT_SUPPORTED_BY_ZM` | RRSIG verification requires an unsupported DNSSEC algorithm. |
| `DS08_DNSKEY_RRSIG_VALID` | At least one nameserver returned DNSKEY with RRSIG and all RRSIG checks passed. |
| `DS08_DNSKEY_RRSIG_EXPIRED` | DNSKEY-related RRSIG expiration is before packet test time. |
| `DS08_DNSKEY_RRSIG_NOT_YET_VALID` | DNSKEY-related RRSIG inception is after packet test time. |
| `DS08_MISSING_RRSIG_IN_RESPONSE` | DNSKEY answer exists but contains no RRSIG records. |
| `DS08_NO_MATCHING_DNSKEY` | RRSIG keytag has no matching DNSKEY in DNSKEY RRset. |
| `DS08_RRSIG_NOT_VALID_BY_DNSKEY` | Matching DNSKEY candidates exist but none validate the RRSIG. |
| `DS08_RSA_EXPONENT_UNSUPPORTED` | RRSIG could not be checked only because the matching DNSKEY is an RSA key whose public exponent exceeds what the local verifier supports (more than 64 bits). |
| `IPV4_DISABLED` | IPv4 transport is disabled for a queried nameserver (`DNSKEY`). |
| `IPV6_DISABLED` | IPv6 transport is disabled for a queried nameserver (`DNSKEY`). |
| `TEST_CASE_END` | Testcase completion marker is emitted. |
| `TEST_CASE_START` | Testcase start marker is emitted. |

## Tag Arguments
| Tag | Argument key | Type | Meaning |
| --- | --- | --- | --- |
| `DS08_ALGO_NOT_SUPPORTED_BY_ZM` | `keytag` | `int` | RRSIG keytag associated with unsupported algorithm. |
| `DS08_ALGO_NOT_SUPPORTED_BY_ZM` | `algo_num` | `int` | Unsupported DNSSEC algorithm number. |
| `DS08_ALGO_NOT_SUPPORTED_BY_ZM` | `algo_mnemo` | `string` | Unsupported DNSSEC algorithm mnemonic. |
| `DS08_ALGO_NOT_SUPPORTED_BY_ZM` | `addresses` | `array<string>` | Structured child nameserver IP list. |
| `DS08_DNSKEY_RRSIG_EXPIRED` | `keytag` | `int` | RRSIG keytag with expired validity window. |
| `DS08_DNSKEY_RRSIG_EXPIRED` | `addresses` | `array<string>` | Structured child nameserver IP list. |
| `DS08_DNSKEY_RRSIG_NOT_YET_VALID` | `keytag` | `int` | RRSIG keytag with not-yet-valid validity window. |
| `DS08_DNSKEY_RRSIG_NOT_YET_VALID` | `addresses` | `array<string>` | Structured child nameserver IP list. |
| `DS08_DNSKEY_RRSIG_VALID` | `addresses` | `array<string>` | Structured child nameserver IP list where all DNSKEY RRSIG checks passed. |
| `DS08_MISSING_RRSIG_IN_RESPONSE` | `addresses` | `array<string>` | Structured child nameserver IP list lacking RRSIG in DNSKEY response. |
| `DS08_NO_MATCHING_DNSKEY` | `keytag` | `int` | RRSIG keytag with no matching DNSKEY keytag in DNSKEY RRset. |
| `DS08_NO_MATCHING_DNSKEY` | `addresses` | `array<string>` | Structured child nameserver IP list. |
| `DS08_RRSIG_NOT_VALID_BY_DNSKEY` | `keytag` | `int` | RRSIG keytag that failed DNSKEY verification. |
| `DS08_RRSIG_NOT_VALID_BY_DNSKEY` | `addresses` | `array<string>` | Structured child nameserver IP list. |
| `DS08_RSA_EXPONENT_UNSUPPORTED` | `keytag` | `int` | RRSIG keytag whose matching DNSKEY RSA public exponent the local verifier cannot use. |
| `DS08_RSA_EXPONENT_UNSUPPORTED` | `addresses` | `array<string>` | Structured child nameserver IP list. |
| `IPV4_DISABLED` | `ns` | `string` | Nameserver identity (`ns` name only; use `address` for IP) skipped on IPv4. |
| `IPV4_DISABLED` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `IPV4_DISABLED` | `rrtype` | `string` | rrtype skipped (`DNSKEY`). |
| `IPV6_DISABLED` | `ns` | `string` | Nameserver identity (`ns` name only; use `address` for IP) skipped on IPv6. |
| `IPV6_DISABLED` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `IPV6_DISABLED` | `rrtype` | `string` | rrtype skipped (`DNSKEY`). |
| `TEST_CASE_END` | `testcase` | `string` | Testcase display name (`DNSSEC08`). |
| `TEST_CASE_START` | `testcase` | `string` | Testcase display name (`DNSSEC08`). |

## Severity Levels Per Tag
| Tag | Level | Notes |
| --- | --- | --- |
| `DS08_ALGO_NOT_SUPPORTED_BY_ZM` | `NOTICE` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS08_DNSKEY_RRSIG_EXPIRED` | `ERROR` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS08_DNSKEY_RRSIG_NOT_YET_VALID` | `ERROR` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS08_DNSKEY_RRSIG_VALID` | `INFO` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS08_MISSING_RRSIG_IN_RESPONSE` | `ERROR` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS08_NO_MATCHING_DNSKEY` | `ERROR` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS08_RRSIG_NOT_VALID_BY_DNSKEY` | `ERROR` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS08_RSA_EXPONENT_UNSUPPORTED` | `NOTICE` | Default from `share/profile.json` (`test_levels.DNSSEC`); carries zero score penalty (`scoring` `TagPenalties`). |
| `IPV4_DISABLED` | `DEBUG` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `IPV6_DISABLED` | `DEBUG` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `TEST_CASE_END` | `DEBUG` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `TEST_CASE_START` | `DEBUG` | Default from `share/profile.json` (`test_levels.DNSSEC`). |

## Differences From Upstream
- Differences (Upstream vs Gonemaster):
  - Upstream: procedure text describes evaluation of DNSKEY RRSIG records (covering the DNSKEY RRset). Gonemaster: evaluates all answer-section RRSIG records and does not explicitly filter on `TypeCovered == DNSKEY` before classification.
  - Upstream: does not explicitly specify testcase boundary and per-query transport debug emissions in this testcase summary. Gonemaster: emits `TEST_CASE_START`, `TEST_CASE_END`, `IPV4_DISABLED`, and `IPV6_DISABLED`.
- Potential upstream report:
  - `no`

## Edge Cases And Limitations
- Nameserver evaluation is deduplicated by IP; repeated names on one IP share one query outcome.
- Responses failing shape checks (`Msg`, `NOERROR`, `AA`, apex DNSKEY presence) are silently skipped for DS08 findings.
- Unsupported algorithm can be detected either before verification (`dnssecAlgorithmSupported`) or during verification (`dns.ErrAlg`).
- Large RSA public exponent handling: an RSA DNSKEY the DNS library refuses although RFC 3110 permits it (an exponent of more than 4 bytes or greater than 2^31-1, or a leading zero byte in the exponent or modulus) is verified by gonemaster's own RSA path instead, so its RRSIGs pass or fail like any other. Only when the exponent exceeds 64 bits is the finding reclassified from the `ERROR` `DS08_RRSIG_NOT_VALID_BY_DNSKEY` to the `NOTICE` `DS08_RSA_EXPONENT_UNSUPPORTED`. Such a nameserver is treated as indeterminate and is not counted as a `DS08_DNSKEY_RRSIG_VALID` pass.
