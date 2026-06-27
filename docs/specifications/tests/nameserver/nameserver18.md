# Nameserver18

Status: Final

## Purpose
- Query each authoritative nameserver for the zone and report any EDNS Extended DNS Error (RFC 8914, option code 15) option found in the response, classified by what the observed info-code implies about a directly-queried authoritative server.

## Preconditions And Inputs
- Preconditions:
  - A `zone.Zone` object is available.
- Required inputs:
  - Nameserver addresses from [`ZoneNameservers`](../../nameserver-resolution.md#zonenameservers).
  - SOA responses to a plain (DO=0) apex query; any EDE option present is read off the response.
- Profile/config knobs that affect behavior:
  - `net.ipv4` and `net.ipv6`: disabled transports are skipped with transport debug tags.
  - `resolver.defaults.parallel`: parallel nameserver query fanout.

## Algorithm And Decision Flow
1. Emit `TEST_CASE_START`.
2. Initialize collectors:
   - observed EDE findings keyed by `(info_code, sanitized extra_text)`
   - clean nameserver set (NOERROR carrying no EDE)
   - no-response nameserver set
3. Read nameserver list from [`ZoneNameservers`](../../nameserver-resolution.md#zonenameservers).
4. For each nameserver (parallelized, input-order merged logs):
   - If transport is disabled, emit `IPV4_DISABLED` or `IPV6_DISABLED` for rrtype `SOA`, then skip.
   - Send a plain SOA query (DO=0) for the zone name. The DO=0 probe deduplicates against SOA queries other testcases already issue, so it adds no extra traffic.
   - If no response, collect nameserver for `N18_NO_RESPONSE`.
   - Else collect every EDE option present (a response MAY carry more than one). For each option, sanitize the EXTRA-TEXT and collect `(info_code, extra_text, nameserver)`. EDE is collected irrespective of RCODE.
   - Else, only when `RCODE == NOERROR` and no EDE is present, collect nameserver for `N18_NO_EXTENDED_ERROR`. A non-NOERROR response without EDE is left to other testcases (basic/Nameserver16) and is not bucketed here.
5. For each unique `(info_code, extra_text)` pair, emit one finding using the class tag selected from the info-code (see classification), with sorted unique `servers`.
6. Emit `N18_NO_EXTENDED_ERROR` with sorted unique `servers` when non-empty.
7. Emit `N18_NO_RESPONSE` with sorted unique `servers` when non-empty.
8. Emit `TEST_CASE_END`.

### Info-code classification
Each observed info-code maps to exactly one class tag. Coverage is complete and disjoint over IANA codes 0-33; codes >=34 (unassigned) and 49152-65535 (private use) fall to the benign default.

| Class tag | Info-codes |
| --- | --- |
| `N18_SERVER_ERROR_REPORTED` (authoritative conformance problem) | 18, 20, 21 |
| `N18_FILTERED_RESPONSE` (filtering middlebox in path) | 4, 15, 16, 17 |
| `N18_RESOLVER_BEHAVIOR_REPORTED` (resolver-oriented code from a delegated authoritative; possible role confusion) | 1, 2, 3, 5, 6, 7, 8, 9, 10, 11, 12, 13, 19, 22, 23, 25, 27, 29, 33 |
| `N18_EXTENDED_ERROR_REPORTED` (benign / operational annotation) | 0, 14, 24, 26, 28, 30, 31, 32, and any unassigned or private-use code |

## Emitted Tags (Possible Set)
| Tag | Emitted when |
| --- | --- |
| `IPV4_DISABLED` | IPv4 nameserver evaluation is skipped because IPv4 is disabled. |
| `IPV6_DISABLED` | IPv6 nameserver evaluation is skipped because IPv6 is disabled. |
| `N18_EXTENDED_ERROR_REPORTED` | Server returned a benign/operational EDE info-code. |
| `N18_FILTERED_RESPONSE` | Server returned a filtering EDE info-code, indicating a policy middlebox in the path. |
| `N18_NO_EXTENDED_ERROR` | A NOERROR response carried no EDE option (the common healthy case). |
| `N18_NO_RESPONSE` | The EDE probe produced no DNS response. |
| `N18_RESOLVER_BEHAVIOR_REPORTED` | Server returned an EDE info-code normally produced by a recursive resolver. |
| `N18_SERVER_ERROR_REPORTED` | Server reported a server-side problem (prohibited, not authoritative, not supported) via EDE. |
| `TEST_CASE_END` | Testcase completion marker is emitted. |
| `TEST_CASE_START` | Testcase start marker is emitted. |

## Tag Arguments
| Tag | Argument key | Type | Meaning |
| --- | --- | --- | --- |
| `IPV4_DISABLED` | `ns` | `string` | Nameserver identity (`ns` name only; use `address` for IP) skipped on IPv4. |
| `IPV4_DISABLED` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `IPV4_DISABLED` | `rrtype` | `string` | rrtype skipped (`SOA`). |
| `IPV6_DISABLED` | `ns` | `string` | Nameserver identity (`ns` name only; use `address` for IP) skipped on IPv6. |
| `IPV6_DISABLED` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `IPV6_DISABLED` | `rrtype` | `string` | rrtype skipped (`SOA`). |
| `N18_EXTENDED_ERROR_REPORTED` | `info_code` | `int` | EDE info-code (RFC 8914). |
| `N18_EXTENDED_ERROR_REPORTED` | `info_name` | `string` | EDE info-code registry name, or `code <n>` when unnamed. |
| `N18_EXTENDED_ERROR_REPORTED` | `extra_text` | `string` | Sanitized EXTRA-TEXT (valid UTF-8, NUL-free, capped at 256 bytes). |
| `N18_EXTENDED_ERROR_REPORTED` | `servers` | `array<object>` | Structured sorted unique nameserver identities (`{ns,address}` object) returning this `(info_code, extra_text)`. |
| `N18_FILTERED_RESPONSE` | `info_code` | `int` | EDE info-code (RFC 8914). |
| `N18_FILTERED_RESPONSE` | `info_name` | `string` | EDE info-code registry name, or `code <n>` when unnamed. |
| `N18_FILTERED_RESPONSE` | `extra_text` | `string` | Sanitized EXTRA-TEXT (valid UTF-8, NUL-free, capped at 256 bytes). |
| `N18_FILTERED_RESPONSE` | `servers` | `array<object>` | Structured sorted unique nameserver identities (`{ns,address}` object) returning this `(info_code, extra_text)`. |
| `N18_NO_EXTENDED_ERROR` | `servers` | `array<object>` | Structured sorted unique nameserver identities (`{ns,address}` object). |
| `N18_NO_RESPONSE` | `servers` | `array<object>` | Structured sorted unique nameserver identities (`{ns,address}` object). |
| `N18_RESOLVER_BEHAVIOR_REPORTED` | `info_code` | `int` | EDE info-code (RFC 8914). |
| `N18_RESOLVER_BEHAVIOR_REPORTED` | `info_name` | `string` | EDE info-code registry name, or `code <n>` when unnamed. |
| `N18_RESOLVER_BEHAVIOR_REPORTED` | `extra_text` | `string` | Sanitized EXTRA-TEXT (valid UTF-8, NUL-free, capped at 256 bytes). |
| `N18_RESOLVER_BEHAVIOR_REPORTED` | `servers` | `array<object>` | Structured sorted unique nameserver identities (`{ns,address}` object) returning this `(info_code, extra_text)`. |
| `N18_SERVER_ERROR_REPORTED` | `info_code` | `int` | EDE info-code (RFC 8914). |
| `N18_SERVER_ERROR_REPORTED` | `info_name` | `string` | EDE info-code registry name, or `code <n>` when unnamed. |
| `N18_SERVER_ERROR_REPORTED` | `extra_text` | `string` | Sanitized EXTRA-TEXT (valid UTF-8, NUL-free, capped at 256 bytes). |
| `N18_SERVER_ERROR_REPORTED` | `servers` | `array<object>` | Structured sorted unique nameserver identities (`{ns,address}` object) returning this `(info_code, extra_text)`. |
| `TEST_CASE_END` | `testcase` | `string` | Testcase display name (`Nameserver18`). |
| `TEST_CASE_START` | `testcase` | `string` | Testcase display name (`Nameserver18`). |

## Severity Levels Per Tag
| Tag | Level | Notes |
| --- | --- | --- |
| `IPV4_DISABLED` | `DEBUG` | Default from `share/profile.json` (`test_levels.NAMESERVER`). |
| `IPV6_DISABLED` | `DEBUG` | Default from `share/profile.json` (`test_levels.NAMESERVER`). |
| `N18_EXTENDED_ERROR_REPORTED` | `NOTICE` | Default from `share/profile.json` (`test_levels.NAMESERVER`). |
| `N18_FILTERED_RESPONSE` | `WARNING` | Default from `share/profile.json` (`test_levels.NAMESERVER`). |
| `N18_NO_EXTENDED_ERROR` | `INFO` | Default from `share/profile.json` (`test_levels.NAMESERVER`). |
| `N18_NO_RESPONSE` | `WARNING` | Default from `share/profile.json` (`test_levels.NAMESERVER`). |
| `N18_RESOLVER_BEHAVIOR_REPORTED` | `WARNING` | Default from `share/profile.json` (`test_levels.NAMESERVER`). |
| `N18_SERVER_ERROR_REPORTED` | `WARNING` | Default from `share/profile.json` (`test_levels.NAMESERVER`). |
| `TEST_CASE_END` | `DEBUG` | Default from `share/profile.json` (`test_levels.NAMESERVER`). |
| `TEST_CASE_START` | `DEBUG` | Default from `share/profile.json` (`test_levels.NAMESERVER`). |

## Differences From Upstream
- Upstream reference: none; this is a gonemaster testcase.

## Edge Cases And Limitations
- EDE is purely diagnostic: it never changes the RCODE and never alters DNS protocol processing. This testcase ships with zero numeric penalty, but WARNING-level findings still suppress the A+ bonus.
- Findings are aggregated by `(info_code, extra_text)`, so the same code with different text yields separate findings, and the same code+text across servers yields one finding listing all servers.
- The resolver-behavior class is a hypothesis, not a verdict: a resolver-oriented code from a delegated address can have legitimate or ambiguous sources (combined authoritative+recursive deployments, hidden primaries behind forwarders, anycast nodes). The message is phrased as a possibility and carries no numeric penalty.
- EXTRA-TEXT is free-form bytes from an arbitrary nameserver; it is sanitized to valid UTF-8, stripped of a trailing NUL, trimmed, and capped at 256 bytes with a truncation marker. It is never parsed for logic.
- Draft-assigned info-codes that the dns library registry does not name (e.g. 31, 32, 33) render as `code <n>` until a future library release names them; the classification is by numeric code and is unaffected.
