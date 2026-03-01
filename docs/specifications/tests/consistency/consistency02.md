# Consistency02 (consistency02)

Status: Final

## Purpose
- Check SOA RNAME consistency across nameservers for the tested zone.

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
   - Otherwise extract lowercase SOA RNAME and store it for that nameserver.
4. If exactly one RNAME value exists, emit `ONE_SOA_RNAME`.
5. If multiple RNAME values exist:
   - Emit `MULTIPLE_SOA_RNAMES`.
   - Emit `SOA_RNAME` once per observed RNAME with associated `ns_list`.
6. Emit `TEST_CASE_END`.

## Emitted Tags (Possible Set)
| Tag | Emitted when |
| --- | --- |
| `IPV4_DISABLED` | IPv4 transport is disabled for a queried nameserver/rrtype. |
| `IPV6_DISABLED` | IPv6 transport is disabled for a queried nameserver/rrtype. |
| `MULTIPLE_SOA_RNAMES` | At least two distinct SOA RNAME values were observed. |
| `NO_RESPONSE` | SOA query had no response message from a nameserver. |
| `NO_RESPONSE_SOA_QUERY` | Response did not contain a usable SOA record for zone apex. |
| `ONE_SOA_RNAME` | Exactly one SOA RNAME value was observed. |
| `SOA_RNAME` | A specific SOA RNAME value and associated nameservers are reported. |
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
| `MULTIPLE_SOA_RNAMES` | `count` | `int` | Number of distinct RNAME values observed. |
| `NO_RESPONSE` | `ns` | `string` | Nameserver identity (`ns` name only; use `address` for IP) with no response. |
| `NO_RESPONSE` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `NO_RESPONSE_SOA_QUERY` | `ns` | `string` | Nameserver identity (`ns` name only; use `address` for IP) without usable SOA answer. |
| `NO_RESPONSE_SOA_QUERY` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `ONE_SOA_RNAME` | `rname` | `string` | The single observed SOA RNAME value. |
| `SOA_RNAME` | `rname` | `string` | One observed SOA RNAME value. |
| `SOA_RNAME` | `ns_list` | `string` | Semicolon-delimited nameserver identities (`name/ip`) serving that RNAME. |
| `TEST_CASE_END` | `testcase` | `string` | Testcase display name (`Consistency02`). |
| `TEST_CASE_START` | `testcase` | `string` | Testcase display name (`Consistency02`). |

## Severity Levels Per Tag
| Tag | Level | Notes |
| --- | --- | --- |
| `IPV4_DISABLED` | `DEBUG` | Default from `share/profile.json` (`test_levels.CONSISTENCY`). |
| `IPV6_DISABLED` | `DEBUG` | Default from `share/profile.json` (`test_levels.CONSISTENCY`). |
| `MULTIPLE_SOA_RNAMES` | `NOTICE` | Default from `share/profile.json` (`test_levels.CONSISTENCY`). |
| `NO_RESPONSE` | `DEBUG` | Default from `share/profile.json` (`test_levels.CONSISTENCY`). |
| `NO_RESPONSE_SOA_QUERY` | `DEBUG` | Default from `share/profile.json` (`test_levels.CONSISTENCY`). |
| `ONE_SOA_RNAME` | `INFO` | Default from `share/profile.json` (`test_levels.CONSISTENCY`). |
| `SOA_RNAME` | `INFO` | Default from `share/profile.json` (`test_levels.CONSISTENCY`). |
| `TEST_CASE_END` | `DEBUG` | Default from `share/profile.json` (`test_levels.CONSISTENCY`). |
| `TEST_CASE_START` | `DEBUG` | Default from `share/profile.json` (`test_levels.CONSISTENCY`). |

## Differences From Upstream
- Upstream reference: [`consistency02.md`](../../upstream/tests/Consistency-TP/consistency02.md)
- Differences (Upstream vs Gonemaster):
  - Upstream: does not explicitly define this detail. Gonemaster: emits `SOA_RNAME` detail entries for each observed RNAME value.
  - Upstream: does not explicitly define this detail. Gonemaster: Per-query transport debug tags (`IPV4_DISABLED`, `IPV6_DISABLED`) are emitted when transport is disabled.
- Potential upstream report:
  - `no`

## Edge Cases And Limitations
- If no usable SOA RNAME is obtained, neither `ONE_SOA_RNAME` nor `MULTIPLE_SOA_RNAMES` is emitted.
- `SOA_RNAME` `ns_list` ordering follows nameserver processing order.
