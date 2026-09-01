# gonemaster

gonemaster tests the DNS health of a domain. It checks the delegation, the
nameservers, zone consistency, DNSSEC, and more, and turns what it finds into
plain-language findings with severities, a numeric score, and a letter grade.
Use it to verify a zone before and after a change, to keep an eye on the
domains you are responsible for, or to measure DNS quality across thousands
of domains over time.

It ships as one test engine with a set of small tools around it: a CLI, an
HTTP server with embedded web UIs, an automation client, a Nagios plugin,
and an MCP bridge for AI agents.

Try it without installing anything: a public instance runs at
[gonemaster.evilbit.de](https://gonemaster.evilbit.de/).

New to the project?
[Why gonemaster](https://pawal.codeberg.page/gonemaster/why/) covers what it is,
how it compares with Zonemaster, and when to use something else.

## What gonemaster Tests

A test run takes a domain through a series of testcases grouped into modules:

- **basic** - does the zone exist and have a working authoritative nameserver.
- **address** - nameserver IP addresses and their reverse DNS (PTR) mappings.
- **connectivity** - UDP/TCP reachability and network (ASN and prefix) diversity.
- **consistency** - whether nameservers agree on SOA, serials, NS sets, and more.
- **delegation** - parent/child delegation: NS records, glue, and referrals.
- **dnssec** - the DNSSEC chain of trust: DS, DNSKEY, signatures, and algorithms.
- **nameserver** - nameserver behaviour and capabilities, such as EDNS handling.
- **syntax** - hostname and domain name syntax.
- **zone** - zone-level records such as SOA timers and MX.

Every finding is a tagged log message with a severity from DEBUG to CRITICAL,
and the findings are scored into a numeric result and a letter grade from A+
to F. The full inventory of testcases is in the
[specifications](https://pawal.codeberg.page/gonemaster/specifications/), and
scores and grades are described in the
[scoring documentation](https://pawal.codeberg.page/gonemaster/scoring/).

## Highlights

- **Test a domain in one command.** The `gonemaster` CLI needs no server or
  database, prints human-readable text or JSON for scripts, and translates
  findings into twelve languages.
- **Check a delegation before it goes live.** Undelegated tests take explicit
  nameserver and DS input, so a zone can be tested at a new operator before
  the parent delegation is switched.
- **Monitor zones continuously.** `gonemaster-nagios` maps finding severities
  to Nagios/Icinga service states, turning delegation health into a standard
  operational check.
- **Run it as a service.** `gonemaster-server` adds a persistent queue,
  batches, stored run history, test profiles, Prometheus metrics, and
  embedded web UIs. SQLite works out of the box; PostgreSQL and MariaDB are
  supported.
- **Compare before and after.** Two runs, or two whole batches, can be diffed
  at the finding level to confirm that a change fixed what it was meant to fix.
- **Analyze domains at scale.** Tag collections of domains, test them in
  batches, and publish read-only cohort dashboards backed by immutable
  snapshots.
- **Automate and embed.** A command-line automation client, an HTTP API, an
  MCP bridge for AI agents, and a Go engine library for direct embedding.

## Install

Prebuilt binaries for Linux, macOS, and Windows are published on the
[releases page](https://codeberg.org/pawal/gonemaster/releases).

Install the CLI with Go (1.27 or later):

```console
go install codeberg.org/pawal/gonemaster/cmd/gonemaster@latest
```

Build from source:

```console
git clone https://codeberg.org/pawal/gonemaster.git
cd gonemaster
go test ./...
go build -o gonemaster ./cmd/gonemaster
sudo install -m 0755 gonemaster /usr/local/bin/gonemaster
```

`make packages` builds Debian and RPM packages (building the embedded web UIs
requires Node.js), and `make help` lists the other targets: builds, tests,
UI builds, packaging, and documentation checks.

## Quick Start

### Test a Domain

```console
gonemaster example.com
gonemaster --score example.com
gonemaster --json --domain example.com | jq
gonemaster --module dnssec --testcase dnssec01 example.com
```

Test a zone that is not delegated yet, for example before a change of DNS
operator, by passing the nameservers (and optionally DS records) directly:

```console
gonemaster --domain example.com \
  --ns ns1.example.com/192.0.2.10 \
  --ns ns2.example.net/2001:db8::10
```

CLI documentation: [pawal.codeberg.page/gonemaster/cli](https://pawal.codeberg.page/gonemaster/cli/)

### Monitor a Zone

```console
gonemaster-nagios -H example.com -w WARNING -c ERROR
```

Works with Nagios, Icinga, Naemon, and other Nagios-compatible systems.
Plugin documentation: [docs/nagios.md](docs/nagios.md)

### Start the Server

The server embeds the admin, public, and analysis web UIs; building them
requires Node.js:

```console
make ui-build
go build -o ./gonemaster-server ./cmd/gonemaster-server
./gonemaster-server
```

For an API-only server without the embedded UIs (no Node.js required), build
with `make build-gonemaster-server-noui`.

Server documentation: [pawal.codeberg.page/gonemaster/server](https://pawal.codeberg.page/gonemaster/server/)

### Automate and Analyze

`gonemaster-client` drives a running server from the shell or from scripts:

```console
gonemaster-client jobs create --domain example.com --wait --view summary
gonemaster-client jobs batch --file domains.txt --tag tld --wait
gonemaster-client batches diff batch_before batch_after
```

Client documentation: [pawal.codeberg.page/gonemaster/client](https://pawal.codeberg.page/gonemaster/client/).
Cohort analysis and public dashboards: [pawal.codeberg.page/gonemaster/analysis](https://pawal.codeberg.page/gonemaster/analysis/).

## Documentation

Full documentation: [pawal.codeberg.page/gonemaster](https://pawal.codeberg.page/gonemaster/)

Start with the [architecture overview](https://pawal.codeberg.page/gonemaster/architecture/) for a one-sitting tour of the system: binaries, request lifecycles, data model, concurrency, security posture, and known limitations.

- [Why gonemaster](https://pawal.codeberg.page/gonemaster/why/) - what it is, how it compares with Zonemaster
- [Migrating from Zonemaster](https://pawal.codeberg.page/gonemaster/migration-from-zonemaster/) - port a CLI, API, or batch workflow
- [CLI](https://pawal.codeberg.page/gonemaster/cli/) - local test runner
- [Server](https://pawal.codeberg.page/gonemaster/server/) - HTTP server and queue
- [Client](https://pawal.codeberg.page/gonemaster/client/) - automation client
- [Nagios](docs/nagios.md) - Nagios and Icinga plugin
- [MCP](https://pawal.codeberg.page/gonemaster/mcp/) - Model Context Protocol bridge for AI agents
- [Analysis](https://pawal.codeberg.page/gonemaster/analysis/) - cohort analysis and snapshots
- [Specifications](https://pawal.codeberg.page/gonemaster/specifications/) - testcase and tag reference
- [OpenAPI](docs/openapi.yaml) - machine-readable API spec
- [Changelog](Changelog) - release history
- [pkg.go.dev](https://pkg.go.dev/codeberg.org/pawal/gonemaster/engine) - Go package docs; `engine` is the main entry point for embedding gonemaster programmatically

## Screenshots

CLI output:

![ascii animation](docs/demo.gif)

Admin UI:

![UI screenshot](docs/ui-screenshot.png)

Metrics view:

![Metrics screenshot](docs/metrics.png)

## License

gonemaster is released under a BSD-style license. See [LICENSE](LICENSE).
