# Nameserver04

Status: Final

## Purpose
- Verify that nameserver responses come from the same IP address that was queried.

## Preconditions And Inputs
- Preconditions:
  - A `zone.Zone` object is available.
- Required inputs:
  - Nameserver addresses from `AllNameservers`.
  - SOA query responses and response-source address metadata.
- Profile/config knobs that affect behavior:
  - `net.ipv4` and `net.ipv6`: disabled transports are skipped with transport debug tags.
  - `resolver.defaults.parallel`: parallel nameserver query fanout.

## Algorithm And Decision Flow
1. Emit `TEST_CASE_START`.
2. Read nameserver list from `AllNameservers`, deduplicate by `name/ip`, preserving first-seen order.
3. For each deduplicated nameserver (parallelized, input-order merged logs):
   - If transport is disabled, emit `IPV4_DISABLED` or `IPV6_DISABLED` for rrtype `SOA`, mark not included in summary, and skip.
   - Mark nameserver as included in summary.
   - Query zone SOA.
   - If response exists and `AnswerFrom` parses as IP address (with optional port), compare parsed source IP with queried nameserver IP.
   - If parsed source differs, emit `DIFFERENT_SOURCE_IP` (`ns`, `source`) and mark error for summary.
4. After all tasks, if at least one nameserver was included and no source-IP mismatches were found, emit `SAME_SOURCE_IP`.
5. Emit `TEST_CASE_END`.

## Emitted Tags (Possible Set)
| Tag | Emitted when |
| --- | --- |
| `DIFFERENT_SOURCE_IP` | Query response source IP differs from queried nameserver IP. |
| `IPV4_DISABLED` | IPv4 nameserver evaluation is skipped because IPv4 is disabled. |
| `IPV6_DISABLED` | IPv6 nameserver evaluation is skipped because IPv6 is disabled. |
| `SAME_SOURCE_IP` | At least one nameserver was included and no source-IP mismatch was found. |
| `TEST_CASE_END` | Testcase completion marker is emitted. |
| `TEST_CASE_START` | Testcase start marker is emitted. |

## Tag Arguments
| Tag | Argument key | Type | Meaning |
| --- | --- | --- | --- |
| `DIFFERENT_SOURCE_IP` | `ns` | `string` | Queried nameserver identity (`ns` name only; use `address` for IP). |
| `DIFFERENT_SOURCE_IP` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `DIFFERENT_SOURCE_IP` | `source` | `string` | Observed response source value in `ip:port` format. |
| `IPV4_DISABLED` | `ns` | `string` | Nameserver identity (`ns` name only; use `address` for IP) skipped on IPv4. |
| `IPV4_DISABLED` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `IPV4_DISABLED` | `rrtype` | `string` | rrtype skipped (`SOA`). |
| `IPV6_DISABLED` | `ns` | `string` | Nameserver identity (`ns` name only; use `address` for IP) skipped on IPv6. |
| `IPV6_DISABLED` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `IPV6_DISABLED` | `rrtype` | `string` | rrtype skipped (`SOA`). |
| `SAME_SOURCE_IP` | `names` | `string` | Comma-delimited sorted included nameserver identities (`name/ip`). |
| `TEST_CASE_END` | `testcase` | `string` | Testcase display name (`Nameserver04`). |
| `TEST_CASE_START` | `testcase` | `string` | Testcase display name (`Nameserver04`). |

## Severity Levels Per Tag
| Tag | Level | Notes |
| --- | --- | --- |
| `DIFFERENT_SOURCE_IP` | `WARNING` | Default from `share/profile.json` (`test_levels.NAMESERVER`). |
| `IPV4_DISABLED` | `DEBUG` | Default from `share/profile.json` (`test_levels.NAMESERVER`). |
| `IPV6_DISABLED` | `DEBUG` | Default from `share/profile.json` (`test_levels.NAMESERVER`). |
| `SAME_SOURCE_IP` | `INFO` | Default from `share/profile.json` (`test_levels.NAMESERVER`). |
| `TEST_CASE_END` | `DEBUG` | Default from `share/profile.json` (`test_levels.NAMESERVER`). |
| `TEST_CASE_START` | `DEBUG` | Default from `share/profile.json` (`test_levels.NAMESERVER`). |

## Differences From Upstream
- Upstream reference: [`nameserver04.md`](../../upstream/tests/Nameserver-TP/nameserver04.md)
- Differences (Upstream vs Gonemaster):
  - Upstream: states any answer must come from the queried IP. Gonemaster: only evaluates mismatch when response metadata `AnswerFrom` can be parsed as an IP address.
  - Upstream: describes failure-only semantics. Gonemaster: emits a positive summary tag `SAME_SOURCE_IP` when no mismatches are found among included nameservers.
  - Upstream: does not explicitly describe testcase boundary and transport-disabled debug emissions. Gonemaster: emits `TEST_CASE_START`, `TEST_CASE_END`, `IPV4_DISABLED`, and `IPV6_DISABLED`.
- Potential upstream report:
  - `no`

## Edge Cases And Limitations
- Missing responses or unparsable `AnswerFrom` values do not emit `DIFFERENT_SOURCE_IP`.
- `SAME_SOURCE_IP` is not emitted when no nameservers are included (for example all skipped due disabled transport).
- `SAME_SOURCE_IP` uses comma as delimiter in `names`, unlike many other tags that use semicolon.
