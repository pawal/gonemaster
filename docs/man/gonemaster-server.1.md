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
: Capture request/response bodies in the access log and imply **--log-level** *debug*.

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

**--stuck-job-timeout** *N*
: Fail abandoned running jobs after N minutes (0 = disable, default 20).

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
: Storage backend: **memory** (default), **sqlite**, **postgres**, or **mariadb**.

**--db-dsn** *DSN*
: SQLite file path or database connection string.

**--db-retention-days** *N*
: Delete completed jobs older than N days on an hourly schedule. 0 (default) disables automatic purging.

### Reverse proxy

**--trusted-proxy-cidrs** *LIST*
: Comma-separated CIDRs (or bare IPs) of reverse proxies allowed to set **X-Forwarded-For**. Default empty: trust nothing, attribute every request to its **RemoteAddr**. Without this, a direct-exposed server (or one behind a proxy that does not strip incoming XFF) is vulnerable to XFF spoofing - an attacker rotates the header to bypass per-IP rate limits or pin them on a victim. Set to the CIDR of your reverse proxy when one is in front. Example: `--trusted-proxy-cidrs 127.0.0.1/32,10.0.0.0/8`.

### HTTP timeouts

**--read-timeout** *DURATION*
: Per-connection read timeout (default: **30s**). Caps slow / stalled request bodies (slowloris).

**--write-timeout** *DURATION*
: Per-connection write timeout (default: **60s**). Caps slow / stalled responses. Must exceed **--public-api-analysis-request-timeout** (default 10s) so legitimate long analysis responses can complete; widen if you have raised the analysis timeout.

**--idle-timeout** *DURATION*
: Idle keep-alive timeout (default: **60s**).

### Public API

**--public-api-rate-limit-enabled**
: Enable per-IP rate limiting on POST /pub/api/v1/jobs (default: disabled). **Required for internet-facing deployments**: without it, anyone can fill the job queue from a single IP and starve legitimate users.

**--public-api-rate-limit-max** *N*
: Maximum job submissions per IP per window (default: 10).

**--public-api-rate-limit-window** *DURATION*
: Sliding window for rate limiting, e.g. **5m** or **1h** (default: 10m).

**--public-api-allow-private-undelegated-ip**
: Allow undelegated nameserver IPs in loopback / link-local / private / CGNAT / multicast / broadcast ranges on POST /pub/api/v1/jobs (default: refused). Internet-facing deployments must leave this off so the public API cannot be used as an internal-network SSRF probe via the engine's outbound DNS queries. Enable on private/internal deployments that legitimately need to test such targets.

**--public-api-allow-non-global-targets**
: Permit querying non-globally-reachable nameserver addresses (default: off). When off, the engine guard is clamped on for every job so no caller-selected profile can relax it, covering private addresses learned from glue or DNS resolution that the admission check above cannot see. Enable on private/internal deployments. Complements, and does not replace, **--public-api-allow-private-undelegated-ip**; a public instance that runs private undelegated tests must set both.

### Output

**--min-level** *LEVEL*
: Minimum result log level (default: INFO).

### Logging

**--log-format** *FORMAT*
: Operational log encoding: **text** (default, human-readable) or **json** (one object per line, for aggregation).

**--log-level** *LEVEL*
: Minimum operational log level: **debug**, **info** (default), **warn**, or **error**. Independent of **--min-level**, which governs the DNS test result data.

## ENVIRONMENT

**GONEMASTER_LISTEN**
: Equivalent to **--listen**.

**GONEMASTER_WORKER_COUNT**
: Equivalent to **--workers**.

**GONEMASTER_MAX_CONCURRENT_JOBS**
: Equivalent to **--max-concurrent-jobs**.

**GONEMASTER_STUCK_JOB_TIMEOUT**
: Equivalent to **--stuck-job-timeout**.

**GONEMASTER_MIN_LEVEL**
: Equivalent to **--min-level**.

**GONEMASTER_PROFILE**
: Equivalent to **--profile**.

**GONEMASTER_DEBUG**
: Equivalent to **--debug**.

**GONEMASTER_LOG_FORMAT**
: Equivalent to **--log-format**.

**GONEMASTER_LOG_LEVEL**
: Equivalent to **--log-level**.

**GONEMASTER_DB_DRIVER**
: Equivalent to **--db-driver**.

**GONEMASTER_DB_DSN**
: Equivalent to **--db-dsn**.

**GONEMASTER_DB_RETENTION_DAYS**
: Equivalent to **--db-retention-days**.

**GONEMASTER_PUBLIC_API_RATE_LIMIT_ENABLED**
: Equivalent to **--public-api-rate-limit-enabled**.

**GONEMASTER_PUBLIC_API_RATE_LIMIT_MAX**
: Equivalent to **--public-api-rate-limit-max**.

**GONEMASTER_PUBLIC_API_RATE_LIMIT_WINDOW**
: Equivalent to **--public-api-rate-limit-window**.

**GONEMASTER_PUBLIC_API_ALLOW_PRIVATE_UNDELEGATED_IP**
: Equivalent to **--public-api-allow-private-undelegated-ip**.

**GONEMASTER_PUBLIC_API_ALLOW_NON_GLOBAL_TARGETS**
: Equivalent to **--public-api-allow-non-global-targets**.

**GONEMASTER_TRUSTED_PROXY_CIDRS**
: Equivalent to **--trusted-proxy-cidrs**.

**GONEMASTER_READ_TIMEOUT**
: Equivalent to **--read-timeout**.

**GONEMASTER_WRITE_TIMEOUT**
: Equivalent to **--write-timeout**.

**GONEMASTER_IDLE_TIMEOUT**
: Equivalent to **--idle-timeout**.

## CONFIG FILE

The **--config** file is JSON with optional fields:

    {
      "listen_addr": "127.0.0.1:8080",
      "worker_count": 4,
      "max_concurrent_jobs": 0,
      "debug": false,
      "min_level": "INFO",
      "log_format": "text",
      "log_level": "info",
      "profile_path": "",
      "database": {
        "driver": "sqlite",
        "dsn": "/var/lib/gonemaster/db.sqlite",
        "retention_days": 90
      },
      "public_api": {
        "rate_limit_enabled": true,
        "rate_limit_max": 10,
        "rate_limit_window": "10m",
        "allow_private_undelegated_ip": false
      },
      "trusted_proxy_cidrs": ["127.0.0.1/32"],
      "read_timeout": "30s",
      "write_timeout": "60s",
      "idle_timeout": "60s"
    }

## FILES

When installed from a distribution package, the server is wired up with these
paths. Standalone builds use whatever paths you pass via flags or env vars.

**/usr/bin/gonemaster-server**
: The server binary.

**/etc/gonemaster/server.env**
: Environment file read by the systemd unit. Sets **GONEMASTER_LISTEN**,
  **GONEMASTER_DB_DSN**, and other knobs. Marked as a config file, so package
  upgrades will not overwrite operator edits.

**/lib/systemd/system/gonemaster-server.service**
: Systemd unit that runs the server as the **gonemaster** system user with
  hardening flags and **EnvironmentFile=/etc/gonemaster/server.env**.

**/var/lib/gonemaster/**
: Runtime state directory, mode 0750, owned by the **gonemaster** user.
  Default location for the SQLite database when **GONEMASTER_DB_DRIVER=sqlite**.

**/usr/lib/tmpfiles.d/gonemaster.conf**
: tmpfiles.d snippet that recreates **/var/lib/gonemaster/** with the right
  ownership and permissions on boot.

**/usr/share/gonemaster/badkeys/blocklist.dat**, **badkeysdata.json**
: Compromised-key blocklist data shipped by the **gonemaster-badkeys-data**
  package. The server looks here via XDG_DATA_DIRS when no embedded data or
  user-level dataset is available.

## EXAMPLES

Start with defaults (in-memory, 4 workers):

    gonemaster-server

Start with SQLite persistence:

    gonemaster-server --db-driver sqlite --db-dsn /var/lib/gonemaster/db.sqlite

Start with SQLite and 90-day retention:

    gonemaster-server --db-driver sqlite --db-dsn /var/lib/gonemaster/db.sqlite \
        --db-retention-days 90

Start with a config file:

    gonemaster-server --config /etc/gonemaster/server.json

Listen on all interfaces with 8 workers:

    gonemaster-server --listen 0.0.0.0:8080 --workers 8

## SEE ALSO

**gonemaster**(1), **gonemaster-client**(1), **gonemaster-nagios**(1)
