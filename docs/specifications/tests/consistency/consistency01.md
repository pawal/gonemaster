# Consistency01

Status: Final

## Purpose
- Check SOA serial consistency across nameservers for the tested zone.
- Report serial distribution and detect serial variation above the configured threshold.

## Preconditions And Inputs
- Preconditions:
  - A `zone.Zone` object is available.
- Required inputs:
  - Nameserver list from `methods.Method4` and `methods.Method5`.
  - SOA answers from queried nameservers.
- Profile/config knobs that affect behavior:
  - `net.ipv4` and `net.ipv6`: disabled transports are skipped per nameserver.
  - `resolver.defaults.parallel`: per-nameserver query task parallelism.
  - `constants.SerialMaxVariation`: accepted numeric delta threshold (default `0`).

## Algorithm And Decision Flow
1. Emit `TEST_CASE_START`.
2. Build deduplicated nameserver list from Method4+Method5 by `ns.String()` (`name/ip`).
3. For each nameserver (parallelized):
   - If transport is disabled, emit `IPV4_DISABLED` or `IPV6_DISABLED` for rrtype `SOA` and skip.
   - Query SOA for zone apex.
   - No response message -> emit `NO_RESPONSE`.
   - Response without usable SOA record for zone apex -> emit `NO_RESPONSE_SOA_QUERY`.
   - Otherwise store serial value for that nameserver.
4. Group successful responses by serial value.
5. Emit `SOA_SERIAL` once per serial with sorted `servers`.
6. If exactly one serial exists, emit `ONE_SOA_SERIAL`.
7. If multiple serials exist:
   - Emit `MULTIPLE_SOA_SERIALS`.
   - Compute numeric delta using first/last sorted serial keys and, when delta exceeds `SerialMaxVariation`, emit `SOA_SERIAL_VARIATION`.
8. Emit `TEST_CASE_END`.

### Per-NS SOA Probe and Serial Aggregation (steps 2-8)

{{% expand "Show diagram" %}}
```
gather Method4 then Method5 nameservers
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

After all tasks (serialKeys sorted lexicographically):
  emit one SOA_SERIAL per serial key  (serial, servers)

  len(serialKeys) == 1                  -> ONE_SOA_SERIAL (serial)
  len(serialKeys) > 1                   -> MULTIPLE_SOA_SERIALS (count)
      if int(serialKeys[last]) - int(serialKeys[0]) > SerialMaxVariation
                                        -> SOA_SERIAL_VARIATION
                                           (serial_min, serial_max, max_variation)

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
| `SOA_SERIAL_VARIATION` | `serial_min` | `string` | Lowest serial key used in variation check. |
| `SOA_SERIAL_VARIATION` | `serial_max` | `string` | Highest serial key used in variation check. |
| `SOA_SERIAL_VARIATION` | `max_variation` | `int` | Allowed maximum variation threshold. |
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
- Upstream reference: [`consistency01.md`](../../upstream/tests/Consistency-TP/consistency01.md)
- Differences (Upstream vs Gonemaster):
  - Upstream documents `MULTIPLE_SOA_SERIALS_OK`; Gonemaster does not emit that tag.
  - Upstream: does not explicitly define this detail. Gonemaster: Serial variation delta uses integer subtraction on the first and last serial values after lexicographic string-key sorting.
- Potential upstream report:
  - `no`

## Implementation Notes

The following behaviors are implementation choices, not mandated by protocol:

- **String-key sort for variation delta**: When multiple distinct serials are observed, the variation delta is computed by sorting the serial values as strings (lexicographically) and subtracting the first from the last.  Lexicographic ordering differs from numeric ordering when serials have different digit counts (e.g., `"9"` sorts after `"10"` lexicographically, so the min/max assignment and resulting delta can differ from a numerically sorted result).  This only affects `SOA_SERIAL_VARIATION` emission when `SerialMaxVariation > 0`; with the default of `0` any difference between serials is flagged regardless of this sort order.
- **Deduplication by `name/ip`**: Nameservers are deduplicated using their full `name/ip` identity string.  Two entries with the same IP but different names are treated as distinct sources.  The protocol defines no deduplication rule for testcase purposes; this choice is implementation-defined.
- **Sorted `servers` in `SOA_SERIAL`**: Nameserver identities in `SOA_SERIAL` arguments are sorted before joining.  Deterministic ordering is an implementation choice for reproducible output.

## Edge Cases And Limitations
- If no usable SOA serial is obtained from any nameserver, no serial-summary tag (`ONE_SOA_SERIAL`/`MULTIPLE_SOA_SERIALS`) is emitted.
- Nameserver deduplication is by `name/ip`; same IP with different names is treated as separate sources.
- Variation check behavior is sensitive to string-key ordering of serial values.
