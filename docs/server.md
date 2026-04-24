# Gonemaster Server

This page is kept for older links. The server documentation is now split into
focused pages under [server/](server/README.md).

## Start Here

- [Server overview](server/README.md): surfaces, lifecycle, build, quick start.
- [Configuration](server/configuration.md): flags, environment variables,
  config files, profiles, localization, and result-display settings.
- [Database](server/database.md): backend selection, DSNs, connection pools,
  migration behavior, retention, and backups.
- [Operations](server/operations.md): jobs, batches, queue controls, purge,
  batch deletion, health checks, and metrics entry points.
- [Public API and reverse proxy](server/public-api-and-proxy.md): safe public
  exposure, public job IDs, rate limiting, and proxy examples.
- [Performance](server/performance.md): workers, concurrency, hot-cache,
  fast-fail, and resolver tuning.
- [Web interfaces](server/ui.md): admin UI, public test UI, and public analysis
  UI.

## Database Backends

The server can store jobs, batches, cohort definitions, snapshots, and analysis
projections in these database backends:

| Backend | Typical DSN |
|---|---|
| `sqlite` | `./gonemaster.db` |
| `postgres` | `postgres://user:pass@host:5432/gonemaster?sslmode=disable` |
| `mariadb` | `user:pass@tcp(host:3306)/gonemaster?parseTime=true` |

Set the DSN with `GONEMASTER_DB_DSN`, the config file, or `--db-dsn`.
MariaDB DSNs should include `parseTime=true`; the server appends it when the
parameter is missing.

Connection pool defaults are documented in [server/database.md](server/database.md):

| Setting | Default |
|---|---:|
| Max open connections | `25` |
| Max idle | `25` |
| Connection lifetime | `5m` |

For production database setup, users, permissions, and backup examples, see
[database-setup.md](database-setup.md).

## Data retention

Automatic cleanup is configured with `database.retention_days`,
`GONEMASTER_DB_RETENTION_DAYS`, or `--db-retention-days`. A positive value
starts an hourly purge loop that removes jobs and related rows older than the
configured retention window.

Manual cleanup is available through `POST /jobs/purge` with
`older_than_days`. The response includes `purged_jobs`. When a caller asks the
server to use its configured retention window but no retention is configured,
the server returns `retention_not_configured`.

See [server/database.md](server/database.md) and
[server/operations.md](server/operations.md) for details.

## API Reference

The admin API lives under `/api/v1/`. The public API lives under
`/pub/api/v1/`.

For API conventions and the OpenAPI contract, see
[reference/api.md](reference/api.md) and [openapi.yaml](openapi.yaml).
