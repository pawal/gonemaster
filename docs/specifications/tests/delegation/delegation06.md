# Delegation06

Status: Final

## Purpose
- Verify SOA RRset existence on nameservers collected from delegation and child sources.

## Preconditions And Inputs
- Preconditions:
  - A `zone.Zone` object is available.
- Required inputs:
  - Delegation addressed NS from [`GlueNameservers`](../../nameserver-resolution.md#gluenameservers).
  - Child addressed NS from [`ApexNameservers`](../../nameserver-resolution.md#apexnameservers).
  - SOA query responses from each evaluated nameserver.
- Profile/config knobs that affect behavior:
  - `net.ipv4` and `net.ipv6`: disabled transports emit transport-debug tags and skip query evaluation on that transport.
  - `resolver.defaults.parallel`: parallel nameserver evaluation fanout.

## Algorithm And Decision Flow
1. Emit `TEST_CASE_START`.
2. Read delegation addressed NS ([`GlueNameservers`](../../nameserver-resolution.md#gluenameservers)) and child addressed NS ([`ApexNameservers`](../../nameserver-resolution.md#apexnameservers)), concatenate in that order.
3. Build an ordered task list:
   - Nameservers are deduplicated by NS name (`nameKey`), not by IP.
   - If transport family is disabled, task is marked as disabled.
   - Duplicate name entries after first occurrence are skipped.
4. Execute tasks in parallel (deterministic merged log order):
   - For disabled tasks, emit `IPV4_DISABLED` or `IPV6_DISABLED` (`rrtype=SOA`).
   - For query tasks, query SOA.
   - If response has `RCODE=NOERROR` and SOA answer section is empty, emit `SOA_NOT_EXISTS` (`ns`).
5. After all tasks, emit `SOA_EXISTS` only when both conditions are true:
   - At least one nameserver was present ([`GlueNameservers`](../../nameserver-resolution.md#gluenameservers) or [`ApexNameservers`](../../nameserver-resolution.md#apexnameservers) non-empty).
   - No `SOA_NOT_EXISTS` tag has been emitted (transport-disabled debug tags do not suppress this).
6. Emit `TEST_CASE_END`.

## Emitted Tags (Possible Set)
| Tag | Emitted when |
| --- | --- |
| `IPV4_DISABLED` | IPv4 nameserver evaluation is skipped because IPv4 is disabled. |
| `IPV6_DISABLED` | IPv6 nameserver evaluation is skipped because IPv6 is disabled. |
| `SOA_EXISTS` | Nameserver set is non-empty and no `SOA_NOT_EXISTS` tag was emitted. |
| `SOA_NOT_EXISTS` | SOA query returned `NOERROR` but answer section had no SOA RR. |
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
| `SOA_EXISTS` | `-` | `-` | No arguments. |
| `SOA_NOT_EXISTS` | `ns` | `string` | Nameserver identity (`ns` name only; use `address` for IP) with empty SOA answer on `NOERROR`. |
| `SOA_NOT_EXISTS` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `TEST_CASE_END` | `testcase` | `string` | Testcase display name (`Delegation06`). |
| `TEST_CASE_START` | `testcase` | `string` | Testcase display name (`Delegation06`). |

## Severity Levels Per Tag
| Tag | Level | Notes |
| --- | --- | --- |
| `IPV4_DISABLED` | `DEBUG` | Default from `share/profile.json` (`test_levels.DELEGATION`). |
| `IPV6_DISABLED` | `DEBUG` | Default from `share/profile.json` (`test_levels.DELEGATION`). |
| `SOA_EXISTS` | `INFO` | Default from `share/profile.json` (`test_levels.DELEGATION`). |
| `SOA_NOT_EXISTS` | `ERROR` | Default from `share/profile.json` (`test_levels.DELEGATION`). |
| `TEST_CASE_END` | `DEBUG` | Default from `share/profile.json` (`test_levels.DELEGATION`). |
| `TEST_CASE_START` | `DEBUG` | Default from `share/profile.json` (`test_levels.DELEGATION`). |

## Differences From Upstream
- Differences (Upstream vs Gonemaster):
  - Upstream: says uniquely obtained address records are evaluated. Gonemaster: deduplicates by NS name, so distinct IPs under one NS name are not independently evaluated.
  - Upstream: describes testcase success when SOA exists. Gonemaster: emits explicit `SOA_EXISTS` only when no other testcase findings were emitted.
  - Upstream: does not describe testcase boundary debug markers. Gonemaster: emits `TEST_CASE_START` and `TEST_CASE_END`.
- Potential upstream report:
  - `yes`
- If yes, include:
  - Upstream expected behavior: Address-record uniqueness is the evaluation unit.
  - Gonemaster observed behavior: NS-name uniqueness is the evaluation unit.
  - evidence: `docs/specifications/upstream/tests/Delegation-TP/delegation06.md`, `engine/test/delegation/delegation.go`
  - report status: `not filed`

## Edge Cases And Limitations
- Non-`NOERROR` responses and query failures do not emit `SOA_NOT_EXISTS`.
- `SOA_EXISTS` is suppressed only by `SOA_NOT_EXISTS`; transport-disabled debug tags (`IPV4_DISABLED`, `IPV6_DISABLED`) do not suppress it.
- If no nameservers are available from [`GlueNameservers`](../../nameserver-resolution.md#gluenameservers) and [`ApexNameservers`](../../nameserver-resolution.md#apexnameservers), testcase emits start/end only.
