# DNSSEC03 (dnssec03)

Status: Final

## Purpose
- Verify NSEC3 parameter consistency and policy compliance across child nameservers when DNSKEY support is present.

## Preconditions And Inputs
- Preconditions:
  - A `zone.Zone` object is available.
- Required inputs:
  - Child nameserver list from `methods.Method4and5`.
  - DNSKEY and NSEC query responses from child nameservers.
- Profile/config knobs that affect behavior:
  - `net.ipv4` and `net.ipv6`: disabled transports are skipped with transport debug tags.
  - `resolver.defaults.parallel`: parallel nameserver query execution fanout.

## Algorithm And Decision Flow
1. Emit `TEST_CASE_START`.
2. Resolve nameservers from Method4+Method5 and deduplicate by IP.
3. For each unique nameserver IP (parallelized):
   - If transport is disabled, emit `IPV4_DISABLED` or `IPV6_DISABLED` for rrtypes `DNSKEY` and `NSEC` and skip.
   - Query DNSKEY:
     - If response is absent, non-`NOERROR`, or non-`AA`, skip nameserver without DS03 findings.
     - If no apex DNSKEY is present, mark nameserver in `Responds Without DNSKEY`.
     - Else mark nameserver in `Responds With DNSKEY` and continue.
   - Query NSEC:
     - No response -> mark `No Response NSEC Query`.
     - Non-`NOERROR` or non-`AA` -> mark `Error Response NSEC Query`.
     - No NSEC3 in authority -> mark `Responds Without NSEC3`.
     - Otherwise mark `Responds With NSEC3`; if multiple NSEC3 RRs exist, also mark `Multiple NSEC3`.
     - Use the first NSEC3 RR to extract hash algorithm, flags, iterations, and salt-length value.
4. Emit DNSSEC-support summary tags:
   - `DS03_NO_DNSSEC_SUPPORT` if no nameserver had DNSKEY but at least one lacked DNSKEY.
   - `DS03_SERVER_NO_DNSSEC_SUPPORT` if mixed DNSKEY support exists.
5. Emit NSEC3-availability summary tags:
   - `DS03_NO_NSEC3` if no nameserver had NSEC3 but at least one lacked NSEC3.
   - `DS03_SERVER_NO_NSEC3` if mixed NSEC3 support exists.
6. Emit `DS03_ERR_MULT_NSEC3` for nameservers that returned more than one NSEC3 RR.
7. Hash algorithm checks:
   - Emit `DS03_INCONSISTENT_HASH_ALGO` when more than one hash value appears.
   - Emit `DS03_LEGAL_HASH_ALGO` for value `1`; otherwise emit `DS03_ILLEGAL_HASH_ALGO` with `algo_num`.
8. NSEC3 flags checks:
   - Emit `DS03_INCONSISTENT_NSEC3_FLAGS` when more than one flags value appears.
   - For each flags value, emit `DS03_UNASSIGNED_FLAG_USED` for set bits `0..6`.
   - Determine opt-out from bit `7`:
     - Emit `DS03_NSEC3_OPT_OUT_ENABLED_TLD` when bit `7` is set and tested zone is root or single-label TLD.
     - Emit `DS03_NSEC3_OPT_OUT_ENABLED_NON_TLD` when bit `7` is set and zone is not root/single-label TLD.
     - Emit `DS03_NSEC3_OPT_OUT_DISABLED` when bit `7` is unset.
9. Iteration checks:
   - Emit `DS03_INCONSISTENT_ITERATION` when multiple values appear.
   - Emit `DS03_LEGAL_ITERATION_VALUE` for value `0`; otherwise `DS03_ILLEGAL_ITERATION_VALUE` with `int`.
10. Salt-length checks:
    - Emit `DS03_INCONSISTENT_SALT_LENGTH` when multiple lengths appear.
    - Emit `DS03_LEGAL_EMPTY_SALT` for length `0`; otherwise `DS03_ILLEGAL_SALT_LENGTH` with `int`.
11. Emit per-nameserver NSEC query-error tags:
    - `DS03_NO_RESPONSE_NSEC_QUERY` for each nameserver that returned no NSEC query response.
    - `DS03_ERROR_RESPONSE_NSEC_QUERY` for each nameserver that returned a non-`NOERROR` or non-`AA` NSEC response.
12. Emit `TEST_CASE_END`.

### Per-NS DNSKEY/NSEC Probe and Param Aggregation (steps 2-11)

{{% expand "Show diagram" %}}
{{< mermaid >}}
stateDiagram-v2
    [*] --> probe
    probe : per-NS DNSKEY/NSEC
    probe --> disabled : transport off
    probe --> query : transport on
    disabled : transport-disabled tag
    query : query DNSKEY and NSEC
    query --> dnskeyCheck
    dnskeyCheck : DNSKEY response
    dnskeyCheck --> noDnssec : no DNSKEY
    dnskeyCheck --> hasDnssec : DNSKEY present
    noDnssec --> nsecCheck
    hasDnssec --> nsecCheck
    nsecCheck : NSEC response
    nsecCheck --> nsecErr : error or missing
    nsecCheck --> noNsec3 : no NSEC3
    nsecCheck --> hasNsec3 : NSEC3 present
    hasNsec3 --> extract
    extract : extract NSEC3 params
    extract --> aggregate
    nsecErr --> aggregate
    noNsec3 --> aggregate
    aggregate : aggregate and check
    aggregate --> emit
    emit : multi-tag emission
    emit --> done
    done : emit test-case-end
    disabled --> [*]
    done --> [*]
{{< /mermaid >}}
{{% /expand %}}

## Emitted Tags (Possible Set)
| Tag | Emitted when |
| --- | --- |
| `DS03_ERROR_RESPONSE_NSEC_QUERY` | NSEC query response exists but is non-`NOERROR` or non-`AA`. |
| `DS03_ERR_MULT_NSEC3` | Authority section contains more than one NSEC3 record. |
| `DS03_ILLEGAL_HASH_ALGO` | NSEC3 hash algorithm is not `1` (SHA-1). |
| `DS03_ILLEGAL_ITERATION_VALUE` | NSEC3 iterations value is non-zero. |
| `DS03_ILLEGAL_SALT_LENGTH` | NSEC3 salt length is non-zero. |
| `DS03_INCONSISTENT_HASH_ALGO` | Different hash algorithm values are observed across nameservers. |
| `DS03_INCONSISTENT_ITERATION` | Different iteration values are observed across nameservers. |
| `DS03_INCONSISTENT_NSEC3_FLAGS` | Different NSEC3 flags values are observed across nameservers. |
| `DS03_INCONSISTENT_SALT_LENGTH` | Different NSEC3 salt lengths are observed across nameservers. |
| `DS03_LEGAL_EMPTY_SALT` | NSEC3 salt length is zero. |
| `DS03_LEGAL_HASH_ALGO` | NSEC3 hash algorithm is `1`. |
| `DS03_LEGAL_ITERATION_VALUE` | NSEC3 iterations value is `0`. |
| `DS03_NO_DNSSEC_SUPPORT` | No queried nameserver returned usable apex DNSKEY data. |
| `DS03_NO_NSEC3` | No queried nameserver returned NSEC3 after usable DNSKEY support was present. |
| `DS03_NO_RESPONSE_NSEC_QUERY` | NSEC query had no response message. |
| `DS03_NSEC3_OPT_OUT_DISABLED` | NSEC3 flags have opt-out bit unset. |
| `DS03_NSEC3_OPT_OUT_ENABLED_NON_TLD` | NSEC3 flags have opt-out bit set for a non-root/non-single-label-TLD zone. |
| `DS03_NSEC3_OPT_OUT_ENABLED_TLD` | NSEC3 flags have opt-out bit set for root or single-label TLD zone. |
| `DS03_SERVER_NO_DNSSEC_SUPPORT` | Mixed DNSKEY support exists across nameservers. |
| `DS03_SERVER_NO_NSEC3` | Mixed NSEC3 support exists across nameservers. |
| `DS03_UNASSIGNED_FLAG_USED` | One or more unassigned NSEC3 flag bits (`0..6`) are set. |
| `IPV4_DISABLED` | IPv4 transport is disabled for a queried nameserver (`DNSKEY`/`NSEC`). |
| `IPV6_DISABLED` | IPv6 transport is disabled for a queried nameserver (`DNSKEY`/`NSEC`). |
| `TEST_CASE_END` | Testcase completion marker is emitted. |
| `TEST_CASE_START` | Testcase start marker is emitted. |

## Tag Arguments
| Tag | Argument key | Type | Meaning |
| --- | --- | --- | --- |
| `DS03_ERROR_RESPONSE_NSEC_QUERY` | `servers` | `array<object>` | Structured nameserver identities (`{ns,address}` object) with erroneous NSEC response. |
| `DS03_ERR_MULT_NSEC3` | `servers` | `array<object>` | Structured nameserver identities (`{ns,address}` object) returning multiple NSEC3 RRs. |
| `DS03_ILLEGAL_HASH_ALGO` | `servers` | `array<object>` | Structured nameserver identities (`{ns,address}` object) using non-1 NSEC3 hash. |
| `DS03_ILLEGAL_HASH_ALGO` | `algo_num` | `int` | NSEC3 hash algorithm number. |
| `DS03_ILLEGAL_ITERATION_VALUE` | `servers` | `array<object>` | Structured nameserver identities (`{ns,address}` object) with non-zero NSEC3 iterations. |
| `DS03_ILLEGAL_ITERATION_VALUE` | `int` | `int` | NSEC3 iteration value. |
| `DS03_ILLEGAL_SALT_LENGTH` | `servers` | `array<object>` | Structured nameserver identities (`{ns,address}` object) with non-zero salt length. |
| `DS03_ILLEGAL_SALT_LENGTH` | `int` | `int` | NSEC3 salt length value used by implementation. |
| `DS03_INCONSISTENT_HASH_ALGO` | `-` | `-` | No arguments. |
| `DS03_INCONSISTENT_ITERATION` | `-` | `-` | No arguments. |
| `DS03_INCONSISTENT_NSEC3_FLAGS` | `-` | `-` | No arguments. |
| `DS03_INCONSISTENT_SALT_LENGTH` | `-` | `-` | No arguments. |
| `DS03_LEGAL_EMPTY_SALT` | `servers` | `array<object>` | Structured nameserver identities (`{ns,address}` object) with zero salt length. |
| `DS03_LEGAL_HASH_ALGO` | `servers` | `array<object>` | Structured nameserver identities (`{ns,address}` object) using hash algorithm `1`. |
| `DS03_LEGAL_ITERATION_VALUE` | `servers` | `array<object>` | Structured nameserver identities (`{ns,address}` object) using iteration `0`. |
| `DS03_NO_DNSSEC_SUPPORT` | `servers` | `array<object>` | Structured nameserver identities (`{ns,address}` object) without usable DNSKEY support. |
| `DS03_NO_NSEC3` | `servers` | `array<object>` | Structured nameserver identities (`{ns,address}` object) without NSEC3. |
| `DS03_NO_RESPONSE_NSEC_QUERY` | `servers` | `array<object>` | Structured nameserver identities (`{ns,address}` object) with no NSEC response. |
| `DS03_NSEC3_OPT_OUT_DISABLED` | `servers` | `array<object>` | Structured nameserver identities (`{ns,address}` object) with opt-out bit unset. |
| `DS03_NSEC3_OPT_OUT_ENABLED_NON_TLD` | `servers` | `array<object>` | Structured nameserver identities (`{ns,address}` object) with opt-out bit set for non-root/non-single-label-TLD zone. |
| `DS03_NSEC3_OPT_OUT_ENABLED_TLD` | `servers` | `array<object>` | Structured nameserver identities (`{ns,address}` object) with opt-out bit set for root/single-label-TLD zone. |
| `DS03_SERVER_NO_DNSSEC_SUPPORT` | `servers` | `array<object>` | Structured nameserver identities (`{ns,address}` object) lacking DNSKEY support while others support it. |
| `DS03_SERVER_NO_NSEC3` | `servers` | `array<object>` | Structured nameserver identities (`{ns,address}` object) lacking NSEC3 while others provide it. |
| `DS03_UNASSIGNED_FLAG_USED` | `servers` | `array<object>` | Structured nameserver identities (`{ns,address}` object) where the unassigned bit is set. |
| `DS03_UNASSIGNED_FLAG_USED` | `int` | `int` | NSEC3 flag bit index (`0..6`) treated as unassigned. |
| `IPV4_DISABLED` | `ns` | `string` | Nameserver identity (`ns` name only; use `address` for IP) skipped on IPv4. |
| `IPV4_DISABLED` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `IPV4_DISABLED` | `rrtype` | `string` | rrtype skipped (`DNSKEY` or `NSEC`). |
| `IPV6_DISABLED` | `ns` | `string` | Nameserver identity (`ns` name only; use `address` for IP) skipped on IPv6. |
| `IPV6_DISABLED` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `IPV6_DISABLED` | `rrtype` | `string` | rrtype skipped (`DNSKEY` or `NSEC`). |
| `TEST_CASE_END` | `testcase` | `string` | Testcase display name (`DNSSEC03`). |
| `TEST_CASE_START` | `testcase` | `string` | Testcase display name (`DNSSEC03`). |

## Severity Levels Per Tag
| Tag | Level | Notes |
| --- | --- | --- |
| `DS03_ERROR_RESPONSE_NSEC_QUERY` | `ERROR` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS03_ERR_MULT_NSEC3` | `ERROR` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS03_ILLEGAL_HASH_ALGO` | `ERROR` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS03_ILLEGAL_ITERATION_VALUE` | `WARNING` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS03_ILLEGAL_SALT_LENGTH` | `WARNING` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS03_INCONSISTENT_HASH_ALGO` | `ERROR` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS03_INCONSISTENT_ITERATION` | `ERROR` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS03_INCONSISTENT_NSEC3_FLAGS` | `ERROR` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS03_INCONSISTENT_SALT_LENGTH` | `ERROR` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS03_LEGAL_EMPTY_SALT` | `INFO` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS03_LEGAL_HASH_ALGO` | `INFO` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS03_LEGAL_ITERATION_VALUE` | `INFO` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS03_NO_DNSSEC_SUPPORT` | `NOTICE` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS03_NO_NSEC3` | `INFO` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS03_NO_RESPONSE_NSEC_QUERY` | `ERROR` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS03_NSEC3_OPT_OUT_DISABLED` | `INFO` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS03_NSEC3_OPT_OUT_ENABLED_NON_TLD` | `NOTICE` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS03_NSEC3_OPT_OUT_ENABLED_TLD` | `INFO` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS03_SERVER_NO_DNSSEC_SUPPORT` | `ERROR` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS03_SERVER_NO_NSEC3` | `ERROR` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DS03_UNASSIGNED_FLAG_USED` | `ERROR` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `IPV4_DISABLED` | `DEBUG` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `IPV6_DISABLED` | `DEBUG` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `TEST_CASE_END` | `DEBUG` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `TEST_CASE_START` | `DEBUG` | Default from `share/profile.json` (`test_levels.DNSSEC`). |

## Differences From Upstream
- Upstream reference: [`dnssec03.md`](../../upstream/tests/DNSSEC-TP/dnssec03.md)
- Differences (Upstream vs Gonemaster):
  - Upstream: allows TLD-like classification based on Public Suffix List data for opt-out interpretation. Gonemaster: treats TLD context as only root (`.`) or a direct single-label TLD (no PSL-based classification in this testcase).
  - Upstream: does not explicitly specify testcase boundary and per-query transport debug emissions in this testcase summary. Gonemaster: emits `TEST_CASE_START`, `TEST_CASE_END`, `IPV4_DISABLED`, and `IPV6_DISABLED`.
- Potential upstream report:
  - `no`

## Edge Cases And Limitations
- When multiple NSEC3 records are present, only the first NSEC3 RR is used for parameter extraction (hash, flags, iterations, salt length).
- Nameservers whose DNSKEY response fails shape checks (`NOERROR` and `AA`) are silently skipped for DS03 finding sets.
- Salt length is derived from the NSEC3 `Salt` string length used by the implementation, not a decoded-octet length conversion step.
