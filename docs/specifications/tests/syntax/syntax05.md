# Syntax05 (syntax05)

Status: Draft

## Purpose
- Detect misuse of `@` in SOA `RNAME` and ensure mailbox form is represented with dot-separated DNS notation.

## Preconditions And Inputs
- Preconditions:
  - A `zone.Zone` object is available.
- Required inputs:
  - A SOA response for the child zone apex, acquired via `z.QueryOne(..., "SOA", ...)`.
- Profile/config knobs that affect behavior:
  - `net.ipv4` and `net.ipv6` can affect which nameserver transport `QueryOne` can use.

## Algorithm And Decision Flow
1. Emit `TEST_CASE_START`.
2. Perform `z.QueryOne` for SOA at the zone apex.
3. If response contains SOA records in answer section:
   - Use the first SOA record encountered.
   - Read `Mbox` (`RNAME`) and de-escape `\.` only for `@`-presence check.
   - Emit `RNAME_MISUSED_AT_SIGN` if `@` is present.
   - Otherwise emit `RNAME_NO_AT_SIGN`.
   - Emit `TEST_CASE_END` and return.
4. If no SOA record is available from the query path, emit `NO_RESPONSE_SOA_QUERY`.
5. Emit `TEST_CASE_END`.

## Emitted Tags (Possible Set)
| Tag | Emitted when |
| --- | --- |
| `RNAME_MISUSED_AT_SIGN` | SOA `RNAME` contains `@`. |
| `RNAME_NO_AT_SIGN` | SOA `RNAME` has no `@`. |
| `NO_RESPONSE_SOA_QUERY` | No usable SOA answer is obtained. |
| `TEST_CASE_END` | Testcase completion marker is emitted. |
| `TEST_CASE_START` | Testcase start marker is emitted. |

## Tag Arguments
| Tag | Argument key | Type | Meaning |
| --- | --- | --- | --- |
| `RNAME_MISUSED_AT_SIGN` | `rname` | `string` | Raw SOA `RNAME` value. |
| `RNAME_NO_AT_SIGN` | `rname` | `string` | Raw SOA `RNAME` value. |
| `NO_RESPONSE_SOA_QUERY` | `-` | `-` | No arguments. |
| `TEST_CASE_END` | `testcase` | `string` | Testcase display name (`Syntax05`). |
| `TEST_CASE_START` | `testcase` | `string` | Testcase display name (`Syntax05`). |

## Severity Levels Per Tag
| Tag | Level | Notes |
| --- | --- | --- |
| `RNAME_MISUSED_AT_SIGN` | `WARNING` | Default from `share/profile.json` (`test_levels.SYNTAX`). |
| `RNAME_NO_AT_SIGN` | `INFO` | Default from `share/profile.json` (`test_levels.SYNTAX`). |
| `NO_RESPONSE_SOA_QUERY` | `DEBUG` | Default from `share/profile.json` (`test_levels.SYNTAX`). |
| `TEST_CASE_END` | `DEBUG` | Default from `share/profile.json` (`test_levels.SYNTAX`). |
| `TEST_CASE_START` | `DEBUG` | Default from `share/profile.json` (`test_levels.SYNTAX`). |

## Differences From Upstream
- Upstream reference: [`syntax05.md`](../../upstream/tests/Syntax-TP/syntax05.md)
- Differences (Upstream vs Gonemaster):
  - Upstream text describes iterating over Method4/Method5 nameserver sets directly; Gonemaster uses `z.QueryOne` abstraction.
  - Upstream text says de-escaped output from this testcase is used by `SYNTAX08`; Gonemaster does not pass state from `Syntax05` to `Syntax08`.
- Potential upstream report:
  - `yes`
- If yes, include:
  - Upstream expected behavior: Explicit Syntax05->Syntax08 dependency.
  - Gonemaster observed behavior: `Syntax08` is independent and always runs when enabled.
  - evidence: `docs/specifications/upstream/tests/Syntax-TP/syntax05.md`, `engine/test/syntax/syntax.go` (`All`, `Syntax05`, `Syntax08`).
  - report status: `not filed`

## Edge Cases And Limitations
- If multiple SOA records exist, only the first SOA record found is evaluated.
- A DNS response without SOA in answer is treated the same as no response (`NO_RESPONSE_SOA_QUERY`).
- When running through `syntax.All`, this testcase is skipped if `syntax01` did not emit `ONLY_ALLOWED_CHARS`.
