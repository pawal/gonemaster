package server

import (
	"database/sql"
	"fmt"
)

type sqlMigration struct {
	version int
	stmts   []string
}

// sqlMigrations is the ordered list of schema migrations.
// Each migration is applied exactly once, tracked by version in schema_migrations.
var sqlMigrations = []sqlMigration{
	{
		version: 1,
		stmts: []string{
			`CREATE TABLE IF NOT EXISTS jobs (
				id                  TEXT PRIMARY KEY,
				batch_id            TEXT NOT NULL DEFAULT '',
				domain              TEXT NOT NULL DEFAULT '',
				status              TEXT NOT NULL DEFAULT 'queued',
				created_at          TEXT NOT NULL,
				started_at          TEXT,
				finished_at         TEXT,
				progress            INTEGER NOT NULL DEFAULT 0,
				result_url          TEXT NOT NULL DEFAULT '',
				error               TEXT NOT NULL DEFAULT '',
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
				job_id       TEXT PRIMARY KEY,
				batch_id     TEXT NOT NULL DEFAULT '',
				status       TEXT NOT NULL DEFAULT '',
				summary_json TEXT,
				raw_json     TEXT
			)`,
		},
	},
}

// runMigrations creates the schema_migrations tracking table and applies any
// pending migrations. Each migration runs inside its own transaction so a
// partial failure leaves the database in the last fully-applied state.
func runMigrations(db *sql.DB) error {
	if _, err := db.Exec(
		`CREATE TABLE IF NOT EXISTS schema_migrations (version INTEGER PRIMARY KEY)`,
	); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	for _, m := range sqlMigrations {
		var count int
		if err := db.QueryRow(
			`SELECT COUNT(*) FROM schema_migrations WHERE version = ?`, m.version,
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
		for _, stmt := range m.stmts {
			if _, err := tx.Exec(stmt); err != nil {
				_ = tx.Rollback()
				return fmt.Errorf("migration %d: %w", m.version, err)
			}
		}
		if _, err := tx.Exec(
			`INSERT INTO schema_migrations(version) VALUES (?)`, m.version,
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
