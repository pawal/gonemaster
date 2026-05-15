# Zone05

Status: Final

## Purpose
- Validate SOA `expire` constraints:
  - `expire` must be at or above configured minimum;
  - `expire` must not be lower than `refresh`.

## Preconditions And Inputs
- Preconditions:
  - A `zone.Zone` object is available.
- Required inputs:
  - Child nameserver addresses from `methods.Method5`.
  - One authoritative SOA response for the child zone apex (if obtainable).
- Profile/config knobs that affect behavior:
  - `test_cases_vars.zone05.soa_expire_minimum_value` (`Zone05.SOAExpireMinimumValue` in code): minimum accepted `expire`.
  - `net.ipv4` and `net.ipv6` affect transport availability during SOA retrieval.

## Algorithm And Decision Flow
1. Emit `TEST_CASE_START`.
2. Retrieve SOA from child nameservers using shared helper logic:
   - iterate `Method5` nameservers in order;
   - skip disabled transports;
   - return the first response that has SOA in answer and `AA=true`.
3. If no qualifying SOA response is found, emit `NO_RESPONSE_SOA_QUERY`.
4. Else read SOA `expire` and `refresh` and evaluate:
   - if `expire < required_expire`, emit `EXPIRE_MINIMUM_VALUE_LOWER`;
   - if `expire < refresh`, emit `EXPIRE_LOWER_THAN_REFRESH`;
   - if no non-start tag has been emitted in testcase state so far, emit `EXPIRE_MINIMUM_VALUE_OK`.
5. Emit `TEST_CASE_END`.

## Emitted Tags (Possible Set)
| Tag | Emitted when |
| --- | --- |
| `EXPIRE_LOWER_THAN_REFRESH` | SOA `expire` is lower than SOA `refresh`. |
| `EXPIRE_MINIMUM_VALUE_LOWER` | SOA `expire` is below configured minimum. |
| `EXPIRE_MINIMUM_VALUE_OK` | No non-start tag was emitted during the testcase (gating condition): `expire` is not below the configured minimum, not below `refresh`, and no shared-helper debug tag was emitted. |
| `NO_RESPONSE_SOA_QUERY` | No authoritative SOA response containing an SOA answer record was received from any queried nameserver. |
| `TEST_CASE_END` | Testcase completion marker is emitted. |
| `TEST_CASE_START` | Testcase start marker is emitted. |

## Tag Arguments
| Tag | Argument key | Type | Meaning |
| --- | --- | --- | --- |
| `EXPIRE_LOWER_THAN_REFRESH` | `expire` | `int` | Observed SOA `expire` value. |
| `EXPIRE_LOWER_THAN_REFRESH` | `refresh` | `int` | Observed SOA `refresh` value. |
| `EXPIRE_MINIMUM_VALUE_LOWER` | `expire` | `int` | Observed SOA `expire` value. |
| `EXPIRE_MINIMUM_VALUE_LOWER` | `required_expire` | `int` | Configured minimum expire threshold. |
| `EXPIRE_MINIMUM_VALUE_OK` | `expire` | `int` | Observed SOA `expire` value. |
| `EXPIRE_MINIMUM_VALUE_OK` | `refresh` | `int` | Observed SOA `refresh` value. |
| `EXPIRE_MINIMUM_VALUE_OK` | `required_expire` | `int` | Configured minimum expire threshold. |
| `NO_RESPONSE_SOA_QUERY` | `-` | `-` | No arguments. |
| `TEST_CASE_END` | `testcase` | `string` | Testcase display name (`Zone05`). |
| `TEST_CASE_START` | `testcase` | `string` | Testcase display name (`Zone05`). |

## Severity Levels Per Tag
| Tag | Level | Notes |
| --- | --- | --- |
| `EXPIRE_LOWER_THAN_REFRESH` | `WARNING` | Default from `share/profile.json` (`test_levels.ZONE`). |
| `EXPIRE_MINIMUM_VALUE_LOWER` | `WARNING` | Default from `share/profile.json` (`test_levels.ZONE`). |
| `EXPIRE_MINIMUM_VALUE_OK` | `INFO` | Default from `share/profile.json` (`test_levels.ZONE`). |
| `NO_RESPONSE_SOA_QUERY` | `DEBUG` | Default from `share/profile.json` (`test_levels.ZONE`). |
| `TEST_CASE_END` | `DEBUG` | Default from `share/profile.json` (`test_levels.ZONE`). |
| `TEST_CASE_START` | `DEBUG` | Default from `share/profile.json` (`test_levels.ZONE`). |

## Differences From Upstream
- Upstream reference: [`zone05.md`](../../upstream/tests/Zone-TP/zone05.md)
- Differences (Upstream vs Gonemaster):
  - Upstream: specifies a fixed minimum of `604800` seconds. Gonemaster: uses profile-configurable minimum (`Zone05.SOAExpireMinimumValue`).
  - Upstream: describes below-threshold or lower-than-refresh cases as testcase failure. Gonemaster: emits warning-level findings (`EXPIRE_MINIMUM_VALUE_LOWER`, `EXPIRE_LOWER_THAN_REFRESH`) under default profile.
  - Upstream: does not define an explicit positive-result tag. Gonemaster: emits `EXPIRE_MINIMUM_VALUE_OK` when no non-start findings were emitted in current testcase state.
  - Upstream: does not describe testcase boundary debug markers. Gonemaster: emits `TEST_CASE_START` and `TEST_CASE_END`.
- Potential upstream report:
  - `yes`
- If yes, include:
  - Upstream expected behavior: Upstream prose defines fixed-threshold fail semantics for both expire checks.
  - Gonemaster observed behavior: Threshold is profile-driven and outcomes are tag/severity based (default warnings plus an explicit OK tag branch).
  - evidence: `docs/specifications/upstream/tests/Zone-TP/zone05.md`, `engine/test/zone/zone.go`, `share/profile.json`
  - report status: `not filed`

## Edge Cases And Limitations
- The shared retrieval helper emits `IPV4_DISABLED` or `IPV6_DISABLED` when the corresponding transport is disabled; these tags are not declared in this testcase's `Metadata()` function.
- Because `EXPIRE_MINIMUM_VALUE_OK` uses a generic “no non-start entry yet” gate, helper-emitted non-metadata entries can suppress the OK tag even when numeric checks are satisfied.
