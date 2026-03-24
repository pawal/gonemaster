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
| `GONEMASTER_DB_RETENTION_DAYS` | `database.retention_days` | integer; 0 = keep forever |
| `GONEMASTER_PUBLIC_API_RATE_LIMIT_ENABLED` | `public_api.rate_limit_enabled` | `true`/`false`/`1`/`0` |
| `GONEMASTER_PUBLIC_API_RATE_LIMIT_MAX` | `public_api.rate_limit_max` | integer; requests per window per IP |
| `GONEMASTER_PUBLIC_API_RATE_LIMIT_WINDOW` | `public_api.rate_limit_window` | Go duration string e.g. `10m` |

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
| `postgres` | PostgreSQL 13+. |
| `mariadb` | MariaDB 10.6+ / MySQL 8.0+. |

#### DSN formats

**SQLite** — file path:
```
gonemaster-server --db-driver sqlite --db-dsn /var/lib/gonemaster/gonemaster.db
```

**PostgreSQL** — connection URL:
```
gonemaster-server --db-driver postgres \
  --db-dsn "postgres://user:pass@host:5432/dbname?sslmode=disable"
```

**MariaDB / MySQL** — DSN string:
```
gonemaster-server --db-driver mariadb \
  --db-dsn "user:pass@tcp(host:3306)/dbname"
```
`parseTime=true` is appended automatically if not already present.

Use environment variables to keep passwords out of process listings and shell history:
```
GONEMASTER_DB_DRIVER=postgres \
GONEMASTER_DB_DSN="postgres://user:pass@host:5432/dbname?sslmode=disable" \
  gonemaster-server
```

For a backend selection guide, per-backend recommended settings, and setup/tuning/backup
procedures see [docs/database-setup.md](database-setup.md).

#### Connection pool defaults

| Backend | Max open connections | Max idle | Connection lifetime |
|---|---|---|---|
| `sqlite` | 1 (serialised writes) | — | — |
| `postgres` | 25 | 5 | 5 minutes |
| `mariadb` | 25 | 5 | 5 minutes |

On startup with a persistent backend, the server automatically:
- Runs any pending schema migrations.
- Marks jobs that were `running` when the server last stopped as `failed`.
- Re-enqueues jobs that were `queued` but not yet started.

#### Data retention

Completed jobs (`succeeded`, `failed`, `canceled`, `expired`) accumulate over time. Configure automatic purging with `retention_days`:

```
gonemaster-server --db-driver sqlite --db-dsn /var/lib/gonemaster/gonemaster.db \
  --db-retention-days 90
```

- `0` (default) — keep forever, no automatic purge.
- Any positive value starts a background purge loop that runs **hourly** and deletes completed jobs with `finished_at` older than that many days, along with their results.
- Running, queued, and paused jobs are never purged automatically.

Recommended production setting: `90` days.

### Public API

The server exposes a separate, restricted API at `/pub/api/v1/` intended for
reverse-proxy exposure to untrusted clients. It supports job submission and
result lookup by an opaque public ID; internal UUIDs are never disclosed.

Available endpoints:

| Method | Path | Description |
|---|---|---|
| `POST` | `/pub/api/v1/jobs` | Submit a job (returns `public_id`, not internal UUID) |
| `GET` | `/pub/api/v1/jobs/{public_id}` | Poll job status by public ID |
| `GET` | `/pub/api/v1/jobs/{public_id}/result` | Fetch result by public ID |
| `GET` | `/pub/api/v1/locales` | List available locale codes |

Admin-only paths (`/metrics`, `/queue/*`, `/batches`, `/jobs/purge`) are not
reachable via the `/pub/` prefix — the boundary is enforced server-side.

#### Rate limiting

The public API supports per-IP sliding-window rate limiting on `POST /pub/api/v1/jobs`.
GET requests are never counted. Blocked requests receive `429 Too Many Requests`
with a `Retry-After` header indicating how many seconds to wait.

Rate limiting uses the client IP resolved from (in order):
`X-Forwarded-For` first value, `X-Real-IP`, `RemoteAddr`.

Enable and configure via flags, environment variables, or config file:

```
gonemaster-server \
  --public-api-rate-limit-enabled \
  --public-api-rate-limit-max 10 \
  --public-api-rate-limit-window 10m
```

| Setting | Default | Description |
|---|---|---|
| `public_api.rate_limit_enabled` | `false` | Enable rate limiting |
| `public_api.rate_limit_max` | `10` | Max submissions per IP per window |
| `public_api.rate_limit_window` | `10m` | Sliding window duration |

### Reverse proxy setup

To expose the public UI and its API to the internet while keeping the admin
interface private, configure your reverse proxy to forward only two path
prefixes:

| Prefix | Purpose |
|---|---|
| `/public/` | Public Svelte SPA (static assets) |
| `/pub/api/v1/` | Public API (job submission and result lookup) |

The server enforces the boundary internally — no additional path filtering
is required in the proxy.

#### nginx

```nginx
server {
    listen 443 ssl;
    server_name dns.example.com;

    location /public/ {
        proxy_pass http://127.0.0.1:8080/public/;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    }

    location /pub/api/v1/ {
        proxy_pass http://127.0.0.1:8080/pub/api/v1/;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    }
}
```

#### Caddy

```caddyfile
dns.example.com {
    reverse_proxy /public/* localhost:8080
    reverse_proxy /pub/api/v1/* localhost:8080
}
```

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
    "dsn": "/var/lib/gonemaster/gonemaster.db",
    "retention_days": 90
  },
  "public_api": {
    "rate_limit_enabled": true,
    "rate_limit_max": 10,
    "rate_limit_window": "10m"
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
- `--db-retention-days` Delete completed jobs older than N days; 0 = keep forever (env: `GONEMASTER_DB_RETENTION_DAYS`)
- `--public-api-rate-limit-enabled` Enable per-IP rate limiting on `POST /pub/api/v1/jobs` (env: `GONEMASTER_PUBLIC_API_RATE_LIMIT_ENABLED`)
- `--public-api-rate-limit-max` Max submissions per IP per window, default 10 (env: `GONEMASTER_PUBLIC_API_RATE_LIMIT_MAX`)
- `--public-api-rate-limit-window` Sliding window duration e.g. `10m`, default `10m` (env: `GONEMASTER_PUBLIC_API_RATE_LIMIT_WINDOW`)

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

Purge completed jobs older than a given age:
```
POST /jobs/purge
{ "older_than_days": 90 }
```
Response: `{ "purged_jobs": 42 }`

- `older_than_days` is optional; if omitted the server's configured `retention_days` is used.
- Returns `400` with `error.code=retention_not_configured` if both are `0`.
- Only terminal-status jobs (`succeeded`, `failed`, `canceled`, `expired`) are deleted; associated results are also removed.

### Batches
Submit a batch:
```
POST /jobs/batch
{
  "domains": ["example.com", "example.org"],
  "from_tag": "tld",
  "tags": ["tld"],
  "description": "TLD sweep 2024-Q1",
  "min_level": "NOTICE",
  "profile_overrides": { "timeout": 5 }
}
```

Either `domains` or `from_tag` (or both) must be provided. `from_tag` expands to all domains currently in that tag, deduplicated against any explicit `domains` list. `tags` tags the resulting batch and all its runs (creating domains if needed).

Batch submission does not support undelegated input; if `nameservers` or `ds_info` is included, the API returns `400` with `error.code=undelegated_not_supported_for_batch`.

Get batch summary:
```
GET /batches/{batch_id}
```

### Domains
List domains (paginated):
```
GET /domains?tag=tld&name=.se&level=ERROR&min_level=WARNING&limit=100&offset=0
```

Query params:
- `tag` — filter to domains belonging to this tag.
- `name` — substring filter on domain name.
- `level` — exact `latest_level` filter (e.g. `ERROR`).
- `min_level` — minimum severity threshold; `WARNING` matches `WARNING`, `ERROR`, `CRITICAL`.
- `limit` / `offset` — pagination (max 500, default 100).

Get a domain by ID:
```
GET /domains/{id}
```
Returns the domain record with its tags populated.

List run history for a domain:
```
GET /domains/{id}/runs?limit=50&offset=0
```

### Tags
List all tags:
```
GET /tags?limit=100&offset=0
```

Create a tag:
```
POST /tags
{ "name": "tld", "description": "Top-level domains" }
```
Returns `201` with the new tag. Returns `409` with `error.code=tag_exists` if the name is taken.

Update tag description:
```
PUT /tags/{name}
{ "description": "Updated description" }
```

Delete a tag:
```
DELETE /tags/{name}
```
Returns `204 No Content`. Removes all domain associations; domain records and their runs are preserved.

Add domains to a tag (creates domain records if they don't exist):
```
POST /tags/{name}/domains
{ "domains": ["example.com", "example.org"] }
```
Returns `204 No Content`.

Remove domains from a tag:
```
DELETE /tags/{name}/domains
{ "domains": ["example.com"] }
```
Returns `204 No Content`.

List domains in a tag (same shape as `GET /domains`):
```
GET /tags/{name}/domains?limit=100&offset=0
```

Get tag severity summary (domain counts by worst-level bucket):
```
GET /tags/{name}/summary
```
```json
{
  "tag": "tld",
  "domain_count": 1520,
  "ok": 1200,
  "notice": 150,
  "warning": 100,
  "error": 60,
  "critical": 10
}
```

### Runs
List runs (paginated):
```
GET /runs?tag=tld&domain=example.com&batch=batch_123&status=succeeded&level=ERROR&finished_after=2024-01-01T00:00:00Z&finished_before=2024-02-01T00:00:00Z&limit=100&offset=0
```

Query params:
- `tag` — filter to runs for domains in this tag.
- `domain` — filter by domain name substring.
- `batch` — filter by batch ID.
- `status` — filter by run status.
- `level` — filter by `worst_level`.
- `finished_after` / `finished_before` — RFC3339 timestamps.
- `limit` / `offset` — pagination (max 500, default 100).

Get a run:
```
GET /runs/{id}
```

Get a run result (same shape as `GET /jobs/{id}/result`):
```
GET /runs/{id}/result?locale=en
```

### Entries
Query individual engine log entries across all runs:
```
GET /entries?run=run_abc&tag=tld&domain=123&module=DNSSEC&testcase=dnssec01&entry_tag=DS_ALGO_NOT_SUPPORTED&level=ERROR&latest=1&batch=batch_123&format=csv&limit=100&offset=0
```

Query params:
- `run` — exact run ID.
- `domain` — domain ID (integer).
- `tag` — domain tag filter (joined via domain→tag associations).
- `module` — exact module name.
- `testcase` — exact testcase name.
- `entry_tag` — log event tag (the engine `tag` field, e.g. `DS_ALGO_NOT_SUPPORTED`).
- `level` — exact severity level.
- `latest` — `1` or `true` to restrict to each domain's latest run only.
- `batch` — restrict to runs from this batch.
- `format=csv` — download as CSV instead of JSON; columns: `id`, `run_id`, `domain_id`, `domain`, `timestamp`, `module`, `testcase`, `tag`, `level`, `args`.
- `limit` / `offset` — pagination (max 500, default 100).

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

`GET /metrics` returns a JSON snapshot by default. Optional query params:
- `format=json|prom` to choose JSON snapshot output or Prometheus text exposition.
- `window=1h|6h|24h|48h` to restrict `trends.windows` to one window.
- `include=` comma-separated sections: `all`, `health`, `jobs`, `api`, `quality`, `insights`, `trends`.
- `limit_domains=1..100` to cap `insights.domains.items`.
- `limit_batches=1..100` to cap `insights.batches.items`.

When `format=prom` is used, the JSON-only query params above are ignored and the response contains
stable low-cardinality Prometheus counters, gauges, and histograms instead.

Responses:
- `200` JSON snapshot or Prometheus text exposition.
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
- Domains tab: browse/filter the domain registry; drill into per-domain run history.
- Tags tab: create/edit/delete tags, manage domain membership, view per-tag severity summary.
- Run inspector: shows duration, entry count, and worst level alongside the full result.

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
