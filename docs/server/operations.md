# Server Operations

This page covers day-to-day operation of `gonemaster-server`: jobs, batches,
queue control, deletion, health, and metrics.

## Jobs and Batches

Use jobs for individual domain tests. Use batches for many domains or for
rerunning every domain in a tag.

Interactive single-domain jobs use normal priority. Batch jobs use batch
priority, so a large sweep does not block a waiting interactive user.

## Queue Controls

The admin API and admin UI can:

- pause and resume queue processing
- reorder queued jobs within priority rules
- remove queued jobs
- cancel queued or running jobs

Queue changes affect pending work only. Completed runs are historical records.

## Retention and Deletion

Retention purges old terminal jobs according to the configured retention
window. Manual batch deletion is stronger: it deletes a batch, completed runs,
entries, analysis facts, and cohort snapshots derived from that batch.

Use snapshot retire or purge when the batch itself should stay but a public
snapshot should be hidden or removed.

## Health and Metrics

- `GET /api/v1/healthz` reports liveness.
- `GET /api/v1/metrics` returns JSON metrics by default.
- `GET /api/v1/metrics?format=prom` returns Prometheus text exposition.

See [../metrics.md](../metrics.md) for metric names and dashboard guidance.
