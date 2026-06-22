# Consistency01

Status: Final

## Purpose
- Check SOA serial consistency across nameservers for the tested zone.
- Report serial distribution and detect serial variation above the configured threshold.

## Preconditions And Inputs
- Preconditions:
  - A `zone.Zone` object is available.
- Required inputs:
  - Nameserver list from [`GlueNameservers`](../../nameserver-resolution.md#gluenameservers) and [`ApexNameservers`](../../nameserver-resolution.md#apexnameservers).
  - SOA answers from queried nameservers.
- Profile/config knobs that affect behavior:
  - `net.ipv4` and `net.ipv6`: disabled transports are skipped per nameserver.
  - `resolver.defaults.parallel`: per-nameserver query task parallelism.
  - `constants.SerialMaxVariation`: accepted numeric delta threshold (default `0`).

## Algorithm And Decision Flow
1. Emit `TEST_CASE_START`.
2. Build deduplicated nameserver list from the union of [`GlueNameservers`](../../nameserver-resolution.md#gluenameservers) and [`ApexNameservers`](../../nameserver-resolution.md#apexnameservers) by `ns.String()` (`name/ip`).
3. For each nameserver (parallelized):
   - If transport is disabled, emit `IPV4_DISABLED` or `IPV6_DISABLED` for rrtype `SOA` and skip.
   - Query SOA for zone apex.
   - No response message -> emit `NO_RESPONSE`.
   - Response without usable SOA record for zone apex -> emit `NO_RESPONSE_SOA_QUERY`.
   - Otherwise store serial value for that nameserver.
4. Group successful responses by serial value.
5. Emit `SOA_SERIAL` once per serial (ordered numerically) with sorted `servers`.
6. If exactly one serial exists, emit `ONE_SOA_SERIAL`.
7. If multiple serials exist:
   - Emit `MULTIPLE_SOA_SERIALS`.
   - Select the oldest and newest serial using RFC 1982 serial arithmetic, compute the wrap-safe forward distance between them, and when it exceeds `SerialMaxVariation` emit `SOA_SERIAL_VARIATION` with the lagging nameservers in `servers_behind`.
8. Emit `TEST_CASE_END`.

### Per-NS SOA Probe and Serial Aggregation (steps 2-8)

{{% expand "Show diagram" %}}
```
gather GlueNameservers then ApexNameservers nameservers
 +- dedupe by ns.String() ("name/ip")

For each nameserver (parallel; fan-out = resolver.defaults.parallel):

   transport check for SOA
    +- disabled                          -> IPV4_DISABLED / IPV6_DISABLED, mark skip
    +- enabled                           -> query SOA at z.Name

   resp.Msg absent                       -> NO_RESPONSE
   no SOA record for z.Name in resp      -> NO_RESPONSE_SOA_QUERY
   first record not *dns.SOA             -> NO_RESPONSE_SOA_QUERY
   otherwise                             -> serial = soa.Serial (as string)
                                            serials[serial] += "name/ip"

After all tasks (serials ordered numerically):
  emit one SOA_SERIAL per serial  (serial, servers)

  len(serials) == 1                  -> ONE_SOA_SERIAL (serial)
  len(serials) > 1                   -> MULTIPLE_SOA_SERIALS (count)
      oldest/newest via RFC 1982 (util.SerialGT); variation = newest - oldest (wrap-safe)
      if variation > SerialMaxVariation
                                        -> SOA_SERIAL_VARIATION
                                           (serial_min, serial_max, max_variation, servers_behind)

emit TEST_CASE_END
```
{{% /expand %}}

## Emitted Tags (Possible Set)
| Tag | Emitted when |
| --- | --- |
| `IPV4_DISABLED` | IPv4 transport is disabled for a queried nameserver/rrtype. |
| `IPV6_DISABLED` | IPv6 transport is disabled for a queried nameserver/rrtype. |
| `MULTIPLE_SOA_SERIALS` | At least two distinct SOA serial values were observed. |
| `NO_RESPONSE` | SOA query had no response message from a nameserver. |
| `NO_RESPONSE_SOA_QUERY` | Response did not contain a usable SOA record for zone apex. |
| `ONE_SOA_SERIAL` | Exactly one SOA serial value was observed. |
| `SOA_SERIAL` | A specific SOA serial value and associated nameservers are reported. |
| `SOA_SERIAL_VARIATION` | Serial delta exceeded `constants.SerialMaxVariation`. |
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
| `MULTIPLE_SOA_SERIALS` | `count` | `int` | Number of distinct serial values observed. |
| `NO_RESPONSE` | `ns` | `string` | Nameserver identity (`ns` name only; use `address` for IP) with no response. |
| `NO_RESPONSE` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `NO_RESPONSE_SOA_QUERY` | `ns` | `string` | Nameserver identity (`ns` name only; use `address` for IP) without usable SOA answer. |
| `NO_RESPONSE_SOA_QUERY` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `ONE_SOA_SERIAL` | `serial` | `string` | The single observed SOA serial value. |
| `SOA_SERIAL` | `serial` | `string` | One observed SOA serial value. |
| `SOA_SERIAL` | `servers` | `array<object>` | Structured nameserver identities (`{ns,address}` object) serving that serial. |
| `SOA_SERIAL_VARIATION` | `serial_min` | `string` | Oldest serial (RFC 1982) used in variation check. |
| `SOA_SERIAL_VARIATION` | `serial_max` | `string` | Newest serial (RFC 1982) used in variation check. |
| `SOA_SERIAL_VARIATION` | `max_variation` | `int` | Allowed maximum variation threshold. |
| `SOA_SERIAL_VARIATION` | `servers_behind` | `array<object>` | Nameservers serving an older serial than the newest observed. |
| `TEST_CASE_END` | `testcase` | `string` | Testcase display name (`Consistency01`). |
| `TEST_CASE_START` | `testcase` | `string` | Testcase display name (`Consistency01`). |

## Severity Levels Per Tag
| Tag | Level | Notes |
| --- | --- | --- |
| `IPV4_DISABLED` | `DEBUG` | Default from `share/profile.json` (`test_levels.CONSISTENCY`). |
| `IPV6_DISABLED` | `DEBUG` | Default from `share/profile.json` (`test_levels.CONSISTENCY`). |
| `MULTIPLE_SOA_SERIALS` | `WARNING` | Default from `share/profile.json` (`test_levels.CONSISTENCY`). |
| `NO_RESPONSE` | `DEBUG` | Default from `share/profile.json` (`test_levels.CONSISTENCY`). |
| `NO_RESPONSE_SOA_QUERY` | `DEBUG` | Default from `share/profile.json` (`test_levels.CONSISTENCY`). |
| `ONE_SOA_SERIAL` | `INFO` | Default from `share/profile.json` (`test_levels.CONSISTENCY`). |
| `SOA_SERIAL` | `INFO` | Default from `share/profile.json` (`test_levels.CONSISTENCY`). |
| `SOA_SERIAL_VARIATION` | `NOTICE` | Default from `share/profile.json` (`test_levels.CONSISTENCY`). |
| `TEST_CASE_END` | `DEBUG` | Default from `share/profile.json` (`test_levels.CONSISTENCY`). |
| `TEST_CASE_START` | `DEBUG` | Default from `share/profile.json` (`test_levels.CONSISTENCY`). |

## Differences From Upstream
- Differences (Upstream vs Gonemaster):
  - Upstream documents `MULTIPLE_SOA_SERIALS_OK`; Gonemaster does not emit that tag.
  - Upstream sorts serial values lexically as strings and subtracts the first from the last, which mis-ranks differing-length serials and the 32-bit wrap boundary. Gonemaster orders serials and selects the oldest/newest using RFC 1982 serial arithmetic, and reports the lagging nameservers in `servers_behind`.
- Potential upstream report:
  - `yes` (reported upstream: the upstream string-sort plus integer-subtraction variation check is incorrect for differing-length serials and for serials near the 32-bit wrap boundary).

## Implementation Notes

The following behaviors are implementation choices, not mandated by protocol:

- **RFC 1982 serial ordering**: Observed serials are parsed to 32-bit integers and ordered numerically; the oldest and newest are selected with RFC 1982 serial-number arithmetic (`util.SerialGT`), and the variation is the wrap-safe forward distance between them. With the default `SerialMaxVariation` of `0` any difference between serials is flagged. Serials that fail to parse as 32-bit integers are skipped from the variation check.
- **Deduplication by `name/ip`**: Nameservers are deduplicated using their full `name/ip` identity string.  Two entries with the same IP but different names are treated as distinct sources.  The protocol defines no deduplication rule for testcase purposes; this choice is implementation-defined.
- **Sorted `servers` in `SOA_SERIAL`**: Nameserver identities in `SOA_SERIAL` arguments are sorted before joining.  Deterministic ordering is an implementation choice for reproducible output.

## Edge Cases And Limitations
- If no usable SOA serial is obtained from any nameserver, no serial-summary tag (`ONE_SOA_SERIAL`/`MULTIPLE_SOA_SERIALS`) is emitted.
- Nameserver deduplication is by `name/ip`; same IP with different names is treated as separate sources.
- Serials that do not parse as 32-bit unsigned integers are skipped from the variation check.
