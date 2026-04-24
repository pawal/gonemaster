# Server Performance

This page covers throughput and latency tuning for server workloads.

## Worker Count

`--workers` controls how many workers dequeue jobs. `--max-concurrent-jobs`
caps how many engine runs execute at the same time. Effective concurrency is
the smaller of those two values.

For DNS-heavy batch workloads, a worker count above CPU core count can help
because much of the runtime is network I/O. Measure before raising it on shared
hosts.

On an 8-core host, start with:

```sh
gonemaster-server --workers 16 --max-concurrent-jobs 16
```

Raise concurrency only after measuring both throughput and latency. Increasing
`--workers` above `--max-concurrent-jobs` has no effect on engine concurrency.

## Cross-Job Hot Cache

The cross-job hot cache shares warmed nameserver data across nearby jobs with
compatible resolver settings. It is most useful when a batch contains domains
served by the same authoritative infrastructure.

Controls:

```text
--cross-job-hot-cache
--no-cross-job-hot-cache
--cross-job-hot-cache-ttl N
```

Environment variables:

```text
GONEMASTER_CROSS_JOB_HOT_CACHE
GONEMASTER_CROSS_JOB_HOT_CACHE_TTL
```

Caches are separated by effective resolver settings. Jobs with different
profiles or network settings do not share hot-cache entries.

## Fast-Fail

Fast-fail stops sending queries to a nameserver after repeated transport
timeouts during one job. It reduces wasted time on unresponsive servers.

Set the threshold in the engine profile:

```json
{
  "resolver": {
    "defaults": {
      "fast_fail_timeout_count": 3
    }
  }
}
```

Set it to `0` to disable fast-fail.

## Resolver Settings

Timeout, retry, retransmit, fallback, parallelism, and ordering settings live
in the effective engine profile. Tune them in the server base profile or in a
stored profile selected by a job, batch, public profile, or tag default.

Common overrides:

```json
{
  "resolver": {
    "defaults": {
      "timeout": 2,
      "retry": 0,
      "retrans": 1,
      "fallback": false
    }
  }
}
```

Use deterministic settings when stable ordering is more important than speed:

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
