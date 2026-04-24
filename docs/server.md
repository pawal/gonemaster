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

## API Reference

The admin API lives under `/api/v1/`. The public API lives under
`/pub/api/v1/`.

For API conventions and the OpenAPI contract, see
[reference/api.md](reference/api.md) and [openapi.yaml](openapi.yaml).
