# Nameserver16 (nameserver16)

Status: Final

## Purpose
- Query authoritative nameservers with an EDNS NSID option request (RFC 5001, option code 3) and report which servers provide NSID values and what those values contain.

## Preconditions And Inputs
- Preconditions:
  - A `zone.Zone` object is available.
- Required inputs:
  - Nameserver addresses from `methods.Method4and5`.
  - SOA responses to EDNS query carrying NSID option (option code `3`, empty value).
- Profile/config knobs that affect behavior:
  - `net.ipv4` and `net.ipv6`: disabled transports are skipped with transport debug tags.
  - `resolver.defaults.parallel`: parallel nameserver query fanout.

## Algorithm And Decision Flow
1. Emit `TEST_CASE_START`.
2. Initialize collectors:
   - NSID string data by `nsid` value
   - no-NSID-revealed nameserver set
   - no-response nameserver set
   - unexpected-RCODE nameserver set by `rcode`
3. Read nameserver list from `Method4and5`.
4. For each nameserver (parallelized, input-order merged logs):
   - If transport is disabled, emit `IPV4_DISABLED` or `IPV6_DISABLED` for rrtype `SOA`, then skip.
   - Send SOA query for zone name with EDNS version `0` and NSID option (option code `3`, empty payload).
   - If no response, collect nameserver for `N16_NO_RESPONSE`.
   - Else if `RCODE != NOERROR`, collect nameserver for `N16_UNEXPECTED_RCODE` under the observed `rcode` value.
   - Else if response EDNS options include NSID (option code `3`), extract the NSID payload as a printable string (non-UTF-8 bytes are hex-escaped), trim leading and trailing whitespace, and collect `(nsid, nameserver)` for `N16_HAS_NSID`.
   - Else collect nameserver for `N16_NO_NSID_REVEALED`.
5. Emit `N16_HAS_NSID` for each unique `nsid` value with sorted unique `servers`.
6. Emit `N16_NO_NSID_REVEALED` with sorted unique `servers` when non-empty.
7. Emit `N16_NO_RESPONSE` with sorted unique `servers` when non-empty.
8. Emit `N16_UNEXPECTED_RCODE` per `rcode` with sorted unique `servers` when non-empty.
9. Emit `TEST_CASE_END`.

## Emitted Tags (Possible Set)
| Tag | Emitted when |
| --- | --- |
| `IPV4_DISABLED` | IPv4 nameserver evaluation is skipped because IPv4 is disabled. |
| `IPV6_DISABLED` | IPv6 nameserver evaluation is skipped because IPv6 is disabled. |
| `N16_HAS_NSID` | Server included NSID option in response; reports the NSID value and nameservers returning it. |
| `N16_NO_NSID_REVEALED` | Server responded but did not include NSID option in response. |
| `N16_NO_RESPONSE` | NSID query produced no DNS response. |
| `N16_UNEXPECTED_RCODE` | NSID query response had non-`NOERROR` RCODE. |
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
| `N16_HAS_NSID` | `nsid` | `string` | NSID payload as a trimmed printable string (non-UTF-8 bytes hex-escaped). |
| `N16_HAS_NSID` | `servers` | `array<object>` | Structured sorted unique nameserver identities (`{ns,address}` object) returning this NSID value. |
| `N16_NO_NSID_REVEALED` | `servers` | `array<object>` | Structured sorted unique nameserver identities (`{ns,address}` object). |
| `N16_NO_RESPONSE` | `servers` | `array<object>` | Structured sorted unique nameserver identities (`{ns,address}` object). |
| `N16_UNEXPECTED_RCODE` | `rcode` | `string` | Unexpected response code name. |
| `N16_UNEXPECTED_RCODE` | `servers` | `array<object>` | Structured sorted unique nameserver identities (`{ns,address}` object) for that rcode. |
| `TEST_CASE_END` | `testcase` | `string` | Testcase display name (`Nameserver16`). |
| `TEST_CASE_START` | `testcase` | `string` | Testcase display name (`Nameserver16`). |

## Severity Levels Per Tag
| Tag | Level | Notes |
| --- | --- | --- |
| `IPV4_DISABLED` | `DEBUG` | Default from `share/profile.json` (`test_levels.NAMESERVER`). |
| `IPV6_DISABLED` | `DEBUG` | Default from `share/profile.json` (`test_levels.NAMESERVER`). |
| `N16_HAS_NSID` | `NOTICE` | Default from `share/profile.json` (`test_levels.NAMESERVER`). |
| `N16_NO_NSID_REVEALED` | `INFO` | Default from `share/profile.json` (`test_levels.NAMESERVER`). |
| `N16_NO_RESPONSE` | `WARNING` | Default from `share/profile.json` (`test_levels.NAMESERVER`). |
| `N16_UNEXPECTED_RCODE` | `WARNING` | Default from `share/profile.json` (`test_levels.NAMESERVER`). |
| `TEST_CASE_END` | `DEBUG` | Default from `share/profile.json` (`test_levels.NAMESERVER`). |
| `TEST_CASE_START` | `DEBUG` | Default from `share/profile.json` (`test_levels.NAMESERVER`). |

## Differences From Upstream
- Upstream reference: none (Gonemaster extension — no corresponding Zonemaster testcase exists).

## Edge Cases And Limitations
- A server that returns a non-`NOERROR` RCODE for an NSID-carrying query is not separately tested for basic SOA reachability; `N16_UNEXPECTED_RCODE` covers this outcome.
- A nameserver can appear in both `N16_HAS_NSID` and `N16_NO_NSID_REVEALED` only across different transport addresses if they disagree; within a single address it falls into exactly one collector.
- NSID payloads that are empty after extraction are treated as no-NSID-revealed, not as a distinct zero-length NSID.
- Non-UTF-8 NSID bytes are hex-escaped to ensure the `nsid` argument is always a printable string.
