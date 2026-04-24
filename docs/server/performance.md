# Server Performance

This page covers throughput and latency tuning for server workloads.

## Worker Count

`--workers` controls how many workers dequeue jobs. `--max-concurrent-jobs`
caps how many engine runs execute at the same time. Effective concurrency is
the smaller of those two values.

For DNS-heavy batch workloads, a worker count above CPU core count can help
because much of the runtime is network I/O. Measure before raising it on shared
hosts.

## Cross-Job Hot Cache

The cross-job hot cache shares warmed nameserver data across nearby jobs with
compatible resolver settings. It is most useful when a batch contains domains
served by the same authoritative infrastructure.

## Fast-Fail

Fast-fail stops sending queries to a nameserver after repeated transport
timeouts during one job. It reduces wasted time on unresponsive servers.

## Resolver Settings

Timeout, retry, retransmit, fallback, parallelism, and ordering settings live
in the effective engine profile. Tune them in the server base profile or in a
stored profile selected by a job, batch, public profile, or tag default.
