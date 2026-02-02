# Gonemaster Server

## Overview
- `gonemaster-server` is a REST API wrapper around the Gonemaster engine.
- API contract is defined in `docs/openapi.yaml`.

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

## Per-job overrides
- `min_level` can be set per request in `POST /jobs` and `POST /jobs/batch` to override the server default.

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
  "min_level": "INFO"
}
```

## Flags
- `--config` JSON config file path
- `--listen` Address to listen on
- `--max-body-size` Max request body size in bytes
- `--debug` Enable request/response logging
- `--workers` Number of worker goroutines
- `--min-level` Minimum log level for results
- `--shutdown-timeout` Graceful shutdown timeout
