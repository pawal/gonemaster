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

A result lives at `/public/result/<public_id>`. That path reaches the server, so
a shared link previews with the domain and grade, and clients without scripts
get a rendered summary in the `<noscript>` block. Result pages are `noindex`.
An unknown or expired id answers 404; one still running answers 200.

Links of the older `/public/#/result/<public_id>` form still resolve; the app
rewrites them on load.

`?lang=xx` selects the language and gives each locale a shareable URL, which is
what the `hreflang` tags and `sitemap.xml` advertise. English shares the plain
`/public/` URL. Without scripts only the page metadata is localized.

### Serving the public UI at the site root

`public_ui_path` is where visitors reach the UI under `public_url`. It defaults
to `public/`; set it to `""` when a proxy maps the site root onto the UI. It
changes the URLs advertised in `og:url`, canonical, `hreflang` and
`sitemap.xml`, and the base the SPA builds links from. The server still mounts
at `/public/`, so the proxy must keep passing that through for assets, along
with `/pub/api/v1/`, `/sitemap.xml` and `/robots.txt`. A Caddy example:

```
gonemaster.example {
    @passthrough path /pub/api/v1/* /public/* /analysis /analysis/* /sitemap.xml /robots.txt
    handle @passthrough {
        reverse_proxy backend:8080
    }
    handle {
        rewrite * /public{uri}
        reverse_proxy backend:8080
    }
}
```

Without the `/sitemap.xml` and `/robots.txt` passthrough, the rewrite sends them
into the SPA fallback, which answers 200 with HTML.

When a signed zone is tested, the result page shows a collapsed "DNSSEC chain of
trust" section. Expanding it lazily fetches the stored chain summary and draws a
hand-rolled SVG graph of the parent DS records, the zone's DNSKEYs, and the
signatures linking them, with a parallel text summary for assistive tech. The
graph frames each zone under a header naming it, and the tested zone's frame
carries the roll-up status. Every box is a tab stop, and clicking or pressing
Enter on one repeats its details in a panel below the graph. A "Save as SVG"
button saves the graph as a standalone file in the theme the reader is
viewing. The section only appears
when `show_dnssec_chain_public` is enabled and the run has chain data
(public-UI runs only).

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
