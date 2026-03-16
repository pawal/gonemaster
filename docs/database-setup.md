# Database Setup Guide

This guide covers setting up PostgreSQL and MariaDB for production use with
`gonemaster-server`. For DSN formats and connection pool defaults, see
[server.md — Database](server.md#database).

---

## PostgreSQL

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
`gonemaster-server` periodically deletes old rows from `jobs` and `results`.
PostgreSQL's autovacuum must reclaim the dead tuples promptly to prevent table
bloat. The `autovacuum_vacuum_scale_factor = 0.05` value above triggers a
vacuum once 5% of a table's rows are dead, which is appropriate for tables that
see frequent bulk deletes.

Run `VACUUM ANALYZE jobs; VACUUM ANALYZE results;` manually after the first
large purge to update planner statistics.

---

## MariaDB

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
OPTIMIZE TABLE jobs;
OPTIMIZE TABLE results;
```

This rebuilds the table and releases space back to the OS.

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
| Table sizes | `pg_total_relation_size('jobs')` | `information_schema.tables` |
| Cache hit rate | `pg_statio_user_tables` | `Innodb_buffer_pool_read_requests` |
| Slow queries | `pg_stat_statements` | `slow_query_log = ON` |
| Dead tuples / bloat | `pg_stat_user_tables.n_dead_tup` | `SHOW ENGINE INNODB STATUS` |
