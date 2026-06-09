# Nameserver10

Status: Final

## Purpose
- Validate authoritative nameserver behavior for unsupported EDNS version queries (version 1).

## Preconditions And Inputs
- Preconditions:
  - A `zone.Zone` object is available.
- Required inputs:
  - Nameserver addresses from [`ZoneNameservers`](../../nameserver-resolution.md#zonenameservers).
  - SOA responses to EDNS version 0 and EDNS version 1 queries.
- Profile/config knobs that affect behavior:
  - `net.ipv4` and `net.ipv6`: disabled transports are skipped with transport debug tags.
  - `resolver.defaults.parallel`: parallel nameserver query fanout.

## Algorithm And Decision Flow
1. Emit `TEST_CASE_START`.
2. Initialize outcome collectors:
   - `No Response EDNS1 Query` (IP list)
   - `Unexpected RCODE` (rcode -> IP list)
   - `EDNS Response Error` (IP list)
3. Read nameserver list from [`ZoneNameservers`](../../nameserver-resolution.md#zonenameservers).
4. For each nameserver (parallelized, input-order merged logs):
   - If transport is disabled, emit `IPV4_DISABLED` or `IPV6_DISABLED` for rrtype `SOA`, then skip this nameserver.
   - Send SOA query with EDNS version 0.
   - Continue only when version-0 response exists and has `RCODE=NOERROR`.
   - Send SOA query with EDNS version 1.
   - If version-1 response is missing, mark nameserver IP for `N10_NO_RESPONSE_EDNS1_QUERY`.
   - Else determine whether response is BADVERS by either:
     - DNS header RCODE `BADVERS`, or
     - header low 4 bits `NOERROR` and EDNS extended rcode `1`.
   - If not BADVERS, mark nameserver IP under response `rcode` for `N10_UNEXPECTED_RCODE`.
   - Else if response has EDNS version `0` and empty answer section, treat as expected and do not mark issues.
   - Else mark nameserver IP for `N10_EDNS_RESPONSE_ERROR`.
5. Emit aggregate tags for non-empty collectors:
   - `N10_NO_RESPONSE_EDNS1_QUERY` with sorted unique `addresses`.
   - For each sorted `rcode`, `N10_UNEXPECTED_RCODE` with sorted unique `addresses`.
   - `N10_EDNS_RESPONSE_ERROR` with sorted unique `addresses`.
6. Emit `TEST_CASE_END`.

## Emitted Tags (Possible Set)
| Tag | Emitted when |
| --- | --- |
| `IPV4_DISABLED` | IPv4 nameserver evaluation is skipped because IPv4 is disabled. |
| `IPV6_DISABLED` | IPv6 nameserver evaluation is skipped because IPv6 is disabled. |
| `N10_EDNS_RESPONSE_ERROR` | BADVERS condition is met but response does not match expected EDNSv1 error-shape check. |
| `N10_NO_RESPONSE_EDNS1_QUERY` | Nameserver responded to EDNSv0 probe but not to EDNSv1 probe. |
| `N10_UNEXPECTED_RCODE` | EDNSv1 probe returned response with RCODE not interpreted as BADVERS. |
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
| `N10_EDNS_RESPONSE_ERROR` | `addresses` | `array<string>` | Structured sorted unique nameserver IPs. |
| `N10_NO_RESPONSE_EDNS1_QUERY` | `addresses` | `array<string>` | Structured sorted unique nameserver IPs. |
| `N10_UNEXPECTED_RCODE` | `rcode` | `string` | Unexpected response code for EDNSv1 query. |
| `N10_UNEXPECTED_RCODE` | `addresses` | `array<string>` | Structured sorted unique nameserver IPs for that rcode. |
| `TEST_CASE_END` | `testcase` | `string` | Testcase display name (`Nameserver10`). |
| `TEST_CASE_START` | `testcase` | `string` | Testcase display name (`Nameserver10`). |

## Severity Levels Per Tag
| Tag | Level | Notes |
| --- | --- | --- |
| `IPV4_DISABLED` | `DEBUG` | Default from `share/profile.json` (`test_levels.NAMESERVER`). |
| `IPV6_DISABLED` | `DEBUG` | Default from `share/profile.json` (`test_levels.NAMESERVER`). |
| `N10_EDNS_RESPONSE_ERROR` | `WARNING` | Default from `share/profile.json` (`test_levels.NAMESERVER`). |
| `N10_NO_RESPONSE_EDNS1_QUERY` | `WARNING` | Default from `share/profile.json` (`test_levels.NAMESERVER`). |
| `N10_UNEXPECTED_RCODE` | `WARNING` | Default from `share/profile.json` (`test_levels.NAMESERVER`). |
| `TEST_CASE_END` | `DEBUG` | Default from `share/profile.json` (`test_levels.NAMESERVER`). |
| `TEST_CASE_START` | `DEBUG` | Default from `share/profile.json` (`test_levels.NAMESERVER`). |

## Differences From Upstream
- Differences (Upstream vs Gonemaster):
  - Upstream: says input is nameserver IP set. Gonemaster: iterates raw [`ZoneNameservers`](../../nameserver-resolution.md#zonenameservers) output, but aggregate `addresses` values are sorted and deduplicated by IP.
  - Upstream: summary assumes this testcase is relevant only after EDNSv0 success. Gonemaster: implements that gating explicitly by only evaluating EDNSv1 when EDNSv0 response exists and has `NOERROR`.
  - Upstream: does not explicitly describe testcase boundary and transport-disabled debug emissions. Gonemaster: emits `TEST_CASE_START`, `TEST_CASE_END`, `IPV4_DISABLED`, and `IPV6_DISABLED`.
- Potential upstream report:
  - `no`

## Edge Cases And Limitations
- Nameservers failing the EDNSv0 gating query produce no N10 finding tags.
- BADVERS detection accepts two encodings (header `BADVERS` or extended-rcode form).
- Aggregate tags are emitted once per collector category, not once per nameserver.
