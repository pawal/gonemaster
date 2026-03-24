# Database Setup Guide

This guide covers choosing and configuring a storage backend for `gonemaster-server`,
including setup, recommended settings, tuning, and backup procedures.
For DSN formats and connection pool defaults, see
[server.md — Database](server.md#database).

---

## Choosing a backend

| Backend | Use when |
|---|---|
| `memory` | Development, one-off tests, or ephemeral CI environments |
| `sqlite` | Single-server production; simple operations; no external database required |
| `postgres` | Multiple server instances; high job volume; existing PostgreSQL infrastructure |
| `mariadb` | Existing MariaDB or MySQL infrastructure |

If in doubt: **use SQLite** for a single server. It requires no external database,
handles the gonemaster workload well, and can be switched to PostgreSQL or MariaDB later
by pointing `--db-dsn` at the new server after a data migration (or a clean start).

---

## Memory (default)

```
gonemaster-server
```

The default backend. All job data is stored in RAM and lost when the server stops.
The purge loop works with this backend — set `--db-retention-days` if you run the server
long-term to prevent unbounded memory growth.

**Recommended settings:**

| Setting | Recommended value | Notes |
|---|---|---|
| `--db-retention-days` | `1`–`7` for long-running services | Prevents unbounded RAM growth; omit for short-lived processes |

**When to use:**
- Development and local testing.
- One-off batch runs where you only care about the current session.
- CI environments where results are consumed before the process exits.

**Limitations:**
- No persistence across restarts. All job data is lost when the server stops.
- Memory grows unbounded if many jobs accumulate without a retention policy; set `--db-retention-days` for long-running instances.

---

## SQLite

```
gonemaster-server \
  --db-driver sqlite \
  --db-dsn /var/lib/gonemaster/gonemaster.db \
  --db-retention-days 90
```

Embedded database with no external dependency. The schema is created automatically on
first start. A background purge loop runs hourly when `--db-retention-days` is set.

**Recommended settings:**

| Setting | Recommended value | Notes |
|---|---|---|
| `--db-retention-days` | `90` | Keeps disk use bounded; tune to your audit requirements |

**When to use:**
- Single-server production deployments.
- Environments where running a separate database server is impractical.
- Monitoring or NOC tooling that needs history across server restarts.

**Limitations:**
- Write-serialised: the connection pool is limited to one open connection.
  Concurrent writes queue up, which is fine for gonemaster's workload but would bottleneck
  extremely high batch throughput (thousands of jobs/minute).
- Not suitable for multiple gonemaster-server instances sharing a single store.

**Config file example:**

```json
{
  "database": {
    "driver": "sqlite",
    "dsn": "/var/lib/gonemaster/gonemaster.db",
    "retention_days": 90
  }
}
```

**Directory setup:**

```bash
mkdir -p /var/lib/gonemaster
chown gonemaster:gonemaster /var/lib/gonemaster
chmod 750 /var/lib/gonemaster
```

**Backup:**

```bash
# Safe online copy (SQLite's backup API handles live writes)
sqlite3 /var/lib/gonemaster/gonemaster.db \
  ".backup /var/backups/gonemaster_$(date +%Y%m%d).db"
```

---

## PostgreSQL

**When to use:**
- Multiple gonemaster-server instances sharing one database.
- High job volume (hundreds of jobs per minute).
- When you already operate a PostgreSQL cluster.

**Recommended settings:**

| Setting | Recommended value | Notes |
|---|---|---|
| `--db-retention-days` | `90` | Rows accumulate quickly; regular purging prevents table bloat |
| `sslmode` | `require` or `verify-full` | Never use `disable` in production |
| `max_connections` in postgresql.conf | `≥ 30` | gonemaster pool uses 25 max by default |

**Connection pool defaults:** max 25 open, 5 idle, 5-minute lifetime.
These work well for single-instance deployments. If running multiple server processes,
lower `max_connections` per instance or increase the PostgreSQL `max_connections` accordingly.

### Quick start

```
gonemaster-server \
  --db-driver postgres \
  --db-dsn "postgres://gonemaster:pass@host:5432/gonemaster?sslmode=require" \
  --db-retention-days 90
```

Use environment variables to keep credentials out of process listings and shell history:

```bash
export GONEMASTER_DB_DRIVER=postgres
export GONEMASTER_DB_DSN="postgres://gonemaster:pass@host:5432/gonemaster?sslmode=require"
export GONEMASTER_DB_RETENTION_DAYS=90
gonemaster-server
```

**Config file example:**

```json
{
  "database": {
    "driver": "postgres",
    "dsn": "postgres://gonemaster:pass@host:5432/gonemaster?sslmode=require",
    "retention_days": 90
  }
}
```

### Create database and user

```sql
CREATE DATABASE gonemaster
    ENCODING 'UTF8'
    LC_COLLATE 'en_US.UTF-8'
    LC_CTYPE 'en_US.UTF-8'
    TEMPLATE template0;

CREATE USER gonemaster WITH PASSWORD 'strongpassword';

GRANT CONNECT ON DATABASE gonemaster TO gonemaster;
\c gonemaster
GRANT USAGE  ON SCHEMA public TO gonemaster;
GRANT CREATE ON SCHEMA public TO gonemaster;
```

`gonemaster-server` creates and manages its own tables via schema migrations on
first start. The user only needs `CONNECT`, `USAGE`, and `CREATE` — no
superuser privileges are required.

### pg_hba.conf

Add a line for the gonemaster user (adjust `host` or `local` as appropriate):

```
# TYPE  DATABASE    USER        ADDRESS         METHOD
host    gonemaster  gonemaster  127.0.0.1/32    scram-sha-256
```

Reload after editing:
```
pg_ctlcluster <version> main reload
# or:
sudo systemctl reload postgresql
```

### postgresql.conf tuning

The gonemaster workload is write-heavy (many short jobs) with moderate reads.
Suggested starting values for a dedicated or shared small server:

```ini
# Memory
shared_buffers = 256MB          # 25% of RAM is a good starting point
work_mem = 4MB                  # per sort/hash operation; raise if List() is slow
maintenance_work_mem = 64MB     # for VACUUM, index builds

# Connections — keep below max_connections to leave headroom for admin tools
max_connections = 50            # gonemaster pool uses 25 max; leave room for psql

# WAL / checkpoint
checkpoint_completion_target = 0.9
wal_buffers = 16MB

# Autovacuum — gonemaster purges rows frequently; keep autovacuum responsive
autovacuum_vacuum_scale_factor = 0.05   # vacuum sooner on busy tables
autovacuum_analyze_scale_factor = 0.02
```

### SSL/TLS

To require TLS, set `ssl = on` in `postgresql.conf` and provide `ssl_cert_file`
and `ssl_key_file`. Update the DSN:

```
postgres://gonemaster:pass@host:5432/gonemaster?sslmode=require
```

For self-signed certificates or internal CAs:
```
postgres://gonemaster:pass@host:5432/gonemaster?sslmode=verify-ca&sslrootcert=/etc/ssl/certs/ca.crt
```

Common `sslmode` values:
| Value | Behaviour |
|---|---|
| `disable` | No TLS (development only) |
| `require` | TLS required, certificate not verified |
| `verify-ca` | TLS + CA verification |
| `verify-full` | TLS + CA + hostname verification (recommended in production) |

### Backup

```bash
# Logical dump (small databases, simple restore)
pg_dump -U gonemaster -F custom gonemaster > gonemaster_$(date +%Y%m%d).dump

# Restore
pg_restore -U gonemaster -d gonemaster gonemaster_20260101.dump
```

For larger deployments consider WAL archiving (`archive_mode = on`) or a
streaming replica for point-in-time recovery.

### Autovacuum and the purge workload

When data retention / purge is enabled (`database.retention_days > 0`),
`gonemaster-server` periodically deletes old rows from `runs` and `entries`.
PostgreSQL's autovacuum must reclaim the dead tuples promptly to prevent table
bloat. The `autovacuum_vacuum_scale_factor = 0.05` value above triggers a
vacuum once 5% of a table's rows are dead, which is appropriate for tables that
see frequent bulk deletes.

Run `VACUUM ANALYZE runs; VACUUM ANALYZE entries;` manually after the first
large purge to update planner statistics.

---

## MariaDB

**When to use:**
- When your infrastructure already runs MariaDB or MySQL.
- Multi-server deployments on an existing managed MySQL service.

**Recommended settings:**

| Setting | Recommended value | Notes |
|---|---|---|
| `--db-retention-days` | `90` | Keeps the `runs` and `entries` tables from growing indefinitely |
| `tls=true` or `tls=skip-verify` | production / internal CA | Protects credentials in transit |
| `innodb_file_per_table` | `ON` | Allows disk reclamation after large purges |

**Connection pool defaults:** max 25 open, 5 idle, 5-minute lifetime.

### Quick start

```
gonemaster-server \
  --db-driver mariadb \
  --db-dsn "gonemaster:pass@tcp(host:3306)/gonemaster?tls=true" \
  --db-retention-days 90
```

`parseTime=true` is appended to the DSN automatically if not already present.

Use environment variables to keep credentials out of process listings and shell history:

```bash
export GONEMASTER_DB_DRIVER=mariadb
export GONEMASTER_DB_DSN="gonemaster:pass@tcp(host:3306)/gonemaster"
export GONEMASTER_DB_RETENTION_DAYS=90
gonemaster-server
```

**Config file example:**

```json
{
  "database": {
    "driver": "mariadb",
    "dsn": "gonemaster:pass@tcp(host:3306)/gonemaster",
    "retention_days": 90
  }
}
```

### Create database and user

```sql
CREATE DATABASE gonemaster
    CHARACTER SET utf8mb4
    COLLATE utf8mb4_unicode_ci;

CREATE USER 'gonemaster'@'localhost' IDENTIFIED BY 'strongpassword';

GRANT SELECT, INSERT, UPDATE, DELETE, CREATE, DROP, INDEX, ALTER
    ON gonemaster.*
    TO 'gonemaster'@'localhost';

FLUSH PRIVILEGES;
```

Replace `'localhost'` with the application host if connecting over the network.

### my.cnf tuning

```ini
[mysqld]
# InnoDB — use InnoDB for all tables (default in MariaDB 10.x+)
default_storage_engine = InnoDB

# Buffer pool — set to 50-70% of RAM on a dedicated database server
innodb_buffer_pool_size = 512M

# Redo log — larger = fewer checkpoints, better write throughput
innodb_log_file_size = 128M

# One file per table — essential for reclaiming disk space after purge
innodb_file_per_table = ON

# Connections — gonemaster pool uses 25 max; leave headroom for admin tools
max_connections = 50

# Character set defaults — must match the database collation
character_set_server = utf8mb4
collation_server = utf8mb4_unicode_ci
```

### DSN parameters

The minimum required DSN:
```
gonemaster:pass@tcp(host:3306)/gonemaster
```

`parseTime=true` is added automatically by gonemaster-server if not present.

For full control, or to set charset and timezone explicitly:
```
gonemaster:pass@tcp(host:3306)/gonemaster?charset=utf8mb4&loc=UTC&parseTime=true
```

| Parameter | Required | Notes |
|---|---|---|
| `parseTime=true` | Yes (auto-added) | Required for time.Time scanning |
| `charset=utf8mb4` | Recommended | Match the database collation |
| `loc=UTC` | Recommended | Ensures consistent timestamp handling |
| `tls=custom` | Production | See SSL section below |

### SSL/TLS

Register a TLS config in your application, or use the DSN `tls=` parameter:

```
gonemaster:pass@tcp(host:3306)/gonemaster?tls=true
```

For a custom CA certificate, register a named TLS config before opening the
connection. With gonemaster-server, set the full DSN including `tls=`:

```
gonemaster:pass@tcp(host:3306)/gonemaster?tls=skip-verify
```

See the [go-sql-driver/mysql TLS documentation](https://github.com/go-sql-driver/mysql#tls)
for registering a custom CA.

### Backup

```bash
# Logical dump
mysqldump -u gonemaster -p --single-transaction gonemaster > gonemaster_$(date +%Y%m%d).sql

# Restore
mysql -u gonemaster -p gonemaster < gonemaster_20260101.sql
```

For larger deployments consider `mariabackup` (physical backup) or a replica
for point-in-time recovery.

### Disk reclamation after purge

With `innodb_file_per_table = ON`, each table has its own `.ibd` file. After a
large purge, reclaim space with:

```sql
OPTIMIZE TABLE runs;
OPTIMIZE TABLE entries;
```

This rebuilds the table and releases space back to the OS.

---

## Data retention

All backends — including the default in-memory backend — support automatic purging of old
completed jobs. Configure it with `--db-retention-days`:

```
gonemaster-server --db-driver sqlite --db-dsn /var/lib/gonemaster/gonemaster.db \
  --db-retention-days 90
```

- The purge loop runs **hourly** in the background.
- Only terminal-status jobs (`succeeded`, `failed`, `canceled`, `expired`) are deleted.
  Running, queued, and paused jobs are never purged automatically.
- Associated runs and entries are also deleted in the same operation.
- **Recommended production value:** `90` days.
- `0` (default) disables automatic purging; data accumulates indefinitely.

You can also trigger a one-off purge via the API:

```bash
curl -s -X POST http://localhost:8080/api/v1/jobs/purge \
  -H 'Content-Type: application/json' \
  -d '{"older_than_days": 30}'
```

Or via the client:

```bash
gonemaster-client jobs purge --older-than 30
```

---

## Switching backends

`gonemaster-server` does not migrate data between backends. To switch:

1. Let running jobs finish (or cancel them).
2. Stop the server.
3. Start the server with the new `--db-driver` and `--db-dsn`.

The new backend will start empty. If you need to keep historical data, export
results before switching (for example with `gonemaster-client runs list --batch <id>` or
`gonemaster-client entries query --tag <name> --format csv`).

---

## Docker Compose (development)

The repository includes [docker-compose.test.yml](../docker-compose.test.yml)
for spinning up PostgreSQL and MariaDB locally:

```bash
docker compose -f docker-compose.test.yml up -d --wait
```

DSNs for local use:
```
# PostgreSQL
postgres://gonemaster:gonemaster@localhost:5432/gonemaster_test?sslmode=disable

# MariaDB
gonemaster:gonemaster@tcp(localhost:3306)/gonemaster_test
```

To run integration tests against both backends:
```bash
make test-integration
```

---

## Monitoring

Key metrics to watch in production:

| Metric | PostgreSQL | MariaDB |
|---|---|---|
| Active connections | `pg_stat_activity` | `SHOW STATUS LIKE 'Threads_connected'` |
| Table sizes | `pg_total_relation_size('runs')`, `pg_total_relation_size('entries')` | `information_schema.tables` |
| Cache hit rate | `pg_statio_user_tables` | `Innodb_buffer_pool_read_requests` |
| Slow queries | `pg_stat_statements` | `slow_query_log = ON` |
| Dead tuples / bloat | `pg_stat_user_tables.n_dead_tup` | `SHOW ENGINE INNODB STATUS` |
