# Cohort Snapshots

Public analysis cohorts pin to immutable snapshots. A snapshot is backed by one
snapshot-intent batch. When the batch finishes, the projector captures the
cohort state for that batch and makes it permanently addressable.

Snapshots solve two operational problems:

- public URLs do not drift when newer runs finish
- partial retests do not silently change a published cohort

## Capture a New Snapshot

### From the Cohort Admin Panel

1. Open **Settings > Analysis > Cohorts** in the admin UI.
2. Click **Run new snapshot** on the cohort row.
3. The server submits a batch with the cohort source tag, the tag default
   profile when configured, and `snapshot_intent = true`.
4. The snapshot is `pending` until every job in the batch has graduated.

### From the Batch Form

1. Open the **Batches** tab.
2. Select **From tag** and choose the cohort source tag.
3. Enable **Capture as cohort snapshot**.
4. Submit the batch.

If the selected domain set is narrower than the full cohort, the UI warns that
the snapshot will cover only the selected domains.

## Lifecycle

- `pending`: jobs in the batch are still graduating.
- `captured`: every job has graduated and aggregates are computed.
- `retired`: hidden from the public path, but facts remain stored.
- `failed_mixed_profiles`: hidden because the batch used more than one profile.

Mixed-profile snapshots are intentionally hidden. A snapshot should represent
one comparable run profile across the cohort.

## Stable URLs

Every public analysis URL can carry `?snapshot=<slug>`.

```text
/analysis/
/analysis/?snapshot=2026-04-20-a1b2c3d4e5f6
/analysis/domains/example.se?snapshot=2026-04-20-a1b2c3d4e5f6
```

Without `snapshot`, the cohort resolves according to its default snapshot
policy. With `snapshot`, the URL stays pinned to that snapshot.

Use pinned URLs for reports, slide decks, tickets, and public references where
the numbers must not change later.

## Trends and Diffs

The Trends view compares aggregate categories across captured snapshots.
Current categories include:

- `severity_distribution`
- `grade_distribution`
- `signed`
- `dnskey_algo`

The Diff view compares two snapshots:

- added domains
- removed domains
- grade changes
- worst-level changes

Rows link back to the relevant domain detail in the selected snapshot.

## Retire, Restore, Purge

Use **Retire** when a snapshot should disappear from public views but remain
recoverable. Use **Restore** to make a retired snapshot public again. Use
**Purge** when the snapshot row and aggregate rows should be deleted.

Retiring or purging a snapshot that was the cohort's pinned default reverts the
cohort to `auto_latest`.

## Batch Deletion Interaction

Manual batch deletion removes the batch and everything derived from it,
including cohort snapshots backed by that batch. Use it when the batch itself
was wrong, such as a bad profile or a bad domain set.

Use snapshot retire or purge when the batch is valid but the snapshot should no
longer be public.

See [../server/batch-deletion.md](../server/batch-deletion.md).

## First-Boot Backfill

On first start after the snapshot model is installed, the server enumerates
existing `(cohort, batch)` pairs with materialized analysis rows and creates a
captured snapshot for each pair. The migration is idempotent and runs once.

Startup log example:

```text
analysis: snapshot backfill complete - cohorts=2 created=17 skipped=0
```
