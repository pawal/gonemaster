# Data Analysis with Gonemaster

This guide covers using gonemaster as a bulk DNS analysis platform - running large domain
sets, tracking results over time, and querying the data through the API or directly via SQL.

---

## Choosing a database backend

| Backend | Best for |
|---|---|
| `memory` | Development, single-run scripts that consume results before exit |
| `sqlite` | Single-server, small-to-medium domain sets (up to ~1 M entries) |
| `postgres` | Multi-server, high volume, or workloads using `args_json` filtering |
| `mariadb` | Existing MariaDB/MySQL infrastructure |

**SQLite** is the easiest starting point. It requires no external service and performs
well for most analysis workloads. `args_json` queries require loading JSON in application
code or using SQLite's `json_extract()`.

**PostgreSQL** is recommended when you need to query inside `args_json` efficiently.
The schema stores `args_json` as `jsonb` on PostgreSQL, enabling GIN-indexed arbitrary
key lookups without application-side JSON parsing.

See [database-setup.md](database-setup.md) for connection strings, tuning, and backup
procedures.

---

## Server configuration for analysis workloads

For bulk batch runs, increase workers and the concurrent job limit:

```
gonemaster-server \
  --db-driver sqlite \
  --db-dsn /var/lib/gonemaster/gonemaster.db \
  --db-retention-days 180 \
  --workers 24 \
  --max-concurrent-jobs 24
```

Or via a JSON config file (`--config gonemaster.json`):

```json
{
  "database": {
    "driver": "sqlite",
    "dsn": "/var/lib/gonemaster/gonemaster.db",
    "retention_days": 180
  },
  "worker_count": 24,
  "max_concurrent_jobs": 24
}
```

**Key settings:**

| Setting | Notes |
|---|---|
| `--workers` / `worker_count` | Goroutines that pick up jobs. 24 is a good baseline on an 8-core host. |
| `--max-concurrent-jobs` | Cap on parallel engine executions. Match to `--workers`. |
| `--db-retention-days` | Automatic purge of runs and entries older than N days. 0 keeps everything. |

---

## Creating and managing tags

Tags are named domain collections. A domain can belong to multiple tags.
Tags are the primary analysis dimension - most API filters and CLI commands accept `--tag`.

**Create a tag:**
```
gonemaster-client tags create tld --description "All IANA top-level domains"
```

**List all tags:**
```
gonemaster-client tags list
```

**Show severity distribution for a tag:**
```
gonemaster-client tags summary tld
```

Tags are also created implicitly when first used in a batch submission.

---

## Importing domain lists

**From a file (one domain per line):**
```
gonemaster-client tags add-domains tld --file tlds.txt
```

**From stdin:**
```
cat tlds.txt | gonemaster-client tags add-domains tld --stdin
```

**Inline:**
```
gonemaster-client tags add-domains municipalities-se \
  stockholm.se malmo.se goteborg.se
```

The `add-domains` command creates domain records if they do not exist yet, then
adds them to the tag. Domains that already exist are linked without duplication.

---

## Running a tagged batch

**Submit a batch for all domains in a file and tag them:**
```
gonemaster-client jobs batch \
  --file domains.txt \
  --tag tld \
  --wait
```

**Re-run all domains already in a tag:**
```
gonemaster-client jobs batch \
  --from-tag tld \
  --tag tld \
  --wait
```

`--from-tag` enqueues one job per domain currently in the tag.
`--tag` links the resulting runs back to the same tag.

**Monitor a batch without blocking:**
```
BATCH=$(gonemaster-client jobs batch --from-tag tld --tag tld | jq -r .id)
gonemaster-client batches watch $BATCH
```

---

## Re-running a tag to update results

Re-running is the same as the initial run - use `--from-tag`:

```
gonemaster-client jobs batch --from-tag tld --tag tld --wait
```

After completion, `domains.latest_*` is updated for each domain, and new
`runs` + `entries` rows are written. Old runs are preserved until the
retention window expires.

---

## Querying results via the API

All analysis endpoints require admin API access (`/api/v1/`).

### Per-tag severity summary

```
GET /api/v1/tags/tld/summary
```

Returns counts of domains at each worst level (OK, NOTICE, WARNING, ERROR, CRITICAL).

```
gonemaster-client tags summary tld
```

### Domain list with filtering

```
GET /api/v1/domains?tag=tld&level=ERROR&limit=100
```

Returns domains whose latest run reached at least ERROR.

```
gonemaster-client domains list --tag tld --level ERROR
```

### Run history for a domain

```
GET /api/v1/runs?domain=example.com&limit=20
```

```
gonemaster-client domains runs example.com
```

### Entry-level queries

```
GET /api/v1/entries?tag=tld&module=DNSSEC&latest=true&limit=500
```

Filters log entries across all latest runs for domains in `tld` where the module
is `DNSSEC`. Useful for pinpointing specific failures at scale.

```
gonemaster-client entries query --tag tld --module DNSSEC --latest
```

**Common entry filters:**

| Parameter | Description |
|---|---|
| `tag` | Filter to domains in this tag |
| `module` | Engine module (e.g. `DNSSEC`, `NAMESERVER`, `ZONE`) |
| `testcase` | Specific test (e.g. `DNSSEC02`) |
| `entry_tag` | Log event tag (e.g. `DS_ALGO_NOT_SUPPORTED`) |
| `level` | Minimum severity (`NOTICE`, `WARNING`, `ERROR`, `CRITICAL`) |
| `latest` | Only entries from each domain's most recent run |

---

## Querying results via SQL

For complex analysis, query the database directly.

### SQLite

```
sqlite3 /var/lib/gonemaster/gonemaster.db
```

### PostgreSQL

```
psql "$GONEMASTER_DB_DSN"
```

### Schema overview

```
domains      – one row per domain; latest_* columns are denormalized fast-path
tags         – named collections; name is the primary key
domain_tags  – many-to-many: domain_id ↔ tag
runs         – one row per completed test execution (immutable after creation)
entries      – one row per engine log entry; the primary analysis table
batches      – batch metadata
```

---

## Example: DNSSEC compliance across all TLDs

Domains in the `tld` tag whose latest run has any DNSSEC error or critical entry:

```sql
SELECT DISTINCT d.name, d.latest_level
FROM domains d
JOIN domain_tags dt ON dt.domain_id = d.id AND dt.tag = 'tld'
JOIN runs r ON r.id = d.latest_run_id
JOIN entries e ON e.run_id = r.id
WHERE e.module = 'DNSSEC'
  AND e.level IN ('ERROR', 'CRITICAL')
ORDER BY d.name;
```

Count by specific DNSSEC test case tag:

```sql
SELECT e.tag, COUNT(DISTINCT e.domain_id) AS affected_domains
FROM entries e
JOIN runs r ON r.id = e.run_id
JOIN domain_tags dt ON dt.domain_id = e.domain_id AND dt.tag = 'tld'
WHERE e.module = 'DNSSEC'
  AND e.level IN ('ERROR', 'CRITICAL')
  AND r.id = (
    SELECT id FROM runs r2
    WHERE r2.domain_id = e.domain_id
    ORDER BY r2.finished_at DESC
    LIMIT 1
  )
GROUP BY e.tag
ORDER BY affected_domains DESC;
```

---

## Example: Severity distribution for a tagged group

Quick summary using the `runs` severity counters (no join to `entries` needed):

```sql
SELECT
  worst_level,
  COUNT(*) AS domain_count
FROM domains d
JOIN domain_tags dt ON dt.domain_id = d.id AND dt.tag = 'municipalities-se'
JOIN runs r ON r.id = d.latest_run_id
GROUP BY worst_level
ORDER BY CASE worst_level
  WHEN 'CRITICAL' THEN 0 WHEN 'ERROR' THEN 1
  WHEN 'WARNING'  THEN 2 WHEN 'NOTICE' THEN 3
  ELSE 4 END;
```

---

## Example: Trend analysis - weekly comparison of results

Compare DNSSEC error rate between two time windows:

```sql
WITH week1 AS (
  SELECT DISTINCT r.domain_id
  FROM runs r
  JOIN entries e ON e.run_id = r.id
  JOIN domain_tags dt ON dt.domain_id = r.domain_id AND dt.tag = 'tld'
  WHERE r.finished_at >= '2026-03-01' AND r.finished_at < '2026-03-08'
    AND e.module = 'DNSSEC' AND e.level IN ('ERROR', 'CRITICAL')
),
week2 AS (
  SELECT DISTINCT r.domain_id
  FROM runs r
  JOIN entries e ON e.run_id = r.id
  JOIN domain_tags dt ON dt.domain_id = r.domain_id AND dt.tag = 'tld'
  WHERE r.finished_at >= '2026-03-08' AND r.finished_at < '2026-03-15'
    AND e.module = 'DNSSEC' AND e.level IN ('ERROR', 'CRITICAL')
)
SELECT
  (SELECT COUNT(*) FROM week1) AS week1_failing,
  (SELECT COUNT(*) FROM week2) AS week2_failing,
  (SELECT COUNT(*) FROM week2) - (SELECT COUNT(*) FROM week1) AS delta;
```

---

## Example: Finding domains that got worse between runs

Domains where the worst level increased between two consecutive runs:

```sql
WITH ranked AS (
  SELECT
    domain_id,
    domain,
    worst_level,
    finished_at,
    LAG(worst_level) OVER (PARTITION BY domain_id ORDER BY finished_at) AS prev_level
  FROM runs
  WHERE domain_id IN (
    SELECT domain_id FROM domain_tags WHERE tag = 'tld'
  )
)
SELECT domain, prev_level, worst_level, finished_at
FROM ranked
WHERE prev_level IS NOT NULL
  AND CASE worst_level
    WHEN 'CRITICAL' THEN 4 WHEN 'ERROR' THEN 3
    WHEN 'WARNING'  THEN 2 WHEN 'NOTICE' THEN 1 ELSE 0 END
  > CASE prev_level
    WHEN 'CRITICAL' THEN 4 WHEN 'ERROR' THEN 3
    WHEN 'WARNING'  THEN 2 WHEN 'NOTICE' THEN 1 ELSE 0 END
ORDER BY finished_at DESC;
```

---

## Example: Per-testcase failure rates

Fraction of domains (in the `tld` tag) failing each testcase in their latest run:

```sql
SELECT
  e.testcase,
  COUNT(DISTINCT e.domain_id) AS failing,
  ROUND(100.0 * COUNT(DISTINCT e.domain_id) /
    (SELECT COUNT(DISTINCT domain_id) FROM domain_tags WHERE tag = 'tld'), 1)
    AS pct
FROM entries e
JOIN domain_tags dt ON dt.domain_id = e.domain_id AND dt.tag = 'tld'
WHERE e.level IN ('ERROR', 'CRITICAL')
  AND e.run_id IN (
    SELECT latest_run_id FROM domains
    WHERE id IN (SELECT domain_id FROM domain_tags WHERE tag = 'tld')
  )
GROUP BY e.testcase
ORDER BY failing DESC;
```

---

## Example: CSV export for spreadsheet or pandas analysis

**Via the client (recommended):**
```
gonemaster-client --format csv entries query \
  --tag tld --module DNSSEC --latest > dnssec-entries.csv
```

**Via curl:**
```
curl -s "http://localhost:8080/api/v1/entries?tag=tld&module=DNSSEC&latest=true&format=csv" \
  > dnssec-entries.csv
```

The CSV includes: `domain`, `run_id`, `module`, `testcase`, `tag`, `level`, `args_json`.

**Load into pandas:**
```python
import pandas as pd
df = pd.read_csv("dnssec-entries.csv")
print(df.groupby("tag")["domain"].nunique().sort_values(ascending=False))
```

---

## Deployment note: Separating public UI and analysis instances

For production deployments it is common to run two separate `gonemaster-server`
instances pointing at the same database:

- **Public instance** - exposed via reverse proxy; serves `/public/` and
  `/pub/api/v1/` only; the admin API is blocked at the proxy layer.
- **Analysis instance** - internal network only; serves `/api/v1/` with full
  admin access for batch runs, tag management, and data queries.

Both instances share the same SQLite file (read-heavy workloads only) or the
same PostgreSQL/MariaDB database. The public instance should be given a
read-only database user if your backend supports it, to prevent accidental
writes from public API traffic.

Example nginx split:

```nginx
# Public UI and public API only
location /public/   { proxy_pass http://gonemaster-public/public/; }
location /pub/      { proxy_pass http://gonemaster-public/pub/; }

# Block admin API from the internet
location /api/      { return 403; }
```
