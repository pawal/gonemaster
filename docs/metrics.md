# Metrics

## Overview
The server exposes runtime and quality telemetry at:

- Relative API path: `GET /metrics`
- Full default URL: `GET http://127.0.0.1:8080/api/v1/metrics`

The endpoint returns a JSON snapshot intended for operators and the embedded web UI Metrics tab.

## Endpoint

### Method and path
`GET /metrics`

### Query parameters
- `window` (optional): `1h | 6h | 24h | 48h`
  - Limits the `trends.windows` payload to one window.
- `include` (optional): comma-separated sections
  - Values: `all`, `health`, `jobs`, `api`, `quality`, `insights`, `trends`
  - Example: `include=health,jobs,quality`
- `limit_domains` (optional): integer `1..100`
  - Caps `insights.domains.items`.
- `limit_batches` (optional): integer `1..100`
  - Caps `insights.batches.items`.

### Response codes
- `200`: metrics snapshot JSON
- `400`: structured error for invalid query params

### Caching behavior
The server caches rendered metrics responses for 1 second per unique query option set.

## Snapshot structure

Top-level fields:
- `schema_version`
- `generated_at`
- `health`
- `jobs`
- `api`
- `quality`
- `insights`
- `trends`

Commonly used fields:
- `health.queue_depth`
- `health.in_flight_jobs`
- `jobs.completed_total`
- `quality.outcomes.success_rate`
- `quality.outcomes.failed_rate`
- `quality.outcomes.failed_total`
- `quality.severity.totals` (`NOTICE`, `WARNING`, `ERROR`, `CRITICAL`)
- `insights.domains.items[]`
- `insights.batches.items[]`
- `trends.windows["1h"|"6h"|"24h"|"48h"]`
- `trends.windows[...].points[].dns_cache_hit_rate`

## Examples

Fetch default snapshot:
```bash
curl -s "http://127.0.0.1:8080/api/v1/metrics" | jq .
```

Fetch health/jobs/quality only:
```bash
curl -s "http://127.0.0.1:8080/api/v1/metrics?include=health,jobs,quality" | jq .
```

Fetch 1h trend window with bounded insights:
```bash
curl -s "http://127.0.0.1:8080/api/v1/metrics?window=1h&limit_domains=10&limit_batches=10" | jq .
```

Invalid query example:
```bash
curl -s "http://127.0.0.1:8080/api/v1/metrics?limit_domains=0" | jq .
```

Example error response:
```json
{
  "error": {
    "code": "invalid_limit_domains",
    "message": "limit_domains must be between 1 and 100"
  }
}
```

## Metrics Tab (Embedded UI)

The embedded web UI (`/`) has a **Metrics** tab backed by `GET /api/v1/metrics`.

It provides:
- Top cards for queue/flow/quality signals (queue depth, in-flight, rates, durations, finished/failed totals, severity totals).
- Trend mini-charts for throughput, failures, and queue depth.
- Insight tables for top domains and error-heavy batches.
- Refresh controls (`Refresh metrics`, auto-refresh toggle, trend window, insight limits).
- Loading/empty/error states and retry behavior.

Auto-refresh runs on a 10-second interval while the tab is active.

## References
- API schema: `docs/openapi.yaml`
- Server overview: `docs/server.md`
