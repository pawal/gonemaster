# Analysis Schema

V1 keeps the raw job/run data in the existing tables and adds a derived
analysis layer for public-dashboard queries.

## Catalog

`analysis_cohort_catalog` is the control-plane table for admin-managed cohorts.

- `source_type` starts as `tag`
- `source_tag` points at the tag backing the cohort
- `analysis_enabled` means the projector maintains materialized rows for the cohort
- `public_enabled` means the cohort is exposed through `/pub/api/v1/analysis`
- `is_default` selects the default public cohort
- `materialization_status`, `last_materialized_at`, and `last_materialization_error`
  track rebuild state for operator visibility

`analysis_default_tag` should be treated only as a bootstrap or migration aid.
The cohort catalog is the long-term source of truth.

## Entities

These tables hold normalized shared infrastructure entities:

- `analysis_nameservers`
- `analysis_addresses`
- `analysis_prefixes`
- `analysis_asns`

Each entity table keeps a stable identifier plus `first_seen_at` and
`last_seen_at` for later backfill and rebuild work.

## Fact Tables

These tables are explicitly cohort-scoped in V1:

- `analysis_run_ns_endpoints`
- `analysis_run_address_asns`
- `analysis_run_domain_summary`
- `analysis_projection_state`

Every materialized fact row is keyed by `cohort_id` so multiple analyzed tags
can coexist safely. Public visibility is applied at query time from
`analysis_cohort_catalog`; it is not implied by the presence of materialized
rows alone.

## Scope Semantics

The public analysis layer should use these normalized scope modes:

- `latest_global`
  - default mode when no scope is supplied
  - selects the latest run per domain within the active cohort
  - does not allow `batch_id`, `from`, or `to`
- `latest_in_batch`
  - selects the latest run per domain within one batch
  - requires `batch_id`
  - does not allow `from` or `to`
- `batch`
  - selects all runs within one batch
  - requires `batch_id`
  - does not allow `from` or `to`
- `time_window`
  - selects all runs whose effective run timestamp falls within the given window
  - requires at least one of `from` or `to`
  - does not allow `batch_id`

Reject invalid filter combinations rather than silently guessing.

## Cohort Resolution Semantics

Public cohort resolution should be explicit and server-controlled.

- A cohort is selectable in the public dashboard only when:
  - `analysis_enabled = true`
  - `public_enabled = true`
- `public_enabled = true` with `analysis_enabled = false` is invalid catalog state.
- When `dataset_tag` is supplied:
  - resolve it against selectable cohorts by `source_tag`
  - fail if it does not match a selectable public cohort
- When `dataset_tag` is omitted:
  - resolve the single cohort marked `is_default = true`
  - fail if multiple selectable cohorts are marked default
  - fail if no default public cohort exists
- `analysis_default_tag` may be used only as a temporary bootstrap fallback when
  no catalog default exists. It should not override the catalog once the admin
  cohort table is populated.

## Snapshot Model (V2)

The public analysis layer pins to immutable **cohort snapshots** instead of
collapsing runs to "latest per domain" on every request.

- Every snapshot is keyed by `(cohort_id, batch_id)`; one batch with
  `snapshot_intent = true` produces exactly one snapshot.
- A snapshot starts as `pending`, accumulates runs as its jobs graduate,
  and promotes to `captured` once every job in the batch has finished.
  Once captured, the snapshot is immutable — rematerialize explicitly to
  rebuild its aggregates.
- `analysis_cohort_catalog.default_snapshot_policy` is `auto_latest` by
  default (the newest captured public snapshot wins) or `pinned`
  (`default_snapshot_id`). Admin UI's "Make default" action pins.
- Non-snapshot-intent batches still run and their jobs graduate normally,
  but the projector never writes fact rows or snapshot rows for them —
  ad-hoc retests stay out of the cohort series entirely.
- Mixed-profile batches land in `status = 'failed_mixed_profiles'` with
  `is_public = false` so a broken batch does not leak into the public
  path. Admins see a banner in the cohort panel.

### First-boot backfill (Phase 7)

On the first server start after the snapshot model ships the runtime
enumerates every `(cohort_id, batch_id)` pair with materialized
`analysis_run_domain_summary` rows and creates one `captured` snapshot
per pair. The migration is idempotent, gated by the
`analysis_snapshot_backfill_v1` setting, and emits a one-line log banner
summarising what it did:

```
analysis: snapshot backfill complete — cohorts=2 created=17 skipped=0
```

Existing `/pub/api/v1/analysis/` bookmarks keep working because
auto-latest resolution picks the newest captured snapshot, which after
backfill is the most recent historical batch.
