# Nameserver07

Status: Final

## Purpose
- Detect upward referrals (root NS records in authority section) returned by authoritative nameservers.

## Preconditions And Inputs
- Preconditions:
  - A `zone.Zone` object is available.
- Required inputs:
  - Nameserver addresses from `methods.Method4and5`.
  - NS query responses for qname `.`.
- Profile/config knobs that affect behavior:
  - `net.ipv4` and `net.ipv6`: disabled transports are skipped with transport debug tags.
  - `resolver.defaults.parallel`: parallel nameserver query fanout.

## Algorithm And Decision Flow
1. Emit `TEST_CASE_START`.
2. If tested zone name is `.`:
   - Emit `UPWARD_REFERRAL_IRRELEVANT`.
   - Emit `TEST_CASE_END` and return.
3. Read nameserver list from `Method4and5`, deduplicate by `name/ip`, preserving first-seen order.
4. For each deduplicated nameserver (parallelized, input-order merged logs):
   - If transport is disabled, emit `IPV4_DISABLED` or `IPV6_DISABLED` for rrtype `NS`, mark not included in summary, and skip.
   - Mark nameserver as included.
   - Query qname `.` rrtype `NS`.
   - If response exists and authority section contains `NS` records, record server as having upward referral.
5. After all tasks, emit a single consolidated `UPWARD_REFERRAL` with `servers` list (if any), or a single `NO_UPWARD_REFERRAL` with `servers` list (if at least one nameserver was included and none had errors).
6. Emit `TEST_CASE_END`.

## Emitted Tags (Possible Set)
| Tag | Emitted when |
| --- | --- |
| `IPV4_DISABLED` | IPv4 nameserver evaluation is skipped because IPv4 is disabled. |
| `IPV6_DISABLED` | IPv6 nameserver evaluation is skipped because IPv6 is disabled. |
| `NO_UPWARD_REFERRAL` | Included nameservers showed no upward-referral evidence. |
| `TEST_CASE_END` | Testcase completion marker is emitted. |
| `TEST_CASE_START` | Testcase start marker is emitted. |
| `UPWARD_REFERRAL` | Authority section in root NS query response contains `NS` records. |
| `UPWARD_REFERRAL_IRRELEVANT` | Tested zone is root (`.`), so upward-referral check is skipped. |

## Tag Arguments
| Tag | Argument key | Type | Meaning |
| --- | --- | --- | --- |
| `IPV4_DISABLED` | `ns` | `string` | Nameserver identity (`ns` name only; use `address` for IP) skipped on IPv4. |
| `IPV4_DISABLED` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `IPV4_DISABLED` | `rrtype` | `string` | rrtype skipped (`NS`). |
| `IPV6_DISABLED` | `ns` | `string` | Nameserver identity (`ns` name only; use `address` for IP) skipped on IPv6. |
| `IPV6_DISABLED` | `address` | `string` | Nameserver IP address for the same endpoint. |
| `IPV6_DISABLED` | `rrtype` | `string` | rrtype skipped (`NS`). |
| `NO_UPWARD_REFERRAL` | `servers` | `array<object>` | Structured sorted list of nameservers without upward referral (`{ns}`, `{address}` items). |
| `TEST_CASE_END` | `testcase` | `string` | Testcase display name (`Nameserver07`). |
| `TEST_CASE_START` | `testcase` | `string` | Testcase display name (`Nameserver07`). |
| `UPWARD_REFERRAL` | `servers` | `array<object>` | Structured sorted list of nameservers returning upward referral (`{ns}`, `{address}` items). |
| `UPWARD_REFERRAL_IRRELEVANT` | `-` | `-` | No arguments. |

## Severity Levels Per Tag
| Tag | Level | Notes |
| --- | --- | --- |
| `IPV4_DISABLED` | `DEBUG` | Default from `share/profile.json` (`test_levels.NAMESERVER`). |
| `IPV6_DISABLED` | `DEBUG` | Default from `share/profile.json` (`test_levels.NAMESERVER`). |
| `NO_UPWARD_REFERRAL` | `INFO` | Default from `share/profile.json` (`test_levels.NAMESERVER`). |
| `TEST_CASE_END` | `DEBUG` | Default from `share/profile.json` (`test_levels.NAMESERVER`). |
| `TEST_CASE_START` | `DEBUG` | Default from `share/profile.json` (`test_levels.NAMESERVER`). |
| `UPWARD_REFERRAL` | `WARNING` | Default from `share/profile.json` (`test_levels.NAMESERVER`). |
| `UPWARD_REFERRAL_IRRELEVANT` | `INFO` | Default from `share/profile.json` (`test_levels.NAMESERVER`). |

## Differences From Upstream
- Upstream reference: [`nameserver07.md`](../../upstream/tests/Nameserver-TP/nameserver07.md)
- Differences (Upstream vs Gonemaster):
  - Upstream: says to exit when tested zone is root. Gonemaster: emits explicit `UPWARD_REFERRAL_IRRELEVANT` before ending testcase.
  - Upstream: describes evaluation over nameserver IP set. Gonemaster: deduplicates by `name/ip` and emits positive summary tag `NO_UPWARD_REFERRAL` when included nameservers have no findings.
  - Upstream: does not explicitly describe testcase boundary and transport-disabled debug emissions. Gonemaster: emits `TEST_CASE_START`, `TEST_CASE_END`, `IPV4_DISABLED`, and `IPV6_DISABLED`.
- Potential upstream report:
  - `no`

## Edge Cases And Limitations
- Query failures or missing responses do not produce dedicated failure tags in this testcase.
- If all nameservers are skipped (disabled transport), `NO_UPWARD_REFERRAL` is not emitted.
- Both `UPWARD_REFERRAL` and `NO_UPWARD_REFERRAL` now use consolidated `servers` lists with `{ns}` and `{address}` items.
