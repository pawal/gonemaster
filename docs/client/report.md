# Client Cohort Report

`gonemaster-client report` renders the comparison of two snapshots of an
analysis cohort as Markdown or JSON. It is the command-line form of the
Diff tab's report, and it reads the same endpoint,
`GET /pub/api/v1/analysis/cohorts/{dataset_tag}/report`.

What the report establishes, and what each classification and category
means, is in [../analysis/cohort-report.md](../analysis/cohort-report.md).
This page covers the command.

## Usage

```sh
gonemaster-client report [dataset-tag] --from SLUG --to SLUG
```

The dataset tag is the cohort's source tag. Omit it and the server's default
public cohort is used. Omit `--to` and the newest snapshot is used; omit
`--from` and the snapshot captured before `--to` is used. So the common case
is:

```sh
gonemaster-client report
```

Options:

- `--from SLUG`: the baseline snapshot.
- `--to SLUG`: the later snapshot.
- `--min-cluster N`: how many domains a cluster needs (server default 3).
- `--max-spread N`: how far the score moves within a cluster may spread
  (server default 3).
- `--snapshots`: list the cohort's snapshot slugs and exit.

`--format` selects the rendering and takes either position: `markdown` is
the default and `json` writes the response verbatim. `--output PATH` writes
to a file.

## Finding a Snapshot Slug

A snapshot is captured per batch, so comparing two batches of a cohort is the
same call. List the slugs with:

```sh
gonemaster-client report kommuner --snapshots
```

Each row gives the slug, the capture date, and the domain count. The same
list is served by `GET /pub/api/v1/analysis/cohorts/{dataset_tag}/snapshots`.

## Output

The Markdown report carries, in order: a provenance paragraph, the totals,
the movers by cause, the clusters, the three tag tables, and the movers. It
is the same content the Diff tab renders, in the same order.

## Examples

Compare the two newest snapshots of the default cohort:

```sh
gonemaster-client report
```

Compare two named snapshots of a cohort and write the Markdown to a file:

```sh
gonemaster-client report kommuner --from 2026-06 --to 2026-09 --output report.md
```

Fetch the same comparison as JSON:

```sh
gonemaster-client --format json report kommuner --from 2026-06 --to 2026-09
```

Require six domains to form a cluster:

```sh
gonemaster-client report kommuner --min-cluster 6
```

## See Also

- [Cohort report](../analysis/cohort-report.md) for the model.
- [Public analysis UI](../analysis/public-ui.md) for the browser form.
- `batches diff` in [batches.md](batches.md) compares two batches directly
  from the stored runs, without a cohort or a snapshot.
