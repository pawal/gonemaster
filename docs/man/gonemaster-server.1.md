# gonemaster-server 1 "2026" "gonemaster" "User Commands"

## NAME

gonemaster-server - HTTP API server for DNS zone testing

## SYNOPSIS

**gonemaster-server** [*OPTIONS*]

## DESCRIPTION

**gonemaster-server** runs a persistent HTTP server that accepts DNS zone test
requests via a REST API. It manages a job queue, worker pool, and optional
persistent storage. A web UI is embedded by default.

Configuration is resolved in order: defaults, JSON config file, environment
variables, CLI flags. Later sources override earlier ones.

## OPTIONS

### General

**--config** *PATH*
: Load configuration from a JSON file.

**--listen** *ADDR*
: Address to listen on (default: 127.0.0.1:8080).

**--max-body-size** *BYTES*
: Maximum request body size (default: 1048576).

**--debug**
: Enable request/response logging.

**--shutdown-timeout** *DURATION*
: Graceful shutdown timeout (default: 10s).

**--dump-config**
: Print effective configuration as JSON and exit.

**--version**
: Print version information and exit.

### Concurrency

**--workers** *N*
: Number of worker goroutines (default: 4).

**--max-concurrent-jobs** *N*
: Maximum concurrent engine runs (0 = unlimited).

### Resolver

**--profile** *PATH*
: Load a custom profile from a JSON or YAML file.

**--timeout** *SECONDS*
: Query timeout in seconds.

**--retry** *N*
: Number of query retries.

**--retrans** *SECONDS*
: Retransmission interval in seconds.

**--fallback**
: Enable TCP fallback on UDP failure.

**--no-fallback**
: Disable TCP fallback on UDP failure.

**--sourceaddr4** *IPADDR*
: Source IPv4 address for outgoing queries.

**--sourceaddr6** *IPADDR*
: Source IPv6 address for outgoing queries.

**--positive-cache-ttl** *SECONDS*
: Cache positive DNS responses for this duration.

**--negative-cache-ttl** *SECONDS*
: Cache negative DNS responses for this duration.

### Database

**--db-driver** *DRIVER*
: Storage backend: **sqlite** or empty for in-memory.

**--db-dsn** *DSN*
: SQLite file path or database connection string.

### Output

**--min-level** *LEVEL*
: Minimum result log level (default: INFO).

## ENVIRONMENT

**GONEMASTER_LISTEN**
: Equivalent to **--listen**.

**GONEMASTER_WORKER_COUNT**
: Equivalent to **--workers**.

**GONEMASTER_MAX_CONCURRENT_JOBS**
: Equivalent to **--max-concurrent-jobs**.

**GONEMASTER_MIN_LEVEL**
: Equivalent to **--min-level**.

**GONEMASTER_PROFILE**
: Equivalent to **--profile**.

**GONEMASTER_DEBUG**
: Equivalent to **--debug**.

**GONEMASTER_DB_DRIVER**
: Equivalent to **--db-driver**.

**GONEMASTER_DB_DSN**
: Equivalent to **--db-dsn**.

## CONFIG FILE

The **--config** file is JSON with optional fields:

    {
      "listen_addr": "127.0.0.1:8080",
      "worker_count": 4,
      "max_concurrent_jobs": 0,
      "debug": false,
      "min_level": "INFO",
      "profile_path": "",
      "database": {
        "driver": "sqlite",
        "dsn": "/var/lib/gonemaster/db.sqlite"
      }
    }

## EXAMPLES

Start with defaults (in-memory, 4 workers):

    gonemaster-server

Start with SQLite persistence:

    gonemaster-server --db-driver sqlite --db-dsn /var/lib/gonemaster/db.sqlite

Start with a config file:

    gonemaster-server --config /etc/gonemaster/server.json

Listen on all interfaces with 8 workers:

    gonemaster-server --listen 0.0.0.0:8080 --workers 8

## SEE ALSO

**gonemaster**(1), **gonemaster-client**(1), **gonemaster-nagios**(1)
