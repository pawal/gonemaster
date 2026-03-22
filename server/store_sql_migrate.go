package server

import (
	"database/sql"
	"fmt"
)

type sqlMigration struct {
	version int
	stmts   []string
	stmtsFn func(sqlDialect) []string // dialect-specific statements (appended to stmts)
}

// sqlMigrations is the ordered list of schema migrations.
// Each migration is applied exactly once, tracked by version in schema_migrations.
var sqlMigrations = []sqlMigration{
	{
		version: 1,
		stmts: []string{
			// VARCHAR is used for PRIMARY KEY and indexed columns because
			// MariaDB/MySQL requires a key length for TEXT columns in index
			// specifications. SQLite and PostgreSQL treat VARCHAR identically
			// to TEXT so the schema works across all three backends.
			`CREATE TABLE IF NOT EXISTS jobs (
				id                  VARCHAR(255) NOT NULL PRIMARY KEY,
				batch_id            VARCHAR(255) NOT NULL DEFAULT '',
				domain              TEXT         NOT NULL DEFAULT '',
				status              VARCHAR(32)  NOT NULL DEFAULT 'queued',
				created_at          VARCHAR(64)  NOT NULL,
				started_at          VARCHAR(64),
				finished_at         VARCHAR(64),
				progress            INTEGER NOT NULL DEFAULT 0,
				result_url          TEXT    NOT NULL DEFAULT '',
				error               TEXT    NOT NULL DEFAULT '',
				sev_notice          INTEGER NOT NULL DEFAULT 0,
				sev_warning         INTEGER NOT NULL DEFAULT 0,
				sev_error           INTEGER NOT NULL DEFAULT 0,
				sev_critical        INTEGER NOT NULL DEFAULT 0,
				tests_json          TEXT,
				overrides_json      TEXT,
				undelegated_ns_json TEXT,
				undelegated_ds_json TEXT,
				min_level           TEXT NOT NULL DEFAULT ''
			)`,
			`CREATE INDEX IF NOT EXISTS idx_jobs_batch_id   ON jobs(batch_id)`,
			`CREATE INDEX IF NOT EXISTS idx_jobs_status     ON jobs(status)`,
			`CREATE INDEX IF NOT EXISTS idx_jobs_created_at ON jobs(created_at)`,
			`CREATE TABLE IF NOT EXISTS results (
				job_id       VARCHAR(255) NOT NULL PRIMARY KEY,
				batch_id     TEXT NOT NULL DEFAULT '',
				status       TEXT NOT NULL DEFAULT '',
				summary_json TEXT,
				raw_json     TEXT
			)`,
		},
	},
	{
		version: 2,
		stmts: []string{
			// Index on finished_at makes PurgeOlderThan efficient even with
			// millions of rows; NULL values (unfinished jobs) are excluded by
			// the purge WHERE clause so the index stays compact.
			`CREATE INDEX IF NOT EXISTS idx_jobs_finished_at ON jobs(finished_at)`,
		},
	},
	{
		version: 3,
		stmts: []string{
			// public_id is nullable so that the unique index works across all
			// three backends — SQLite, PostgreSQL and MariaDB all treat NULL
			// as distinct in a UNIQUE index, so pre-migration rows (NULL) do
			// not collide with each other or with newly generated IDs.
			`ALTER TABLE jobs ADD COLUMN public_id VARCHAR(16) DEFAULT NULL`,
			`CREATE UNIQUE INDEX IF NOT EXISTS idx_jobs_public_id ON jobs(public_id)`,
		},
	},
	{
		version: 4,
		stmtsFn: func(d sqlDialect) []string {
			if _, ok := d.(mariadbDialect); ok {
				return []string{
					// MariaDB TEXT is limited to 64 KB; large zones (e.g. "com")
					// produce raw_json well over that limit. MEDIUMTEXT allows
					// up to 16 MB. SQLite and PostgreSQL TEXT is unlimited.
					`ALTER TABLE results MODIFY raw_json MEDIUMTEXT`,
					`ALTER TABLE results MODIFY summary_json MEDIUMTEXT`,
				}
			}
			return nil
		},
	},
}

// backfillPublicIDs assigns a public_id to every job that does not yet have
// one. It is called after runMigrations so that jobs created before migration 3
// get a stable public ID on the next server start.
func backfillPublicIDs(db *sql.DB, d sqlDialect) error {
	rows, err := db.Query(`SELECT id FROM jobs WHERE public_id IS NULL`)
	if err != nil {
		return fmt.Errorf("backfill public IDs: query: %w", err)
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return fmt.Errorf("backfill public IDs: scan: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("backfill public IDs: rows: %w", err)
	}

	stmt := fmt.Sprintf(`UPDATE jobs SET public_id = %s WHERE id = %s`,
		d.Placeholder(1), d.Placeholder(2))
	for _, id := range ids {
		if _, err := db.Exec(stmt, GeneratePublicID(), id); err != nil {
			return fmt.Errorf("backfill public IDs: update %s: %w", id, err)
		}
	}
	return nil
}

// runMigrations creates the schema_migrations tracking table and applies any
// pending migrations. Each migration runs inside its own transaction so a
// partial failure leaves the database in the last fully-applied state.
// d is used for placeholder syntax so the runner works with any SQL dialect.
func runMigrations(db *sql.DB, d sqlDialect) error {
	if _, err := db.Exec(
		`CREATE TABLE IF NOT EXISTS schema_migrations (version INTEGER PRIMARY KEY)`,
	); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	ph := d.Placeholder(1)
	for _, m := range sqlMigrations {
		var count int
		if err := db.QueryRow(
			`SELECT COUNT(*) FROM schema_migrations WHERE version = `+ph, m.version,
		).Scan(&count); err != nil {
			return fmt.Errorf("check migration %d: %w", m.version, err)
		}
		if count > 0 {
			continue
		}

		tx, err := db.Begin()
		if err != nil {
			return fmt.Errorf("begin migration %d: %w", m.version, err)
		}
		allStmts := m.stmts
		if m.stmtsFn != nil {
			allStmts = append(allStmts, m.stmtsFn(d)...)
		}
		for _, stmt := range allStmts {
			if _, err := tx.Exec(stmt); err != nil {
				_ = tx.Rollback()
				return fmt.Errorf("migration %d: %w", m.version, err)
			}
		}
		if _, err := tx.Exec(
			`INSERT INTO schema_migrations(version) VALUES (`+ph+`)`, m.version,
		); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("record migration %d: %w", m.version, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration %d: %w", m.version, err)
		}
	}
	return nil
}
