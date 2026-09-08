# Connectivity05

Status: Draft

## Purpose
- Determine whether the zone apex `DNSKEY` answer, the largest routine answer of a signed zone, is delivered over UDP by each authoritative address.
- Determine what an address does when a client advertises an EDNS UDP payload at least as large as that answer: it delivers the answer, it truncates, or it produces no answer at all.
- Distinguish size-dependent UDP loss from generic unreachability, which other testcases already report.

## Preconditions And Inputs
- Preconditions:
  - A `zone.Zone` object is available.
- Required inputs:
  - Nameserver addresses from [`ZoneNameservers`](../../nameserver-resolution.md#zonenameservers).
  - Child zone name (`z.Name`).
- Profile/config knobs that affect behavior:
  - `net.ipv4` and `net.ipv6`: disabled families are skipped per address.
  - `resolver.defaults.parallel`: per-address task fan-out.
  - `resolver.defaults.fallback`: when false, the truncated `DNSKEY` answer is not fetched over TCP by the transport and the testcase issues one forced-TCP query instead.
  - `resolver.defaults.timeout`: bounds the time a lost probe costs.

## Algorithm And Decision Flow

All queries are for `z.Name`, type `DNSKEY`, class `IN`. `D` is the default advertised
EDNS UDP payload for DNSSEC queries (`constants.EDNSUDPPayloadDNSSECDefault`, 1232 bytes).
`S` is the answer size in bytes as defined under Edge Cases And Limitations.

1. Emit `TEST_CASE_START`.
2. For each nameserver address (parallelized):
   1. If the address family is disabled, emit `IPV4_DISABLED` or `IPV6_DISABLED` for rrtype `DNSKEY` and skip the address.
   2. Issue the reference query with DNSSEC enabled and otherwise default options, so it shares the per-run response cache entry with the `DNSKEY` queries of the DNSSEC module. Classify the outcome:
      - No message: continue at step 2.5 (loss branch).
      - `RCODE != NOERROR`: inconclusive for this testcase, which reports delivery and not response content. Emit nothing for the address.
      - Answered over UDP and not truncated: the answer fits within `D`. Record `S` for the `CN05_ANSWER_FITS_UDP` summary and stop.
      - Answered over TCP: the answer was truncated at `D` and the transport fell back. `S` is the size of the TCP answer. Continue at step 2.3.
      - Truncated over UDP with no fallback: learn `S` with one forced-TCP query. If that query does not return a NOERROR answer, emit nothing for the address, since TCP failures belong to Connectivity02. Otherwise continue at step 2.3.
   3. Record `CN05_ANSWER_NEEDS_TCP` for the address, then decide whether a probe is warranted:
      - `S <= D`: the address truncates below the payload it was offered, so no larger advertisement can change the outcome. Stop.
      - `S > 4096`: no client advertising 4096 bytes or less can receive the answer over UDP. Stop.
      - Otherwise continue at step 2.4.
   4. Issue the probe: one UDP query with DNSSEC enabled, an advertised payload `P`, UDP-to-TCP fallback disabled, a retry count of 1, and the diagnostic flag set so its failure is excluded from server-health bookkeeping. `P` is the smallest multiple of 256 strictly greater than `S`, capped at 4096. Classify:
      - No message: `CN05_LARGE_ANSWER_NO_UDP_ANSWER`.
      - Truncated: `CN05_SERVER_CAPS_UDP_ANSWER`. The address limits its own UDP answer below `P`, so clients receive a truncated response rather than silence.
      - `NOERROR` and not truncated: `CN05_LARGE_ANSWER_DELIVERED_UDP`.
      - Any other RCODE: inconclusive. Emit nothing further for the address.
   5. Loss branch, entered when the reference query yielded no message. Issue the small-answer probe with the option set of Nameserver13 (DNSSEC enabled, EDNS version 0, advertised payload 512, fallback disabled) so it shares that testcase's cache entry. Classify:
      - No message: EDNS queries do not reach the address at all, or the address is unreachable. This is not a size question and is reported by Nameserver02, Nameserver13 and DNSSEC07. Emit nothing.
      - Answered without truncation: the answer fits 512 bytes, so the loss at `D` was not size-dependent. Emit nothing.
      - Answered with truncation: a small answer arrives and the full answer exceeds 512 bytes. Learn `S` with one forced-TCP query. If TCP returns a NOERROR answer, record `CN05_UDP_LOSS_SIZE_DEPENDENT`. Otherwise emit nothing.
3. Group the recorded per-address outcomes by identical tag, `size` and `payload`, and emit one entry per group with the addresses of that group in `servers`. Emit `CN05_ANSWER_FITS_UDP` once for the addresses that answered over UDP at `D`, with `size` set to the largest `S` among them.
4. Emit `TEST_CASE_END`.

An absent message is `err != nil` or a nil message. A cached no-response marker replays as
an empty packet with a nil error, so both conditions are tested together.

### Per-Address Classification (step 2)

{{% expand "Show diagram" %}}
```
For each address (parallel; fan-out = resolver.defaults.parallel):

   family disabled -> IPV4_DISABLED / IPV6_DISABLED (query_type=DNSKEY); skip address

   Q1: DNSKEY, DNSSEC=true, default options (shared with the DNSSEC module)
    +- no message            -> loss branch (below)
    +- RCODE != NOERROR      -> emit nothing for this address
    +- UDP, not truncated    -> record for CN05_ANSWER_FITS_UDP; stop
    +- TCP (fallback taken)  -> S = len(TCP answer); needs-TCP branch
    +- UDP, truncated        -> forced-TCP query
                                 +- no NOERROR answer -> emit nothing
                                 +- NOERROR answer    -> S = len(answer); needs-TCP
                                                         branch

   needs-TCP branch: record CN05_ANSWER_NEEDS_TCP (size=S, payload=1232)
    +- S <= 1232 -> stop (address truncates below the offered payload)
    +- S >  4096 -> stop (unreachable over UDP for any client)
    +- else      -> probe

   probe: DNSKEY, DNSSEC=true, EDNS_SIZE=P, FALLBACK=false, RETRY=1, diagnostic
          P = min(4096, 256 * (floor(S / 256) + 1))
    +- no message         -> CN05_LARGE_ANSWER_NO_UDP_ANSWER (size=S, payload=P)
    +- truncated          -> CN05_SERVER_CAPS_UDP_ANSWER     (size=S, payload=P)
    +- NOERROR, complete  -> CN05_LARGE_ANSWER_DELIVERED_UDP (size=|answer|, payload=P)
    +- other RCODE        -> emit nothing
```
{{% /expand %}}

### Loss Branch (step 2.5)

{{% expand "Show diagram" %}}
```
Q1 yielded no message at payload 1232.

   Q2: DNSKEY, DNSSEC=true, EDNS version 0, EDNS_SIZE=512, FALLBACK=false
       (identical options to Nameserver13, so the cache entry is shared)
    +- no message        -> emit nothing (not a size question)
    +- not truncated     -> emit nothing (answer fits 512; loss was transient)
    +- truncated         -> forced-TCP query
                              +- no NOERROR answer -> emit nothing
                              +- NOERROR answer    -> CN05_UDP_LOSS_SIZE_DEPENDENT
                                                      (size=S, payload=1232)
```
{{% /expand %}}

### Query Budget

| Address outcome | Queries beyond those other testcases already issue |
| --- | --- |
| Unsigned zone, or `DNSKEY` answer within 1232 bytes | 0 |
| Answer above 1232 bytes delivered over UDP at 1232 | 0 |
| Answer above 1232 bytes truncated at 1232 and fetched over TCP | 1 (the probe) |
| Truncated at 1232 with `resolver.defaults.fallback` false | 2 (forced TCP, probe) |
| No answer at 1232, small-answer probe truncated | 1 (forced TCP) |

## Emitted Tags (Possible Set)
| Tag | Emitted when |
| --- | --- |
| `CN05_ANSWER_FITS_UDP` | The `DNSKEY` answer arrived over UDP, untruncated, within the default advertised payload. |
| `CN05_ANSWER_NEEDS_TCP` | The `DNSKEY` answer was truncated at the default advertised payload and had to be fetched over TCP. |
| `CN05_LARGE_ANSWER_DELIVERED_UDP` | The probe received the complete answer over UDP at the larger advertised payload. |
| `CN05_LARGE_ANSWER_NO_UDP_ANSWER` | The probe received no answer at the larger advertised payload. |
| `CN05_SERVER_CAPS_UDP_ANSWER` | The probe received a truncated answer although the larger payload permitted the full one. |
| `CN05_UDP_LOSS_SIZE_DEPENDENT` | No answer arrived at the default advertised payload, a 512-byte advertisement was answered with truncation, and TCP delivered the answer. |
| `IPV4_DISABLED` | The address is IPv4 and IPv4 is disabled. |
| `IPV6_DISABLED` | The address is IPv6 and IPv6 is disabled. |
| `TEST_CASE_END` | Testcase completion marker is emitted. |
| `TEST_CASE_START` | Testcase start marker is emitted. |

## Tag Arguments
| Tag | Argument key | Type | Meaning |
| --- | --- | --- | --- |
| `CN05_ANSWER_FITS_UDP` | `size` | `int` | Largest observed answer size in bytes among the listed addresses. |
| `CN05_ANSWER_FITS_UDP` | `payload` | `int` | Advertised EDNS UDP payload in bytes (1232). |
| `CN05_ANSWER_FITS_UDP` | `query_type` | `string` | Query type used (`DNSKEY`). |
| `CN05_ANSWER_FITS_UDP` | `servers` | `array<object>` | Structured nameserver items (`{ns,address}` object) with this outcome. |
| `CN05_ANSWER_NEEDS_TCP` | `size` | `int` | Answer size in bytes as delivered over TCP. |
| `CN05_ANSWER_NEEDS_TCP` | `payload` | `int` | Advertised EDNS UDP payload in bytes (1232). |
| `CN05_ANSWER_NEEDS_TCP` | `query_type` | `string` | Query type used (`DNSKEY`). |
| `CN05_ANSWER_NEEDS_TCP` | `servers` | `array<object>` | Structured nameserver items (`{ns,address}` object) with this outcome. |
| `CN05_LARGE_ANSWER_DELIVERED_UDP` | `size` | `int` | Size in bytes of the answer delivered over UDP. |
| `CN05_LARGE_ANSWER_DELIVERED_UDP` | `payload` | `int` | Advertised EDNS UDP payload in bytes used by the probe. |
| `CN05_LARGE_ANSWER_DELIVERED_UDP` | `query_type` | `string` | Query type used (`DNSKEY`). |
| `CN05_LARGE_ANSWER_DELIVERED_UDP` | `servers` | `array<object>` | Structured nameserver items (`{ns,address}` object) with this outcome. |
| `CN05_LARGE_ANSWER_NO_UDP_ANSWER` | `size` | `int` | Answer size in bytes as learned over TCP. |
| `CN05_LARGE_ANSWER_NO_UDP_ANSWER` | `payload` | `int` | Advertised EDNS UDP payload in bytes used by the probe. |
| `CN05_LARGE_ANSWER_NO_UDP_ANSWER` | `query_type` | `string` | Query type used (`DNSKEY`). |
| `CN05_LARGE_ANSWER_NO_UDP_ANSWER` | `servers` | `array<object>` | Structured nameserver items (`{ns,address}` object) with this outcome. |
| `CN05_SERVER_CAPS_UDP_ANSWER` | `size` | `int` | Answer size in bytes as learned over TCP. |
| `CN05_SERVER_CAPS_UDP_ANSWER` | `payload` | `int` | Advertised EDNS UDP payload in bytes used by the probe. |
| `CN05_SERVER_CAPS_UDP_ANSWER` | `query_type` | `string` | Query type used (`DNSKEY`). |
| `CN05_SERVER_CAPS_UDP_ANSWER` | `servers` | `array<object>` | Structured nameserver items (`{ns,address}` object) with this outcome. |
| `CN05_UDP_LOSS_SIZE_DEPENDENT` | `size` | `int` | Answer size in bytes as learned over TCP. |
| `CN05_UDP_LOSS_SIZE_DEPENDENT` | `payload` | `int` | Advertised EDNS UDP payload in bytes that produced no answer (1232). |
| `CN05_UDP_LOSS_SIZE_DEPENDENT` | `query_type` | `string` | Query type used (`DNSKEY`). |
| `CN05_UDP_LOSS_SIZE_DEPENDENT` | `servers` | `array<object>` | Structured nameserver items (`{ns,address}` object) with this outcome. |
| `IPV4_DISABLED` | `ns` | `string` | Nameserver identity (`ns` name only; use `address` for IP) skipped on IPv4. |
| `IPV4_DISABLED` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `IPV4_DISABLED` | `query_type` | `string` | Query type skipped (`DNSKEY`). |
| `IPV6_DISABLED` | `ns` | `string` | Nameserver identity (`ns` name only; use `address` for IP) skipped on IPv6. |
| `IPV6_DISABLED` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `IPV6_DISABLED` | `query_type` | `string` | Query type skipped (`DNSKEY`). |
| `TEST_CASE_END` | `testcase` | `string` | Testcase display name (`Connectivity05`). |
| `TEST_CASE_START` | `testcase` | `string` | Testcase display name (`Connectivity05`). |

## Severity Levels Per Tag
| Tag | Level | Notes |
| --- | --- | --- |
| `CN05_ANSWER_FITS_UDP` | `INFO` | Default from `share/profile.json` (`test_levels.CONNECTIVITY`). |
| `CN05_ANSWER_NEEDS_TCP` | `NOTICE` | Default from `share/profile.json` (`test_levels.CONNECTIVITY`). Scored at penalty 0: truncation of a large signed answer is correct behavior, and the tag exists for visibility. |
| `CN05_LARGE_ANSWER_DELIVERED_UDP` | `INFO` | Default from `share/profile.json` (`test_levels.CONNECTIVITY`). |
| `CN05_LARGE_ANSWER_NO_UDP_ANSWER` | `WARNING` | Default from `share/profile.json` (`test_levels.CONNECTIVITY`). Not raised above `WARNING`: the observation is made from a single vantage point. |
| `CN05_SERVER_CAPS_UDP_ANSWER` | `INFO` | Default from `share/profile.json` (`test_levels.CONNECTIVITY`). |
| `CN05_UDP_LOSS_SIZE_DEPENDENT` | `WARNING` | Default from `share/profile.json` (`test_levels.CONNECTIVITY`). Not raised above `WARNING`, for the same reason. |
| `IPV4_DISABLED` | `DEBUG2` | Default from `share/profile.json` (`test_levels.CONNECTIVITY`). |
| `IPV6_DISABLED` | `DEBUG2` | Default from `share/profile.json` (`test_levels.CONNECTIVITY`). |
| `TEST_CASE_END` | `DEBUG` | Default from `share/profile.json` (`test_levels.CONNECTIVITY`). |
| `TEST_CASE_START` | `DEBUG` | Default from `share/profile.json` (`test_levels.CONNECTIVITY`). |

## Differences From Upstream
- Upstream reference: No upstream Zonemaster equivalent. Connectivity05 is a new testcase unique to Gonemaster.
- Differences (Upstream vs Gonemaster):
  - Upstream: no testcase measures whether a zone's largest answer is delivered over UDP, and truncation followed by a TCP retry is not reported. Gonemaster: reports the answer size, whether the default advertised payload delivers it, and the outcome of advertising a payload at least as large as the answer.
- Potential upstream report:
  - `no`

## Edge Cases And Limitations
- The result of the probe is a property of the path between this Gonemaster instance and the address, not of the address alone. A different vantage point may legitimately observe a different outcome, which is why the delivery failures stay at `WARNING`. The tag wording states what was observed and does not attribute a cause.
- IPv6 is more exposed than IPv4: the minimum link MTU is 1280 bytes, routers do not fragment, and fragment filtering is common. A per-family difference is visible in `servers`.
- `S` is the length of the response as read from the wire when that length is recorded, and an uncompressed estimate otherwise. A restored cache holds messages repacked at save time, so a replayed run may report a `size` that differs from the live run by the compression delta. The choice of `P` is unaffected, since `P` rounds up to a multiple of 256.
- Undelegated runs and fake delegations return synthesized responses with no transport recorded. An unknown transport is never read as UDP.
- An address that answers a 1232-byte advertisement with a larger response violates RFC 6891, section 6.2.5. It is reported as delivered, with `payload` 1232 and `size` above it.
- An address that ignores the advertised payload entirely and loses both the 1232-byte and the 512-byte answer makes the loss branch conclude that the loss is not size-dependent. This is an accepted false negative.
- `REFUSED` on the probe, which some addresses return after a burst of queries, is treated as inconclusive rather than as loss.
- The probe is issued only after the same address answered the reference query, so a UDP fast-fail or reachability blackout is unlikely to suppress it. Should it be suppressed, the empty response would be read as loss.

## Evidence In Gonemaster
- Code paths:
  - `engine/test/connectivity/connectivity.go`
  - `engine/nameserver/nameserver.go` (`QueryOptions.Diagnostic`, which keeps the probe out of timeout, fast-fail, error-cache, reachability and latency-budget bookkeeping)
- Related tests:
  - `engine/test/connectivity/connectivity_test.go`
  - `engine/nameserver/diagnostic_query_test.go`

## Open Questions
- Whether `CN05_ANSWER_NEEDS_TCP` should keep penalty 0 once its prevalence across a large cohort is known.
- Whether an address answering above the advertised payload deserves a tag of its own.
