# Debugging a Run

This page covers the `gonemaster` flags that show what a run did on the
network: which nameservers were slow, which queries were skipped, and how to
replay a run without the network.

## Slow Runs

Run time is time spent waiting for responses. Two flags attribute it to
nameserver addresses.

`--nstimes` appends per-address statistics for answered queries: max, min,
avg, stddev, median, total and count in milliseconds, plus a `timeout` column
counting exchanges that spent every attempt without an answer and a `refused`
column counting responses with rcode REFUSED.

```sh
gonemaster --nstimes example.com
```

`--debug-queries` appends a per-attempt trace, slowest address first:

```text
Query trace: 1626 attempts (0 timeouts) across 127 nameservers
Name servers        Attempts   Timeouts     Errors  Elapsed/ms  Decisions
==================  ========== ========== ========== ===========
192.5.6.30                  70          0         28     5331.42
199.180.182.53              32          0          8     4151.67  skipped_errorcache:2
```

| Column | Meaning |
|---|---|
| Name servers | `name/address` when the host name is known, else the address. |
| Attempts | Transport attempts, counting retries and the TCP fallback after a truncated UDP reply. |
| Timeouts | Attempts whose deadline fired with no response. |
| Errors | Attempts that failed for another reason. This includes attempts abandoned because another server in the same batch answered first; a high error count on a responsive server is normal. |
| Elapsed/ms | Sum of attempt wall-clock time for the address. |
| Decisions | Slow-server controls that fired for the address, as `kind:count`. |

Decision kinds:

| Kind | Meaning |
|---|---|
| `fastfail_blocked` | Fast-fail blocked the address for one transport after repeated timeouts. |
| `skipped_fastfail` | A query was skipped because fast-fail had blocked the address. |
| `errorcached` | A failed query was written to the error cache. |
| `skipped_errorcache` | A query was skipped because the error cache held its key. |
| `blacklisted` | The address was blacklisted after a failed SOA query. |
| `skipped_blacklist` | A query was skipped because the address was blacklisted. |
| `skipped_reachability` | A query was skipped because the address was in reachability backoff. |
| `latency_budget_blocked` | The address exceeded `nameserver_max_total_ms`. |
| `skipped_latency_budget` | A query was skipped because the latency budget had blocked the address. |

Fast-fail and reachability backoff are described in
[server/performance.md](../server/performance.md#fast-fail); the resolver
keys behind every decision are in
[profile-settings.md](../profile-settings.md).

The trace table is printed after human output, including to a file given by
`--output`. It is not printed with `--json`, `--json-stream` or `--raw`.

To attribute time to testcases, stream JSON at DEBUG level. Every entry
carries `timestamp` in seconds since run start, and `TEST_CASE_START` and
`TEST_CASE_END` bound each testcase:

```sh
gonemaster --json-stream --min-level DEBUG example.com | grep TEST_CASE_
```

## Seeing Every Query

The System module logs the query layer. The default profile places its tags
at these levels; a tag without a level in the profile is DEBUG.

| Level | Tags |
|---|---|
| DEBUG | `EXTERNAL_QUERY` (one line per query sent: `ns`, `address`, `query_name`, `query_type`), `BLACKLISTING`, `IS_BLACKLISTED`, `PACKET_BIG`, `TEST_CASE_START`, `TEST_CASE_END` |
| DEBUG2 | `QUERY`, `RECURSE_QUERY`, `ERROR_CACHE_SKIP`, `REACHABILITY_CACHE_SKIP`, `NS_CREATED`, `CACHE_FETCHED`, `NO_SUCH_NAME`, `NO_SUCH_RECORD` |
| DEBUG3 | `EXTERNAL_RESPONSE`, `CACHED_RETURN`, `EMPTY_RETURN` |

```sh
gonemaster --raw --min-level DEBUG example.com | grep EXTERNAL_QUERY
```

`--raw` lines carry no timestamp; use `--json-stream` when timing matters.

## Replaying Without the Network

`--save` writes the DNS packet cache after a run and `--restore` primes it
before one. A restored run answers from the cache and queries the network
only on a miss. Human output ends with the line `packet cache: N hits,
M misses`.

```sh
gonemaster --save /tmp/example.json.gz example.com
gonemaster --restore /tmp/example.json.gz --testcase dnssec10 example.com
gonemaster --cache-stats /tmp/example.json.gz
```

A saved cache reproduces a finding on another machine and isolates a
testcase change from network variation. The file format is in
[cache-format.md](cache-format.md).

## Stopping Early

`--stop-level LEVEL` ends the run at the first entry at that level or higher:

```sh
gonemaster --stop-level ERROR example.com
```

## Server Jobs

A server job does not produce the query trace. To debug a slow job, run the
CLI with the job's profile:

```sh
gonemaster --profile job-profile.json --debug-queries example.com
```

Server-side causes of slow or stuck jobs are covered in
[server/performance.md](../server/performance.md) and
[server/operations.md](../server/operations.md#jobs-that-will-not-finish).
`gonemaster-server --debug` captures HTTP request and response bodies in the
access log and is unrelated to query tracing; see
[server/configuration.md](../server/configuration.md).
