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

When a signed zone is tested, the result page shows a collapsed "DNSSEC chain of
trust" section. Expanding it lazily fetches the stored chain summary and draws a
hand-rolled SVG graph of the parent DS records, the zone's DNSKEYs, and the
signatures linking them, with a parallel text summary for assistive tech. The
section only appears when `show_dnssec_chain_public` is enabled and the run has
chain data (public-UI runs only).

The start page also shows a "Recent tests" list: tests started in the same
browser, with the domain, the finish time, and the grade when public scoring
is enabled. Each row links back to the stored result. The list is kept only
in the browser (localStorage); nothing is stored on the server, and other
devices or browsers do not see it. It holds at most 20 entries. When a result
has expired on the server, opening it shows the normal expired page and the
entry is removed from the list. The "Clear" button empties the list.

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
