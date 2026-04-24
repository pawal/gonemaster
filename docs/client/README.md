# Gonemaster Client

`gonemaster-client` is the command-line client for `gonemaster-server`. Use it
when you want to submit jobs, run batches, inspect stored results, or query
tagged analysis data through the admin API.

For direct local tests that do not use the server, use [../cli.md](../cli.md).

## Connection

The default server is:

```sh
gonemaster-client --server http://localhost:8080/api/v1 jobs list
```

Set `--server` to the admin API base URL. Public API endpoints are not the
default target for this client.

## Output

Use `--format pretty` for terminal output and `--format json` for scripts.
Most commands also accept `--output PATH`.

## Guides

- Jobs: [jobs.md](jobs.md)
- Batches: [batches.md](batches.md)
- Domains, tags, runs, and entries: [domains-tags-runs-entries.md](domains-tags-runs-entries.md)
- Examples: [examples.md](examples.md)
