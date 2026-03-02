# Consistency03 (consistency03)

Status: Final

## Purpose
- Check consistency of SOA timer fields (`refresh`, `retry`, `expire`, `minimum`) across nameservers.

## Preconditions And Inputs
- Preconditions:
  - A `zone.Zone` object is available.
- Required inputs:
  - Nameserver list from `methods.Method4` and `methods.Method5`.
  - SOA answers from queried nameservers.
- Profile/config knobs that affect behavior:
  - `net.ipv4` and `net.ipv6`: disabled transports are skipped per nameserver.
  - `resolver.defaults.parallel`: per-nameserver query task parallelism.

## Algorithm And Decision Flow
1. Emit `TEST_CASE_START`.
2. Build deduplicated nameserver list from Method4+Method5 by `ns.String()`.
3. For each nameserver (parallelized):
   - If transport is disabled, emit `IPV4_DISABLED` or `IPV6_DISABLED` for rrtype `SOA` and skip.
   - Query SOA for zone apex.
   - No response message -> emit `NO_RESPONSE`.
   - Response without usable SOA record -> emit `NO_RESPONSE_SOA_QUERY`.
   - Otherwise extract timer tuple `(refresh,retry,expire,minimum)` and store it for that nameserver.
4. If exactly one timer tuple exists, emit `ONE_SOA_TIME_PARAMETER_SET`.
5. If multiple timer tuples exist:
   - Emit `MULTIPLE_SOA_TIME_PARAMETER_SET`.
   - Emit `SOA_TIME_PARAMETER_SET` once per tuple with associated sorted `servers`.
6. Emit `TEST_CASE_END`.

## Emitted Tags (Possible Set)
| Tag | Emitted when |
| --- | --- |
| `IPV4_DISABLED` | IPv4 transport is disabled for a queried nameserver/rrtype. |
| `IPV6_DISABLED` | IPv6 transport is disabled for a queried nameserver/rrtype. |
| `MULTIPLE_SOA_TIME_PARAMETER_SET` | At least two distinct SOA timer tuples were observed. |
| `NO_RESPONSE` | SOA query had no response message from a nameserver. |
| `NO_RESPONSE_SOA_QUERY` | Response did not contain a usable SOA record for zone apex. |
| `ONE_SOA_TIME_PARAMETER_SET` | Exactly one SOA timer tuple was observed. |
| `SOA_TIME_PARAMETER_SET` | A specific timer tuple and associated nameservers are reported. |
| `TEST_CASE_END` | Testcase completion marker is emitted. |
| `TEST_CASE_START` | Testcase start marker is emitted. |

## Tag Arguments
| Tag | Argument key | Type | Meaning |
| --- | --- | --- | --- |
| `IPV4_DISABLED` | `ns` | `string` | Nameserver identity (`ns` name only; use `address` for IP) skipped on IPv4. |
| `IPV4_DISABLED` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `IPV4_DISABLED` | `rrtype` | `string` | rrtype skipped (`SOA`). |
| `IPV6_DISABLED` | `ns` | `string` | Nameserver identity (`ns` name only; use `address` for IP) skipped on IPv6. |
| `IPV6_DISABLED` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `IPV6_DISABLED` | `rrtype` | `string` | rrtype skipped (`SOA`). |
| `MULTIPLE_SOA_TIME_PARAMETER_SET` | `count` | `int` | Number of distinct timer tuples observed. |
| `NO_RESPONSE` | `ns` | `string` | Nameserver identity (`ns` name only; use `address` for IP) with no response. |
| `NO_RESPONSE` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `NO_RESPONSE_SOA_QUERY` | `ns` | `string` | Nameserver identity (`ns` name only; use `address` for IP) without usable SOA answer. |
| `NO_RESPONSE_SOA_QUERY` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `ONE_SOA_TIME_PARAMETER_SET` | `refresh` | `int` | SOA refresh value. |
| `ONE_SOA_TIME_PARAMETER_SET` | `retry` | `int` | SOA retry value. |
| `ONE_SOA_TIME_PARAMETER_SET` | `expire` | `int` | SOA expire value. |
| `ONE_SOA_TIME_PARAMETER_SET` | `minimum` | `int` | SOA minimum (minttl) value. |
| `SOA_TIME_PARAMETER_SET` | `refresh` | `int` | SOA refresh value. |
| `SOA_TIME_PARAMETER_SET` | `retry` | `int` | SOA retry value. |
| `SOA_TIME_PARAMETER_SET` | `expire` | `int` | SOA expire value. |
| `SOA_TIME_PARAMETER_SET` | `minimum` | `int` | SOA minimum (minttl) value. |
| `SOA_TIME_PARAMETER_SET` | `servers` | `array<object>` | Structured nameserver identities (`{ns,address}` object) serving that tuple. |
| `TEST_CASE_END` | `testcase` | `string` | Testcase display name (`Consistency03`). |
| `TEST_CASE_START` | `testcase` | `string` | Testcase display name (`Consistency03`). |

## Severity Levels Per Tag
| Tag | Level | Notes |
| --- | --- | --- |
| `IPV4_DISABLED` | `DEBUG` | Default from `share/profile.json` (`test_levels.CONSISTENCY`). |
| `IPV6_DISABLED` | `DEBUG` | Default from `share/profile.json` (`test_levels.CONSISTENCY`). |
| `MULTIPLE_SOA_TIME_PARAMETER_SET` | `NOTICE` | Default from `share/profile.json` (`test_levels.CONSISTENCY`). |
| `NO_RESPONSE` | `DEBUG` | Default from `share/profile.json` (`test_levels.CONSISTENCY`). |
| `NO_RESPONSE_SOA_QUERY` | `DEBUG` | Default from `share/profile.json` (`test_levels.CONSISTENCY`). |
| `ONE_SOA_TIME_PARAMETER_SET` | `INFO` | Default from `share/profile.json` (`test_levels.CONSISTENCY`). |
| `SOA_TIME_PARAMETER_SET` | `INFO` | Default from `share/profile.json` (`test_levels.CONSISTENCY`). |
| `TEST_CASE_END` | `DEBUG` | Default from `share/profile.json` (`test_levels.CONSISTENCY`). |
| `TEST_CASE_START` | `DEBUG` | Default from `share/profile.json` (`test_levels.CONSISTENCY`). |

## Differences From Upstream
- Upstream reference: [`consistency03.md`](../../upstream/tests/Consistency-TP/consistency03.md)
- Differences (Upstream vs Gonemaster):
  - Upstream: does not explicitly define this detail. Gonemaster: emits `SOA_TIME_PARAMETER_SET` detail entries for each observed timer tuple.
  - Upstream: does not explicitly define this detail. Gonemaster: Per-query transport debug tags (`IPV4_DISABLED`, `IPV6_DISABLED`) are emitted when transport is disabled.
- Potential upstream report:
  - `no`

## Edge Cases And Limitations
- If no usable SOA timer tuple is obtained, neither `ONE_SOA_TIME_PARAMETER_SET` nor `MULTIPLE_SOA_TIME_PARAMETER_SET` is emitted.
- Timer tuple identity is exact integer equality across the four SOA timer fields.
