# Known Behavior Divergences (Pre-Tagged For Review)

This file captures behavior divergences already identified in prior bug
investigations and assigns review tags so testcase-spec migration can reference
them consistently.
This document is the running list of clear spec/code deviations tracked for
upstream reporting.

Status meanings:
- `open`: confirmed during investigation, not yet resolved in upstream spec/code
- `tracking`: known and linked, pending testcase-by-testcase assessment
- `resolved`: no longer a divergence (fixed upstream and/or gonemaster already matches); kept for history

## Reported Upstream

| Review tag | Scope | Status | Upstream report status | Upstream issue | Summary | Primary evidence | Testcase spec evidence |
| --- | --- | --- | --- | --- | --- | --- | --- |
| `DIV-ZONE07-SPEC-TEXT` | `zone07` | open | reported | [zonemaster#1467](https://github.com/zonemaster/zonemaster/issues/1467) | Upstream `zone07` spec text contains unrelated RIPE-203/SOA minimum paragraph. | upstream `zone07` test plan | [upstream-testcase-matrix.md](upstream-testcase-matrix.md) (`zone07`, `needs-review`) |
| `DIV-ZONE07-SPEC-OUTCOMES` | `zone07` | open | reported | [zonemaster#1467](https://github.com/zonemaster/zonemaster/issues/1467) | Upstream `zone07` outcomes are narrower than implementation-emitted tags (`MNAME_HAS_NO_ADDRESS`, `NO_RESPONSE_SOA_QUERY`, `MNAME_IS_NOT_CNAME`). | upstream `zone07` test plan, upstream engine `Zone.pm` | [upstream-testcase-matrix.md](upstream-testcase-matrix.md) (`zone07`, `needs-review`) |
| `DIV-ZONE07-INTERCASE` | `zone07` | open | reported | [zonemaster#1467](https://github.com/zonemaster/zonemaster/issues/1467) | Upstream says no intercase dependencies, but shared recursive cache can influence later checks. | upstream `zone07` test plan, upstream engine `Recursor.pm` | [upstream-testcase-matrix.md](upstream-testcase-matrix.md) (`zone07`, `needs-review`) |
| `DIV-ZONE07-UNDEF-CACHE` | `zone07` | open | reported | [zonemaster-engine#1500](https://github.com/zonemaster/zonemaster-engine/issues/1500) | Recursive `undef` cache reuse can produce false `MNAME_HAS_NO_ADDRESS` despite resolvable SOA MNAME. | upstream engine `Recursor.pm` and `Zone.pm` | [upstream-testcase-matrix.md](upstream-testcase-matrix.md) (`zone07`, `needs-review`) |
| `DIV-NS13-SPEC-QUERYTYPE` | `nameserver13` | resolved | reported | [zonemaster#1468](https://github.com/zonemaster/zonemaster/issues/1468) | Resolved: spec expects query type `DNSKEY`; the upstream engine previously sent `SOA` and was fixed in engine v8.1.1 (PR #1505). gonemaster already sends `DNSKEY`, so both sides now match the spec. | upstream engine `Nameserver.pm` | [tests/nameserver/nameserver13.md](tests/nameserver/nameserver13.md) (`Resolved upstream issues`) |
| `DIV-RECURSOR-GLUE-BAILIWICK` | cross-cutting recursor | tracking | reported | [zonemaster-engine#1501](https://github.com/zonemaster/zonemaster-engine/issues/1501) | Glue acceptance path appears insufficiently constrained by in-domain/delegation relation before reuse. | upstream engine `Recursor.pm` | Cross-cutting; attach affected testcase spec evidence during Phase 5 migration. |
| `DIV-RECURSOR-CACHE-CONTEXT` | cross-cutting recursor | resolved | reported | [zonemaster-engine#1502](https://github.com/zonemaster/zonemaster-engine/issues/1502) | Resolved: upstream fixed recursor cache keying to distinguish recursive context (custom NS set vs default) in engine v9.0.0 (PR #1521). gonemaster was never affected; its recurse cache key already includes the sorted custom-NS set (`cacheNameKey`). | [resolve.go](../../engine/recursor/resolve.go), upstream engine `Recursor.pm` | Cross-cutting. |

## Not Yet Reported Upstream

| Review tag | Scope | Status | Upstream report status | Upstream issue | Summary | Primary evidence | Testcase spec evidence |
| --- | --- | --- | --- | --- | --- | --- | --- |
| `DIV-RECURSOR-CACHE-GROWTH` | cross-cutting recursor | tracking | not-reported | `-` | Global recursive cache has no built-in size/TTL controls in recursor layer. | upstream engine `Recursor.pm` | Cross-cutting; attach affected testcase spec evidence during Phase 5 migration. |
| `DIV-RECURSOR-CACHE-UNDEF` | cross-cutting recursor | tracking | not-reported | `-` | `undef` recursion results can be cached and reused as negatives instead of retrying. | upstream engine `Recursor.pm` | Cross-cutting; attach affected testcase spec evidence during Phase 5 migration. |

## Testcase Pre-Tagging

Current testcase-specific pre-tagging from known investigations:

- `zone07`: `DIV-ZONE07-UNDEF-CACHE`, `DIV-ZONE07-SPEC-TEXT`, `DIV-ZONE07-SPEC-OUTCOMES`, `DIV-ZONE07-INTERCASE`
- `nameserver13`: `DIV-NS13-SPEC-QUERYTYPE` (resolved; see table)

Cross-cutting recursor tags above should be considered during testcase migration
when a testcase depends on recursive resolution behavior.
Items in `Not Yet Reported Upstream` should be filed upstream and moved to
`Reported Upstream` once an issue exists.
