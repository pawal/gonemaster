# Nameserver01 (nameserver01)

Status: Draft

## Purpose
- Detect whether authoritative nameservers also behave as recursors.

## Preconditions And Inputs
- Preconditions:
  - A `zone.Zone` object is available.
- Required inputs:
  - Nameserver addresses from `methods.Method4and5`.
  - Three hardcoded probe names:
    - `xn--nameservertest.iis.se`
    - `xn--nameservertest.icann.org`
    - `xn--nameservertest.ripe.net`
  - `A` query responses from each nameserver.
- Profile/config knobs that affect behavior:
  - `net.ipv4` and `net.ipv6`: disabled transports are skipped with transport debug tags.
  - `resolver.defaults.parallel`: parallel nameserver query fanout.

## Algorithm And Decision Flow
1. Emit `TEST_CASE_START`.
2. Read nameserver list from `Method4and5`.
3. For each nameserver (parallelized, input-order merged logs):
   - If transport is disabled, emit `IPV4_DISABLED` or `IPV6_DISABLED` for rrtype `A`, then skip this nameserver.
   - Initialize counters: `responseCount`, `nxdomainCount`, `hasSeenRA`, and `isNoRecursor=true`.
   - For each probe name:
     - Query `A`.
     - If no DNS message is returned, emit `NO_RESPONSE` (`ns`, `domain`), set `isNoRecursor=false`, and continue.
     - Increment `responseCount`.
     - If response has `RA=1`, set `hasSeenRA=true`.
     - If response `RCODE` is `NXDOMAIN`, increment `nxdomainCount`.
   - If `hasSeenRA=true` or (`responseCount>0` and `nxdomainCount==responseCount`), emit `IS_A_RECURSOR` and set `isNoRecursor=false`.
   - If `isNoRecursor` is still true, emit `NO_RECURSOR`.
4. Emit `TEST_CASE_END`.

## Emitted Tags (Possible Set)
| Tag | Emitted when |
| --- | --- |
| `IPV4_DISABLED` | IPv4 nameserver evaluation is skipped because IPv4 is disabled. |
| `IPV6_DISABLED` | IPv6 nameserver evaluation is skipped because IPv6 is disabled. |
| `IS_A_RECURSOR` | Nameserver set `RA=1` on at least one probe response, or all received probe responses were `NXDOMAIN`. |
| `NO_RECURSOR` | Nameserver produced responses but did not match recursor criteria and had no `NO_RESPONSE` for probes. |
| `NO_RESPONSE` | A probe query returned no DNS message. |
| `TEST_CASE_END` | Testcase completion marker is emitted. |
| `TEST_CASE_START` | Testcase start marker is emitted. |

## Tag Arguments
| Tag | Argument key | Type | Meaning |
| --- | --- | --- | --- |
| `IPV4_DISABLED` | `ns` | `string` | Nameserver identity (`name/ip`) skipped on IPv4. |
| `IPV4_DISABLED` | `rrtype` | `string` | rrtype skipped (`A`). |
| `IPV6_DISABLED` | `ns` | `string` | Nameserver identity (`name/ip`) skipped on IPv6. |
| `IPV6_DISABLED` | `rrtype` | `string` | rrtype skipped (`A`). |
| `IS_A_RECURSOR` | `ns` | `string` | Nameserver identity (`name/ip`) classified as recursor. |
| `NO_RECURSOR` | `ns` | `string` | Nameserver identity (`name/ip`) classified as non-recursor. |
| `NO_RESPONSE` | `ns` | `string` | Nameserver identity (`name/ip`) with missing response. |
| `NO_RESPONSE` | `domain` | `string` | Probe name queried. |
| `TEST_CASE_END` | `testcase` | `string` | Testcase display name (`Nameserver01`). |
| `TEST_CASE_START` | `testcase` | `string` | Testcase display name (`Nameserver01`). |

## Severity Levels Per Tag
| Tag | Level | Notes |
| --- | --- | --- |
| `IPV4_DISABLED` | `DEBUG` | Default from `share/profile.json` (`test_levels.NAMESERVER`). |
| `IPV6_DISABLED` | `DEBUG` | Default from `share/profile.json` (`test_levels.NAMESERVER`). |
| `IS_A_RECURSOR` | `ERROR` | Default from `share/profile.json` (`test_levels.NAMESERVER`). |
| `NO_RECURSOR` | `INFO` | Default from `share/profile.json` (`test_levels.NAMESERVER`). |
| `NO_RESPONSE` | `DEBUG` | Default from `share/profile.json` (`test_levels.NAMESERVER`). |
| `TEST_CASE_END` | `DEBUG` | Default from `share/profile.json` (`test_levels.NAMESERVER`). |
| `TEST_CASE_START` | `DEBUG` | Default from `share/profile.json` (`test_levels.NAMESERVER`). |

## Differences From Upstream
- Upstream reference: [`nameserver01.md`](../../upstream/tests/Nameserver-TP/nameserver01.md)
- Differences (Upstream vs Gonemaster):
  - Upstream: describes evaluation over the retrieved nameserver IP set. Gonemaster: iterates the raw `Method4and5` list without testcase-local deduplication, so duplicate `name/ip` entries can be evaluated more than once.
  - Upstream: does not explicitly describe testcase boundary and transport-disabled debug emissions. Gonemaster: emits `TEST_CASE_START`, `TEST_CASE_END`, `IPV4_DISABLED`, and `IPV6_DISABLED`.
- Potential upstream report:
  - `no`

## Edge Cases And Limitations
- A nameserver can emit both `NO_RESPONSE` (for one or more probes) and `IS_A_RECURSOR` (from other probe responses) in the same testcase run.
- Up to three `NO_RESPONSE` entries can be emitted per nameserver (one per probe name).
- If transport is disabled for a nameserver, no recursor classification tags are emitted for that nameserver.
