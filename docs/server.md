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

### Config example
```json
{
  "listen_addr": ":8080",
  "max_body_size": 1048576,
  "debug": true,
  "worker_count": 4
}
```

## Flags
- `--config` JSON config file path
- `--listen` Address to listen on
- `--max-body-size` Max request body size in bytes
- `--debug` Enable request/response logging
- `--workers` Number of worker goroutines
- `--shutdown-timeout` Graceful shutdown timeout
