# Gonemaster Server

## Overview
- `gonemaster-server` is a REST API wrapper around the Gonemaster engine with an embedded web UI.
- The API contract is defined in [openapi.yaml](openapi.yaml).
- The UI is served at `/` from the embedded build output in `server/ui/dist`.
- The API is served under `/api/v1`.
- Job progress is reported as a percentage (0-100).

When running the API and the UI, please note that the default profile is set to also use IPv6.
If you don't have access to IPv6 on your development machine, you must modify the profile.
You can dump the profile using `--dump-profile` in the CLI, and assign the file to the
server with the `--profile` option.

The default profile is located in `share/profile.json`, but is built into the server binary.

The current server lacks any sort of persistence, for example using SQLite or PostgreSQL.

## Build
```
go build -o ./gonemaster-server ./cmd/gonemaster-server
```

To rebuild the embedded UI:
```
make ui-build
```
Note: building the UI requires `npm` to be available in your PATH.

## Quick start
Start the server:
```
./gonemaster-server --listen :8080
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
  if [ "$STATUS" = "succeeded" ] || [ "$STATUS" = "failed" ] || [ "$STATUS" = "canceled" ]; then
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
./gonemaster-server --listen :8080
```

## Configuration
- Config file is optional JSON and loaded with `--config`.
- Flags override config file values.
- `profile_path` sets the default profile used for all jobs (same as `gonemaster --profile`).

### Config example
```json
{
  "listen_addr": ":8080",
  "max_body_size": 1048576,
  "debug": true,
  "worker_count": 4,
  "min_level": "INFO",
  "profile_path": "/path/to/profile.json"
}
```

## Flags
- `--config` JSON config file path
- `--listen` Address to listen on
- `--max-body-size` Max request body size in bytes
- `--debug` Enable request/response logging
- `--workers` Number of worker goroutines
- `--min-level` Minimum log level for results
- `--profile` Profile JSON/YAML path (default for all jobs)
- `--shutdown-timeout` Graceful shutdown timeout

## Domain normalization (IDN)
Domains are normalized to IDNA A-labels (punycode). For example:
`räksmörgås.se` becomes `xn--rksmrgs-5wao1o.se`.
Invalid domains return a `400` error with `code=invalid_domain`.

## API basics
- Base URL: the server listen address plus `/api/v1` (default `http://localhost:8080/api/v1`).
- All endpoint paths below are relative to the base URL.
- Content-Type: JSON for requests and responses.
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
  "min_level": "NOTICE",
  "profile_overrides": { "timeout": 5 }
}
```

List jobs:
```
GET /jobs?status=running&limit=100
```

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

### Ops
```
GET /metrics
GET /healthz
```

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
