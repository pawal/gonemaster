# MCP Analysis Examples

Worked examples of driving gonemaster analysis from an MCP-capable agent.
Companion to the bridge overview in [README.md](README.md), which covers
build, configuration, and the full tool list.

The same questions can also be answered with SQL or the admin API; see
[../analysis/querying.md](../analysis/querying.md) for those paths. MCP is
the right choice when the agent should explore interactively without
needing schema knowledge or DB access.

## Plain-Language Queries

You do not have to know the tool names. In an MCP-capable client (Claude
Code, Claude Desktop, Cursor, Cline, Zed, or any agent built on an MCP
SDK), describe the question in normal prose and the agent picks the right
tools, chains them together, and turns the JSON into a readable answer.
The direct tool calls shown later in this document are what the agent
will end up calling on your behalf.

A few examples of prompts that work well with a capable LLM (the examples
below were run against Claude):

> **"Give me a summary of all the TLDs gonemaster has tested."**
>
> Agent calls `batch_list(label="TLDs")` to find the canonical cohort,
> then `cohort_stats(batch_id=...)` on the latest one, and reports the
> grade and severity distribution plus the cohort size.

> **"Highlights from the latest municipalities job."**
>
> Agent calls `batch_list(label="Kommuner")`, then on the newest batch
> calls `cohort_stats` for the headline numbers and
> `failures_by_tag(severity_min="ERROR")` for the top failure modes.

> **"Which ASNs serve the most Swedish municipalities, and are any of
> them regional?"**
>
> Agent calls `cohort_tag_values(tag="IPV4_ONE_ASN", arg="asn")`,
> then groups the sample-domain lists geographically and reports both
> the big national providers and the small ASNs that cluster on
> specific regions.

> **"Did anything regress for example.se in the last test compared to
> the previous one?"**
>
> Agent calls `latest_for(domain="example.se", limit=2)` to get the two
> most recent run ids, then `run_diff` between them, and summarises the
> added or severity-changed tags.

Two caveats worth knowing:

- Some questions need post-processing the agent does locally (for
  example "top 10 slowest TLDs" requires aggregating `duration_ms` from
  `run_search` results - no cohort tool exposes that today). The agent
  handles this in its own scratch space; you get the answer either way,
  but a single MCP call cannot.
- The agent reasons about Swedish geography, ASN ownership, or
  vendor names from its general knowledge. Treat those labels as
  hints, not source of truth; verify with `whois` or registry data
  before quoting them in a report.

## Discover a Cohort

Goal: find the batch_id of the latest "Kommuner" (municipalities) run.

```
batch_list(label="Kommuner", limit=5)
```

Returns recent batches matching the label substring, newest first. Each
entry includes `batch_id`, `tag`, `total`, `completed`, and `finished_at`.
Feed the resulting `batch_id` into the cohort tools below.

`batch_list` with no `label` lists the most recent batches across all tags.

## Cohort Grade and Severity Distribution

Goal: summarise quality across a completed batch.

```
cohort_stats(batch_id="batch_1780476907150173141_2")
```

Returns counts by letter grade (`A+`, `A`, `B`, `C`, `D`, `F`) and by
worst-severity level (`NOTICE`, `WARNING`, `ERROR`, `CRITICAL`). One call,
one row each - cheaper than paging `run_search`.

## Top Failure Tags

Goal: which message tags drive the WARNINGs and ERRORs in this batch?

```
failures_by_tag(
  batch_id="batch_1780476907150173141_2",
  severity_min="ERROR",
  limit=15
)
```

Returns tag names ranked by count, with three example domains per tag and
the worst level seen for that tag. Use `severity_min="WARNING"` for the
broader picture; `severity_min="ERROR"` narrows to the failures most worth
acting on.

## Operator / ASN Concentration

Goal: which ASNs serve domains in this cohort?

```
cohort_tag_values(
  batch_id="batch_1780476907150173141_2",
  tag="IPV4_ONE_ASN",
  arg="asn",
  limit=20
)
```

Returns each value of `asn` with its count and a small list of sample
domains. Useful for spotting both the dominant national providers and the
small regional vendors that serve a tight cluster of domains.

To rank by quality rather than reach, add `weight_by_score=true` and the
result is sorted by mean domain score, surfacing the ASNs whose domains
score best (or worst).

The `(tag, arg)` pair must come from the
[log-args inventory](../specifications/log-args-inventory.json); use
`spec_list_testcases` and `spec_get_testcase` to find which tags a module
emits and which args they carry.

## Follow a Finding to the Affected Runs

Goal: which runs raised a particular tag, and what did the worst-case look
like?

```
run_search(tag="SAME_IP_ADDRESS", level="ERROR", limit=20)
run_get(run_id="job_1780476907230635262_134")
```

`run_search` filters completed runs by domain, tag, status, severity,
grade, or finish-time range; `run_get` fetches one run's full result
(per-testcase findings, per-nameserver response times, score breakdown).

Each result row from `run_search`, `run_get`, and `latest_for` includes a
`batch_id` (empty for ad-hoc runs) and a `public_id`. Build a shareable
report link as `https://<host>/#/result/<public_id>`.

## Compare Two Runs of One Domain

Goal: did anything change between the last two test runs of a domain?

```
latest_for(domain="example.se", limit=2)
run_diff(run_id_a="<older>", run_id_b="<newer>")
```

`run_diff` returns tag-level deltas (added, removed, severity-changed),
which is the right granularity for "did this regression appear today, or
was it already there".

## One-Shot Re-Test

Goal: re-run a single domain now and read the result.

```
test_domain(domain="example.se")
```

Synchronous: blocks until the engine finishes, returns grade, score,
findings, and per-nameserver response times. No batch needed.

## Inspect a Testcase

Goal: understand what a specific testcase checks and what tags it emits.

```
spec_list_testcases(category="dnssec")
spec_get_testcase(testcase="ds03")
```

Returns module, description, and the message tags the testcase can emit
with their rendered messages. Use this to translate a `failures_by_tag`
result into "what is gonemaster actually checking here".

## Anti-Patterns

- **Paging `run_search` to compute per-cohort stats.** Use
  `cohort_stats`, `failures_by_tag`, or `cohort_tag_values` instead -
  they aggregate server-side in one call.
- **Treating MCP as the only path.** For repeated programmatic use over
  large result sets, `gonemaster-client` or direct SQL is faster and
  cheaper. MCP shines for agent-driven exploration where the next
  question depends on the previous answer.
- **Calling write tools without enabling them.** `batch_enqueue`,
  `batch_cancel`, and `cancel_job` register only when
  `GONEMASTER_MCP_ALLOW_WRITE=1` is set on the bridge.
