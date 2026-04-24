# Client Jobs

Use `gonemaster-client jobs` for individual server-side tests and result
retrieval.

## Common Flow

```sh
gonemaster-client jobs create --domain example.com --wait --view summary
```

Useful commands:

- `jobs create`: submit one domain.
- `jobs list`: inspect queued and running jobs.
- `jobs get`: fetch one job record.
- `jobs watch`: wait for a job to finish.
- `jobs cancel`: cancel queued or running work.
- `jobs results`: retrieve results for one or more jobs.
- `jobs purge`: delete old completed jobs and their results.

## Create

```sh
gonemaster-client jobs create --domain example.com [options]
```

Options:

- `--domain DOMAIN`: required.
- `--tests NAME`: repeatable testcase selector.
- `--min-level LEVEL`: minimum result level.
- `--profile-override KEY=VALUE`: repeatable profile override.
- `--profile-overrides-file PATH`: JSON or YAML object to merge.
- `--wait`: wait for completion and print results.
- `--view summary|modules|raw|json`: result view when waiting.
- `--score`: show score and grade after results.
- `--no-score`: suppress scoring output.
- `--scoring-config PATH`: load custom scoring config JSON.

Use a single job, not a batch, when you need undelegated nameserver or DS input
through the server API.

## List, Watch, and Cancel

```sh
gonemaster-client jobs list --status running --limit 100
gonemaster-client jobs get job_123
gonemaster-client jobs watch job_123
gonemaster-client jobs cancel job_123
```

List filters include:

- `--status STATUS`
- `--batch-id ID`
- `--created-after RFC3339`
- `--limit N`
- `--offset N`

`jobs watch` uses server-sent events when available and falls back to polling.
It stops when the job is `succeeded`, `failed`, `canceled`, or `expired`.

## Results

```sh
gonemaster-client jobs results job_123 --view raw --format json
```

Selection options:

- positional `JOB_ID`
- `--job-id ID`, repeatable
- `--batch-id ID`
- `--all`
- `--status STATUS`, with `--all`
- `--created-after RFC3339`, with `--all`

Output options:

- `--levels NOTICE,WARNING,ERROR,CRITICAL`
- `--aggregate`
- `--per-job`
- `--split-dir PATH`
- `--score`
- `--no-score`
- `--scoring-config PATH`

## Result Views

Choose a result view with `--view`:

- `summary`: compact counts.
- `modules`: grouped by module.
- `raw`: raw log entries.
- `json`: full JSON payload.

Use `--locale` when fetching translated messages and `--score` when you want
score and grade output.

## Purge

```sh
gonemaster-client jobs purge --older-than 90
```

`--older-than 0` uses the server's configured `retention_days`. The server
returns an error when both values are `0`.
