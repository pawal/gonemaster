# Cohorts

A cohort is a curated public analysis dataset backed by one source tag. Tags
are internal grouping tools. Cohorts decide which tagged results are
materialized and whether they are exposed in the public analysis UI.

## Core Model

| Concept | Meaning |
|---|---|
| Tag | Named domain collection used by operators. |
| Cohort | Public dataset backed by one source tag. |
| Materialization | Derived analysis rows built from completed runs. |
| Snapshot | Immutable public view captured from one snapshot-intent batch. |

The split between tag and cohort is deliberate. A tag can exist for any
operator task. A cohort is a publication decision.

## Settings

| Field | Meaning |
|---|---|
| `source_type` | Always `tag` in the current version. |
| `source_tag` | Tag that supplies the cohort domains. Immutable after creation. |
| `label` | Display name shown in the public analysis UI. |
| `description` | Optional long description shown on the cohort overview. |
| `analysis_enabled` | Whether matching runs are projected into analysis tables. |
| `public_enabled` | Whether the cohort appears in the public catalog. |
| `sort_order` | Display order in the public catalog. |

Only `analysis_enabled` cohorts receive materialized rows. Only
`public_enabled` cohorts are listed on the public path.

## End-to-End Workflow

1. Create a tag and add domains.

   ```sh
   gonemaster-client tags create tld --description "Top-level domains"
   gonemaster-client tags add-domains tld --file tlds.txt
   ```

2. Create a cohort for that tag in the admin UI under
   **Settings > Analysis > Cohorts**, or use the admin API:

   ```bash
   curl -s -X POST http://localhost:8080/api/v1/analysis/cohorts \
     -H "Content-Type: application/json" \
     -d '{"source_tag": "tld", "label": "TLDs", "analysis_enabled": true}'
   ```

3. Run a snapshot-intent batch for the source tag.

   The admin UI has a **Run new snapshot** action on the cohort row. The batch
   form also has **Capture as cohort snapshot** when the selected tag backs a
   cohort.

4. Browse the public view at `/analysis/`.

## Materialization

For each completed run in an analysis-enabled cohort, the server projects
derived facts into analysis tables:

- domain summary
- nameserver endpoints
- address to ASN facts
- domain to ASN facts
- finding tag summaries
- small domain facts such as signing and algorithm categories

The public UI reads these tables. It does not scan raw `entries` for every
request.

## Status and Repair

Cohort state is visible in the admin UI and through:

```bash
curl -s http://localhost:8080/api/v1/analysis/status
```

Materialization statuses:

- `pending`: materialization has not finished.
- `ready`: materialized rows are usable.
- `failed`: projection failed. Inspect `last_materialization_error`.

Repair actions:

```bash
curl -s -X POST http://localhost:8080/api/v1/analysis/cohorts/{id}/rebuild
curl -s -X POST http://localhost:8080/api/v1/analysis/cohorts/{id}/clear
```

Rebuild clears and reprojects matching runs. Clear removes materialized rows and
leaves the cohort pending. Disabling analysis clears materialization; enabling
it again triggers a rebuild.

## Snapshots

Public cohorts resolve to snapshots. Without a `snapshot` query parameter, the
cohort uses its default snapshot policy, usually the latest captured public
snapshot.

Snapshots keep public URLs stable and keep ad-hoc retests out of the public
cohort series unless the batch was explicitly marked as snapshot-intent.

See [snapshots.md](snapshots.md).
