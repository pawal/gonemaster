# Zone06 (zone06)

Status: Draft

## Purpose
- Validate that SOA default TTL (`minimum` field) is within configured lower and upper bounds.

## Preconditions And Inputs
- Preconditions:
  - A `zone.Zone` object is available.
- Required inputs:
  - Child nameserver addresses from `methods.Method5`.
  - One authoritative SOA response for the child zone apex (if obtainable).
- Profile/config knobs that affect behavior:
  - `test_cases_vars.zone06.soa_default_ttl_maximum_value` (`Zone06.SOADefaultTTLMaximumValue` in code): upper bound.
  - `test_cases_vars.zone06.soa_default_ttl_minimum_value` (`Zone06.SOADefaultTTLMinimumValue` in code): lower bound.
  - `net.ipv4` and `net.ipv6` affect transport availability during SOA retrieval.

## Algorithm And Decision Flow
1. Emit `TEST_CASE_START`.
2. Retrieve SOA from child nameservers using shared helper logic:
   - iterate `Method5` nameservers in order;
   - skip disabled transports;
   - return the first response that has SOA in answer and `AA=true`.
3. If no qualifying SOA response is found, emit `NO_RESPONSE_SOA_QUERY`.
4. Else read SOA `minimum` (`Minttl`) and compare against configured bounds:
   - if `minimum > highest_minimum`, emit `SOA_DEFAULT_TTL_MAXIMUM_VALUE_HIGHER`;
   - else if `minimum < lowest_minimum`, emit `SOA_DEFAULT_TTL_MAXIMUM_VALUE_LOWER`;
   - else emit `SOA_DEFAULT_TTL_MAXIMUM_VALUE_OK`.
5. Emit `TEST_CASE_END`.

## Emitted Tags (Possible Set)
| Tag | Emitted when |
| --- | --- |
| `NO_RESPONSE_SOA_QUERY` | No qualifying authoritative SOA response with SOA answer could be obtained. |
| `SOA_DEFAULT_TTL_MAXIMUM_VALUE_HIGHER` | SOA `minimum` is above configured upper bound. |
| `SOA_DEFAULT_TTL_MAXIMUM_VALUE_LOWER` | SOA `minimum` is below configured lower bound. |
| `SOA_DEFAULT_TTL_MAXIMUM_VALUE_OK` | SOA `minimum` is within configured bounds (inclusive). |
| `TEST_CASE_END` | Testcase completion marker is emitted. |
| `TEST_CASE_START` | Testcase start marker is emitted. |

## Tag Arguments
| Tag | Argument key | Type | Meaning |
| --- | --- | --- | --- |
| `NO_RESPONSE_SOA_QUERY` | `-` | `-` | No arguments. |
| `SOA_DEFAULT_TTL_MAXIMUM_VALUE_HIGHER` | `minimum` | `int` | Observed SOA `minimum` (Minttl). |
| `SOA_DEFAULT_TTL_MAXIMUM_VALUE_HIGHER` | `highest_minimum` | `int` | Configured maximum allowed value. |
| `SOA_DEFAULT_TTL_MAXIMUM_VALUE_LOWER` | `minimum` | `int` | Observed SOA `minimum` (Minttl). |
| `SOA_DEFAULT_TTL_MAXIMUM_VALUE_LOWER` | `lowest_minimum` | `int` | Configured minimum allowed value. |
| `SOA_DEFAULT_TTL_MAXIMUM_VALUE_OK` | `minimum` | `int` | Observed SOA `minimum` (Minttl). |
| `SOA_DEFAULT_TTL_MAXIMUM_VALUE_OK` | `highest_minimum` | `int` | Configured maximum allowed value. |
| `SOA_DEFAULT_TTL_MAXIMUM_VALUE_OK` | `lowest_minimum` | `int` | Configured minimum allowed value. |
| `TEST_CASE_END` | `testcase` | `string` | Testcase display name (`Zone06`). |
| `TEST_CASE_START` | `testcase` | `string` | Testcase display name (`Zone06`). |

## Severity Levels Per Tag
| Tag | Level | Notes |
| --- | --- | --- |
| `NO_RESPONSE_SOA_QUERY` | `DEBUG` | Default from `share/profile.json` (`test_levels.ZONE`). |
| `SOA_DEFAULT_TTL_MAXIMUM_VALUE_HIGHER` | `NOTICE` | Default from `share/profile.json` (`test_levels.ZONE`). |
| `SOA_DEFAULT_TTL_MAXIMUM_VALUE_LOWER` | `NOTICE` | Default from `share/profile.json` (`test_levels.ZONE`). |
| `SOA_DEFAULT_TTL_MAXIMUM_VALUE_OK` | `INFO` | Default from `share/profile.json` (`test_levels.ZONE`). |
| `TEST_CASE_END` | `DEBUG` | Default from `share/profile.json` (`test_levels.ZONE`). |
| `TEST_CASE_START` | `DEBUG` | Default from `share/profile.json` (`test_levels.ZONE`). |

## Differences From Upstream
- Upstream reference: [`zone06.md`](../../upstream/tests/Zone-TP/zone06.md)
- Differences (Upstream vs Gonemaster):
  - Upstream: describes querying over a Method4+Method5-derived nameserver IP set. Gonemaster: retrieval helper iterates `Method5` child nameserver addresses only.
  - Upstream: describes fixed bounds (`300` to `86400`) as failure criteria. Gonemaster: uses profile-configurable bounds and notice-level findings for out-of-range cases under default profile.
  - Upstream: does not describe testcase boundary debug markers. Gonemaster: emits `TEST_CASE_START` and `TEST_CASE_END`.
- Potential upstream report:
  - `yes`
- If yes, include:
  - Upstream expected behavior: Upstream prose defines fixed numeric bounds and explicit fail semantics.
  - Gonemaster observed behavior: Bounds are profile-driven and out-of-range branches are tagged findings (`NOTICE` by default).
  - evidence: `docs/specifications/upstream/tests/Zone-TP/zone06.md`, `engine/test/zone/zone.go`, `share/profile.json`
  - report status: `not filed`

## Edge Cases And Limitations
- Shared retrieval helper may emit transport-disabled debug tags (`IPV4_DISABLED`/`IPV6_DISABLED`), but these are outside this testcase metadata contract.
- If helper cannot find an authoritative SOA answer, bound checks are not attempted.
