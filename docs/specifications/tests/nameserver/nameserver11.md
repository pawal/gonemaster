# Nameserver11 (nameserver11)

Status: Final

## Purpose
- Verify handling of an unknown EDNS option code in authoritative SOA responses.

## Preconditions And Inputs
- Preconditions:
  - A `zone.Zone` object is available.
- Required inputs:
  - Nameserver addresses from `methods.Method4and5`.
  - Baseline EDNSv0 SOA responses.
  - SOA responses to EDNS query with unknown option code (`137`).
- Profile/config knobs that affect behavior:
  - `net.ipv4` and `net.ipv6`: disabled transports are skipped with transport debug tags.
  - `resolver.defaults.parallel`: parallel nameserver query fanout.

## Algorithm And Decision Flow
1. Emit `TEST_CASE_START`.
2. Read nameserver list from `Method4and5`.
3. For each nameserver (parallelized, input-order merged logs):
   - If transport is disabled, emit `IPV4_DISABLED` or `IPV6_DISABLED` for rrtype `SOA`, then skip.
   - Send baseline SOA query with EDNS version `0` and no unknown option.
   - Continue to unknown-option test only if baseline response satisfies all:
     - response exists,
     - EDNS present,
     - `RCODE=NOERROR`,
     - `AA=true`,
     - answer contains SOA for zone name.
   - Send SOA query with EDNS version `0` and unknown option code `137`.
   - Classify outcome into one collector:
     - `N11_NO_RESPONSE` when no response.
     - `N11_UNEXPECTED_RCODE` when `RCODE!=NOERROR` (bucketed by `rcode`).
     - `N11_NO_EDNS` when EDNS is absent.
     - `N11_UNEXPECTED_ANSWER_SECTION` when SOA answer for zone name is absent.
     - `N11_UNSET_AA` when `AA=false`.
     - `N11_RETURNS_UNKNOWN_OPTION_CODE` when response EDNS data still includes option code `137`.
     - no finding when none of the above applies.
4. Emit aggregate tags for non-empty collectors (sorted unique `addresses` values; `N11_UNEXPECTED_RCODE` emitted per sorted `rcode`).
5. Emit `TEST_CASE_END`.

## Emitted Tags (Possible Set)
| Tag | Emitted when |
| --- | --- |
| `IPV4_DISABLED` | IPv4 nameserver evaluation is skipped because IPv4 is disabled. |
| `IPV6_DISABLED` | IPv6 nameserver evaluation is skipped because IPv6 is disabled. |
| `N11_NO_EDNS` | Unknown-option query response lacked EDNS. |
| `N11_NO_RESPONSE` | Unknown-option query produced no DNS response. |
| `N11_RETURNS_UNKNOWN_OPTION_CODE` | Unknown EDNS option code `137` was echoed back in response EDNS options. |
| `N11_UNEXPECTED_ANSWER_SECTION` | Unknown-option query response did not include expected zone SOA answer. |
| `N11_UNEXPECTED_RCODE` | Unknown-option query response had non-`NOERROR` RCODE. |
| `N11_UNSET_AA` | Unknown-option query response was not authoritative. |
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
| `N11_NO_EDNS` | `addresses` | `array<string>` | Structured sorted unique nameserver IPs. |
| `N11_NO_RESPONSE` | `addresses` | `array<string>` | Structured sorted unique nameserver IPs. |
| `N11_RETURNS_UNKNOWN_OPTION_CODE` | `addresses` | `array<string>` | Structured sorted unique nameserver IPs. |
| `N11_UNEXPECTED_ANSWER_SECTION` | `addresses` | `array<string>` | Structured sorted unique nameserver IPs. |
| `N11_UNEXPECTED_RCODE` | `rcode` | `string` | Unexpected response code name. |
| `N11_UNEXPECTED_RCODE` | `addresses` | `array<string>` | Structured sorted unique nameserver IPs for that rcode. |
| `N11_UNSET_AA` | `addresses` | `array<string>` | Structured sorted unique nameserver IPs. |
| `TEST_CASE_END` | `testcase` | `string` | Testcase display name (`Nameserver11`). |
| `TEST_CASE_START` | `testcase` | `string` | Testcase display name (`Nameserver11`). |

## Severity Levels Per Tag
| Tag | Level | Notes |
| --- | --- | --- |
| `IPV4_DISABLED` | `DEBUG` | Default from `share/profile.json` (`test_levels.NAMESERVER`). |
| `IPV6_DISABLED` | `DEBUG` | Default from `share/profile.json` (`test_levels.NAMESERVER`). |
| `N11_NO_EDNS` | `WARNING` | Default from `share/profile.json` (`test_levels.NAMESERVER`). |
| `N11_NO_RESPONSE` | `WARNING` | Default from `share/profile.json` (`test_levels.NAMESERVER`). |
| `N11_RETURNS_UNKNOWN_OPTION_CODE` | `WARNING` | Default from `share/profile.json` (`test_levels.NAMESERVER`). |
| `N11_UNEXPECTED_ANSWER_SECTION` | `WARNING` | Default from `share/profile.json` (`test_levels.NAMESERVER`). |
| `N11_UNEXPECTED_RCODE` | `WARNING` | Default from `share/profile.json` (`test_levels.NAMESERVER`). |
| `N11_UNSET_AA` | `WARNING` | Default from `share/profile.json` (`test_levels.NAMESERVER`). |
| `TEST_CASE_END` | `DEBUG` | Default from `share/profile.json` (`test_levels.NAMESERVER`). |
| `TEST_CASE_START` | `DEBUG` | Default from `share/profile.json` (`test_levels.NAMESERVER`). |

## Differences From Upstream
- Upstream reference: [`nameserver11.md`](../../upstream/tests/Nameserver-TP/nameserver11.md)
- Differences (Upstream vs Gonemaster):
  - Upstream: assumes nameserver IP evaluation set. Gonemaster: iterates raw `Method4and5` output, then emits deduplicated/sorted `addresses` aggregates.
  - Upstream: defines baseline gate before unknown-option probe. Gonemaster: implements this gate exactly and silently skips nameservers failing baseline checks.
  - Upstream: does not explicitly describe testcase boundary and transport-disabled debug emissions. Gonemaster: emits `TEST_CASE_START`, `TEST_CASE_END`, `IPV4_DISABLED`, and `IPV6_DISABLED`.
- Potential upstream report:
  - `no`

## Edge Cases And Limitations
- Nameservers that fail baseline validation emit no `N11_*` findings.
- Multiple failures for one nameserver cannot be reported simultaneously; classification is first-match by branch order.
- Unknown option detection only checks for option code `137` in response EDNS options.
