# Nameserver13 (nameserver13)

Status: Draft

## Purpose
- Check truncated EDNS responses for missing OPT records.

## Preconditions And Inputs
- Preconditions:
  - A `zone.Zone` object is available.
- Required inputs:
  - Nameserver addresses from `methods.Method4and5`.
  - SOA responses to UDP EDNS query with `DO=1` and EDNS size `512`.
- Profile/config knobs that affect behavior:
  - `net.ipv4` and `net.ipv6`: disabled transports are skipped with transport debug tags.
  - `resolver.defaults.parallel`: parallel nameserver query fanout.

## Algorithm And Decision Flow
1. Emit `TEST_CASE_START`.
2. Read nameserver list from `Method4and5`.
3. For each nameserver (parallelized, input-order merged logs):
   - If transport is disabled, emit `IPV4_DISABLED` or `IPV6_DISABLED` for rrtype `SOA`, then skip.
   - Send SOA query with options:
     - EDNS version `0`
     - EDNS `DO=true`
     - EDNS size `512`
     - `UseVC=false`
     - `Fallback=false`
   - If no response, emit `NO_RESPONSE` (`ns`, `domain`).
   - Else if `RCODE=FORMERR` and EDNS extended rcode is `0`, emit `NO_EDNS_SUPPORT`.
   - Else if response is truncated (`TC=1`) and has no EDNS OPT, emit `MISSING_OPT_IN_TRUNCATED`.
   - Else if response shape is (`RCODE=NOERROR`, `EdnsRcode=0`, `EdnsVersion=0`), emit no finding.
   - Else emit `NS_ERROR`.
4. Emit `TEST_CASE_END`.

## Emitted Tags (Possible Set)
| Tag | Emitted when |
| --- | --- |
| `IPV4_DISABLED` | IPv4 nameserver evaluation is skipped because IPv4 is disabled. |
| `IPV6_DISABLED` | IPv6 nameserver evaluation is skipped because IPv6 is disabled. |
| `MISSING_OPT_IN_TRUNCATED` | Response was truncated but lacked EDNS OPT record. |
| `NO_EDNS_SUPPORT` | Response indicates FORMERR EDNS handling fallback path. |
| `NO_RESPONSE` | Query produced no DNS response. |
| `NS_ERROR` | Response did not fit expected success or explicit failure branches. |
| `TEST_CASE_END` | Testcase completion marker is emitted. |
| `TEST_CASE_START` | Testcase start marker is emitted. |

## Tag Arguments
| Tag | Argument key | Type | Meaning |
| --- | --- | --- | --- |
| `IPV4_DISABLED` | `ns` | `string` | Nameserver identity (`name/ip`) skipped on IPv4. |
| `IPV4_DISABLED` | `rrtype` | `string` | rrtype skipped (`SOA`). |
| `IPV6_DISABLED` | `ns` | `string` | Nameserver identity (`name/ip`) skipped on IPv6. |
| `IPV6_DISABLED` | `rrtype` | `string` | rrtype skipped (`SOA`). |
| `MISSING_OPT_IN_TRUNCATED` | `ns` | `string` | Nameserver identity (`name/ip`) returning truncated response without OPT. |
| `NO_EDNS_SUPPORT` | `ns` | `string` | Nameserver identity (`name/ip`) treated as no-EDNS support path. |
| `NO_RESPONSE` | `ns` | `string` | Nameserver identity (`name/ip`) with no response. |
| `NO_RESPONSE` | `domain` | `string` | Tested zone name. |
| `NS_ERROR` | `ns` | `string` | Nameserver identity (`name/ip`) with unexpected behavior. |
| `TEST_CASE_END` | `testcase` | `string` | Testcase display name (`Nameserver13`). |
| `TEST_CASE_START` | `testcase` | `string` | Testcase display name (`Nameserver13`). |

## Severity Levels Per Tag
| Tag | Level | Notes |
| --- | --- | --- |
| `IPV4_DISABLED` | `DEBUG` | Default from `share/profile.json` (`test_levels.NAMESERVER`). |
| `IPV6_DISABLED` | `DEBUG` | Default from `share/profile.json` (`test_levels.NAMESERVER`). |
| `MISSING_OPT_IN_TRUNCATED` | `WARNING` | Default from `share/profile.json` (`test_levels.NAMESERVER`). |
| `NO_EDNS_SUPPORT` | `WARNING` | Default from `share/profile.json` (`test_levels.NAMESERVER`). |
| `NO_RESPONSE` | `DEBUG` | Default from `share/profile.json` (`test_levels.NAMESERVER`). |
| `NS_ERROR` | `WARNING` | Default from `share/profile.json` (`test_levels.NAMESERVER`). |
| `TEST_CASE_END` | `DEBUG` | Default from `share/profile.json` (`test_levels.NAMESERVER`). |
| `TEST_CASE_START` | `DEBUG` | Default from `share/profile.json` (`test_levels.NAMESERVER`). |

## Differences From Upstream
- Upstream reference: [`nameserver13.md`](../../upstream/tests/Nameserver-TP/nameserver13.md)
- Differences (Upstream vs Gonemaster):
  - Upstream specification text: says to send a `DNSKEY` query.
  - Upstream Zonemaster engine actual behavior: sends `SOA` queries (see `Zonemaster::Engine::Test::Nameserver`, `nameserver13`).
  - Gonemaster behavior: sends `SOA` queries, intentionally matching upstream engine behavior.
  - Upstream: describes iterating nameserver IP set. Gonemaster: iterates raw `Method4and5` output (no testcase-local deduplication).
  - Upstream: does not explicitly describe testcase boundary and transport-disabled debug emissions. Gonemaster: emits `TEST_CASE_START`, `TEST_CASE_END`, `IPV4_DISABLED`, and `IPV6_DISABLED`.
- Potential upstream report:
  - `yes`
- If yes, include:
  - Upstream expected behavior: Upstream testcase specification text says query type `DNSKEY` for truncation/OPT checks.
  - Gonemaster observed behavior: Query type is `SOA` with EDNS/DO/size options, matching upstream engine behavior.
  - evidence: `docs/specifications/upstream/tests/Nameserver-TP/nameserver13.md`, `zonemaster-engine/lib/Zonemaster/Engine/Test/Nameserver.pm`, `engine/test/nameserver/nameserver.go`
  - report status: `reported` ([zonemaster#1468](https://github.com/zonemaster/zonemaster/issues/1468))

## Edge Cases And Limitations
- Success branch does not explicitly require SOA answer content.
- `MISSING_OPT_IN_TRUNCATED` is checked before the generic success-shape branch.
- Any behavior outside explicit branches collapses into `NS_ERROR`.
