# Address02 (address02)

Status: Draft

## Purpose
- Verify that every unique nameserver IP address has a usable reverse DNS PTR mapping.
- Distinguish between negative PTR results and total lack of PTR query response.

## Preconditions And Inputs
- Preconditions:
  - A `zone.Zone` object is available.
  - A recursor is available on the zone object.
- Required inputs:
  - Nameserver addresses from `methods.Method4` (delegation/glue view).
  - Nameserver addresses from `methods.Method5` (child/authoritative view).
- Profile/config knobs that affect behavior:
  - `resolver.defaults.parallel`: controls PTR query task parallelism.

## Algorithm And Decision Flow
1. Emit `TEST_CASE_START`.
2. Collect nameserver entries from `Method4` and `Method5`.
3. Build an ordered unique list by IP string:
   - Concatenate `Method4` then `Method5`.
   - Keep the first `(nsname, ip)` seen for each unique IP.
4. For each unique IP, execute a PTR-check task (parallelized):
   - Compute reverse lookup owner with `dns.ReverseAddr`.
   - Send recursive PTR query.
   - If response has `NOERROR` and a CNAME in answer, follow the first CNAME target with one additional PTR query.
   - If a response message exists:
     - If RCODE is not `NOERROR` or PTR answer set is empty, emit `NAMESERVER_IP_WITHOUT_REVERSE` (`nsname`, `ns_ip`).
   - If no response message exists, emit `NO_RESPONSE_PTR_QUERY` (`domain`).
5. After all tasks complete, if at least one IP was checked and no tag besides `TEST_CASE_START` was emitted, emit `NAMESERVERS_IP_WITH_REVERSE`.
6. Emit `TEST_CASE_END`.

## Emitted Tags (Possible Set)
| Tag | Emitted when |
| --- | --- |
| `NAMESERVER_IP_WITHOUT_REVERSE` | PTR response is present but not successful (`RCODE != NOERROR`) or has no PTR record. |
| `NAMESERVERS_IP_WITH_REVERSE` | All checked IPs have successful PTR answers and no PTR-query failure tag was emitted. |
| `NO_RESPONSE_PTR_QUERY` | PTR recursive query returned no response message. |
| `TEST_CASE_END` | Testcase completion marker is emitted. |
| `TEST_CASE_START` | Testcase start marker is emitted. |

## Tag Arguments
| Tag | Argument key | Type | Meaning |
| --- | --- | --- | --- |
| `NAMESERVER_IP_WITHOUT_REVERSE` | `nsname` | `string` | Nameserver name associated with the checked IP (first-seen for that IP). |
| `NAMESERVER_IP_WITHOUT_REVERSE` | `ns_ip` | `string` | Checked nameserver IP address. |
| `NAMESERVERS_IP_WITH_REVERSE` | `-` | `-` | No arguments. |
| `NO_RESPONSE_PTR_QUERY` | `domain` | `string` | PTR owner name queried (reverse name or followed CNAME target). |
| `TEST_CASE_END` | `testcase` | `string` | Testcase display name (`Address02`). |
| `TEST_CASE_START` | `testcase` | `string` | Testcase display name (`Address02`). |

## Severity Levels Per Tag
| Tag | Level | Notes |
| --- | --- | --- |
| `NAMESERVER_IP_WITHOUT_REVERSE` | `WARNING` | Default from `share/profile.json` (`test_levels.ADDRESS`). |
| `NAMESERVERS_IP_WITH_REVERSE` | `INFO` | Default from `share/profile.json` (`test_levels.ADDRESS`). |
| `NO_RESPONSE_PTR_QUERY` | `WARNING` | Default from `share/profile.json` (`test_levels.ADDRESS`). |
| `TEST_CASE_END` | `DEBUG` | Default from `share/profile.json` (`test_levels.ADDRESS`). |
| `TEST_CASE_START` | `DEBUG` | Default from `share/profile.json` (`test_levels.ADDRESS`). |

## Differences From Upstream
- Upstream reference: [`address02.md`](../../upstream/tests/Address-TP/address02.md)
- Differences (Upstream vs Gonemaster):
  - Upstream: describes overall success/failure semantics only. Gonemaster: emits explicit diagnostic tags for pass, fail, and no-response PTR paths.
  - Upstream: does not specify PTR CNAME follow-up behavior. Gonemaster: follows one PTR CNAME hop before final PTR evaluation.
- Potential upstream report:
  - `no`

## Edge Cases And Limitations
- If `Method4`+`Method5` yields no IP addresses, only `TEST_CASE_START` and `TEST_CASE_END` are emitted.
- Duplicate IPs are checked once; if multiple nameservers share an IP, the first-seen nameserver name is used in emitted arguments.
- Only one CNAME follow-up lookup is performed for PTR checks.
