# Cohort Snapshots

A snapshot is an immutable public view of a cohort captured from one
snapshot-intent batch. Snapshots make public URLs stable, so a report or
presentation can keep showing the same data after newer batches complete.

## Lifecycle

- `pending`: the snapshot batch still has unfinished jobs.
- `captured`: all jobs have graduated and aggregates are ready.
- `retired`: hidden from public views but still stored.
- `failed_mixed_profiles`: hidden because the batch used more than one profile.

## Stable URLs

Use `?snapshot=<slug>` to pin a public analysis URL:

```text
/analysis/?snapshot=2026-04-20-strict
```

Without a snapshot parameter, the cohort resolves according to its default
snapshot policy.

## Operations

Use retire when a snapshot should disappear from the public UI but remain
recoverable. Use purge when only the snapshot row and aggregates should be
deleted. Use batch deletion when the entire underlying batch and all derived
data should be removed.
