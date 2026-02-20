# Known Intentional Gaps

This document records upstream testcase gaps that are currently intentional in
gonemaster, with rationale and evidence.

## Gap Inventory

| Upstream testcase | Gonemaster status | Rationale | Evidence | Follow-up |
| --- | --- | --- | --- | --- |
| `dnssec12` | Not implemented (scope-deferred) | Current gonemaster intentionally exposes only implemented DNSSEC testcases. `dnssec12` is excluded from the executable testcase maps and from the documented implemented inventory. | `engine/engine.go` (`dnssecTests` omits `dnssec12`), `docs/specifications/implemented-testcases.md`, `docs/specifications/upstream-testcase-matrix.md` | Keep as deferred gap until a dedicated DNSSEC12 implementation is created. |

## Notes

- No additional intentional implementation gaps are currently identified in the
  upstream testcase matrix.
