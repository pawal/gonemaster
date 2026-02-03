# Gonemaster CLI

The `gonemaster` command runs the Zonemaster test suites against a DNS zone and
prints results in human-readable or machine-readable formats.

## Usage

```
gonemaster --domain DOMAIN [options]
```

Notes:
- `--domain` is required for test runs.
- `--version` and `--list-tests` do not require `--domain`.
- `--dump-profile` can be used without `--domain` to inspect defaults.

## Output modes

By default, output is translated, human-readable text on stdout with a small
progress spinner when stdout is a terminal. Errors and progress information are
written to stderr.

You can switch output modes:
- `--json` prints a single JSON array of log entries.
- `--json-stream` prints newline-delimited JSON objects (one per log entry).
- `--raw` prints raw log lines (one per log entry).
- `--dump-profile` prints the effective profile as pretty JSON and exits.

Use `--output PATH` to write the selected output to a file.

## Options

| Flag | Type | Details |
| --- | --- | --- |
| `--domain DOMAIN` | string | Zone name to test (required for runs). |
| `--module MODULE` | string | Run a single module (optional). |
| `--testcase TESTCASE` | string | Run a single testcase (optional). |
| `--profile PATH` | string | Profile file in JSON or YAML (optional). |
| `--min-level LEVEL` | string | Minimum log level (default `NOTICE`). Must be one of `DEBUG3`, `DEBUG2`, `DEBUG`, `INFO`, `NOTICE`, `WARNING`, `ERROR`, `CRITICAL`. |
| `--output PATH` | string | Write output to a file instead of stdout. |
| `--raw` | bool | Stream raw log entries as they are produced. Incompatible with `--json` and `--json-stream`. |
| `--json` | bool | Print a single JSON array of log entries. Incompatible with `--raw` and `--json-stream`. |
| `--json-stream` | bool | Stream newline-delimited JSON entries. Incompatible with `--raw` and `--json`. Also incompatible with `--dump-profile`. |
| `--dump-profile` | bool | Print the effective profile as JSON and exit. Incompatible with `--raw` and `--json-stream`. |
| `--locale LOCALE` | string | Locale for translated output (defaults to `LANGUAGE`, then `LC_ALL`, `LC_MESSAGES`, `LANG`, and finally `en`). |
| `--no-ipv4` | bool | Disable IPv4 queries (overrides profile setting). |
| `--no-ipv6` | bool | Disable IPv6 queries (overrides profile setting). |
| `--parallel N` | int | Override `resolver.defaults.parallel`. Must be `>= 1` when set. |
| `--unordered` | bool | Allow unordered resolver behavior (overrides `resolver.defaults.unordered`). |
| `--ordered` | bool | Force ordered resolver behavior (overrides `resolver.defaults.unordered`). |
| `--error-cache-ttl N` | int | Seconds to skip queries after network errors. Must be `>= 0` when set. |
| `--no-progress` | bool | Disable progress indicator/spinner. |
| `--list-tests` | bool | List available test cases and exit. |
| `--version` | bool | Print version information and exit. |

## Examples

Run a full test with human-readable output:

```
gonemaster --domain example.com
```

Run a single module and testcase:

```
gonemaster --module address --testcase address01 --domain example.com
```

JSON output, formatted with `jq`:

```
gonemaster --json --domain example.com | jq
```

Stream JSON entries to a file:

```
gonemaster --json-stream --output /tmp/gonemaster.jsonl --domain example.com
```

Disable IPv6 and raise parallelism:

```
gonemaster --no-ipv6 --parallel 4 --domain example.com
```

Dump the effective profile (no domain required):

```
gonemaster --dump-profile --profile ./profile.yaml
```

List available test cases:

```
gonemaster --list-tests
```

High performance test, translated to Swedish:

```
gonemaster --unordered --parallel 8 --locale sv --domain example.com
```

## Return codes

- `0` Success
- `2` Usage or runtime error (invalid args, output file errors, engine errors)
- `130` Interrupted (SIGINT/SIGTERM)
