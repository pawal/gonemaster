# Nameserver08 (nameserver08)

Status: Final

## Purpose
- Check whether nameservers preserve or alter query-name case in the echoed question section.

## Preconditions And Inputs
- Preconditions:
  - A `zone.Zone` object is available.
- Required inputs:
  - Nameserver addresses from `methods.Method4and5`.
  - SOA response question sections for randomized-case `www.<zone>` query names.
- Profile/config knobs that affect behavior:
  - `net.ipv4` and `net.ipv6`: disabled transports are skipped with transport debug tags.
  - `resolver.defaults.parallel`: parallel nameserver query fanout.

## Algorithm And Decision Flow
1. Emit `TEST_CASE_START`.
2. Build `original = "www." + <zone>` (trailing dot removed).
3. Generate `randomized` by scrambling letter case until it differs from `original`.
4. Read nameserver list from `Method4and5`, deduplicate by `name/ip`, preserving first-seen order.
5. For each deduplicated nameserver (parallelized, input-order merged logs):
   - If transport is disabled, emit `IPV4_DISABLED` or `IPV6_DISABLED` for rrtype `SOA`, then skip.
   - Query `randomized` with rrtype `SOA`.
   - If response exists and question section is non-empty:
     - Compare first question name (trim trailing dot) to `randomized`.
     - If equal, emit `QNAME_CASE_SENSITIVE`.
     - Else, emit `QNAME_CASE_INSENSITIVE`.
6. Emit `TEST_CASE_END`.

## Emitted Tags (Possible Set)
| Tag | Emitted when |
| --- | --- |
| `IPV4_DISABLED` | IPv4 nameserver evaluation is skipped because IPv4 is disabled. |
| `IPV6_DISABLED` | IPv6 nameserver evaluation is skipped because IPv6 is disabled. |
| `QNAME_CASE_INSENSITIVE` | Response question name does not preserve the randomized query case exactly. |
| `QNAME_CASE_SENSITIVE` | Response question name preserves the randomized query case exactly. |
| `TEST_CASE_END` | Testcase completion marker is emitted. |
| `TEST_CASE_START` | Testcase start marker is emitted. |

## Tag Arguments
| Tag | Argument key | Type | Meaning |
| --- | --- | --- | --- |
| `IPV4_DISABLED` | `ns` | `string` | Nameserver identity (`name/ip`) skipped on IPv4. |
| `IPV4_DISABLED` | `rrtype` | `string` | rrtype skipped (`SOA`). |
| `IPV6_DISABLED` | `ns` | `string` | Nameserver identity (`name/ip`) skipped on IPv6. |
| `IPV6_DISABLED` | `rrtype` | `string` | rrtype skipped (`SOA`). |
| `QNAME_CASE_INSENSITIVE` | `ns` | `string` | Nameserver identity (`name/ip`) with case-insensitive echo behavior. |
| `QNAME_CASE_INSENSITIVE` | `domain` | `string` | Randomized query name used for check. |
| `QNAME_CASE_SENSITIVE` | `ns` | `string` | Nameserver identity (`name/ip`) with case-preserving echo behavior. |
| `QNAME_CASE_SENSITIVE` | `domain` | `string` | Randomized query name used for check. |
| `TEST_CASE_END` | `testcase` | `string` | Testcase display name (`Nameserver08`). |
| `TEST_CASE_START` | `testcase` | `string` | Testcase display name (`Nameserver08`). |

## Severity Levels Per Tag
| Tag | Level | Notes |
| --- | --- | --- |
| `IPV4_DISABLED` | `DEBUG` | Default from `share/profile.json` (`test_levels.NAMESERVER`). |
| `IPV6_DISABLED` | `DEBUG` | Default from `share/profile.json` (`test_levels.NAMESERVER`). |
| `QNAME_CASE_INSENSITIVE` | `WARNING` | Default from `share/profile.json` (`test_levels.NAMESERVER`). |
| `QNAME_CASE_SENSITIVE` | `INFO` | Default from `share/profile.json` (`test_levels.NAMESERVER`). |
| `TEST_CASE_END` | `DEBUG` | Default from `share/profile.json` (`test_levels.NAMESERVER`). |
| `TEST_CASE_START` | `DEBUG` | Default from `share/profile.json` (`test_levels.NAMESERVER`). |

## Differences From Upstream
- Upstream reference: [`nameserver08.md`](../../upstream/tests/Nameserver-TP/nameserver08.md)
- Differences (Upstream vs Gonemaster):
  - Upstream: describes a failure condition; messaging detail is not defined. Gonemaster: emits per-server explicit tags `QNAME_CASE_SENSITIVE` and `QNAME_CASE_INSENSITIVE`.
  - Upstream: describes querying unique nameserver IPs. Gonemaster: deduplicates by `name/ip` and then queries.
  - Upstream: does not explicitly describe testcase boundary and transport-disabled debug emissions. Gonemaster: emits `TEST_CASE_START`, `TEST_CASE_END`, `IPV4_DISABLED`, and `IPV6_DISABLED`.
- Potential upstream report:
  - `no`

## Edge Cases And Limitations
- If response is missing or question section is empty, no case-behavior tag is emitted for that nameserver.
- Only the first question entry in the response is evaluated.
- Classification is based on exact string comparison against the randomized query name.
