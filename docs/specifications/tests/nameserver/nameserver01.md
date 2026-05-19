# Nameserver01

Status: Final

## Purpose
- Detect whether authoritative nameservers also behave as recursors.

## Preconditions And Inputs
- Preconditions:
  - A `zone.Zone` object is available.
- Required inputs:
  - Nameserver addresses from `ZoneNameservers`.
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
2. Read nameserver list from `ZoneNameservers`.
3. For each nameserver (parallelized, input-order merged logs):
   - If transport is disabled, emit `IPV4_DISABLED` or `IPV6_DISABLED` for rrtype `A`, then skip this nameserver.
   - Initialize counters: `responseCount`, `nxdomainCount`, `hasSeenRA`, `allNxdomainAA=true`, and `isNoRecursor=true`.
   - For each probe name:
     - Query `A`.
     - If no DNS message is returned, emit `NO_RESPONSE` (`ns`, `domain`), set `isNoRecursor=false`, and continue.
     - Increment `responseCount`.
     - If response has `RA=1`, set `hasSeenRA=true`.
     - If response `RCODE` is `NXDOMAIN`, increment `nxdomainCount`. If the response does not have `AA=1`, set `allNxdomainAA=false`.
   - If `hasSeenRA=true`, record server as recursor and set `isNoRecursor=false`.
   - Else if `responseCount>0` and `nxdomainCount==responseCount` and `allNxdomainAA==false`, record server as recursor and set `isNoRecursor=false`.
   - If `isNoRecursor` is still true, record server as non-recursor.
4. After all parallel tasks, emit a single consolidated `IS_A_RECURSOR` with `servers` list (if any), and a single consolidated `NO_RECURSOR` with `servers` list (if any).
5. Emit `TEST_CASE_END`.

### Per-NS Recursor Probe and Classification (steps 2-5)

{{% expand "Show diagram" %}}
```
ns list = ZoneNameservers  (no testcase-local dedupe)

probes = [
   xn--nameservertest.iis.se,
   xn--nameservertest.icann.org,
   xn--nameservertest.ripe.net,
]

For each nameserver (parallel; fan-out = resolver.defaults.parallel):

   transport disabled for A  -> IPV4_DISABLED / IPV6_DISABLED, skip
   init responseCount=0, nxdomainCount=0, hasSeenRA=false,
        allNxdomainAA=true, isNoRecursor=true

   for each probe in probes:
      query A at probe
      +- resp.Msg == nil          -> NO_RESPONSE (ns, domain=probe)
      |                              isNoRecursor=false; continue
      +- otherwise:
            responseCount += 1
            resp.RA  -> hasSeenRA=true
            RCODE == NXDOMAIN -> nxdomainCount += 1
                                 !AA -> allNxdomainAA=false

   hasSeenRA == true
      -> recursorSet[ns]; isNoRecursor=false
   else if responseCount > 0
              AND nxdomainCount == responseCount
              AND allNxdomainAA == false
      -> recursorSet[ns]; isNoRecursor=false
   isNoRecursor still true
      -> nonRecursorSet[ns]

After all tasks:
  recursorSet    non-empty -> IS_A_RECURSOR (servers; sorted)
  nonRecursorSet non-empty -> NO_RECURSOR   (servers; sorted)

emit TEST_CASE_END
```
{{% /expand %}}

## Emitted Tags (Possible Set)
| Tag | Emitted when |
| --- | --- |
| `IPV4_DISABLED` | IPv4 nameserver evaluation is skipped because IPv4 is disabled. |
| `IPV6_DISABLED` | IPv6 nameserver evaluation is skipped because IPv6 is disabled. |
| `IS_A_RECURSOR` | Nameserver set `RA=1` on at least one probe response, or all received probe responses were `NXDOMAIN` without all having `AA=1`. |
| `NO_RECURSOR` | Nameserver produced responses but did not match recursor criteria and had no `NO_RESPONSE` for probes. |
| `NO_RESPONSE` | A probe query returned no DNS message. |
| `TEST_CASE_END` | Testcase completion marker is emitted. |
| `TEST_CASE_START` | Testcase start marker is emitted. |

## Tag Arguments
| Tag | Argument key | Type | Meaning |
| --- | --- | --- | --- |
| `IPV4_DISABLED` | `ns` | `string` | Nameserver identity (`ns` name only; use `address` for IP) skipped on IPv4. |
| `IPV4_DISABLED` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `IPV4_DISABLED` | `rrtype` | `string` | rrtype skipped (`A`). |
| `IPV6_DISABLED` | `ns` | `string` | Nameserver identity (`ns` name only; use `address` for IP) skipped on IPv6. |
| `IPV6_DISABLED` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `IPV6_DISABLED` | `rrtype` | `string` | rrtype skipped (`A`). |
| `IS_A_RECURSOR` | `servers` | `array<object>` | Structured sorted list of nameservers classified as recursors (`{ns}`, `{address}` items). |
| `NO_RECURSOR` | `servers` | `array<object>` | Structured sorted list of nameservers classified as non-recursors (`{ns}`, `{address}` items). |
| `NO_RESPONSE` | `ns` | `string` | Nameserver identity (`ns` name only; use `address` for IP) with missing response. |
| `NO_RESPONSE` | `address` | `string` | Nameserver IP address for the same endpoint. |
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
  - Upstream: describes evaluation over the retrieved nameserver IP set. Gonemaster: iterates the raw `ZoneNameservers` list without testcase-local deduplication, so duplicate `name/ip` entries can be evaluated more than once.
  - Upstream: does not explicitly describe testcase boundary and transport-disabled debug emissions. Gonemaster: emits `TEST_CASE_START`, `TEST_CASE_END`, `IPV4_DISABLED`, and `IPV6_DISABLED`.
  - Upstream: classifies a server as a recursor when all probe responses are `NXDOMAIN`, regardless of the `AA` flag. Gonemaster: excludes servers from recursor classification when all `NXDOMAIN` responses also have `AA=1`, since this indicates the server claims authoritative knowledge (e.g. a fake root zone) rather than performing recursion. Reported upstream.
- Potential upstream report:
  - `no`

## Edge Cases And Limitations
- A nameserver can emit both `NO_RESPONSE` (for one or more probes) and `IS_A_RECURSOR` (from other probe responses) in the same testcase run.
- Up to three `NO_RESPONSE` entries can be emitted per nameserver (one per probe name).
- If transport is disabled for a nameserver, no recursor classification tags are emitted for that nameserver.
- A nameserver that claims to be authoritative for the root zone (responds with `AA=1` and `NXDOMAIN` to all probes) is not classified as a recursor, since the `NXDOMAIN` responses come from fake authoritative data rather than recursive resolution.
