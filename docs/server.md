# Gonemaster Server

## Overview

`gonemaster-server` wraps the Gonemaster DNS-testing engine in an HTTP server with a
persistent job queue, pluggable storage, and two distinct APIs:

| Path prefix | Who uses it | What it can do |
|---|---|---|
| `/api/v1/` | Trusted clients (admin UI, `gonemaster-client`, scripts) | Full control: jobs, batches, queue, domains, tags, runs, entries, metrics |
| `/pub/api/v1/` | Untrusted clients over the internet (via reverse proxy) | Submit a job, poll status, fetch result - by opaque `public_id` only |

The **admin API** (`/api/v1/`) exposes all server capabilities. Internal job UUIDs are
visible. Never expose this directly to the internet.

The **public API** (`/pub/api/v1/`) is a deliberately restricted subset intended for
reverse-proxy exposure. It never discloses internal UUIDs - every response uses a
randomly-generated `public_id` instead. Only four endpoints are available:
`POST /jobs`, `GET /jobs/{public_id}`, `GET /jobs/{public_id}/result`,
`GET /locales`. Admin paths are unreachable at this prefix, enforced server-side.

Two matching UIs sit alongside the APIs:

| Path prefix | Description |
|---|---|
| `/` | Admin UI - full job/batch/domain/tag management |
| `/public/` | Public UI - single-page app for end-user zone testing |

The full admin API contract is defined in [openapi.yaml](openapi.yaml).

The default resolver profile is built into the binary (`share/profile.json`).
It enables IPv4+IPv6 with `resolver.defaults.parallel=8` and `resolver.defaults.unordered=true`.
Use `--profile` to override if you lack IPv6 or need deterministic ordered output.

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

### Stored profiles

Beyond the file pointed to by `profile_path`, the server can store named profiles
in the database. Stored profiles can be referenced by id or by name from jobs,
batches, and tags, and they are managed via the admin UI under
**Settings → Profiles** or via the `/api/v1/profiles` endpoints.

#### Override-only model

Stored profiles are **sparse overrides** on top of the engine default profile,
not full snapshots. A stored profile JSON only contains the keys you want to
change; everything else is inherited from the built-in default at run time.

For example, a stored profile that just shortens the resolver timeout looks
like this:

```json
{
  "resolver": { "defaults": { "timeout": 5 } }
}
```

When this profile is used for a job, the engine starts from the default profile
and applies these keys on top of it. Properties absent from the stored JSON —
`net.ipv4`, `net.ipv6`, `test_cases`, `test_levels`, etc. — keep their default
values automatically.

This means you do **not** need to copy the full default profile into every
stored profile. A common pattern is to keep stored profiles as small as
possible: each one expresses *only* the difference from the default.

The server validates the stored JSON against the engine profile schema on
create and update, so invalid keys or types are rejected with a `400` error.

#### `schema_version` and engine upgrades

Each stored profile carries a `schema_version` field that records the
gonemaster engine version that was current the last time the profile was
created, edited, or explicitly marked reviewed. The server bumps it
automatically on every successful create, update, or PATCH.

When you upgrade gonemaster, the engine default profile may add new test
cases, new test level tags for existing modules, or other new properties.
A stored profile that explicitly sets one of these properties — for example,
a custom `test_cases` array — will *not* automatically pick up new entries
the upgraded engine introduces, because explicit overrides win over defaults.

To make this visible, the server provides compatibility endpoints that
compare each stored profile against the current engine default and report
what is missing:

- `GET /api/v1/profiles/compatibility` — batch summary of all stored profiles
  (`compatible`, `issue_count` per profile).
- `GET /api/v1/profiles/{id}/compatibility` — detailed issue list for one
  profile, with one entry per missing test case set or per `test_levels`
  module that is missing tags.
- `GET /api/v1/profiles/defaults` — the current engine default `test_cases`
  list and `test_levels` map, useful as a reference when editing.

The admin UI surfaces this in three places:

1. A **Needs review** badge on every profile row in the library that has
   any compatibility issues.
2. A summary line above the profile list — *N profile(s) need review* — with
   a **Mark all as reviewed** button.
3. A warning banner in the profile editor with action buttons to fix or
   acknowledge the issues for the open profile.

#### Upgrade workflow

The recommended workflow after upgrading gonemaster is:

1. Restart `gonemaster-server` against the new binary.
2. Open the admin UI **Settings → Profiles** page. Profiles whose
   `schema_version` no longer matches the new engine version and that have
   actual gaps against the new defaults will show the **Needs review** badge.
3. For each flagged profile, open it in the editor. The compatibility
   banner lists the specific gaps (missing test cases, missing `test_levels`
   tags) and offers fix actions:
   - **Add missing test cases** — appends new defaults to the stored
     `test_cases` array.
   - **Add missing test level tags** — fills missing tags in already-overridden
     `test_levels` modules using the new default severities.
   - **Reset test_cases to inherit** — removes the `test_cases` override
     entirely so the profile inherits whatever the engine default carries.
   - **Reset {module} to inherit** — removes one module from the
     `test_levels` override.
   - **Mark as reviewed** — leaves the config alone but bumps `schema_version`
     so the warning goes away. Use this when you have audited the gaps and
     decided the existing override is intentional.
4. If you have many profiles and want to acknowledge them all at once
   without reading each in detail, use **Mark all as reviewed** from the
   profile list summary bar. This bumps every stored profile's
   `schema_version` to the current engine version.

The compatibility check is purely a UI hint — gonemaster will continue to
run jobs with the existing stored profile JSON until you change it. Nothing
breaks if you ignore the warning; you just may not be exercising newer test
cases the engine ships with.

### Database

The server supports pluggable storage backends selected by `--db-driver`:

| Driver | Description |
|---|---|
| `memory` | In-memory only (default). All data lost on restart. |
| `sqlite` | Embedded SQLite database. Recommended for single-server production use. |
| `postgres` | PostgreSQL 13+. |
| `mariadb` | MariaDB 10.6+ / MySQL 8.0+. |

#### DSN formats

**SQLite** - file path:
```
gonemaster-server --db-driver sqlite --db-dsn /var/lib/gonemaster/gonemaster.db
```

**PostgreSQL** - connection URL:
```
gonemaster-server --db-driver postgres \
  --db-dsn "postgres://user:pass@host:5432/dbname?sslmode=disable"
```

**MariaDB / MySQL** - DSN string:
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
| `sqlite` | 1 (serialised writes) | - | - |
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

- `0` (default) - keep forever, no automatic purge.
- Any positive value starts a background purge loop that runs **hourly** and deletes completed jobs with `finished_at` older than that many days, along with their results.
- Running, queued, and paused jobs are never purged automatically.

Recommended production setting: `90` days.

### Public API

The server exposes a separate, restricted API at `/pub/api/v1/` intended for
reverse-proxy exposure to untrusted clients (see [Overview](#overview) for the
full admin vs. public comparison). It supports job submission and result lookup
by an opaque public ID; internal UUIDs are never disclosed.

Available endpoints:

| Method | Path | Description |
|---|---|---|
| `POST` | `/pub/api/v1/jobs` | Submit a job (returns `public_id`, not internal UUID) |
| `GET` | `/pub/api/v1/jobs/{public_id}` | Poll job status by public ID |
| `GET` | `/pub/api/v1/jobs/{public_id}/result` | Fetch result by public ID |
| `GET` | `/pub/api/v1/locales` | List available locale codes |

Admin-only paths (`/metrics`, `/queue/*`, `/batches`, `/jobs/purge`, `/domains`, `/tags`,
`/runs`, `/entries`) are not reachable via the `/pub/` prefix - the boundary is enforced
server-side.

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

The server enforces the boundary internally - no additional path filtering
is required in the proxy.

#### nginx

```nginx
server {
    listen 443 ssl;
    server_name dns.example.com;

    # HSTS — set at the proxy since the app may also serve plain HTTP internally
    add_header Strict-Transport-Security "max-age=31536000; includeSubDomains" always;

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

All other security headers (`X-Content-Type-Options`, `X-Frame-Options`,
`Content-Security-Policy`, `Referrer-Policy`, `Permissions-Policy`) are set
by the application on every response. Only `Strict-Transport-Security` belongs
at the proxy layer, since the app may also be reached over plain HTTP internally.

#### Caddy

```caddyfile
dns.example.com {
    # Caddy adds HSTS automatically when it manages TLS.
    # If terminating TLS elsewhere, add it explicitly:
    # header Strict-Transport-Security "max-age=31536000; includeSubDomains"
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

This section describes the **admin API** (`/api/v1/`). For the public API, see
[Public API](#public-api) under Configuration.

- Base URL: the server listen address plus `/api/v1` (default `http://127.0.0.1:8080/api/v1`).
- All endpoint paths in the [Endpoints](#endpoints) section below are relative to this base URL.
- Content-Type: JSON for requests and responses.
- CSRF protection: mutating endpoints (`POST`, `PUT`, `DELETE`) validate `Origin` when provided and require it to match the request host. Clients without an `Origin` header (e.g. `gonemaster-client` or `curl`) continue to work unchanged.
- Errors use a standard JSON envelope:
  ```json
  { "error": { "code": "invalid_domain", "message": "..." } }
  ```

## Endpoints

All paths below are relative to `/api/v1/`. This is the **admin API** - do not expose
it to untrusted clients. See [Public API](#public-api) for the internet-safe subset.

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

Jobs submitted via `POST /jobs` (and `POST /pub/api/v1/jobs`) are assigned **priority 0 (normal)**. The `priority` field is included in all job responses.

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

Jobs submitted via `POST /jobs/batch` are assigned **priority 1 (batch)**. Normal-priority jobs always drain before batch-priority jobs, so a large batch run does not delay interactive single-domain submissions.

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
- `tag` - filter to domains belonging to this tag.
- `name` - substring filter on domain name.
- `level` - exact `latest_level` filter (e.g. `ERROR`).
- `min_level` - minimum severity threshold; `WARNING` matches `WARNING`, `ERROR`, `CRITICAL`.
- `limit` / `offset` - pagination (max 500, default 100).

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
- `tag` - filter to runs for domains in this tag.
- `domain` - filter by domain name substring.
- `batch` - filter by batch ID.
- `status` - filter by run status.
- `level` - filter by `worst_level`.
- `finished_after` / `finished_before` - RFC3339 timestamps.
- `limit` / `offset` - pagination (max 500, default 100).

Get a run:
```
GET /runs/{id}
```

Get a run result (same shape as `GET /jobs/{id}/result`):
```
GET /runs/{id}/result?locale=en
```

### Profiles
List stored profiles:
```
GET /profiles
```
Returns an array of stored profiles. Each profile carries a `schema_version`
field recording the engine version that was current at the time of the last
edit. See [Stored profiles](#stored-profiles) for the override-only model.

Get a stored profile by id:
```
GET /profiles/{id}
```

Get the server default profile (built-in default after `profile_path`
overrides are applied — id `0`):
```
GET /profiles/default
```

Get the engine default `test_cases` and `test_levels` (used as the reference
for compatibility checks):
```
GET /profiles/defaults
```
Response shape:
```json
{
  "test_cases": ["basic01", "basic02", "..."],
  "test_levels": {
    "DNSSEC": { "DS_ALGO_NOT_SUPPORTED": "ERROR", "...": "..." }
  }
}
```

Create a stored profile:
```
POST /profiles
{
  "name": "fast-resolver",
  "description": "Shorter timeouts for known-good zones",
  "config": { "resolver": { "defaults": { "timeout": 5 } } },
  "public": false
}
```
Returns `201` with the new profile. The server validates `config` against the
engine profile schema and rejects invalid keys with `400`. Returns `409` with
`error.code=name_exists` if `name` is taken. The new profile is created with
`schema_version` set to the current engine version.

Update a stored profile (full replacement of `config`):
```
PUT /profiles/{id}
{
  "name": "fast-resolver",
  "description": "Shorter timeouts",
  "config": { "resolver": { "defaults": { "timeout": 4 } } },
  "public": false
}
```
Bumps `schema_version` to the current engine version on success.

Apply a targeted compatibility fix to a stored profile:
```
PATCH /profiles/{id}
{ "op": "add_missing_test_cases" }
```
Supported operations:

| `op` | Effect |
|---|---|
| `add_missing_test_cases` | Append any default test cases missing from the profile's `test_cases` array. No-op if the profile does not explicitly set `test_cases`. |
| `add_missing_test_levels` | For each `test_levels` module the profile already overrides, fill any tags present in the default but missing in the override using the default severity. |
| `reset_test_cases` | Remove the `test_cases` override entirely so the profile inherits all defaults. |
| `reset_test_levels` | Remove one module from the `test_levels` override. Requires `"module": "DNSSEC"` (or whichever module). |
| `mark_reviewed` | Leave the config unchanged but bump `schema_version` so the compatibility warning goes away. |

All PATCH operations bump `schema_version` to the current engine version.
Returns the updated profile on `200`.

Get the compatibility status of one stored profile against the current engine
default:
```
GET /profiles/{id}/compatibility
```
Response:
```json
{
  "compatible": false,
  "schema_version": "v0.9.0",
  "current_version": "v1.0.0",
  "issues": [
    {
      "type": "missing_test_case",
      "detail": "Profile sets test_cases but is missing: zone14",
      "suggestion": "Add the missing test case IDs to test_cases, or remove test_cases entirely to inherit all defaults."
    }
  ]
}
```
Issue `type` is one of `missing_test_case`, `missing_test_levels` (with a
`module` field), or `invalid_config`.

Get a batch compatibility summary for all stored profiles:
```
GET /profiles/compatibility
```
Returns an array of `{id, name, compatible, issue_count}` entries — used by the
admin UI to drive the **Needs review** badges and the *N profile(s) need
review* summary line.

Mark every stored profile as reviewed (bumps `schema_version` to the current
engine version on every profile that is not already current):
```
POST /profiles/mark-all-reviewed
```
Returns `{"updated": N}` where `N` is the number of profiles that were bumped.

Delete a stored profile:
```
DELETE /profiles/{id}
```
Returns `204 No Content`.

### Entries
Query individual engine log entries across all runs:
```
GET /entries?run=run_abc&tag=tld&domain=123&module=DNSSEC&testcase=dnssec01&entry_tag=DS_ALGO_NOT_SUPPORTED&level=ERROR&latest=1&batch=batch_123&format=csv&limit=100&offset=0
```

Query params:
- `run` - exact run ID.
- `domain` - domain ID (integer).
- `tag` - domain tag filter (joined via domain→tag associations).
- `module` - exact module name.
- `testcase` - exact testcase name.
- `entry_tag` - log event tag (the engine `tag` field, e.g. `DS_ALGO_NOT_SUPPORTED`).
- `level` - exact severity level.
- `latest` - `1` or `true` to restrict to each domain's latest run only.
- `batch` - restrict to runs from this batch.
- `format=csv` - download as CSV instead of JSON; columns: `id`, `run_id`, `domain_id`, `domain`, `timestamp`, `module`, `testcase`, `tag`, `level`, `args`.
- `limit` / `offset` - pagination (max 500, default 100).

### Queue priority

The queue has two priority tiers. Workers always drain the normal tier before
picking up any batch job.

| Tier | Value | Assigned to |
|---|---|---|
| Normal | `0` | `POST /jobs` (admin UI) and `POST /pub/api/v1/jobs` (public UI) |
| Batch | `1` | `POST /jobs/batch` and `gonemaster-client jobs batch` |

Interactive single-domain submissions — whether from the public UI or the admin
UI — always get priority 0 and are served first. Bulk batch runs get priority 1
and never block a waiting user. Within each tier, jobs are served in FIFO order.

The `priority` field is included in all job and run API responses.

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
The list must contain all currently queued job IDs. Reordering is within-tier only: all normal-priority jobs must appear before all batch-priority jobs. Placing a batch job before a normal job returns `400`.

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
