# Why gonemaster

gonemaster is an open source DNS and DNSSEC test engine written in Go. This page
answers the questions that come up first: what it is, how it relates to
Zonemaster, why it exists as a separate project, and when you should reach for
something else.

## What it is

Give gonemaster a domain and it runs 83 testcases in 9 modules against the live
delegation: basic, address, connectivity, consistency, delegation, dnssec,
nameserver, syntax, and zone. Every finding is a tagged message with a severity
from DEBUG to CRITICAL, and the findings are scored into a number out of 100 and
a letter grade from A+ to F.

The engine comes with a small set of tools around it:

- `gonemaster` - the CLI. No server and no database; text, JSON, or streaming
  output.
- `gonemaster-server` - an HTTP server with a job queue, batches, stored run
  history, profiles, Prometheus metrics, and three embedded web UIs, on SQLite,
  PostgreSQL, or MariaDB.
- `gonemaster-client` - an automation client for shells and pipelines.
- `gonemaster-nagios` - a Nagios and Icinga plugin that maps finding severities
  to service states.
- `gonemaster-mcp` - a Model Context Protocol server, so an AI agent can run
  tests, search and diff runs, and roll up cohort statistics.

A public instance runs at <https://gonemaster.evilbit.de/>. Locally, one command
is enough:

```console
gonemaster example.com
```

## Why it exists, and where it stands

gonemaster began as an independent implementation of the Zonemaster testcase
specifications, in a different language. Independence is what made it quick: a
testcase can be added the week its RFC question is settled, and a fix ships in
the next tag rather than in the next release cycle.

It has since moved past its starting point. gonemaster implements ten testcases
that have no upstream counterpart, maintains its own specification corpus, and
has found and reported cases where the shared specification and the reference
implementation disagreed. For command-line, API, and batch testing, gonemaster
can replace a Zonemaster deployment today. The rest of this page is the
evidence, so the claim can be checked rather than taken.

## How it compares with Zonemaster

Figures for Zonemaster below are as of upstream v2026.1, released 29 June 2026.

### Credit where it is due

Zonemaster is the work of Internetstiftelsen (The Swedish Internet Foundation)
and AFNIC. They defined this way of testing a DNS delegation and wrote the
specification corpus gonemaster grew from. gonemaster's testcase specifications
are derived from theirs, carry their copyright alongside gonemaster's, and stay
under CC BY 4.0.

Reimplementing a specification is an unusually effective way of finding bugs in
it, and everything found this way goes back upstream:
[twenty-one issue reports](https://github.com/search?q=org%3Azonemaster+is%3Aissue+author%3Apawal+created%3A%3E%3D2026-01-01&type=issues)
across the Zonemaster repositories since January 2026, nine of them already
closed. Some of what they were.

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
- [zonemaster#1485](https://github.com/zonemaster/zonemaster/issues/1485): the
  Backend's `add_api_user` localhost check could be bypassed with a spoofed
  `X-Forwarded-For` header.

Still open:

- [zonemaster#1481](https://github.com/zonemaster/zonemaster/issues/1481): a
  proposal to add the check gonemaster ships as `dnssec21`, so that the parent's
  DS RRset signing becomes part of the shared specification too.
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

The two projects now maintain separate specification sets, and gonemaster's has
moved ahead in two ways that can be counted.

**Coverage.** gonemaster implements 83 testcases. Upstream publishes 74 testcase
specifications, one of which (`dnssec12`, DNSSEC algorithm completeness) is a
placeholder that upstream states is not yet implemented. gonemaster implements
the other 73, and adds ten testcases that have no upstream counterpart:

| Testcase | What it checks | Reference |
|---|---|---|
| `dnssec19` | DNSKEY records against known cryptographic weaknesses and blocklists of compromised keys | RFC 3110, RFC 6605, RFC 8080 |
| `dnssec20` | that the NSEC/NSEC3 apex type bitmap matches the RR types actually present | RFC 4034, RFC 5155, RFC 8198 |
| `dnssec21` | that the parent zone signs the DS RRset delegating the child | RFC 4035 |
| `nameserver16` | NSID: which servers answer with one, and what it contains | RFC 5001 |
| `nameserver17` | DNS Cookie support, and whether the Server Cookie is well formed and accepted | RFC 7873, RFC 9018 |
| `nameserver18` | Extended DNS Errors from an authoritative server, classified by info-code | RFC 8914 |
| `zone12` | CSYNC at the zone apex | RFC 7477 |
| `zone13` | that the apex SPF policy stays inside the DNS lookup limit | RFC 7208 section 4.6.4 |
| `zone14` | ZONEMD at the zone apex | RFC 8976 |
| `zone15` | CAA presence and syntax at the zone apex | RFC 8659 |

**Cadence.** Upstream ships roughly two feature releases a year: v2024.1 (July
2024), v2024.2 (December 2024), v2025.1 (June 2025), v2025.2 (December 2025),
v2026.1 (June 2026). gonemaster releases continuously from CI: 67 tags, most
recently v1.7.5 on 4 September 2026. A change proposed upstream waits for a
release cycle; in gonemaster it waits for the next tag.

### Side by side

|  | gonemaster | Zonemaster |
|---|---|---|
| Testcases | 83 implemented, 9 modules | 74 documented specifications, one of them a placeholder |
| Specification corpus | own corpus under CC BY 4.0, derived from upstream's | upstream corpus under CC BY 4.0 |
| Release cadence | continuous from CI, 67 tags, v1.7.5 | about two feature releases a year |
| Language and runtime | Go, one static binary | Perl |
| Deployment shape | one binary with the web UIs embedded, on SQLite, PostgreSQL, or MariaDB | Engine, CLI, Backend, and GUI installed separately |
| Numeric score and letter grade | 0-100 and A+ to F | not available |
| Batch testing | yes | yes, through the Backend |
| Cohort analysis over time | immutable snapshots, trends, snapshot diff, public dashboards | not available |
| Nagios and Icinga plugin | shipped | not shipped |
| MCP server for AI agents | yes | not available |
| Reproducible runs from a saved packet cache | yes | yes |

### What Zonemaster still is

The reference implementation, run as a public service by both sponsoring
registries, with more than a decade of operational history behind it and the
Backend and GUI ecosystem that many deployments are built around. Nothing on
this page is a claim about upstream's internal quality beyond the issues linked
above.

## More than an engine

Beyond testing one domain, gonemaster is built for the question "how are all of
these doing, and is it getting better?"

- **Batches and cohort analysis.** Queue thousands of domains, score them, and
  freeze the result as an immutable snapshot. Snapshots carry trends and diff
  against each other, so "did this TLD improve over the year" becomes a query,
  and read-only dashboards can be published without exposing the admin surface.
  See [analysis](https://pawal.codeberg.page/gonemaster/analysis/).
- **Monitoring.** `gonemaster-nagios` puts a domain's delegation health into the
  monitoring system you already run. See
  [nagios](https://pawal.codeberg.page/gonemaster/nagios/).
- **AI agents.** `gonemaster-mcp` exposes testing and analysis over the Model
  Context Protocol: submit a test, search and diff runs, roll up failure tags and
  tag values across a cohort. See
  [mcp](https://pawal.codeberg.page/gonemaster/mcp/).

## How it relates to the neighbours

These tools answer different questions, and using more than one is normal.

- **internet.nl** tests a domain and its web and mail services against modern
  standards - IPv6, DNSSEC, TLS, DMARC - and reports conformance. gonemaster goes
  deep on the delegation and the zone, and ignores web and mail configuration
  beyond the DNS records.
- **DNSViz** visualises the DNSSEC chain of trust for one name, and is what to
  reach for when a validation failure needs to be seen. gonemaster reports DNSSEC
  findings as part of a wider delegation test.
- **OpenINTEL** measures the DNS at internet scale and publishes the raw
  longitudinal data. gonemaster runs targeted tests and grades the domains you
  point it at.

## When not to use gonemaster {#when-not-to-use-gonemaster}

- **Your workflow is built on the Zonemaster Backend or GUI.** gonemaster has its
  own server, API, and UIs. It is not a drop-in replacement for the Backend's
  JSON-RPC API and it is not a backend for the Zonemaster GUI, so if a registrar
  integration, ticketing system, or customer portal already speaks to those,
  staying with Zonemaster is the smaller job.
- **You need the reference implementation's exact behaviour.** If a registry
  policy, a contract, or an acceptance test is written against Zonemaster's
  output, use Zonemaster. gonemaster's divergences are deliberate and documented,
  which is not the same as absent.
- **You want general web and email standards testing.** internet.nl is the right
  tool for that.

## Project facts

- **License.** Code under a BSD-style license; the testcase specifications under
  `docs/specifications/` under CC BY 4.0.
- **Development.** <https://codeberg.org/pawal/gonemaster>. Issues, patches, CI,
  releases, and this documentation site all live there.
- **Releases.** Continuous, built by CI for Linux, macOS, and Windows, with
  SHA-256 checksums attached to each release.
- **Localisation.** 12 locales in the engine message catalog and in the admin and
  public UIs.
- **Try it.** <https://gonemaster.evilbit.de/>, or
  `go install codeberg.org/pawal/gonemaster/cmd/gonemaster@latest` followed by
  `gonemaster example.com`.

## Questions people ask

### Is it a fork? {#is-it-a-fork}

No. gonemaster is a from-scratch implementation in Go and shares no code with the
Perl engine. What it inherited is the testcase specifications: they are derived
from upstream's, credited as such in the license, and have since diverged.

### Can it replace my Zonemaster installation? {#can-it-replace-zonemaster}

For command-line testing, API testing, and batch runs, yes. If your workflow
depends on the Zonemaster Backend's JSON-RPC API or on the Zonemaster GUI, no:
see [when not to use gonemaster](#when-not-to-use-gonemaster). The
[migration guide](https://pawal.codeberg.page/gonemaster/migration-from-zonemaster/)
covers the mechanics: flags, profiles, API calls, and output consumers.

### Do results match Zonemaster? {#do-results-match}

Largely. 73 testcases implement the same specifications, and shared findings keep
upstream's tag identifiers, so two outputs can be diffed. Where results differ,
the difference is deliberate and written down: the ten extra testcases emit tags
upstream has no equivalent for, and in the cases linked above gonemaster's
behaviour is a fix of upstream behaviour, with an issue filed to say so.

### Why Go? {#why-go}

One static binary to deploy, with no runtime or module tree to install. Native
concurrency for a workload that is almost entirely waiting on the network. A
memory-safe language for code that parses hostile input off the wire. And a
second implementation is worth something on its own: a category served by one
engine has no way to catch its own bugs.

### Why Codeberg? {#why-codeberg}

Codeberg is a non-profit forge running on Free Software (Forgejo), which suits a
project about open infrastructure. Development, issues, CI, releases, and this
documentation site all run there.
