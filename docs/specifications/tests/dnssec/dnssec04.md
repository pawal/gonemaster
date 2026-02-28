# DNSSEC04 (dnssec04)

Status: Final

## Purpose
- Evaluate DNSKEY/SOA RRSIG validity windows and flag signatures that are expired, too short-lived, too long-lived, or otherwise within configured limits.

## Preconditions And Inputs
- Preconditions:
  - A `zone.Zone` object is available.
- Required inputs:
  - DNSKEY and SOA query responses from `zoneQueryOne` for child apex.
  - RRSIG records covering DNSKEY and SOA RRsets.
- Profile/config knobs that affect behavior:
  - `resolver.defaults.parallel`: controls sequential vs parallel DNSKEY/SOA query execution.
  - `test_cases_vars.dnssec04.REMAINING_SHORT`: remaining-validity short threshold (default 43200 seconds, 12h).
  - `test_cases_vars.dnssec04.REMAINING_LONG`: remaining-validity long threshold (default 15552000 seconds, 180d).
  - `test_cases_vars.dnssec04.DURATION_LONG`: signature-duration long threshold (default 15552000 seconds, 180d).

## Algorithm And Decision Flow
1. Emit `TEST_CASE_START`.
2. Query child apex `DNSKEY` and `SOA` with DNSSEC enabled:
   - If `resolver.defaults.parallel <= 1`, run sequentially (`DNSKEY` then `SOA`).
   - Else run both queries concurrently via ordered parallel execution.
3. If either query fails with an error, return error.
4. If either query returns no DNS message (`Msg == nil`), emit `TEST_CASE_END` and stop.
5. Collect `RRSIG` records from answer sections of both responses.
6. Load thresholds from profile (`REMAINING_SHORT`, `REMAINING_LONG`, `DURATION_LONG`) and compute current reference time from DNSKEY packet timestamp.
7. For each RRSIG:
   - Emit `RRSIG_EXPIRATION` with human-readable UTC expiration date, keytag, and covered RR type.
   - Compute remaining time (`expiration - now`) and emit one of:
     - `RRSIG_EXPIRED` if remaining is negative,
     - `REMAINING_SHORT` if remaining is below short threshold,
     - `REMAINING_LONG` if remaining exceeds long threshold.
   - Compute total signature duration (`expiration - inception`) and emit `DURATION_LONG` if above duration threshold.
   - If neither a remaining-time tag nor `DURATION_LONG` was emitted for this RRSIG, emit `DURATION_OK`.
8. Emit `TEST_CASE_END`.

## Emitted Tags (Possible Set)
| Tag | Emitted when |
| --- | --- |
| `DURATION_LONG` | Signature validity duration (`expiration - inception`) is greater than configured long-duration threshold. |
| `DURATION_OK` | For this RRSIG, no expiration/remaining-duration warning/error and no long-duration warning was emitted. |
| `REMAINING_LONG` | Remaining validity (`expiration - now`) is above configured long-remaining threshold. |
| `REMAINING_SHORT` | Remaining validity (`expiration - now`) is below configured short-remaining threshold and non-negative. |
| `RRSIG_EXPIRATION` | Per-RRSIG expiration detail is logged. |
| `RRSIG_EXPIRED` | Signature expiration is already in the past (`expiration < now`). |
| `TEST_CASE_END` | Testcase completion marker is emitted. |
| `TEST_CASE_START` | Testcase start marker is emitted. |

## Tag Arguments
| Tag | Argument key | Type | Meaning |
| --- | --- | --- | --- |
| `DURATION_LONG` | `duration` | `int` | Signature validity duration in seconds (`expiration - inception`). |
| `DURATION_LONG` | `keytag` | `int` | DNSKEY keytag from the RRSIG. |
| `DURATION_LONG` | `types` | `string` | Covered RR type mnemonic (`DNSKEY` or `SOA`). |
| `DURATION_OK` | `duration` | `int` | Signature validity duration in seconds (`expiration - inception`). |
| `DURATION_OK` | `keytag` | `int` | DNSKEY keytag from the RRSIG. |
| `DURATION_OK` | `types` | `string` | Covered RR type mnemonic (`DNSKEY` or `SOA`). |
| `REMAINING_LONG` | `duration` | `int` | Remaining validity in seconds (`expiration - now`). |
| `REMAINING_LONG` | `keytag` | `int` | DNSKEY keytag from the RRSIG. |
| `REMAINING_LONG` | `types` | `string` | Covered RR type mnemonic (`DNSKEY` or `SOA`). |
| `REMAINING_SHORT` | `duration` | `int` | Remaining validity in seconds (`expiration - now`). |
| `REMAINING_SHORT` | `keytag` | `int` | DNSKEY keytag from the RRSIG. |
| `REMAINING_SHORT` | `types` | `string` | Covered RR type mnemonic (`DNSKEY` or `SOA`). |
| `RRSIG_EXPIRATION` | `date` | `string` | UTC expiration timestamp formatted with `time.ANSIC`. |
| `RRSIG_EXPIRATION` | `keytag` | `int` | DNSKEY keytag from the RRSIG. |
| `RRSIG_EXPIRATION` | `types` | `string` | Covered RR type mnemonic (`DNSKEY` or `SOA`). |
| `RRSIG_EXPIRED` | `expiration` | `int` | Signature expiration epoch timestamp (seconds). |
| `RRSIG_EXPIRED` | `keytag` | `int` | DNSKEY keytag from the RRSIG. |
| `RRSIG_EXPIRED` | `types` | `string` | Covered RR type mnemonic (`DNSKEY` or `SOA`). |
| `TEST_CASE_END` | `testcase` | `string` | Testcase display name (`DNSSEC04`). |
| `TEST_CASE_START` | `testcase` | `string` | Testcase display name (`DNSSEC04`). |

## Severity Levels Per Tag
| Tag | Level | Notes |
| --- | --- | --- |
| `DURATION_LONG` | `WARNING` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `DURATION_OK` | `DEBUG` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `REMAINING_LONG` | `WARNING` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `REMAINING_SHORT` | `WARNING` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `RRSIG_EXPIRATION` | `INFO` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `RRSIG_EXPIRED` | `ERROR` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `TEST_CASE_END` | `DEBUG` | Default from `share/profile.json` (`test_levels.DNSSEC`). |
| `TEST_CASE_START` | `DEBUG` | Default from `share/profile.json` (`test_levels.DNSSEC`). |

## Differences From Upstream
- Upstream reference: [`dnssec04.md`](../../upstream/tests/DNSSEC-TP/dnssec04.md)
- Differences (Upstream vs Gonemaster):
  - Upstream: describes fixed lifetime policy points (12h and 180d). Gonemaster: uses profile-controlled thresholds via `test_cases_vars.dnssec04.*` (defaults match 12h/180d/180d).
  - Upstream: is outcome-oriented and does not enumerate this detailed tag model. Gonemaster: emits explicit diagnostics (`RRSIG_EXPIRATION`, `REMAINING_*`, `DURATION_*`, plus testcase boundary tags).
- Potential upstream report:
  - `no`

## Implementation Notes

The following behaviors are implementation choices, not mandated by RFC 4034/4035:

- **Reference time source**: RRSIG remaining-validity checks use the DNSKEY response packet timestamp as the reference "now", not wall-clock time.  RFC 4034 requires checking whether a signature is currently valid; the specific reference clock is unspecified.  Using the packet timestamp avoids race conditions when queries take time but introduces a subtle inconsistency: if the DNSKEY and SOA responses arrive at different moments, the DNSKEY packet time applies uniformly to all RRSIG records from both responses.  Contrast with `dnssec10`, which uses wall-clock time.
- **Parallel query execution**: When `resolver.defaults.parallel > 1`, DNSKEY and SOA queries run concurrently.  The protocol defines no query ordering requirement; sequential vs concurrent is an implementation choice controlled by `resolver.defaults.parallel`.
- **`DURATION_OK` gating**: `DURATION_OK` is emitted per RRSIG only when no remaining-time or duration-long tag was emitted for that same RRSIG.  This "no prior finding" gating pattern is not protocol-defined.

## Edge Cases And Limitations
- If either DNSKEY or SOA query returns no DNS message, testcase ends with only boundary tags and no RRSIG findings.
- `DURATION_OK` is emitted only when no remaining-time tag and no `DURATION_LONG` tag were emitted for the same RRSIG.
- All remaining-time checks use the DNSKEY response packet time as the reference "now".
