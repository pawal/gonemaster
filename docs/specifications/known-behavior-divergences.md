# Known Behavior Divergences

This document is the running list of clear spec/code deviations between gonemaster
and upstream Zonemaster. Each item carries a stable review tag so testcase
specifications and [upstream-testcase-matrix.md](upstream-testcase-matrix.md) can
reference it consistently.

Every divergence reported upstream belongs in this file. The full set of reports,
including the few that are not behavior divergences, is
[`author:pawal org:zonemaster`](https://github.com/search?q=org%3Azonemaster+is%3Aissue+author%3Apawal+created%3A%3E%3D2026-01-01&type=issues)
on GitHub: 21 issues since January 2026, 9 of them closed. The three not listed
below are a GUI usability report ([zonemaster#1476](https://github.com/zonemaster/zonemaster/issues/1476)),
a licensing-text issue ([zonemaster#1480](https://github.com/zonemaster/zonemaster/issues/1480)),
and a root-hints refresh ([zonemaster-engine#1489](https://github.com/zonemaster/zonemaster-engine/issues/1489)).

Status meanings:
- `open`: confirmed during investigation, not yet resolved in upstream spec/code
- `tracking`: known and linked, pending testcase-by-testcase assessment
- `proposal`: not a divergence today, both sides behave the same; a change has been proposed upstream
- `resolved`: no longer a divergence (fixed upstream and/or gonemaster already matches); kept for history

## Reported Upstream

| Review tag | Scope | Status | Upstream report status | Upstream issue | Summary | Primary evidence | Testcase spec evidence |
| --- | --- | --- | --- | --- | --- | --- | --- |
| `DIV-CONS04-NS-TTL` | `consistency04` | open | reported | [zonemaster#1503](https://github.com/zonemaster/zonemaster/issues/1503) | Upstream spec makes TTL part of NS-RRset equality, but the implementation compares NS target names only, so the two contradict each other. Gonemaster keeps name-only equality and reports differing apex NS RRset TTLs separately as `INCONSISTENT_NS_TTL`. | upstream `consistency04` test plan, upstream engine `Consistency.pm` | [tests/consistency/consistency04.md](tests/consistency/consistency04.md) (`Potential upstream report`) |
| `DIV-CONS05-LAME` | `consistency05` | open | reported | [zonemaster-engine#1522](https://github.com/zonemaster/zonemaster-engine/issues/1522) | Upstream emits `CHILD_ZONE_LAME` when every server fails for one in-domain NS name, so a disjoint parent/child NS set whose servers all answer authoritatively is reported as lameness instead of an address mismatch. Gonemaster emits it only when every in-domain NS name failed on every server. | upstream engine `Consistency.pm` | [tests/consistency/consistency05.md](tests/consistency/consistency05.md) (`Spec/Implementation Divergences`) |
| `DIV-DEL01-IPV6-SEVERITY` | `delegation01` | proposal | reported | [zonemaster#1529](https://github.com/zonemaster/zonemaster/issues/1529) | RFC 10001 section 4.1 makes a delegation without IPv6 a stronger fault than the current severity. Both implementations emit `NO_IPV6_NS_DEL` at `NOTICE` today, so this is a proposed severity change, not a deviation. | RFC 10001 section 4.1, [share/profile.json](../../share/profile.json) | [tests/delegation/delegation01.md](tests/delegation/delegation01.md) (`Severity`) |
| `DIV-DEL03-512-BAND` | `delegation03` | open | reported | [zonemaster#1499](https://github.com/zonemaster/zonemaster/issues/1499) | Upstream grades referral size against the 512-byte non-EDNS limit alone, so a referral of 513 to 1232 bytes raises a `WARNING` even though it is delivered in one UDP packet after DNS flag day 2020. Gonemaster grades in three bands with a 1232-byte EDNS tier. | upstream `delegation03` test plan | [tests/delegation/delegation03.md](tests/delegation/delegation03.md) (`Procedure`) |
| `DIV-DS02-PRIVATE-DS-ALGO` | `dnssec02` | open | reported | [zonemaster-engine#1544](https://github.com/zonemaster/zonemaster-engine/issues/1544) | Upstream reports a DS/DNSKEY mismatch without emitting anything that says the parent publishes a private DS algorithm (253 or 254), so the cause of the mismatch is invisible. Gonemaster classifies 253 and 254 explicitly as `DS01_DS_ALGO_PRIVATE` and `DS05_ALGO_PRIVATE`, so the run names the cause. | [dnssec.go](../../engine/test/dnssec/dnssec.go), upstream engine `DNSSEC.pm` | [tests/dnssec/dnssec01.md](tests/dnssec/dnssec01.md), [tests/dnssec/dnssec05.md](tests/dnssec/dnssec05.md) |
| `DIV-DS05-ALGO18` | `dnssec05` | open | reported | [zonemaster#1530](https://github.com/zonemaster/zonemaster/issues/1530) | Upstream reports DNSKEY algorithm 18 (ML-DSA-44) as unassigned after IANA allocated it. Gonemaster recognises algorithm 18 and verifies its signatures. | IANA DNSSEC algorithm registry, [dnssec.go](../../engine/test/dnssec/dnssec.go) | [tests/dnssec/dnssec05.md](tests/dnssec/dnssec05.md) |
| `DIV-DS09-RRSIG-FALSE-POSITIVE` | `dnssec09` | resolved | reported | [zonemaster-engine#1512](https://github.com/zonemaster/zonemaster-engine/issues/1512) | Resolved: upstream emitted `DS09_RRSIG_NOT_VALID_BY_DNSKEY` for a zone whose SOA RRSIG does validate against the matching algorithm-13 DNSKEY. Found while cross-checking DNSSEC09 results; no gonemaster-side deviation is recorded for this testcase. | upstream engine `DNSSEC.pm` | [tests/dnssec/dnssec09.md](tests/dnssec/dnssec09.md) |
| `DIV-DS21-PARENT-DS-RRSIG` | `dnssec21` (no upstream testcase) | open | reported | [zonemaster#1481](https://github.com/zonemaster/zonemaster/issues/1481) | No upstream testcase validates the parent's RRSIG over the DS RRset, so a parent-side DNSSEC failure leaves the child reporting a clean DNSSEC result while validating resolvers return SERVFAIL. Gonemaster ships this as `dnssec21` and proposed it upstream as a new testcase. | upstream DNSSEC test plan | [tests/dnssec/dnssec21.md](tests/dnssec/dnssec21.md) |
| `DIV-NS01-NXDOMAIN-FLAGS` | `nameserver01` | open | reported | [zonemaster#1466](https://github.com/zonemaster/zonemaster/issues/1466) | Upstream classifies a server as a recursor when all probe responses are `NXDOMAIN`, regardless of the `AA` and `RA` flags, so authoritative-only servers that answer `NXDOMAIN` with `RA=0` are reported as open recursors. Gonemaster requires `RA=1` on some probe response and excludes `NXDOMAIN` answers carrying `AA=1`. | upstream engine `Nameserver.pm` | [tests/nameserver/nameserver01.md](tests/nameserver/nameserver01.md) (`Spec/Implementation Divergences`) |
| `DIV-NS11-TAG-TYPO` | `nameserver11` | resolved | reported | [zonemaster-engine#1507](https://github.com/zonemaster/zonemaster-engine/issues/1507) | Resolved: upstream emitted the tag `N11_N11_NO_EDNS` where the message catalog defines `N11_NO_EDNS`. Gonemaster emits `N11_NO_EDNS`. | [nameserver.go](../../engine/test/nameserver/nameserver.go), upstream engine `Nameserver.pm` | [tests/nameserver/nameserver11.md](tests/nameserver/nameserver11.md) |
| `DIV-NS13-SPEC-QUERYTYPE` | `nameserver13` | resolved | reported | [zonemaster-engine#1503](https://github.com/zonemaster/zonemaster-engine/issues/1503) | Resolved: spec expects query type `DNSKEY`; the upstream engine previously sent `SOA` and was fixed in engine v8.1.1 (PR #1505). gonemaster already sends `DNSKEY`, so both sides now match the spec. | upstream engine `Nameserver.pm` | [tests/nameserver/nameserver13.md](tests/nameserver/nameserver13.md) (`Resolved upstream issues`) |
| `DIV-ZONE07-SPEC-TEXT` | `zone07` | open | reported | [zonemaster#1467](https://github.com/zonemaster/zonemaster/issues/1467) | Upstream `zone07` spec text contains unrelated RIPE-203/SOA minimum paragraph. | upstream `zone07` test plan | [upstream-testcase-matrix.md](upstream-testcase-matrix.md) (`zone07`, `needs-review`) |
| `DIV-ZONE07-SPEC-OUTCOMES` | `zone07` | open | reported | [zonemaster#1467](https://github.com/zonemaster/zonemaster/issues/1467) | Upstream `zone07` outcomes are narrower than implementation-emitted tags (`MNAME_HAS_NO_ADDRESS`, `NO_RESPONSE_SOA_QUERY`, `MNAME_IS_NOT_CNAME`). | upstream `zone07` test plan, upstream engine `Zone.pm` | [upstream-testcase-matrix.md](upstream-testcase-matrix.md) (`zone07`, `needs-review`) |
| `DIV-ZONE07-INTERCASE` | `zone07` | open | reported | [zonemaster#1467](https://github.com/zonemaster/zonemaster/issues/1467) | Upstream says no intercase dependencies, but shared recursive cache can influence later checks. | upstream `zone07` test plan, upstream engine `Recursor.pm` | [upstream-testcase-matrix.md](upstream-testcase-matrix.md) (`zone07`, `needs-review`) |
| `DIV-ZONE07-UNDEF-CACHE` | `zone07` | open | reported | [zonemaster-engine#1500](https://github.com/zonemaster/zonemaster-engine/issues/1500) | Recursive `undef` cache reuse can produce false `MNAME_HAS_NO_ADDRESS` despite resolvable SOA MNAME. | upstream engine `Recursor.pm` and `Zone.pm` | [upstream-testcase-matrix.md](upstream-testcase-matrix.md) (`zone07`, `needs-review`) |
| `DIV-ZONE11-SPF-DOMAIN-ARG` | `zone11` | resolved | reported | [zonemaster#1469](https://github.com/zonemaster/zonemaster/issues/1469) | Resolved: the upstream `zone11` spec lists a `domain` argument for `Z11_NO_SPF_NON_MAIL_DOMAIN`, but `Zone.pm` emitted an empty argument hash. Gonemaster emits `domain`. | upstream `zone11` test plan, upstream engine `Zone.pm` | [tests/zone/zone11.md](tests/zone/zone11.md) (`Log Arguments`) |
| `DIV-RECURSOR-QNAME-GUARD` | cross-cutting recursor | resolved | reported | [zonemaster-engine#1488](https://github.com/zonemaster/zonemaster-engine/issues/1488) | Resolved: upstream's guard against following a referral back up the hierarchy compared against `state->{qname}`, which no caller set, so the common-suffix length was always 0 and the guard never fired. Gonemaster sets the query name once on entry, so its equivalent guard is active. | [recurse.go](../../engine/recursor/recurse.go), upstream engine `Recursor.pm` | Cross-cutting. |
| `DIV-RECURSOR-GLUE-BAILIWICK` | cross-cutting recursor | tracking | reported | [zonemaster-engine#1501](https://github.com/zonemaster/zonemaster-engine/issues/1501) | Glue acceptance path appears insufficiently constrained by in-domain/delegation relation before reuse. | upstream engine `Recursor.pm` | Cross-cutting; attach affected testcase spec evidence when a testcase is shown to depend on it. |
| `DIV-RECURSOR-CACHE-CONTEXT` | cross-cutting recursor | resolved | reported | [zonemaster-engine#1502](https://github.com/zonemaster/zonemaster-engine/issues/1502) | Resolved: upstream fixed recursor cache keying to distinguish recursive context (custom NS set vs default) in engine v9.0.0 (PR #1521). gonemaster was never affected; its recurse cache key already includes the sorted custom-NS set (`cacheNameKey`). | [resolve.go](../../engine/recursor/resolve.go), upstream engine `Recursor.pm` | Cross-cutting. |

## Not Yet Reported Upstream

| Review tag | Scope | Status | Upstream report status | Upstream issue | Summary | Primary evidence | Testcase spec evidence |
| --- | --- | --- | --- | --- | --- | --- | --- |
| `DIV-CONS01-SERIAL-ARITH` | `consistency01` | open | not-reported | `-` | Upstream sorts serial values as strings and subtracts the first from the last, which mis-ranks serials of differing length and serials near the 32-bit wrap boundary. Gonemaster orders serials with RFC 1982 arithmetic. | upstream engine `Consistency.pm` | [tests/consistency/consistency01.md](tests/consistency/consistency01.md) (`Potential upstream report`) |
| `DIV-RECURSOR-CACHE-GROWTH` | cross-cutting recursor | tracking | not-reported | `-` | Global recursive cache has no built-in size/TTL controls in recursor layer. | upstream engine `Recursor.pm` | Cross-cutting; attach affected testcase spec evidence when a testcase is shown to depend on it. |
| `DIV-RECURSOR-CACHE-UNDEF` | cross-cutting recursor | tracking | not-reported | `-` | `undef` recursion results can be cached and reused as negatives instead of retrying. | upstream engine `Recursor.pm` | Cross-cutting; attach affected testcase spec evidence when a testcase is shown to depend on it. |

## Per-Testcase Index

- `consistency01`: `DIV-CONS01-SERIAL-ARITH`
- `consistency04`: `DIV-CONS04-NS-TTL`
- `consistency05`: `DIV-CONS05-LAME`
- `delegation01`: `DIV-DEL01-IPV6-SEVERITY` (proposal; see table)
- `delegation03`: `DIV-DEL03-512-BAND`
- `dnssec02`: `DIV-DS02-PRIVATE-DS-ALGO`
- `dnssec05`: `DIV-DS05-ALGO18`
- `dnssec09`: `DIV-DS09-RRSIG-FALSE-POSITIVE` (resolved; see table)
- `dnssec21`: `DIV-DS21-PARENT-DS-RRSIG`
- `nameserver01`: `DIV-NS01-NXDOMAIN-FLAGS`
- `nameserver11`: `DIV-NS11-TAG-TYPO` (resolved; see table)
- `nameserver13`: `DIV-NS13-SPEC-QUERYTYPE` (resolved; see table)
- `zone07`: `DIV-ZONE07-UNDEF-CACHE`, `DIV-ZONE07-SPEC-TEXT`, `DIV-ZONE07-SPEC-OUTCOMES`, `DIV-ZONE07-INTERCASE`
- `zone11`: `DIV-ZONE11-SPF-DOMAIN-ARG` (resolved; see table)

Cross-cutting recursor tags should be considered when a testcase depends on
recursive resolution behavior.
Items in `Not Yet Reported Upstream` should be filed upstream and moved to
`Reported Upstream` once an issue exists.
