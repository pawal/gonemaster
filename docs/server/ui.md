# Server Web Interfaces

`gonemaster-server` serves three browser interfaces.

## Admin UI

Path: `/`

The admin UI is for trusted operators. It manages jobs, batches, queue state,
domains, tags, profiles, cohorts, settings, and result inspection.

Features include:

- single and batch job submission
- job and batch inspectors with auto-refresh
- queue controls
- domain registry browsing
- tag creation, editing, deletion, and membership management
- stored profile management
- cohort and snapshot administration
- translated result messages
- optional scoring and nameserver timing display

## Public Test UI

Path: `/public/`

The public test UI lets an end user submit one domain test and retrieve the
result through the restricted public API.

## Public Analysis UI

Path: `/analysis/`

The analysis UI is a read-only browser for public cohorts and snapshots. It is
backed by public analysis endpoints under `/pub/api/v1/analysis/`.

See [../analysis/public-ui.md](../analysis/public-ui.md) for cohort and
snapshot behavior.

## Development

Rebuild the embedded UI:

```sh
make ui-build
```

Run the UI dev server:

```sh
make ui-dev
```

The dev server runs on `http://localhost:5173` and calls the API on the
configured server host.
