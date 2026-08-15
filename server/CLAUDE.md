# server/

## Database Migrations

The SQL schema is versioned in [server/store_sql_migrate.go](server/store_sql_migrate.go). To change the schema, append a new versioned entry to `sqlMigrations` (never edit the v1 consolidated DDL or an already-shipped migration). Keep statements dialect-neutral (`CREATE TABLE IF NOT EXISTS`, `VARCHAR(n)`, `TEXT`) so they run on SQLite, PostgreSQL, and MariaDB. There are no foreign keys, so every deletion path (`PurgeOlderThan`, `PurgeByTag`, `DeleteBatch`, and the in-memory store) must delete dependent rows explicitly or they leak. `TestRunMigrationsRecordsVersion` asserts the exact migration count - bump it when adding one. Run `make test-integration` (Docker) to exercise all three backends.
