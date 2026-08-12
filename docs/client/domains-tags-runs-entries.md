# Domains, Tags, Runs, and Entries

These `gonemaster-client` commands query stored analysis data through the admin
API.

## Domains

- `domains list`: filter the domain registry.
- `domains get DOMAIN`: show stored details for a domain.
- `domains runs DOMAIN`: list historical runs for a domain.
- `domains tag DOMAIN TAG...`: add a domain to tags.
- `domains untag DOMAIN TAG...`: remove a domain from tags.

Examples:

```sh
gonemaster-client domains list --tag tld --level ERROR
gonemaster-client domains get example.com
gonemaster-client domains runs example.com --limit 20
gonemaster-client domains tag example.com tld monitored
gonemaster-client domains untag example.com monitored
```

`domains list` filters include `--tag`, `--name`, `--level`, and `--limit`.

## Tags

- `tags list`: list tags.
- `tags create TAG`: create a tag.
- `tags delete TAG`: delete a tag and its memberships.
- `tags domains TAG`: list domains in a tag.
- `tags summary TAG`: count latest domain severity by bucket.
- `tags add-domains TAG`: bulk-add domains to a tag.

Examples:

```sh
gonemaster-client tags create tld --description "Top-level domains"
gonemaster-client tags add-domains tld --file tlds.txt
gonemaster-client tags domains tld --limit 100
gonemaster-client tags summary tld
gonemaster-client tags delete old-tag
```

Deleting a tag removes tag memberships. It does not delete domains, runs,
entries, batches, or cohort snapshots.

## Runs

Runs are completed job records. Use `runs list`, `runs get`, and
`runs results` when you want historical completed work instead of currently
queued or running jobs.

Examples:

```sh
gonemaster-client runs list --tag tld --level WARNING --limit 100
gonemaster-client runs get run_123
gonemaster-client runs results run_123 --view raw --format json
gonemaster-client runs diff run_123 run_456
```

Filters include `--tag`, `--domain`, `--batch`, `--level`, and `--limit`.

`runs diff` compares two runs at the tag level: which findings appeared,
which cleared, and which changed severity. Each tag is collapsed to its
worst level within a run first, so a tag emitted once per nameserver is
compared by its severest occurrence rather than by count. The exit status is
`0` when the runs are identical and `1` when they differ, which makes it
usable as a gate:

```sh
# Before and after a fix, or the same domain under two profiles.
gonemaster-client runs diff --quiet run_before run_after || echo "findings changed"
```

## Entries

`entries query` filters log entries across runs. Common filters are:

- `--tag`
- `--module`
- `--testcase`
- `--level`
- `--latest`

Use `--format csv` for spreadsheet and pandas workflows.

Examples:

```sh
gonemaster-client entries query --tag tld --module DNSSEC --latest
gonemaster-client entries query --tag tld --entry-tag DS_ALGO_NOT_SUPPORTED --format csv
```

Common filters:

- `--run`
- `--domain`
- `--tag`
- `--module`
- `--testcase`
- `--entry-tag`
- `--level`
- `--latest`
- `--batch`
