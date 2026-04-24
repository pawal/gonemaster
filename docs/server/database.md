# Server Database

This page owns database selection and operational storage guidance for
`gonemaster-server`.

## Backends

| Backend | Use when |
|---|---|
| `memory` | Developing, testing, or running short-lived scripts. |
| `sqlite` | Running one server with small or medium data sets. |
| `postgres` | Running high-volume analysis or querying JSON arguments heavily. |
| `mariadb` | Reusing existing MariaDB or MySQL infrastructure. |

## DSNs

SQLite:

```sh
gonemaster-server \
  --db-driver sqlite \
  --db-dsn /var/lib/gonemaster/gonemaster.db
```

PostgreSQL:

```sh
GONEMASTER_DB_DRIVER=postgres \
GONEMASTER_DB_DSN="postgres://user:pass@host:5432/dbname?sslmode=disable" \
  gonemaster-server
```

MariaDB or MySQL:

```sh
GONEMASTER_DB_DRIVER=mariadb \
GONEMASTER_DB_DSN="user:pass@tcp(host:3306)/dbname" \
  gonemaster-server
```

`parseTime=true` is appended automatically for MariaDB/MySQL if it is not
already present.

## Stored Data

The server stores:

- queued and running jobs
- completed runs and log entries
- domain and tag registry data
- batch metadata
- stored profiles
- analysis cohort materialization and snapshots

Retention removes old terminal jobs and their stored results. Domain and tag
records are shared metadata and are not owned by one batch.

## Connection Pools

| Backend | Max open connections | Max idle connections | Connection lifetime |
|---|---:|---:|---|
| `sqlite` | 1 | default | default |
| `postgres` | 25 | 5 | 5 minutes |
| `mariadb` | 25 | 5 | 5 minutes |

SQLite is serialized to avoid concurrent writer issues. PostgreSQL and MariaDB
use a small pool suitable for the server worker model.

## Startup Recovery

With a persistent backend, startup performs three recovery steps:

1. Run pending schema migrations.
2. Mark jobs that were `running` during the last shutdown as `failed`.
3. Re-enqueue jobs that were still `queued`.

This keeps the queue consistent after a restart without rewriting completed
runs.

## Retention

Configure automatic retention with `--db-retention-days` or
`database.retention_days`:

```sh
gonemaster-server \
  --db-driver sqlite \
  --db-dsn /var/lib/gonemaster/gonemaster.db \
  --db-retention-days 90
```

- `0` keeps completed results forever.
- A positive value starts an hourly purge loop.
- Only terminal jobs are purged.
- Queued, running, and paused jobs are never purged automatically.

Manual batch deletion is separate from retention. It removes one batch and its
derived records regardless of age. See [operations.md](operations.md).

## Related Pages

- Full backend setup: [../database-setup.md](../database-setup.md)
- Analysis querying: [../analysis/querying.md](../analysis/querying.md)
- Batch deletion behavior: [operations.md](operations.md)
