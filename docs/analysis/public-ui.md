# Public Analysis UI

The public analysis UI is served at `/analysis/`. It is read-only and backed by
public analysis endpoints under `/pub/api/v1/analysis/`.

## What It Shows

The UI presents one cohort and one snapshot at a time. Visitors can browse:

- overview metrics
- domains
- nameservers
- endpoints
- ASNs and prefixes
- finding tags and testcases
- snapshot trends and diffs

## URL State

`dataset_tag` selects a cohort. `snapshot` pins an immutable snapshot. Shared
links should include `snapshot` when the numbers must not drift.

## Admin Work

Operators create tags, cohorts, batches, and snapshots in the admin UI or
through the admin API. Public users only read the published data.
