# Gonemaster documentation site

This directory contains the Hugo site that renders the project documentation
under `docs/` as a static website. The `docs/` directory is mounted read-only;
no files there are modified by the site build.

## Requirements

Hugo extended, version >= 0.121.0. The site uses the Relearn theme (git
submodule), so submodules must be initialised before building.

Install Hugo extended on Ubuntu/Debian:

```
sudo apt install hugo
```

Or download a binary from https://github.com/gohugoio/hugo/releases — pick a
release whose filename contains `extended`.

After cloning the repo, initialise the theme submodule:

```
git submodule update --init
```

## Building

```
make docs
```

Output is written to `site/public/`. That directory is gitignored and contains
the complete static site (HTML, CSS, JS, OpenAPI spec, Lunr search index).

## Serving locally

```
make docs-serve
```

Opens a live-reloading dev server at http://localhost:1313. The site is served
at the path prefix `/gonemaster/` to match the production URL, so the local
address is http://localhost:1313/gonemaster/.

## Verifying the standalone build

After `make docs`, serve `site/public/` with any static file server to confirm
it works without Hugo:

```
python3 -m http.server -d site/public/ 8080
```

Then open http://localhost:8080/gonemaster/.

## Site structure

- `hugo.toml` — Hugo config, theme, module mounts, output formats
- `content/` — overlay pages: section `_index.md` stubs and the API reference page
- `layouts/shortcodes/` — `include.html` (renders README files) and `redoc.html` (OpenAPI viewer)
- `layouts/partials/pageHelper/title.hugo` — title computation (humanize + testcase heading fallback)
- `data/titles.toml` — manual title overrides for a handful of pages
- `static/js/redoc.standalone.js` — vendored Redoc 2.5.2 (no CDN)
- `themes/relearn` — Relearn theme, pinned to 6.4.1 as a git submodule
