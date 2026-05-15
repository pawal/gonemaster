# Zone13

Status: Draft

## Purpose
- Validate that the SPF policy at the zone apex does not exceed the DNS lookup limit defined in RFC 7208 Section 4.6.4.
- Detect use of the deprecated `ptr` mechanism (RFC 7208 Section 5.5).

## Preconditions And Inputs
- Preconditions:
  - A `zone.Zone` object is available.
  - Zone11 has been run and emitted `Z11_SPF_SYNTAX_OK`, confirming a valid SPF record exists.
- Required inputs:
  - The SPF TXT record at the zone apex.
  - Live DNS resolution for following `include`/`redirect` chains.
- Profile/config knobs that affect behavior:
  - `test_cases_vars.zone13.spf_lookup_limit`: maximum allowed DNS-resolving mechanisms (default: `10` per RFC 7208).
  - `net.ipv4` and `net.ipv6`: disabled transports are skipped.
  - `resolver.defaults.parallel`: parallel nameserver query fanout.

## Algorithm And Decision Flow
1. Emit `TEST_CASE_START`.
2. Check prior results for `Z11_SPF_SYNTAX_OK`. If absent, emit `Z13_NO_SPF_FOUND` and stop.
3. Retrieve the SPF TXT record from the zone apex (query authoritative nameserver).
   - If no authoritative response is available, emit `Z13_UNABLE_TO_CHECK` and stop.
4. Parse the SPF record into terms.
5. Walk the terms recursively, maintaining a visited-domain set for loop detection:
   - For each term, classify by mechanism type:
     - `include:domain`: count +1, recursively fetch and walk the target domain's SPF record.
     - `redirect=domain`: count +1, recursively fetch and walk the target domain's SPF record.
     - `a`, `a:domain`, `a:domain/cidr`: count +1.
     - `mx`, `mx:domain`, `mx:domain/cidr`: count +1.
     - `ptr`, `ptr:domain`: count +1, also flag as deprecated.
     - `exists:domain`: count +1.
     - `all`, `ip4:...`, `ip6:...`: do not count (no DNS lookup).
     - `exp=domain`: do not count toward the mechanism limit.
   - If a domain has already been visited during recursion, emit `Z13_SPF_LOOKUP_LOOP` and stop recursing that branch.
   - If an `include`/`redirect` target cannot be resolved, emit `Z13_SPF_RECURSIVE_ERROR` and stop recursing that branch.
   - If a `ptr` or `ptr:domain` mechanism is encountered, emit `Z13_SPF_PTR_DEPRECATED`.
6. Compare the total count against the configured `spf_lookup_limit`:
   - If count <= limit, emit `Z13_SPF_LOOKUP_COUNT_OK`.
   - If count > limit, emit `Z13_SPF_LOOKUP_COUNT_EXCEEDED`.
7. Emit `TEST_CASE_END`.

## Emitted Tags (Possible Set)
| Tag | Emitted when |
| --- | --- |
| `Z13_NO_SPF_FOUND` | Zone11 did not emit `Z11_SPF_SYNTAX_OK`; no SPF record to evaluate. |
| `Z13_SPF_LOOKUP_COUNT_EXCEEDED` | Total DNS-resolving mechanism count exceeds the configured limit. |
| `Z13_SPF_LOOKUP_COUNT_OK` | Total DNS-resolving mechanism count is within the configured limit. |
| `Z13_SPF_LOOKUP_LOOP` | Recursive `include`/`redirect` chain revisits a previously seen domain. |
| `Z13_SPF_PTR_DEPRECATED` | SPF record uses the deprecated `ptr` mechanism (RFC 7208 Section 5.5). |
| `Z13_SPF_RECURSIVE_ERROR` | An `include`/`redirect` target could not be resolved via DNS. |
| `Z13_UNABLE_TO_CHECK` | No authoritative TXT response could be obtained for the zone apex. |
| `TEST_CASE_END` | Testcase completion marker is emitted. |
| `TEST_CASE_START` | Testcase start marker is emitted. |

## Tag Arguments
| Tag | Argument key | Type | Meaning |
| --- | --- | --- | --- |
| `Z13_NO_SPF_FOUND` | `domain` | `string` | Tested zone name. |
| `Z13_SPF_LOOKUP_COUNT_EXCEEDED` | `domain` | `string` | Tested zone name. |
| `Z13_SPF_LOOKUP_COUNT_EXCEEDED` | `count` | `int` | Total number of DNS-resolving mechanisms found. |
| `Z13_SPF_LOOKUP_COUNT_EXCEEDED` | `limit` | `int` | Configured lookup limit. |
| `Z13_SPF_LOOKUP_COUNT_OK` | `domain` | `string` | Tested zone name. |
| `Z13_SPF_LOOKUP_COUNT_OK` | `count` | `int` | Total number of DNS-resolving mechanisms found. |
| `Z13_SPF_LOOKUP_LOOP` | `domain` | `string` | Tested zone name. |
| `Z13_SPF_LOOKUP_LOOP` | `loop_domain` | `string` | The domain that was visited a second time. |
| `Z13_SPF_PTR_DEPRECATED` | `domain` | `string` | Tested zone name. |
| `Z13_SPF_RECURSIVE_ERROR` | `domain` | `string` | Tested zone name. |
| `Z13_SPF_RECURSIVE_ERROR` | `target` | `string` | The `include`/`redirect` target domain that could not be resolved. |
| `Z13_UNABLE_TO_CHECK` | `-` | `-` | No arguments. |
| `TEST_CASE_END` | `testcase` | `string` | Testcase display name (`Zone13`). |
| `TEST_CASE_START` | `testcase` | `string` | Testcase display name (`Zone13`). |

## Severity Levels Per Tag
| Tag | Level | Notes |
| --- | --- | --- |
| `Z13_NO_SPF_FOUND` | `INFO` | Default from `share/profile.json` (`test_levels.ZONE`). |
| `Z13_SPF_LOOKUP_COUNT_EXCEEDED` | `WARNING` | Default from `share/profile.json` (`test_levels.ZONE`). |
| `Z13_SPF_LOOKUP_COUNT_OK` | `INFO` | Default from `share/profile.json` (`test_levels.ZONE`). |
| `Z13_SPF_LOOKUP_LOOP` | `WARNING` | Default from `share/profile.json` (`test_levels.ZONE`). |
| `Z13_SPF_PTR_DEPRECATED` | `WARNING` | Default from `share/profile.json` (`test_levels.ZONE`). |
| `Z13_SPF_RECURSIVE_ERROR` | `NOTICE` | Default from `share/profile.json` (`test_levels.ZONE`). |
| `Z13_UNABLE_TO_CHECK` | `WARNING` | Default from `share/profile.json` (`test_levels.ZONE`). |
| `TEST_CASE_END` | `DEBUG` | Default from `share/profile.json` (`test_levels.ZONE`). |
| `TEST_CASE_START` | `DEBUG` | Default from `share/profile.json` (`test_levels.ZONE`). |

## Differences From Upstream
- No upstream (Zonemaster) equivalent exists; this is a gonemaster-specific test case.
- References: [RFC 7208 Section 4.6.4](https://datatracker.ietf.org/doc/html/rfc7208#section-4.6.4), [RFC 7208 Section 5.5](https://datatracker.ietf.org/doc/html/rfc7208#section-5.5)

## Edge Cases And Limitations
- Zone13 depends on Zone11 having run first. If Zone11 is disabled or did not find a valid SPF record, Zone13 emits `Z13_NO_SPF_FOUND` and stops.
- The test performs live DNS lookups to follow `include`/`redirect` chains. Results may vary depending on network conditions and the state of external DNS records.
- Loop detection prevents infinite recursion but the count up to the loop detection point is still included in the total.
- `exp=domain` modifiers are not counted toward the lookup limit per RFC 7208, as they are only evaluated during result explanation and do not affect SPF evaluation.
- The `ptr` mechanism is counted toward the lookup limit AND flagged as deprecated; both tags may be emitted for the same record.
- Qualified mechanisms (e.g., `+include:`, `-a`, `~mx`) are handled identically to their unqualified forms for counting purposes.
