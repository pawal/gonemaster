# Gonemaster Server

## Overview
- `gonemaster-server` is a REST API wrapper around the Gonemaster engine.
- API contract is defined in `docs/openapi.yaml`.
- Job progress is reported as a percentage (0-100).

## Build
```
go build -o ./gonemaster-server ./cmd/gonemaster-server
```

## Run
```
./gonemaster-server --listen :8080
```

## Configuration
- Config file is optional JSON and loaded with `--config`.
- Flags override config file values.
- `profile_path` sets the default profile used for all jobs (same as `gonemaster --profile`).

## Per-job overrides
- `min_level` can be set per request in `POST /jobs` and `POST /jobs/batch` to override the server default.
- `profile_overrides` are merged on top of `profile_path` when both are provided.

## Batch summary endpoint
- `GET /batches/{batch_id}` returns batch metadata, job list, status counts, and timestamps (`created_at`, optional `started_at`, optional `finished_at`).

Example
```
curl -s http://localhost:8080/batches/batch_123 | jq .
```

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
