# Gonemaster Client

`gonemaster-client` is the command-line client for `gonemaster-server`. Use it
when you want to submit jobs, run batches, inspect stored results, or query
tagged analysis data through the admin API.

For direct local tests that do not use the server, use [../cli/](../cli/README.md).

## Connection

The default server is:

```sh
gonemaster-client --server http://localhost:8080/api/v1 jobs list
```

Set `--server` to the admin API base URL. Public API endpoints are not the
default target for this client.

## Global Options

| Flag | Type | Details |
|---|---|---|
| `--server URL` | string | Base admin API URL. Default `http://localhost:8080/api/v1`. |
| `--timeout DURATION` | string | HTTP timeout. Default `30s`. |
| `--format FORMAT` | string | `pretty`, `json`, `jsonl`, or `json-stream`. |
| `--output PATH` | string | Write output to a file. |
| `--locale LOCALE` | string | Locale for translated result messages. Default `en`. |
| `--no-color` | bool | Disable ANSI colors in pretty output. |
| `--header NAME:VALUE` | repeatable | Add an HTTP header. |
| `--version` | bool | Print version information and exit. |

Subcommand flags may be placed before or after positional arguments.

## Output Formats

Use `--format pretty` for terminal output and `--format json` for scripts.
Use `jsonl` or `json-stream` when a command emits multiple JSON records.
Most commands also accept `--output PATH`.

Exit codes:

- `0`: success
- `2`: usage or runtime error, including HTTP errors
- `130`: interrupted by signal

## Command Groups

- `jobs`: submit, watch, cancel, purge, and fetch job results.
- `batches`: inspect and manage batch groups.
- `queue`: pause, resume, reorder, or remove queued jobs.
- `domains`: inspect and tag domain records.
- `tags`: manage named domain collections.
- `runs`: inspect completed historical runs.
- `entries`: query stored log entries.

## Guides

- Jobs: [jobs.md](jobs.md)
- Batches: [batches.md](batches.md)
- Domains, tags, runs, and entries: [domains-tags-runs-entries.md](domains-tags-runs-entries.md)
- Examples: [examples.md](examples.md)

The concise Unix man-page form is kept at
[../man/gonemaster-client.1.md](../man/gonemaster-client.1.md).
