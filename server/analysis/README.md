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
