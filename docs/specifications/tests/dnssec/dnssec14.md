# DNSSEC14

Status: Final

## Purpose
- Validate RSA DNSKEY key sizes against per-algorithm minimum/maximum ranges and recommended size thresholds.

## Preconditions And Inputs
- Preconditions:
  - A `zone.Zone` object is available.
- Required inputs:
  - Child nameservers from `GlueNameservers` and `ApexNameservers`.
  - DNSKEY responses from child nameservers.
  - RSA key-size policy map (`rsaKeySizeByAlgo`).
- Profile/config knobs that affect behavior:
  - `net.ipv4` and `net.ipv6`: disabled transports are skipped with transport debug tags.
  - `resolver.defaults.parallel`: parallel nameserver query fanout.

## Algorithm And Decision Flow
1. Emit `TEST_CASE_START`.
2. Build child nameserver set from AllNameservers by unique nameserver string (not IP-deduplicated).
3. For each nameserver (parallelized):
   - If transport is disabled, emit `IPV4_DISABLED` or `IPV6_DISABLED` for rrtype `DNSKEY` and skip.
   - Query `DNSKEY` with DNSSEC enabled.
   - If response message is absent, emit `NO_RESPONSE`.
   - Else if answer has no DNSKEY records, emit `NO_RESPONSE_DNSKEY`.
   - Else collect DNSKEY records for key-size validation.
4. For each collected DNSKEY, only when its algorithm exists in the RSA key-size table:
   - Compute key size via `dnskeyKeySize`.
   - Deduplicate by `(keytag,keysize,algorithm)`.
   - Emit:
     - `DNSKEY_TOO_SMALL_FOR_ALGO` when key size `< keysizemin`.
     - `DNSKEY_SMALLER_THAN_REC` when key size `< keysizerec`.
     - `DNSKEY_TOO_LARGE_FOR_ALGO` when key size `> keysizemax`.
5. If at least one DNSKEY was collected and internal `KEY_SIZE_OK` condition is met, emit `KEY_SIZE_OK`.
6. Emit `TEST_CASE_END`.

## Emitted Tags (Possible Set)
| Tag | Emitted when |
| --- | --- |
| `DNSKEY_SMALLER_THAN_REC` | RSA DNSKEY size is below recommended size for algorithm. |
| `DNSKEY_TOO_LARGE_FOR_ALGO` | RSA DNSKEY size is above allowed maximum for algorithm. |
| `DNSKEY_TOO_SMALL_FOR_ALGO` | RSA DNSKEY size is below allowed minimum for algorithm. |
| `IPV4_DISABLED` | IPv4 transport is disabled for a queried nameserver (`DNSKEY`). |
| `IPV6_DISABLED` | IPv6 transport is disabled for a queried nameserver (`DNSKEY`). |
| `KEY_SIZE_OK` | DNSKEY key-size checks are considered OK by current implementation condition. |
| `NO_RESPONSE` | DNSKEY query returned no DNS message. |
| `NO_RESPONSE_DNSKEY` | DNS response did not contain DNSKEY records in answer. |
| `TEST_CASE_END` | Testcase completion marker is emitted. |
| `TEST_CASE_START` | Testcase start marker is emitted. |

## Tag Arguments
| Tag | Argument key | Type | Meaning |
| --- | --- | --- | --- |
| `DNSKEY_SMALLER_THAN_REC` | `algo_num` | `int` | DNSKEY algorithm number. |
| `DNSKEY_SMALLER_THAN_REC` | `algo_descr` | `string` | DNSKEY algorithm description. |
| `DNSKEY_SMALLER_THAN_REC` | `keytag` | `int` | DNSKEY keytag. |
| `DNSKEY_SMALLER_THAN_REC` | `keysize` | `int` | Calculated key size in bits. |
| `DNSKEY_SMALLER_THAN_REC` | `keysizemin` | `int` | Allowed minimum size for algorithm. |
| `DNSKEY_SMALLER_THAN_REC` | `keysizemax` | `int` | Allowed maximum size for algorithm. |
| `DNSKEY_SMALLER_THAN_REC` | `keysizerec` | `int` | Recommended size for algorithm. |
| `DNSKEY_TOO_LARGE_FOR_ALGO` | `algo_num` | `int` | DNSKEY algorithm number. |
| `DNSKEY_TOO_LARGE_FOR_ALGO` | `algo_descr` | `string` | DNSKEY algorithm description. |
| `DNSKEY_TOO_LARGE_FOR_ALGO` | `keytag` | `int` | DNSKEY keytag. |
| `DNSKEY_TOO_LARGE_FOR_ALGO` | `keysize` | `int` | Calculated key size in bits. |
| `DNSKEY_TOO_LARGE_FOR_ALGO` | `keysizemin` | `int` | Allowed minimum size for algorithm. |
| `DNSKEY_TOO_LARGE_FOR_ALGO` | `keysizemax` | `int` | Allowed maximum size for algorithm. |
| `DNSKEY_TOO_LARGE_FOR_ALGO` | `keysizerec` | `int` | Recommended size for algorithm. |
| `DNSKEY_TOO_SMALL_FOR_ALGO` | `algo_num` | `int` | DNSKEY algorithm number. |
| `DNSKEY_TOO_SMALL_FOR_ALGO` | `algo_descr` | `string` | DNSKEY algorithm description. |
| `DNSKEY_TOO_SMALL_FOR_ALGO` | `keytag` | `int` | DNSKEY keytag. |
| `DNSKEY_TOO_SMALL_FOR_ALGO` | `keysize` | `int` | Calculated key size in bits. |
| `DNSKEY_TOO_SMALL_FOR_ALGO` | `keysizemin` | `int` | Allowed minimum size for algorithm. |
| `DNSKEY_TOO_SMALL_FOR_ALGO` | `keysizemax` | `int` | Allowed maximum size for algorithm. |
| `DNSKEY_TOO_SMALL_FOR_ALGO` | `keysizerec` | `int` | Recommended size for algorithm. |
| `IPV4_DISABLED` | `ns` | `string` | Nameserver identity (`ns` name only; use `address` for IP) skipped on IPv4. |
| `IPV4_DISABLED` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `IPV4_DISABLED` | `rrtype` | `string` | rrtype skipped (`DNSKEY`). |
| `IPV6_DISABLED` | `ns` | `string` | Nameserver identity (`ns` name only; use `address` for IP) skipped on IPv6. |
| `IPV6_DISABLED` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `IPV6_DISABLED` | `rrtype` | `string` | rrtype skipped (`DNSKEY`). |
| `KEY_SIZE_OK` | `-` | `-` | No arguments. |
| `NO_RESPONSE` | `ns` | `string` | Nameserver identity (`ns` name only; use `address` for IP) with no response. |
| `NO_RESPONSE` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `NO_RESPONSE_DNSKEY` | `ns` | `string` | Nameserver identity (`ns` name only; use `address` for IP) with no DNSKEY answer RRset. |
| `NO_RESPONSE_DNSKEY` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `TEST_CASE_END` | `testcase` | `string` | Testcase display name (`DNSSEC14`). |
| `TEST_CASE_START` | `testcase` | `string` | Testcase display name (`DNSSEC14`). |

## Severity Levels Per Tag
| Tag | Level | Notes |
| --- | --- | --- |
| `DNSKEY_SMALLER_THAN_REC` | `WARNING` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DNSKEY_TOO_LARGE_FOR_ALGO` | `ERROR` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DNSKEY_TOO_SMALL_FOR_ALGO` | `ERROR` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `IPV4_DISABLED` | `DEBUG` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `IPV6_DISABLED` | `DEBUG` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `KEY_SIZE_OK` | `INFO` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `NO_RESPONSE` | `DEBUG` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `NO_RESPONSE_DNSKEY` | `ERROR` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `TEST_CASE_END` | `DEBUG` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `TEST_CASE_START` | `DEBUG` | Default from `share/profile.json` (`test_levels.DNSSEC`). |

## Differences From Upstream
- Upstream reference: [`dnssec14.md`](../../upstream/tests/DNSSEC-TP/dnssec14.md)
- Differences (Upstream vs Gonemaster):
  - Upstream: describes mutually exclusive key-size outcomes (ordered `else if`). Gonemaster: evaluates size checks with independent `if` branches, so a single key can emit both `DNSKEY_TOO_SMALL_FOR_ALGO` and `DNSKEY_SMALLER_THAN_REC`.
  - Upstream: default level table lists `NO_RESPONSE_DNSKEY` as `WARNING`. Gonemaster: default level is `ERROR` in `share/profile.json`.
  - Upstream: describes emitting `KEY_SIZE_OK` when no non-`NO_RESPONSE` issues occur. Gonemaster: current condition compares total result count (including testcase boundary tags) against `NO_RESPONSE` count, which makes `KEY_SIZE_OK` effectively unreachable.
  - Upstream: does not explicitly specify testcase boundary and transport-disabled debug emissions in this testcase summary. Gonemaster: emits `TEST_CASE_START`, `TEST_CASE_END`, `IPV4_DISABLED`, and `IPV6_DISABLED`.
- Potential upstream report:
  - `no`

## Edge Cases And Limitations
- Non-RSA algorithms are ignored in key-size checks.
- Nameservers are not deduplicated by IP in this testcase; duplicate IPs under different names are queried separately.
- Invalid/undecodable RSA public keys produce key size `0`, which can trigger small-key findings.
- **`KEY_SIZE_OK` is effectively unreachable in the current implementation.** The gate condition compares the total result-entry count (which always includes at least the `TEST_CASE_START` and `TEST_CASE_END` boundary entries) against the `NO_RESPONSE` count. Because boundary entries are always present, the two counts can never be equal, so `KEY_SIZE_OK` is never emitted. This is a known gonemaster implementation defect (see Differences From Upstream).
