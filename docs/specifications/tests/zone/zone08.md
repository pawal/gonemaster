# Zone08

Status: Final

## Purpose
- Validate that MX exchange hostnames are not aliases (CNAME).
- RFC 2181 section 10.3 states that the domain name used as part of the value
  of an MX resource record must not be an alias, and that the name may carry
  any RR but never a CNAME RR.
- RFC 5321 section 5.1 requires that name, when queried, to return at least one
  address record, and places a response that returns a CNAME outside the scope
  of the standard.

## Preconditions And Inputs
- Preconditions:
  - A `zone.Zone` object is available.
- Required inputs:
  - Authoritative MX response for child zone apex (`queryAuth`).
  - Authoritative A response for each MX exchange hostname other than the root
    (`queryAuth`).
- Profile/config knobs that affect behavior:
  - No testcase-local profile knob.

## Algorithm And Decision Flow
1. Emit `TEST_CASE_START`.
2. Query authoritative `MX` for zone apex.
3. If no response, emit `NO_RESPONSE_MX_QUERY`.
4. Else, for each MX RR in apex answer:
   - if the exchange is the root name (`.`, the null MX of RFC 7505), emit no
     tag for that exchange; Zone09 evaluates null MX records;
   - query authoritative `A` for the exchange;
   - if the answer section carries a CNAME RR owned by the exchange, emit
     `MX_RECORD_IS_CNAME`;
   - else if the RCODE is `NOERROR` or `NXDOMAIN`, emit
     `MX_RECORD_IS_NOT_CNAME`;
   - else, including when no nameserver of the zone returned an authoritative
     response, emit `MX_RECORD_NOT_CHECKED`;
   - each of these tags carries the exchange hostname in `mx`.
5. Emit `TEST_CASE_END`.

## Emitted Tags (Possible Set)
| Tag | Emitted when |
| --- | --- |
| `MX_RECORD_IS_CNAME` | The authoritative A response for an MX exchange carries a CNAME RR owned by the exchange in its answer section. |
| `MX_RECORD_IS_NOT_CNAME` | The authoritative A response for an MX exchange carries no CNAME RR owned by the exchange and has RCODE `NOERROR` or `NXDOMAIN`. |
| `MX_RECORD_NOT_CHECKED` | No nameserver of the zone returned an authoritative A response for an MX exchange, or the response carries no CNAME RR owned by the exchange and has an RCODE other than `NOERROR` and `NXDOMAIN`. |
| `NO_RESPONSE_MX_QUERY` | Apex MX query returned no response. |
| `TEST_CASE_END` | Testcase completion marker is emitted. |
| `TEST_CASE_START` | Testcase start marker is emitted. |

## Tag Arguments
| Tag | Argument key | Type | Meaning |
| --- | --- | --- | --- |
| `MX_RECORD_IS_CNAME` | `mx` | `string` | Normalized MX exchange hostname that resolves as an alias. |
| `MX_RECORD_IS_NOT_CNAME` | `mx` | `string` | Normalized MX exchange hostname that is not an alias. |
| `MX_RECORD_NOT_CHECKED` | `mx` | `string` | Normalized MX exchange hostname whose alias status is not determined. |
| `NO_RESPONSE_MX_QUERY` | `-` | `-` | No arguments. |
| `TEST_CASE_END` | `testcase` | `string` | Testcase display name (`Zone08`). |
| `TEST_CASE_START` | `testcase` | `string` | Testcase display name (`Zone08`). |

## Severity Levels Per Tag
| Tag | Level | Notes |
| --- | --- | --- |
| `MX_RECORD_IS_CNAME` | `ERROR` | Default from `share/profile.json` (`test_levels.ZONE`). RFC 5321 section 5.1 requires the exchange to return an address record. |
| `MX_RECORD_IS_NOT_CNAME` | `INFO` | Default from `share/profile.json` (`test_levels.ZONE`). |
| `MX_RECORD_NOT_CHECKED` | `INFO` | Default from `share/profile.json` (`test_levels.ZONE`). |
| `NO_RESPONSE_MX_QUERY` | `DEBUG` | Default from `share/profile.json` (`test_levels.ZONE`). |
| `TEST_CASE_END` | `DEBUG` | Default from `share/profile.json` (`test_levels.ZONE`). |
| `TEST_CASE_START` | `DEBUG` | Default from `share/profile.json` (`test_levels.ZONE`). |

## Differences From Upstream
- Differences (Upstream vs Gonemaster):
  - Upstream: the test plan checks whether "the MX answer is a CNAME" and names no query type for the exchange; the upstream engine queries each exchange for `CNAME`. Gonemaster: queries each exchange for `A` and reads a CNAME RR owned by the exchange from the answer section. RFC 1034 section 4.3.2 places the CNAME RR of an alias in the answer to any query type other than `CNAME`, so an alias yields `MX_RECORD_IS_CNAME` under both queries, while a server that synthesises `<name> CNAME <name>` for a `CNAME` question yields no CNAME RR for an `A` question.
  - Upstream: the test plan does not exclude the null MX exchange `.`, and the upstream engine queries it. Gonemaster: skips the exchange `.`; Zone09 evaluates null MX records.
  - Upstream: the upstream engine emits `MX_RECORD_IS_NOT_CNAME` for every authoritative response without a CNAME RR, whatever its RCODE. Gonemaster: emits `MX_RECORD_NOT_CHECKED` for an RCODE other than `NOERROR` and `NXDOMAIN`.
  - Upstream: emits no tag for an exchange without an authoritative answer. Gonemaster: emits `MX_RECORD_NOT_CHECKED`.
  - Upstream: describes a high-level authoritative MX/CNAME check. Gonemaster: performs explicit per-exchange A probes and emits explicit positive/negative tags (`MX_RECORD_IS_CNAME` / `MX_RECORD_IS_NOT_CNAME`).
  - Upstream: does not describe testcase boundary debug markers. Gonemaster: emits `TEST_CASE_START` and `TEST_CASE_END`.
  - Upstream: does not describe explicit no-response MX tag. Gonemaster: emits `NO_RESPONSE_MX_QUERY`.
  - Upstream: verdict messages name no exchange. Gonemaster: both verdict messages name the exchange (`mx`), so a zone with several MX records yields one distinguishable entry per exchange.
- Potential upstream report:
  - `yes`
- If yes, include:
  - Upstream expected behavior: An exchange whose authoritative servers answer a `CNAME` question with a synthesised `<name> CNAME <name>` RR yields `MX_RECORD_IS_CNAME` at `ERROR`, although an `A` question returns an address record and mail delivery works.
  - Gonemaster observed behavior: The same exchange yields `MX_RECORD_IS_NOT_CNAME`, because the `A` response carries no CNAME RR.
  - evidence: live cases `agn.se` (exchange `localhost.`, servers `ns1.sedoparking.com` and `ns2.sedoparking.com`) and `1a.se` (exchange `1a.se.`, servers `ns1.triop.se` and `ns2.triop.se`), observed 2026-10-04. Tracked as `DIV-ZONE08-CNAME-QUERY` in [known-behavior-divergences.md](../../known-behavior-divergences.md).
  - report status: `filed` by a third party ([zonemaster-engine#1558](https://github.com/zonemaster/zonemaster-engine/issues/1558))

## Edge Cases And Limitations
- Multiple MX RRs yield one tag per exchange, distinguished by `mx`.
- MX RRs naming the same exchange share one A query through the per-run cache.
- An exchange outside the zone yields `MX_RECORD_IS_CNAME` or `MX_RECORD_IS_NOT_CNAME` only when a nameserver of the zone answers the A query with `AA` set, and `MX_RECORD_NOT_CHECKED` otherwise.
- A CNAME chain ending in a nonexistent name returns RCODE `NXDOMAIN` with the CNAME RR in the answer section (RFC 2308 section 2.1) and yields `MX_RECORD_IS_CNAME`.
- An exchange with AAAA records and no A record returns `NOERROR` with an empty answer section and yields `MX_RECORD_IS_NOT_CNAME`.
- The exchange `.` yields no Zone08 tag and no query.
- A missing A response, or an RCODE other than `NOERROR` and `NXDOMAIN` without a CNAME RR, yields `MX_RECORD_NOT_CHECKED`.
