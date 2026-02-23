# Known Behavior Divergences (Pre-Tagged For Review)

This file captures behavior divergences already identified in prior bug
investigations and assigns review tags so testcase-spec migration can reference
them consistently.
This document is the running list of clear spec/code deviations tracked for
upstream reporting.

Status meanings:
- `open`: confirmed during investigation, not yet resolved in upstream spec/code
- `tracking`: known and linked, pending testcase-by-testcase assessment

## Reported Upstream

| Review tag | Scope | Status | Upstream report status | Upstream issue | Summary | Primary evidence | Testcase spec evidence |
| --- | --- | --- | --- | --- | --- | --- | --- |
| `DIV-ZONE07-SPEC-TEXT` | `zone07` | open | reported | [zonemaster#1467](https://github.com/zonemaster/zonemaster/issues/1467) | Upstream `zone07` spec text contains unrelated RIPE-203/SOA minimum paragraph. | [zone07.md](upstream/tests/Zone-TP/zone07.md) | [upstream-testcase-matrix.md](upstream-testcase-matrix.md) (`zone07`, `needs-review`) |
| `DIV-ZONE07-SPEC-OUTCOMES` | `zone07` | open | reported | [zonemaster#1467](https://github.com/zonemaster/zonemaster/issues/1467) | Upstream `zone07` outcomes are narrower than implementation-emitted tags (`MNAME_HAS_NO_ADDRESS`, `NO_RESPONSE_SOA_QUERY`, `MNAME_IS_NOT_CNAME`). | [zone07.md](upstream/tests/Zone-TP/zone07.md), [Zone.pm](../../zonemaster-engine/lib/Zonemaster/Engine/Test/Zone.pm) | [upstream-testcase-matrix.md](upstream-testcase-matrix.md) (`zone07`, `needs-review`) |
| `DIV-ZONE07-INTERCASE` | `zone07` | open | reported | [zonemaster#1467](https://github.com/zonemaster/zonemaster/issues/1467) | Upstream says no intercase dependencies, but shared recursive cache can influence later checks. | [zone07.md](upstream/tests/Zone-TP/zone07.md), [Recursor.pm](../../zonemaster-engine/lib/Zonemaster/Engine/Recursor.pm) | [upstream-testcase-matrix.md](upstream-testcase-matrix.md) (`zone07`, `needs-review`) |
| `DIV-ZONE07-UNDEF-CACHE` | `zone07` | open | reported | [zonemaster-engine#1500](https://github.com/zonemaster/zonemaster-engine/issues/1500) | Recursive `undef` cache reuse can produce false `MNAME_HAS_NO_ADDRESS` despite resolvable SOA MNAME. | [Recursor.pm](../../zonemaster-engine/lib/Zonemaster/Engine/Recursor.pm), [Zone.pm](../../zonemaster-engine/lib/Zonemaster/Engine/Test/Zone.pm) | [upstream-testcase-matrix.md](upstream-testcase-matrix.md) (`zone07`, `needs-review`) |
| `DIV-NS13-SPEC-QUERYTYPE` | `nameserver13` | open | reported | [zonemaster#1468](https://github.com/zonemaster/zonemaster/issues/1468) | Upstream testcase specification says query type `DNSKEY`, while both upstream engine and gonemaster implementation send `SOA`. | [nameserver13.md](upstream/tests/Nameserver-TP/nameserver13.md), [Nameserver.pm](../../zonemaster-engine/lib/Zonemaster/Engine/Test/Nameserver.pm) | [tests/nameserver/nameserver13.md](tests/nameserver/nameserver13.md) (`Differences From Upstream`) |
| `DIV-RECURSOR-GLUE-BAILIWICK` | cross-cutting recursor | tracking | reported | [zonemaster-engine#1501](https://github.com/zonemaster/zonemaster-engine/issues/1501) | Glue acceptance path appears insufficiently constrained by bailiwick/delegation relation before reuse. | [Recursor.pm](../../zonemaster-engine/lib/Zonemaster/Engine/Recursor.pm) | Cross-cutting; attach affected testcase spec evidence during Phase 5 migration. |
| `DIV-RECURSOR-CACHE-CONTEXT` | cross-cutting recursor | tracking | reported | [zonemaster-engine#1502](https://github.com/zonemaster/zonemaster-engine/issues/1502) | Recursor cache keying does not distinguish recursive context (e.g., custom NS set vs default). | [Recursor.pm](../../zonemaster-engine/lib/Zonemaster/Engine/Recursor.pm) | Cross-cutting; attach affected testcase spec evidence during Phase 5 migration. |

## Not Yet Reported Upstream

| Review tag | Scope | Status | Upstream report status | Upstream issue | Summary | Primary evidence | Testcase spec evidence |
| --- | --- | --- | --- | --- | --- | --- | --- |
| `DIV-RECURSOR-CACHE-GROWTH` | cross-cutting recursor | tracking | not-reported | `-` | Global recursive cache has no built-in size/TTL controls in recursor layer. | [Recursor.pm](../../zonemaster-engine/lib/Zonemaster/Engine/Recursor.pm) | Cross-cutting; attach affected testcase spec evidence during Phase 5 migration. |
| `DIV-RECURSOR-CACHE-UNDEF` | cross-cutting recursor | tracking | not-reported | `-` | `undef` recursion results can be cached and reused as negatives instead of retrying. | [Recursor.pm](../../zonemaster-engine/lib/Zonemaster/Engine/Recursor.pm) | Cross-cutting; attach affected testcase spec evidence during Phase 5 migration. |

## Testcase Pre-Tagging

Current testcase-specific pre-tagging from known investigations:

- `zone07`: `DIV-ZONE07-UNDEF-CACHE`, `DIV-ZONE07-SPEC-TEXT`, `DIV-ZONE07-SPEC-OUTCOMES`, `DIV-ZONE07-INTERCASE`
- `nameserver13`: `DIV-NS13-SPEC-QUERYTYPE`

Cross-cutting recursor tags above should be considered during testcase migration
when a testcase depends on recursive resolution behavior.
Items in `Not Yet Reported Upstream` should be filed upstream and moved to
`Reported Upstream` once an issue exists.

---

Copyright (c) Patrik Wallström  
Copyright (c) The Swedish Internet Foundation (https://internetstiftelsen.se/en/)  
Copyright (c) AFNIC (https://www.afnic.fr/en/)  
All rights reserved.  

Copyright belongs to external contributor where applicable.  

Creative Commons Attribution 4.0 International License applies. See https://creativecommons.org/licenses/by/4.0/ for the license conditions.
