# Documentation

## Getting Started

| If you want to… | Start here |
|---|---|
| Run DNS checks from the command line | [cli.md](cli.md) |
| Stand up the HTTP server | [server.md](server.md) |
| Set up PostgreSQL, MariaDB, or SQLite | [database-setup.md](database-setup.md) |
| Integrate with Nagios / Icinga | [nagios.md](nagios.md) |

---

## User Guides

### [cli.md](cli.md)
Running `gonemaster` and `gonemaster-client` from the command line — flags,
output formats (text, JSON, JSON-stream), undelegated testing, and packet
cache recording/replay with `--save` / `--restore`.

### [server.md](server.md)
Configuring and operating `gonemaster-server` — DSN formats, connection pool
tuning, all REST API endpoints (jobs, batches, queue management, locale
discovery, metrics), and result localization via `?locale=`.

### [database-setup.md](database-setup.md)
Choosing and setting up a storage backend (SQLite, PostgreSQL, MariaDB/MySQL)
— installation, recommended settings, connection strings, backup procedures,
and when to use each backend.

### [nagios.md](nagios.md)
The `gonemaster-nagios` plugin — exit codes, `--testcase` / `--module` filters,
undelegated nameserver testing with `--ns` / `--ds`, RRSIG expiry warnings with
`--rrsig-warn-days`, and Icinga2 service examples.

### [metrics.md](metrics.md)
Prometheus metrics exposed at `/metrics` — job counts by status, queue depth,
queue pause state, worker utilisation, purge stats, and HTTP request histograms.

### [scoring.md](scoring.md)
How gonemaster computes numeric quality scores (0–100) and letter grades
(A+ through F) from test log entries — grade thresholds, penalty weights per
severity, and the rationale behind the model.

### [data-analysis.md](data-analysis.md)
Using gonemaster as a bulk DNS analysis platform — running large domain sets
via the batch API, tracking results over time, and querying data through the
REST API or directly via SQL.

### [analysis-ui.md](analysis-ui.md)
The public analysis UI at `/analysis/` — creating tags, wiring them to
cohorts, running batches, and exposing materialized per-cohort results to
end users.

---

## Reference

### [cache-format.md](cache-format.md)
On-disk format for packet cache files produced by `--save` and consumed by
`--restore` — JSON schema, checksum contract, cache kinds (nameserver, recursor,
ASN), and strict vs. lenient parsing behaviour.

### [MIGRATION-1.1.md](MIGRATION-1.1.md)
Upgrade guide for consumers of gonemaster JSON output migrating from pre-1.1
to the v1.1 log-args contract — what changed in `raw.entries[].args`, CLI JSON
flags, and server result payloads.

### [openapi.yaml](openapi.yaml)
OpenAPI 3.0 specification for the full `gonemaster-server` REST API.

---

## Developer

### [dev.md](dev.md)
Calling the engine directly from Go — the `RunRequest` struct, pre-seeding
nameserver/recursor/ASN caches, running test modules or individual test cases,
and interpreting log entries.

### [specifications/](specifications/README.md)
Canonical per-testcase specifications — exact behaviour, emitted tags, RFC
references, and the workflow for keeping documentation aligned with code.
