# MCP Tool Reference

Every tool below is served by both the stdio bridge and the server
endpoint. Inputs are JSON object fields; a field marked optional MAY be
omitted. Limits are clamped to the server's caps, never rejected.

Each tool carries MCP annotations. Read tools are `readOnlyHint: true`
with a closed world. `test_domain` and `batch_enqueue` are additive writes
in an open world: they create runs and send DNS queries. `batch_cancel`
and `cancel_job` are destructive.

## Two kinds of tags

gonemaster has two unrelated things called tags:

- **Finding tags** are the message tags testcases emit, such as `SOATIME`
  or `N16_HAS_NSID`. `run_search`'s `tag`, `failures_by_tag`,
  `cohort_tag_values`, and `spec_get_testcase` use these.
- **User domain tags** are named groups of domains that batches are built
  from, such as `se-weekly`. `domain_tag_list` lists them and
  `batch_enqueue`'s `from_tag` takes one.

## Findings and `min_level`

`test_domain` and `run_get` return every stored log entry by default. The
server stores entries at its `min_level` and above, `INFO` by default, so
a result is often dominated by `INFO` lines. Pass `min_level` (`NOTICE`,
`WARNING`, `ERROR`, or `CRITICAL`) to keep only issues. `level_counts`
always reports how many stored entries each level has, so a floor hides
nothing silently.

## Connectivity

### ping

Checks the server and the credential.

| Output | Meaning |
|---|---|
| `server_url` | The admin API base the tools call. |
| `reachable` | `true` when a gonemaster admin API answered. |
| `auth_mode` | `open` or `token`. |
| `authenticated` | Whether the credential is accepted; always `true` in open mode. |
| `detail` | `ok`, or what to fix. A `404` from the URL is reported as "not a gonemaster-server admin API". |

## Testing and reading runs

### test_domain

Submits one domain and waits for the verdict. Sends progress
notifications when the client passes a progress token.

| Input | Default | Notes |
|---|---|---|
| `domain` | required | |
| `profile_id` | the default profile | An id from `profile_list`. |
| `timeout_seconds` | 120 | Maximum 600. On timeout the error names the run id; fetch it later with `run_get`. |
| `lang` | `en` | Language of the rendered messages. |
| `min_level` | every stored level | Severity floor for `findings`. |

Output: `domain`, `run_id`, `batch_id` (empty for an ad-hoc run),
`public_id`, `status` (`succeeded`, `failed`, `canceled`, `expired`),
`grade`, `score`, `duration_ms`, `findings` (`tag`, `level`, `module`,
`testcase`, `message`), `level_counts`, `nameserver_timings`
(`nameserver`, `address`, `median_ms`, `avg_ms`, `min_ms`, `max_ms`,
`count`, `status`), `error`.

A failed result request fails the call, except a `404`, which leaves the
run's status and error in place with no findings.

### run_get

Fetches a stored run by id. Inputs `id` (required), `lang`, `min_level`.
Output as `test_domain`.

### latest_for

Lists a domain's completed runs, newest first. The domain is matched whole.

| Input | Default | Notes |
|---|---|---|
| `domain` | required | Exact match. |
| `limit` | 5 | Maximum 500. |

Output: `domain`, `count`, `runs` with `run_id`, `domain`, `batch_id`,
`public_id`, `status`, `grade`, `score`, `worst_level`, `duration_ms`,
`finished_at`.

### run_search

Searches completed runs, newest first.

| Input | Default | Notes |
|---|---|---|
| `domain` | | Substring match unless `exact` is set. |
| `exact` | `false` | Match `domain` whole. |
| `tag` | | A finding tag the run emitted. |
| `status` | | `succeeded`, `failed`, `canceled`, `expired`. |
| `level` | | Worst severity, for example `WARNING`. |
| `grade` | | A letter grade. |
| `finished_after`, `finished_before` | | RFC 3339 bounds. |
| `limit` | 20 | Maximum 500. |
| `offset` | 0 | |

Output: `count`, `total` (matches before paging), `runs` as `latest_for`.

### run_diff

Compares two runs at the tag level. Inputs `run_a` (baseline), `run_b`,
`lang`. Output: `grade_a`, `grade_b`, `added` (tags only in `run_b`),
`removed` (tags only in `run_a`), `changed` (tags in both whose worst
level differs), each row with `tag`, `module`, and `level` or
`from_level` and `to_level`.

## Reference and discovery

### spec_list_testcases

Lists implemented testcases. Input `category` filters to one module.
Output rows: `id`, `module`, `description`, `excluded` (`true` when the
server never runs it).

### spec_get_testcase

Describes one testcase. Inputs `testcase` (an id such as `dnssec09`, not a
finding tag), `lang`. Output: `id`, `module`, `description`, `excluded`,
`locale`, `tags` with `tag` and the rendered `message` template.

### profile_list

Lists the stored test profiles. Output rows: `id` (for `test_domain`),
`name` (for `batch_enqueue`), `description`, `public`.

### domain_tag_list

Lists user domain tags. Input `limit` (default 100, maximum 500). Output
rows: `name`, `description`, `domain_count`, `default_profile_id`.

### cohort_list

Lists the public analysis cohorts, and the snapshots of one when
`dataset_tag` is given. Output: `default_tag`, `cohorts` (`dataset_tag`,
`label`, `is_default`), and with `dataset_tag` also `snapshots`, newest
first, with `slug`, `label`, `captured_at`, `domain_count`. Slugs feed
`cohort_report`.

### cohort_schedule_list

Lists the cohort snapshot schedules. No inputs. Output: `count`,
`schedules` with `dataset_tag`, `label`, `enabled`, `summary` (the rule in
English), `next_run_at`, `last_run_at`, `last_outcome` (`submitted`,
`skipped_missed`, `skipped_disabled`, `skipped_active`, `skipped_empty` or
`error`) and `last_batch_id`, which `batch_get` takes. A server without the
analysis controller answers an error.

## Batches and cohorts

A batch is one test run over many domains.

### batch_list

Lists batches, newest first. Inputs `label` (substring of the batch tag),
`limit` (default 20, maximum 100). Output: `count`, `total`, `batches`
with `batch_id`, `tag`, `description`, `status` (`done` or `running`),
`total`, `completed`, `completion` (percent), `created_at`, `finished_at`.

### batch_get

Polls one batch. Input `batch_id`. Output: `batch_id`, `tag`, `total`,
`status_counts`, `done` (no job queued, running, or paused), `created_at`,
`finished_at`.

### cohort_stats

Grade and worst-severity distribution over a batch's completed runs.
Input `batch_id`. Output: `batch_id`, `total`, `grades`, `worst_levels`.
One call against a current server; against a server without
`worst_levels` in its batch summary the tool pages the runs instead.

### failures_by_tag

Ranks the finding tags in a batch at or above a severity.

| Input | Default | Notes |
|---|---|---|
| `batch_id` | required | |
| `severity_min` | `WARNING` | `NOTICE`, `WARNING`, `ERROR`, or `CRITICAL`. |
| `limit` | 20 | Ranked tags to return. |

Output: `tags` ranked by `count`, the number of distinct domains carrying
the tag, with `entries` (log entries; higher than `count` when a tag fires
once per nameserver), `worst_level`, and up to three `example_domains`.
The scan runs from `CRITICAL` down and stops after 5000 entries; `capped`
and `capped_at_level` then say which level was cut, and every more severe
level is complete.

### cohort_tag_values

Rolls up the values one argument of one finding tag takes across a batch.

| Input | Default | Notes |
|---|---|---|
| `batch_id`, `tag`, `arg` | required | The `(tag, arg)` pair comes from the [log-args inventory](../specifications/log-args-inventory.json). |
| `min_count` | 1 | Hide values carried by fewer domains. |
| `limit` | 50 | Maximum 500. |
| `weight_by_score` | `false` | Rank by mean domain score instead of count. |

Output rows: `value`, `count` (distinct domains), `avg_score` (with
`weight_by_score`), `sample_domains` (up to 10). List-valued arguments
contribute one count per element.

### cohort_report

Compares two snapshots of an analysis cohort and classifies every change
as engine-driven or real.

| Input | Default | Notes |
|---|---|---|
| `dataset_tag` | the default cohort | From `cohort_list`. |
| `from`, `to` | the two newest snapshots | Slugs from `cohort_list`. |
| `limit` | 20 | Rows per list, maximum 200. |
| `min_cluster`, `max_spread` | server defaults | Cluster bounds. |

Output: `provenance` (engine versions, vocabulary deltas, scoring config
state, tag floor, profiles), `totals`, `tags_appeared`, `tags_cleared`,
`tags_level_changed` (each with `classification`: `new_in_engine`,
`removed_from_engine`, `level_reclassified`, `cohort_change`, `unknown`),
`movers` (each with `category`: `real`, `measurement`, `mixed`, `unknown`),
`clusters`, `truncated`. The model is described in
[../analysis/cohort-report.md](../analysis/cohort-report.md).

## Write tools

Registered only with `GONEMASTER_MCP_ALLOW_WRITE=1` on the bridge or
`mcp_allow_write` on the server.

### batch_enqueue

Enqueues a batch. Inputs: `domains` or `from_tag` (one is required),
`profile` (a `profile_list` name), `tags` (user domain tags to apply).
Output: `batch_id`, `job_count`. Poll with `batch_get`.

### batch_cancel

Cancels a batch's in-flight jobs and deletes the batch with its derived
data. Input `batch_id`. Output: `batch_id`, `canceled`.

### cancel_job

Cancels one queued or running job. Input `job_id`. Output: `job_id`,
`status`.

## Errors

A tool call fails with a text error. `401` names the fix: on the bridge,
set `GONEMASTER_TOKEN`; on the endpoint, the bearer token was rejected.
`404` is "not found". Other `4xx` carry the server's message as "request
rejected". `5xx` and transport failures are reported as server errors.

## Public report links

`run_get`, `run_search`, `latest_for`, and `test_domain` include a
`public_id` when the run has one. The shareable report is
`https://<host>/#/result/<public_id>`.
