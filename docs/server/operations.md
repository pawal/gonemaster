# Server Operations

This page covers day-to-day operation of `gonemaster-server`: jobs, batches,
queue control, deletion, health, and metrics.

## Jobs and Batches

Use jobs for individual domain tests. Use batches for many domains or for
rerunning every domain in a tag.

Interactive single-domain jobs use normal priority. Batch jobs use batch
priority, so a large sweep does not block a waiting interactive user.

Create one job:

```bash
curl -s -X POST http://127.0.0.1:8080/api/v1/jobs \
  -H "Content-Type: application/json" \
  -d '{"domain": "example.com", "min_level": "NOTICE"}'
```

Create a batch:

```bash
curl -s -X POST http://127.0.0.1:8080/api/v1/jobs/batch \
  -H "Content-Type: application/json" \
  -d '{"from_tag": "tld", "tags": ["tld"], "description": "TLD sweep"}'
```

Batch submission does not support undelegated input. Use a single job when you
need explicit nameservers or DS records.

## Queue Controls

The admin API and admin UI can:

- pause and resume queue processing
- reorder queued jobs within priority rules
- remove queued jobs
- cancel queued or running jobs

Queue changes affect pending work only. Completed runs are historical records.

Priority tiers:

| Tier | Value | Assigned to |
|---|---:|---|
| Normal | 0 | Admin and public single-domain jobs. |
| Batch | 1 | Batch submissions and tag sweeps. |

Workers always drain normal jobs before batch jobs. Reordering is allowed only
within the tier rules.

## Retention and Deletion

Retention purges old terminal jobs according to the configured retention
window. [Manual batch deletion](batch-deletion.md) is stronger: it deletes a batch, completed runs,
entries, analysis facts, and cohort snapshots derived from that batch.

Use snapshot retire or purge when the batch itself should stay but a public
snapshot should be hidden or removed.

Batch deletion entry points:

- `GET /api/v1/batches/{id}/delete-preview`
- `DELETE /api/v1/batches/{id}`
- Admin UI batch inspector
- Admin UI tag detail earlier-batches panel
- Admin UI cohort snapshot panel

Batch deletion keeps domains and tag memberships. It removes records derived
from the selected batch.

## Health and Metrics

- `GET /api/v1/healthz` reports liveness.
- `GET /api/v1/metrics` returns JSON metrics by default.
- `GET /api/v1/metrics?format=prom` returns Prometheus text exposition.

See [metrics.md](metrics.md) for metric names and dashboard guidance.

The JSON metrics endpoint accepts:

- `format=json|prom`
- `window=1h|6h|24h|48h`
- `include=all,health,jobs,api,quality,insights,trends`
- `limit_domains=1..100`
- `limit_batches=1..100`

Prometheus output ignores JSON-only query parameters.

## Jobs that will not finish

A job only leaves `running` when the worker writes its result. If that write
cannot commit, the worker marks the job `failed` with an error beginning
`graduation failed:`, so callers stop polling and the in-flight gauge drains.

As a backstop, a sweep every minute fails any job left at `running` with no
worker on it for longer than `stuck_job_timeout_minutes` (default 20, `0`
disables it). Reaped jobs are logged as `reaped stuck job`.

This assumes one server process per database. Two servers sharing one
database will reap each other's live jobs, so set the timeout to `0` on
such a deployment.

Log lines to watch:

- `job graduation failed` - a result could not be stored; the underlying
  database error follows.
- `marking ungraduated job failed did not persist` - the fallback write
  failed too; the reaper or the next restart clears it.
- `startup recovery left orphan jobs ungraduated` - recovery skipped some
  rows and started anyway.

Database contention is retried and does not appear here. A steady stream of
`job graduation failed` points at the database; on MariaDB see
[database-setup.md](database-setup.md).

## Logging

The server writes operational logs (lifecycle, access log, warnings, errors) to
stderr using `log/slog`. Two encodings are available, selected with `log_format`
(see [configuration.md](configuration.md)):

- `text` (default): human-readable `key=value` lines for local use and journald.
- `json`: one JSON object per line, for aggregation (Loki, ELK, cloud logging).

Ship logs by letting systemd/journald or the container runtime capture stderr;
there is no in-process file rotation.

`log_level` sets the minimum level (`debug`, `info`, `warn`, `error`). Levels are
chosen so operators can alert on `error` lines without parsing status codes.

### Access log

One `http_request` line per request, on the API mounts (`/api/v1`,
`/pub/api/v1`) and on the page-serving mounts (`/`, `/public/`, `/analysis/`,
`/robots.txt`, `/sitemap.xml`). Static assets, meaning the hashed bundles,
fonts, and icons a page pulls in, are served without a line, so the log holds
roughly one entry per page view rather than one per file.

Page views are visitor activity and each line carries the client IP in `remote`,
so the retention policy of whatever collects stderr also governs this data.

The line's level follows the response status: 2xx/3xx -> `info`, 4xx -> `warn`,
5xx -> `error`. Fields:

| Field | Meaning |
|---|---|
| `method` | HTTP method. |
| `path` | Request path. |
| `route` | The router's matched pattern, prefixed with the mount, for aggregation. |
| `status` | HTTP status code. |
| `duration_ms` | Handler wall time in milliseconds. |
| `bytes` | Response body bytes written. |
| `remote` | Client IP (honors `trusted_proxy_cidrs`). |
| `request_id` | Correlation ID, also returned in the `X-Request-Id` header. |
| `body` | Response body preview, only when `--debug`/`log_level=debug` is set. |

A sample JSON access line:

```json
{"time":"2026-07-19T10:00:00Z","level":"INFO","msg":"http_request","method":"GET","path":"/api/v1/runs/r1","route":"/api/v1/runs/{id}","status":200,"duration_ms":12,"bytes":842,"remote":"203.0.113.7","request_id":"9f1c2a3b4d5e6f70"}
```

### Request IDs

Every request is tagged with a correlation ID that ties the access-log line,
any handler audit/error lines, and a panic line to the same request. The ID is
returned in the `X-Request-Id` response header. An inbound `X-Request-Id` is
trusted only from a `trusted_proxy_cidrs` peer; otherwise a fresh ID is
generated.

### Body capture

`--debug` (or `debug: true`) implies `log_level=debug` and adds a `body` field
with a truncated response-body preview to each access line. Leave it off in
production to avoid logging response bodies.
