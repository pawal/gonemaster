# Public Analysis UI

The public analysis UI is served at `/analysis/`. It is read-only and backed by
public analysis endpoints under `/pub/api/v1/analysis/`.

Operators create tags, cohorts, batches, and snapshots through the admin UI or
admin API. Public visitors only browse published cohort data.

## Web Surfaces

| Path | Purpose | Audience |
|---|---|---|
| `/` | Admin UI for jobs, tags, batches, cohorts, settings. | Trusted operators. |
| `/analysis/` | Public cohort analysis UI. | Public visitors. |
| `/public/` | Public single-domain result UI. | Public visitors. |

Block `/` and `/api/v1/` at the public reverse proxy. See
[../server/public-api-and-proxy.md](../server/public-api-and-proxy.md).

## What It Shows

The UI presents one cohort and one snapshot at a time. Visitors can browse:

- overview metrics
- domains
- nameservers
- endpoints
- ASNs and prefixes
- finding tags
- testcases
- snapshot trends
- snapshot diffs

Clicking an entity such as a domain, nameserver, ASN, prefix, finding tag, or
testcase opens a filtered detail view.

## URL State

`dataset_tag` selects the cohort. `snapshot` pins the snapshot.

```text
/analysis/?dataset_tag=tld&snapshot=2026-04-20-strict
```

The UI preserves the active snapshot as visitors click through detail pages.
Shared links should include `snapshot` when the numbers must remain stable.

## Snapshot Diffs

The diff view compares two snapshots of a cohort. Alongside the per-domain
changes (added, removed, grade or worst-level changes) it shows a tag-level
summary: which finding tags appeared, cleared, or changed severity
cohort-wide, with domain counts. This makes a regression explainable, for
example "14 domains regressed; DS02_NO_MATCHING_DS appeared on 12 of them".
The tag section degrades gracefully: if a snapshot has no materialized tag
view it shows a short notice rather than an error.

## Empty States

If the UI is empty after a batch finishes, check:

- the cohort has `analysis_enabled=true`
- the cohort has `public_enabled=true`
- the batch used the cohort source tag
- the batch was marked as snapshot-intent when it should publish
- `GET /api/v1/analysis/status` has no cohort error

If "Last analyzed" does not move, check server logs for analysis projection
errors and inspect `last_materialization_error` on the cohort.
