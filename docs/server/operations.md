# Server Operations

This page covers day-to-day operation of `gonemaster-server`: jobs, batches,
queue control, deletion, health, and metrics.

## Jobs and Batches

Use jobs for individual domain tests. Use batches for many domains or for
rerunning every domain in a tag.

Interactive single-domain jobs use normal priority. Batch jobs use batch
priority, so a large sweep does not block a waiting interactive user.

Create one job:

```http
POST /api/v1/jobs
Content-Type: application/json

{
  "domain": "example.com",
  "min_level": "NOTICE"
}
```

Create a batch:

```http
POST /api/v1/jobs/batch
Content-Type: application/json

{
  "from_tag": "tld",
  "tags": ["tld"],
  "description": "TLD sweep"
}
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
window. Manual batch deletion is stronger: it deletes a batch, completed runs,
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

See [../metrics.md](../metrics.md) for metric names and dashboard guidance.

The JSON metrics endpoint accepts:

- `format=json|prom`
- `window=1h|6h|24h|48h`
- `include=all,health,jobs,api,quality,insights,trends`
- `limit_domains=1..100`
- `limit_batches=1..100`

Prometheus output ignores JSON-only query parameters.
