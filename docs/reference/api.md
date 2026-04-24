# API Reference

`gonemaster-server` exposes a trusted admin API and a restricted public API.

## Admin API

Base path: `/api/v1/`

Use this API from trusted clients such as the admin UI, `gonemaster-client`,
cron jobs, and internal scripts. It can manage jobs, batches, domains, tags,
runs, entries, profiles, queue state, settings, metrics, and cohorts.

Keep it private.

## Public API

Base path: `/pub/api/v1/`

Use this API for public job submission, public job lookup, public profiles, and
read-only public analysis. Public job endpoints use opaque public IDs instead
of internal job IDs.

## Conventions

- Requests and responses use JSON unless the endpoint documents another format.
- Errors use `{ "error": { "code": "...", "message": "..." } }`.
- Mutating admin requests validate `Origin` when the header is present.
- List endpoints use `limit` and `offset` where pagination is available.
- Result endpoints often accept `locale`.

## OpenAPI

The endpoint contract lives in [../openapi.yaml](../openapi.yaml). Narrative
guides should link to OpenAPI instead of duplicating long endpoint lists.
