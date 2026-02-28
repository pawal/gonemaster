# Nameserver03 (nameserver03)

Status: Final

## Purpose
- Check whether nameservers allow AXFR zone transfer.

## Preconditions And Inputs
- Preconditions:
  - A `zone.Zone` object is available.
- Required inputs:
  - Nameserver addresses from `methods.Method4and5`.
  - AXFR behavior per nameserver.
- Profile/config knobs that affect behavior:
  - `net.ipv4` and `net.ipv6`: disabled transports are skipped with transport debug tags.
  - `resolver.defaults.parallel`: parallel nameserver query fanout.

## Algorithm And Decision Flow
1. Emit `TEST_CASE_START`.
2. Read nameserver list from `Method4and5`, deduplicate by `name/ip`, preserving first-seen order.
3. For each deduplicated nameserver (parallelized, input-order merged logs):
   - If transport is disabled, emit `IPV4_DISABLED` or `IPV6_DISABLED` for rrtype `AXFR`, then skip.
   - Attempt AXFR for zone name.
   - Capture first RR returned by AXFR callback and stop callback immediately.
   - If AXFR call returns an error, emit `AXFR_FAILURE`.
   - Else if first RR is an `SOA`, emit `AXFR_AVAILABLE`.
   - Else (AXFR succeeded but first RR is not `SOA`): emit no tag for this nameserver.
4. Emit `TEST_CASE_END`.

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
| `AXFR_AVAILABLE` | `ns` | `string` | Nameserver identity (`name/ip`) allowing AXFR. |
| `AXFR_FAILURE` | `ns` | `string` | Nameserver identity (`name/ip`) where AXFR failed. |
| `IPV4_DISABLED` | `ns` | `string` | Nameserver identity (`name/ip`) skipped on IPv4. |
| `IPV4_DISABLED` | `rrtype` | `string` | rrtype skipped (`AXFR`). |
| `IPV6_DISABLED` | `ns` | `string` | Nameserver identity (`name/ip`) skipped on IPv6. |
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
