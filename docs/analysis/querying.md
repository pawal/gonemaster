# Querying Analysis Data

You can query stored results through `gonemaster-client`, the admin API, or SQL.

## Client

```sh
gonemaster-client domains list --tag tld --level ERROR
gonemaster-client entries query --tag tld --module DNSSEC --latest
gonemaster-client batches results batch_123 --view json --per-job
```

Use `--format json` for scripts and `--format csv` for entry exports.

## Admin API

The admin API exposes domains, tags, runs, entries, batches, and profiles under
`/api/v1/`. Keep this API private.

Common queries:

- `GET /domains?tag=tld&level=ERROR`
- `GET /runs?tag=tld&level=WARNING`
- `GET /entries?tag=tld&module=DNSSEC&latest=true`
- `GET /tags/tld/summary`

## SQL

SQL is useful for large reports and custom analysis. PostgreSQL is the best
choice when you need indexed queries inside JSON log arguments.

The older [../data-analysis.md](../data-analysis.md) page contains the current
SQL cookbook while this page is being expanded.
