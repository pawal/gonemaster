# gonemaster architecture

Last reviewed: 2026-08-11.

## 1. System overview

gonemaster tests DNS zones for delegation, configuration, and
protocol problems. A test run executes a fixed suite of testcases
against a zone and emits a structured stream of findings; each
finding carries a severity, a tag, and machine-readable arguments.
Findings aggregate into a numeric score and letter grade. The engine
is exposed through four surfaces: a local CLI (`gonemaster`), a
Nagios-compatible probe (`gonemaster-nagios`), an HTTP service with
embedded web UIs (`gonemaster-server`), and a CLI client for that
service (`gonemaster-client`).

```
   [gonemaster]   [gonemaster-nagios]   [gonemaster-client]
        |                  |                     |
        |                  |                     | HTTP
        |                  |                     v
        |                  |          [gonemaster-server] <--> [Database]
        |                  |                     |
        +------------------+---------------------+
                           |
                           v
                       [engine] --> [DNS]
```

Each binary instantiates its own engine. The engine itself has no
shared state across binaries.

## 2. Component map

gonemaster ships five binaries and three web UIs. The UIs are built
to static assets and embedded into `gonemaster-server` with
`go:embed`.

### Binaries

| Binary | Source | Role | External I/O |
|---|---|---|---|
| `gonemaster` | [cmd/gonemaster/](../cmd/gonemaster/) | Local CLI test runner. Runs the engine in-process. | DNS; local files (cache, profile). |
| `gonemaster-server` | [cmd/gonemaster-server/](../cmd/gonemaster-server/) | HTTP service. Admin and public APIs, job queue, persistence, embedded UIs. | DNS; configured database. |
| `gonemaster-client` | [cmd/gonemaster-client/](../cmd/gonemaster-client/) | CLI client for the admin API. | `gonemaster-server`. |
| `gonemaster-nagios` | [cmd/gonemaster-nagios/](../cmd/gonemaster-nagios/) | Nagios-compatible probe. Runs the engine in-process and exits with a Nagios status code. | DNS. |
| `gonemaster-mcp` | [cmd/gonemaster-mcp/](../cmd/gonemaster-mcp/) | Model Context Protocol (stdio) bridge. Forwards MCP tool calls to the admin API; runs no engine. | `gonemaster-server`. |

`gonemaster`, `gonemaster-nagios`, and `gonemaster-server` are
self-contained: each can run without the others. `gonemaster-client`
and `gonemaster-mcp` require `gonemaster-server`.

### Embedded UIs

`gonemaster-server` serves three independent Svelte 5 + Vite
frontends.

| UI | Source | Mount | Audience |
|---|---|---|---|
| Admin | [ui/](../ui/) | `/` | Trusted operators. Job, batch, domain, tag, cohort, and settings management. |
| Public | [ui-public/](../ui-public/) | `/public/` | Internet users. Single-domain test page. |
| Analysis | [analysis-ui/](../analysis-ui/) | `/analysis/` | Internet users. Read-only cohort dashboards. |

Build tags:

- `nogui` drops all three UIs. API routes are unaffected; UI routes
  return `404 ui not available`.
- `badkeys_embed` embeds the badkeys blocklist into the binary.

### HTTP surfaces

| Path | API | Audience |
|---|---|---|
| `/` | - | Admin UI. |
| `/api/v1/` | Admin | `gonemaster-client`, admin UI, scripts. |
| `/public/` | - | Public UI. |
| `/analysis/` | - | Analysis UI. |
| `/pub/api/v1/` | Public | Internet-facing. Rate-limited; scoped to public jobs and analysis reads. |

`/` and `/api/v1/` have no built-in authentication. Deployments
restrict them to a trusted network or place them behind a
reverse-proxy auth layer. `/public/`, `/analysis/`, and
`/pub/api/v1/` are designed for direct internet exposure behind a
rate-limiting reverse proxy.

Further reading:
[docs/cli/](cli/README.md),
[docs/server/](server/README.md),
[docs/client/](client/README.md),
[docs/nagios.md](nagios.md),
[docs/analysis/](analysis/README.md).

## 3. Request lifecycles

Three concrete flows through `gonemaster-server`, each shown as the
ordered steps between HTTP entry and terminal state.

### 3.1. Single-domain test (public API)

Entry: `POST /pub/api/v1/jobs` → `handlePublicCreateJob`
([server/handlers_public.go](../server/handlers_public.go)).

1. Validate and normalize the domain.
2. Create an in-memory `Job` with `status=JobQueued`.
3. Persist the job via `Store.Create`.
4. Enqueue at `PriorityNormal` via `Queue.Enqueue`.
5. A worker dequeues, acquires the engine concurrency limiter, and
   transitions the job to `JobRunning`
   ([server/worker.go](../server/worker.go)).
6. Worker calls `engine.Run`, then `Store.GraduateJob`, which writes
   a `Run` row and its `Entries` and deletes the `Job` row.

State: `JobQueued → JobRunning → JobSucceeded` (or `JobFailed`,
`JobCanceled`).

Result fetch: `GET /pub/api/v1/jobs/{publicID}/result` reads either
the still-queued `Job` or the graduated `Run` and its `Entries`.

### 3.2. Batch submission (admin API)

Entry: `POST /api/v1/jobs/batch` → `handleJobsBatch`
([server/handlers.go](../server/handlers.go)).

1. Resolve the domain list directly or via a `from_tag` lookup.
2. Create one `Job` per domain, sharing a `BatchID` and
   `Priority=PriorityBatch`.
3. Persist all jobs; enqueue each at `PriorityBatch`.
4. Persist a `Batch` row recording `DomainCount` and metadata.
5. Workers consume the batch tier only after the normal tier is
   empty ([server/queue.go](../server/queue.go)).
6. Each batch job follows the same `runJob` → `engine.Run` →
   `GraduateJob` path as a single-domain test.

State per job: `JobQueued → JobRunning → JobSucceeded`. A batch
completes when every job graduates.

Poll: `GET /api/v1/batches/{id}` aggregates the in-flight `Jobs`
and the graduated `Runs` for the batch to produce counts and the
grade distribution.

### 3.3. Cohort snapshot read (public API)

Entry: `GET /pub/api/v1/analysis/cohorts/{tag}/snapshots/{slug}/overview`
→ `handlePublicAnalysisOverview`
([server/handlers_public_analysis.go](../server/handlers_public_analysis.go)).

1. Resolve the public cohort by source tag.
2. Resolve the snapshot by slug, or fall back to the cohort's
   default snapshot.
3. Reject the request unless the snapshot has `Status=Captured` and
   `IsPublic=true`.
4. Read the pre-aggregated row from `analysis_snapshot_overviews`
   via `Store.GetSnapshotOverview`.
5. Return the aggregated payload with cache headers: immutable
   `max-age=1y` for an explicitly named captured snapshot,
   `no-cache, must-revalidate` for default-snapshot resolution.

Snapshot population is upstream of this read: batch jobs graduate
into `Runs`, the worker projects each run into fact tables via
`analysis.ProjectRun`, and a periodic loop materializes overview
rows via `analysis.CaptureCompletedSnapshots`.

Further reading: [docs/server/](server/README.md),
[docs/analysis/](analysis/README.md).

## 4. Data model

`gonemaster-server` stores two layers of persistent entities. Both
are defined in [server/models.go](../server/models.go) and persisted
across backends through the `Store` interface.

### Core entities

| Entity | Purpose | Cardinality |
|---|---|---|
| `Job` | In-flight or queued test request. Graduates into a `Run` when complete. | Many per domain over time. |
| `Run` | Immutable record of a completed test. Carries score, grade, and severity totals. | One per completed `Job`. |
| `Entry` | One log line emitted during a run; has severity, tag, module, testcase, and JSON args. | Many per `Run`. |
| `Domain` | Persistent domain registry. Stores the latest run's denormalized result so common filters do not scan all historical runs. | One per distinct tested domain. |
| `Tag` | User-defined domain collection. Many-to-many with `Domain`. | One per collection. |
| `Batch` | Metadata for one batch submission. Groups its `Jobs` (and later their `Runs`) by a shared `BatchID`. | One per batch submission. |
| `Profile` | Named server-stored test configuration. Referenced by `Job` and `Run` via `ProfileID`. | Many per server. |

`Domain` and `Tag` records are shared metadata, not owned by any one
batch. Deleting a batch does not remove the domains it tested.

### Analysis layer

A second layer sits above `Run` for cohort analysis.

| Entity | Purpose | Cardinality |
|---|---|---|
| `AnalysisCohort` | Admin overlay on one source `Tag`: label, public/analysis flags, default snapshot, sort order. | One per published cohort. |
| `AnalysisCohortSnapshot` | Point-in-time view of a cohort, backed by exactly one snapshot-intent `Batch`. | Many per cohort. |
| `analysis_run_*` projections | Per-run facts (domain summary, tags, nameservers, addresses, ASNs, prefixes), materialized into each cohort by `analysis.ProjectRun`. | Many per (run, cohort). |
| `analysis_snapshot_*` views | Per-snapshot pre-aggregated rows, materialized by `analysis.CaptureCompletedSnapshots`. Public read paths serve these directly. | Many per snapshot. |

Projection and snapshot capture are read-optimized materializations:
per-run projection lets cohort listings avoid scanning entries;
snapshot capture lets public reads avoid scanning facts.

### Lifecycle

```
   POST --> Job --graduate--> Run + Entries
                                   |
                                   v  per (run, cohort)
                       analysis_run_* projections
                                   |
                                   v  on snapshot-intent batch complete
                  Snapshot + analysis_snapshot_* views
```

Further reading: [docs/server/database.md](server/database.md),
[docs/analysis/](analysis/README.md).

## 5. Persistence backends

`gonemaster-server` supports four storage backends, selected at
startup with `--db-driver` or `GONEMASTER_DB_DRIVER`.

| Backend | Use when | Concurrent writes | Connection pool |
|---|---|---|---|
| `memory` | Development, tests, short-lived scripts. State is lost on restart. | n/a | n/a |
| `sqlite` | One server with small or medium data. | Serialized (single writer). | 1 open. |
| `postgres` | High-volume analysis or heavy JSON-args querying. | Concurrent. | 25 open, 5 idle, 5-minute lifetime. |
| `mariadb` | Existing MariaDB or MySQL infrastructure. | Concurrent. | 25 open, 5 idle, 5-minute lifetime. |

The three SQL drivers share schema migrations; a dialect layer
([server/store_sql_dialect.go](../server/store_sql_dialect.go))
translates one statement template into per-driver SQL.

### Startup recovery

On a persistent backend, the server performs three steps before
accepting requests:

1. Run pending schema migrations.
2. Mark any `Job` left in `running` state at the last shutdown as
   `failed`.
3. Re-enqueue any `Job` still in `queued` state.

`memory` skips this; the queue starts empty.

### Retention

`--db-retention-days N` (or `database.retention_days`) runs an
hourly purge of terminal `Job` and `Run` rows older than `N` days.
`0` keeps results forever. Queued, running, and paused jobs are
never purged automatically. Manual batch deletion is a separate
operation and ignores the retention window.

Further reading: [docs/server/database.md](server/database.md),
[docs/server/database-setup.md](server/database-setup.md).

## 6. Concurrency model

`gonemaster-server` runs tests on a fixed worker pool fed from a
two-tier priority queue. The engine itself is parallel-safe and is
the same code used by `gonemaster` and `gonemaster-nagios`.

### Engine isolation

`engine.Run` is re-entrant. Each call constructs isolated per-run
state from its arguments; nothing is shared between concurrent runs
through global variables. A binary may call `engine.Run` from many
goroutines without external synchronization.

Per-run backoff state - the query cache, the error cache and the
reachability cache that suppresses queries to unreachable addresses -
belongs to the `nameserver.CacheStore` the run was given. Any sharing
across runs is therefore something the caller opts into by reusing a
store, not something the engine does behind its back. The server does
reuse warmed query data across jobs through the nameserver hot cache;
derived failure state is never shared that way.

### Worker pool

`gonemaster-server` starts `WorkerCount` worker goroutines on
startup (default `16`,
[server/config.go](../server/config.go)). Each worker loops:

1. Dequeue the next job.
2. Mark the job `running`; register a per-job cancel function.
3. Acquire the engine limiter if one is configured.
4. Call `engine.Run`.
5. Graduate the job to a `Run` plus its `Entries`.

Workers can be resized without restarting the server. Scaling down
cancels the last `N` worker contexts and they exit after their
current job ([server/worker.go](../server/worker.go)).

### Priority queue

The queue has two tiers ([server/queue.go](../server/queue.go)):

- `PriorityNormal` - interactive single-domain submits.
- `PriorityBatch` - batch and tag-sweep submits.

Workers always drain `PriorityNormal` before `PriorityBatch`. FIFO
within each tier. Interactive requests are not starved by an active
batch.

### Engine concurrency limit

`MaxConcurrentJobs` caps the number of concurrent `engine.Run`
calls when set above zero (default `0`: no extra cap; only the
worker pool bounds concurrency). Useful when the engine is
bottlenecked on something other than worker goroutines, e.g.
resolver capacity or DNS rate budgets.

### Cancellation

The server keeps a map of `jobID → context.CancelFunc`
([server/cancel.go](../server/cancel.go)). Canceling a job calls
the function, which cancels the per-run context. `engine.Run`
returns promptly; the worker records the terminal state as
`canceled`.

Further reading: [docs/server/configuration.md](server/configuration.md),
[docs/server/performance.md](server/performance.md).

## 7. Build and distribution

Build is driven by the [Makefile](../Makefile). CI runs on
[Woodpecker](../.woodpecker/).

### Build targets

| Target | Output | Notes |
|---|---|---|
| `make build` | All four binaries into `bin/`. | Builds the embedded UIs first. |
| `make build-gonemaster-server` | `bin/gonemaster-server` with all three UIs embedded. | Default. Requires Node >= 20 and npm >= 9. |
| `make build-gonemaster-server-noui` | `bin/gonemaster-server` without UIs. | Uses build tag `nogui`. |
| `make build-gonemaster[-server]-badkeys-embed` | Variant with the badkeys blocklist baked in. | Uses build tag `badkeys_embed`. Downloads the blocklist via `badkeys-update-embed`. |
| `make ui-build` | `dist/` assets in each of `server/{ui,public,analysisui}/` for `go:embed`. | Three independent Vite builds. |

### Build tags

- `nogui` drops all three embedded UIs. API routes are unchanged;
  UI routes return `404 ui not available`. Used when Node is not
  available in the build environment.
- `badkeys_embed` embeds the gzipped badkeys blocklist into the
  binary. Without the tag, the blocklist is read from
  `share/badkeys/` at runtime.

The tags compose: `-tags "nogui badkeys_embed"` is a valid build.

### CI pipelines

[.woodpecker/](../.woodpecker/) defines two workflows, one per file.
Woodpecker loads every `.yml` file in that directory and gates each on
its own `when` block.

Default pipeline (every push and pull request):

1. `ui_checks` - test and build all three frontends, then
   `make spec-check-node`. All three must be built: the Go embeds
   read `server/*/dist`, and a missing one silently skips the
   public SSR and analysis-UI CSP tests.
2. `static_checks` - `make fmt-check`, `make architecture-check`,
   and `make spec-check-go`.
3. `go_test` - `go test ./...`.
4. `race_test` - `make race-ci`, the engine tree plus the
   concurrency-sensitive server tests under `-race`.
5. `integration_test` - server tests against PostgreSQL 17 and
   MariaDB 11 services started in-pipeline.
6. `build_server_with_ui` - confirm `gonemaster-server` builds with
   UI embedding.
7. `deploy_pages` (main branch only) - build the Hugo docs site and
   force-push to `codeberg.org/pawal/gonemaster:pages`.

Every Go step points `GOCACHE` and `GOMODCACHE` at the shared
workspace volume, so steps that start later reuse the compiled
dependency graph instead of rebuilding it.

A second workflow for release artifacts is declared in
[.woodpecker/release.yml](../.woodpecker/release.yml) but has not yet
been exercised; see [Chapter 15](#15-known-limitations).

Further reading: [Makefile](../Makefile),
[.woodpecker/ci.yml](../.woodpecker/ci.yml).

## 8. Configuration and deployment

### Configuration sources

Config is applied with this precedence (later wins):

1. Built-in defaults.
2. JSON config file (`--config PATH`).
3. `GONEMASTER_*` environment variables.
4. Command-line flags.

`gonemaster-server --dump-config` prints the effective config and
exits.

### Key settings

The full settings catalog lives at
[docs/server/configuration.md](server/configuration.md). The
architecturally load-bearing sections:

| Section | Examples |
|---|---|
| Listener | `listen_addr`, `read_timeout`, `write_timeout`, `idle_timeout`, `max_body_size`. |
| Workers | `worker_count`, `max_concurrent_jobs`. |
| Database | `database.driver`, `database.dsn`, `database.retention_days`. |
| Reverse proxy | `trusted_proxy_cidrs`, `public_url`. |
| Public API | `public_api.rate_limit_*`, `public_api.allow_private_undelegated_ip`. |
| Engine defaults | `profile_path`, `min_level`, resolver overrides. |

Database DSNs belong in environment variables
(`GONEMASTER_DB_DSN`); everything else fits in the JSON config
file.

### Deployment topologies

`gonemaster-server` is a single binary. It does not terminate TLS,
does not authenticate admin paths, and binds to one listener.

```
   Single host (dev):
     user -> gonemaster-server -> SQLite

   Trusted network:
     browser --HTTPS--> proxy --HTTP--> gonemaster-server -> Postgres/MariaDB

   Internet-facing:
     internet --HTTPS--> proxy --HTTP--> gonemaster-server -> DB
     (proxy adds TLS, rate-limit, ACL; forwards only /public/, /analysis/, /pub/api/v1/)
```

Reverse-proxy expectations:

- TLS terminates upstream; `gonemaster-server` speaks plain HTTP.
- The proxy sets `X-Forwarded-For`; `trusted_proxy_cidrs` lists the
  proxy IPs whose `X-Forwarded-For` is honored.
- For internet-facing deployments, restrict `/` and `/api/v1/` at
  the proxy.
- Rate limiting can run at the proxy or at the server
  (`public_api.rate_limit_*`); they compose.

### Profiles

Profile resolution has two layers:

- A process-wide base profile, built from the engine default plus
  `profile_path`.
- Stored profiles in the database. Jobs, batches, public profile
  dropdowns, and tag defaults reference one by ID.

Stored profiles are sparse: they record only the overrides above
the base profile. Each carries the engine schema version it was
last edited against; the admin UI flags profiles that need review
after an engine update.

Further reading: [docs/server/configuration.md](server/configuration.md),
[docs/server/public-api-and-proxy.md](server/public-api-and-proxy.md),
[docs/server/operations.md](server/operations.md).

## 9. Security posture

### Authentication

`/` and `/api/v1/` have no built-in authentication.
`gonemaster-server` expects either a reverse proxy to enforce
authentication on those routes or the listener to be bound to a
private interface. Public routes (`/public/`, `/analysis/`,
`/pub/api/v1/`) are designed to require no authentication.

### Content Security Policy

Three CSP policies are applied per route in
[server/server.go](../server/server.go):

| Route prefix | Policy summary |
|---|---|
| `/api/v1/`, `/pub/api/v1/` | `default-src 'none'`. API responses serve no UI; nothing should load from them. |
| `/analysis/` | Includes `script-src 'self' 'unsafe-inline'` for SvelteKit's bootstrap script and `style-src 'self' 'unsafe-hashes' sha256-...` for the SvelteKit announcer. |
| `/`, `/public/`, other UI/static | `default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'`. No `'unsafe-inline'`. |

[server/middleware_test.go](../server/middleware_test.go) asserts
that the admin and public CSP must not regress to include
`'unsafe-inline'`. Inline `style` attributes and Svelte `style:`
directives in those UIs are CSP violations and rejected in review.

### Public API restrictions

The public API
([docs/server/public-api-and-proxy.md](server/public-api-and-proxy.md))
is shaped for safe internet exposure:

- Job lookups use opaque public IDs only; internal job and run IDs
  never leave the public surface.
- `POST /pub/api/v1/jobs` accepts `profile_id` only when the stored
  profile is marked public.
- `profile_overrides` are rejected: public users cannot inject
  arbitrary resolver settings.
- `public_api.allow_private_undelegated_ip` defaults to `false`,
  blocking loopback, link-local, private, CGNAT, multicast, and
  broadcast IPs as undelegated NS targets.

### Rate limiting

Public API rate limiting is **off by default** and is required for
internet exposure. Configure with `public_api.rate_limit_enabled`,
`rate_limit_max`, and `rate_limit_window`. The limit applies to
`POST /pub/api/v1/jobs`, keyed by client IP after
`X-Forwarded-For` resolution.

### Trusted proxies

`trusted_proxy_cidrs` lists the CIDRs of reverse proxies allowed to
set `X-Forwarded-For`, `X-Forwarded-Host`, and `X-Forwarded-Proto`.
The default is empty: with no trusted proxies the server removes all
three and uses `RemoteAddr`, `Host`, and the connection scheme. Set
this to the proxy's IP or CIDR before relying on rate limiting,
per-client logs, or canonical URL auto-detection.

### Secrets

Database DSNs and CI release tokens are passed via environment
variables, never via the JSON config file:

- `GONEMASTER_DB_DSN` carries the database connection string.
- `CODEBERG_PAGES_TOKEN` and `CODEBERG_RELEASE_TOKEN` are injected
  into CI publish steps
  ([.woodpecker/](../.woodpecker/)).

There are no API keys to manage; access control to admin routes is
host-level.

### Supply chain

The badkeys blocklist (compromised public-key fingerprints) is
updated by an explicit operator step, not as part of normal
builds:

```sh
make badkeys-update         # fetch into share/badkeys/
make badkeys-update-embed   # additionally gzip for embedded builds
```

Builds without the `badkeys_embed` tag read the blocklist from
`share/badkeys/` at runtime. The CI pipeline does not refresh the
blocklist on its own.

Further reading:
[docs/server/public-api-and-proxy.md](server/public-api-and-proxy.md).

## 10. Observability

`gonemaster-server` exposes three observability surfaces: a health
probe, a metrics endpoint with JSON and Prometheus formats, and
standard-output logs.

### Health probe

```
GET /api/v1/healthz -> 200 {"status": "ok"}
```

Returns 200 unconditionally when the server is up. There is no
separate readiness probe; the same endpoint serves both checks.

### Metrics

```
GET /api/v1/metrics                # JSON snapshot
GET /api/v1/metrics?format=prom    # Prometheus exposition
```

The JSON snapshot has seven sections: `health`, `jobs`, `api`,
`quality`, `insights`, `trends`, and metadata
(`schema_version`, `generated_at`). Common fields:

- `health.queue_depth`, `health.in_flight_jobs`.
- `jobs.completed_total`.
- `quality.outcomes.{success_rate, failed_rate, failed_total}`.
- `quality.severity.totals` per severity level.
- `trends.windows.{1h, 6h, 24h, 48h}.points[]`.

The Prometheus form exposes low-cardinality counters and gauges
under the `gonemaster_` prefix: server gauges (build, worker
counts, queue depth, in-flight), DNS counters, job lifecycle
counters and status gauges, API request counters and latency
histograms, job-duration histograms, and locale usage.
High-cardinality data (insight tables, trend point series) and
precomputed ratios are JSON-only; Prometheus consumers derive
ratios from counters and histograms.

Both formats share a 1-second response cache keyed by query
parameters.

### Logs

`gonemaster-server` writes plain-text logs to stdout via Go's
standard `log` package. Coverage includes server lifecycle, worker
and queue events, API errors, and panic recovery (with stack). Logs
are not JSON-structured.

Further reading: [docs/server/metrics.md](server/metrics.md).

## 11. Testing strategy

Tests run at three levels: Go unit and integration tests, frontend
tests, and spec-coherency checks. CI
([.woodpecker/ci.yml](../.woodpecker/ci.yml)) runs all of them on every
commit.

### Layers

| Layer | Target | Scope |
|---|---|---|
| Go unit / integration | `make test-go` | All Go packages, `go test ./...`. In-memory store by default. |
| Cross-driver integration | `make test-integration` | `server/...` tests against SQLite, PostgreSQL 17, and MariaDB 11 (Docker Compose). |
| Race detector | `make race-ci` | Runs in CI over `./engine/...`, `./scoring/...` and the shared-store server tests. `make race` covers the full tree and is run periodically. |
| Vet | `make vet` | `go vet ./...`. |
| Admin and public UIs | `make ui-test`, `make ui-public-test` | `npm test` in each frontend. |
| Analysis UI | `make ui-analysis-test` | Run separately; not bundled into `make test`. |
| CSP regression | `make ui-csp-check` | Grep for inline `style=...` attributes and `style:` directives in admin and public UI `.svelte` sources. |
| Spec coherency | `make spec-check` | Four substeps (see below). |

`make test` is the canonical "did I break something" target. It
runs `ui-test`, `ui-public-test`, `test-go`, `spec-check`, and
`ui-csp-check` in that order.

### Spec coherency

`make spec-check` runs six substeps; any drift is a CI failure,
not a warning:

1. `spec-validate` - canonical testcase specs match implementation
   metadata (name, module, severity floors).
2. `spec-check-tags` - per-module tag catalog markdown files match
   the current tag registry.
3. `spec-check-log-args` - the log-args inventory regenerated
   from `appendLog*` call sites equals the committed one, and the
   coherency guardrails hold. Refresh with
   `make spec-export-log-args`.
4. `spec-check-i18n-placeholders` - i18n template placeholders are
   on the allowlist; no legacy placeholders sneak in.
5. `spec-check-testcase-descriptions` - the site testcase
   description data file matches the specs.
6. `spec-check-ui-explanations` - `ui-public` `en.json` matches the
   ui-explanations markdown.

The first five need only Go and are grouped as `spec-check-go`; the
last needs node and is `spec-check-node`. CI runs each half in the
image that has the toolchain, so both stay reachable from the one
`make spec-check` definition rather than being restated in
[.woodpecker/ci.yml](../.woodpecker/ci.yml).

Each substep is runnable in isolation for fix-and-recheck loops.

### Cross-driver integration

`make test-integration` starts PostgreSQL 17 and MariaDB 11 in
Docker Compose, points the test suite at them via
`TEST_POSTGRES_DSN` and `TEST_MARIADB_DSN`, and runs the
`server/...` test packages against all three drivers (SQLite is
in-process and always runs). The CI pipeline mirrors this with
services declared in [.woodpecker/ci.yml](../.woodpecker/ci.yml).

Further reading:
[docs/dev.md](dev.md),
[docs/specifications/log-args-coherency.md](specifications/log-args-coherency.md).

## 12. Specification and i18n discipline

Testcase metadata is partially generated and CI-enforced to stay
coherent with the code. This chapter summarises what is generated,
where it lives, and how drift is caught.

### What lives where

| Location | Contents | Source of truth |
|---|---|---|
| `engine/test/` | Testcase implementation: `appendLog*` calls, severity floors, module structure. | Runtime. |
| [docs/specifications/tests/](specifications/tests/README.md) | Per-testcase specs: algorithm, emitted tags, arguments, severity, upstream differences. | Documentation. |
| [docs/specifications/tags/](specifications/tags/README.md) | Per-module tag catalogs: each tag's severity and i18n coverage. | Generated from code. |
| [implemented-testcases.md](specifications/implemented-testcases.md) | Authoritative list of implemented testcases. | Generated. |
| [possible-tags-by-testcase.md](specifications/possible-tags-by-testcase.md) | All possible tags per testcase. | Generated. |
| [log-args-inventory.md](specifications/log-args-inventory.md) | Every `entry.args` key emitted, its tag, value shape, and producer file. | Generated. |
| `share/lang/*.po` | Gettext-style translations for log entry messages. | Manual. |
| [known-intentional-gaps.md](specifications/known-intentional-gaps.md) | Upstream testcases intentionally not implemented, with rationale. | Manual. |
| [known-behavior-divergences.md](specifications/known-behavior-divergences.md) | Behavior divergences observed during investigation. | Manual. |

### Generation tools

Under [tools/specifications/](../tools/specifications/) and
[tools/i18n/](../tools/i18n/):

- `export-implemented` rebuilds the implemented-testcases
  inventory.
- `generate-tag-catalog` rebuilds the per-module tag catalog
  markdown.
- `export-log-args` rebuilds the log-args inventory from the
  engine's `appendLog*` call sites.
- `validate` cross-checks canonical specs against implementation
  metadata.
- `i18n/check-placeholders` validates that template placeholders
  match the allowlist.

`make spec-export` runs the refresh path; `make spec-check` runs
the verification path used by CI.

### Enforcement

CI runs `make spec-check-go` and `make spec-check-node` on every
commit ([.woodpecker/ci.yml](../.woodpecker/ci.yml)), which together are
`make spec-check`. The six substeps listed in
[Chapter 11](#11-testing-strategy) apply. Drift in any of them is a
build failure.

After any change to `appendLog*`, message tags, or testcase
metadata, the change author runs `make spec-check` locally before
pushing.

### i18n discipline

Log messages are localised through `share/lang/*.po` gettext-style
catalogs (`da`, `es`, `fi`, `fr`, `ja`, `nb`, `sl`, `sv`).
Templates use a fixed set of placeholders. New placeholders are
rejected unless added to the allowlist consumed by
`tools/i18n/check-placeholders`. This prevents one-off translator
syntax from leaking into the runtime contract.

Further reading:
[docs/specifications/](specifications/README.md),
[docs/specifications/log-args-coherency.md](specifications/log-args-coherency.md).

## 13. IP and licensing boundary

### Project license

`gonemaster` is released under the BSD-2-Clause license
([LICENSE](../LICENSE)). Copyright is held by Patrik Wallström,
the Swedish Internet Foundation, and AFNIC. The license file
records contributor copyright on incorporated upstream code.

This chapter is the **public** licensing boundary only. Commercial
extensions, hosted-service architecture, and customer-specific
integrations do not appear in this document.

### Direct Go dependencies

| Module | Role |
|---|---|
| `codeberg.org/miekg/dns` | DNS protocol library; the wire format used by the engine. |
| `github.com/go-sql-driver/mysql` | MariaDB / MySQL driver. |
| `github.com/lib/pq` | PostgreSQL driver. |
| `modernc.org/sqlite` | Pure-Go SQLite driver. |
| `github.com/ulikunitz/xz` | xz / LZMA compression for badkeys data. |
| `github.com/cloudflare/circl` | Ed448 signature verification (DNSSEC algorithm 16), which the standard library does not provide. |
| `golang.org/x/{net,sync,text}` | Go standard-library extensions: networking, sync, i18n / IDNA. |

All direct dependencies are permissive OSS (BSD / MIT / MPL-2.0
family). Indirect dependencies are pinned in
[go.sum](../go.sum). An authoritative third-party-license
inventory is open work (see [Chapter 15](#15-known-limitations)).

### Frontend dependencies

The three embedded UIs are Svelte 5 + Vite projects. JavaScript
dependencies are pinned per UI in
[ui/package-lock.json](../ui/package-lock.json),
[ui-public/package-lock.json](../ui-public/package-lock.json),
and
[analysis-ui/package-lock.json](../analysis-ui/package-lock.json).
Major direct dependencies are Svelte (MIT), Vite (MIT), and
SvelteKit (MIT, analysis UI only).

### Data assets

| Path | Source | License / status |
|---|---|---|
| `share/named.root` | IANA root hints, from [IANA Root Files](https://www.iana.org/domains/root/files). | Public-domain reference. |
| `share/iana-ipv*-special-registry.csv` | IANA special-purpose address registries. | Public-domain reference. |
| `share/badkeys/` | Blocklist of compromised public-key fingerprints. Refreshed via [tools/badkeys-update/](../tools/badkeys-update/). | Inherits the upstream badkeys project licence; operator-driven refresh. |

Further reading: [LICENSE](../LICENSE), [go.mod](../go.mod).

## 14. External dependencies and vendors

`gonemaster` depends on external infrastructure only for builds,
distribution, and runtime DNS testing. Hosting choices for any
hosted offering are not part of this document.

### Runtime DNS targets

The engine performs delegation tests by querying the public DNS
hierarchy directly:

- Root zone servers, enumerated in
  [share/named.root](../share/named.root), taken from
  [IANA Root Files](https://www.iana.org/domains/root/files). The
  file ships with the binary; operators refresh it manually when
  IANA publishes updates.
- TLD and authoritative servers for the zone under test.
- The operator's local resolver, only when explicitly configured
  as a fallback.

No DNS data is sent to a vendor-controlled resolver as part of
normal operation.

### Build and CI vendors

| Vendor | Purpose |
|---|---|
| Codeberg | Source hosting (`codeberg.org/pawal/gonemaster`), CI (Woodpecker), docs site (Codeberg Pages). |
| Go module proxy (`proxy.golang.org`) | Build-time fetch of Go dependencies. |
| npm registry (`registry.npmjs.org`) | Build-time fetch of frontend dependencies. |
| Docker Hub | Build-time pulls of `postgres`, `mariadb`, `golang`, `node`, and `alpine` images for CI services. |

The release-artifact distribution channel is open work and not
yet described here; see [Chapter 15](#15-known-limitations).

### Operator-driven external fetches

| Source | Mechanism | Frequency |
|---|---|---|
| badkeys blocklist | [tools/badkeys-update/](../tools/badkeys-update/) | Explicit operator step. |
| IANA registries (`named.root` from [IANA Root Files](https://www.iana.org/domains/root/files), special-purpose CSVs) | Manual replacement of files in `share/`. | When IANA publishes updates. |

Further reading:
[.woodpecker/](../.woodpecker/),
[go.mod](../go.mod).

## 15. Known limitations

This chapter records what is open, mitigated but not yet fixed,
or shaped by pragmatic compromise.

### Open engineering work

| Area | State | Reference |
|---|---|---|
| Built-in admin authentication | Not implemented. Deployments rely on a reverse proxy or network isolation. | [Chapter 9](#9-security-posture). |
| Structured (JSON) logs | Not implemented. Logs are plain text from Go's `log` package. | [Chapter 10](#10-observability). |
| Release artifact pipeline | Declared in [.woodpecker/release.yml](../.woodpecker/release.yml) but not yet exercised; no `v*` tag has been published. Artifacts are per-target `.tar.gz` archives plus `.deb` and `.rpm` packages attached to the Codeberg release; package signing is deferred. | [Chapter 7](#7-build-and-distribution), [Chapter 14](#14-external-dependencies-and-vendors). |
| Generated third-party-licence inventory | Not generated. Direct dependencies are known; an indirect-dependency licence report is open work. | [Chapter 13](#13-ip-and-licensing-boundary). |

### Testcase coverage

| Item | State | Source of truth |
|---|---|---|
| Intentional upstream gaps | `dnssec12` (scope-deferred) and `nameserver14` (upstream numbering hole). | [docs/specifications/known-intentional-gaps.md](specifications/known-intentional-gaps.md). |
| Behaviour divergences from upstream | Several open and tracked (zone07, nameserver13, recursor cache and in-domain glue handling). | [docs/specifications/known-behavior-divergences.md](specifications/known-behavior-divergences.md). |

These files are the source of truth for missing or divergent
testcase behaviour.

### Operational risk areas

- **Public API rate limiting is off by default**
  ([Chapter 9](#9-security-posture)). Internet-facing deployments
  that forget to enable it can be saturated by a single client.
- **`trusted_proxy_cidrs` defaults to empty**. Without it,
  per-client rate limiting and access logs key on the reverse
  proxy's address, not the originating client.
- **No built-in backup tooling**. Database backup is the
  operator's responsibility; the server does not snapshot itself.
- **`memory` backend loses in-flight jobs on restart**. Use a
  persistent backend (SQLite, PostgreSQL, MariaDB) outside of
  development.

### Not yet documented

Topics that warrant their own chapters as the system grows:

- **Threat model**. Tracked separately;
  `docs/security/threat-model.md` will land alongside the next
  security review.
- **Performance benchmarks**. Some operational guidance lives in
  [docs/server/performance.md](server/performance.md); SLO-style
  numbers do not exist yet.

## 16. Glossary

**Batch.** A group of jobs submitted together, often for the domains
in a tag.

**Cohort.** A curated public dataset backed by one source tag.

**Entry.** A single log line emitted during a run; carries a
severity, a tag, and machine-readable arguments.

**Grade.** A letter (A+ through F) derived from the score.

**Job.** A queued test request in `gonemaster-server`. Graduates into
a run when complete.

**Profile.** A configuration document that controls engine behavior:
which modules and testcases run, resolver settings, severity
overrides.

**Run.** An immutable record of a completed test, with its entries
and metadata.

**Score.** A numeric value from 0 to 100 derived from a run's
entries.

**Severity.** One of `DEBUG3`, `DEBUG2`, `DEBUG`, `INFO`, `NOTICE`,
`WARNING`, `ERROR`, `CRITICAL`.

**Snapshot.** An immutable view of a cohort, captured from one
snapshot-intent batch.

**Tag.** Two unrelated namespaces. (1) In `engine/` and run output:
the identifier of a kind of entry, e.g. `NS_SAME_IP`. (2) In the
admin UI and analysis: a user-defined named collection of domains.

**Testcase.** A single named test executed during a run, e.g.
`dnssec01`. Testcases are grouped into modules.

Further reading: [docs/scoring.md](scoring.md),
[docs/analysis/](analysis/README.md),
[docs/specifications/](specifications/).

## 17. Appendix: Directory map

Top-level directories tracked in the repository.

| Path | Contents |
|---|---|
| [cmd/](../cmd/) | Binary entry points: `gonemaster`, `gonemaster-server`, `gonemaster-client`, `gonemaster-nagios`, `gonemaster-mcp`. |
| [engine/](../engine/) | DNS test engine. Parallel-safe; each `engine.Run` builds isolated per-run state. |
| [server/](../server/) | HTTP handlers, job queue, batches, snapshots, persistence drivers. |
| [scoring/](../scoring/) | Score and grade computation from run entries. |
| [share/](../share/) | Embedded assets: default profile, named.root, IANA registries, translations, badkeys data. |
| [tools/](../tools/) | Code-generation and spec tooling: testcase metadata, log-args inventory, i18n placeholders, badkeys-update. |
| [ui/](../ui/) | Admin UI source (Svelte 5 + Vite). Embedded into `gonemaster-server`. |
| [ui-public/](../ui-public/) | Public UI source. Embedded. |
| [analysis-ui/](../analysis-ui/) | Cohort analysis UI source. Embedded. |
| [site/](../site/) | Hugo source for the documentation site, using the Relearn theme. Mounts `docs/` as content. |
| [docs/](../docs/) | User and developer documentation. Source of truth for the docs site. |

Build outputs (`bin/`, embedded `dist/` directories) are gitignored
and not part of the source tree.
