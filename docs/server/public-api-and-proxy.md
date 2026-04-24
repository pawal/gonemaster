# Public API and Reverse Proxy

This page owns the boundary between trusted admin surfaces and public
internet-facing surfaces.

## Admin Surfaces

Keep these private:

- `/`
- `/api/v1/`

They expose full control over jobs, batches, domains, tags, profiles, queue
state, cohorts, settings, and metrics.

## Public Surfaces

These are designed for public exposure:

- `/public/`
- `/analysis/`
- `/pub/api/v1/`

The public API uses opaque public IDs for public job lookup and does not expose
internal job or run IDs. Public analysis endpoints are read-only.

## Rate Limiting

Public job submission can be rate limited per client IP. Enable it before
exposing public job creation to the internet.

## Reverse Proxy

Configure the proxy so public paths are reachable and admin paths are blocked
or protected by authentication. See the examples in [../server.md](../server.md)
until this page receives the full proxy reference.
