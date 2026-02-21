# Zone03 (zone03)

Status: Draft

## Purpose
- Validate ordering relationship between SOA timers: `refresh` should be greater than `retry`.

## Preconditions And Inputs
- Preconditions:
  - A `zone.Zone` object is available.
- Required inputs:
  - Child nameserver addresses from `methods.Method5`.
  - One authoritative SOA response for the child zone apex (if obtainable).
- Profile/config knobs that affect behavior:
  - `net.ipv4` and `net.ipv6` affect transport availability during SOA retrieval.

## Algorithm And Decision Flow
1. Emit `TEST_CASE_START`.
2. Retrieve SOA from child nameservers using shared helper logic:
   - iterate `Method5` nameservers in order;
   - skip disabled transports;
   - return the first response that has SOA in answer and `AA=true`.
3. If no qualifying SOA response is found, emit `NO_RESPONSE_SOA_QUERY`.
4. Else read SOA `refresh` and `retry`:
   - if `retry >= refresh`, emit `REFRESH_LOWER_THAN_RETRY`;
   - else emit `REFRESH_HIGHER_THAN_RETRY`.
5. Emit `TEST_CASE_END`.

## Emitted Tags (Possible Set)
| Tag | Emitted when |
| --- | --- |
| `NO_RESPONSE_SOA_QUERY` | No qualifying authoritative SOA response with SOA answer could be obtained. |
| `REFRESH_HIGHER_THAN_RETRY` | SOA `refresh` is greater than SOA `retry`. |
| `REFRESH_LOWER_THAN_RETRY` | SOA `retry` is greater than or equal to SOA `refresh`. |
| `TEST_CASE_END` | Testcase completion marker is emitted. |
| `TEST_CASE_START` | Testcase start marker is emitted. |

## Tag Arguments
| Tag | Argument key | Type | Meaning |
| --- | --- | --- | --- |
| `NO_RESPONSE_SOA_QUERY` | `-` | `-` | No arguments. |
| `REFRESH_HIGHER_THAN_RETRY` | `retry` | `int` | Observed SOA `retry` value. |
| `REFRESH_HIGHER_THAN_RETRY` | `refresh` | `int` | Observed SOA `refresh` value. |
| `REFRESH_LOWER_THAN_RETRY` | `retry` | `int` | Observed SOA `retry` value. |
| `REFRESH_LOWER_THAN_RETRY` | `refresh` | `int` | Observed SOA `refresh` value. |
| `TEST_CASE_END` | `testcase` | `string` | Testcase display name (`Zone03`). |
| `TEST_CASE_START` | `testcase` | `string` | Testcase display name (`Zone03`). |

## Severity Levels Per Tag
| Tag | Level | Notes |
| --- | --- | --- |
| `NO_RESPONSE_SOA_QUERY` | `DEBUG` | Default from `share/profile.json` (`test_levels.ZONE`). |
| `REFRESH_HIGHER_THAN_RETRY` | `INFO` | Default from `share/profile.json` (`test_levels.ZONE`). |
| `REFRESH_LOWER_THAN_RETRY` | `INFO` | Default from `share/profile.json` (`test_levels.ZONE`). |
| `TEST_CASE_END` | `DEBUG` | Default from `share/profile.json` (`test_levels.ZONE`). |
| `TEST_CASE_START` | `DEBUG` | Default from `share/profile.json` (`test_levels.ZONE`). |

## Differences From Upstream
- Upstream reference: [`zone03.md`](../../upstream/tests/Zone-TP/zone03.md)
- Differences (Upstream vs Gonemaster):
  - Upstream: describes `retry >= refresh` as testcase failure. Gonemaster: emits `REFRESH_LOWER_THAN_RETRY` with default severity `INFO`.
  - Upstream: does not define explicit result tag for retrieval failure. Gonemaster: emits `NO_RESPONSE_SOA_QUERY`.
  - Upstream: does not describe testcase boundary debug markers. Gonemaster: emits `TEST_CASE_START` and `TEST_CASE_END`.
- Potential upstream report:
  - `yes`
- If yes, include:
  - Upstream expected behavior: Upstream prose defines `retry >= refresh` as failure condition.
  - Gonemaster observed behavior: The failure branch is an informational tag (`REFRESH_LOWER_THAN_RETRY`) under default profile.
  - evidence: `docs/specifications/upstream/tests/Zone-TP/zone03.md`, `engine/test/zone/zone.go`, `share/profile.json`
  - report status: `not filed`

## Edge Cases And Limitations
- Shared retrieval helper may emit transport-disabled debug tags (`IPV4_DISABLED`/`IPV6_DISABLED`), but these are outside this testcase metadata contract.
- If helper cannot find an authoritative SOA answer, timer-order comparison is not attempted.
