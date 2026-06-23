# Profile Settings Reference

The engine profile is the source of truth for how runs behave: resolver timing,
the IP stack, slow-server controls, response caching, and per-testcase severity.
A run starts from the built-in default and applies sparse overrides on top.

How overrides are applied:

- CLI: `gonemaster --profile PATH` (JSON or YAML), merged over the default.
- Server: a process-wide `profile_path`, stored profiles, and per-job overrides.
  See [server/configuration.md](server/configuration.md#profiles).

Inspect the effective profile for a run with `gonemaster --dump-profile` (CLI),
or the recorded effective profile on a job (server).

## resolver.defaults

The resolver tuning knobs. Defaults are the built-in values; ranges are the
accepted bounds.

| Key | Default | Unit / type (range) | CLI flag | Description |
|-----|---------|---------------------|----------|-------------|
| `timeout` | `5` | seconds | `--timeout` | Per-attempt query timeout. |
| `retry` | `2` | count (1-255) | `--retry` | Retries after the initial attempt. |
| `retrans` | `3` | seconds (1-255) | `--retrans` | Interval between attempts; also the effective per-attempt UDP budget when below `timeout`. |
| `fallback` | `true` | bool | `--fallback` / `--no-fallback` | Retry truncated UDP responses over TCP. |
| `igntc` | `false` | bool | - | Ignore TC; do not retry truncated responses over TCP. |
| `recurse` | `false` | bool | - | Set the RD bit on outbound queries. |
| `usevc` | `false` | bool | - | Force queries over TCP. |
| `parallel` | `8` | count (1-255) | `--parallel` | Concurrent resolver workers. |
| `unordered` | `true` | bool | `--unordered` / `--ordered` | Allow unordered result handling. For deterministic output use `false` with `parallel: 1`. |
| `error_cache_ttl` | `30` | seconds (0-86400) | `--error-cache-ttl` | Skip a query for this long after a network error (debounced). Capped by the per-query timeout/retry budget. |
| `positive_cache_ttl` | `0` | seconds (0-86400) | `--positive-cache-ttl` | Cache positive responses across runs. `0` disables. |
| `negative_cache_ttl` | `30` | seconds (0-86400) | `--negative-cache-ttl` | Cache negative responses across runs. |
| `fast_fail_timeout_count` | `3` | count (0-100) | - | Skip a nameserver/protocol after this many consecutive timeouts. `0` disables. Reacts to silence only. |
| `nameserver_concurrency` | `0` | count (0-256) | - | Maximum concurrent queries to one nameserver address. `0` is unlimited. |
| `nameserver_max_total_ms` | `0` | milliseconds (0-600000) | - | Skip a nameserver address once cumulative query time in a run exceeds this. `0` disables. Unlike fast-fail it also bounds slow-but-responding servers. See [Bounding Slow Nameservers](server/configuration.md#bounding-slow-nameservers). |
| `debug` | `false` | bool | `--debug-queries` | Emit a per-attempt query trace (including timeouts) and the control decisions taken. No overhead when off. |

Source addresses live alongside the defaults, under `resolver`:

| Key | Default | CLI flag | Description |
|-----|---------|----------|-------------|
| `resolver.source4` | `""` | `--sourceaddr4` | IPv4 source address for outbound queries. |
| `resolver.source6` | `""` | `--sourceaddr6` | IPv6 source address for outbound queries. |

## Other profile sections

These are part of the profile but configured elsewhere:

- `net.ipv4` / `net.ipv6` - enable or disable an IP stack (CLI `--no-ipv4`,
  `--no-ipv6`, `--ipv6`).
- `net.allow_non_global_targets` - default `false`. When `false` (the default), the
  engine refuses to query nameserver addresses that are private, loopback, or otherwise
  not globally reachable (loopback, RFC1918, CGNAT, link-local, ULA, documentation,
  benchmarking, and similar IANA special-purpose ranges), even when learned from glue or
  DNS resolution, and emits a `NON_GLOBAL_QUERY_BLOCKED` notice instead. Set `true` (CLI
  `--allow-non-global`) on private/internal instances that test such zones. Addresses an
  operator pins explicitly via `--ns name/IP` (undelegated tests) are always queried
  regardless of this flag. Note: enabling the guard by default is a behavior change -
  delegated tests against a private nameserver now need `--allow-non-global`.
- `test_levels` - the severity level emitted per message tag, per module.
  Largely generated; see the [test specifications](specifications/).
- `test_cases_vars` - per-testcase thresholds (for example SOA timer minimums).
- `asndb` - ASN lookup backend and sources.
- `badkeys` - badkeys blocklist path.

## Example: a sparse override

Only the keys that differ from the default are needed:

```json
{
  "resolver": {
    "defaults": {
      "nameserver_max_total_ms": 60000
    }
  }
}
```
