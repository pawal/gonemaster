# Delegation01 (delegation01)

Status: Final

## Purpose
- Validate that delegation-side and child-side nameserver sets meet minimum-count requirements overall and per IP family.

## Preconditions And Inputs
- Preconditions:
  - A `zone.Zone` object is available.
- Required inputs:
  - Delegation names from `methods.Method2`.
  - Child NS names from `methods.Method3`.
  - Delegation nameserver addresses from `methods.Method4`.
  - Child nameserver addresses from `methods.Method5`.
- Profile/config knobs that affect behavior:
  - No direct profile knob in this testcase.
  - Minimum required nameserver count is fixed by `constants.MinimumNumberOfNameservers`.

## Algorithm And Decision Flow
1. Emit `TEST_CASE_START`.
2. Get delegation NS names (`Method2`), sort names, and emit one of:
   - `ENOUGH_NS_DEL` when count is at least the minimum.
   - `NOT_ENOUGH_NS_DEL` otherwise.
3. Get child NS names (`Method3`), sort names, and emit one of:
   - `ENOUGH_NS_CHILD` when count is at least the minimum.
   - `NOT_ENOUGH_NS_CHILD` otherwise.
4. Get child addressed NS (`Method5`), split by IP family, count unique NS names per family, and emit per family:
   - IPv4: `ENOUGH_IPV4_NS_CHILD`, `NOT_ENOUGH_IPV4_NS_CHILD`, or `NO_IPV4_NS_CHILD`.
   - IPv6: `ENOUGH_IPV6_NS_CHILD`, `NOT_ENOUGH_IPV6_NS_CHILD`, or `NO_IPV6_NS_CHILD`.
5. Get delegation addressed NS (`Method4`), split by IP family, count unique NS names per family, and emit per family:
   - IPv4: `ENOUGH_IPV4_NS_DEL`, `NOT_ENOUGH_IPV4_NS_DEL`, or `NO_IPV4_NS_DEL`.
   - IPv6: `ENOUGH_IPV6_NS_DEL`, `NOT_ENOUGH_IPV6_NS_DEL`, or `NO_IPV6_NS_DEL`.
6. Emit `TEST_CASE_END`.

## Emitted Tags (Possible Set)
| Tag | Emitted when |
| --- | --- |
| `ENOUGH_IPV4_NS_CHILD` | Child side has at least minimum number of NS names with IPv4 addresses. |
| `ENOUGH_IPV4_NS_DEL` | Delegation side has at least minimum number of NS names with IPv4 addresses. |
| `ENOUGH_IPV6_NS_CHILD` | Child side has at least minimum number of NS names with IPv6 addresses. |
| `ENOUGH_IPV6_NS_DEL` | Delegation side has at least minimum number of NS names with IPv6 addresses. |
| `ENOUGH_NS_CHILD` | Child NS name count is at least minimum. |
| `ENOUGH_NS_DEL` | Delegation NS name count is at least minimum. |
| `NOT_ENOUGH_IPV4_NS_CHILD` | Child side has IPv4-addressed NS names, but fewer than minimum. |
| `NOT_ENOUGH_IPV4_NS_DEL` | Delegation side has IPv4-addressed NS names, but fewer than minimum. |
| `NOT_ENOUGH_IPV6_NS_CHILD` | Child side has IPv6-addressed NS names, but fewer than minimum. |
| `NOT_ENOUGH_IPV6_NS_DEL` | Delegation side has IPv6-addressed NS names, but fewer than minimum. |
| `NOT_ENOUGH_NS_CHILD` | Child NS name count is fewer than minimum. |
| `NOT_ENOUGH_NS_DEL` | Delegation NS name count is fewer than minimum. |
| `NO_IPV4_NS_CHILD` | Child side has zero NS names with IPv4 addresses. |
| `NO_IPV4_NS_DEL` | Delegation side has zero NS names with IPv4 addresses. |
| `NO_IPV6_NS_CHILD` | Child side has zero NS names with IPv6 addresses. |
| `NO_IPV6_NS_DEL` | Delegation side has zero NS names with IPv6 addresses. |
| `TEST_CASE_END` | Testcase completion marker is emitted. |
| `TEST_CASE_START` | Testcase start marker is emitted. |

## Tag Arguments
| Tag | Argument key | Type | Meaning |
| --- | --- | --- | --- |
| `ENOUGH_IPV4_NS_CHILD` | `count` | `int` | Number of unique child NS names with IPv4 addresses. |
| `ENOUGH_IPV4_NS_CHILD` | `minimum` | `int` | Required minimum NS count. |
| `ENOUGH_IPV4_NS_CHILD` | `servers` | `array<object>` | Structured child `{ns,address}` object items considered for IPv4. |
| `ENOUGH_IPV4_NS_DEL` | `count` | `int` | Number of unique delegation NS names with IPv4 addresses. |
| `ENOUGH_IPV4_NS_DEL` | `minimum` | `int` | Required minimum NS count. |
| `ENOUGH_IPV4_NS_DEL` | `servers` | `array<object>` | Structured delegation `{ns,address}` object items considered for IPv4. |
| `ENOUGH_IPV6_NS_CHILD` | `count` | `int` | Number of unique child NS names with IPv6 addresses. |
| `ENOUGH_IPV6_NS_CHILD` | `minimum` | `int` | Required minimum NS count. |
| `ENOUGH_IPV6_NS_CHILD` | `servers` | `array<object>` | Structured child `{ns,address}` object items considered for IPv6. |
| `ENOUGH_IPV6_NS_DEL` | `count` | `int` | Number of unique delegation NS names with IPv6 addresses. |
| `ENOUGH_IPV6_NS_DEL` | `minimum` | `int` | Required minimum NS count. |
| `ENOUGH_IPV6_NS_DEL` | `servers` | `array<object>` | Structured delegation `{ns,address}` object items considered for IPv6. |
| `ENOUGH_NS_CHILD` | `count` | `int` | Number of child NS names. |
| `ENOUGH_NS_CHILD` | `minimum` | `int` | Required minimum NS count. |
| `ENOUGH_NS_CHILD` | `servers` | `array<object>` | Structured child nameserver names as `{ns}` items. |
| `ENOUGH_NS_DEL` | `count` | `int` | Number of delegation NS names. |
| `ENOUGH_NS_DEL` | `minimum` | `int` | Required minimum NS count. |
| `ENOUGH_NS_DEL` | `servers` | `array<object>` | Structured delegation nameserver names as `{ns}` items. |
| `NOT_ENOUGH_IPV4_NS_CHILD` | `count` | `int` | Number of unique child NS names with IPv4 addresses. |
| `NOT_ENOUGH_IPV4_NS_CHILD` | `minimum` | `int` | Required minimum NS count. |
| `NOT_ENOUGH_IPV4_NS_CHILD` | `servers` | `array<object>` | Structured child `{ns,address}` object items considered for IPv4. |
| `NOT_ENOUGH_IPV4_NS_DEL` | `count` | `int` | Number of unique delegation NS names with IPv4 addresses. |
| `NOT_ENOUGH_IPV4_NS_DEL` | `minimum` | `int` | Required minimum NS count. |
| `NOT_ENOUGH_IPV4_NS_DEL` | `servers` | `array<object>` | Structured delegation `{ns,address}` object items considered for IPv4. |
| `NOT_ENOUGH_IPV6_NS_CHILD` | `count` | `int` | Number of unique child NS names with IPv6 addresses. |
| `NOT_ENOUGH_IPV6_NS_CHILD` | `minimum` | `int` | Required minimum NS count. |
| `NOT_ENOUGH_IPV6_NS_CHILD` | `servers` | `array<object>` | Structured child `{ns,address}` object items considered for IPv6. |
| `NOT_ENOUGH_IPV6_NS_DEL` | `count` | `int` | Number of unique delegation NS names with IPv6 addresses. |
| `NOT_ENOUGH_IPV6_NS_DEL` | `minimum` | `int` | Required minimum NS count. |
| `NOT_ENOUGH_IPV6_NS_DEL` | `servers` | `array<object>` | Structured delegation `{ns,address}` object items considered for IPv6. |
| `NOT_ENOUGH_NS_CHILD` | `count` | `int` | Number of child NS names. |
| `NOT_ENOUGH_NS_CHILD` | `minimum` | `int` | Required minimum NS count. |
| `NOT_ENOUGH_NS_CHILD` | `servers` | `array<object>` | Structured child nameserver names as `{ns}` items. |
| `NOT_ENOUGH_NS_DEL` | `count` | `int` | Number of delegation NS names. |
| `NOT_ENOUGH_NS_DEL` | `minimum` | `int` | Required minimum NS count. |
| `NOT_ENOUGH_NS_DEL` | `servers` | `array<object>` | Structured delegation nameserver names as `{ns}` items. |
| `NO_IPV4_NS_CHILD` | `count` | `int` | Number of unique child NS names with IPv4 addresses (`0`). |
| `NO_IPV4_NS_CHILD` | `minimum` | `int` | Required minimum NS count. |
| `NO_IPV4_NS_CHILD` | `servers` | `array<object>` | Structured child `{ns,address}` object items considered for IPv4. |
| `NO_IPV4_NS_DEL` | `count` | `int` | Number of unique delegation NS names with IPv4 addresses (`0`). |
| `NO_IPV4_NS_DEL` | `minimum` | `int` | Required minimum NS count. |
| `NO_IPV4_NS_DEL` | `servers` | `array<object>` | Structured delegation `{ns,address}` object items considered for IPv4. |
| `NO_IPV6_NS_CHILD` | `count` | `int` | Number of unique child NS names with IPv6 addresses (`0`). |
| `NO_IPV6_NS_CHILD` | `minimum` | `int` | Required minimum NS count. |
| `NO_IPV6_NS_CHILD` | `servers` | `array<object>` | Structured child `{ns,address}` object items considered for IPv6. |
| `NO_IPV6_NS_DEL` | `count` | `int` | Number of unique delegation NS names with IPv6 addresses (`0`). |
| `NO_IPV6_NS_DEL` | `minimum` | `int` | Required minimum NS count. |
| `NO_IPV6_NS_DEL` | `servers` | `array<object>` | Structured delegation `{ns,address}` object items considered for IPv6. |
| `TEST_CASE_END` | `testcase` | `string` | Testcase display name (`Delegation01`). |
| `TEST_CASE_START` | `testcase` | `string` | Testcase display name (`Delegation01`). |

## Severity Levels Per Tag
| Tag | Level | Notes |
| --- | --- | --- |
| `ENOUGH_IPV4_NS_CHILD` | `INFO` | Default from `share/profile.json` (`test_levels.DELEGATION`). |
| `ENOUGH_IPV4_NS_DEL` | `INFO` | Default from `share/profile.json` (`test_levels.DELEGATION`). |
| `ENOUGH_IPV6_NS_CHILD` | `INFO` | Default from `share/profile.json` (`test_levels.DELEGATION`). |
| `ENOUGH_IPV6_NS_DEL` | `INFO` | Default from `share/profile.json` (`test_levels.DELEGATION`). |
| `ENOUGH_NS_CHILD` | `INFO` | Default from `share/profile.json` (`test_levels.DELEGATION`). |
| `ENOUGH_NS_DEL` | `INFO` | Default from `share/profile.json` (`test_levels.DELEGATION`). |
| `NOT_ENOUGH_IPV4_NS_CHILD` | `ERROR` | Default from `share/profile.json` (`test_levels.DELEGATION`). |
| `NOT_ENOUGH_IPV4_NS_DEL` | `ERROR` | Default from `share/profile.json` (`test_levels.DELEGATION`). |
| `NOT_ENOUGH_IPV6_NS_CHILD` | `ERROR` | Default from `share/profile.json` (`test_levels.DELEGATION`). |
| `NOT_ENOUGH_IPV6_NS_DEL` | `ERROR` | Default from `share/profile.json` (`test_levels.DELEGATION`). |
| `NOT_ENOUGH_NS_CHILD` | `ERROR` | Default from `share/profile.json` (`test_levels.DELEGATION`). |
| `NOT_ENOUGH_NS_DEL` | `ERROR` | Default from `share/profile.json` (`test_levels.DELEGATION`). |
| `NO_IPV4_NS_CHILD` | `WARNING` | Default from `share/profile.json` (`test_levels.DELEGATION`). |
| `NO_IPV4_NS_DEL` | `WARNING` | Default from `share/profile.json` (`test_levels.DELEGATION`). |
| `NO_IPV6_NS_CHILD` | `NOTICE` | Default from `share/profile.json` (`test_levels.DELEGATION`). |
| `NO_IPV6_NS_DEL` | `NOTICE` | Default from `share/profile.json` (`test_levels.DELEGATION`). |
| `TEST_CASE_END` | `DEBUG` | Default from `share/profile.json` (`test_levels.DELEGATION`). |
| `TEST_CASE_START` | `DEBUG` | Default from `share/profile.json` (`test_levels.DELEGATION`). |

## Differences From Upstream
- Upstream reference: [`delegation01.md`](../../upstream/tests/Delegation-TP/delegation01.md)
- Differences (Upstream vs Gonemaster):
  - Upstream: describes delegation-side IPv4/IPv6 checks before child-side IPv4/IPv6 checks. Gonemaster: performs child-side family checks first, then delegation-side family checks.
  - Upstream: does not describe testcase boundary debug markers. Gonemaster: emits `TEST_CASE_START` and `TEST_CASE_END`.
- Potential upstream report:
  - `no`

## Edge Cases And Limitations
- Per-family counts are by unique NS name, not by count of IP addresses.
- A single NS name can contribute to both IPv4 and IPv6 counts when it has both address families.
- Any method error aborts testcase execution and can prevent later tags from being emitted.
