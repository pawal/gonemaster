# Upstream Testcase Mapping Matrix

This matrix maps upstream Zonemaster testcases to gonemaster testcase IDs and migration status.

Status meanings:
- `implemented`: testcase exists in current gonemaster implementation.
- `not-implemented`: upstream testcase spec exists but testcase is not currently implemented in gonemaster.
- `needs-review`: testcase exists in gonemaster, but known spec/code or behavior deviations need review and potential upstream reporting.

## Summary
- upstream testcase specs: 74
- implemented: 71
- needs-review: 2
- not-implemented: 1

## Matrix
| Upstream testcase | Gonemaster testcase | Status | Notes |
| --- | --- | --- | --- |
| `address01` | `address01` | implemented | Direct testcase ID mapping. |
| `address02` | `address02` | implemented | Direct testcase ID mapping. |
| `address03` | `address03` | implemented | Direct testcase ID mapping. |
| `basic01` | `basic01` | implemented | Direct testcase ID mapping. |
| `basic02` | `basic02` | implemented | Direct testcase ID mapping. |
| `basic03` | `basic03` | implemented | Direct testcase ID mapping. |
| `connectivity01` | `connectivity01` | implemented | Direct testcase ID mapping. |
| `connectivity02` | `connectivity02` | implemented | Direct testcase ID mapping. |
| `connectivity03` | `connectivity03` | implemented | Direct testcase ID mapping. |
| `connectivity04` | `connectivity04` | implemented | Direct testcase ID mapping. |
| `consistency01` | `consistency01` | implemented | Direct testcase ID mapping. |
| `consistency02` | `consistency02` | implemented | Direct testcase ID mapping. |
| `consistency03` | `consistency03` | implemented | Direct testcase ID mapping. |
| `consistency04` | `consistency04` | implemented | Direct testcase ID mapping. |
| `consistency05` | `consistency05` | implemented | Direct testcase ID mapping. |
| `consistency06` | `consistency06` | implemented | Direct testcase ID mapping. |
| `dnssec01` | `dnssec01` | implemented | Direct testcase ID mapping. |
| `dnssec02` | `dnssec02` | implemented | Direct testcase ID mapping. |
| `dnssec03` | `dnssec03` | implemented | Direct testcase ID mapping. |
| `dnssec04` | `dnssec04` | implemented | Direct testcase ID mapping. |
| `dnssec05` | `dnssec05` | implemented | Direct testcase ID mapping. |
| `dnssec06` | `dnssec06` | implemented | Direct testcase ID mapping. |
| `dnssec07` | `dnssec07` | implemented | Direct testcase ID mapping. |
| `dnssec08` | `dnssec08` | implemented | Direct testcase ID mapping. |
| `dnssec09` | `dnssec09` | implemented | Direct testcase ID mapping. |
| `dnssec10` | `dnssec10` | implemented | Direct testcase ID mapping. |
| `dnssec11` | `dnssec11` | implemented | Direct testcase ID mapping. |
| `dnssec12` | `-` | not-implemented | Known intentional scope gap. See `docs/specifications/known-intentional-gaps.md`. |
| `dnssec13` | `dnssec13` | implemented | Direct testcase ID mapping. |
| `dnssec14` | `dnssec14` | implemented | Direct testcase ID mapping. |
| `dnssec15` | `dnssec15` | implemented | Direct testcase ID mapping. |
| `dnssec16` | `dnssec16` | implemented | Direct testcase ID mapping. |
| `dnssec17` | `dnssec17` | implemented | Direct testcase ID mapping. |
| `dnssec18` | `dnssec18` | implemented | Direct testcase ID mapping. |
| `delegation01` | `delegation01` | implemented | Direct testcase ID mapping. |
| `delegation02` | `delegation02` | implemented | Direct testcase ID mapping. |
| `delegation03` | `delegation03` | implemented | Direct testcase ID mapping. |
| `delegation04` | `delegation04` | implemented | Direct testcase ID mapping. |
| `delegation05` | `delegation05` | implemented | Direct testcase ID mapping. |
| `delegation06` | `delegation06` | implemented | Direct testcase ID mapping. |
| `delegation07` | `delegation07` | implemented | Direct testcase ID mapping. |
| `nameserver01` | `nameserver01` | implemented | Direct testcase ID mapping. |
| `nameserver02` | `nameserver02` | implemented | Direct testcase ID mapping. |
| `nameserver03` | `nameserver03` | implemented | Direct testcase ID mapping. |
| `nameserver04` | `nameserver04` | implemented | Direct testcase ID mapping. |
| `nameserver05` | `nameserver05` | implemented | Direct testcase ID mapping. |
| `nameserver06` | `nameserver06` | implemented | Direct testcase ID mapping. |
| `nameserver07` | `nameserver07` | implemented | Direct testcase ID mapping. |
| `nameserver08` | `nameserver08` | implemented | Direct testcase ID mapping. |
| `nameserver09` | `nameserver09` | implemented | Direct testcase ID mapping. |
| `nameserver10` | `nameserver10` | implemented | Direct testcase ID mapping. |
| `nameserver11` | `nameserver11` | implemented | Direct testcase ID mapping. |
| `nameserver12` | `nameserver12` | implemented | Direct testcase ID mapping. |
| `nameserver13` | `nameserver13` | needs-review | Review tag: `DIV-NS13-SPEC-QUERYTYPE`. Reported as [zonemaster-engine#1503](https://github.com/zonemaster/zonemaster-engine/issues/1503). See `docs/specifications/known-behavior-divergences.md`. |
| `nameserver15` | `nameserver15` | implemented | Direct testcase ID mapping. |
| `syntax01` | `syntax01` | implemented | Direct testcase ID mapping. |
| `syntax02` | `syntax02` | implemented | Direct testcase ID mapping. |
| `syntax03` | `syntax03` | implemented | Direct testcase ID mapping. |
| `syntax04` | `syntax04` | implemented | Direct testcase ID mapping. |
| `syntax05` | `syntax05` | implemented | Direct testcase ID mapping. |
| `syntax06` | `syntax06` | implemented | Direct testcase ID mapping. |
| `syntax07` | `syntax07` | implemented | Direct testcase ID mapping. |
| `syntax08` | `syntax08` | implemented | Direct testcase ID mapping. |
| `zone01` | `zone01` | implemented | Direct testcase ID mapping. |
| `zone02` | `zone02` | implemented | Direct testcase ID mapping. |
| `zone03` | `zone03` | implemented | Direct testcase ID mapping. |
| `zone04` | `zone04` | implemented | Direct testcase ID mapping. |
| `zone05` | `zone05` | implemented | Direct testcase ID mapping. |
| `zone06` | `zone06` | implemented | Direct testcase ID mapping. |
| `zone07` | `zone07` | needs-review | Review tags: `DIV-ZONE07-UNDEF-CACHE`, `DIV-ZONE07-SPEC-TEXT`, `DIV-ZONE07-SPEC-OUTCOMES`, `DIV-ZONE07-INTERCASE`. See `docs/specifications/known-behavior-divergences.md`. |
| `zone08` | `zone08` | implemented | Direct testcase ID mapping. |
| `zone09` | `zone09` | implemented | Direct testcase ID mapping. |
| `zone10` | `zone10` | implemented | Direct testcase ID mapping. |
| `zone11` | `zone11` | implemented | Direct testcase ID mapping. |
