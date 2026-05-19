# Nameserver03

Status: Final

## Purpose
- Check whether nameservers allow AXFR zone transfer.

## Preconditions And Inputs
- Preconditions:
  - A `zone.Zone` object is available.
- Required inputs:
  - Nameserver addresses from `AllNameservers`.
  - AXFR behavior per nameserver.
- Profile/config knobs that affect behavior:
  - `net.ipv4` and `net.ipv6`: disabled transports are skipped with transport debug tags.
  - `resolver.defaults.parallel`: parallel nameserver query fanout.

## Algorithm And Decision Flow
1. Emit `TEST_CASE_START`.
2. Read nameserver list from `AllNameservers`, deduplicate by `name/ip`, preserving first-seen order.
3. For each deduplicated nameserver (parallelized, input-order merged logs):
   - If transport is disabled, emit `IPV4_DISABLED` or `IPV6_DISABLED` for rrtype `AXFR`, then skip.
   - Attempt AXFR for zone name.
   - Capture first RR returned by AXFR callback and stop callback immediately.
   - If AXFR call returns an error, record server as AXFR failure.
   - Else if first RR is an `SOA`, record server as AXFR available.
   - Else (AXFR succeeded but first RR is not `SOA`): no record for this nameserver.
4. After all parallel tasks, emit a single consolidated `AXFR_FAILURE` with `servers` list (if any), and a single consolidated `AXFR_AVAILABLE` with `servers` list (if any).
5. Emit `TEST_CASE_END`.

### Per-NS AXFR Attempt and Aggregation (steps 2-5)

{{% expand "Show diagram" %}}
```
ns list = AllNameservers; dedupe by ns.String() ("name/ip"), preserve first-seen order

For each nameserver (parallel; fan-out = resolver.defaults.parallel):

   transport disabled for AXFR -> IPV4_DISABLED / IPV6_DISABLED, skip
   attempt AXFR for z.Name (callback captures first RR then stops)
    +- AXFR call returns error                -> axfrFailure[ns]
    +- first RR is *dns.SOA                   -> axfrAvailable[ns]
    +- first RR not SOA                       -> (no finding)

After all tasks:
  axfrFailure   non-empty -> AXFR_FAILURE   (servers; sorted)
  axfrAvailable non-empty -> AXFR_AVAILABLE (servers; sorted)

emit TEST_CASE_END
```
{{% /expand %}}

## Emitted Tags (Possible Set)
| Tag | Emitted when |
| --- | --- |
| `AXFR_AVAILABLE` | AXFR succeeded and the first transfer RR was `SOA`. |
| `AXFR_FAILURE` | AXFR call returned an error. |
| `IPV4_DISABLED` | IPv4 nameserver evaluation is skipped because IPv4 is disabled. |
| `IPV6_DISABLED` | IPv6 nameserver evaluation is skipped because IPv6 is disabled. |
| `TEST_CASE_END` | Testcase completion marker is emitted. |
| `TEST_CASE_START` | Testcase start marker is emitted. |

## Tag Arguments
| Tag | Argument key | Type | Meaning |
| --- | --- | --- | --- |
| `AXFR_AVAILABLE` | `servers` | `array<object>` | Structured sorted list of nameservers allowing AXFR (`{ns}`, `{address}` items). |
| `AXFR_FAILURE` | `servers` | `array<object>` | Structured sorted list of nameservers where AXFR failed (`{ns}`, `{address}` items). |
| `IPV4_DISABLED` | `ns` | `string` | Nameserver identity (`ns` name only; use `address` for IP) skipped on IPv4. |
| `IPV4_DISABLED` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `IPV4_DISABLED` | `rrtype` | `string` | rrtype skipped (`AXFR`). |
| `IPV6_DISABLED` | `ns` | `string` | Nameserver identity (`ns` name only; use `address` for IP) skipped on IPv6. |
| `IPV6_DISABLED` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `IPV6_DISABLED` | `rrtype` | `string` | rrtype skipped (`AXFR`). |
| `TEST_CASE_END` | `testcase` | `string` | Testcase display name (`Nameserver03`). |
| `TEST_CASE_START` | `testcase` | `string` | Testcase display name (`Nameserver03`). |

## Severity Levels Per Tag
| Tag | Level | Notes |
| --- | --- | --- |
| `AXFR_AVAILABLE` | `NOTICE` | Default from `share/profile.json` (`test_levels.NAMESERVER`). |
| `AXFR_FAILURE` | `INFO` | Default from `share/profile.json` (`test_levels.NAMESERVER`). |
| `IPV4_DISABLED` | `DEBUG` | Default from `share/profile.json` (`test_levels.NAMESERVER`). |
| `IPV6_DISABLED` | `DEBUG` | Default from `share/profile.json` (`test_levels.NAMESERVER`). |
| `TEST_CASE_END` | `DEBUG` | Default from `share/profile.json` (`test_levels.NAMESERVER`). |
| `TEST_CASE_START` | `DEBUG` | Default from `share/profile.json` (`test_levels.NAMESERVER`). |

## Differences From Upstream
- Upstream reference: [`nameserver03.md`](../../upstream/tests/Nameserver-TP/nameserver03.md)
- Differences (Upstream vs Gonemaster):
  - Upstream: describes failure semantics when AXFR starts with SOA. Gonemaster: emits explicit tags for both transfer availability (`AXFR_AVAILABLE`) and transfer errors (`AXFR_FAILURE`).
  - Upstream: does not explicitly describe testcase boundary and transport-disabled debug emissions. Gonemaster: emits `TEST_CASE_START`, `TEST_CASE_END`, `IPV4_DISABLED`, and `IPV6_DISABLED`.
- Potential upstream report:
  - `no`

## Implementation Notes

The following behaviors are implementation choices, not mandated by RFC 5936 (DNS Zone Transfer Protocol):

- **First-RR-only inspection**: The testcase captures only the first RR from the AXFR response stream and immediately terminates the callback.  RFC 5936 specifies that a valid AXFR transfer begins and ends with the zone SOA record.  Inspecting only the first RR is a deliberate shortcut: if the server sends any RR before the leading SOA it is treated as an unusual response rather than an error.  A full conformance check would also verify the trailing SOA.
- **Deduplication by `name/ip`**: Nameservers are deduplicated by their `name/ip` identity string, preserving first-seen order.  The protocol does not define deduplication rules for testcase purposes.

## Edge Cases And Limitations
- Successful AXFR responses where first RR is not `SOA` emit no availability/failure tag.
- Nameservers skipped due disabled transport do not contribute AXFR findings.
- Only the first AXFR RR is inspected in testcase logic.
