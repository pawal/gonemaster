# Known Intentional Gaps

This document records upstream testcase gaps that are currently intentional in
gonemaster, with rationale and evidence.

## Gap Inventory

| Upstream testcase | Gonemaster status | Rationale | Evidence | Follow-up |
| --- | --- | --- | --- | --- |
| `dnssec12` | Not implemented (scope-deferred) | Current gonemaster intentionally exposes only implemented DNSSEC testcases. `dnssec12` is excluded from the executable testcase maps and from the documented implemented inventory. | `engine/engine.go` (`dnssecTests` omits `dnssec12`), `docs/specifications/implemented-testcases.md`, `docs/specifications/upstream-testcase-matrix.md` | Keep as deferred gap until a dedicated DNSSEC12 implementation is created. |
| `nameserver14` | Not implemented (intentional numbering gap) | Upstream testcase set currently has no `nameserver14` specification file, and gonemaster intentionally keeps numbering aligned with upstream by implementing `nameserver01`-`nameserver13` and `nameserver15`. | `docs/specifications/upstream/tests/Nameserver-TP/` (no `nameserver14.md`), `engine/engine.go` (`nameserverTests` omits `nameserver14`), `docs/specifications/implemented-testcases.md` | Revisit only if upstream introduces `nameserver14` (or gonemaster decides to define a local testcase with that ID). |

## Notes

- Intentional gaps currently tracked: `dnssec12` (scope-deferred) and
  `nameserver14` (upstream numbering gap).

---

Copyright (c) Patrik Wallström  
Copyright (c) The Swedish Internet Foundation (https://internetstiftelsen.se/en/)  
Copyright (c) AFNIC (https://www.afnic.fr/en/)  
All rights reserved.  

Copyright belongs to external contributor where applicable.  

Creative Commons Attribution 4.0 International License applies. See https://creativecommons.org/licenses/by/4.0/ for the license conditions.
