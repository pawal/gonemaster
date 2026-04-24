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

## Result Views

Choose a result view with `--view`:

- `summary`: compact counts.
- `modules`: grouped by module.
- `raw`: raw log entries.
- `json`: full JSON payload.

Use `--locale` when fetching translated messages and `--score` when you want
score and grade output.
