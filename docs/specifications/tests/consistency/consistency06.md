# Consistency06 (consistency06)

Status: Final

## Purpose
- Check SOA MNAME consistency across nameservers for the tested zone.

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
   - Otherwise extract lowercase SOA MNAME and store it for that nameserver.
4. If exactly one MNAME value exists, emit `ONE_SOA_MNAME`.
5. If multiple MNAME values exist:
   - Emit `MULTIPLE_SOA_MNAMES`.
   - Emit `SOA_MNAME` once per observed MNAME with associated `ns_list`.
6. Emit `TEST_CASE_END`.

## Emitted Tags (Possible Set)
| Tag | Emitted when |
| --- | --- |
| `IPV4_DISABLED` | IPv4 transport is disabled for a queried nameserver/rrtype. |
| `IPV6_DISABLED` | IPv6 transport is disabled for a queried nameserver/rrtype. |
| `MULTIPLE_SOA_MNAMES` | At least two distinct SOA MNAME values were observed. |
| `NO_RESPONSE` | SOA query had no response message from a nameserver. |
| `NO_RESPONSE_SOA_QUERY` | Response did not contain a usable SOA record for zone apex. |
| `ONE_SOA_MNAME` | Exactly one SOA MNAME value was observed. |
| `SOA_MNAME` | A specific SOA MNAME value and associated nameservers are reported. |
| `TEST_CASE_END` | Testcase completion marker is emitted. |
| `TEST_CASE_START` | Testcase start marker is emitted. |

## Tag Arguments
| Tag | Argument key | Type | Meaning |
| --- | --- | --- | --- |
| `IPV4_DISABLED` | `ns` | `string` | Nameserver identity (`ns` name only; use `address` for IP) skipped on IPv4. |
| `IPV4_DISABLED` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `IPV4_DISABLED` | `arg_schema` | `string` | Schema identifier (`gonemaster.logargs/1.1`) for coherent log arguments. |
| `IPV4_DISABLED` | `rrtype` | `string` | rrtype skipped (`SOA`). |
| `IPV6_DISABLED` | `ns` | `string` | Nameserver identity (`ns` name only; use `address` for IP) skipped on IPv6. |
| `IPV6_DISABLED` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `IPV6_DISABLED` | `arg_schema` | `string` | Schema identifier (`gonemaster.logargs/1.1`) for coherent log arguments. |
| `IPV6_DISABLED` | `rrtype` | `string` | rrtype skipped (`SOA`). |
| `MULTIPLE_SOA_MNAMES` | `count` | `int` | Number of distinct MNAME values observed. |
| `NO_RESPONSE` | `ns` | `string` | Nameserver identity (`ns` name only; use `address` for IP) with no response. |
| `NO_RESPONSE` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `NO_RESPONSE` | `arg_schema` | `string` | Schema identifier (`gonemaster.logargs/1.1`) for coherent log arguments. |
| `NO_RESPONSE_SOA_QUERY` | `ns` | `string` | Nameserver identity (`ns` name only; use `address` for IP) without usable SOA answer. |
| `NO_RESPONSE_SOA_QUERY` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `NO_RESPONSE_SOA_QUERY` | `arg_schema` | `string` | Schema identifier (`gonemaster.logargs/1.1`) for coherent log arguments. |
| `ONE_SOA_MNAME` | `mname` | `string` | The single observed SOA MNAME value. |
| `SOA_MNAME` | `mname` | `string` | One observed SOA MNAME value. |
| `SOA_MNAME` | `ns_list` | `string` | Semicolon-delimited nameserver identities (`name/ip`) serving that MNAME. |
| `TEST_CASE_END` | `testcase` | `string` | Testcase display name (`Consistency06`). |
| `TEST_CASE_START` | `testcase` | `string` | Testcase display name (`Consistency06`). |

## Severity Levels Per Tag
| Tag | Level | Notes |
| --- | --- | --- |
| `IPV4_DISABLED` | `DEBUG` | Default from `share/profile.json` (`test_levels.CONSISTENCY`). |
| `IPV6_DISABLED` | `DEBUG` | Default from `share/profile.json` (`test_levels.CONSISTENCY`). |
| `MULTIPLE_SOA_MNAMES` | `NOTICE` | Default from `share/profile.json` (`test_levels.CONSISTENCY`). |
| `NO_RESPONSE` | `DEBUG` | Default from `share/profile.json` (`test_levels.CONSISTENCY`). |
| `NO_RESPONSE_SOA_QUERY` | `DEBUG` | Default from `share/profile.json` (`test_levels.CONSISTENCY`). |
| `ONE_SOA_MNAME` | `INFO` | Default from `share/profile.json` (`test_levels.CONSISTENCY`). |
| `SOA_MNAME` | `INFO` | Default from `share/profile.json` (`test_levels.CONSISTENCY`). |
| `TEST_CASE_END` | `DEBUG` | Default from `share/profile.json` (`test_levels.CONSISTENCY`). |
| `TEST_CASE_START` | `DEBUG` | Default from `share/profile.json` (`test_levels.CONSISTENCY`). |

## Differences From Upstream
- Upstream reference: [`consistency06.md`](../../upstream/tests/Consistency-TP/consistency06.md)
- Differences (Upstream vs Gonemaster):
  - Upstream: does not explicitly define this detail. Gonemaster: emits `SOA_MNAME` detail entries for each observed MNAME value.
  - Upstream: does not explicitly define this detail. Gonemaster: Per-query transport debug tags (`IPV4_DISABLED`, `IPV6_DISABLED`) are emitted when transport is disabled.
- Potential upstream report:
  - `no`

## Edge Cases And Limitations
- If no usable SOA MNAME is obtained, neither `ONE_SOA_MNAME` nor `MULTIPLE_SOA_MNAMES` is emitted.
- `SOA_MNAME` `ns_list` ordering follows nameserver processing order.
