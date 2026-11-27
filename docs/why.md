# Why gonemaster

gonemaster is an open source DNS and DNSSEC test engine written in Go. This page
describes what it is, how it compares with Zonemaster, and when another tool is
the better choice.

## What it is

Give gonemaster a domain and it runs 87 testcases in 9 modules against the live
delegation: basic, address, connectivity, consistency, delegation, dnssec,
nameserver, syntax, and zone. Every finding is a tagged message with a severity
from DEBUG to CRITICAL, and the findings are scored into a number out of 100 and
a letter grade from A+ to F.

The engine comes with a small set of tools around it:

- `gonemaster`: the CLI. It needs no server and no database, and prints text,
  JSON, or streaming output.
- `gonemaster-server`: an HTTP server with a job queue, batches, stored run
  history, profiles, Prometheus metrics, and three embedded web UIs, on SQLite,
  PostgreSQL, or MariaDB.
- `gonemaster-client`: an automation client for shells and pipelines.
- `gonemaster-nagios`: a Nagios and Icinga plugin that maps finding severities
  to service states.
- `gonemaster-mcp`: a Model Context Protocol server, so an AI agent can run
  tests, search and diff runs, and roll up cohort statistics. The server
  serves the same tools over HTTP.

A public instance runs at <https://gonemaster.evilbit.de/>. Locally, one command
is enough:

```console
gonemaster example.com
```

## Why it exists

gonemaster began as an independent implementation of the Zonemaster testcase
specifications, in a different language. As a separate project it sets its own
schedule: a testcase is added when its specification is settled, and a fix
ships in the next tag.

gonemaster now implements fourteen testcases that have no upstream counterpart
and maintains its own specification corpus. Implementing the shared
specifications turned up cases where the specification and the reference
implementation disagreed, and those were reported upstream. For command-line,
API, and batch testing, gonemaster can replace a Zonemaster deployment today.

## How it compares with Zonemaster

Figures for Zonemaster below are as of upstream v2026.1, released 29 June 2026.

### Origin of the specifications

Zonemaster is the work of Internetstiftelsen (The Swedish Internet Foundation)
and AFNIC. They defined this way of testing a DNS delegation and wrote the
specification corpus gonemaster grew from. gonemaster's testcase specifications
are derived from theirs, carry their copyright alongside gonemaster's, and stay
under CC BY 4.0.

Zonemaster is the reference implementation. Both sponsoring registries run it as
a public service, it has more than a decade of operational history, and many
deployments are built around its Backend and GUI.

### Issues reported upstream

Bugs found while implementing the specifications are reported upstream:
[twenty-one issue reports](https://github.com/search?q=org%3Azonemaster+is%3Aissue+author%3Apawal+created%3A%3E%3D2026-01-01&type=issues)
across the Zonemaster repositories since January 2026, nine of them already
closed.

Fixed upstream:

- [zonemaster-engine#1503](https://github.com/zonemaster/zonemaster-engine/issues/1503):
  `nameserver13` queried SOA where the specification calls for DNSKEY.
- [zonemaster-engine#1488](https://github.com/zonemaster/zonemaster-engine/issues/1488):
  the recursor's upward-referral guard never triggered, because the state field
  it tested was never set.
- [zonemaster-engine#1502](https://github.com/zonemaster/zonemaster-engine/issues/1502):
  recursive lookups with a custom nameserver set shared cache entries with the
  default resolver context.
- [zonemaster-engine#1507](https://github.com/zonemaster/zonemaster-engine/issues/1507):
  `nameserver11` emitted the tag `N11_N11_NO_EDNS`.

Still open:

- [zonemaster#1481](https://github.com/zonemaster/zonemaster/issues/1481): a
  proposal to add the check gonemaster ships as `dnssec21`, which would make the
  parent's DS RRset signing part of the shared specification.
- [zonemaster#1503](https://github.com/zonemaster/zonemaster/issues/1503):
  `consistency04` compares NS RRsets without the TTL, though the specification
  includes it.
- [zonemaster#1499](https://github.com/zonemaster/zonemaster/issues/1499):
  `delegation03` measures referral size against the obsolete 512-byte limit.
- [zonemaster#1467](https://github.com/zonemaster/zonemaster/issues/1467) and
  [zonemaster-engine#1500](https://github.com/zonemaster/zonemaster-engine/issues/1500):
  the `zone07` specification text, and a false `MNAME_HAS_NO_ADDRESS` produced by
  a cached negative recursion result.
- [zonemaster-engine#1522](https://github.com/zonemaster/zonemaster-engine/issues/1522):
  `consistency05` reports a false finding for a lame delegation.

The mapping between the two testcase sets, including what is still under review,
is published as the
[upstream testcase matrix](https://pawal.codeberg.page/gonemaster/specifications/upstream-testcase-matrix/).

### Where the projects have diverged

The two projects now maintain separate specification sets. They differ in
coverage and in release cadence.

gonemaster implements 87 testcases. Upstream publishes 74 testcase
specifications, one of which (`dnssec12`, DNSSEC algorithm completeness) is a
placeholder that upstream states is not yet implemented. gonemaster implements
the other 73, and adds fourteen testcases that have no upstream counterpart:

| Testcase | What it checks | Reference |
|---|---|---|
| `connectivity05` | whether the apex DNSKEY answer is delivered over UDP, and what an address does when a payload large enough for it is advertised | RFC 6891 |
| `dnssec19` | DNSKEY records against known cryptographic weaknesses and blocklists of compromised keys | RFC 3110, RFC 6605, RFC 8080 |
| `dnssec20` | that the NSEC/NSEC3 apex type bitmap matches the RR types actually present | RFC 4034, RFC 5155, RFC 8198 |
| `dnssec21` | that the parent zone signs the DS RRset delegating the child | RFC 4035 |
| `dnssec22` | that the address records of every in-domain nameserver name validate under the chain of trust of the zone | RFC 4035, RFC 9499 |
| `dnssec23` | that the NSEC or NSEC3 records of one denial of existence response are consistent with each other and with the apex NSEC3PARAM RRset | RFC 4034, RFC 5155 |
| `dnssec24` | the authenticated DNSSEC bootstrapping signals of a signed zone that has no DS at its parent | RFC 9615 |
| `nameserver16` | NSID: which servers answer with one, and what it contains | RFC 5001 |
| `nameserver17` | DNS Cookie support, and whether the Server Cookie is well formed and accepted | RFC 7873, RFC 9018 |
| `nameserver18` | Extended DNS Errors from an authoritative server, classified by info-code | RFC 8914 |
| `zone12` | CSYNC at the zone apex | RFC 7477 |
| `zone13` | that the apex SPF policy stays inside the DNS lookup limit | RFC 7208 section 4.6.4 |
| `zone14` | ZONEMD at the zone apex | RFC 8976 |
| `zone15` | CAA presence and syntax at the zone apex | RFC 8659 |

Upstream ships roughly two feature releases a year: v2024.1 (July 2024), v2024.2
(December 2024), v2025.1 (June 2025), v2025.2 (December 2025), v2026.1 (June
2026). gonemaster releases continuously from CI: 91 tags, most recently v1.7.29
on 26 November 2026.

### Side by side

|  | gonemaster | Zonemaster |
|---|---|---|
| Testcases | 87 implemented, 9 modules | 74 documented specifications, one of them a placeholder |
| Specification corpus | own corpus under CC BY 4.0, derived from upstream's | upstream corpus under CC BY 4.0 |
| Release cadence | continuous from CI, 91 tags, v1.7.29 | about two feature releases a year |
| Language and runtime | Go, one static binary | Perl |
| Deployment shape | one binary with the web UIs embedded, on SQLite, PostgreSQL, or MariaDB | Engine, CLI, Backend, and GUI installed separately |
| Numeric score and letter grade | 0-100 and A+ to F | not available |
| Batch testing | yes | yes, through the Backend |
| Cohort analysis over time | immutable snapshots, trends, snapshot diff, public dashboards | not available |
| Nagios and Icinga plugin | shipped | not shipped |
| MCP server for AI agents | yes | not available |
| Reproducible runs from a saved packet cache | yes | yes |

## Beyond single tests

gonemaster also tests groups of domains and tracks how they change over time.

- Batches and cohort analysis: queue thousands of domains, score them, and
  freeze the result as an immutable snapshot. Snapshots carry trends and can be
  diffed against each other, which shows whether a TLD improved over a year.
  Read-only dashboards can be published without exposing the admin surface.
  See [analysis](https://pawal.codeberg.page/gonemaster/analysis/).
- Monitoring: `gonemaster-nagios` reports a domain's delegation health to the
  monitoring system you already run. See
  [nagios](https://pawal.codeberg.page/gonemaster/nagios/).
- AI agents: gonemaster exposes testing and analysis over the Model Context
  Protocol, through the `gonemaster-mcp` bridge or an endpoint in the server.
  An agent can submit a test, search and diff runs, and roll up failure tags
  and tag values across a cohort. See
  [mcp](https://pawal.codeberg.page/gonemaster/mcp/).

## Related tools

- internet.nl tests a domain and its web and mail services against modern
  standards (IPv6, DNSSEC, TLS, DMARC) and reports conformance. gonemaster
  tests the delegation and the zone in more depth, and ignores web and mail
  configuration beyond the DNS records.
- DNSViz draws the DNSSEC chain of trust for one name, which helps when
  diagnosing a validation failure. gonemaster reports DNSSEC findings as part
  of a wider delegation test.
- OpenINTEL measures the DNS at internet scale and publishes the raw
  longitudinal data. gonemaster runs targeted tests and grades the domains you
  point it at.

## When not to use gonemaster {#when-not-to-use-gonemaster}

- Your workflow is built on the Zonemaster Backend or GUI. gonemaster has its
  own server, API, and UIs. It does not implement the Backend's JSON-RPC API
  and cannot serve as a backend for the Zonemaster GUI. If a registrar
  integration, ticketing system, or customer portal already speaks to those,
  staying with Zonemaster is less work.
- You need the reference implementation's exact behaviour. If a registry
  policy, a contract, or an acceptance test is written against Zonemaster's
  output, use Zonemaster. gonemaster's results differ in documented places.
- You want general web and email standards testing. Use internet.nl.

## Project facts

- License: code under a BSD-style license; the testcase specifications under
  `docs/specifications/` under CC BY 4.0.
- Development: <https://codeberg.org/pawal/gonemaster>. Issues, patches, CI,
  releases, and this documentation site all live there.
- Releases: continuous, built by CI for Linux, macOS, and Windows, with
  SHA-256 checksums attached to each release.
- Localisation: 12 locales in the engine message catalog and in the admin and
  public UIs.
- Try it: <https://gonemaster.evilbit.de/>, or
  `go install codeberg.org/pawal/gonemaster/cmd/gonemaster@latest` followed by
  `gonemaster example.com`.

## Questions

### Is it a fork? {#is-it-a-fork}

No. gonemaster is written from scratch in Go and shares no code with the Perl
engine. Its testcase specifications are derived from upstream's, credited as
such in the license, and have since diverged.

### Can it replace my Zonemaster installation? {#can-it-replace-zonemaster}

For command-line testing, API testing, and batch runs, yes. If your workflow
depends on the Zonemaster Backend's JSON-RPC API or on the Zonemaster GUI, no:
see [when not to use gonemaster](#when-not-to-use-gonemaster). The
[migration guide](https://pawal.codeberg.page/gonemaster/migration-from-zonemaster/)
covers flags, profiles, API calls, and output consumers.

### Do results match Zonemaster? {#do-results-match}

Largely. 73 testcases implement the same specifications, and shared findings keep
upstream's tag identifiers, so two outputs can be diffed. The differences are
documented: the fourteen extra testcases emit tags upstream has no equivalent
for, and in the cases linked above gonemaster fixes upstream behaviour that has
an issue filed against it.

### Why Go? {#why-go}

A Go build is one static binary, with no runtime or module tree to install. The
workload is almost entirely waiting on the network, which Go's concurrency
handles well, and the code parses hostile input off the wire, where a
memory-safe language helps. A second implementation also finds bugs that a
single engine does not find in itself, as the issues listed above show.

### Why Codeberg? {#why-codeberg}

Codeberg is a non-profit forge running on Free Software (Forgejo). Development,
issues, CI, releases, and this documentation site all run there.
