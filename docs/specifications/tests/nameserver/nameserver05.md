# Nameserver05 (nameserver05)

Status: Final

## Purpose
- Evaluate nameserver behavior for AAAA queries after successful A-query baseline checks.

## Preconditions And Inputs
- Preconditions:
  - A `zone.Zone` object is available.
- Required inputs:
  - Nameserver addresses from `methods.Method4and5`.
  - `A` and `AAAA` query responses for zone apex.
- Profile/config knobs that affect behavior:
  - `net.ipv4` and `net.ipv6`: disabled transports are skipped with transport debug tags.
  - `resolver.defaults.parallel`: parallel nameserver query fanout.

## Algorithm And Decision Flow
1. Emit `TEST_CASE_START`.
2. Read nameserver list from `Method4and5`, deduplicate by `name/ip`, preserving first-seen order.
3. For each deduplicated nameserver (parallelized, input-order merged logs):
   - If transport is disabled, emit `IPV4_DISABLED` or `IPV6_DISABLED` for rrtype `A`, mark not included in summary, and skip.
   - Mark nameserver as included in summary.
   - Send apex `A` query over UDP (`UseVC=false`).
   - If no DNS message is returned, emit `NO_RESPONSE` and stop processing this nameserver.
   - If `A` response `RCODE != NOERROR`, emit `A_UNEXPECTED_RCODE` and stop processing this nameserver.
   - Send apex `AAAA` query over UDP (`UseVC=false`).
   - If no DNS message is returned, emit `AAAA_QUERY_DROPPED`, increment AAAA-issue counter for this nameserver, and stop processing this nameserver.
   - If `AAAA` response `RCODE != NOERROR`, emit `AAAA_UNEXPECTED_RCODE`, increment AAAA-issue counter, and stop processing this nameserver.
   - For each `AAAA` RR in answer:
     - If RDATA length is not 16 bytes, emit `AAAA_BAD_RDATA` and increment AAAA-issue counter.
     - Else increment AAAA-ok counter.
4. Aggregate counters over included nameservers.
5. If total AAAA-ok count is greater than zero and total AAAA-issue count is zero, emit `AAAA_WELL_PROCESSED`.
6. Emit `TEST_CASE_END`.

## Emitted Tags (Possible Set)
| Tag | Emitted when |
| --- | --- |
| `AAAA_BAD_RDATA` | A returned AAAA RR had invalid RDATA length. |
| `AAAA_QUERY_DROPPED` | AAAA query returned no DNS message after successful A-query baseline. |
| `AAAA_UNEXPECTED_RCODE` | AAAA query returned non-`NOERROR` RCODE after successful A-query baseline. |
| `AAAA_WELL_PROCESSED` | At least one valid AAAA RR was observed and no AAAA-issue tags were triggered across included nameservers. |
| `A_UNEXPECTED_RCODE` | Initial A-query baseline returned non-`NOERROR` RCODE. |
| `IPV4_DISABLED` | IPv4 nameserver evaluation is skipped because IPv4 is disabled. |
| `IPV6_DISABLED` | IPv6 nameserver evaluation is skipped because IPv6 is disabled. |
| `NO_RESPONSE` | Initial A-query baseline returned no DNS message. |
| `TEST_CASE_END` | Testcase completion marker is emitted. |
| `TEST_CASE_START` | Testcase start marker is emitted. |

## Tag Arguments
| Tag | Argument key | Type | Meaning |
| --- | --- | --- | --- |
| `AAAA_BAD_RDATA` | `ns` | `string` | Nameserver identity (`ns` name only; use `address` for IP) returning invalid AAAA RDATA length. |
| `AAAA_BAD_RDATA` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `AAAA_BAD_RDATA` | `arg_schema` | `string` | Schema identifier (`gonemaster.logargs/1.1`) for coherent log arguments. |
| `AAAA_BAD_RDATA` | `length` | `int` | Observed AAAA RDATA byte length. |
| `AAAA_QUERY_DROPPED` | `ns` | `string` | Nameserver identity (`ns` name only; use `address` for IP) that dropped AAAA query. |
| `AAAA_QUERY_DROPPED` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `AAAA_QUERY_DROPPED` | `arg_schema` | `string` | Schema identifier (`gonemaster.logargs/1.1`) for coherent log arguments. |
| `AAAA_UNEXPECTED_RCODE` | `ns` | `string` | Nameserver identity (`ns` name only; use `address` for IP) with unexpected AAAA RCODE. |
| `AAAA_UNEXPECTED_RCODE` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `AAAA_UNEXPECTED_RCODE` | `arg_schema` | `string` | Schema identifier (`gonemaster.logargs/1.1`) for coherent log arguments. |
| `AAAA_UNEXPECTED_RCODE` | `rcode` | `string` | Returned AAAA-query RCODE string. |
| `AAAA_WELL_PROCESSED` | `ns_list` | `string` | Semicolon-delimited sorted included nameserver identities (`name/ip`). |
| `A_UNEXPECTED_RCODE` | `ns` | `string` | Nameserver identity (`ns` name only; use `address` for IP) with unexpected A-query RCODE. |
| `A_UNEXPECTED_RCODE` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `A_UNEXPECTED_RCODE` | `arg_schema` | `string` | Schema identifier (`gonemaster.logargs/1.1`) for coherent log arguments. |
| `A_UNEXPECTED_RCODE` | `rcode` | `string` | Returned A-query RCODE string. |
| `IPV4_DISABLED` | `ns` | `string` | Nameserver identity (`ns` name only; use `address` for IP) skipped on IPv4. |
| `IPV4_DISABLED` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `IPV4_DISABLED` | `arg_schema` | `string` | Schema identifier (`gonemaster.logargs/1.1`) for coherent log arguments. |
| `IPV4_DISABLED` | `rrtype` | `string` | rrtype skipped (`A`). |
| `IPV6_DISABLED` | `ns` | `string` | Nameserver identity (`ns` name only; use `address` for IP) skipped on IPv6. |
| `IPV6_DISABLED` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `IPV6_DISABLED` | `arg_schema` | `string` | Schema identifier (`gonemaster.logargs/1.1`) for coherent log arguments. |
| `IPV6_DISABLED` | `rrtype` | `string` | rrtype skipped (`A`). |
| `NO_RESPONSE` | `ns` | `string` | Nameserver identity (`ns` name only; use `address` for IP) with no A-query response. |
| `NO_RESPONSE` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `NO_RESPONSE` | `arg_schema` | `string` | Schema identifier (`gonemaster.logargs/1.1`) for coherent log arguments. |
| `NO_RESPONSE` | `domain` | `string` | Tested zone name. |
| `TEST_CASE_END` | `testcase` | `string` | Testcase display name (`Nameserver05`). |
| `TEST_CASE_START` | `testcase` | `string` | Testcase display name (`Nameserver05`). |

## Severity Levels Per Tag
| Tag | Level | Notes |
| --- | --- | --- |
| `AAAA_BAD_RDATA` | `ERROR` | Default from `share/profile.json` (`test_levels.NAMESERVER`). |
| `AAAA_QUERY_DROPPED` | `ERROR` | Default from `share/profile.json` (`test_levels.NAMESERVER`). |
| `AAAA_UNEXPECTED_RCODE` | `ERROR` | Default from `share/profile.json` (`test_levels.NAMESERVER`). |
| `AAAA_WELL_PROCESSED` | `INFO` | Default from `share/profile.json` (`test_levels.NAMESERVER`). |
| `A_UNEXPECTED_RCODE` | `WARNING` | Default from `share/profile.json` (`test_levels.NAMESERVER`). |
| `IPV4_DISABLED` | `DEBUG` | Default from `share/profile.json` (`test_levels.NAMESERVER`). |
| `IPV6_DISABLED` | `DEBUG` | Default from `share/profile.json` (`test_levels.NAMESERVER`). |
| `NO_RESPONSE` | `DEBUG` | Default from `share/profile.json` (`test_levels.NAMESERVER`). |
| `TEST_CASE_END` | `DEBUG` | Default from `share/profile.json` (`test_levels.NAMESERVER`). |
| `TEST_CASE_START` | `DEBUG` | Default from `share/profile.json` (`test_levels.NAMESERVER`). |

## Differences From Upstream
- Upstream reference: [`nameserver05.md`](../../upstream/tests/Nameserver-TP/nameserver05.md)
- Differences (Upstream vs Gonemaster):
  - Upstream: models an `AAAA OK` set of nameserver IPs and uses that set for the final positive condition. Gonemaster: final `AAAA_WELL_PROCESSED` condition is also global (no AAAA issues anywhere), but emitted `ns_list` contains all included nameservers, not only nameservers with successful AAAA records.
  - Upstream: describes iterating nameserver IP set. Gonemaster: deduplicates nameservers by `name/ip` before evaluation.
  - Upstream: does not explicitly describe testcase boundary and transport-disabled debug emissions. Gonemaster: emits `TEST_CASE_START`, `TEST_CASE_END`, `IPV4_DISABLED`, and `IPV6_DISABLED`.
- Potential upstream report:
  - `no`

## Edge Cases And Limitations
- If AAAA responses are `NOERROR` but contain no AAAA RRs, they contribute neither AAAA-ok nor AAAA-issue counts.
- Any AAAA issue on any included nameserver suppresses `AAAA_WELL_PROCESSED` for the whole testcase.
- Nameservers with failing A-query baseline are excluded from AAAA evaluation for that nameserver.
