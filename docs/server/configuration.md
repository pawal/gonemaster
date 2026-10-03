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

## Runtime Settings

The admin UI Settings page and `PUT /api/v1/settings` store settings in the
database. A stored setting overrides the config file and the environment, but
not a command-line flag, and takes effect without a restart. This includes the
rate limits and `public_url` on `/public/` and `/analysis/`.

`PUT /api/v1/settings` takes a JSON object of setting names and values. The
server stores nothing and answers `400` when a name is read-only
(`readonly_setting`), when a name is not a runtime setting
(`unknown_setting`), or when a value is outside its type or range
(`invalid_setting`):

| Setting | Accepted values |
|---|---|
| `worker_count` | Integer 1 to 1024. |
| `max_concurrent_jobs` | Integer 0 to 1024. |
| `stuck_job_timeout_minutes` | Integer 0 to 525600. |
| `retention_days` | Integer 0 to 36500. |
| `purge_interval_seconds` | Integer 1 to 31536000. |
| `cross_job_hot_cache_ttl_seconds` | Integer 1 to 86400. |
| `rate_limit_max`, `rate_limit_get_max` | Integer 1 to 100000. |
| `rate_limit_window` | Duration above zero, such as `1m`. |
| `min_level` | A log level name. |
| `ipv6_mode` | `auto`, `on` or `off`. |
| `public_url` | As in [Core Settings](#core-settings). |
| `rate_limit_enabled`, `allow_private_undelegated_ip`, `allow_non_global_targets`, `show_score_admin`, `show_score_public`, `show_nameserver_timings_admin`, `show_nameserver_timings_public`, `show_dnssec_chain_public`, `show_asn_names_public`, `mcp_enabled`, `mcp_allow_write` | `true` or `false`. |

The read-only settings are `listen_addr`, `db_driver`, `db_dsn`,
`profile_path`, and every setting set by a command-line flag.
`GET /api/v1/settings` reports such a setting with its effective value, source
`cli_flag` and `readonly` true, and the Settings page cannot change it. The
server checks stored settings against the same ranges at startup and after each
write, and logs and ignores a stored value outside them.

## Core Settings

| Setting | Purpose |
|---|---|
| `listen_addr` | Address and port for the HTTP listener. |
| `max_body_size` | Maximum request body size. |
| `worker_count` | Number of workers that dequeue jobs. |
| `max_concurrent_jobs` | Maximum number of engine runs at once. |
| `stuck_job_timeout_minutes` | Fail abandoned running jobs after this many minutes (0 = disable). |
| `cross_job_hot_cache` | Enables cross-job nameserver cache sharing. |
| `cross_job_hot_cache_ttl_seconds` | TTL for cross-job hot-cache entries. |
| `min_level` | Minimum log level stored and returned in results. |
| `ipv6_mode` | IPv6 in test runs. `auto` (default): before each run the engine checks that the host has a route to a global IPv6 address from a source address that is not link-local, and turns IPv6 off for the run without one, logging `IPV6_AUTO_DISABLED`. The check opens a UDP socket and sends nothing. `on`: no check; `net.ipv6` of the profile applies. `off`: no run sends IPv6 queries. A job that disables IPv6 keeps it off in every mode. |
| `profile_path` | Default engine profile file. |
| `exclude` | Testcase ids and module names that no run executes. See [Excluded testcases](#excluded-testcases). |
| `public_url` | Site root the deployment answers at, e.g. `https://example.com/`. Also builds `og:image`, the API examples, `robots.txt` and `sitemap.xml`, so it is the root and not the public UI's own URL. MUST be empty or an absolute `http` or `https` URL without userinfo, query, fragment, whitespace, control characters, quotes, angle brackets, backslash or backtick; the server refuses to start otherwise. |
| `public_ui_path` | Path under `public_url` where visitors reach the public UI. Default `public/`; set `""` when a proxy serves it at the root. Does not move the server's own mount. |
| `scoring_config_path` | Optional JSON scoring configuration file. |
| `debug` | Captures request/response bodies in the access log and implies `log_level=debug`. |
| `log_format` | Operational log encoding: `text` (default, human-readable) or `json` (one object per line, for aggregation). |
| `log_level` | Minimum operational log level: `debug`, `info` (default), `warn`, or `error`. |
| `trusted_proxy_cidrs` | CIDRs (or bare IPs) of reverse proxies allowed to set `X-Forwarded-For`. Empty (default) trusts nothing and uses `RemoteAddr`. See [public-api-and-proxy.md](public-api-and-proxy.md). |
| `read_timeout` | Per-connection read timeout (default 30s). |
| `write_timeout` | Per-connection write timeout (default 60s). Must exceed `public_api.analysis_request_timeout`. |
| `idle_timeout` | Idle keep-alive timeout (default 60s). |
| `auth.admin_tokens` | Admin token hashes. Empty (default) is open mode. See [authentication.md](authentication.md). |
| `auth.protect_public` | Requires an admin token on `/public/`, `/analysis/` and `/pub/api/v1/` too. Default `false`; MUST NOT be `true` without `auth.admin_tokens`. See [authentication.md](authentication.md#protect-the-public-surfaces). |
| `public_api.allow_private_undelegated_ip` | Allow loopback / link-local / private / CGNAT / multicast / broadcast IPs as undelegated NS targets on the public API. Default `false`; enable on private/internal deployments. |
| `public_api.allow_non_global_targets` | Permit querying non-globally-reachable nameserver addresses. Default `false`, which clamps the engine guard on for every job so no caller-selected profile can relax it; set `true` on private/internal deployments. Complements (does not replace) `allow_private_undelegated_ip`: that flag is admission-time input validation, this is the query-time guard. A public instance that wants to run private undelegated tests must set both. The guard also covers the RIPE whois sources of the profile `asn_db`, so an internal whois mirror requires `true`. |

## Environment Variables

| Variable | Config field |
|---|---|
| `GONEMASTER_LISTEN` | `listen_addr` |
| `GONEMASTER_WORKER_COUNT` | `worker_count` |
| `GONEMASTER_MAX_CONCURRENT_JOBS` | `max_concurrent_jobs` |
| `GONEMASTER_STUCK_JOB_TIMEOUT` | `stuck_job_timeout_minutes` |
| `GONEMASTER_MIN_LEVEL` | `min_level` |
| `GONEMASTER_IPV6_MODE` | `ipv6_mode` |
| `GONEMASTER_PROFILE` | `profile_path` |
| `GONEMASTER_EXCLUDE` | `exclude` (comma-separated) |
| `GONEMASTER_DEBUG` | `debug` |
| `GONEMASTER_LOG_FORMAT` | `log_format` |
| `GONEMASTER_LOG_LEVEL` | `log_level` |
| `GONEMASTER_DB_DRIVER` | `database.driver` |
| `GONEMASTER_DB_DSN` | `database.dsn` |
| `GONEMASTER_DB_RETENTION_DAYS` | `database.retention_days` |
| `GONEMASTER_PUBLIC_API_RATE_LIMIT_ENABLED` | `public_api.rate_limit_enabled` |
| `GONEMASTER_PUBLIC_API_RATE_LIMIT_MAX` | `public_api.rate_limit_max` |
| `GONEMASTER_PUBLIC_API_RATE_LIMIT_GET_MAX` | `public_api.rate_limit_get_max` |
| `GONEMASTER_PUBLIC_API_RATE_LIMIT_WINDOW` | `public_api.rate_limit_window` |
| `GONEMASTER_PUBLIC_API_ALLOW_PRIVATE_UNDELEGATED_IP` | `public_api.allow_private_undelegated_ip` |
| `GONEMASTER_PUBLIC_API_ALLOW_NON_GLOBAL_TARGETS` | `public_api.allow_non_global_targets` |
| `GONEMASTER_TRUSTED_PROXY_CIDRS` | `trusted_proxy_cidrs` (comma-separated) |
| `GONEMASTER_READ_TIMEOUT` | `read_timeout` |
| `GONEMASTER_WRITE_TIMEOUT` | `write_timeout` |
| `GONEMASTER_IDLE_TIMEOUT` | `idle_timeout` |
| `GONEMASTER_CROSS_JOB_HOT_CACHE` | `cross_job_hot_cache` |
| `GONEMASTER_CROSS_JOB_HOT_CACHE_TTL` | `cross_job_hot_cache_ttl_seconds` |
| `GONEMASTER_EXTERNAL_DATA_ENABLED` | `external_data.enabled` |
| `GONEMASTER_EXTERNAL_DATA_REFRESH_INTERVAL` | `external_data.refresh_interval` |
| `GONEMASTER_EXTERNAL_DATA_RECORD_TTL` | `external_data.record_ttl` |
| `GONEMASTER_EXTERNAL_DATA_NEGATIVE_TTL` | `external_data.negative_ttl` |
| `GONEMASTER_EXTERNAL_DATA_TIMEOUT` | `external_data.timeout` |
| `GONEMASTER_EXTERNAL_DATA_MAX_REQUESTS_PER_MINUTE` | `external_data.max_requests_per_minute` |
| `GONEMASTER_EXTERNAL_DATA_MAX_CACHED_RECORDS` | `external_data.max_cached_records` |
| `GONEMASTER_ANALYSIS_VANTAGE_LABEL` | `analysis.vantage_label` |
| `GONEMASTER_ADMIN_TOKEN_HASHES` | `auth.admin_tokens` (comma-separated `label=sha256:...`) |
| `GONEMASTER_AUTH_PROTECT_PUBLIC` | `auth.protect_public` |

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
--exclude NAME
--min-level LEVEL
--log-format text|json
--log-level debug|info|warn|error
--trusted-proxy-cidrs LIST
--read-timeout DURATION
--write-timeout DURATION
--idle-timeout DURATION
--public-api-allow-private-undelegated-ip
```

External data flags:

```text
--external-data-enabled
--external-data-refresh-interval DURATION
--external-data-record-ttl DURATION
--external-data-negative-ttl DURATION
--external-data-timeout DURATION
--external-data-max-requests-per-minute N
--external-data-max-cached-records N
```

Analysis flags:

```text
--analysis-vantage-label LABEL
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
[public-api-and-proxy.md](public-api-and-proxy.md), and the authentication flags
`--admin-token-hashes` and `--auth-protect-public` in
[authentication.md](authentication.md).

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
  "ipv6_mode": "auto",
  "log_format": "text",
  "log_level": "info",
  "profile_path": "/etc/gonemaster/profile.json",
  "public_url": "https://gonemaster.example/",
  "public_ui_path": "public/",
  "database": {
    "driver": "sqlite",
    "dsn": "/var/lib/gonemaster/gonemaster.db",
    "retention_days": 90
  },
  "public_api": {
    "rate_limit_enabled": true,
    "rate_limit_max": 10,
    "rate_limit_get_max": 600,
    "rate_limit_window": "10m",
    "allow_private_undelegated_ip": false,
    "allow_non_global_targets": false
  },
  "analysis": {
    "tag_view_min_level": "NOTICE",
    "vantage_label": "Stockholm, SE"
  },
  "external_data": {
    "enabled": false
  },
  "trusted_proxy_cidrs": ["127.0.0.1/32"],
  "read_timeout": "30s",
  "write_timeout": "60s",
  "idle_timeout": "60s"
}
```

To proxy the public UI to the site root, set `public_ui_path` to `""` and see
[ui.md](ui.md).

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

### Excluded testcases

`exclude` removes testcases from every run on the instance, as defined in
[Testcase selection](../profile-settings.md#testcase-selection).

Each run stores its effective profile, whose `test_cases` list omits the
excluded testcases. A run diff across a change of `exclude` reports the
findings of a newly excluded testcase as cleared.

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

The config file can hide score, nameserver timing and AS holder UI elements:

```json
{
  "show_score_admin": true,
  "show_score_public": true,
  "show_nameserver_timings_admin": true,
  "show_nameserver_timings_public": true,
  "show_asn_names_public": true
}
```

These settings affect UI display. They do not remove stored data.

`show_asn_names_public` (default `true`) enables
`GET /pub/api/v1/jobs/{public_id}/asn-names`, which the public UI calls to show
the registered holder under each finding that names an AS number. When it is
on, the server queries Team Cymru's IP to ASN mapping service outside test
runs, for the AS numbers of each result that is opened. Set it to `false` to
stop these queries. See
[public-api-and-proxy.md](public-api-and-proxy.md#as-holder-names).

## MCP Settings

```json
{
  "mcp_enabled": false,
  "mcp_allow_write": false
}
```

`mcp_enabled` (default `false`) serves the Model Context Protocol tools at
`POST /api/v1/mcp`, behind the admin token auth. `mcp_allow_write` (default
`false`) also registers the write tools `batch_enqueue`, `batch_cancel`, and
`cancel_job`, for every token holder. Both are in the admin UI under
Settings, MCP, and take effect without a restart. See
[../mcp/server-endpoint.md](../mcp/server-endpoint.md).

## Analysis Settings

The `analysis` block holds capture-time and presentation policy for the
analysis dashboard:

```json
{
  "analysis": {
    "tag_view_min_level": "NOTICE",
    "vantage_label": "Stockholm, SE"
  }
}
```

| Setting | Purpose |
|---|---|
| `tag_view_min_level` | Floor for the tags written into a snapshot's tag view at capture time. A tag whose worst level in the snapshot is below the floor gets no row and no detail page. One of `INFO`, `NOTICE`, `WARNING`, `ERROR`, `CRITICAL`; default `NOTICE`. A cohort can override it. |
| `vantage_label` | Name of the network location the runs were made from. Default empty. |

`vantage_label` changes presentation only. The latency figures come from
queries this instance made, which is one network location; the dashboard says
so either way. With the label set the caveat names the place ("Measured from
Stockholm, SE; a single vantage point."), which matters once results from
several instances are compared. The label is served through the public
catalog, is trimmed, and is ignored if longer than 64 characters.

## External Reference Data

The analysis dashboard can show public registry data next to a measurement:
the RDAP registration record for a domain, and a direct link to the registry's
own RDAP service. The data comes from a provider that fetches on its own
schedule and serves from an in-memory cache. It is off by default:

```json
{
  "external_data": {
    "enabled": false,
    "refresh_interval": "24h",
    "record_ttl": "168h",
    "negative_ttl": "1h",
    "timeout": "10s",
    "max_requests_per_minute": 30,
    "max_cached_records": 20000,
    "sources": {
      "iana_tlds": "https://data.iana.org/TLD/tlds-alpha-by-domain.txt",
      "rdap_bootstrap": "https://data.iana.org/rdap/dns.json"
    }
  }
}
```

| Setting | Purpose |
|---|---|
| `enabled` | Turns the provider on. Default `false`. |
| `refresh_interval` | How often the two IANA datasets are re-fetched. Conditional requests make an unchanged file cost one 304. |
| `record_ttl` | How long a cached per-domain RDAP record is served before it is refreshed. |
| `negative_ttl` | How long a failed fetch suppresses retries for the same object. |
| `timeout` | Per-request timeout for one outbound fetch. |
| `max_requests_per_minute` | Budget shared by all outbound fetches. It bounds what a crawler walking every detail page can make the server do. |
| `max_cached_records` | Cache ceiling for per-domain records; least recently used are dropped first. |
| `sources` | Dataset locations. Point them at a mirror if the deployment cannot reach IANA. |

Enabling this makes the server open outbound HTTPS connections on its own
initiative: to IANA for the two datasets, and to the RDAP service of the
registry behind each domain a visitor opens. The domain names sent are already
public in the cohorts the dashboard serves. Air-gapped and privacy-sensitive
deployments should leave it off.

Fetching is constrained: HTTPS only, redirects followed only to HTTPS, literal
and resolved loopback, link-local and private destinations refused, a 2 MiB
response cap, and a `gonemaster/<version>` user agent. A request for a page
never waits on a third party: a cache miss renders a placeholder and the
record appears on the next request. `GET /api/v1/analysis/status` reports the
dataset freshness, the cached record count, the queue depth, and the last
fetch error.

The cache is memory only. A restart re-fetches the two datasets and repopulates
records as they are viewed.

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
