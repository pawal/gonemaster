# Zone12 (zone12)

Status: Final

## Purpose
- Check existence and RFC 7477 compliance of the CSYNC RR at the zone apex.

## Preconditions And Inputs
- Preconditions:
  - A `zone.Zone` object is available.
- Required inputs:
  - Nameserver addresses from `methods.Method4and5`.
  - CSYNC and SOA responses from authoritative nameservers at the zone apex.
- Profile/config knobs that affect behavior:
  - `net.ipv4` and `net.ipv6`: disabled transports are skipped with transport debug tags.
  - `resolver.defaults.parallel`: parallel nameserver query fanout.

## Algorithm And Decision Flow
1. Emit `TEST_CASE_START`.
2. Read nameserver list from `Method4and5`.
3. For each nameserver (parallelized, input-order merged logs):
   - If transport is disabled, emit `IPV4_DISABLED` or `IPV6_DISABLED` for rrtype `CSYNC`, then skip.
   - Send CSYNC query to the zone apex with default query options.
   - If no response, or response is not authoritative NOERROR, skip this nameserver silently.
   - Else record the CSYNC RRset from the answer section.
   - Send SOA query to the same nameserver and record the SOA serial for comparison.
4. Post-processing (sequential, over all collected outcomes):
   - For each nameserver that returned an authoritative NOERROR response:
     - If the CSYNC RRset has more than one record, emit `Z12_MULTIPLE_CSYNC` (`ns`, `count`).
     - Else if exactly one CSYNC record is present:
       - Emit `Z12_CSYNC_FOUND` (`ns`, `serial`, `flags`, `type_bitmap`).
       - If the SOA serial was retrieved, evaluate CSYNC serial against SOA serial:
         - When `soaminimum` flag is set (bit 1), emit `Z12_SERIAL_MISMATCH` only if `csync soaserial` is greater than current SOA serial.
         - When `soaminimum` flag is not set, emit `Z12_SERIAL_MISMATCH` if `csync soaserial` differs from current SOA serial.
     - Else (zero CSYNC records) emit `Z12_NO_CSYNC` (`ns`).
   - If at least one nameserver has CSYNC and at least one has no CSYNC, emit `Z12_MIXED_PRESENCE`.
   - If more than one nameserver has CSYNC and the CSYNC content differs across them, emit `Z12_INCONSISTENT_CSYNC`.
5. Emit `TEST_CASE_END`.

CSYNC content identity is determined by comparing the concatenation of `soaserial`, `flags`, and `TypeBitMap` fields.

## Emitted Tags (Possible Set)
| Tag | Emitted when |
| --- | --- |
| `IPV4_DISABLED` | IPv4 nameserver evaluation is skipped because IPv4 is disabled. |
| `IPV6_DISABLED` | IPv6 nameserver evaluation is skipped because IPv6 is disabled. |
| `Z12_CSYNC_FOUND` | CSYNC record found at zone apex on this nameserver. |
| `Z12_INCONSISTENT_CSYNC` | CSYNC content differs across authoritative nameservers. |
| `Z12_MIXED_PRESENCE` | CSYNC present on some nameservers but absent on others. |
| `Z12_MULTIPLE_CSYNC` | More than one CSYNC RR found at zone apex on this nameserver. |
| `Z12_NO_CSYNC` | No CSYNC record found at zone apex on this nameserver. |
| `Z12_SERIAL_MISMATCH` | CSYNC soaserial fails RFC 7477 serial precondition against current SOA serial from the same nameserver. |
| `TEST_CASE_END` | Testcase completion marker is emitted. |
| `TEST_CASE_START` | Testcase start marker is emitted. |

## Tag Arguments
| Tag | Argument key | Type | Meaning |
| --- | --- | --- | --- |
| `IPV4_DISABLED` | `ns` | `string` | Nameserver identity (`ns` name only; use `address` for IP) skipped on IPv4. |
| `IPV4_DISABLED` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `IPV4_DISABLED` | `arg_schema` | `string` | Schema identifier (`gonemaster.logargs/1.1`) for coherent log arguments. |
| `IPV4_DISABLED` | `rrtype` | `string` | rrtype skipped (`CSYNC`). |
| `IPV6_DISABLED` | `ns` | `string` | Nameserver identity (`ns` name only; use `address` for IP) skipped on IPv6. |
| `IPV6_DISABLED` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `IPV6_DISABLED` | `arg_schema` | `string` | Schema identifier (`gonemaster.logargs/1.1`) for coherent log arguments. |
| `IPV6_DISABLED` | `rrtype` | `string` | rrtype skipped (`CSYNC`). |
| `Z12_CSYNC_FOUND` | `ns` | `string` | Nameserver identity (`ns` name only; use `address` for IP). |
| `Z12_CSYNC_FOUND` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `Z12_CSYNC_FOUND` | `arg_schema` | `string` | Schema identifier (`gonemaster.logargs/1.1`) for coherent log arguments. |
| `Z12_CSYNC_FOUND` | `serial` | `uint32` | SOA serial from the CSYNC `soaserial` field. |
| `Z12_CSYNC_FOUND` | `flags` | `uint16` | CSYNC flags field (bit 0 = `immediate`, bit 1 = `soaminimum`). |
| `Z12_CSYNC_FOUND` | `type_bitmap` | `string` | Semicolon-separated DNS type names from the CSYNC TypeBitMap (e.g. `NS;A;AAAA`). |
| `Z12_MULTIPLE_CSYNC` | `ns` | `string` | Nameserver identity (`ns` name only; use `address` for IP). |
| `Z12_MULTIPLE_CSYNC` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `Z12_MULTIPLE_CSYNC` | `arg_schema` | `string` | Schema identifier (`gonemaster.logargs/1.1`) for coherent log arguments. |
| `Z12_MULTIPLE_CSYNC` | `count` | `int` | Number of CSYNC records returned. |
| `Z12_NO_CSYNC` | `ns` | `string` | Nameserver identity (`ns` name only; use `address` for IP). |
| `Z12_NO_CSYNC` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `Z12_NO_CSYNC` | `arg_schema` | `string` | Schema identifier (`gonemaster.logargs/1.1`) for coherent log arguments. |
| `Z12_SERIAL_MISMATCH` | `ns` | `string` | Nameserver identity (`ns` name only; use `address` for IP). |
| `Z12_SERIAL_MISMATCH` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `Z12_SERIAL_MISMATCH` | `arg_schema` | `string` | Schema identifier (`gonemaster.logargs/1.1`) for coherent log arguments. |
| `Z12_SERIAL_MISMATCH` | `csync_serial` | `uint32` | The serial carried in the CSYNC `soaserial` field. |
| `Z12_SERIAL_MISMATCH` | `soa_serial` | `uint32` | The current SOA serial from the same nameserver. |
| `TEST_CASE_END` | `testcase` | `string` | Testcase display name (`Zone12`). |
| `TEST_CASE_START` | `testcase` | `string` | Testcase display name (`Zone12`). |

## Severity Levels Per Tag
| Tag | Level | Notes |
| --- | --- | --- |
| `IPV4_DISABLED` | `DEBUG` | Default from `share/profile.json` (`test_levels.ZONE`). |
| `IPV6_DISABLED` | `DEBUG` | Default from `share/profile.json` (`test_levels.ZONE`). |
| `Z12_CSYNC_FOUND` | `INFO` | Default from `share/profile.json` (`test_levels.ZONE`). |
| `Z12_INCONSISTENT_CSYNC` | `WARNING` | Default from `share/profile.json` (`test_levels.ZONE`). |
| `Z12_MIXED_PRESENCE` | `WARNING` | Default from `share/profile.json` (`test_levels.ZONE`). |
| `Z12_MULTIPLE_CSYNC` | `WARNING` | Default from `share/profile.json` (`test_levels.ZONE`). |
| `Z12_NO_CSYNC` | `INFO` | Default from `share/profile.json` (`test_levels.ZONE`). CSYNC is optional per RFC 7477. |
| `Z12_SERIAL_MISMATCH` | `WARNING` | Default from `share/profile.json` (`test_levels.ZONE`). |
| `TEST_CASE_END` | `DEBUG` | Default from `share/profile.json` (`test_levels.ZONE`). |
| `TEST_CASE_START` | `DEBUG` | Default from `share/profile.json` (`test_levels.ZONE`). |

## Differences From Upstream
- No upstream (Zonemaster) equivalent exists; this is a gonemaster-specific test case.
- References: [RFC 7477](https://datatracker.ietf.org/doc/html/rfc7477)

## Edge Cases And Limitations
- CSYNC is an optional zone apex record per RFC 7477; `Z12_NO_CSYNC` is informational only and does not indicate a problem.
- Only authoritative NOERROR responses are evaluated. Nameservers returning non-NOERROR or non-AA responses are skipped silently.
- SOA serial comparison (`Z12_SERIAL_MISMATCH`) is only performed when the SOA query to the same nameserver succeeds and returns a SOA record. If the SOA query fails, no mismatch is reported for that nameserver.
- With CSYNC `soaminimum` flag set, an older CSYNC serial than current SOA serial is accepted and does not emit `Z12_SERIAL_MISMATCH`.
- For nameservers returning multiple CSYNC records (`Z12_MULTIPLE_CSYNC`), the records are not included in the cross-nameserver consistency comparison.
