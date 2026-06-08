# Documentation

Start with the path that matches what you are doing. The older top-level pages
remain in place while the documentation is being split into narrower guides.

The [architecture overview](architecture.md) is a one-sitting tour of the system: binaries, request lifecycles, data model, concurrency, security posture, and known limitations.

## Common Paths

| I want to... | Start here |
|---|---|
| Run one DNS check locally | [cli/](cli/README.md) |
| Start and operate the HTTP server | [server/](server/README.md) |
| Automate the server from a shell or script | [client/](client/README.md) |
| Manage domain tags and batch runs | [analysis/tags.md](analysis/tags.md) |
| Publish public cohort analysis | [analysis/](analysis/README.md) |
| Integrate with Nagios or Icinga | [nagios.md](nagios.md) |
| Drive gonemaster from an AI agent (MCP) | [mcp/](mcp/README.md) |
| Call the engine from Go | [dev.md](dev.md) |
| Tune resolver behavior (timeouts, slow-NS limits) | [profile-settings.md](profile-settings.md) |

## Server

- [server/README.md](server/README.md): server overview, API boundaries, job lifecycle.
- [server/configuration.md](server/configuration.md): flags, environment variables, config files, profiles.
- [server/database.md](server/database.md): storage backends, DSNs, retention, backups.
- [server/operations.md](server/operations.md): jobs, batches, queue controls, purge, deletion, health.
- [server/public-api-and-proxy.md](server/public-api-and-proxy.md): public endpoints, rate limiting, reverse proxies.
- [server/performance.md](server/performance.md): worker sizing, hot-cache, resolver tuning.
- [server/ui.md](server/ui.md): embedded admin UI, public UI, analysis UI.


## Client

- [client/README.md](client/README.md): `gonemaster-client` overview.
- [client/jobs.md](client/jobs.md): submit, watch, cancel, purge, and fetch job results.
- [client/batches.md](client/batches.md): run many domains and manage batch results.
- [client/domains-tags-runs-entries.md](client/domains-tags-runs-entries.md): query stored analysis data.
- [client/examples.md](client/examples.md): common command sequences.

Direct local tests with the `gonemaster` binary stay in [cli/](cli/README.md).

## Analysis

- [analysis/README.md](analysis/README.md): analysis model and workflow.
- [analysis/tags.md](analysis/tags.md): domain collections, tag membership, default profiles.
- [analysis/cohorts.md](analysis/cohorts.md): curated public datasets backed by tags.
- [analysis/snapshots.md](analysis/snapshots.md): immutable cohort snapshots, trends, diff views.
- [analysis/querying.md](analysis/querying.md): API, client, SQL, and CSV analysis patterns.
- [analysis/public-ui.md](analysis/public-ui.md): public analysis UI at `/analysis/`.

## Reference

- [specifications/api.md](specifications/api.md): API conventions and links to OpenAPI.
- [openapi.yaml](openapi.yaml): OpenAPI 3.0 specification for `gonemaster-server`.
- [server/metrics.md](server/metrics.md): Prometheus and JSON metrics.
- [scoring.md](scoring.md): numeric scores and letter grades.
- [cli/cache-format.md](cli/cache-format.md): packet cache save/restore file format.
- [MIGRATION-1.1.md](MIGRATION-1.1.md): JSON output migration guide.
- [specifications/](specifications/README.md): canonical testcase and tag specifications.
