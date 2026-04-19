# Analysis UI and Cohorts

This guide walks through the full path from a list of domains to a browsable
analysis UI: create a tag, add domains to it, wire it to a cohort, run the
batch, and view results at `/analysis/`.

The analysis UI is a read-only public view of the materialized results for
one cohort at a time. Admins create and manage cohorts; end users just pick
one and browse.

---

## The two web surfaces

Gonemaster ships three UIs on the same server:

| Path | Purpose | Who sees it |
|---|---|---|
| `/` | Admin UI. Domains, tags, batches, cohorts, settings. | Operator only. Put behind auth / internal network. |
| `/analysis/` | Public analysis UI. Browse one cohort's materialized data. | Safe to expose to the internet. |
| `/public/` | Public per-domain result pages. | Safe to expose to the internet. |

The public API paths (`/pub/api/v1/`) back `/analysis/` and `/public/`. The
admin API (`/api/v1/`) backs `/`. If you expose this server to the internet,
block `/api/v1/` and `/` at the reverse proxy. See the nginx example in
[server.md § Reverse proxy setup](server.md#reverse-proxy-setup).

---

## Core concepts

**Tag**: a named collection of domains. Domains can belong to many tags.
Tags are the main grouping mechanism across the whole system (batches, API
filters, scoring views). See [data-analysis.md § Creating and managing
tags](data-analysis.md#creating-and-managing-tags).

**Cohort**: a public-facing "view" of one tag's analysis data. A cohort is
pinned to exactly one tag (its `source_tag`) and controls whether the
analysis tables get materialized for that tag and whether the result is
exposed in the public analysis UI.

The split exists because a tag is an internal grouping concept (any admin
can create one, at any time, for any reason) while a cohort is a curated
public dataset (you decide what gets published and under what label).

**Materialization**: for each completed run in a cohort's tag, the server
projects a set of derived "facts" (nameserver endpoints, address→ASN,
domain→ASN, summary) into `analysis_run_*` tables. The analysis UI reads
those tables. Raw `entries` are not used at read time.

---

## End-to-end workflow

### 1. Create a tag and add domains

Using the CLI (see [data-analysis.md](data-analysis.md) for the full set of
options):

```
gonemaster-client tags create tld --description "All IANA top-level domains"
gonemaster-client tags add-domains tld --file tlds.txt
```

Or in the admin UI: the **Tags** tab has create/add forms.

### 2. Create the cohort

Open the admin UI at `/`, go to the **Cohorts** tab, and fill in the form.
Cohorts are admin-UI-only today, there is no CLI subcommand for them.

| Field | Meaning |
|---|---|
| `source_type` | Always `tag` in the current version. |
| `source_tag` | The tag name from step 1 (e.g. `tld`). Immutable after creation. |
| `label` | Display name shown in the analysis UI and cohort picker. |
| `description` | Optional long description shown on the cohort overview. |
| `analysis_enabled` | If true, runs in this tag get materialized into the analysis tables. Disable to stop materialization without deleting data. |
| `public_enabled` | If true, the cohort appears in the analysis UI catalog. Requires `analysis_enabled=true`. |
| `is_default` | If true, this cohort is selected automatically when a visitor lands on `/analysis/` with no `?dataset_tag=` parameter. Only one cohort can be default. Requires both of the above. |
| `sort_order` | Controls the order of cohorts in the public catalog. Lower first. |

The same options are exposed on `POST /api/v1/analysis/cohorts` if you want
to script it.

### 3. Run a batch against the tag

```
gonemaster-client jobs batch --from-tag tld --tag tld --wait
```

`--from-tag` enqueues one run per domain in the tag; `--tag tld` links the
resulting runs back to the same tag so the cohort picks them up. As each
run finishes, the server projects it into the cohort's analysis tables
synchronously; there is no batch-level commit gate. The analysis UI shows
rows appearing live as workers complete each domain.

Re-running later is the same command; the UI updates per-domain as newer
runs land.

### 4. Browse the results

Navigate to `/analysis/`. If you set `is_default` on a cohort, it loads
directly; otherwise the first cohort in the catalog is picked. Users can
switch cohorts with the in-UI picker, which sets `?dataset_tag=<source_tag>`.

Typical tabs:

- **Overview**: cohort-level summary, counts, "Last analyzed".
- **Domains**: per-domain list with score, grade, worst level, operator ASN.
- **Nameservers / Endpoints / ASNs / Prefixes**: cross-cohort aggregates.
- **Tags / Testcases**: finding-level breakdowns.

Clicking through any chip or row links to the matching detail view.

---

## Rebuild, clear, and catch-up

Materialization normally keeps up on its own: each completed run is
projected immediately. A few admin actions are still available when you
need them.

- **Rebuild** (`POST /api/v1/analysis/cohorts/{id}/rebuild`, or the admin UI
  button): clears the cohort's materialization and reprojects every run
  whose domain is currently tagged. Use after changing tag membership, or
  if you suspect the materialized tables have drifted.
- **Clear** (`POST /api/v1/analysis/cohorts/{id}/clear`): removes all
  materialized rows for the cohort and leaves it in the `pending` state.
- **Disable analysis**: flip `analysis_enabled=false`. This clears the
  cohort's materialization; flipping it back to true triggers a rebuild.
- **Restart catch-up**: at server startup, any runs that finished while
  the server was down get projected incrementally. Cohorts that are
  already up to date are skipped. No action required.

Cohort state is visible on `GET /api/v1/analysis/status` and in the admin
UI: `pending`, `ready`, or `failed`, plus `last_materialized_at` and the
last error message.

---

## Configuration options

The analysis UI itself has no separate configuration. It inherits from the
server's existing settings.

### Database

The analysis tables live in the same database as `runs` and `entries`.
For larger cohorts (tens of thousands of domains, millions of runs over
time), use PostgreSQL. See
[database-setup.md](database-setup.md) for DSN formats, connection pooling,
indexes, and backup procedures. Driver selection and retention tuning are
documented in [server.md § Database](server.md#database) and
[data-analysis.md § Server configuration](data-analysis.md#server-configuration-for-analysis-workloads).

### Reverse proxy

To expose only the public surfaces (`/analysis/`, `/public/`, `/pub/api/v1/`)
to the internet while keeping the admin UI and admin API private, follow
the nginx or Caddy setup in
[server.md § Reverse proxy setup](server.md#reverse-proxy-setup).

A common deployment pattern is two server processes against one database:
an internal "analysis instance" for batch runs and admin tasks, and a
public instance that only serves the read paths. See
[data-analysis.md § Separating public UI and analysis
instances](data-analysis.md#deployment-note-separating-public-ui-and-analysis-instances).

### Public API rate limiting

The public endpoints backing `/analysis/` are covered by the same rate
limiter as `/pub/api/v1/`. Tune it with the options in
[server.md § Rate limiting](server.md#rate-limiting).

---

## Troubleshooting

**The analysis UI is empty even though a batch finished.** Confirm the
cohort has `analysis_enabled=true` and `public_enabled=true`, and that its
`source_tag` matches the tag used on the batch (`--tag`). Check
`GET /api/v1/analysis/status` for the cohort's state.

**"Last analyzed" does not update.** The stamp moves when a run is
projected or a rebuild finishes. If workers are running but the stamp is
stuck, check the server log for `analysis: project run ... failed` lines.

**A cohort is stuck in `failed`.** Read `last_materialization_error` on the
cohort (admin UI or `GET /api/v1/analysis/status`), fix the underlying
issue, then click **Rebuild**.

**Switching cohort in the URL does nothing.** The picker sets
`?dataset_tag=<source_tag>`. That value must match the cohort's
`source_tag`, not its label or ID.
