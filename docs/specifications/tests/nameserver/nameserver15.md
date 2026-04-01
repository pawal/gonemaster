# Nameserver15 (nameserver15)

Status: Final

## Purpose
- Detect whether authoritative nameservers reveal software version data through CHAOS-class TXT queries.

## Preconditions And Inputs
- Preconditions:
  - A `zone.Zone` object is available.
- Required inputs:
  - Nameserver addresses from `methods.Method4and5`.
  - Baseline SOA responses for zone name.
  - TXT/CH responses for query names `version.bind` and `version.server`.
- Profile/config knobs that affect behavior:
  - `net.ipv4` and `net.ipv6`: disabled transports are skipped with transport debug tags.
  - `resolver.defaults.parallel`: parallel nameserver query fanout.

## Algorithm And Decision Flow
1. Emit `TEST_CASE_START`.
2. Initialize collectors:
   - text data by `string` and `query_name`
   - error-on-version-query by `query_name`
   - no-version-revealed nameserver set
   - wrong-class nameserver set
3. Read nameserver list from `Method4and5`.
4. For each nameserver (parallelized, input-order merged logs):
   - If transport is disabled, emit `IPV4_DISABLED` or `IPV6_DISABLED` for rrtype `SOA TXT`, then skip.
   - Send baseline SOA query for zone; if no response, skip nameserver.
   - For each query name in `{version.bind, version.server}`:
     - Send TXT query with class `CH`.
     - If query fails, has no response, or returns `SERVFAIL`, mark error for that query name.
     - Else extract TXT RRs whose owner name matches query name.
     - For each TXT RR:
       - If RR class is not CHAOS, mark wrong-class for nameserver.
       - Concatenate TXT strings, trim leading/trailing whitespace.
       - If resulting string is non-empty, store `(string, query_name, nameserver)` as revealed data.
   - If no non-empty string was revealed for this nameserver, mark nameserver for no-version-revealed.
5. Emit `N15_SOFTWARE_VERSION` for each unique `(string, query_name)` pair with sorted unique `servers`.
6. Emit `N15_ERROR_ON_VERSION_QUERY` per query name with sorted unique `servers`.
7. Emit `N15_NO_VERSION_REVEALED` with sorted unique nameserver list when non-empty.
8. Emit `N15_WRONG_CLASS` with sorted unique nameserver list when non-empty.
9. Emit `TEST_CASE_END`.

## Emitted Tags (Possible Set)
| Tag | Emitted when |
| --- | --- |
| `IPV4_DISABLED` | IPv4 nameserver evaluation is skipped because IPv4 is disabled. |
| `IPV6_DISABLED` | IPv6 nameserver evaluation is skipped because IPv6 is disabled. |
| `N15_ERROR_ON_VERSION_QUERY` | TXT/CH query failed, had no response, or returned SERVFAIL for a version query name. |
| `N15_NO_VERSION_REVEALED` | Nameserver passed baseline SOA probe but revealed no non-empty version string. |
| `N15_SOFTWARE_VERSION` | Non-empty version string was returned for a version query name. |
| `N15_WRONG_CLASS` | TXT answer RR class was not CHAOS (`CH`) on a CH query. |
| `TEST_CASE_END` | Testcase completion marker is emitted. |
| `TEST_CASE_START` | Testcase start marker is emitted. |

## Tag Arguments
| Tag | Argument key | Type | Meaning |
| --- | --- | --- | --- |
| `IPV4_DISABLED` | `ns` | `string` | Nameserver identity (`ns` name only; use `address` for IP) skipped on IPv4. |
| `IPV4_DISABLED` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `IPV4_DISABLED` | `rrtype` | `string` | rrtype skipped (`SOA TXT`). |
| `IPV6_DISABLED` | `ns` | `string` | Nameserver identity (`ns` name only; use `address` for IP) skipped on IPv6. |
| `IPV6_DISABLED` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `IPV6_DISABLED` | `rrtype` | `string` | rrtype skipped (`SOA TXT`). |
| `N15_ERROR_ON_VERSION_QUERY` | `query_name` | `string` | Version query name (`version.bind` or `version.server`). |
| `N15_ERROR_ON_VERSION_QUERY` | `servers` | `array<object>` | Structured sorted unique nameserver identities (`{ns,address}` object). |
| `N15_NO_VERSION_REVEALED` | `servers` | `array<object>` | Structured sorted unique nameserver identities (`{ns,address}` object). |
| `N15_SOFTWARE_VERSION` | `string` | `string` | Revealed trimmed software/version string. |
| `N15_SOFTWARE_VERSION` | `query_name` | `string` | Query name producing the string (`version.bind` or `version.server`). |
| `N15_SOFTWARE_VERSION` | `servers` | `array<object>` | Structured sorted unique nameserver identities (`{ns,address}` object). |
| `N15_WRONG_CLASS` | `servers` | `array<object>` | Structured sorted unique nameserver identities (`{ns,address}` object). |
| `TEST_CASE_END` | `testcase` | `string` | Testcase display name (`Nameserver15`). |
| `TEST_CASE_START` | `testcase` | `string` | Testcase display name (`Nameserver15`). |

## Severity Levels Per Tag
| Tag | Level | Notes |
| --- | --- | --- |
| `IPV4_DISABLED` | `DEBUG` | Default from `share/profile.json` (`test_levels.NAMESERVER`). |
| `IPV6_DISABLED` | `DEBUG` | Default from `share/profile.json` (`test_levels.NAMESERVER`). |
| `N15_ERROR_ON_VERSION_QUERY` | `NOTICE` | Default from `share/profile.json` (`test_levels.NAMESERVER`). |
| `N15_NO_VERSION_REVEALED` | `INFO` | Default from `share/profile.json` (`test_levels.NAMESERVER`). |
| `N15_SOFTWARE_VERSION` | `NOTICE` | Default from `share/profile.json` (`test_levels.NAMESERVER`). |
| `N15_WRONG_CLASS` | `WARNING` | Default from `share/profile.json` (`test_levels.NAMESERVER`). |
| `TEST_CASE_END` | `DEBUG` | Default from `share/profile.json` (`test_levels.NAMESERVER`). |
| `TEST_CASE_START` | `DEBUG` | Default from `share/profile.json` (`test_levels.NAMESERVER`). |

## Differences From Upstream
- Upstream reference: [`nameserver15.md`](../../upstream/tests/Nameserver-TP/nameserver15.md)
- Differences (Upstream vs Gonemaster):
  - Upstream: describes a conceptual "Sending Version Query" set then removal based on TXT data. Gonemaster: computes equivalent behavior via per-server `noVersion` state (set only when no non-empty version string was revealed).
  - Upstream: states nameserver IP collection semantics. Gonemaster: iterates raw `Method4and5` output and emits deduplicated sorted `servers` aggregates.
  - Upstream: does not explicitly describe testcase boundary and transport-disabled debug emissions. Gonemaster: emits `TEST_CASE_START`, `TEST_CASE_END`, `IPV4_DISABLED`, and `IPV6_DISABLED`.
- Potential upstream report:
  - `no`

## Edge Cases And Limitations
- Nameservers with no baseline SOA response are silently ignored for `N15_*` findings.
- A nameserver can appear in both `N15_ERROR_ON_VERSION_QUERY` and `N15_SOFTWARE_VERSION` (different query names).
- Empty or whitespace-only TXT strings are ignored for software-version revelation.
