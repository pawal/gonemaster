# Cohorts

A cohort is a curated public analysis dataset backed by one source tag. Tags
are internal grouping tools. Cohorts decide which tagged results are
materialized and whether they are exposed in the public analysis UI.

## Core Model

| Concept | Meaning |
|---|---|
| Tag | Named domain collection used by operators. |
| Cohort | Public dataset backed by one source tag. |
| Materialization | Derived analysis rows built from completed runs. |
| Snapshot | Immutable public view captured from one snapshot-intent batch. |

The split between tag and cohort is deliberate. A tag can exist for any
operator task. A cohort is a publication decision.

## Settings

| Field | Meaning |
|---|---|
| `source_type` | Always `tag` in the current version. |
| `source_tag` | Tag that supplies the cohort domains. Immutable after creation. |
| `label` | Display name shown in the public analysis UI. |
| `description` | Optional long description shown on the cohort overview. |
| `analysis_enabled` | Whether matching runs are projected into analysis tables. |
| `public_enabled` | Whether the cohort appears in the public catalog. |
| `sort_order` | Display order in the public catalog. |
| `reference_list` | Optional external list the source tag is compared against. Empty means no comparison; the only other value is `iana_tlds`. |

Only `analysis_enabled` cohorts receive materialized rows. Only
`public_enabled` cohorts are listed on the public path.

## Source Drift

A source tag is filled once. Delegations change, so the tag and the
population it was meant to represent diverge over time. Setting
`reference_list` makes the server report that divergence.

`reference_list = iana_tlds` compares the tag against the IANA list of
delegated top-level domains. The list comes from the external data provider,
so it is reported only when `external_data.enabled` is true and the dataset
has been fetched. See
[configuration.md](../server/configuration.md#external-reference-data).

`GET /api/v1/analysis/cohorts/{id}` then carries a `source_drift` object:

| Field | Meaning |
|---|---|
| `checked_at` | When the comparison was made. It is derived per request, not stored. |
| `list_version` | Version marker of the reference list used. |
| `missing` | Names in the list that are not in the source tag. |
| `extra` | Names in the source tag that are not in the list. |

Comparison is on the A-label, case-insensitive, with a trailing root dot
ignored. The field is absent when the cohort names no list, the provider is
off, or the list has not been fetched yet: an unfetched list would otherwise
report every member of the tag as extra.

The admin cohort row shows the two counts as `Drift: +N / -M` and expands to
both name lists. Drift is reported only. Fixing it stays an explicit tag
operation:

```sh
gonemaster-client tags add-domains tld --file new-tlds.txt
gonemaster-client domains untag retired.example tld
```

The next rebuild or snapshot then reflects the corrected tag.

## End-to-End Workflow

1. Create a tag and add domains.

   ```sh
   gonemaster-client tags create tld --description "Top-level domains"
   gonemaster-client tags add-domains tld --file tlds.txt
   ```

2. Create a cohort for that tag in the admin UI under
   **Settings > Analysis > Cohorts**, or use the admin API:

   ```bash
   curl -s -X POST http://localhost:8080/api/v1/analysis/cohorts \
     -H "Content-Type: application/json" \
     -d '{"source_tag": "tld", "label": "TLDs", "analysis_enabled": true}'
   ```

3. Run a snapshot-intent batch for the source tag.

   The admin UI has a **Run new snapshot** action on the cohort row. The batch
   form also has **Capture as cohort snapshot** when the selected tag backs a
   cohort. To repeat the snapshot on a calendar, give the cohort a schedule;
   see [Scheduling](#scheduling).

4. Browse the public view at `/analysis/`.

## Materialization

For each completed run in an analysis-enabled cohort, the server projects
derived facts into analysis tables:

- domain summary
- nameserver endpoints
- address to ASN facts
- domain to ASN facts
- finding tag summaries
- small domain facts such as signing and algorithm categories

The public UI reads these tables. It does not scan raw `entries` for every
request.

## Status and Repair

Cohort state is visible in the admin UI and through:

```bash
curl -s http://localhost:8080/api/v1/analysis/status
```

Materialization statuses:

- `pending`: materialization has not finished.
- `ready`: materialized rows are usable.
- `failed`: projection failed. Inspect `last_materialization_error`.

Repair actions:

```bash
curl -s -X POST http://localhost:8080/api/v1/analysis/cohorts/{id}/rebuild
curl -s -X POST http://localhost:8080/api/v1/analysis/cohorts/{id}/clear
```

Rebuild clears and reprojects matching runs. Clear removes materialized rows and
leaves the cohort pending. Disabling analysis clears materialization; enabling
it again triggers a rebuild.

## Scheduling

A cohort MAY carry one schedule. On each occurrence the server submits the
batch **Run new snapshot** submits: `from_tag` set to the source tag,
`snapshot_intent` true, and the tag default profile unless the schedule
names one. The batch carries `origin` `schedule`. Scheduling requires the
analysis controller, which runs on the SQL backends only.

### Rules

| Kind | Fields | Occurs |
|---|---|---|
| `interval` | `interval_days` (1 to 365), `anchor_date` (`YYYY-MM-DD`) | On `anchor_date` and every `interval_days` days after it. |
| `weekly` | `weekdays`, one or more of `mon` to `sun` | On each listed weekday. |
| `monthly` | `days_of_month` (1 to 28), `last_day` | On each listed day, and on the last day of the month when `last_day` is true. |

Every kind takes `time_of_day` (`HH:MM`) and `timezone`, an IANA time zone
name, `UTC` by default. Occurrences are computed in that zone. Days 29 to 31
are refused so that no month skips an occurrence; `last_day` names the end of
every month. Twice a month is a `monthly` rule with two days; every second
week is an `interval` rule of 14 days, which does not follow month
boundaries.

At a daylight saving transition:

- A `time_of_day` that does not exist that day moves forward by the length
  of the gap: 02:30 on 2026-03-29 in `Europe/Stockholm` runs at 03:30.
- A `time_of_day` that exists twice runs once, at the first instant.

### Fields

| Field | Default | Meaning |
|---|---|---|
| `enabled` | `true` | A disabled schedule never fires. |
| `profile_id` | `null` | Stored profile for the batch; `null` uses the tag default. |
| `promote_default` | `false` | Pin each captured snapshot as the cohort default. |
| `catch_up` | `true` | Run one missed occurrence after downtime. |
| `next_run_at` | computed | The next occurrence in UTC, set from the time of every `PUT`. |
| `last_run_at`, `last_outcome`, `last_batch_id`, `last_error` | | The last firing. |
| `summary` | | The rule in English. |

### Firing

The server checks for due schedules at startup and once a minute. For each
due schedule, oldest `next_run_at` first, it:

1. computes the first occurrence after the current time and moves
   `next_run_at` there with a compare-and-set update; a schedule another
   check has already moved is left alone;
2. decides the outcome by the first matching row below;
3. records the outcome, the time and, for `submitted`, the batch id.

| Outcome | Condition |
|---|---|
| `skipped_missed` | `catch_up` is false and the occurrence is more than one hour late. |
| `skipped_disabled` | The cohort is gone or its analysis is disabled. |
| `skipped_active` | The last batch the schedule submitted has queued, running or paused jobs. |
| `skipped_empty` | The source tag has no domains. |
| `error` | Submission failed; `last_error` holds the reason. |
| `submitted` | The batch is queued; `last_batch_id` names it. |

Downtime that spans several occurrences yields at most one run, since the
next occurrence is computed from the current time. Runs never overlap: an
occurrence that falls while the previous scheduled batch is active is
skipped. A stored rule that no longer yields an occurrence, such as one
naming a zone the time zone database has dropped, disables its schedule
with outcome `error`.

Batches of two schedules due at once share the worker pool and
`max_concurrent_jobs`, so the second waits behind the first and its
snapshot's runs start later than its scheduled time. Different times keep
them apart.

Several servers MAY share one database: the compare-and-set update lets
exactly one of them fire each occurrence. An occurrence claimed when the
server stops is not retried. A stop before submission records `error`.

The `scheduler_enabled` runtime setting stops all firing without changing
any schedule; see
[configuration.md](../server/configuration.md#scheduler-settings).

### Admin UI, API and Client

The cohort row shows the schedule in its **Schedule** column: the rule, the
next run in the browser's time zone, and the last outcome. The column, and
**Schedule...** in the **Run new snapshot** menu, open the schedule editor,
which previews the next occurrences as the rule is edited.

| Method | Path | Result |
|---|---|---|
| `GET` | `/api/v1/analysis/cohorts/{id}/schedule` | The schedule; `404 no_schedule` without one. |
| `PUT` | `/api/v1/analysis/cohorts/{id}/schedule` | Creates or replaces the schedule; keeps its last run. |
| `DELETE` | `/api/v1/analysis/cohorts/{id}/schedule` | `204`. |
| `GET` | `/api/v1/analysis/schedules` | Every schedule, with `source_tag` and `label`. |
| `POST` | `/api/v1/analysis/schedules/preview` | The next five occurrences of the rule in the body. |

`PUT` answers `400 invalid_schedule` for an invalid rule and
`409 cohort_analysis_disabled` for a cohort whose analysis is disabled.
Every route answers `503 analysis_unavailable` without the analysis
controller.

The TLD cohort on the 1st and the 15th at 02:00 Stockholm time:

```bash
curl -s -X PUT http://localhost:8080/api/v1/analysis/cohorts/1/schedule \
  -H "Content-Type: application/json" \
  -d '{"kind": "monthly", "days_of_month": [1, 15], "time_of_day": "02:00", "timezone": "Europe/Stockholm"}'
```

```sh
gonemaster-client cohorts schedule set tld --monthly 1,15 --at 02:00 --tz Europe/Stockholm
```

See [../client/cohorts.md](../client/cohorts.md) for the client commands.

## Snapshots

Public cohorts resolve to snapshots. Without a `snapshot` query parameter, the
cohort uses its default snapshot policy, usually the latest captured public
snapshot.

Snapshots keep public URLs stable and keep ad-hoc retests out of the public
cohort series unless the batch was explicitly marked as snapshot-intent.

See [snapshots.md](snapshots.md).
