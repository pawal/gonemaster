# Zone07

Status: Final

## Purpose
- Validate SOA MNAME alias/address behavior:
  - detect whether SOA MNAME behaves as a CNAME target;
  - detect whether SOA MNAME has at least one address (`A`/`AAAA`).

## Preconditions And Inputs
- Preconditions:
  - A `zone.Zone` object is available.
  - A recursor is available on the zone object.
- Required inputs:
  - Child nameserver addresses from [`ApexNameservers`](../../nameserver-resolution.md#apexnameservers) (via shared SOA retrieval helper).
  - Recursive `A` and `AAAA` lookups for SOA MNAME.
- Profile/config knobs that affect behavior:
  - `net.ipv4` and `net.ipv6` affect transport availability during SOA retrieval.

## Algorithm And Decision Flow
1. Emit `TEST_CASE_START`.
2. Retrieve SOA from child nameservers using shared helper logic:
   - iterate [`ApexNameservers`](../../nameserver-resolution.md#apexnameservers) nameservers in order;
   - skip disabled transports;
   - return the first response that has SOA in answer and `AA=true`.
3. If no qualifying SOA response is found, emit `NO_RESPONSE_SOA_QUERY`.
4. Else, take SOA MNAME (`soa.Ns`) without trailing dot and evaluate both query types `A` and `AAAA`:
   - recurse for MNAME and query type;
   - if recurse result is missing, skip this query type;
   - increment `addresses` counter when answer contains queried type for original MNAME or final question name;
   - emit `MNAME_IS_CNAME` if a CNAME RR exists for original MNAME or final question name differs from original MNAME;
   - otherwise emit `MNAME_IS_NOT_CNAME`.
5. After both query types, if `addresses == 0`, emit `MNAME_HAS_NO_ADDRESS`.
6. Emit `TEST_CASE_END`.

## Emitted Tags (Possible Set)
| Tag | Emitted when |
| --- | --- |
| `MNAME_HAS_NO_ADDRESS` | Neither `A` nor `AAAA` lookup produced an address answer for SOA MNAME. |
| `MNAME_IS_CNAME` | SOA MNAME lookup shows alias behavior (CNAME RR or rewritten final query name). |
| `MNAME_IS_NOT_CNAME` | SOA MNAME lookup did not show alias behavior for that query-type evaluation. |
| `NO_RESPONSE_SOA_QUERY` | No authoritative SOA response containing an SOA answer record was received from any queried nameserver. |
| `TEST_CASE_END` | Testcase completion marker is emitted. |
| `TEST_CASE_START` | Testcase start marker is emitted. |

## Tag Arguments
| Tag | Argument key | Type | Meaning |
| --- | --- | --- | --- |
| `MNAME_HAS_NO_ADDRESS` | `mname` | `string` | SOA MNAME hostname that had no address answers. |
| `MNAME_IS_CNAME` | `mname` | `string` | SOA MNAME hostname evaluated as alias-like. |
| `MNAME_IS_NOT_CNAME` | `mname` | `string` | SOA MNAME hostname evaluated as non-alias for that branch. |
| `NO_RESPONSE_SOA_QUERY` | `-` | `-` | No arguments. |
| `TEST_CASE_END` | `testcase` | `string` | Testcase display name (`Zone07`). |
| `TEST_CASE_START` | `testcase` | `string` | Testcase display name (`Zone07`). |

## Severity Levels Per Tag
| Tag | Level | Notes |
| --- | --- | --- |
| `MNAME_HAS_NO_ADDRESS` | `WARNING` | Default from `share/profile.json` (`test_levels.ZONE`). |
| `MNAME_IS_CNAME` | `NOTICE` | Default from `share/profile.json` (`test_levels.ZONE`). |
| `MNAME_IS_NOT_CNAME` | `INFO` | Default from `share/profile.json` (`test_levels.ZONE`). |
| `NO_RESPONSE_SOA_QUERY` | `DEBUG` | Default from `share/profile.json` (`test_levels.ZONE`). |
| `TEST_CASE_END` | `DEBUG` | Default from `share/profile.json` (`test_levels.ZONE`). |
| `TEST_CASE_START` | `DEBUG` | Default from `share/profile.json` (`test_levels.ZONE`). |

## Differences From Upstream
- Upstream reference: [`zone07.md`](../../upstream/tests/Zone-TP/zone07.md)
- Differences (Upstream vs Gonemaster):
  - Upstream: specification text in `zone07.md` includes unrelated SOA minimum/RIPE-203 wording. Gonemaster: implementation has no such check in `zone07`.
  - Upstream: procedural text focuses on CNAME-failure condition. Gonemaster: emits additional explicit outcome tags (`MNAME_IS_NOT_CNAME`, `MNAME_HAS_NO_ADDRESS`).
  - Upstream: states no intercase dependencies. Gonemaster: runtime behavior can be influenced by recursive cache interactions in known upstream engine behavior.
- Potential upstream report:
  - `yes`
- If yes, include:
  - Upstream expected behavior: Zone07 spec text and outcome framing should match actual engine behavior and recursive side effects.
  - Gonemaster observed behavior: Current behavior tracks upstream engine behavior and exposes additional emitted outcomes that are not fully reflected in upstream spec wording.
  - evidence: `docs/specifications/upstream/tests/Zone-TP/zone07.md`, `engine/test/zone/zone.go`, `docs/specifications/known-behavior-divergences.md`, [zonemaster#1467](https://github.com/zonemaster/zonemaster/issues/1467), [zonemaster-engine#1500](https://github.com/zonemaster/zonemaster-engine/issues/1500)
  - report status: `reported`

## Edge Cases And Limitations
- `MNAME_IS_CNAME` and `MNAME_IS_NOT_CNAME` are evaluated independently for each of the `A` and `AAAA` query types; both tags are emitted in the same testcase run when MNAME resolves to a CNAME for one address family but not the other.
- Address success counts by query type branch and does not require both families to resolve.
- The shared retrieval helper emits `IPV4_DISABLED` or `IPV6_DISABLED` when the corresponding transport is disabled; these tags are not declared in this testcase's `Metadata()` function.
