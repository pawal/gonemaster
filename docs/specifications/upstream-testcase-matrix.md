# Upstream Testcase Mapping Matrix

This matrix maps upstream Zonemaster testcase specification files to gonemaster testcase IDs and migration status.

Status meanings:
- `implemented`: testcase exists in current gonemaster implementation.
- `not-implemented`: upstream testcase spec exists but testcase is not currently implemented in gonemaster.
- `needs-review`: testcase exists in gonemaster, but known spec/code or behavior deviations need review and potential upstream reporting.

## Summary
- upstream testcase specs: 74
- implemented: 72
- needs-review: 1
- not-implemented: 1

## Matrix
| Upstream testcase spec file | Upstream testcase | Gonemaster testcase | Status | Notes |
| --- | --- | --- | --- | --- |
| `zonemaster/docs/public/specifications/tests/Address-TP/address01.md` | `address01` | `address01` | implemented | Direct testcase ID mapping. |
| `zonemaster/docs/public/specifications/tests/Address-TP/address02.md` | `address02` | `address02` | implemented | Direct testcase ID mapping. |
| `zonemaster/docs/public/specifications/tests/Address-TP/address03.md` | `address03` | `address03` | implemented | Direct testcase ID mapping. |
| `zonemaster/docs/public/specifications/tests/Basic-TP/basic01.md` | `basic01` | `basic01` | implemented | Direct testcase ID mapping. |
| `zonemaster/docs/public/specifications/tests/Basic-TP/basic02.md` | `basic02` | `basic02` | implemented | Direct testcase ID mapping. |
| `zonemaster/docs/public/specifications/tests/Basic-TP/basic03.md` | `basic03` | `basic03` | implemented | Direct testcase ID mapping. |
| `zonemaster/docs/public/specifications/tests/Connectivity-TP/connectivity01.md` | `connectivity01` | `connectivity01` | implemented | Direct testcase ID mapping. |
| `zonemaster/docs/public/specifications/tests/Connectivity-TP/connectivity02.md` | `connectivity02` | `connectivity02` | implemented | Direct testcase ID mapping. |
| `zonemaster/docs/public/specifications/tests/Connectivity-TP/connectivity03.md` | `connectivity03` | `connectivity03` | implemented | Direct testcase ID mapping. |
| `zonemaster/docs/public/specifications/tests/Connectivity-TP/connectivity04.md` | `connectivity04` | `connectivity04` | implemented | Direct testcase ID mapping. |
| `zonemaster/docs/public/specifications/tests/Consistency-TP/consistency01.md` | `consistency01` | `consistency01` | implemented | Direct testcase ID mapping. |
| `zonemaster/docs/public/specifications/tests/Consistency-TP/consistency02.md` | `consistency02` | `consistency02` | implemented | Direct testcase ID mapping. |
| `zonemaster/docs/public/specifications/tests/Consistency-TP/consistency03.md` | `consistency03` | `consistency03` | implemented | Direct testcase ID mapping. |
| `zonemaster/docs/public/specifications/tests/Consistency-TP/consistency04.md` | `consistency04` | `consistency04` | implemented | Direct testcase ID mapping. |
| `zonemaster/docs/public/specifications/tests/Consistency-TP/consistency05.md` | `consistency05` | `consistency05` | implemented | Direct testcase ID mapping. |
| `zonemaster/docs/public/specifications/tests/Consistency-TP/consistency06.md` | `consistency06` | `consistency06` | implemented | Direct testcase ID mapping. |
| `zonemaster/docs/public/specifications/tests/DNSSEC-TP/dnssec01.md` | `dnssec01` | `dnssec01` | implemented | Direct testcase ID mapping. |
| `zonemaster/docs/public/specifications/tests/DNSSEC-TP/dnssec02.md` | `dnssec02` | `dnssec02` | implemented | Direct testcase ID mapping. |
| `zonemaster/docs/public/specifications/tests/DNSSEC-TP/dnssec03.md` | `dnssec03` | `dnssec03` | implemented | Direct testcase ID mapping. |
| `zonemaster/docs/public/specifications/tests/DNSSEC-TP/dnssec04.md` | `dnssec04` | `dnssec04` | implemented | Direct testcase ID mapping. |
| `zonemaster/docs/public/specifications/tests/DNSSEC-TP/dnssec05.md` | `dnssec05` | `dnssec05` | implemented | Direct testcase ID mapping. |
| `zonemaster/docs/public/specifications/tests/DNSSEC-TP/dnssec06.md` | `dnssec06` | `dnssec06` | implemented | Direct testcase ID mapping. |
| `zonemaster/docs/public/specifications/tests/DNSSEC-TP/dnssec07.md` | `dnssec07` | `dnssec07` | implemented | Direct testcase ID mapping. |
| `zonemaster/docs/public/specifications/tests/DNSSEC-TP/dnssec08.md` | `dnssec08` | `dnssec08` | implemented | Direct testcase ID mapping. |
| `zonemaster/docs/public/specifications/tests/DNSSEC-TP/dnssec09.md` | `dnssec09` | `dnssec09` | implemented | Direct testcase ID mapping. |
| `zonemaster/docs/public/specifications/tests/DNSSEC-TP/dnssec10.md` | `dnssec10` | `dnssec10` | implemented | Direct testcase ID mapping. |
| `zonemaster/docs/public/specifications/tests/DNSSEC-TP/dnssec11.md` | `dnssec11` | `dnssec11` | implemented | Direct testcase ID mapping. |
| `zonemaster/docs/public/specifications/tests/DNSSEC-TP/dnssec12.md` | `dnssec12` | `-` | not-implemented | Known intentional scope gap. See `docs/specifications/known-intentional-gaps.md`. |
| `zonemaster/docs/public/specifications/tests/DNSSEC-TP/dnssec13.md` | `dnssec13` | `dnssec13` | implemented | Direct testcase ID mapping. |
| `zonemaster/docs/public/specifications/tests/DNSSEC-TP/dnssec14.md` | `dnssec14` | `dnssec14` | implemented | Direct testcase ID mapping. |
| `zonemaster/docs/public/specifications/tests/DNSSEC-TP/dnssec15.md` | `dnssec15` | `dnssec15` | implemented | Direct testcase ID mapping. |
| `zonemaster/docs/public/specifications/tests/DNSSEC-TP/dnssec16.md` | `dnssec16` | `dnssec16` | implemented | Direct testcase ID mapping. |
| `zonemaster/docs/public/specifications/tests/DNSSEC-TP/dnssec17.md` | `dnssec17` | `dnssec17` | implemented | Direct testcase ID mapping. |
| `zonemaster/docs/public/specifications/tests/DNSSEC-TP/dnssec18.md` | `dnssec18` | `dnssec18` | implemented | Direct testcase ID mapping. |
| `zonemaster/docs/public/specifications/tests/Delegation-TP/delegation01.md` | `delegation01` | `delegation01` | implemented | Direct testcase ID mapping. |
| `zonemaster/docs/public/specifications/tests/Delegation-TP/delegation02.md` | `delegation02` | `delegation02` | implemented | Direct testcase ID mapping. |
| `zonemaster/docs/public/specifications/tests/Delegation-TP/delegation03.md` | `delegation03` | `delegation03` | implemented | Direct testcase ID mapping. |
| `zonemaster/docs/public/specifications/tests/Delegation-TP/delegation04.md` | `delegation04` | `delegation04` | implemented | Direct testcase ID mapping. |
| `zonemaster/docs/public/specifications/tests/Delegation-TP/delegation05.md` | `delegation05` | `delegation05` | implemented | Direct testcase ID mapping. |
| `zonemaster/docs/public/specifications/tests/Delegation-TP/delegation06.md` | `delegation06` | `delegation06` | implemented | Direct testcase ID mapping. |
| `zonemaster/docs/public/specifications/tests/Delegation-TP/delegation07.md` | `delegation07` | `delegation07` | implemented | Direct testcase ID mapping. |
| `zonemaster/docs/public/specifications/tests/Nameserver-TP/nameserver01.md` | `nameserver01` | `nameserver01` | implemented | Direct testcase ID mapping. |
| `zonemaster/docs/public/specifications/tests/Nameserver-TP/nameserver02.md` | `nameserver02` | `nameserver02` | implemented | Direct testcase ID mapping. |
| `zonemaster/docs/public/specifications/tests/Nameserver-TP/nameserver03.md` | `nameserver03` | `nameserver03` | implemented | Direct testcase ID mapping. |
| `zonemaster/docs/public/specifications/tests/Nameserver-TP/nameserver04.md` | `nameserver04` | `nameserver04` | implemented | Direct testcase ID mapping. |
| `zonemaster/docs/public/specifications/tests/Nameserver-TP/nameserver05.md` | `nameserver05` | `nameserver05` | implemented | Direct testcase ID mapping. |
| `zonemaster/docs/public/specifications/tests/Nameserver-TP/nameserver06.md` | `nameserver06` | `nameserver06` | implemented | Direct testcase ID mapping. |
| `zonemaster/docs/public/specifications/tests/Nameserver-TP/nameserver07.md` | `nameserver07` | `nameserver07` | implemented | Direct testcase ID mapping. |
| `zonemaster/docs/public/specifications/tests/Nameserver-TP/nameserver08.md` | `nameserver08` | `nameserver08` | implemented | Direct testcase ID mapping. |
| `zonemaster/docs/public/specifications/tests/Nameserver-TP/nameserver09.md` | `nameserver09` | `nameserver09` | implemented | Direct testcase ID mapping. |
| `zonemaster/docs/public/specifications/tests/Nameserver-TP/nameserver10.md` | `nameserver10` | `nameserver10` | implemented | Direct testcase ID mapping. |
| `zonemaster/docs/public/specifications/tests/Nameserver-TP/nameserver11.md` | `nameserver11` | `nameserver11` | implemented | Direct testcase ID mapping. |
| `zonemaster/docs/public/specifications/tests/Nameserver-TP/nameserver12.md` | `nameserver12` | `nameserver12` | implemented | Direct testcase ID mapping. |
| `zonemaster/docs/public/specifications/tests/Nameserver-TP/nameserver13.md` | `nameserver13` | `nameserver13` | implemented | Direct testcase ID mapping. |
| `zonemaster/docs/public/specifications/tests/Nameserver-TP/nameserver15.md` | `nameserver15` | `nameserver15` | implemented | Direct testcase ID mapping. |
| `zonemaster/docs/public/specifications/tests/Syntax-TP/syntax01.md` | `syntax01` | `syntax01` | implemented | Direct testcase ID mapping. |
| `zonemaster/docs/public/specifications/tests/Syntax-TP/syntax02.md` | `syntax02` | `syntax02` | implemented | Direct testcase ID mapping. |
| `zonemaster/docs/public/specifications/tests/Syntax-TP/syntax03.md` | `syntax03` | `syntax03` | implemented | Direct testcase ID mapping. |
| `zonemaster/docs/public/specifications/tests/Syntax-TP/syntax04.md` | `syntax04` | `syntax04` | implemented | Direct testcase ID mapping. |
| `zonemaster/docs/public/specifications/tests/Syntax-TP/syntax05.md` | `syntax05` | `syntax05` | implemented | Direct testcase ID mapping. |
| `zonemaster/docs/public/specifications/tests/Syntax-TP/syntax06.md` | `syntax06` | `syntax06` | implemented | Direct testcase ID mapping. |
| `zonemaster/docs/public/specifications/tests/Syntax-TP/syntax07.md` | `syntax07` | `syntax07` | implemented | Direct testcase ID mapping. |
| `zonemaster/docs/public/specifications/tests/Syntax-TP/syntax08.md` | `syntax08` | `syntax08` | implemented | Direct testcase ID mapping. |
| `zonemaster/docs/public/specifications/tests/Zone-TP/zone01.md` | `zone01` | `zone01` | implemented | Direct testcase ID mapping. |
| `zonemaster/docs/public/specifications/tests/Zone-TP/zone02.md` | `zone02` | `zone02` | implemented | Direct testcase ID mapping. |
| `zonemaster/docs/public/specifications/tests/Zone-TP/zone03.md` | `zone03` | `zone03` | implemented | Direct testcase ID mapping. |
| `zonemaster/docs/public/specifications/tests/Zone-TP/zone04.md` | `zone04` | `zone04` | implemented | Direct testcase ID mapping. |
| `zonemaster/docs/public/specifications/tests/Zone-TP/zone05.md` | `zone05` | `zone05` | implemented | Direct testcase ID mapping. |
| `zonemaster/docs/public/specifications/tests/Zone-TP/zone06.md` | `zone06` | `zone06` | implemented | Direct testcase ID mapping. |
| `zonemaster/docs/public/specifications/tests/Zone-TP/zone07.md` | `zone07` | `zone07` | needs-review | Known upstream/spec behavior divergence tracked in `plans/zonemaster-zone07-bug-report.md`. |
| `zonemaster/docs/public/specifications/tests/Zone-TP/zone08.md` | `zone08` | `zone08` | implemented | Direct testcase ID mapping. |
| `zonemaster/docs/public/specifications/tests/Zone-TP/zone09.md` | `zone09` | `zone09` | implemented | Direct testcase ID mapping. |
| `zonemaster/docs/public/specifications/tests/Zone-TP/zone10.md` | `zone10` | `zone10` | implemented | Direct testcase ID mapping. |
| `zonemaster/docs/public/specifications/tests/Zone-TP/zone11.md` | `zone11` | `zone11` | implemented | Direct testcase ID mapping. |
