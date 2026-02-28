# Zone01 (zone01)

Status: Final

## Purpose
- Validate SOA MNAME handling for the child zone: name sanity, resolvability, authority behavior, and serial-based master inference.

## Preconditions And Inputs
- Preconditions:
  - A `zone.Zone` object is available.
  - A recursor is available on the zone object.
- Required inputs:
  - Nameserver addresses from `methods.Method4and5`.
  - Child NS names from `methods.Method3`.
  - Recursive A/AAAA lookups for SOA MNAME hostnames.
  - Direct SOA queries to SOA MNAME host/IP combinations.
- Profile/config knobs that affect behavior:
  - `net.ipv4` and `net.ipv6` control whether transport families are queried.

## Algorithm And Decision Flow
1. Emit `TEST_CASE_START`.
2. Query each nameserver from `Method4and5` for `SOA` at the child zone apex.
3. For each response, continue only when all are true:
   - response exists;
   - `RCODE=NOERROR`;
   - `AA=true`;
   - at least one SOA RR for the zone name exists.
4. From accepted SOA answers, collect:
   - SOA MNAME values;
   - SOA serial values from child nameservers;
   - source nameserver IPs where MNAME is `localhost` or `.`.
5. Emit:
   - `Z01_MNAME_IS_LOCALHOST` when any accepted response had MNAME `localhost`;
   - `Z01_MNAME_IS_DOT` when any accepted response had MNAME `.`.
6. For each non-`localhost` and non-dot MNAME:
   - If MNAME is not in `Method3` child NS name set, emit `Z01_MNAME_NOT_IN_NS_LIST`.
   - Resolve MNAME addresses using recursor lookup (`A`/`AAAA`).
   - For each resolved address:
     - If localhost address (`127.0.0.1` or `::1`), emit `Z01_MNAME_HAS_LOCALHOST_ADDR`.
     - Else query `SOA` directly against that `mname/ip`.
       - If no response, emit `Z01_MNAME_NO_RESPONSE`.
       - If `RCODE != NOERROR`, emit `Z01_MNAME_UNEXPECTED_RCODE`.
       - If no SOA in answer, emit `Z01_MNAME_MISSING_SOA_RECORD`.
       - If SOA present but `AA=false`, emit `Z01_MNAME_NOT_AUTHORITATIVE`.
       - If SOA present and `AA=true`, store returned serial for master comparison.
   - If no MNAME address was resolved, emit `Z01_MNAME_NOT_RESOLVE` (subject to the implementation caveat documented below).
7. If at least one authoritative MNAME serial was collected, compare each collected MNAME serial against serials gathered from child nameservers using RFC1982 ordering:
   - If any child nameserver serial is greater than the candidate MNAME serial, classify as not master.
   - Otherwise classify as master.
8. Emit:
   - `Z01_MNAME_NOT_MASTER` for non-master MNAME host/IPs, with serial context;
   - `Z01_MNAME_IS_MASTER` for master-candidate MNAME host/IPs.
9. Emit `TEST_CASE_END`.

## Emitted Tags (Possible Set)
| Tag | Emitted when |
| --- | --- |
| `TEST_CASE_END` | Testcase completion marker is emitted. |
| `TEST_CASE_START` | Testcase start marker is emitted. |
| `Z01_MNAME_HAS_LOCALHOST_ADDR` | SOA MNAME resolves to `127.0.0.1` or `::1`. |
| `Z01_MNAME_IS_DOT` | One or more accepted child SOA responses report MNAME as `.`. |
| `Z01_MNAME_IS_LOCALHOST` | One or more accepted child SOA responses report MNAME as `localhost`. |
| `Z01_MNAME_IS_MASTER` | One or more MNAME host/IP pairs are inferred to be master by serial comparison. |
| `Z01_MNAME_MISSING_SOA_RECORD` | A direct SOA query to an MNAME host/IP responds without SOA in answer. |
| `Z01_MNAME_NOT_AUTHORITATIVE` | A direct SOA query to an MNAME host/IP returns SOA but is not authoritative (`AA=false`). |
| `Z01_MNAME_NOT_IN_NS_LIST` | MNAME hostname is not listed among child NS names (`Method3`). |
| `Z01_MNAME_NOT_MASTER` | One or more MNAME host/IP pairs have a serial lower than at least one child nameserver serial. |
| `Z01_MNAME_NOT_RESOLVE` | MNAME hostname cannot be resolved to any address. |
| `Z01_MNAME_NO_RESPONSE` | A direct SOA query to an MNAME host/IP receives no response. |
| `Z01_MNAME_UNEXPECTED_RCODE` | A direct SOA query to an MNAME host/IP returns non-`NOERROR` rcode. |

## Tag Arguments
| Tag | Argument key | Type | Meaning |
| --- | --- | --- | --- |
| `TEST_CASE_END` | `testcase` | `string` | Testcase display name (`Zone01`). |
| `TEST_CASE_START` | `testcase` | `string` | Testcase display name (`Zone01`). |
| `Z01_MNAME_HAS_LOCALHOST_ADDR` | `nsname` | `string` | SOA MNAME hostname. |
| `Z01_MNAME_HAS_LOCALHOST_ADDR` | `ns_ip` | `string` | Localhost IP address for that MNAME (`127.0.0.1` or `::1`). |
| `Z01_MNAME_IS_DOT` | `ns_ip_list` | `string` | Semicolon-delimited source child nameserver IPs returning MNAME as dot. |
| `Z01_MNAME_IS_LOCALHOST` | `ns_ip_list` | `string` | Semicolon-delimited source child nameserver IPs returning MNAME as localhost. |
| `Z01_MNAME_IS_MASTER` | `ns_list` | `string` | Semicolon-delimited inferred-master `mname/ip` list. |
| `Z01_MNAME_MISSING_SOA_RECORD` | `ns` | `string` | Queried MNAME endpoint (`name/ip`) that returned no SOA in answer. |
| `Z01_MNAME_NOT_AUTHORITATIVE` | `ns` | `string` | Queried MNAME endpoint (`name/ip`) that returned non-authoritative answer. |
| `Z01_MNAME_NOT_IN_NS_LIST` | `nsname` | `string` | MNAME hostname absent from child NS set. |
| `Z01_MNAME_NOT_MASTER` | `ns_list` | `string` | Semicolon-delimited non-master `mname/ip` list. |
| `Z01_MNAME_NOT_MASTER` | `soaserial` | `uint32` | Highest serial among non-master candidates in the emitted set. |
| `Z01_MNAME_NOT_MASTER` | `soaserial_list` | `string` | Semicolon-delimited unique child nameserver serial values used for comparison. |
| `Z01_MNAME_NOT_RESOLVE` | `nsname` | `string` | MNAME hostname that did not resolve. |
| `Z01_MNAME_NO_RESPONSE` | `ns` | `string` | Queried MNAME endpoint (`name/ip`) with no response. |
| `Z01_MNAME_UNEXPECTED_RCODE` | `ns` | `string` | Queried MNAME endpoint (`name/ip`). |
| `Z01_MNAME_UNEXPECTED_RCODE` | `rcode` | `string` | Non-`NOERROR` response code text. |

## Severity Levels Per Tag

SOA MNAME is never used for authoritative nameserver discovery and is not part of normal DNS lookup, so operational impact of MNAME errors is limited. Accordingly, no MNAME-related tag exceeds `NOTICE` severity.

| Tag | Level | Notes |
| --- | --- | --- |
| `TEST_CASE_END` | `DEBUG` | Default from `share/profile.json` (`test_levels.ZONE`). |
| `TEST_CASE_START` | `DEBUG` | Default from `share/profile.json` (`test_levels.ZONE`). |
| `Z01_MNAME_HAS_LOCALHOST_ADDR` | `NOTICE` | Default from `share/profile.json` (`test_levels.ZONE`). |
| `Z01_MNAME_IS_DOT` | `NOTICE` | Default from `share/profile.json` (`test_levels.ZONE`). |
| `Z01_MNAME_IS_LOCALHOST` | `NOTICE` | Default from `share/profile.json` (`test_levels.ZONE`). |
| `Z01_MNAME_IS_MASTER` | `DEBUG` | Default from `share/profile.json` (`test_levels.ZONE`). |
| `Z01_MNAME_MISSING_SOA_RECORD` | `NOTICE` | Default from `share/profile.json` (`test_levels.ZONE`). |
| `Z01_MNAME_NOT_AUTHORITATIVE` | `NOTICE` | Default from `share/profile.json` (`test_levels.ZONE`). |
| `Z01_MNAME_NOT_IN_NS_LIST` | `INFO` | Default from `share/profile.json` (`test_levels.ZONE`). |
| `Z01_MNAME_NOT_MASTER` | `NOTICE` | Default from `share/profile.json` (`test_levels.ZONE`). |
| `Z01_MNAME_NOT_RESOLVE` | `NOTICE` | Default from `share/profile.json` (`test_levels.ZONE`). |
| `Z01_MNAME_NO_RESPONSE` | `NOTICE` | Default from `share/profile.json` (`test_levels.ZONE`). |
| `Z01_MNAME_UNEXPECTED_RCODE` | `NOTICE` | Default from `share/profile.json` (`test_levels.ZONE`). |

## Differences From Upstream
- Upstream reference: [`zone01.md`](../../upstream/tests/Zone-TP/zone01.md)
- Differences (Upstream vs Gonemaster):
  - Upstream: does not describe testcase boundary debug markers in testcase outputs. Gonemaster: emits `TEST_CASE_START` and `TEST_CASE_END`.
  - Upstream: describes MNAME non-resolve handling per MNAME name. Gonemaster: uses a cumulative `foundIP` counter across all MNAME names, which can suppress `Z01_MNAME_NOT_RESOLVE` for later unresolved MNAME values after any earlier MNAME resolved.
  - Upstream: describes processing a name server IP set. Gonemaster: iterates raw `Method4and5` output (no testcase-local IP deduplication before initial SOA probing).
- Potential upstream report:
  - `no`

## Edge Cases And Limitations
- Only child SOA responses that are `NOERROR`, authoritative, and include SOA for the zone name contribute MNAME candidates.
- If no such candidate is collected, the testcase ends without emitting a dedicated “missing candidate MNAME” result tag.
- Transport-family skips (`IPV4_DISABLED`/`IPV6_DISABLED`) can be emitted by shared helper paths, but these tags are currently outside this testcase metadata contract.
