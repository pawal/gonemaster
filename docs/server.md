# Gonemaster Server

## Overview
- `gonemaster-server` is a REST API wrapper around the Gonemaster engine with an embedded web UI.
- The API contract is defined in [openapi.yaml](openapi.yaml).
- The UI is served at `/` from the embedded build output in `server/ui/dist`.
- The API is served under `/api/v1`.
- Job progress is reported as a percentage (0-100).

The default profile is located in `share/profile.json`, but is built into the server binary.
By default, it enables IPv4+IPv6 and currently sets `resolver.defaults.parallel=8` and
`resolver.defaults.unordered=true`.
If you don't have access to IPv6 on your development machine, or you need deterministic
ordered behavior, use a custom profile via `--profile`.

## Build
```
go build -o ./gonemaster-server ./cmd/gonemaster-server
```

To rebuild the embedded UI:
```
make ui-build
```
Note: building the UI requires `npm` to be available in your PATH.

To build an API-only server (no UI embed, no npm required):
```
make build-gonemaster-server-noui
```
or:
```
go build -tags nogui -o ./gonemaster-server ./cmd/gonemaster-server
```
When built with `nogui`, the `/` UI route serves a short informational HTML page explaining that the UI is not embedded, while API routes under `/api/v1` are unaffected. Static UI asset routes still return `404 ui not available`.

## Quick start
Start the server:
```
./gonemaster-server
```

Submit a single job and capture the job id:
```
JOB_ID=$(curl -s http://localhost:8080/api/v1/jobs \
  -H 'Content-Type: application/json' \
  -d '{"domain":"example.com"}' | jq -r .id)
```

Wait for completion (poll status):
```
while true; do
  STATUS=$(curl -s http://localhost:8080/api/v1/jobs/$JOB_ID | jq -r .status)
  echo "status=$STATUS"
  if [ "$STATUS" = "succeeded" ] || [ "$STATUS" = "failed" ] || [ "$STATUS" = "canceled" ] || [ "$STATUS" = "expired" ]; then
    break
  fi
  sleep 2
done
```

Fetch the result (localized messages, default English):
```
curl -s "http://localhost:8080/api/v1/jobs/$JOB_ID/result?locale=en" | jq .
```

## Run
```
./gonemaster-server
```

## Configuration

Configuration is applied in priority order (highest wins):
1. **CLI flags** (e.g. `--listen`, `--db-driver`)
2. **Environment variables** (`GONEMASTER_*`)
3. **Config file** (`--config path/to/config.json`)
4. **Built-in defaults**

`profile_path` sets the default profile used for all jobs (same as `gonemaster --profile`).

### Environment variables

| Variable | Config field | Notes |
|---|---|---|
| `GONEMASTER_LISTEN` | `listen_addr` | |
| `GONEMASTER_WORKER_COUNT` | `worker_count` | integer |
| `GONEMASTER_MAX_CONCURRENT_JOBS` | `max_concurrent_jobs` | integer |
| `GONEMASTER_MIN_LEVEL` | `min_level` | |
| `GONEMASTER_PROFILE` | `profile_path` | |
| `GONEMASTER_DEBUG` | `debug` | `true`/`false`/`1`/`0` |
| `GONEMASTER_DB_DRIVER` | `database.driver` | |
| `GONEMASTER_DB_DSN` | `database.dsn` | Use this for connection strings containing passwords |

Invalid values for integer or boolean variables emit a warning and are ignored (the server continues with the lower-priority value).

### Deterministic mode
If you want deterministic ordered resolver behavior on the server, set:
- `resolver.defaults.unordered: false`
- `resolver.defaults.parallel: 1`

You can do this in the profile used by `profile_path` (or `--profile`), and/or via
per-job `profile_overrides`.

### Batch throughput tuning (8-core reference)
For high-volume batch runs, `--workers` and `--max-concurrent-jobs` have large impact.

Measured on an 8-core host with the fixed 600-domain corpus:
- `workers=24`, `max-concurrent-jobs=24`: strong throughput gain with moderate tails.
- `workers=32`, `max-concurrent-jobs=32`: best throughput, but worse p95/p99 tails than 24.
- `workers>32`: throughput regressed and tail latency worsened.

Recommended starting points for new users on 8-core machines:
1. Balanced profile:
   - `--workers 24 --max-concurrent-jobs 24`
2. Throughput-max profile:
   - `--workers 32 --max-concurrent-jobs 32`

If you are latency-sensitive, start at `24/24` and validate before increasing.
If you are throughput-first, try `32/32` and monitor p95/p99.
Always re-check on your own network/workload before finalizing defaults.

### Tuning timeouts and retries
`gonemaster-server` uses the profile defaults for query timing. To tune these,
set them in the profile referenced by `profile_path`, or override with flags:
```yaml
resolver:
  defaults:
    timeout: 2     # seconds per attempt
    retry: 0       # retry count
    retrans: 1     # seconds between retries
    fallback: false
```
These settings apply to all jobs unless a job overrides the profile.

### Database

The server supports pluggable storage backends selected by `--db-driver`:

| Driver | Description |
|---|---|
| `memory` | In-memory only (default). All data lost on restart. |
| `sqlite` | Embedded SQLite database. Recommended for single-server production use. |
| `postgres` | PostgreSQL. Phase 2 — not yet available. |
| `mariadb` | MariaDB/MySQL. Phase 2 — not yet available. |

For SQLite, `--db-dsn` is the file path:
```
gonemaster-server --db-driver sqlite --db-dsn /var/lib/gonemaster/gonemaster.db
```

Using environment variables (recommended for DSNs containing passwords):
```
GONEMASTER_DB_DRIVER=sqlite GONEMASTER_DB_DSN=/var/lib/gonemaster/gonemaster.db \
  gonemaster-server
```

On startup with a persistent backend, the server automatically:
- Runs any pending schema migrations.
- Marks jobs that were `running` when the server last stopped as `failed`.
- Re-enqueues jobs that were `queued` but not yet started.

### Config example
```json
{
  "listen_addr": "127.0.0.1:8080",
  "max_body_size": 1048576,
  "debug": true,
  "worker_count": 4,
  "max_concurrent_jobs": 0,
  "positive_cache_ttl": 0,
  "negative_cache_ttl": 0,
  "timeout": 5,
  "retry": 2,
  "retrans": 3,
  "fallback": true,
  "min_level": "INFO",
  "profile_path": "/path/to/profile.json",
  "database": {
    "driver": "sqlite",
    "dsn": "/var/lib/gonemaster/gonemaster.db"
  }
}
```

## Flags
- `--config` JSON config file path
- `--listen` Address to listen on
- `--max-body-size` Max request body size in bytes
- `--debug` Enable request/response logging
- `--workers` Number of worker goroutines
- `--max-concurrent-jobs` Max concurrent engine runs (0 = unlimited)
- `--positive-cache-ttl` Seconds to cache positive DNS responses (optional)
- `--negative-cache-ttl` Seconds to cache negative DNS responses (optional)
- `--timeout` Override resolver.defaults.timeout in seconds (optional)
- `--retry` Override resolver.defaults.retry (optional)
- `--retrans` Override resolver.defaults.retrans in seconds (optional)
- `--fallback` Enable TCP fallback on UDP failure (optional)
- `--no-fallback` Disable TCP fallback on UDP failure (optional)
- `--min-level` Minimum log level for results
- `--profile` Profile JSON/YAML path (default for all jobs)
- `--shutdown-timeout` Graceful shutdown timeout
- `--db-driver` Storage backend (`memory`, `sqlite`, `postgres`, `mariadb`; env: `GONEMASTER_DB_DRIVER`)
- `--db-dsn` Database file path or connection string (env: `GONEMASTER_DB_DSN`)

## Domain normalization (IDN)
Domains are normalized to IDNA A-labels (punycode). For example:
`räksmörgås.se` becomes `xn--rksmrgs-5wao1o.se`.
Invalid domains return a `400` error with `code=invalid_domain`.

## API basics
- Base URL: the server listen address plus `/api/v1` (default `http://127.0.0.1:8080/api/v1`).
- All endpoint paths below are relative to the base URL.
- Content-Type: JSON for requests and responses.
- CSRF protection: mutating endpoints (`POST`) validate `Origin` when provided and require it to match the request host. Clients without an `Origin` header (for example `gonemaster-client`) continue to work unchanged.
- Errors: standard JSON envelope:
  ```json
  { "error": { "code": "invalid_domain", "message": "..." } }
  ```

## Endpoints

### Jobs
Create a single job:
```
POST /jobs
{
  "domain": "example.com",
  "tests": ["basic01"],
  "nameservers": [
    { "ns": "ns1.example.com", "ip": "192.0.2.10" },
    { "ns": "ns1.example.com", "ip": "2001:db8::10" },
    { "ns": "ns2.example.net" }
  ],
  "ds_info": [
    { "keytag": 12345, "algorithm": 13, "digtype": 2, "digest": "ABCD..." }
  ],
  "min_level": "NOTICE",
  "profile_overrides": { "timeout": 5 }
}
```

`nameservers` and `ds_info` are optional and only supported on single-job `POST /jobs`.

List jobs:
```
GET /jobs?status=running&limit=100
```

Useful list filters include:
- `domain=<substring>` to match domain names.
- `batch_id=<id>` to scope to one batch.
- `severity=warnings_plus|errors_only` to filter by aggregated severity totals before pagination.

Get a job:
```
GET /jobs/{job_id}
```

Get a job result (localized messages):
```
GET /jobs/{job_id}/result?locale=en
```

Result schema (relevant fields):
```json
{
  "job_id": "job_123",
  "status": "succeeded",
  "summary": {
    "total": 42,
    "levels": { "NOTICE": 2, "WARNING": 1, "ERROR": 0 }
  },
  "raw": {
    "locale": "en",
    "entries": [
      {
        "timestamp": 0.12,
        "module": "BASIC",
        "testcase": "basic01",
        "tag": "BASIC01",
        "level": "NOTICE",
        "args": { "domain": "example.com" },
        "message": "Translated message (locale=en)",
        "raw": "BASIC:basic01:BASIC01 domain=example.com"
      }
    ]
  }
}
```

Machine extraction from API result:
```
curl -s "http://localhost:8080/api/v1/jobs/$JOB_ID/result?locale=en" \
  | jq -r '.raw.entries[]
           | select(.args.ns and .args.address)
           | [.args.ns, .args.address] | @tsv'
```

Extract ASN lists (when present):
```
curl -s "http://localhost:8080/api/v1/jobs/$JOB_ID/result?locale=en" \
  | jq -r '.raw.entries[]
           | select(.args.asns != null)
           | [.tag, (.args.asns | map(tostring) | join(","))] | @tsv'
```

Stream job events (SSE):
```
GET /jobs/{job_id}/events
```
Currently emits a basic `state` event; more event types can be added later.

Cancel a job:
```
POST /jobs/{job_id}/cancel
```

### Batches
Submit a batch:
```
POST /jobs/batch
{
  "domains": ["example.com", "example.org"]
}
```

Batch submission does not support undelegated input; if `nameservers` or `ds_info` is included, the API returns `400` with `error.code=undelegated_not_supported_for_batch`.

Get batch summary:
```
GET /batches/{batch_id}
```

### Queue controls
Pause queue:
```
POST /queue/pause
```

Resume queue:
```
POST /queue/resume
```

Reorder queued jobs:
```
POST /queue/reorder
{ "job_ids": ["job_a", "job_b"] }
```

Remove queued jobs:
```
POST /queue/remove
{ "job_ids": ["job_a"] }
```

### Ops
```
GET /metrics
GET /healthz
```

`GET /metrics` returns a JSON snapshot. Optional query params:
- `window=1h|6h|24h|48h` to restrict `trends.windows` to one window.
- `include=` comma-separated sections: `all`, `health`, `jobs`, `api`, `quality`, `insights`, `trends`.
- `limit_domains=1..100` to cap `insights.domains.items`.
- `limit_batches=1..100` to cap `insights.batches.items`.

Responses:
- `200` JSON snapshot.
- `400` structured error for invalid query params.

Trend points in the metrics snapshot include throughput, failures, DNS query
rates, and per-bucket cache hit rate (`0..1`).

The metrics endpoint caches rendered responses for 1 second per unique query option set.

For full Metrics API details and Metrics tab notes, see `docs/metrics.md`.

## UI
The embedded UI is served at `/` and calls the API on the same host.

### Features
- Single and batch job submission.
- Job and batch inspectors with auto-refresh.
- Auto-refresh for a job stops automatically when progress reaches 100%.
- Progress displayed as a bar.
- Summary view for NOTICE/WARNING/ERROR/CRITICAL.
- Raw results grouped by module; click a module to expand/collapse.
- Raw results show a CLI-style table (seconds, level, message) and use translated messages when available.

### Build & dev
Rebuild the embedded UI:
```
make ui-build
```

Run the UI dev server (Vite):
```
make ui-dev
```

The dev server runs on `http://localhost:5173` and uses the browser to talk to the API.
