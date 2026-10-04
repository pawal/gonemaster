# Client Cohorts

The `cohorts` group lists the analysis cohorts and their snapshots, and
manages each cohort's snapshot schedule.

```text
gonemaster-client cohorts list
gonemaster-client cohorts snapshots [DATASET-TAG]
gonemaster-client cohorts schedules
gonemaster-client cohorts schedule TAG
gonemaster-client cohorts schedule set TAG [flags]
gonemaster-client cohorts schedule remove TAG
```

`list` and `snapshots` read the public analysis API. The schedule commands
use the admin API and need an admin token in token mode.

## Cohorts and Snapshots

```sh
gonemaster-client cohorts list
gonemaster-client cohorts snapshots kommuner
```

`cohorts list` names every public cohort with its label, its snapshot count,
and which one is the default. `cohorts snapshots` gives one cohort's slugs
with the capture date and the domain count, newest first. Its dataset tag is
optional and resolves as in [report.md](report.md): the server's default
public cohort when omitted.

A snapshot is captured per batch, so comparing two batches of a cohort is the
same call as comparing their snapshots.

## Schedules

A schedule submits a cohort's snapshot run on a recurrence. The model, the
outcomes and the daylight saving rules are in
[../analysis/cohorts.md](../analysis/cohorts.md#scheduling). The schedule
commands name a cohort by its source tag.

`cohorts schedules` lists every schedule: tag, state, rule, next run and the
last outcome. `cohorts schedule TAG` shows one. With `--format json` both
print the API objects.

`cohorts schedule set TAG` creates or changes a schedule. It reads the
current schedule first, so each flag changes one field and the others keep
their value. A new schedule starts enabled, in UTC, with catch-up on and the
tag default profile.

| Flag | Effect |
|---|---|
| `--monthly LIST` | Monthly on days 1 to 28; `last` adds the last day of the month. |
| `--weekly LIST` | Weekly on `mon`, `tue`, `wed`, `thu`, `fri`, `sat`, `sun`. |
| `--every N` | Every N days, 1 to 365. |
| `--from DATE` | First date of an `--every` rule, `YYYY-MM-DD`. Default today. |
| `--at HH:MM` | Time of day in the schedule's time zone. |
| `--tz ZONE` | IANA time zone name. |
| `--profile NAME` | Stored profile by name or id; `--profile=` restores the tag default. |
| `--promote-default`, `--no-promote-default` | Pin, or do not pin, each captured snapshot as cohort default. |
| `--catch-up`, `--no-catch-up` | Run, or skip, an occurrence missed during downtime. |
| `--enable`, `--disable` | Resume or pause the schedule. |

At most one of `--monthly`, `--weekly` and `--every` is accepted. The server
validates the result; an invalid rule exits with code 2 and its message.

```sh
gonemaster-client cohorts schedule set tld --monthly 1,15 --at 02:00 --tz Europe/Stockholm
gonemaster-client cohorts schedule set gov --monthly last --at 02:00
gonemaster-client cohorts schedule set kommuner --weekly mon,thu --at 23:30
gonemaster-client cohorts schedule set kommuner --every 3 --from 2026-11-01 --at 02:00 \
    --profile strict --promote-default --no-catch-up
gonemaster-client cohorts schedule set tld --disable
```

`cohorts schedule remove TAG` deletes the schedule.
