# Cohort Snapshots

Public analysis cohorts in gonemaster pin to immutable **snapshots**
rather than collapsing to "latest per domain" on every request. Each
snapshot is backed by one snapshot-intent batch: when the batch
finishes, the projector captures the cohort's state at that point and
makes it permanently addressable.

This guide covers the admin-facing workflow for operating snapshots:
how to capture a new one, how to share a URL that does not drift, how
to diff two snapshots, and how to retire stale ones.

## Capturing a new snapshot

A snapshot is produced when a **snapshot-intent batch** for the
cohort's tag finishes. Two paths produce one:

### From the Cohort admin panel (recommended)

1. Open **Settings → Analysis → Cohorts** in the admin UI.
2. On the cohort's row, click **Run new snapshot**. The server submits
   a batch with the cohort's source tag, the tag's default profile,
   and `snapshot_intent = true`.
3. The batch runs in the background. The Snapshots sub-panel shows
   the pending snapshot; it flips to `captured` once every job in the
   batch has graduated.

### From the Batch form

1. Open the **Batches** tab.
2. Pick **From tag** and select the cohort's tag.
3. Tick **Capture as cohort snapshot**. The form shows the proposed
   slug (`YYYY-MM-DD-<profile>` or `YYYY-MM-DD-*` when no profile
   applies) so the submission is deliberate.
4. If you narrow the domain list below the full cohort, the form
   warns that the snapshot will only cover the domains you picked —
   tick the box anyway if that is what you want, or untick it for an
   ad-hoc retest that stays out of the cohort series.

### Lifecycle

- **pending** — jobs in the batch are still graduating.
- **captured** — every job has graduated and the snapshot's
  aggregates are pre-computed. Immutable from here.
- **retired** — admin hid it from the public path. Fact rows stay
  intact until purged.
- **failed_mixed_profiles** — the batch was submitted with more than
  one profile. The snapshot is hidden from public and the cohort
  panel shows a red banner until the admin re-runs the batch with a
  single profile or purges the broken row.

## Sharing a stable URL

Every public URL carries an optional `?snapshot=<slug>` parameter. Use
it when you want a slide deck, email link, or conference screenshot
to keep showing the same numbers a week later.

- `/analysis/` (no slug) — auto-latest. Resolves to the most recent
  captured public snapshot; drifts when a new snapshot is captured.
- `/analysis/?snapshot=2026-04-20-strict` — pinned. Resolves to this
  exact snapshot forever, even after newer snapshots ship.

Find the slug in the **Snapshot** pill on the overview page, or in
the snapshot selector dropdown in the filter bar. All entity chips
(domain, nameserver, ASN, prefix, tag) preserve the active snapshot
as you click through, so a shared link deeper than the overview
stays pinned too.

The admin **Make default** action sets a cohort's default to a
specific snapshot — useful for hosting a fixed "headline numbers"
view while still letting visitors jump to a newer snapshot via the
selector.

## Comparing snapshots over time

### Trends tab

`/analysis/trends?category=<category>` renders one aggregate as a
time series across the cohort's captured snapshots. Categories:

- `severity_distribution`
- `grade_distribution`
- `signed`
- `dnskey_algo`

Each snapshot contributes one bar; the category dropdown switches the
chart without losing the cohort pin.

### Diff tab

`/analysis/diff?from=<slug>&to=<slug>` returns the delta between two
snapshots in four tabs:

- **Added** — domains present in the target snapshot but not the
  source. Useful for detecting expanding cohorts.
- **Removed** — domains that dropped out.
- **Grade changed** — domains whose grade moved between snapshots.
- **Worst-level changed** — domains whose worst severity bucket
  moved.

Each row links back into the per-domain detail in the relevant
snapshot for a closer look at what changed.

## Retiring stale snapshots

Old snapshots accumulate; retire the ones you no longer want
surfaced publicly.

1. Open **Settings → Analysis → Cohorts** and expand the cohort's
   Snapshots sub-panel.
2. **Retire** (soft delete) sets status to `retired`, hides the row
   from the public path, and keeps fact rows intact so you can undo
   by clicking **Restore**.
3. **Purge** hard-deletes the snapshot row and its aggregates.
   Confirmable; not reversible without re-running the underlying
   batch.

Retiring or purging a snapshot that was the cohort's pinned default
automatically reverts the cohort to `auto_latest`, so the next
captured snapshot becomes the default.

## First-boot backfill

On the first start after the snapshot model ships, gonemaster
enumerates every `(cohort, batch)` pair with materialized analysis
rows and creates one captured snapshot per pair. The migration is
idempotent and gated by an internal setting so it runs exactly once.
Existing bookmarks keep working — auto-latest resolves to the most
recent historical batch.

Startup log banner on first boot:

```
analysis: snapshot backfill complete — cohorts=2 created=17 skipped=0
```

Subsequent starts skip the backfill silently.
