# Address03 (address03)

Status: Final

## Purpose
- Verify that reverse PTR hostnames for nameserver IPs match the corresponding nameserver hostname.
- Flag mismatch, missing reverse data, and no-response cases separately.

## Preconditions And Inputs
- Preconditions:
  - A `zone.Zone` object is available.
  - A recursor is available on the zone object.
- Required inputs:
  - Nameserver addresses from `methods.Method5`.
  - PTR lookup responses for each checked nameserver IP.
- Profile/config knobs that affect behavior:
  - `resolver.defaults.parallel`: controls PTR query task parallelism.

## Algorithm And Decision Flow
1. Emit `TEST_CASE_START`.
2. Collect nameserver entries with `Method5`.
3. Build an ordered unique list by IP string:
   - Keep the first `(nsname, ip)` seen for each unique IP.
4. For each unique IP, execute a PTR-check task (parallelized):
   - Compute reverse lookup owner with `dns.ReverseAddr`.
   - Send recursive PTR query.
   - If response message exists and `RCODE == NOERROR` with at least one PTR answer:
     - Collect PTR target names.
     - Compare targets against expected nameserver name (`nsname`) case-insensitively.
     - If none match, emit `NAMESERVER_IP_PTR_MISMATCH` (`nsname`, `ns_ip`, `names`).
   - Else if response message exists but PTR conditions above are not met, emit `NAMESERVER_IP_WITHOUT_REVERSE` (`nsname`, `ns_ip`).
   - Else emit `NO_RESPONSE_PTR_QUERY` (`domain`).
5. After all tasks complete, if at least one IP was checked and no tag besides `TEST_CASE_START` was emitted, emit `NAMESERVER_IP_PTR_MATCH`.
6. Emit `TEST_CASE_END`.
7. When executed through `AddressAll`, this testcase runs only if `Address02` produced `NAMESERVERS_IP_WITH_REVERSE`.

## Emitted Tags (Possible Set)
| Tag | Emitted when |
| --- | --- |
| `NAMESERVER_IP_PTR_MATCH` | All checked IPs returned PTR answers that include their expected nameserver name. |
| `NAMESERVER_IP_PTR_MISMATCH` | PTR answers exist for an IP, but none matches the expected nameserver name. |
| `NAMESERVER_IP_WITHOUT_REVERSE` | PTR response is present but not successful (`RCODE != NOERROR`) or has no PTR record. |
| `NO_RESPONSE_PTR_QUERY` | PTR recursive query returned no response message. |
| `TEST_CASE_END` | Testcase completion marker is emitted. |
| `TEST_CASE_START` | Testcase start marker is emitted. |

## Tag Arguments
| Tag | Argument key | Type | Meaning |
| --- | --- | --- | --- |
| `NAMESERVER_IP_PTR_MATCH` | `-` | `-` | No arguments. |
| `NAMESERVER_IP_PTR_MISMATCH` | `nsname` | `string` | Expected nameserver name for the checked IP (first-seen for that IP). |
| `NAMESERVER_IP_PTR_MISMATCH` | `ns_ip` | `string` | Checked nameserver IP address. |
| `NAMESERVER_IP_PTR_MISMATCH` | `names` | `string` | Slash-delimited PTR target names returned in answer. |
| `NAMESERVER_IP_WITHOUT_REVERSE` | `nsname` | `string` | Nameserver name associated with the checked IP. |
| `NAMESERVER_IP_WITHOUT_REVERSE` | `ns_ip` | `string` | Checked nameserver IP address. |
| `NO_RESPONSE_PTR_QUERY` | `domain` | `string` | PTR owner name queried. |
| `TEST_CASE_END` | `testcase` | `string` | Testcase display name (`Address03`). |
| `TEST_CASE_START` | `testcase` | `string` | Testcase display name (`Address03`). |

## Severity Levels Per Tag
| Tag | Level | Notes |
| --- | --- | --- |
| `NAMESERVER_IP_PTR_MATCH` | `INFO` | Default from `share/profile.json` (`test_levels.ADDRESS`). |
| `NAMESERVER_IP_PTR_MISMATCH` | `NOTICE` | Default from `share/profile.json` (`test_levels.ADDRESS`). |
| `NAMESERVER_IP_WITHOUT_REVERSE` | `WARNING` | Default from `share/profile.json` (`test_levels.ADDRESS`). |
| `NO_RESPONSE_PTR_QUERY` | `WARNING` | Default from `share/profile.json` (`test_levels.ADDRESS`). |
| `TEST_CASE_END` | `DEBUG` | Default from `share/profile.json` (`test_levels.ADDRESS`). |
| `TEST_CASE_START` | `DEBUG` | Default from `share/profile.json` (`test_levels.ADDRESS`). |

## Differences From Upstream
- Upstream reference: [`address03.md`](../../upstream/tests/Address-TP/address03.md)
- Differences (Upstream vs Gonemaster):
  - Upstream: describes consuming `ADDRESS02` outcome data as input. Gonemaster: performs fresh PTR lookups inside `Address03`.
  - Upstream: states `ADDRESS03` depends on `ADDRESS02` success. Gonemaster: enforces gating in `AddressAll` by requiring `NAMESERVERS_IP_WITH_REVERSE` from `Address02`.
- Potential upstream report:
  - `no`

## Implementation Notes

The following behaviors are implementation choices, not mandated by protocol:

- **Module orchestration gating**: `Address03` runs only when `Address02` emitted `NAMESERVERS_IP_WITH_REVERSE`.  No DNS standard mandates this sequencing; it is a gonemaster-specific orchestration decision to skip PTR-match checks when no reverse data was found in the preceding testcase.
- **First-seen-wins deduplication**: When multiple nameserver entries share the same IP, only the first-seen `(nsname, ip)` pair is retained for PTR checking.  The protocol does not define how to handle nameserver IP collisions; first-seen is an implementation choice.
- **PTR name list delimiter**: Multiple PTR target names in the `names` argument of `NAMESERVER_IP_PTR_MISMATCH` are joined with `/` (slash).  This delimiter is an internal formatting choice with no protocol counterpart.

## Edge Cases And Limitations
- If `Method5` yields no IP addresses, only `TEST_CASE_START` and `TEST_CASE_END` are emitted.
- Duplicate IPs are checked once; if multiple nameservers share an IP, only the first-seen nameserver name is evaluated for PTR-name match.
- PTR target matching is case-insensitive and exact on normalized DNS name string.
