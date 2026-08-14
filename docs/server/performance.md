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

## Per-Address Concurrency

`nameserver_concurrency` limits how many queries run at once against one
nameserver address. The default is `0`, unlimited. Testing at 16 to 64 workers
found no batch that lost findings to its own query rate, so it stays off.

The limit applies to every address, so there is nothing to aim at a particular
nameserver. It only takes effect where a batch is concentrated: 500 `.se`
domains that all delegate to `ns1.example.net` and `ns2.example.net` send their
whole query volume to two addresses and hit the limit constantly, while 500
unrelated domains spread it over hundreds and rarely reach it at all. On the
server the limit is shared by all running jobs, so it bounds the batch, not
each job.

Cost at 64 workers: a limit of 3 changed nothing on a batch with many different
nameservers, and added about a third to the run time when every query went to
three addresses.

```json
{ "resolver": { "defaults": { "nameserver_concurrency": 3 } } }
```

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

## Reachability Backoff

Fast-fail handles servers that time out. Reachability backoff handles addresses
the local host cannot reach at all - no route to host, network or host
unreachable - which fail immediately rather than after a timeout. The common
cause is a host with an IPv6 address but no working IPv6 transit.

Two such errors on one address inside the TTL window suppress further queries
to it. A single error does not: one stray ICMP unreachable must not blackhole a
healthy nameserver and cascade into spurious no-working-nameserver verdicts.

The TTL is derived, not configured: it is the smaller of
`resolver.defaults.negative_cache_ttl` (falling back to `error_cache_ttl`) and
the per-query timeout and retry budget. With the shipped profile that is 15
seconds. Setting both TTLs to `0` disables the mechanism.

The state belongs to the run. A suppressed query is invisible at default log
levels; raise the engine to DEBUG2 to see `REACHABILITY_CACHE_SKIP`, or use
`gonemaster --debug-queries`, which reports a `skipped_reachability` tally.

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
