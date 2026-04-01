# Basic03 (basic03)

Status: Final

## Purpose
- Detect the "broken but functional" case by probing `A` records for `www.<child-zone>` against delegation nameservers.
- Provide a compatibility path when `Basic02` failed but the domain can still answer basic web-style queries.

## Preconditions And Inputs
- Preconditions:
  - A `zone.Zone` is provided.
  - Delegation nameserver addresses can be loaded using `Method4`.
- Required inputs:
  - Child zone name (`z.Name`).
  - Derived query name `www.<child-zone>`.
- Profile/config knobs that affect behavior:
  - `net.ipv4`: enables or disables IPv4 `A` probes.
  - `net.ipv6`: enables or disables IPv6 `A` probes.
  - `resolver.defaults.parallel`: controls per-test parallel nameserver probing.

## Algorithm And Decision Flow
1. In module orchestration (`basic.All`):
   - If `Basic02` emitted `B02_AUTH_RESPONSE_SOA`, `Basic03` function is skipped and `HAS_NAMESERVER_NO_WWW_A_TEST` is emitted.
   - Otherwise `Basic03` function runs.
2. `Basic03` function emits `TEST_CASE_START`.
3. Build `queryName = "www." + child-zone`.
4. Resolve nameserver targets with `Method4` and probe each in parallel:
   - Emit transport enable/disable tags (`IPV4_*`, `IPV6_*`) for rrtype `A`.
   - Send `A` query for `queryName`.
   - If response has `A` RR for `queryName`, emit `HAS_A_RECORDS`; otherwise emit `NO_A_RECORDS`.
5. If no probed nameserver produced any response packet, emit `A_QUERY_NO_RESPONSES`.
6. Emit `TEST_CASE_END`.

## Emitted Tags (Possible Set)
| Tag | Emitted when |
| --- | --- |
| `A_QUERY_NO_RESPONSES` | No nameserver produced a response packet for `A` query to `www.<child-zone>`. |
| `HAS_A_RECORDS` | A nameserver responded with at least one `A` RR for `www.<child-zone>`. |
| `HAS_NAMESERVER_NO_WWW_A_TEST` | Module orchestration skipped `Basic03` because `Basic02` succeeded (`B02_AUTH_RESPONSE_SOA`). |
| `IPV4_DISABLED` | IPv4 transport is disabled for `A` probe. |
| `IPV4_ENABLED` | IPv4 transport is enabled for `A` probe. |
| `IPV6_DISABLED` | IPv6 transport is disabled for `A` probe. |
| `IPV6_ENABLED` | IPv6 transport is enabled for `A` probe. |
| `NO_A_RECORDS` | Nameserver responded but no `A` RR matched `www.<child-zone>`. |
| `TEST_CASE_END` | Testcase completion marker is emitted. |
| `TEST_CASE_START` | Testcase start marker is emitted. |

## Tag Arguments
| Tag | Argument key | Type | Meaning |
| --- | --- | --- | --- |
| `A_QUERY_NO_RESPONSES` | `-` | `-` | No arguments. |
| `HAS_A_RECORDS` | `ns` | `string` | Nameserver identity (`ns` name only; use `address` for IP) that returned `A` records. |
| `HAS_A_RECORDS` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `HAS_A_RECORDS` | `domain` | `string` | Queried name (`www.<child-zone>`). |
| `HAS_NAMESERVER_NO_WWW_A_TEST` | `zname` | `string` | Child zone name for skipped `Basic03` probing path. |
| `IPV4_DISABLED` | `ns` | `string` | Nameserver identity (`ns` name only; use `address` for IP). |
| `IPV4_DISABLED` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `IPV4_DISABLED` | `rrtype` | `string` | rrtype skipped (`A`). |
| `IPV4_ENABLED` | `ns` | `string` | Nameserver identity (`ns` name only; use `address` for IP). |
| `IPV4_ENABLED` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `IPV4_ENABLED` | `rrtype` | `string` | rrtype queried (`A`). |
| `IPV6_DISABLED` | `ns` | `string` | Nameserver identity (`ns` name only; use `address` for IP). |
| `IPV6_DISABLED` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `IPV6_DISABLED` | `rrtype` | `string` | rrtype skipped (`A`). |
| `IPV6_ENABLED` | `ns` | `string` | Nameserver identity (`ns` name only; use `address` for IP). |
| `IPV6_ENABLED` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `IPV6_ENABLED` | `rrtype` | `string` | rrtype queried (`A`). |
| `NO_A_RECORDS` | `ns` | `string` | Nameserver identity (`ns` name only; use `address` for IP) with no matching `A` data. |
| `NO_A_RECORDS` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `NO_A_RECORDS` | `domain` | `string` | Queried name (`www.<child-zone>`). |
| `TEST_CASE_END` | `testcase` | `string` | Testcase display name (`Basic03`). |
| `TEST_CASE_START` | `testcase` | `string` | Testcase display name (`Basic03`). |

## Severity Levels Per Tag
| Tag | Level | Notes |
| --- | --- | --- |
| `A_QUERY_NO_RESPONSES` | `INFO` | Default from `share/profile.json`. |
| `HAS_A_RECORDS` | `ERROR` | Default from `share/profile.json`. |
| `HAS_NAMESERVER_NO_WWW_A_TEST` | `INFO` | Default from `share/profile.json`. |
| `IPV4_DISABLED` | `DEBUG` | Default from `share/profile.json`. |
| `IPV4_ENABLED` | `DEBUG` | Default from `share/profile.json`. |
| `IPV6_DISABLED` | `DEBUG` | Default from `share/profile.json`. |
| `IPV6_ENABLED` | `DEBUG` | Default from `share/profile.json`. |
| `NO_A_RECORDS` | `DEBUG` | Default from `share/profile.json`. |
| `TEST_CASE_END` | `DEBUG` | Default from `share/profile.json`. |
| `TEST_CASE_START` | `DEBUG` | Default from `share/profile.json`. |

## Differences From Upstream
- Upstream reference: [`basic03.md`](../../upstream/tests/Basic-TP/basic03.md)
- Differences (Upstream vs Gonemaster):
  - Upstream: does not define a skip marker for this path. Gonemaster: emits `HAS_NAMESERVER_NO_WWW_A_TEST` when `Basic03` is skipped because `Basic02` succeeded.
  - Upstream: outcome text says testcase fails when no response contains an `A` record. Gonemaster: default levels make `HAS_A_RECORDS` the error path and keep `NO_A_RECORDS` at `DEBUG`.
- Potential upstream report:
  - `yes`
- If yes, include:
  - Upstream expected behavior: `Basic03` outcome text describes failure when no `A` evidence is found.
  - Gonemaster observed behavior: severity contract treats `HAS_A_RECORDS` as the strong signal (`ERROR`) and `NO_A_RECORDS` as non-failing (`DEBUG` default).
  - evidence: `engine/test/basic/basic.go` (`All`, `Basic03`), `share/profile.json` (`test_levels.BASIC`).
  - report status: `not filed`

## Edge Cases And Limitations
- If `Method4` yields no nameserver addresses, function path emits only testcase markers unless orchestration emitted `HAS_NAMESERVER_NO_WWW_A_TEST`.
- `A_QUERY_NO_RESPONSES` is emitted only when there are zero response packets; responses without `A` produce `NO_A_RECORDS` instead.
- Per-nameserver output ordering is deterministic after parallel execution due runner merge order.
