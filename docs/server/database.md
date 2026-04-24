# Server Database

This page owns database selection and operational storage guidance for
`gonemaster-server`. The detailed setup notes remain in
[../database-setup.md](../database-setup.md) during the documentation split.

## Backends

| Backend | Use when |
|---|---|
| `memory` | Developing, testing, or running short-lived scripts. |
| `sqlite` | Running one server with small or medium data sets. |
| `postgres` | Running high-volume analysis or querying JSON arguments heavily. |
| `mariadb` | Reusing existing MariaDB or MySQL infrastructure. |

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

## Related Pages

- Full backend setup: [../database-setup.md](../database-setup.md)
- Analysis querying: [../analysis/querying.md](../analysis/querying.md)
- Batch deletion behavior: [operations.md](operations.md)
