# Gonemaster CLI

This page documents `gonemaster`, the direct local test runner. It runs the
engine in the current process without `gonemaster-server`.

`gonemaster` normalizes IDN domains to IDNA A-labels before use.

*This page is for the local runner only.* The server client is documented under
[client/](../client/README.md), including job and batch operations.

## Installation

Install the CLI with Go:

```sh
go install codeberg.org/pawal/gonemaster/cmd/gonemaster@latest
```

For local development from a checkout:

```sh
go build -o gonemaster ./cmd/gonemaster
```

## Synopsis

```sh
gonemaster [flags] [DOMAIN]
```

Notes:

- `DOMAIN` can be provided as a positional argument or with `--domain`.
- `--domain` or positional `DOMAIN` is required for test runs.
- `--version`, `--list-tests`, and `--dump-profile` do not require a domain.
- Malformed undelegated inputs such as `--ns` or `--ds` return exit code `2`.
- The built-in profile uses `resolver.defaults.parallel=8` and
  `resolver.defaults.unordered=true`.
- For deterministic ordered behavior, use `--ordered --parallel 1`.

## Exit Codes

- `0`: success
- `2`: usage or runtime error
- `130`: interrupted by signal

## Output Modes

Default output is translated human-readable text on stdout. Progress goes to
stderr when stdout is a terminal.

| Mode | Flag |
|---|---|
| Human-readable text | default |
| Single JSON array | `--json` |
| Newline-delimited JSON | `--json-stream` |
| Raw log lines | `--raw` |
| Effective profile JSON | `--dump-profile` |

Additional output controls:

- `--count` appends human-readable level and tag counts.
- `--nstimes` appends per-nameserver timing statistics.
- `--output PATH` writes selected output to a file.
- `--save PATH` writes the DNS packet cache after the run.
- `--restore PATH` primes the DNS packet cache before the run.
- `--cache-stats PATH` prints statistics for a saved cache file and exits.
- `--cache-strict` treats cache-file warnings (unknown fields/kinds, missing checksum) as errors when restoring or inspecting.

See [cache-format.md](cache-format.md) for the packet cache file schema.

## Machine-Consumer Result Contract

Use `--json` or `--json-stream` for machine consumption.

For migrated coherent entries:

- `args.ns` is the nameserver name.
- `args.address` is the nameserver IP address.
- `args.servers` is an array of `{ns, address}` objects.
- `args.asns` is an array of ASN integers when ASN data is emitted.

Legacy keys can still appear on non-migrated tags during migration. Prefer the
v1.1 keys when they are present.

Reference:

- [specifications/log-args-coherency.md](../specifications/log-args-coherency.md)
- [specifications/log-args-key-glossary.md](../specifications/log-args-key-glossary.md)

## Options

The flag groups below follow `gonemaster --help`.

### Target

| Flag | Type | Details |
|---|---|---|
| `DOMAIN` | positional | Zone name to test. Alternative to `--domain`. |
| `--domain DOMAIN` | string | Zone name to test. Required for runs when positional `DOMAIN` is not provided. |
| `--module MODULE` | string | Run one module. |
| `--testcase TESTCASE` | string (repeatable) | Run a specific testcase. May be passed multiple times to run several testcases, possibly across modules. Names are case-insensitive. |
| `--profile PATH` | string | Profile file in JSON or YAML. |

### Output

| Flag | Type | Details |
|---|---|---|
| `--min-level LEVEL` | string | Minimum log level. Default `NOTICE`. |
| `--stop-level LEVEL` | string | Stop after the first entry at this level or higher. |
| `--locale LOCALE` | string | Locale for translated output. |
| `--output PATH` | string | Write output to a file. |
| `--raw` | bool | Stream raw log entries. |
| `--json` | bool | Print one JSON array. |
| `--json-stream` | bool | Stream newline-delimited JSON entries. |
| `--count` | bool | Append count summaries in human output. |
| `--nstimes` | bool | Append per-nameserver timing statistics. With `--json`, wraps output as `{"entries":[…],"nameserver_timings":[…]}`. |
| `--no-progress` | bool | Disable progress indicator. |
| `--score` | bool | Print score and grade summary after the run. |
| `--no-score` | bool | Suppress score output. |
| `--scoring-config PATH` | string | Custom scoring config JSON file. Implies `--score`. |

### Cache

| Flag | Type | Details |
|---|---|---|
| `--save PATH` | string | Save DNS packet cache after the run. |
| `--save-compress` | bool | Gzip-compress the saved cache file (also implied by a `.gz` path). |
| `--save-max-entries N` | int | Refuse to save if the cache file would contain more than N entries (0 = unlimited). |
| `--restore PATH` | string | Restore DNS packet cache before the run. |
| `--cache-stats PATH` | string | Print statistics for a saved cache file and exit. |
| `--cache-strict` | bool | Treat cache-file warnings (unknown fields/kinds, missing checksum) as errors. |

### Resolver/Profile Overrides

| Flag | Type | Details |
|---|---|---|
| `--no-ipv4` | bool | Disable IPv4 queries. |
| `--no-ipv6` | bool | Disable IPv6 queries. |
| `--ipv6` | bool | Force IPv6 queries. |
| `--allow-non-global` | bool | Allow querying private / non-globally-reachable nameserver addresses. Off by default: such targets are skipped with a `NON_GLOBAL_QUERY_BLOCKED` notice. Use for internal/split-horizon zones. |
| `--parallel N` | int | Override resolver parallelism. |
| `--unordered` | bool | Allow unordered resolver behavior. |
| `--ordered` | bool | Force ordered resolver behavior. |
| `--timeout N` | int | Override resolver timeout in seconds. |
| `--retry N` | int | Override resolver retry count. |
| `--retrans N` | int | Override resolver retransmit interval in seconds. |
| `--fallback` | bool | Enable TCP fallback on UDP failure. |
| `--no-fallback` | bool | Disable TCP fallback on UDP failure. |
| `--sourceaddr4 IPADDR` | string | IPv4 source address for outgoing queries. |
| `--sourceaddr6 IPADDR` | string | IPv6 source address for outgoing queries. |
| `--error-cache-ttl N` | int | Seconds to skip queries after network errors. |
| `--positive-cache-ttl N` | int | Seconds to cache positive DNS responses. |
| `--negative-cache-ttl N` | int | Seconds to cache negative DNS responses. |
| `--badkeys-path PATH` | string | Badkeys blocklist directory path. |

### Undelegated

| Flag | Type | Details |
|---|---|---|
| `--ns NAME[/IP]` | repeatable | Undelegated nameserver input. |
| `--ds KEYTAG,ALGORITHM,DIGTYPE,DIGEST` | repeatable | Undelegated DS input. |

### Utility

| Flag | Type | Details |
|---|---|---|
| `--badkeys-update` | bool | Download or update badkeys blocklist data and exit. |
| `--dump-profile` | bool | Print effective profile JSON and exit. |
| `--list-tests` | bool | List available testcases and exit. |
| `--version` | bool | Print version information and exit. |

`--raw`, `--json`, and `--json-stream` are mutually exclusive.
`--count` is human-output only.

## Examples

Run a full test:

```sh
gonemaster example.com
```

Run one module and testcase:

```sh
gonemaster --module address --testcase address01 example.com
```

Run several testcases (across modules):

```sh
gonemaster --testcase consistency04 --testcase delegation07 example.com
```

Print JSON:

```sh
gonemaster --json --domain example.com | jq
```

Stream JSON entries to a file:

```sh
gonemaster --json-stream --output /tmp/gonemaster.jsonl --domain example.com
```

Save and replay a DNS packet cache:

```sh
gonemaster --domain example.com --save /tmp/gonemaster-cache.json
gonemaster --domain example.com --restore /tmp/gonemaster-cache.json
```

Inspect a saved cache file without running a test:

```sh
gonemaster --cache-stats /tmp/gonemaster-cache.json
```

Show per-nameserver query timing statistics:

```sh
gonemaster --nstimes example.com
```

The table holds max, min, avg, stddev, median, total and count, sorted by
nameserver name, address, then median, and covers answered queries only. The
`timeout` column counts exchanges that spent every attempt without an answer,
and `refused` counts responses carrying rcode REFUSED.

Include nameserver timing data in JSON output:

```sh
gonemaster --json --nstimes example.com | jq .nameserver_timings
```

Disable IPv6 and raise parallelism:

```sh
gonemaster --no-ipv6 --parallel 4 --domain example.com
```

Dump the effective profile:

```sh
gonemaster --dump-profile --profile ./profile.yaml
```

List available test cases:

```sh
gonemaster --list-tests
```

High-performance test with Swedish output:

```sh
gonemaster --unordered --parallel 8 --locale sv --domain example.com
```

Undelegated test with explicit nameservers and glue:

```sh
gonemaster --domain example.com \
  --ns ns1.example.com/192.0.2.10 \
  --ns ns2.example.net/2001:db8::10
```

Undelegated test with one nameserver and both IPv4 and IPv6:

```sh
gonemaster --domain example.com \
  --ns ns1.example.com/192.0.2.10 \
  --ns ns1.example.com/2001:db8::10
```

Undelegated DS-only test:

```sh
gonemaster --domain example.com \
  --ds 12345,13,2,0123456789ABCDEF0123456789ABCDEF0123456789ABCDEF0123456789ABCDEF
```

Run a test and display score and grade:

```sh
gonemaster --score example.com
```

Use a custom scoring config:

```sh
gonemaster --scoring-config /etc/gonemaster/scoring.json example.com
```

Score with JSON output. The score goes to stderr to keep stdout valid JSON.

```sh
gonemaster --json --score example.com | jq
```

Extract coherent nameserver name and IP pairs:

```sh
gonemaster --json-stream --domain example.com \
  | jq -r 'select(.args.ns and .args.address)
           | [.args.ns, .args.address] | @tsv'
```

Extract ASN lists:

```sh
gonemaster --json --domain example.com \
  | jq -r '.[]
           | select(.args.asns != null)
           | [.tag, (.args.asns | map(tostring) | join(","))] | @tsv'
```

## Badkeys Blocklist Setup

Update blocklist data in the default user data directory:

```sh
gonemaster --badkeys-update
```

For developers and packagers, update blocklist data in `share/badkeys/`:

```sh
make badkeys-update
```

## Further Reference

- Testcase specifications: [specifications/tests/](../specifications/tests/)
- Tag catalogs: [specifications/tags/](../specifications/tags/)
- Cache format: [cache-format.md](cache-format.md)
- Scoring: [scoring.md](../scoring.md)
