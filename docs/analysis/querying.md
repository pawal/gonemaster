# Querying Analysis Data

Gonemaster can be used as a bulk DNS analysis platform: run large domain sets,
track results over time, and query the data through `gonemaster-client`, the
admin API, SQL, or an MCP-capable agent.

## Choose an Access Path

| Path | Best for |
|---|---|
| `gonemaster-client` | Scripted exports, CSV pipelines. |
| Admin API (`/api/v1/`) | Programmatic integration. |
| SQL (sqlite / postgres / mariadb) | Ad-hoc joins, JSON arg lookups, large result sets. |
| MCP (`gonemaster-mcp`) | Agent-driven exploration where the next question depends on the previous answer. See [../mcp/analysis-examples.md](../mcp/analysis-examples.md) and the bridge overview in [../mcp/README.md](../mcp/README.md). |

## Choose a Backend

| Backend | Best for |
|---|---|
| `memory` | Development and short-lived scripts. |
| `sqlite` | Single-server analysis with small or medium data sets. |
| `postgres` | High-volume analysis and indexed JSON argument queries. |
| `mariadb` | Existing MariaDB or MySQL infrastructure. |

See [../server/database.md](../server/database.md) for setup and retention.

## Server Settings for Bulk Runs

```sh
gonemaster-server \
  --db-driver sqlite \
  --db-dsn /var/lib/gonemaster/gonemaster.db \
  --db-retention-days 180 \
  --workers 24 \
  --max-concurrent-jobs 24
```

Use PostgreSQL when reports need frequent filtering inside `args_json`.

## Client Queries

```sh
gonemaster-client domains list --tag tld --level ERROR
gonemaster-client tags summary tld
gonemaster-client runs list --tag tld --level WARNING
gonemaster-client entries query --tag tld --module DNSSEC --latest
gonemaster-client batches results batch_123 --view json --per-job
```

Use `--format json` for scripts and `--format csv` for entry exports.

## Admin API Queries

The admin API lives under `/api/v1/` and must stay private.

Common requests:

```bash
curl -s http://localhost:8080/api/v1/tags/tld/summary
curl -s "http://localhost:8080/api/v1/domains?tag=tld&level=ERROR&limit=100"
curl -s "http://localhost:8080/api/v1/runs?domain=example.com&limit=20"
curl -s "http://localhost:8080/api/v1/entries?tag=tld&module=DNSSEC&latest=true&limit=500"
```

Common `entries` filters:

| Parameter | Description |
|---|---|
| `tag` | Domains in this tag. |
| `module` | Engine module, such as `DNSSEC` or `ZONE`. |
| `testcase` | Specific testcase. |
| `entry_tag` | Log event tag, such as `DS_ALGO_NOT_SUPPORTED`. |
| `level` | Severity level. |
| `latest` | Only each domain's latest run. |
| `batch` | Restrict to one batch. |

## SQL Schema Overview

```text
domains      one row per domain, with latest result columns
tags         named collections
domain_tags  many-to-many domain to tag memberships
jobs         queued and running jobs
runs         completed job executions
entries      one row per engine log entry
batches      batch metadata
profiles     stored profile overrides
```

Analysis cohorts add materialized tables under `analysis_*`.

## SQLite

```sh
sqlite3 /var/lib/gonemaster/gonemaster.db
```

SQLite can query JSON with `json_extract()` and related JSON functions, but it
is not ideal for frequent arbitrary `args_json` lookups.

## PostgreSQL

```sh
psql "$GONEMASTER_DB_DSN"
```

PostgreSQL stores `args_json` as `jsonb`, which supports indexed key lookups.

## DNSSEC Compliance Across a Tag

Domains in the `tld` tag whose latest run has DNSSEC errors:

```sql
SELECT d.name, r.id AS run_id, r.finished_at, e.testcase, e.tag, e.level
FROM domains d
JOIN domain_tags dt ON dt.domain_id = d.id AND dt.tag = 'tld'
JOIN runs r ON r.id = d.latest_run_id
JOIN entries e ON e.run_id = r.id
WHERE e.module = 'DNSSEC'
  AND e.level IN ('ERROR', 'CRITICAL')
ORDER BY d.name, e.testcase, e.tag;
```

Count affected domains by DNSSEC finding tag:

```sql
SELECT e.tag, COUNT(DISTINCT e.domain_id) AS affected_domains
FROM entries e
JOIN domains d ON d.id = e.domain_id
JOIN domain_tags dt ON dt.domain_id = d.id AND dt.tag = 'tld'
WHERE e.run_id = d.latest_run_id
  AND e.module = 'DNSSEC'
  AND e.level IN ('ERROR', 'CRITICAL')
GROUP BY e.tag
ORDER BY affected_domains DESC;
```

## Severity Distribution for a Tag

```sql
SELECT
  d.latest_level,
  COUNT(*) AS domains
FROM domains d
JOIN domain_tags dt ON dt.domain_id = d.id
WHERE dt.tag = 'municipalities-se'
GROUP BY d.latest_level
ORDER BY domains DESC;
```

## Weekly Comparison

Latest run per domain before two timestamps:

```sql
WITH week1 AS (
  SELECT DISTINCT ON (r.domain_id)
    r.domain_id, r.worst_level
  FROM runs r
  JOIN domain_tags dt ON dt.domain_id = r.domain_id AND dt.tag = 'tld'
  WHERE r.finished_at < '2026-04-01T00:00:00Z'
  ORDER BY r.domain_id, r.finished_at DESC
),
week2 AS (
  SELECT DISTINCT ON (r.domain_id)
    r.domain_id, r.worst_level
  FROM runs r
  JOIN domain_tags dt ON dt.domain_id = r.domain_id AND dt.tag = 'tld'
  WHERE r.finished_at < '2026-04-08T00:00:00Z'
  ORDER BY r.domain_id, r.finished_at DESC
)
SELECT
  week1.worst_level AS before,
  week2.worst_level AS after,
  COUNT(*) AS domains
FROM week1
JOIN week2 USING (domain_id)
GROUP BY before, after
ORDER BY domains DESC;
```

## Per-Testcase Failure Rates

```sql
SELECT
  e.module,
  e.testcase,
  COUNT(DISTINCT e.domain_id) AS affected_domains,
  ROUND(
    100.0 * COUNT(DISTINCT e.domain_id) /
    NULLIF((SELECT COUNT(DISTINCT domain_id)
            FROM domain_tags
            WHERE tag = 'tld'), 0),
    1
  ) AS affected_percent
FROM entries e
JOIN domains d ON d.id = e.domain_id
JOIN domain_tags dt ON dt.domain_id = e.domain_id AND dt.tag = 'tld'
WHERE e.run_id = d.latest_run_id
  AND e.level IN ('WARNING', 'ERROR', 'CRITICAL')
GROUP BY e.module, e.testcase
ORDER BY affected_percent DESC;
```

## PostgreSQL JSON Argument Queries

Find entries mentioning a specific IP address:

```sql
SELECT d.name, e.module, e.testcase, e.tag, e.level, e.args_json
FROM entries e
JOIN domains d ON d.id = e.domain_id
JOIN domain_tags dt ON dt.domain_id = d.id AND dt.tag = 'tld'
WHERE e.run_id = d.latest_run_id
  AND e.args_json @> '{"address":"192.0.2.10"}'::jsonb;
```

Find entries with a specific nameserver in a structured `servers` array:

```sql
SELECT d.name, e.module, e.testcase, e.tag, e.level
FROM entries e
JOIN domains d ON d.id = e.domain_id
JOIN domain_tags dt ON dt.domain_id = d.id AND dt.tag = 'tld'
WHERE e.run_id = d.latest_run_id
  AND e.args_json @> '{"servers":[{"ns":"ns1.example.com"}]}'::jsonb;
```

Find domains using an address in a tag:

```sql
SELECT DISTINCT d.name
FROM entries e
JOIN domains d ON d.id = e.domain_id
JOIN domain_tags dt ON dt.domain_id = d.id AND dt.tag = 'tld'
WHERE e.run_id = d.latest_run_id
  AND (
    e.args_json @> '{"address":"192.0.2.10"}'::jsonb
    OR e.args_json @> '{"servers":[{"address":"192.0.2.10"}]}'::jsonb
  )
ORDER BY d.name;
```

## CSV Export

```sh
gonemaster-client entries query \
  --tag tld \
  --module DNSSEC \
  --latest \
  --format csv > dnssec-entries.csv
```

Equivalent API request:

```sh
curl -s \
  "http://localhost:8080/api/v1/entries?tag=tld&module=DNSSEC&latest=true&format=csv" \
  > dnssec-entries.csv
```

Example pandas grouping:

```python
import pandas as pd

df = pd.read_csv("dnssec-entries.csv")
print(df.groupby("tag")["domain"].nunique().sort_values(ascending=False))
```

## Public and Analysis Instances

For large public analysis deployments, use two server processes against one
database:

- an internal instance for admin work, tag management, and batch runs
- a public instance that exposes only `/analysis/`, `/public/`, and
  `/pub/api/v1/`

This keeps public traffic away from admin paths and lets the internal instance
handle write-heavy batch workloads.
