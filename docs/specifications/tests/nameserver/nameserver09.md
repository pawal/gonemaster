# Nameserver09 (nameserver09)

Status: Final

## Purpose
- Compare nameserver results for two differently cased query names that are equivalent under DNS case-insensitive matching.

## Preconditions And Inputs
- Preconditions:
  - A `zone.Zone` object is available.
- Required inputs:
  - Nameserver addresses from `methods.Method4and5`.
  - SOA query responses for two randomized-case `www.<zone>` names.
- Profile/config knobs that affect behavior:
  - `net.ipv4` and `net.ipv6`: disabled transports are skipped with transport debug tags.
  - `resolver.defaults.parallel`: parallel nameserver query fanout.

## Algorithm And Decision Flow
1. Emit `TEST_CASE_START`.
2. Build `original = "www." + <zone>` (trailing dot removed), record type `SOA`.
3. Generate two randomized case variants (`random1`, `random2`) such that both differ from `original` and from each other.
4. Read nameserver list from `Method4and5`, deduplicate by `name/ip`, preserving first-seen order.
5. For each deduplicated nameserver (parallelized, input-order merged logs):
   - If transport is disabled, emit `IPV4_DISABLED` or `IPV6_DISABLED` for rrtype `SOA`, store no mismatch for this nameserver, and skip.
   - Query `random1` and `random2`.
   - If first query has answer RRs:
     - Compare normalized (sorted, lowercased) answer sets.
     - Emit `CASE_QUERY_SAME_ANSWER` when equal, else emit `CASE_QUERY_DIFFERENT_ANSWER` and mark mismatch.
   - Else if both queries returned DNS messages:
     - Compare response RCODE strings.
     - Emit `CASE_QUERY_SAME_RC` when equal, else emit `CASE_QUERY_DIFFERENT_RC` and mark mismatch.
   - Else if exactly one query returned a DNS message:
     - Emit `CASE_QUERY_NO_ANSWER` and mark mismatch.
   - Else (both queries missing DNS messages):
     - Emit no per-server case-result tag and keep mismatch unset.
6. If no nameserver outcome is marked mismatch, emit `CASE_QUERIES_RESULTS_OK`; else emit `CASE_QUERIES_RESULTS_DIFFER`.
7. Emit `TEST_CASE_END`.

## Emitted Tags (Possible Set)
| Tag | Emitted when |
| --- | --- |
| `CASE_QUERIES_RESULTS_DIFFER` | At least one nameserver comparison outcome is marked mismatch. |
| `CASE_QUERIES_RESULTS_OK` | No nameserver comparison outcome is marked mismatch. |
| `CASE_QUERY_DIFFERENT_ANSWER` | Both queries compared by answer set and normalized answers differ. |
| `CASE_QUERY_DIFFERENT_RC` | Answer-set comparison not used and query RCODE values differ. |
| `CASE_QUERY_NO_ANSWER` | Exactly one of the two queries returned a DNS message. |
| `CASE_QUERY_SAME_ANSWER` | Both queries compared by answer set and normalized answers match. |
| `CASE_QUERY_SAME_RC` | Answer-set comparison not used and query RCODE values match. |
| `IPV4_DISABLED` | IPv4 nameserver evaluation is skipped because IPv4 is disabled. |
| `IPV6_DISABLED` | IPv6 nameserver evaluation is skipped because IPv6 is disabled. |
| `TEST_CASE_END` | Testcase completion marker is emitted. |
| `TEST_CASE_START` | Testcase start marker is emitted. |

## Tag Arguments
| Tag | Argument key | Type | Meaning |
| --- | --- | --- | --- |
| `CASE_QUERIES_RESULTS_DIFFER` | `type` | `string` | Compared rrtype (`SOA`). |
| `CASE_QUERIES_RESULTS_DIFFER` | `domain` | `string` | Base name used for comparisons (`www.<zone>`). |
| `CASE_QUERIES_RESULTS_OK` | `type` | `string` | Compared rrtype (`SOA`). |
| `CASE_QUERIES_RESULTS_OK` | `domain` | `string` | Base name used for comparisons (`www.<zone>`). |
| `CASE_QUERY_DIFFERENT_ANSWER` | `ns` | `string` | Nameserver identity (`ns` name only; use `address` for IP). |
| `CASE_QUERY_DIFFERENT_ANSWER` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `CASE_QUERY_DIFFERENT_ANSWER` | `arg_schema` | `string` | Schema identifier (`gonemaster.logargs/1.1`) for coherent log arguments. |
| `CASE_QUERY_DIFFERENT_ANSWER` | `type` | `string` | Compared rrtype (`SOA`). |
| `CASE_QUERY_DIFFERENT_ANSWER` | `query1` | `string` | First randomized query name. |
| `CASE_QUERY_DIFFERENT_ANSWER` | `query2` | `string` | Second randomized query name. |
| `CASE_QUERY_DIFFERENT_RC` | `ns` | `string` | Nameserver identity (`ns` name only; use `address` for IP). |
| `CASE_QUERY_DIFFERENT_RC` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `CASE_QUERY_DIFFERENT_RC` | `arg_schema` | `string` | Schema identifier (`gonemaster.logargs/1.1`) for coherent log arguments. |
| `CASE_QUERY_DIFFERENT_RC` | `type` | `string` | Compared rrtype (`SOA`). |
| `CASE_QUERY_DIFFERENT_RC` | `query1` | `string` | First randomized query name. |
| `CASE_QUERY_DIFFERENT_RC` | `query2` | `string` | Second randomized query name. |
| `CASE_QUERY_DIFFERENT_RC` | `rcode1` | `string` | First query response code. |
| `CASE_QUERY_DIFFERENT_RC` | `rcode2` | `string` | Second query response code. |
| `CASE_QUERY_NO_ANSWER` | `ns` | `string` | Nameserver identity (`ns` name only; use `address` for IP). |
| `CASE_QUERY_NO_ANSWER` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `CASE_QUERY_NO_ANSWER` | `arg_schema` | `string` | Schema identifier (`gonemaster.logargs/1.1`) for coherent log arguments. |
| `CASE_QUERY_NO_ANSWER` | `type` | `string` | Compared rrtype (`SOA`). |
| `CASE_QUERY_NO_ANSWER` | `domain` | `string` | Query name that had a DNS message. |
| `CASE_QUERY_SAME_ANSWER` | `ns` | `string` | Nameserver identity (`ns` name only; use `address` for IP). |
| `CASE_QUERY_SAME_ANSWER` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `CASE_QUERY_SAME_ANSWER` | `arg_schema` | `string` | Schema identifier (`gonemaster.logargs/1.1`) for coherent log arguments. |
| `CASE_QUERY_SAME_ANSWER` | `type` | `string` | Compared rrtype (`SOA`). |
| `CASE_QUERY_SAME_ANSWER` | `query1` | `string` | First randomized query name. |
| `CASE_QUERY_SAME_ANSWER` | `query2` | `string` | Second randomized query name. |
| `CASE_QUERY_SAME_RC` | `ns` | `string` | Nameserver identity (`ns` name only; use `address` for IP). |
| `CASE_QUERY_SAME_RC` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `CASE_QUERY_SAME_RC` | `arg_schema` | `string` | Schema identifier (`gonemaster.logargs/1.1`) for coherent log arguments. |
| `CASE_QUERY_SAME_RC` | `type` | `string` | Compared rrtype (`SOA`). |
| `CASE_QUERY_SAME_RC` | `query1` | `string` | First randomized query name. |
| `CASE_QUERY_SAME_RC` | `query2` | `string` | Second randomized query name. |
| `CASE_QUERY_SAME_RC` | `rcode` | `string` | Shared query response code. |
| `IPV4_DISABLED` | `ns` | `string` | Nameserver identity (`ns` name only; use `address` for IP) skipped on IPv4. |
| `IPV4_DISABLED` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `IPV4_DISABLED` | `arg_schema` | `string` | Schema identifier (`gonemaster.logargs/1.1`) for coherent log arguments. |
| `IPV4_DISABLED` | `rrtype` | `string` | rrtype skipped (`SOA`). |
| `IPV6_DISABLED` | `ns` | `string` | Nameserver identity (`ns` name only; use `address` for IP) skipped on IPv6. |
| `IPV6_DISABLED` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `IPV6_DISABLED` | `arg_schema` | `string` | Schema identifier (`gonemaster.logargs/1.1`) for coherent log arguments. |
| `IPV6_DISABLED` | `rrtype` | `string` | rrtype skipped (`SOA`). |
| `TEST_CASE_END` | `testcase` | `string` | Testcase display name (`Nameserver09`). |
| `TEST_CASE_START` | `testcase` | `string` | Testcase display name (`Nameserver09`). |

## Severity Levels Per Tag
| Tag | Level | Notes |
| --- | --- | --- |
| `CASE_QUERIES_RESULTS_DIFFER` | `ERROR` | Default from `share/profile.json` (`test_levels.NAMESERVER`). |
| `CASE_QUERIES_RESULTS_OK` | `INFO` | Default from `share/profile.json` (`test_levels.NAMESERVER`). |
| `CASE_QUERY_DIFFERENT_ANSWER` | `WARNING` | Default from `share/profile.json` (`test_levels.NAMESERVER`). |
| `CASE_QUERY_DIFFERENT_RC` | `WARNING` | Default from `share/profile.json` (`test_levels.NAMESERVER`). |
| `CASE_QUERY_NO_ANSWER` | `WARNING` | Default from `share/profile.json` (`test_levels.NAMESERVER`). |
| `CASE_QUERY_SAME_ANSWER` | `DEBUG` | Default from `share/profile.json` (`test_levels.NAMESERVER`). |
| `CASE_QUERY_SAME_RC` | `DEBUG` | Default from `share/profile.json` (`test_levels.NAMESERVER`). |
| `IPV4_DISABLED` | `DEBUG` | Default from `share/profile.json` (`test_levels.NAMESERVER`). |
| `IPV6_DISABLED` | `DEBUG` | Default from `share/profile.json` (`test_levels.NAMESERVER`). |
| `TEST_CASE_END` | `DEBUG` | Default from `share/profile.json` (`test_levels.NAMESERVER`). |
| `TEST_CASE_START` | `DEBUG` | Default from `share/profile.json` (`test_levels.NAMESERVER`). |

## Differences From Upstream
- Upstream reference: [`nameserver09.md`](../../upstream/tests/Nameserver-TP/nameserver09.md)
- Differences (Upstream vs Gonemaster):
  - Upstream: specifies compare-and-fail semantics without explicit per-branch message taxonomy. Gonemaster: emits granular per-server branch tags (`CASE_QUERY_*`) plus testcase summary tags (`CASE_QUERIES_RESULTS_OK` / `CASE_QUERIES_RESULTS_DIFFER`).
  - Upstream: describes evaluation over nameserver IP set. Gonemaster: deduplicates nameservers by `name/ip`.
  - Upstream: does not explicitly describe testcase boundary and transport-disabled debug emissions. Gonemaster: emits `TEST_CASE_START`, `TEST_CASE_END`, `IPV4_DISABLED`, and `IPV6_DISABLED`.
- Potential upstream report:
  - `no`

## Edge Cases And Limitations
- If both queries for a nameserver return no DNS message, that nameserver does not set mismatch and emits no `CASE_QUERY_*` tag.
- In the answer-comparison branch (`len(p1.Answer()) > 0`), a missing/no-message second response is treated as `CASE_QUERY_DIFFERENT_ANSWER` (not `CASE_QUERY_NO_ANSWER`).
- `CASE_QUERIES_RESULTS_OK` can still be emitted when all nameservers were skipped by transport-disable checks.
