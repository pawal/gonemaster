# Delegation04 (delegation04)

Status: Final

## Purpose
- Verify whether nameservers from delegation and child sources answer authoritatively for SOA queries.

## Preconditions And Inputs
- Preconditions:
  - A `zone.Zone` object is available.
- Required inputs:
  - Delegation addressed NS from `methods.Method4`.
  - Child addressed NS from `methods.Method5`.
  - SOA query responses over UDP and TCP from each evaluated nameserver.
- Profile/config knobs that affect behavior:
  - `net.ipv4` and `net.ipv6`: disabled transports emit transport-debug tags and skip query evaluation on that transport.
  - `resolver.defaults.parallel`: parallel nameserver evaluation fanout.

## Algorithm And Decision Flow
1. Emit `TEST_CASE_START`.
2. Read delegation addressed NS (`Method4`) and child addressed NS (`Method5`), concatenate in that order.
3. Build an ordered task list:
   - Nameservers are deduplicated by NS name (`nameKey`), not by IP.
   - If transport family is disabled, task is marked as disabled.
   - Duplicate name entries after first occurrence are skipped.
4. Execute tasks in parallel (deterministic merged log order):
   - For disabled tasks, emit `IPV4_DISABLED` or `IPV6_DISABLED` (`rrtype=SOA`).
   - For query tasks, send SOA query twice (`UseVC=false` for UDP, then `UseVC=true` for TCP).
   - For each non-nil response where `AA=false`, emit `IS_NOT_AUTHORITATIVE` with `proto`.
   - Track whether the nameserver produced at least one authoritative response (`AA=true`) across UDP/TCP.
5. After all tasks, emit `ARE_AUTHORITATIVE` with sorted unique NS names only when all conditions are true:
   - At least one nameserver was present (`Method4` or `Method5` non-empty).
   - No `IS_NOT_AUTHORITATIVE` tag has been emitted (transport-disabled debug tags do not suppress this).
   - At least one nameserver was observed authoritative.
6. Emit `TEST_CASE_END`.

### Per-NS SOA Probe and Authoritative Aggregation (steps 2-6)

{{% expand "Show diagram" %}}
{{< mermaid >}}
stateDiagram-v2
    [*] --> dedupe
    dedupe : dedup NS by name
    dedupe --> probe
    probe : per-NS SOA probe
    probe --> disabled : transport off
    probe --> query : transport on
    disabled : transport-disabled tag
    query : UDP then TCP SOA
    query --> notAuth : AA false
    query --> isAuth : AA true seen
    notAuth : not-authoritative tag
    isAuth : NS is authoritative
    isAuth --> aggregate
    notAuth --> aggregate
    aggregate : after all NS
    aggregate --> emitAuth : conditions met
    aggregate --> skip : not met
    emitAuth : are-authoritative tag
    skip --> done
    emitAuth --> done
    done : emit test-case-end
    disabled --> [*]
    done --> [*]
{{< /mermaid >}}
{{% /expand %}}

## Emitted Tags (Possible Set)
| Tag | Emitted when |
| --- | --- |
| `ARE_AUTHORITATIVE` | At least one evaluated nameserver was authoritative and no `IS_NOT_AUTHORITATIVE` tag was emitted. |
| `IPV4_DISABLED` | IPv4 nameserver evaluation is skipped because IPv4 is disabled. |
| `IPV6_DISABLED` | IPv6 nameserver evaluation is skipped because IPv6 is disabled. |
| `IS_NOT_AUTHORITATIVE` | A SOA response was received with `AA=false` on UDP or TCP. |
| `TEST_CASE_END` | Testcase completion marker is emitted. |
| `TEST_CASE_START` | Testcase start marker is emitted. |

## Tag Arguments
| Tag | Argument key | Type | Meaning |
| --- | --- | --- | --- |
| `ARE_AUTHORITATIVE` | `servers` | `array<object>` | Structured authoritative nameserver names as `{ns}` items. |
| `IPV4_DISABLED` | `ns` | `string` | Nameserver identity (`ns` name only; use `address` for IP) skipped on IPv4. |
| `IPV4_DISABLED` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `IPV4_DISABLED` | `rrtype` | `string` | rrtype skipped (`SOA`). |
| `IPV6_DISABLED` | `ns` | `string` | Nameserver identity (`ns` name only; use `address` for IP) skipped on IPv6. |
| `IPV6_DISABLED` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `IPV6_DISABLED` | `rrtype` | `string` | rrtype skipped (`SOA`). |
| `IS_NOT_AUTHORITATIVE` | `ns` | `string` | Nameserver identity (`ns` name only; use `address` for IP) with non-authoritative SOA response. |
| `IS_NOT_AUTHORITATIVE` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `IS_NOT_AUTHORITATIVE` | `proto` | `string` | Query transport label (`UDP` or `TCP`). |
| `TEST_CASE_END` | `testcase` | `string` | Testcase display name (`Delegation04`). |
| `TEST_CASE_START` | `testcase` | `string` | Testcase display name (`Delegation04`). |

## Severity Levels Per Tag
| Tag | Level | Notes |
| --- | --- | --- |
| `ARE_AUTHORITATIVE` | `INFO` | Default from `share/profile.json` (`test_levels.DELEGATION`). |
| `IPV4_DISABLED` | `DEBUG` | Default from `share/profile.json` (`test_levels.DELEGATION`). |
| `IPV6_DISABLED` | `DEBUG` | Default from `share/profile.json` (`test_levels.DELEGATION`). |
| `IS_NOT_AUTHORITATIVE` | `WARNING` | Default from `share/profile.json` (`test_levels.DELEGATION`). |
| `TEST_CASE_END` | `DEBUG` | Default from `share/profile.json` (`test_levels.DELEGATION`). |
| `TEST_CASE_START` | `DEBUG` | Default from `share/profile.json` (`test_levels.DELEGATION`). |

## Differences From Upstream
- Upstream reference: [`delegation04.md`](../../upstream/tests/Delegation-TP/delegation04.md)
- Differences (Upstream vs Gonemaster):
  - Upstream: says uniquely obtained address records are evaluated. Gonemaster: deduplicates by NS name, so distinct IPs under one NS name are not independently evaluated.
  - Upstream: describes pass/fail outcome semantics without explicit per-response message protocol. Gonemaster: emits per-response `IS_NOT_AUTHORITATIVE` findings and emits `ARE_AUTHORITATIVE` only when no other testcase findings were emitted.
  - Upstream: does not describe testcase boundary debug markers. Gonemaster: emits `TEST_CASE_START` and `TEST_CASE_END`.
- Potential upstream report:
  - `yes`
- If yes, include:
  - Upstream expected behavior: Uniqueness and evaluation unit are described at address-record granularity.
  - Gonemaster observed behavior: Uniqueness and evaluation are keyed by NS name.
  - evidence: `docs/specifications/upstream/tests/Delegation-TP/delegation04.md`, `engine/test/delegation/delegation.go`
  - report status: `not filed`

## Edge Cases And Limitations
- Missing/no-response cases emit no dedicated tag in this testcase.
- `ARE_AUTHORITATIVE` is suppressed only by `IS_NOT_AUTHORITATIVE`; transport-disabled debug tags (`IPV4_DISABLED`, `IPV6_DISABLED`) do not suppress it.
- If all SOA queries fail to return usable responses, the testcase emits only `TEST_CASE_START`, `TEST_CASE_END`, and, if any transport is disabled, `IPV4_DISABLED` and/or `IPV6_DISABLED`.
