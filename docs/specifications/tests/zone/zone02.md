# Zone02

Status: Final

## Purpose
- Validate that SOA `refresh` is at or above the configured minimum threshold.

## Preconditions And Inputs
- Preconditions:
  - A `zone.Zone` object is available.
- Required inputs:
  - Child nameserver addresses from [`ApexNameservers`](../../nameserver-resolution.md#apexnameservers).
  - One authoritative SOA response for the child zone apex (if obtainable).
- Profile/config knobs that affect behavior:
  - `test_cases_vars.zone02.soa_refresh_minimum_value` (`Zone02.SOARefreshMinimumValue` in code): minimum accepted `refresh`.
  - `net.ipv4` and `net.ipv6` affect transport availability during SOA retrieval.

## Algorithm And Decision Flow
1. Emit `TEST_CASE_START`.
2. Retrieve SOA from child nameservers using shared helper logic:
   - iterate [`ApexNameservers`](../../nameserver-resolution.md#apexnameservers) nameservers in order;
   - skip disabled transports;
   - return the first response that has SOA in answer and `AA=true`.
3. If no qualifying SOA response is found, emit `NO_RESPONSE_SOA_QUERY`.
4. Else read SOA `refresh` and compare against configured minimum:
   - if `refresh < required_refresh`, emit `REFRESH_MINIMUM_VALUE_LOWER`;
   - else emit `REFRESH_MINIMUM_VALUE_OK`.
5. Emit `TEST_CASE_END`.

## Emitted Tags (Possible Set)
| Tag | Emitted when |
| --- | --- |
| `NO_RESPONSE_SOA_QUERY` | No authoritative SOA response containing an SOA answer record was received from any queried nameserver. |
| `REFRESH_MINIMUM_VALUE_LOWER` | SOA `refresh` is below configured minimum. |
| `REFRESH_MINIMUM_VALUE_OK` | SOA `refresh` is at or above configured minimum. |
| `TEST_CASE_END` | Testcase completion marker is emitted. |
| `TEST_CASE_START` | Testcase start marker is emitted. |

## Tag Arguments
| Tag | Argument key | Type | Meaning |
| --- | --- | --- | --- |
| `NO_RESPONSE_SOA_QUERY` | `-` | `-` | No arguments. |
| `REFRESH_MINIMUM_VALUE_LOWER` | `refresh` | `int` | Observed SOA `refresh` value. |
| `REFRESH_MINIMUM_VALUE_LOWER` | `required_refresh` | `int` | Configured minimum refresh threshold. |
| `REFRESH_MINIMUM_VALUE_OK` | `refresh` | `int` | Observed SOA `refresh` value. |
| `REFRESH_MINIMUM_VALUE_OK` | `required_refresh` | `int` | Configured minimum refresh threshold. |
| `TEST_CASE_END` | `testcase` | `string` | Testcase display name (`Zone02`). |
| `TEST_CASE_START` | `testcase` | `string` | Testcase display name (`Zone02`). |

## Severity Levels Per Tag
| Tag | Level | Notes |
| --- | --- | --- |
| `NO_RESPONSE_SOA_QUERY` | `DEBUG` | Default from `share/profile.json` (`test_levels.ZONE`). |
| `REFRESH_MINIMUM_VALUE_LOWER` | `NOTICE` | Default from `share/profile.json` (`test_levels.ZONE`). |
| `REFRESH_MINIMUM_VALUE_OK` | `INFO` | Default from `share/profile.json` (`test_levels.ZONE`). |
| `TEST_CASE_END` | `DEBUG` | Default from `share/profile.json` (`test_levels.ZONE`). |
| `TEST_CASE_START` | `DEBUG` | Default from `share/profile.json` (`test_levels.ZONE`). |

## Differences From Upstream
- Differences (Upstream vs Gonemaster):
  - Upstream: specifies a fixed minimum of `14400` seconds. Gonemaster: uses profile-configurable minimum (`Zone02.SOARefreshMinimumValue`).
  - Upstream: describes this condition as testcase failure when below threshold. Gonemaster: emits `REFRESH_MINIMUM_VALUE_LOWER` with default severity `NOTICE` (profile-controlled), not a hardcoded fail outcome.
  - Upstream: does not describe testcase boundary debug markers. Gonemaster: emits `TEST_CASE_START` and `TEST_CASE_END`.
- Potential upstream report:
  - `yes`
- If yes, include:
  - Upstream expected behavior: Upstream prose defines below-threshold refresh as testcase failure with fixed numeric minimum.
  - Gonemaster observed behavior: Threshold is profile-driven and below-threshold result is a tagged finding (`NOTICE` by default), not a fixed fail primitive.
  - evidence: `engine/test/zone/zone.go`, `share/profile.json`
  - report status: `not filed`

## Edge Cases And Limitations
- The shared retrieval helper emits `IPV4_DISABLED` or `IPV6_DISABLED` when the corresponding transport is disabled; these tags are not declared in this testcase's `Metadata()` function.
- If helper cannot find an authoritative SOA answer, no threshold comparison is attempted.
