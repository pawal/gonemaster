# Nameserver12

Status: Final

## Purpose
- Validate behavior when querying with unknown EDNS Z flags set.

## Preconditions And Inputs
- Preconditions:
  - A `zone.Zone` object is available.
- Required inputs:
  - Nameserver addresses from [`ZoneNameservers`](../../nameserver-resolution.md#zonenameservers).
  - SOA responses to EDNS query with Z flag bits set (`Z=3`).
- Profile/config knobs that affect behavior:
  - `net.ipv4` and `net.ipv6`: disabled transports are skipped with transport debug tags.
  - `resolver.defaults.parallel`: parallel nameserver query fanout.

## Algorithm And Decision Flow
1. Emit `TEST_CASE_START`.
2. Read nameserver list from [`ZoneNameservers`](../../nameserver-resolution.md#zonenameservers).
3. For each nameserver (parallelized, input-order merged logs):
   - If transport is disabled, emit `IPV4_DISABLED` or `IPV6_DISABLED` for rrtype `SOA`, then skip.
   - Send SOA query with EDNS version `0` and `Z=3`.
   - If no response, emit `NO_RESPONSE` (`ns`, `domain`).
   - Else if `RCODE=FORMERR` and EDNS extended rcode is `0`, emit `NO_EDNS_SUPPORT`.
   - Else if response EDNS Z value is non-zero, emit `Z_FLAGS_NOTCLEAR`.
   - Else if response matches success shape (`RCODE=NOERROR`, `EdnsRcode=0`, `EdnsVersion=0`, `EdnsZ=0`, and SOA answer present), emit no finding.
   - Else emit `NS_ERROR`.
4. Emit `TEST_CASE_END`.

## Emitted Tags (Possible Set)
| Tag | Emitted when |
| --- | --- |
| `IPV4_DISABLED` | IPv4 nameserver evaluation is skipped because IPv4 is disabled. |
| `IPV6_DISABLED` | IPv6 nameserver evaluation is skipped because IPv6 is disabled. |
| `NO_EDNS_SUPPORT` | Response indicates FORMERR EDNS handling fallback path. |
| `NO_RESPONSE` | Query with Z flags set produced no DNS response. |
| `NS_ERROR` | Response did not fit expected success or explicit failure branches. |
| `TEST_CASE_END` | Testcase completion marker is emitted. |
| `TEST_CASE_START` | Testcase start marker is emitted. |
| `Z_FLAGS_NOTCLEAR` | Response EDNS Z flags were not cleared to zero. |

## Tag Arguments
| Tag | Argument key | Type | Meaning |
| --- | --- | --- | --- |
| `IPV4_DISABLED` | `ns` | `string` | Nameserver identity (`ns` name only; use `address` for IP) skipped on IPv4. |
| `IPV4_DISABLED` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `IPV4_DISABLED` | `rrtype` | `string` | rrtype skipped (`SOA`). |
| `IPV6_DISABLED` | `ns` | `string` | Nameserver identity (`ns` name only; use `address` for IP) skipped on IPv6. |
| `IPV6_DISABLED` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `IPV6_DISABLED` | `rrtype` | `string` | rrtype skipped (`SOA`). |
| `NO_EDNS_SUPPORT` | `ns` | `string` | Nameserver identity (`ns` name only; use `address` for IP) treated as no-EDNS support path. |
| `NO_EDNS_SUPPORT` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `NO_RESPONSE` | `ns` | `string` | Nameserver identity (`ns` name only; use `address` for IP) with no response. |
| `NO_RESPONSE` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `NO_RESPONSE` | `domain` | `string` | Tested zone name. |
| `NS_ERROR` | `ns` | `string` | Nameserver identity (`ns` name only; use `address` for IP) with unexpected behavior. |
| `NS_ERROR` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `TEST_CASE_END` | `testcase` | `string` | Testcase display name (`Nameserver12`). |
| `TEST_CASE_START` | `testcase` | `string` | Testcase display name (`Nameserver12`). |
| `Z_FLAGS_NOTCLEAR` | `ns` | `string` | Nameserver identity (`ns` name only; use `address` for IP) returning non-zero EDNS Z flags. |
| `Z_FLAGS_NOTCLEAR` | `address` | `string` | Nameserver IP address for the same endpoint. |

## Severity Levels Per Tag
| Tag | Level | Notes |
| --- | --- | --- |
| `IPV4_DISABLED` | `DEBUG` | Default from `share/profile.json` (`test_levels.NAMESERVER`). |
| `IPV6_DISABLED` | `DEBUG` | Default from `share/profile.json` (`test_levels.NAMESERVER`). |
| `NO_EDNS_SUPPORT` | `WARNING` | Default from `share/profile.json` (`test_levels.NAMESERVER`). |
| `NO_RESPONSE` | `DEBUG` | Default from `share/profile.json` (`test_levels.NAMESERVER`). |
| `NS_ERROR` | `WARNING` | Default from `share/profile.json` (`test_levels.NAMESERVER`). |
| `TEST_CASE_END` | `DEBUG` | Default from `share/profile.json` (`test_levels.NAMESERVER`). |
| `TEST_CASE_START` | `DEBUG` | Default from `share/profile.json` (`test_levels.NAMESERVER`). |
| `Z_FLAGS_NOTCLEAR` | `WARNING` | Default from `share/profile.json` (`test_levels.NAMESERVER`). |

## Differences From Upstream
- Differences (Upstream vs Gonemaster):
  - Upstream: describes iterating nameserver IP set. Gonemaster: iterates raw [`ZoneNameservers`](../../nameserver-resolution.md#zonenameservers) output (no testcase-local deduplication).
  - Upstream: describes ignored disabled transports in prose. Gonemaster: emits explicit `IPV4_DISABLED` / `IPV6_DISABLED` tags.
  - Upstream: does not explicitly describe testcase boundary markers. Gonemaster: emits `TEST_CASE_START` and `TEST_CASE_END`.
- Potential upstream report:
  - `no`

## Edge Cases And Limitations
- A nameserver can emit only one of `NO_EDNS_SUPPORT`, `Z_FLAGS_NOTCLEAR`, or `NS_ERROR` due branch ordering.
- `Z_FLAGS_NOTCLEAR` branch is checked before full success-shape validation.
- Query failures are reported as `NO_RESPONSE`; no retry path exists in this testcase.
