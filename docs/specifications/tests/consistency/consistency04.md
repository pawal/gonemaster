# Consistency04

Status: Final

## Purpose
- Check NS RRset consistency across nameservers for the tested zone.
- Detect when the apex NS RRset is served with inconsistent TTLs across nameservers.

## Preconditions And Inputs
- Preconditions:
  - A `zone.Zone` object is available.
- Required inputs:
  - Nameserver list from [`GlueNameservers`](../../nameserver-resolution.md#gluenameservers) and [`ApexNameservers`](../../nameserver-resolution.md#apexnameservers).
  - NS answers from queried nameservers.
- Profile/config knobs that affect behavior:
  - `net.ipv4` and `net.ipv6`: disabled transports are skipped per nameserver.
  - `resolver.defaults.parallel`: per-nameserver query task parallelism.

## Algorithm And Decision Flow
1. Emit `TEST_CASE_START`.
2. Build deduplicated nameserver list from the union of [`GlueNameservers`](../../nameserver-resolution.md#gluenameservers) and [`ApexNameservers`](../../nameserver-resolution.md#apexnameservers) by `ns.String()`.
3. For each nameserver (parallelized):
   - If transport is disabled, emit `IPV4_DISABLED` or `IPV6_DISABLED` for rrtype `NS` and skip.
   - Query NS for zone apex.
   - No response message -> emit `NO_RESPONSE`.
   - Response without usable NS records for zone apex -> emit `NO_RESPONSE_NS_QUERY`.
   - Otherwise extract lowercase NS targets, sort them, and store as one NS-set key for that nameserver, and record the NS RRset TTL (minimum of the NS record TTLs) for that nameserver.
4. If exactly one NS-set key exists, emit `ONE_NS_SET`.
5. If multiple NS-set keys exist:
   - Emit `MULTIPLE_NS_SET`.
   - Emit `NS_SET` once per NS-set key with the NS target set (`ns_set_servers`) and contributing nameserver endpoints (`servers`).
6. If more than one distinct apex NS RRset TTL was observed across nameservers, emit `INCONSISTENT_NS_TTL` with the distinct-value `count` and the `ttl_min`/`ttl_max` bounds.
7. Emit `TEST_CASE_END`.

## Emitted Tags (Possible Set)
| Tag | Emitted when |
| --- | --- |
| `INCONSISTENT_NS_TTL` | Authoritative nameservers serve the apex NS RRset with more than one distinct TTL. |
| `IPV4_DISABLED` | IPv4 transport is disabled for a queried nameserver/rrtype. |
| `IPV6_DISABLED` | IPv6 transport is disabled for a queried nameserver/rrtype. |
| `MULTIPLE_NS_SET` | At least two distinct NS target sets were observed. |
| `NO_RESPONSE` | NS query had no response message from a nameserver. |
| `NO_RESPONSE_NS_QUERY` | Response did not contain usable NS records for zone apex. |
| `NS_SET` | A specific NS target set and associated nameservers are reported. |
| `ONE_NS_SET` | Exactly one NS target set was observed. |
| `TEST_CASE_END` | Testcase completion marker is emitted. |
| `TEST_CASE_START` | Testcase start marker is emitted. |

## Tag Arguments
| Tag | Argument key | Type | Meaning |
| --- | --- | --- | --- |
| `INCONSISTENT_NS_TTL` | `count` | `int` | Number of distinct apex NS RRset TTLs observed across nameservers. |
| `INCONSISTENT_NS_TTL` | `ttl_min` | `int` | Smallest observed apex NS RRset TTL (seconds). |
| `INCONSISTENT_NS_TTL` | `ttl_max` | `int` | Largest observed apex NS RRset TTL (seconds). |
| `IPV4_DISABLED` | `ns` | `string` | Nameserver identity (`ns` name only; use `address` for IP) skipped on IPv4. |
| `IPV4_DISABLED` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `IPV4_DISABLED` | `rrtype` | `string` | rrtype skipped (`NS`). |
| `IPV6_DISABLED` | `ns` | `string` | Nameserver identity (`ns` name only; use `address` for IP) skipped on IPv6. |
| `IPV6_DISABLED` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `IPV6_DISABLED` | `rrtype` | `string` | rrtype skipped (`NS`). |
| `MULTIPLE_NS_SET` | `count` | `int` | Number of distinct NS target sets observed. |
| `NO_RESPONSE` | `ns` | `string` | Nameserver identity (`ns` name only; use `address` for IP) with no response. |
| `NO_RESPONSE` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `NO_RESPONSE_NS_QUERY` | `ns` | `string` | Nameserver identity (`ns` name only; use `address` for IP) without usable NS answer. |
| `NO_RESPONSE_NS_QUERY` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `NS_SET` | `ns_set_servers` | `array<object>` | Structured NS target names in this set as `{ns}` items. |
| `NS_SET` | `servers` | `array<object>` | Structured nameserver identities (`{ns,address}` object) returning this set. |
| `ONE_NS_SET` | `servers` | `array<object>` | Structured single observed NS target set as `{ns}` items. |
| `TEST_CASE_END` | `testcase` | `string` | Testcase display name (`Consistency04`). |
| `TEST_CASE_START` | `testcase` | `string` | Testcase display name (`Consistency04`). |

## Severity Levels Per Tag
| Tag | Level | Notes |
| --- | --- | --- |
| `INCONSISTENT_NS_TTL` | `NOTICE` | Default from `share/profile.json` (`test_levels.CONSISTENCY`). |
| `IPV4_DISABLED` | `DEBUG` | Default from `share/profile.json` (`test_levels.CONSISTENCY`). |
| `IPV6_DISABLED` | `DEBUG` | Default from `share/profile.json` (`test_levels.CONSISTENCY`). |
| `MULTIPLE_NS_SET` | `NOTICE` | Default from `share/profile.json` (`test_levels.CONSISTENCY`). |
| `NO_RESPONSE` | `DEBUG` | Default from `share/profile.json` (`test_levels.CONSISTENCY`). |
| `NO_RESPONSE_NS_QUERY` | `DEBUG` | Default from `share/profile.json` (`test_levels.CONSISTENCY`). |
| `NS_SET` | `INFO` | Default from `share/profile.json` (`test_levels.CONSISTENCY`). |
| `ONE_NS_SET` | `INFO` | Default from `share/profile.json` (`test_levels.CONSISTENCY`). |
| `TEST_CASE_END` | `DEBUG` | Default from `share/profile.json` (`test_levels.CONSISTENCY`). |
| `TEST_CASE_START` | `DEBUG` | Default from `share/profile.json` (`test_levels.CONSISTENCY`). |

## Differences From Upstream
- Differences (Upstream vs Gonemaster):
  - NS-set equality is based on sorted NS target names only. The upstream `consistency04` specification text additionally lists TTL as part of NS-set equality, but the upstream implementation compares NS target names only and ignores TTL; Gonemaster keeps name-set equality name-only and reports differing apex NS RRset TTLs separately via `INCONSISTENT_NS_TTL` (`NOTICE`).
  - Upstream: does not explicitly define this detail. Gonemaster: Per-query transport debug tags (`IPV4_DISABLED`, `IPV6_DISABLED`) are emitted when transport is disabled.
- Potential upstream report:
  - `yes` (reported upstream: the upstream `consistency04` spec lists TTL as part of NS-set equality, but the implementation never compares the RR TTL).

## Edge Cases And Limitations
- If no usable NS set is obtained, neither `ONE_NS_SET` nor `MULTIPLE_NS_SET` is emitted.
- `servers` ordering in `NS_SET` follows nameserver processing order.
