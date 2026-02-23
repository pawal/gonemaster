# Zone08 (zone08)

Status: Draft

## Purpose
- Validate that MX exchange hostnames are not aliases (CNAME).

## Preconditions And Inputs
- Preconditions:
  - A `zone.Zone` object is available.
- Required inputs:
  - Authoritative MX response for child zone apex (`queryAuth`).
  - Authoritative CNAME lookups for each MX exchange hostname (`queryAuth`).
- Profile/config knobs that affect behavior:
  - No testcase-local profile knob.

## Algorithm And Decision Flow
1. Emit `TEST_CASE_START`.
2. Query authoritative `MX` for zone apex.
3. If no response, emit `NO_RESPONSE_MX_QUERY`.
4. Else, for each MX RR in apex answer:
   - query authoritative `CNAME` for MX exchange hostname;
   - if CNAME query has no response, emit no tag for that exchange;
   - if CNAME answer exists, emit:
     - `MX_RECORD_IS_CNAME` when exchange has CNAME answer;
     - `MX_RECORD_IS_NOT_CNAME` otherwise.
5. Emit `TEST_CASE_END`.

## Emitted Tags (Possible Set)
| Tag | Emitted when |
| --- | --- |
| `MX_RECORD_IS_CNAME` | An MX exchange hostname resolves as CNAME in authoritative CNAME lookup. |
| `MX_RECORD_IS_NOT_CNAME` | An MX exchange hostname does not resolve as CNAME in authoritative CNAME lookup. |
| `NO_RESPONSE_MX_QUERY` | Apex MX query returned no response. |
| `TEST_CASE_END` | Testcase completion marker is emitted. |
| `TEST_CASE_START` | Testcase start marker is emitted. |

## Tag Arguments
| Tag | Argument key | Type | Meaning |
| --- | --- | --- | --- |
| `MX_RECORD_IS_CNAME` | `-` | `-` | No arguments. |
| `MX_RECORD_IS_NOT_CNAME` | `-` | `-` | No arguments. |
| `NO_RESPONSE_MX_QUERY` | `-` | `-` | No arguments. |
| `TEST_CASE_END` | `testcase` | `string` | Testcase display name (`Zone08`). |
| `TEST_CASE_START` | `testcase` | `string` | Testcase display name (`Zone08`). |

## Severity Levels Per Tag
| Tag | Level | Notes |
| --- | --- | --- |
| `MX_RECORD_IS_CNAME` | `ERROR` | Default from `share/profile.json` (`test_levels.ZONE`). |
| `MX_RECORD_IS_NOT_CNAME` | `INFO` | Default from `share/profile.json` (`test_levels.ZONE`). |
| `NO_RESPONSE_MX_QUERY` | `DEBUG` | Default from `share/profile.json` (`test_levels.ZONE`). |
| `TEST_CASE_END` | `DEBUG` | Default from `share/profile.json` (`test_levels.ZONE`). |
| `TEST_CASE_START` | `DEBUG` | Default from `share/profile.json` (`test_levels.ZONE`). |

## Differences From Upstream
- Upstream reference: [`zone08.md`](../../upstream/tests/Zone-TP/zone08.md)
- Differences (Upstream vs Gonemaster):
  - Upstream: describes a high-level authoritative MX/CNAME check. Gonemaster: performs explicit per-MX exchange CNAME probes and emits explicit positive/negative tags (`MX_RECORD_IS_CNAME` / `MX_RECORD_IS_NOT_CNAME`).
  - Upstream: does not describe testcase boundary debug markers. Gonemaster: emits `TEST_CASE_START` and `TEST_CASE_END`.
  - Upstream: does not describe explicit no-response MX tag. Gonemaster: emits `NO_RESPONSE_MX_QUERY`.
- Potential upstream report:
  - `no`

## Edge Cases And Limitations
- Multiple MX RRs can yield multiple CNAME verdict tags in one run.
- A missing CNAME-query response for an MX exchange yields no dedicated per-exchange tag.

---

Copyright (c) Patrik Wallström  
Copyright (c) The Swedish Internet Foundation (https://internetstiftelsen.se/en/)  
Copyright (c) AFNIC (https://www.afnic.fr/en/)  
All rights reserved.  

Copyright belongs to external contributor where applicable.  

Creative Commons Attribution 4.0 International License applies. See https://creativecommons.org/licenses/by/4.0/ for the license conditions.
