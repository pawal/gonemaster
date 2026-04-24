# Cohorts

A cohort is a public analysis dataset backed by one source tag. Tags are
internal grouping tools. Cohorts are curated views that decide what is
materialized and what is exposed publicly.

## Settings

| Field | Meaning |
|---|---|
| `source_tag` | Tag that supplies the cohort domains. |
| `analysis_enabled` | Whether completed runs for the tag are materialized. |
| `public_enabled` | Whether the cohort appears in the public analysis UI. |
| `sort_order` | Display order in the public catalog. |

## Materialization

When a completed run belongs to an analysis-enabled cohort, the server projects
derived facts into analysis tables. The public UI reads those facts instead of
scanning raw log entries for every request.

Admin actions can rebuild or clear a cohort. Rebuild reprojects matching runs.
Clear removes materialized rows and leaves the cohort pending.

## Snapshots

Public cohorts resolve to snapshots. Without a `snapshot` query parameter, the
cohort uses its configured default snapshot policy, usually the latest captured
public snapshot.

See [snapshots.md](snapshots.md).
