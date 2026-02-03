# Gonemaster CLI Specification

This document defines the CLI contracts for two tools:

- `gonemaster`: run the engine locally.
- `gonemaster-client`: talk to `gonemaster-server` over HTTP for jobs, batches, and results.

Both commands normalize IDN domains to IDNA A-labels (punycode) before use.

## Common conventions

### Output formats
- `pretty` (default): human-friendly text suitable for terminals.
- `json`: a single JSON document.
- `jsonl`: newline-delimited JSON (one object per line).

### Exit codes
- `0`: success
- `2`: usage or runtime error (invalid args, I/O, HTTP errors, engine errors)
- `130`: interrupted (SIGINT/SIGTERM)

## gonemaster (local engine)

### Synopsis
```
gonemaster --domain DOMAIN [options]
```

Notes:
- `--domain` is required for test runs.
- `--version` and `--list-tests` do not require `--domain`.
- `--dump-profile` can be used without `--domain`.

### Output modes
By default, output is translated, human-readable text on stdout with a small
progress spinner when stdout is a terminal. Errors and progress information are
written to stderr.

You can switch output modes:
- `--json` prints a single JSON array of log entries.
- `--json-stream` prints newline-delimited JSON objects (one per log entry).
- `--raw` prints raw log lines (one per log entry).
- `--dump-profile` prints the effective profile as pretty JSON and exits.

Use `--output PATH` to write the selected output to a file.

### Options

| Flag | Type | Details |
| --- | --- | --- |
| `--domain DOMAIN` | string | Zone name to test (required for runs). |
| `--module MODULE` | string | Run a single module (optional). |
| `--testcase TESTCASE` | string | Run a single testcase (optional). |
| `--profile PATH` | string | Profile file in JSON or YAML (optional). |
| `--min-level LEVEL` | string | Minimum log level (default `NOTICE`). One of `DEBUG3`, `DEBUG2`, `DEBUG`, `INFO`, `NOTICE`, `WARNING`, `ERROR`, `CRITICAL`. |
| `--output PATH` | string | Write output to a file instead of stdout. |
| `--raw` | bool | Stream raw log entries. Incompatible with `--json` and `--json-stream`. |
| `--json` | bool | Print a single JSON array. Incompatible with `--raw` and `--json-stream`. |
| `--json-stream` | bool | Stream newline-delimited JSON entries. Incompatible with `--raw` and `--json`. Also incompatible with `--dump-profile`. |
| `--dump-profile` | bool | Print the effective profile as JSON and exit. Incompatible with `--raw` and `--json-stream`. |
| `--locale LOCALE` | string | Locale for translated output (defaults to environment, then `en`). |
| `--no-ipv4` | bool | Disable IPv4 queries (overrides profile setting). |
| `--no-ipv6` | bool | Disable IPv6 queries (overrides profile setting). |
| `--parallel N` | int | Override `resolver.defaults.parallel`. Must be `>= 1` when set. |
| `--unordered` | bool | Allow unordered resolver behavior (overrides `resolver.defaults.unordered`). |
| `--ordered` | bool | Force ordered resolver behavior (overrides `resolver.defaults.unordered`). |
| `--error-cache-ttl N` | int | Seconds to skip queries after network errors. Must be `>= 0` when set. |
| `--no-progress` | bool | Disable progress indicator/spinner. |
| `--list-tests` | bool | List available test cases and exit. |
| `--version` | bool | Print version information and exit. |

### Examples

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

## gonemaster-client (HTTP API client)

### Synopsis
```
gonemaster-client [global options] <command> [command options] [args]
```

### Global options

| Flag | Type | Details |
| --- | --- | --- |
| `--server URL` | string | Base API URL (default `http://localhost:8080/api/v1`). |
| `--timeout DURATION` | string | HTTP timeout (default `30s`). |
| `--format FORMAT` | string | Output format: `pretty`, `json`, `jsonl`. |
| `--output PATH` | string | Write output to a file instead of stdout. |
| `--locale LOCALE` | string | Locale for translated result messages (default `en`). |
| `--no-color` | bool | Disable ANSI colors in `pretty` output. |
| `--header NAME:VALUE` | string | Extra HTTP header (repeatable). |

### Commands

#### jobs create
Submit a single job.
```
gonemaster-client jobs create --domain example.com [options]
```
Options:
- `--domain DOMAIN` (required)
- `--tests NAME` (repeatable, optional)
- `--min-level LEVEL` (optional)
- `--profile-override KEY=VALUE` (repeatable; merged into `profile_overrides`)
- `--profile-overrides-file PATH` (JSON/YAML object to merge)
- `--wait` (wait for completion and print results)
- `--view summary|modules|raw|json` (when `--wait` is used)

#### jobs batch
Submit a batch of jobs.
```
gonemaster-client jobs batch [input options] [options]
```
Input options (one or more):
- `--domain DOMAIN` (repeatable)
- `--file PATH` (one domain per line)
- `--stdin` (read domains from stdin)

Options:
- `--tests NAME` (repeatable)
- `--min-level LEVEL`
- `--profile-override KEY=VALUE` (repeatable)
- `--profile-overrides-file PATH`
- `--wait` (wait for batch completion)
- `--view summary|modules|raw|json` (when `--wait` is used)
- `--per-job` (when waiting, show per-job results instead of only batch summary)

#### jobs list
List jobs with filters.
```
gonemaster-client jobs list [options]
```
Options:
- `--status STATUS` (`queued`, `running`, `succeeded`, `failed`, `canceled`, `expired`, `paused`)
- `--batch-id ID`
- `--created-after RFC3339`
- `--limit N` (default `100`, max `500`)
- `--offset N`

#### jobs get
Fetch job status.
```
gonemaster-client jobs get JOB_ID
```

#### jobs watch
Stream job status until completion.
```
gonemaster-client jobs watch JOB_ID [--poll DURATION]
```
Notes:
- Uses SSE events if available; falls back to polling.
- Stops when status is `succeeded`, `failed`, or `canceled`.

#### jobs cancel
Cancel a queued or running job.
```
gonemaster-client jobs cancel JOB_ID
```

#### jobs results
Fetch results for one or more jobs (including all jobs).
```
gonemaster-client jobs results [JOB_ID] [options]
```
Options:
- `--job-id ID` (repeatable; alternative to positional)
- `--batch-id ID` (fetch results for all jobs in a batch)
- `--all` (fetch results for all jobs)
- `--status STATUS` (when using `--all`)
- `--created-after RFC3339` (when using `--all`)
- `--view summary|modules|raw|json` (default `summary` for `pretty`, `json` for `--format json`)
- `--levels NOTICE,WARNING,ERROR,CRITICAL` (filter)
- `--aggregate` (print a combined summary across all selected jobs)
- `--per-job` (print per-job summaries/results)
- `--split-dir PATH` (write per-job results to files; filename includes job id)

This command is the primary way to retrieve results from all jobs in a convenient way.

#### batches get
Fetch a batch summary.
```
gonemaster-client batches get BATCH_ID
```

#### batches watch
Poll or stream a batch until completion.
```
gonemaster-client batches watch BATCH_ID [--poll DURATION]
```

#### batches results
Fetch results for all jobs in a batch.
```
gonemaster-client batches results BATCH_ID [options]
```
Options:
- `--view summary|modules|raw|json`
- `--levels NOTICE,WARNING,ERROR,CRITICAL`
- `--aggregate` (combined summary across the batch)
- `--per-job` (include per-job results)

#### batches cancel
Cancel all queued or running jobs in a batch.
```
gonemaster-client batches cancel BATCH_ID
```
Notes:
- Jobs already completed are skipped.
- Uses the per-job cancel endpoint for each eligible job.

#### batches remove
Remove queued jobs from a batch (running jobs are skipped).
```
gonemaster-client batches remove BATCH_ID [--cancel-running]
```
Notes:
- Uses the queue remove endpoint for queued jobs only.
- With `--cancel-running`, running jobs are canceled via per-job cancel requests.

#### queue pause
Pause queue processing.
```
gonemaster-client queue pause
```

#### queue resume
Resume queue processing.
```
gonemaster-client queue resume
```

#### queue reorder
Reorder queued jobs.
```
gonemaster-client queue reorder --job-id ID [--job-id ID...]
```

#### queue remove
Remove queued jobs.
```
gonemaster-client queue remove --job-id ID [--job-id ID...]
```

### Examples

Submit a batch from file and wait for completion:
```
gonemaster-client jobs batch --file domains.txt --wait --view summary
```

Submit a batch from stdin:
```
cat domains.txt | gonemaster-client jobs batch --stdin --wait --view modules
```

List only failed jobs:
```
gonemaster-client jobs list --status failed
```

Fetch results for all completed jobs since a timestamp:
```
gonemaster-client jobs results --all --status succeeded --created-after 2026-02-01T00:00:00Z --aggregate
```

Fetch full results for a batch and write per-job files:
```
gonemaster-client batches results batch_123 --view json --split-dir /tmp/gonemaster-results
```

Cancel all queued/running jobs in a batch:
```
gonemaster-client batches cancel batch_123
```

Remove queued jobs from a batch:
```
gonemaster-client batches remove batch_123
```

Cancel a job:
```
gonemaster-client jobs cancel job_123
```
