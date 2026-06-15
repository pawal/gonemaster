# Nameserver17

Status: Final

## Purpose
- Probe each authoritative nameserver for DNS Cookie (RFC 7873 / RFC 9018) support and verify the returned Server Cookie is well-formed and accepted by the issuing server.
- Recognise servers that enforce DNS Cookies (answering a client-only probe with BADCOOKIE plus a fresh, well-formed Server Cookie, RFC 7873 5.2.3 / 5.3) as the strongest cookie posture, rather than discarding them as an RCODE anomaly.

## Preconditions And Inputs
- Preconditions:
  - A `zone.Zone` object is available.
- Required inputs:
  - Nameserver addresses from [`ZoneNameservers`](../../nameserver-resolution.md#zonenameservers).
  - A per-run random 8-byte Client Cookie, hex-encoded, reused read-only across the run's probes.
  - SOA responses to UDP queries carrying a COOKIE option (option code `10`).
- Profile/config knobs that affect behavior:
  - `net.ipv4` and `net.ipv6`: disabled transports are skipped with transport debug tags.
  - `resolver.defaults.parallel`: parallel nameserver query fanout.

## Algorithm And Decision Flow
1. Emit `TEST_CASE_START`.
2. Generate one random 8-byte Client Cookie for the run (`crypto/rand`, hex-encoded).
3. Initialize per-behaviour collectors keyed by nameserver.
4. Read nameserver list from [`ZoneNameservers`](../../nameserver-resolution.md#zonenameservers).
5. For each nameserver (parallelized, input-order merged logs):
   - If transport is disabled, emit `IPV4_DISABLED` or `IPV6_DISABLED` for rrtype `SOA`, then skip.
   - Query 1 (support and well-formedness): send a UDP-pinned SOA query for the zone name carrying the COOKIE option with only the 8-byte Client Cookie. UDP is pinned because TCP return-routability lets a server skip cookie processing.
     - No response, or a truncated (TC=1) response: collect for `N17_NO_RESPONSE` (a truncated probe is inconclusive and is not retried over TCP).
     - `BADCOOKIE` (RCODE 23) carrying a well-formed full Server Cookie (validated with the same length and client-echo checks as the NOERROR path): the server enforces DNS Cookies, the strongest posture (RFC 7873 5.2.3 / 5.3). Collect for `N17_COOKIE_ENFORCED` and proceed to query 2 with the returned full cookie.
     - Any other `RCODE != NOERROR` (REFUSED / SERVFAIL / FORMERR, or a `BADCOOKIE` without a well-formed Server Cookie): no cookie verdict (RCODE anomalies are graded by basic/N16).
     - NOERROR, no COOKIE option: collect for `N17_NO_COOKIE`.
     - NOERROR, COOKIE length not in `{8} U [16,40]` bytes, or the client portion does not echo our Client Cookie: collect for `N17_COOKIE_MALFORMED` with the observed `cookie_bytes`.
     - NOERROR, COOKIE exactly 8 bytes (only the Client Cookie echoed): collect for `N17_COOKIE_CLIENT_ONLY`.
     - NOERROR, well-formed full cookie (16-40 bytes, client portion echoed; 24 bytes = RFC 9018 v1): collect for `N17_COOKIE_SUPPORTED` and proceed to query 2.
   - Query 2 (round-trip, for both `N17_COOKIE_SUPPORTED` and `N17_COOKIE_ENFORCED`): re-query with the full cookie returned by query 1.
     - NOERROR: collect for `N17_COOKIE_ROUNDTRIP_OK` (proves the server did not reject its own cookie, not that it validated it).
     - BADCOOKIE: retry once (query 2b) with the fresh Server Cookie the BADCOOKIE reply carries (RFC 7873 5.3). If query 2b is NOERROR, the server merely rotated its cookie: collect for `N17_COOKIE_ROUNDTRIP_OK`. If query 2b is also BADCOOKIE, collect for `N17_COOKIE_SELF_REJECT`.
6. Emit `N17_COOKIE_SUPPORTED` with sorted unique `servers` when non-empty.
7. Emit `N17_COOKIE_ENFORCED` with sorted unique `servers` when non-empty.
8. Emit `N17_NO_COOKIE` with sorted unique `servers` when non-empty.
9. Emit `N17_COOKIE_ROUNDTRIP_OK` with sorted unique `servers` when non-empty.
10. Emit `N17_COOKIE_CLIENT_ONLY` with sorted unique `servers` when non-empty.
11. Emit `N17_COOKIE_MALFORMED` per `cookie_bytes` value with sorted unique `servers` when non-empty.
12. Emit `N17_COOKIE_SELF_REJECT` with sorted unique `servers` when non-empty.
13. Emit `N17_NO_RESPONSE` with sorted unique `servers` when non-empty.
14. Emit `TEST_CASE_END`.

Per-nameserver decision flow:

```
per nameserver (parallel):
 +- transport disabled (IPv4/IPv6)        -> IPV4_DISABLED / IPV6_DISABLED, skip
 +- otherwise
       |
       v
   query 1: UDP-pinned SOA + COOKIE(client cookie)
    +- no response / TC=1                       -> N17_NO_RESPONSE
    +- BADCOOKIE + well-formed Server Cookie    -> N17_COOKIE_ENFORCED (then query 2, as N17_COOKIE_SUPPORTED)
    +- other RCODE != NOERROR                   -> no cookie verdict (graded by basic/N16)
    +- NOERROR, no COOKIE option                -> N17_NO_COOKIE
    +- COOKIE len not in {8} U [16,40] B        -> N17_COOKIE_MALFORMED (cookie_bytes)
    +- client portion != our cookie             -> N17_COOKIE_MALFORMED (cookie_bytes)
    +- COOKIE == 8 B (client cookie only)       -> N17_COOKIE_CLIENT_ONLY
    +- well-formed full cookie (16-40 B)        -> N17_COOKIE_SUPPORTED
                                                   |
                                                   v
        query 2: UDP-pinned SOA + COOKIE(returned full cookie)
         +- NOERROR                         -> N17_COOKIE_ROUNDTRIP_OK
         +- BADCOOKIE
              |
              v
           query 2b: retry with the fresh Server Cookie from the BADCOOKIE reply
            +- NOERROR                      -> N17_COOKIE_ROUNDTRIP_OK (cookie rotation)
            +- BADCOOKIE                    -> N17_COOKIE_SELF_REJECT
```

## Emitted Tags (Possible Set)
| Tag | Emitted when |
| --- | --- |
| `IPV4_DISABLED` | IPv4 nameserver evaluation is skipped because IPv4 is disabled. |
| `IPV6_DISABLED` | IPv6 nameserver evaluation is skipped because IPv6 is disabled. |
| `N17_COOKIE_CLIENT_ONLY` | Server echoed only the 8-byte Client Cookie and returned no Server Cookie. |
| `N17_COOKIE_ENFORCED` | Server enforces DNS Cookies: it answered a client-only probe with BADCOOKIE and a well-formed Server Cookie. |
| `N17_COOKIE_MALFORMED` | Server returned a cookie of an invalid length, or a wrong client-cookie echo. |
| `N17_COOKIE_ROUNDTRIP_OK` | Server did not reject the Server Cookie it issued on a follow-up query. |
| `N17_COOKIE_SELF_REJECT` | Server rejected with BADCOOKIE, even after the RFC retry, a Server Cookie it had just issued. |
| `N17_COOKIE_SUPPORTED` | Server returned a well-formed full DNS Cookie. |
| `N17_NO_COOKIE` | Server responded NOERROR but returned no COOKIE option. |
| `N17_NO_RESPONSE` | Cookie probe produced no DNS response, or a truncated response. |
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
| `N17_COOKIE_CLIENT_ONLY` | `servers` | `array<object>` | Structured sorted unique nameserver identities (`{ns,address}` object). |
| `N17_COOKIE_ENFORCED` | `servers` | `array<object>` | Structured sorted unique nameserver identities (`{ns,address}` object). |
| `N17_COOKIE_MALFORMED` | `cookie_bytes` | `int` | Observed COOKIE option length in bytes. |
| `N17_COOKIE_MALFORMED` | `servers` | `array<object>` | Structured sorted unique nameserver identities (`{ns,address}` object) for that cookie length. |
| `N17_COOKIE_ROUNDTRIP_OK` | `servers` | `array<object>` | Structured sorted unique nameserver identities (`{ns,address}` object). |
| `N17_COOKIE_SELF_REJECT` | `servers` | `array<object>` | Structured sorted unique nameserver identities (`{ns,address}` object). |
| `N17_COOKIE_SUPPORTED` | `servers` | `array<object>` | Structured sorted unique nameserver identities (`{ns,address}` object). |
| `N17_NO_COOKIE` | `servers` | `array<object>` | Structured sorted unique nameserver identities (`{ns,address}` object). |
| `N17_NO_RESPONSE` | `servers` | `array<object>` | Structured sorted unique nameserver identities (`{ns,address}` object). |
| `TEST_CASE_END` | `testcase` | `string` | Testcase display name (`Nameserver17`). |
| `TEST_CASE_START` | `testcase` | `string` | Testcase display name (`Nameserver17`). |

## Severity Levels Per Tag
| Tag | Level | Notes |
| --- | --- | --- |
| `IPV4_DISABLED` | `DEBUG` | Default from `share/profile.json` (`test_levels.NAMESERVER`). |
| `IPV6_DISABLED` | `DEBUG` | Default from `share/profile.json` (`test_levels.NAMESERVER`). |
| `N17_COOKIE_CLIENT_ONLY` | `WARNING` | Default from `share/profile.json` (`test_levels.NAMESERVER`). |
| `N17_COOKIE_ENFORCED` | `INFO` | Default from `share/profile.json` (`test_levels.NAMESERVER`). |
| `N17_COOKIE_MALFORMED` | `WARNING` | Default from `share/profile.json` (`test_levels.NAMESERVER`). |
| `N17_COOKIE_ROUNDTRIP_OK` | `INFO` | Default from `share/profile.json` (`test_levels.NAMESERVER`). |
| `N17_COOKIE_SELF_REJECT` | `WARNING` | Default from `share/profile.json` (`test_levels.NAMESERVER`). |
| `N17_COOKIE_SUPPORTED` | `INFO` | Default from `share/profile.json` (`test_levels.NAMESERVER`). |
| `N17_NO_COOKIE` | `INFO` | Default from `share/profile.json` (`test_levels.NAMESERVER`). |
| `N17_NO_RESPONSE` | `WARNING` | Default from `share/profile.json` (`test_levels.NAMESERVER`). |
| `TEST_CASE_END` | `DEBUG` | Default from `share/profile.json` (`test_levels.NAMESERVER`). |
| `TEST_CASE_START` | `DEBUG` | Default from `share/profile.json` (`test_levels.NAMESERVER`). |

## Differences From Upstream
- Upstream reference: none; this is a gonemaster testcase.

## Edge Cases And Limitations
- "No cookie in the response" is ambiguous: it can mean unsupported, disabled, or silently stripped by the server or a middlebox. It is treated as a negative observation (`N17_NO_COOKIE`, informational), never as proof of misconfiguration.
- A NOERROR round-trip proves only that the server did not reject the cookie it issued, not that it validated it; a non-enforcing server returns NOERROR without validating (RFC 7873 5.2.3).
- A single vantage point cannot prove the RFC 9018 SipHash-2-4 construction or anycast secret consistency. `N17_COOKIE_SELF_REJECT` fires only after the RFC-mandated retry and may still reflect transient anycast secret skew rather than a defect; cross-node anycast detection is out of scope.
- A truncated (TC=1) probe is treated as inconclusive and is not retried over TCP, since TCP return-routability would mask cookie enforcement.
- The BADCOOKIE carve-out for `N17_COOKIE_ENFORCED` is deliberately narrow: only RCODE 23 carrying a well-formed Server Cookie (correct length and client-cookie echo) qualifies. A BADCOOKIE without a well-formed Server Cookie, and any other non-NOERROR RCODE (REFUSED / SERVFAIL / FORMERR), stay generic anomalies graded by basic/N16, so a transient or malformed BADCOOKIE never produces a cookie verdict here.
- `N17_COOKIE_ENFORCED` proves only that the server demanded and issued a Server Cookie; the follow-up round-trip (`N17_COOKIE_ROUNDTRIP_OK`) still proves only non-rejection, not cryptographic validation, and a single vantage point cannot prove the RFC 9018 construction.
- A FORMERR to a valid 8-byte Client Cookie is itself a server defect (RFC 7873 5.2.2), but it is left to basic/N16 and adds no dedicated tag here.
