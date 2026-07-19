# Server Configuration

This page owns the `gonemaster-server` configuration model.

## Precedence

Configuration is applied in this order:

1. Command-line flags
2. `GONEMASTER_*` environment variables
3. JSON config file passed with `--config`
4. Built-in defaults

Use environment variables for secrets such as database connection strings.

Print the effective config and exit:

```sh
gonemaster-server --dump-config
```

## Core Settings

| Setting | Purpose |
|---|---|
| `listen_addr` | Address and port for the HTTP listener. |
| `max_body_size` | Maximum request body size. |
| `worker_count` | Number of workers that dequeue jobs. |
| `max_concurrent_jobs` | Maximum number of engine runs at once. |
| `cross_job_hot_cache` | Enables cross-job nameserver cache sharing. |
| `cross_job_hot_cache_ttl_seconds` | TTL for cross-job hot-cache entries. |
| `min_level` | Minimum log level stored and returned in results. |
| `profile_path` | Default engine profile file. |
| `public_url` | Canonical public base URL for public pages, robots, and sitemap. |
| `scoring_config_path` | Optional JSON scoring configuration file. |
| `debug` | Captures request/response bodies in the access log and implies `log_level=debug`. |
| `log_format` | Operational log encoding: `text` (default, human-readable) or `json` (one object per line, for aggregation). |
| `log_level` | Minimum operational log level: `debug`, `info` (default), `warn`, or `error`. |
| `trusted_proxy_cidrs` | CIDRs (or bare IPs) of reverse proxies allowed to set `X-Forwarded-For`. Empty (default) trusts nothing and uses `RemoteAddr`. See [public-api-and-proxy.md](public-api-and-proxy.md). |
| `read_timeout` | Per-connection read timeout (default 30s). |
| `write_timeout` | Per-connection write timeout (default 60s). Must exceed `public_api.analysis_request_timeout`. |
| `idle_timeout` | Idle keep-alive timeout (default 60s). |
| `public_api.allow_private_undelegated_ip` | Allow loopback / link-local / private / CGNAT / multicast / broadcast IPs as undelegated NS targets on the public API. Default `false`; enable on private/internal deployments. |
| `public_api.allow_non_global_targets` | Permit querying non-globally-reachable nameserver addresses. Default `false`, which clamps the engine guard on for every job so no caller-selected profile can relax it; set `true` on private/internal deployments. Complements (does not replace) `allow_private_undelegated_ip`: that flag is admission-time input validation, this is the query-time guard. A public instance that wants to run private undelegated tests must set both. |

## Environment Variables

| Variable | Config field |
|---|---|
| `GONEMASTER_LISTEN` | `listen_addr` |
| `GONEMASTER_WORKER_COUNT` | `worker_count` |
| `GONEMASTER_MAX_CONCURRENT_JOBS` | `max_concurrent_jobs` |
| `GONEMASTER_MIN_LEVEL` | `min_level` |
| `GONEMASTER_PROFILE` | `profile_path` |
| `GONEMASTER_DEBUG` | `debug` |
| `GONEMASTER_LOG_FORMAT` | `log_format` |
| `GONEMASTER_LOG_LEVEL` | `log_level` |
| `GONEMASTER_DB_DRIVER` | `database.driver` |
| `GONEMASTER_DB_DSN` | `database.dsn` |
| `GONEMASTER_DB_RETENTION_DAYS` | `database.retention_days` |
| `GONEMASTER_PUBLIC_API_RATE_LIMIT_ENABLED` | `public_api.rate_limit_enabled` |
| `GONEMASTER_PUBLIC_API_RATE_LIMIT_MAX` | `public_api.rate_limit_max` |
| `GONEMASTER_PUBLIC_API_RATE_LIMIT_WINDOW` | `public_api.rate_limit_window` |
| `GONEMASTER_PUBLIC_API_ALLOW_PRIVATE_UNDELEGATED_IP` | `public_api.allow_private_undelegated_ip` |
| `GONEMASTER_PUBLIC_API_ALLOW_NON_GLOBAL_TARGETS` | `public_api.allow_non_global_targets` |
| `GONEMASTER_TRUSTED_PROXY_CIDRS` | `trusted_proxy_cidrs` (comma-separated) |
| `GONEMASTER_READ_TIMEOUT` | `read_timeout` |
| `GONEMASTER_WRITE_TIMEOUT` | `write_timeout` |
| `GONEMASTER_IDLE_TIMEOUT` | `idle_timeout` |
| `GONEMASTER_CROSS_JOB_HOT_CACHE` | `cross_job_hot_cache` |
| `GONEMASTER_CROSS_JOB_HOT_CACHE_TTL` | `cross_job_hot_cache_ttl_seconds` |

Invalid integer, boolean, or duration values emit a warning and are ignored.

## Flags

Common flags:

```text
--config PATH
--listen ADDR
--max-body-size BYTES
--debug
--dump-config
--version
--shutdown-timeout DURATION
--workers N
--max-concurrent-jobs N
--cross-job-hot-cache
--no-cross-job-hot-cache
--cross-job-hot-cache-ttl N
--profile PATH
--min-level LEVEL
--log-format text|json
--log-level debug|info|warn|error
--trusted-proxy-cidrs LIST
--read-timeout DURATION
--write-timeout DURATION
--idle-timeout DURATION
--public-api-allow-private-undelegated-ip
```

Resolver override flags:

```text
--positive-cache-ttl N
--negative-cache-ttl N
--timeout N
--retry N
--retrans N
--fallback
--no-fallback
--sourceaddr4 IPADDR
--sourceaddr6 IPADDR
```

Database and public API flags are covered in [database.md](database.md) and
[public-api-and-proxy.md](public-api-and-proxy.md).

## Config File Example

```json
{
  "listen_addr": "127.0.0.1:8080",
  "max_body_size": 1048576,
  "debug": false,
  "worker_count": 16,
  "max_concurrent_jobs": 16,
  "cross_job_hot_cache": true,
  "cross_job_hot_cache_ttl_seconds": 60,
  "timeout": 5,
  "retry": 2,
  "retrans": 3,
  "fallback": true,
  "min_level": "INFO",
  "log_format": "text",
  "log_level": "info",
  "profile_path": "/etc/gonemaster/profile.json",
  "database": {
    "driver": "sqlite",
    "dsn": "/var/lib/gonemaster/gonemaster.db",
    "retention_days": 90
  },
  "public_api": {
    "rate_limit_enabled": true,
    "rate_limit_max": 10,
    "rate_limit_window": "10m",
    "allow_private_undelegated_ip": false,
    "allow_non_global_targets": false
  },
  "trusted_proxy_cidrs": ["127.0.0.1/32"],
  "read_timeout": "30s",
  "write_timeout": "60s",
  "idle_timeout": "60s"
}
```

## Profiles

The server has two profile sources:

- A process-wide base profile from the built-in default plus `profile_path`.
- Stored profiles in the database, referenced by jobs, batches, public
  profiles, and tag defaults.

Stored profiles are sparse overrides. They contain only the settings that
differ from the engine default.

Example stored profile config:

```json
{
  "resolver": {
    "defaults": {
      "timeout": 5
    }
  }
}
```

Stored profiles can be referenced from jobs, batches, public profiles, and tag
defaults. The server validates stored profile JSON on create and update.

See the [Profile Settings Reference](../profile-settings.md) for every
`resolver.defaults` knob, its default, range, and CLI flag.

## Profile Compatibility

Stored profiles record the engine schema version used when they were last
edited. When engine defaults gain new test cases or test-level tags, the
compatibility endpoints and admin UI can show profiles that need review.

Available repair operations include:

- add missing test cases
- add missing test-level tags
- reset test cases to inherited defaults
- reset one test-level module
- mark a profile as reviewed

## Deterministic Resolver Behavior

The built-in profile uses parallel and unordered resolver behavior for speed.
For deterministic ordered output, use a profile with:

```json
{
  "resolver": {
    "defaults": {
      "unordered": false,
      "parallel": 1
    }
  }
}
```

## Bounding Slow Nameservers

By default the engine waits out slow or unresponsive nameservers, and the verdict
reflects that slowness. Two profile settings can cap the wall-clock cost:

- `resolver.defaults.fast_fail_timeout_count` skips a nameserver after this many
  consecutive timeouts on a protocol (default 3, 0 disables). It only reacts to
  silence.
- `resolver.defaults.nameserver_max_total_ms` skips a nameserver address once the
  cumulative time spent querying it in a run exceeds this many milliseconds
  (default 0 = disabled). Unlike fast-fail it also bounds slow-but-responding
  servers, whose successes keep resetting the consecutive-timeout count.

The latency budget trades query coverage for speed: once an address is skipped
the run gathers less data about its zone, which can lower the grade for very slow
zones. It is therefore off by default. A conservative starting point is 60000 to
120000 ms (60-120 s); lower values such as 30000 ms engage sooner and affect more
zones.

Apply it process-wide through `profile_path`:

```json
{
  "resolver": {
    "defaults": {
      "nameserver_max_total_ms": 60000
    }
  }
}
```

Or scope it to specific runs by saving a stored profile with the same sparse
override (Admin UI, Profiles) and selecting it for a job or batch, for example a
profile applied to the TLD cohort. Each run records its effective profile, so you
can confirm the value took effect.

## Result Display Settings

The config file can hide score and nameserver timing UI elements:

```json
{
  "show_score_admin": true,
  "show_score_public": true,
  "show_nameserver_timings_admin": true,
  "show_nameserver_timings_public": true
}
```

These settings affect UI display. They do not remove stored data.

## Operational Logging

`log_format` and `log_level` control the server's operational logs (lifecycle,
access log, warnings, errors). They are independent of `min_level`, which governs
the DNS test result data. Set `log_format=json` for log aggregation and pick a
`log_level` floor of `debug`, `info`, `warn`, or `error`.

`--debug` (or `debug: true`) captures request/response bodies in the access log
and implies `log_level=debug`; leave it off in production so bodies are not
logged.

Every `/api/v1` and `/pub/api/v1` request gets an `X-Request-Id`. The server
generates one by default and echoes it in the response header. An inbound
`X-Request-Id` is honored only when the request comes from a `trusted_proxy_cidrs`
peer; from any other client it is ignored and a fresh ID is generated, so the ID
cannot be spoofed on a directly exposed server.

See [operations.md](operations.md) for the log formats, the access-log fields,
and shipping logs to journald or Loki.

## Related Pages

- Database config and DSNs: [database.md](database.md)
- Throughput tuning: [performance.md](performance.md)
- Stored profile endpoints: [../specifications/api.md](../specifications/api.md)
