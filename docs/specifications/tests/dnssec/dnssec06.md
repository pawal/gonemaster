# DNSSEC06

Status: Final

## Purpose
- Verify DNSSEC additional-processing behavior for DNSKEY responses by checking whether DNSKEY answers include accompanying RRSIG data.

## Preconditions And Inputs
- Preconditions:
  - A `zone.Zone` object is available.
- Required inputs:
  - All DNSKEY responses returned by `zoneQueryAll` for child apex with DNSSEC enabled.
- Profile/config knobs that affect behavior:
  - No testcase-local profile thresholds are read.
  - Effective behavior depends on the resolver/query behavior used by `zoneQueryAll`.

## Algorithm And Decision Flow
1. Emit `TEST_CASE_START`.
2. Query all child nameservers for apex `DNSKEY` with DNSSEC enabled via `zoneQueryAll`.
3. For each response packet:
   - If response message is absent, skip packet with no DS06 finding.
   - Count DNSKEY records in answer and RRSIG records in answer.
   - If both counts are non-zero, emit `EXTRA_PROCESSING_OK` with `address`, `keys`, and `sigs`.
   - Else if `RCODE` is `NOERROR`, emit `EXTRA_PROCESSING_BROKEN` with `address`, `keys`, and `sigs`.
4. Emit `TEST_CASE_END`.

## Emitted Tags (Possible Set)
| Tag | Emitted when |
| --- | --- |
| `EXTRA_PROCESSING_BROKEN` | Response is `NOERROR` but DNSKEY answer does not include both DNSKEY and RRSIG records. |
| `EXTRA_PROCESSING_OK` | DNSKEY answer includes at least one DNSKEY and at least one RRSIG record. |
| `TEST_CASE_END` | Testcase completion marker is emitted. |
| `TEST_CASE_START` | Testcase start marker is emitted. |

## Tag Arguments
| Tag | Argument key | Type | Meaning |
| --- | --- | --- | --- |
| `EXTRA_PROCESSING_BROKEN` | `address` | `string` | Server/source identity from `packet.AnswerFromString()`. |
| `EXTRA_PROCESSING_BROKEN` | `keys` | `int` | Number of DNSKEY records found in answer. |
| `EXTRA_PROCESSING_BROKEN` | `sigs` | `int` | Number of RRSIG records found in answer. |
| `EXTRA_PROCESSING_OK` | `address` | `string` | Server/source identity from `packet.AnswerFromString()`. |
| `EXTRA_PROCESSING_OK` | `keys` | `int` | Number of DNSKEY records found in answer. |
| `EXTRA_PROCESSING_OK` | `sigs` | `int` | Number of RRSIG records found in answer. |
| `TEST_CASE_END` | `testcase` | `string` | Testcase display name (`DNSSEC06`). |
| `TEST_CASE_START` | `testcase` | `string` | Testcase display name (`DNSSEC06`). |

## Severity Levels Per Tag
| Tag | Level | Notes |
| --- | --- | --- |
| `EXTRA_PROCESSING_BROKEN` | `ERROR` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `EXTRA_PROCESSING_OK` | `INFO` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `TEST_CASE_END` | `DEBUG` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `TEST_CASE_START` | `DEBUG` | Default from `share/profile.json` (`test_levels.DNSSEC`). |

## Differences From Upstream
- Differences (Upstream vs Gonemaster):
  - Upstream: describes testcase outcome in pass/fail terms without a concrete message-tag model. Gonemaster: emits explicit per-response tags (`EXTRA_PROCESSING_OK` and `EXTRA_PROCESSING_BROKEN`) with record counters.
  - Upstream: states this testcase should run only after successful `DNSSEC07` signing detection. Gonemaster: `DNSSEC06` function itself has no local gate; ordering/gating is enforced by module runner (`All`) which runs `DNSSEC07` first and short-circuits on `DS07_NOT_SIGNED`.
- Potential upstream report:
  - `no`

## Edge Cases And Limitations
- Responses with missing message (`Msg == nil`) do not produce DS06 tags.
- Responses with non-`NOERROR` RCODE do not produce DS06 tags, even if DNSKEY/RRSIG are absent.
- This testcase does not validate signature cryptography; it only checks presence/absence of DNSKEY and RRSIG records.
