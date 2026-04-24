# Domains, Tags, Runs, and Entries

These `gonemaster-client` commands query stored analysis data through the admin
API.

## Domains

- `domains list`: filter the domain registry.
- `domains get DOMAIN`: show stored details for a domain.
- `domains runs DOMAIN`: list historical runs for a domain.
- `domains tag DOMAIN TAG...`: add a domain to tags.
- `domains untag DOMAIN TAG...`: remove a domain from tags.

## Tags

- `tags list`: list tags.
- `tags create TAG`: create a tag.
- `tags delete TAG`: delete a tag and its memberships.
- `tags domains TAG`: list domains in a tag.
- `tags summary TAG`: count latest domain severity by bucket.
- `tags add-domains TAG`: bulk-add domains to a tag.

## Runs

Runs are completed job records. Use `runs list`, `runs get`, and
`runs results` when you want historical completed work instead of currently
queued or running jobs.

## Entries

`entries query` filters log entries across runs. Common filters are:

- `--tag`
- `--module`
- `--testcase`
- `--level`
- `--latest`

Use `--format csv` for spreadsheet and pandas workflows.
