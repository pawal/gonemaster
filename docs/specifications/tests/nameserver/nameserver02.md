# Nameserver02 (nameserver02)

Status: Final

## Purpose
- Validate EDNS(0) handling on authoritative nameservers.

## Preconditions And Inputs
- Preconditions:
  - A `zone.Zone` object is available.
- Required inputs:
  - Nameserver addresses from `methods.Method4and5`.
  - SOA responses to queries with and without EDNS.
- Profile/config knobs that affect behavior:
  - `net.ipv4` and `net.ipv6`: disabled transports are skipped with transport debug tags.
  - `resolver.defaults.parallel`: parallel nameserver query fanout.

## Algorithm And Decision Flow
1. Emit `TEST_CASE_START`.
2. Read nameserver list from `Method4and5`, deduplicate by `name/ip`, preserving first-seen order.
3. For each deduplicated nameserver (parallelized, input-order merged logs):
   - If transport is disabled, emit `IPV4_DISABLED` or `IPV6_DISABLED` for rrtype `SOA`, mark not included in summary, and skip.
   - Mark nameserver as included in summary.
   - Send SOA query with EDNS version `0`.
   - If response exists:
     - If `RCODE=FORMERR` and response has no OPT, emit `NO_EDNS_SUPPORT`.
     - Else if `RCODE=NOERROR`, `EdnsRcode=0`, answer contains SOA, and `EdnsVersion=0`, treat as compliant and emit no per-server error tag.
     - Else if `RCODE=NOERROR` and response has no OPT, emit `EDNS_RESPONSE_WITHOUT_EDNS`.
     - Else if `RCODE=NOERROR`, response has OPT, and `EdnsVersion!=0`, emit `EDNS_VERSION_ERROR`.
     - Else emit `NS_ERROR`.
   - If response is missing or query failed:
     - Send fallback SOA query without EDNS.
     - If fallback responds, emit `BREAKS_ON_EDNS`.
     - Else emit `NO_RESPONSE`.
4. Count included nameservers and included nameservers with errors.
5. If at least one nameserver was included and none had errors, emit `EDNS0_SUPPORT` with sorted included `name/ip` list.
6. Emit `TEST_CASE_END`.

## Emitted Tags (Possible Set)
| Tag | Emitted when |
| --- | --- |
| `BREAKS_ON_EDNS` | Query with EDNS failed, but fallback query without EDNS got a response. |
| `EDNS0_SUPPORT` | At least one included nameserver was evaluated and none produced EDNS error findings. |
| `EDNS_RESPONSE_WITHOUT_EDNS` | `NOERROR` response to EDNS query omitted OPT record. |
| `EDNS_VERSION_ERROR` | `NOERROR` response to EDNS query returned OPT version other than `0`. |
| `IPV4_DISABLED` | IPv4 nameserver evaluation is skipped because IPv4 is disabled. |
| `IPV6_DISABLED` | IPv6 nameserver evaluation is skipped because IPv6 is disabled. |
| `NO_EDNS_SUPPORT` | EDNS query returned `FORMERR` without OPT record. |
| `NO_RESPONSE` | Neither EDNS query nor fallback non-EDNS query returned a DNS message. |
| `NS_ERROR` | Response did not match the recognized EDNS compliance/error branches. |
| `TEST_CASE_END` | Testcase completion marker is emitted. |
| `TEST_CASE_START` | Testcase start marker is emitted. |

## Tag Arguments
| Tag | Argument key | Type | Meaning |
| --- | --- | --- | --- |
| `BREAKS_ON_EDNS` | `ns` | `string` | Nameserver identity (`ns` name only; use `address` for IP) failing on EDNS query. |
| `BREAKS_ON_EDNS` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `BREAKS_ON_EDNS` | `domain` | `string` | Tested zone name. |
| `EDNS0_SUPPORT` | `ns_list` | `string` | Semicolon-delimited sorted included nameserver identities (`name/ip`). |
| `EDNS_RESPONSE_WITHOUT_EDNS` | `ns` | `string` | Nameserver identity (`ns` name only; use `address` for IP) with missing OPT in EDNS response. |
| `EDNS_RESPONSE_WITHOUT_EDNS` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `EDNS_RESPONSE_WITHOUT_EDNS` | `domain` | `string` | Tested zone name. |
| `EDNS_VERSION_ERROR` | `ns` | `string` | Nameserver identity (`ns` name only; use `address` for IP) with unexpected EDNS version. |
| `EDNS_VERSION_ERROR` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `EDNS_VERSION_ERROR` | `domain` | `string` | Tested zone name. |
| `IPV4_DISABLED` | `ns` | `string` | Nameserver identity (`ns` name only; use `address` for IP) skipped on IPv4. |
| `IPV4_DISABLED` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `IPV4_DISABLED` | `rrtype` | `string` | rrtype skipped (`SOA`). |
| `IPV6_DISABLED` | `ns` | `string` | Nameserver identity (`ns` name only; use `address` for IP) skipped on IPv6. |
| `IPV6_DISABLED` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `IPV6_DISABLED` | `rrtype` | `string` | rrtype skipped (`SOA`). |
| `NO_EDNS_SUPPORT` | `ns` | `string` | Nameserver identity (`ns` name only; use `address` for IP) not supporting EDNS as tested. |
| `NO_EDNS_SUPPORT` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `NO_RESPONSE` | `ns` | `string` | Nameserver identity (`ns` name only; use `address` for IP) with no response in EDNS/non-EDNS fallback path. |
| `NO_RESPONSE` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `NO_RESPONSE` | `domain` | `string` | Tested zone name. |
| `NS_ERROR` | `ns` | `string` | Nameserver identity (`ns` name only; use `address` for IP) with unexpected EDNS response behavior. |
| `NS_ERROR` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `TEST_CASE_END` | `testcase` | `string` | Testcase display name (`Nameserver02`). |
| `TEST_CASE_START` | `testcase` | `string` | Testcase display name (`Nameserver02`). |

## Severity Levels Per Tag
| Tag | Level | Notes |
| --- | --- | --- |
| `BREAKS_ON_EDNS` | `ERROR` | Default from `share/profile.json` (`test_levels.NAMESERVER`). |
| `EDNS0_SUPPORT` | `INFO` | Default from `share/profile.json` (`test_levels.NAMESERVER`). |
| `EDNS_RESPONSE_WITHOUT_EDNS` | `ERROR` | Default from `share/profile.json` (`test_levels.NAMESERVER`). |
| `EDNS_VERSION_ERROR` | `ERROR` | Default from `share/profile.json` (`test_levels.NAMESERVER`). |
| `IPV4_DISABLED` | `DEBUG` | Default from `share/profile.json` (`test_levels.NAMESERVER`). |
| `IPV6_DISABLED` | `DEBUG` | Default from `share/profile.json` (`test_levels.NAMESERVER`). |
| `NO_EDNS_SUPPORT` | `WARNING` | Default from `share/profile.json` (`test_levels.NAMESERVER`). |
| `NO_RESPONSE` | `DEBUG` | Default from `share/profile.json` (`test_levels.NAMESERVER`). |
| `NS_ERROR` | `WARNING` | Default from `share/profile.json` (`test_levels.NAMESERVER`). |
| `TEST_CASE_END` | `DEBUG` | Default from `share/profile.json` (`test_levels.NAMESERVER`). |
| `TEST_CASE_START` | `DEBUG` | Default from `share/profile.json` (`test_levels.NAMESERVER`). |

## Differences From Upstream
- Upstream reference: [`nameserver02.md`](../../upstream/tests/Nameserver-TP/nameserver02.md)
- Differences (Upstream vs Gonemaster):
  - Upstream: outcome table focuses on per-server error findings. Gonemaster: emits additional summary tag `EDNS0_SUPPORT` when all included nameservers pass.
  - Upstream: describes iterating the nameserver IP set. Gonemaster: deduplicates nameservers by `name/ip` before evaluation.
  - Upstream: does not explicitly describe testcase boundary and transport-disabled debug emissions. Gonemaster: emits `TEST_CASE_START`, `TEST_CASE_END`, `IPV4_DISABLED`, and `IPV6_DISABLED`.
- Potential upstream report:
  - `no`

## Edge Cases And Limitations
- A nameserver emits at most one per-server error tag in this testcase due early-return branch logic.
- `EDNS0_SUPPORT` is not emitted if all nameservers are skipped due disabled transport.
- The testcase relies on generic query defaults for EDNS payload size and DO bit when those fields are not explicitly set in testcase logic.
