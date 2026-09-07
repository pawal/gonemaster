# Public Analysis UI

The public analysis UI is served at `/analysis/`. It is read-only and backed by
public analysis endpoints under `/pub/api/v1/analysis/`.

Operators create tags, cohorts, batches, and snapshots through the admin UI or
admin API. Public visitors only browse published cohort data.

## Web Surfaces

| Path | Purpose | Audience |
|---|---|---|
| `/` | Admin UI for jobs, tags, batches, cohorts, settings. | Trusted operators. |
| `/analysis/` | Public cohort analysis UI. | Public visitors. |
| `/public/` | Public single-domain result UI. | Public visitors. |

Block `/` and `/api/v1/` at the public reverse proxy. See
[../server/public-api-and-proxy.md](../server/public-api-and-proxy.md).

## What It Shows

The UI presents one cohort and one snapshot at a time. Visitors can browse:

- overview metrics
- domains
- nameservers
- endpoints
- ASNs and prefixes
- finding tags
- testcases
- snapshot trends
- snapshot diffs

Clicking an entity such as a domain, nameserver, ASN, prefix, finding tag, or
testcase opens a filtered detail view.

## URL State

`dataset_tag` selects the cohort. `snapshot` pins the snapshot.

```text
/analysis/?dataset_tag=tld&snapshot=2026-04-20-strict
```

The UI preserves the active snapshot as visitors click through detail pages.
Shared links should include `snapshot` when the numbers must remain stable.

Some views add their own parameters: the diff `tab`, and the trends focused
bucket `key` and scale `scale`. Back, forward, and shared links reproduce the
same view.

## Overview

The overview page opens with stat tiles for the cohort: total domains, the
share with no warnings, the share graded A or A+, and the share that is
signed. Each tile shows the change since the previous snapshot and a small
sparkline across snapshots. Below the tiles it lists the top issues and the
main nameservers, ASNs, and prefixes, including the leading provider's share
of domains. A movers card lists the domains that improved or regressed most
since the previous snapshot.

## Trends

The trends page shows how the grade, severity, DNSSEC, and DNSKEY algorithm
mix changes over time, one stacked bar per snapshot.

Click a bucket in the legend to focus it: the bars are replaced by a single
line chart of that bucket across snapshots. `key` holds the focused bucket and
`scale` switches between share and absolute count. Click the bucket again or
press Escape to go back. A top-movers panel lists the finding tags that
changed most between the first and last snapshot shown.

## Snapshot Diffs

The diff view compares two snapshots of a cohort. A summary strip counts how
many domains regressed, improved, were added, or removed. The change lists
link each domain, and a grade-transition matrix shows how grades moved. Use
the swap button to reverse the two snapshots; `tab` selects the active list so
a shared link opens on the right one. When only a "to" snapshot is chosen, the
snapshot before it is used as "from".

Alongside the per-domain changes it shows a tag-level summary: which finding
tags appeared, cleared, or changed severity cohort-wide, with domain counts.
This makes a regression explainable, for example "14 domains regressed;
DS02_NO_MATCHING_DS appeared on 12 of them". The tag section degrades
gracefully: if a snapshot has no materialized tag view it shows a short notice
rather than an error.

## Latency

The nameserver, address, and ASN lists show a Latency column with the median
(p50) and 95th-percentile response time aggregated from the queries the
snapshot already recorded (no extra probing). The column only appears when the
snapshot has latency data; snapshots captured before latency aggregation show
no column until they are re-materialized (admin "Rebuild aggregates" or a
cohort rebuild).

The queries were made from wherever the instance runs, so the figures describe
one network vantage point rather than the entity's latency everywhere. The
column header and a footnote under the table repeat that caveat, as does the
overview's response-time card.

## Entity History

The nameserver, ASN, tag, and domain detail pages show a small sparkline of
how the entity moved across the cohort's captured snapshots (domain count for
nameserver/ASN/tag, score for a domain). It is fed by
`GET /pub/api/v1/analysis/cohorts/{dataset_tag}/history?entity=&key=` and only
appears when at least two snapshots carry the entity, so single-snapshot
cohorts and brand-new entities show nothing rather than a flat line.

## Reference Links

Every detail page carries an "Elsewhere" block linking the entity to its
authoritative external reference. A top-level domain links to the IANA root
zone database, its ICANNWiki page, and IANA's RDAP service; any other domain
links to an RDAP record for it. Addresses, prefixes, and ASNs link to
RIPEstat.

The links are plain navigations: nothing is requested from a third party until
a visitor clicks one. They open in a new tab with `rel="noopener noreferrer"`,
so the snapshot URL never reaches the target site.

## Keyboard Shortcuts

Press `?` for the list of shortcuts. `g` followed by a letter jumps between
sections (for example `g d` for domains, `g t` for tags), `/` focuses the
search box, and Escape closes the help. Shortcuts keep the current cohort and
snapshot.

## Exports

The list pages export CSV and JSON. An export covers the whole filtered set,
up to the server's 500-row limit, not just the rows on screen. A note next to
the buttons says how many rows it will include, and the file name marks an
export that hit the limit.

## Empty States

If the UI is empty after a batch finishes, check:

- the cohort has `analysis_enabled=true`
- the cohort has `public_enabled=true`
- the batch used the cohort source tag
- the batch was marked as snapshot-intent when it should publish
- `GET /api/v1/analysis/status` has no cohort error

If "Last analyzed" does not move, check server logs for analysis projection
errors and inspect `last_materialization_error` on the cohort.
