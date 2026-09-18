# Cohort Report

A cohort that looks worse than last time may have got worse, or the engine
may have started looking harder. A plain snapshot diff cannot tell the two
apart: a tag appearing on 48 domains because the engine gained a testcase
reads exactly like 48 domains breaking.

The cohort report separates them. It compares two snapshots of one cohort and
classifies every change against the tag vocabulary each snapshot was levelled
under, which the server records at capture. It is computed server-side from
the snapshot view tables, so it survives the retention purge of the runs
behind it.

The report states what moved. It does not write prose, does not recompute
scores under a pinned vocabulary, and does not confirm a fault against live
DNS.

## Endpoint

```
GET /pub/api/v1/analysis/cohorts/{dataset_tag}/report?from=<slug>&to=<slug>
```

Optional `min_cluster` and `max_spread` bound cluster detection. The response
carries a header, totals, cohort-wide tag rows, the moving domains, and the
clusters. `GET .../diff` keeps its shape for existing consumers; the report
is a superset.

## Provenance

The header records what a reader needs before trusting any row below it:
both engine versions, both profile names, the tag vocabulary delta, whether
the scoring configuration changed, and the tag floor.

The tag floor is the stricter of the two snapshots' tag view floors. Findings
below it are absent from the per-domain lists. `NOTICE` carries a one-point
penalty and `INFO` none, so this bounds what a score attribution can explain.

`scoring_config_changed` is `true`, `false` or `unknown`. A rebuilt snapshot
reports `unknown`: the engine version and the vocabulary can be recovered
from a retained run, the scoring configuration cannot. See
[snapshots.md](snapshots.md).

## Classification

Every cohort-wide tag row and every per-domain finding carries one
classification:

| Classification | Meaning |
|---|---|
| `cohort_change` | The tag is in both vocabularies at the same severity. The domains moved. |
| `new_in_engine` | The tag is only in the later snapshot's vocabulary. The engine gained it. |
| `removed_from_engine` | The tag is only in the baseline's vocabulary. |
| `level_reclassified` | The tag is in both vocabularies at different severities, and the finding moved with it. |
| `unknown` | A vocabulary could not be read. No verdict is available. |

`unknown` never collapses into `cohort_change`. A snapshot captured before
vocabulary provenance existed, and whose runs have since been purged, reports
`unknown` for every row rather than a verdict it cannot support.

One case is correctly classified and still misleading: a testcase whose gate
changed without its tag changing classifies as `cohort_change`. The report
shows that the vocabulary was unchanged while the engine version was not,
which is the signal to go looking.

## Domain Categories

Each moving domain rolls its findings up to one cause:

| Category | Meaning |
|---|---|
| `real` | Only cohort changes moved it. |
| `measurement` | Only engine changes moved it. |
| `mixed` | Both. |
| `unknown` | Its score moved with no visible finding change: the cause is below the tag floor or outside the findings. |

## Score Attribution

Each moving domain carries its score delta and `explained_delta`, the part
of that delta its listed findings account for under the active scoring
configuration. Penalties are weighted by scoring category the way the score
itself is computed, since the score is a weighted mean of per-category
sub-scores rather than a flat sum.

`unexplained_delta` is the remainder. It names a cause the report cannot
see: a finding below the tag floor, sub-score clamping, the CRITICAL
override, or the A+ bonus, none of which are modelled.

## Clusters

Clusters group movers that moved together and share a dimension value:
nameserver, ASN, prefix, or nameserver software version string. Nine domains
improving by the same 8 to 9 points behind one nameserver is one fact about
one operator, not nine independent events.

`min_cluster` sets how many domains a cluster needs (default 3) and
`max_spread` how far their score moves may spread (default 3). A cluster
never mixes improvement with regression. Two dimensions producing the same
member set collapse into one cluster naming both.

A cluster is a fact, not a cause. "Nine domains sharing version string X all
moved +8 to +9" is what the report says. Deciding that an operator upgraded
is the reader's.

## Reading It

Three consumers render the same response:

- The analysis UI's Diff tab. See [public-ui.md](public-ui.md).
- `gonemaster-client report`, as Markdown or JSON. See
  [../client/report.md](../client/report.md).
- The `cohort_report` MCP tool. See
  [../mcp/analysis-examples.md](../mcp/analysis-examples.md).

Snapshots are captured per batch, so comparing two batches of a cohort is the
same call.
