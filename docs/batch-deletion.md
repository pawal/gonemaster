# Manual Batch Deletion

## When to use

Manual batch deletion removes a batch and everything derived from it:
queued and running jobs for the batch are cancelled; completed runs,
their log entries, their per-run analysis fact rows, and any cohort
snapshots backed by the batch are permanently removed.

Use it when a batch was submitted in error, used the wrong profile,
captured garbage that would pollute a cohort series, or otherwise
needs to disappear from the database.

The action is admin-only and confirmed by typing the full batch ID.
It is **not** reversible.

## When to use something else

- **Retire a snapshot** from the cohort panel when the underlying
  batch was fine but the snapshot itself is flawed or unwanted on
  the public path. Retiring hides the snapshot from public views
  without touching the runs or analysis data.
- **Purge a snapshot** (the existing `?purge=true` action) when you
  want to delete only the snapshot row and its aggregates, keeping
  the underlying runs and fact rows available for future rebuilds.
- **Time-based purge** (`POST /jobs/purge`) for retention-driven
  cleanup across every batch older than N days.

## Entry points (admin UI)

- **Batch inspector** (Batches tab) - paste a batch ID or pick one
  from the recent-batches dropdown, then click **Delete batch**.
- **Tag detail** - the **Earlier batches** panel lists recent batches
  for the tag. Click a row to open it in the inspector, or click the
  per-row **Delete batch** button.
- **Cohort snapshot panel** (Settings > Analysis > Cohorts) - each
  snapshot row carries a **Delete batch** action alongside retire
  and purge. Useful when deleting a tainted snapshot-intent batch
  that already captured a snapshot you do not want on record.

All three entry points open the same confirmation modal.

## What gets deleted

Inside a single SQL transaction, in dependency order:

1. Cohort snapshot aggregates (`analysis_cohort_snapshot_aggregates`)
2. Cohort snapshots (`analysis_cohort_snapshots`)
3. Per-run analysis fact rows
   - `analysis_run_ns_endpoints`
   - `analysis_run_address_asns`
   - `analysis_run_domain_asns`
   - `analysis_run_tag_summary`
   - `analysis_run_domain_facts`
   - `analysis_run_domain_summary`
4. Log entries (`entries`)
5. Completed runs (`runs`)
6. Queued or running jobs (`jobs`)
7. The batch metadata row (`batches`)

After the transaction commits, the server:

- Re-derives `domains.latest_*` for every domain whose `latest_run_id`
  pointed at one of the deleted runs, so the domain list does not
  linger with dangling pointers.
- Evicts the per-snapshot materialization cache entries for snapshots
  that were removed.
- Reverts any cohort whose default snapshot was deleted to
  `default_snapshot_policy = auto_latest`.

## What is kept

- Domains (`domains`). They survive independent of any one batch.
- Tag memberships (`tags`, `domain_tags`). The batch's `tag` field
  references a tag but does not own it.
- Profiles (`profiles`). Referenced by runs, not owned by them.
- `analysis_projection_state`. Re-projecting is idempotent and
  cheap; the cursor row is harmless.

## In-flight jobs

If the batch has queued or running jobs at delete time:

- Queued jobs are removed from the queue and graduated as
  `canceled` with `error = "deleted_by_admin"`.
- Running jobs are cancelled via their context. The worker drains
  and graduates them as canceled.

The server polls for up to five seconds waiting for all jobs to
reach a terminal status before starting the delete transaction. If
a worker is stuck and the timeout fires, the API returns HTTP 409
with `"jobs_not_terminal"`. Cancel the jobs manually, or try the
delete again once they drain.

## API

- `GET /api/v1/batches/{id}/delete-preview` - row counts plus
  affected snapshots. Read-only.
- `DELETE /api/v1/batches/{id}` - performs the delete. CSRF-guarded.
  Returns 204 on success, 404 if the batch is missing, 409 on a
  cancellation timeout.
- `GET /api/v1/tags/{name}/batches` - paginated list of batches
  whose `tag` equals `{name}`, newest first. Powers the tag detail
  "Earlier batches" panel.

## Audit trail

The server writes a single structured log line for each successful
batch deletion. There is no dedicated audit table; reconstructing
history past the point of log rotation is not supported.
