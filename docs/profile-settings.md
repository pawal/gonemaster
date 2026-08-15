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
| `parallel` | `8` | count (1-255) | `--parallel` | Concurrent resolver workers. |
| `unordered` | `true` | bool | `--unordered` / `--ordered` | Allow unordered result handling. For deterministic output use `false` with `parallel: 1`. |
| `error_cache_ttl` | `30` | seconds (0-86400) | `--error-cache-ttl` | Skip a query for this long after a network error (debounced). Capped by the per-query timeout/retry budget. |
| `positive_cache_ttl` | `0` | seconds (0-86400) | `--positive-cache-ttl` | Cache positive responses across runs. `0` disables. |
| `negative_cache_ttl` | `30` | seconds (0-86400) | `--negative-cache-ttl` | Cache negative responses across runs. |
| `fast_fail_timeout_count` | `3` | count (0-100) | - | Skip a nameserver/protocol after this many consecutive timeouts. `0` disables. Reacts to silence only. |
| `nameserver_concurrency` | `0` | count (0-256) | - | Maximum concurrent queries to one nameserver address. `0` is unlimited. On the server the cap is shared across concurrent jobs; see [server performance](server/performance.md). |
| `nameserver_max_total_ms` | `0` | milliseconds (0-600000) | - | Skip a nameserver address once cumulative query time in a run exceeds this. `0` disables. Unlike fast-fail it also bounds slow-but-responding servers. See [Bounding Slow Nameservers](server/configuration.md#bounding-slow-nameservers). |
| `debug` | `false` | bool | `--debug-queries` | Emit a per-attempt query trace (including timeouts) and the control decisions taken. No overhead when off. |

The `igntc`, `recurse`, and `usevc` keys are no longer profile properties. Stored
profiles that still contain them load without error and the keys are ignored;
per-query transport (TCP and the RD bit) is decided by the engine and individual
testcases.

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

## Inheritance and wholesale replacement

A property the override does not set is inherited from the default, so sparse
overrides are safe and restating a default value changes nothing.

Two properties are the exception: `test_cases` and `test_levels` replace the
default table as a whole rather than merging into it.

- A testcase absent from an overridden `test_cases` list does not run.
- A tag absent from an overridden `test_levels` module resolves to DEBUG, which
  keeps it out of reports and out of the score.

That makes a full restatement of the defaults a maintenance liability: when a
release adds a testcase or a tag, the override silently keeps the old set. The
`test_cases_vars` tunables are individual properties, so omitting one there just
inherits it.

## Comparing a profile against the defaults

`POST /api/v1/profiles/diff` compares a profile config against the current
engine defaults and reports, for every property the config sets, whether it
deviates from the default or merely restates it, plus the missing and unknown
keys inside overridden maps. The config travels in the request body, so an
unsaved draft can be compared without being stored.

The admin UI profile editor shows this above the config editor: a summary line,
an expandable per-property table, and a "Strip redundant overrides" action that
removes the properties which restate the default. Stripping never changes the
merged profile the engine runs, and it never prunes individual keys out of a
wholesale-replace map.

## Review state

A stored profile carries a `schema_version`: the engine version it was last
saved or reviewed against. When it matches the running engine, the profile is
reviewed - its remaining gaps are treated as deliberate and reported as
`waived_issues` rather than open issues, and the editor names them instead of
staying silent. "Check again" (the `clear_reviewed` patch op) drops the stamp so
the full check runs again and the gaps return as open issues.
