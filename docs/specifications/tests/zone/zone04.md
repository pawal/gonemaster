# Zone04

Status: Final

## Purpose
- Validate that SOA `retry` is at or above the configured minimum threshold.

## Preconditions And Inputs
- Preconditions:
  - A `zone.Zone` object is available.
- Required inputs:
  - Child nameserver addresses from `methods.Method5`.
  - One authoritative SOA response for the child zone apex (if obtainable).
- Profile/config knobs that affect behavior:
  - `test_cases_vars.zone04.soa_retry_minimum_value` (`Zone04.SOARetryMinimumValue` in code): minimum accepted `retry`.
  - `net.ipv4` and `net.ipv6` affect transport availability during SOA retrieval.

## Algorithm And Decision Flow
1. Emit `TEST_CASE_START`.
2. Retrieve SOA from child nameservers using shared helper logic:
   - iterate `Method5` nameservers in order;
   - skip disabled transports;
   - return the first response that has SOA in answer and `AA=true`.
3. If no qualifying SOA response is found, emit `NO_RESPONSE_SOA_QUERY`.
4. Else read SOA `retry` and compare against configured minimum:
   - if `retry < required_retry`, emit `RETRY_MINIMUM_VALUE_LOWER`;
   - else emit `RETRY_MINIMUM_VALUE_OK`.
5. Emit `TEST_CASE_END`.

## Emitted Tags (Possible Set)
| Tag | Emitted when |
| --- | --- |
| `NO_RESPONSE_SOA_QUERY` | No authoritative SOA response containing an SOA answer record was received from any queried nameserver. |
| `RETRY_MINIMUM_VALUE_LOWER` | SOA `retry` is below configured minimum. |
| `RETRY_MINIMUM_VALUE_OK` | SOA `retry` is at or above configured minimum. |
| `TEST_CASE_END` | Testcase completion marker is emitted. |
| `TEST_CASE_START` | Testcase start marker is emitted. |

## Tag Arguments
| Tag | Argument key | Type | Meaning |
| --- | --- | --- | --- |
| `NO_RESPONSE_SOA_QUERY` | `-` | `-` | No arguments. |
| `RETRY_MINIMUM_VALUE_LOWER` | `retry` | `int` | Observed SOA `retry` value. |
| `RETRY_MINIMUM_VALUE_LOWER` | `required_retry` | `int` | Configured minimum retry threshold. |
| `RETRY_MINIMUM_VALUE_OK` | `retry` | `int` | Observed SOA `retry` value. |
| `RETRY_MINIMUM_VALUE_OK` | `required_retry` | `int` | Configured minimum retry threshold. |
| `TEST_CASE_END` | `testcase` | `string` | Testcase display name (`Zone04`). |
| `TEST_CASE_START` | `testcase` | `string` | Testcase display name (`Zone04`). |

## Severity Levels Per Tag
| Tag | Level | Notes |
| --- | --- | --- |
| `NO_RESPONSE_SOA_QUERY` | `DEBUG` | Default from `share/profile.json` (`test_levels.ZONE`). |
| `RETRY_MINIMUM_VALUE_LOWER` | `NOTICE` | Default from `share/profile.json` (`test_levels.ZONE`). |
| `RETRY_MINIMUM_VALUE_OK` | `INFO` | Default from `share/profile.json` (`test_levels.ZONE`). |
| `TEST_CASE_END` | `DEBUG` | Default from `share/profile.json` (`test_levels.ZONE`). |
| `TEST_CASE_START` | `DEBUG` | Default from `share/profile.json` (`test_levels.ZONE`). |

## Differences From Upstream
- Upstream reference: [`zone04.md`](../../upstream/tests/Zone-TP/zone04.md)
- Differences (Upstream vs Gonemaster):
  - Upstream: specifies a fixed minimum of `3600` seconds. Gonemaster: uses profile-configurable minimum (`Zone04.SOARetryMinimumValue`).
  - Upstream: describes below-threshold `retry` as testcase failure. Gonemaster: emits `RETRY_MINIMUM_VALUE_LOWER` with default severity `NOTICE` (profile-controlled), not a hardcoded fail outcome.
  - Upstream: does not describe testcase boundary debug markers. Gonemaster: emits `TEST_CASE_START` and `TEST_CASE_END`.
- Potential upstream report:
  - `yes`
- If yes, include:
  - Upstream expected behavior: Upstream prose defines below-threshold retry as testcase failure with fixed numeric minimum.
  - Gonemaster observed behavior: Threshold is profile-driven and below-threshold result is a tagged finding (`NOTICE` by default), not a fixed fail primitive.
  - evidence: `docs/specifications/upstream/tests/Zone-TP/zone04.md`, `engine/test/zone/zone.go`, `share/profile.json`
  - report status: `not filed`

## Edge Cases And Limitations
- The shared retrieval helper emits `IPV4_DISABLED` or `IPV6_DISABLED` when the corresponding transport is disabled; these tags are not declared in this testcase's `Metadata()` function.
- If helper cannot find an authoritative SOA answer, threshold comparison is not attempted.
