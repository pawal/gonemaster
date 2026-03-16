package server

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/lib/pq"
)

const sortableTimestampLayout = "2006-01-02T15:04:05.000000000Z07:00"

// formatSortableTimestamp returns a fixed-width UTC timestamp string that is
// lexicographically sortable.
func formatSortableTimestamp(t time.Time) string {
	return t.UTC().Format(sortableTimestampLayout)
}

// sqlDialect abstracts backend-specific SQL differences.
type sqlDialect interface {
	// Placeholder returns a bind-parameter marker for position n (1-based).
	// SQLite uses "?" for all positions; PostgreSQL uses "$n".
	Placeholder(n int) string
	// TimestampVal converts t to the value stored in a TEXT timestamp column.
	// A zero time returns nil (stored as SQL NULL).
	TimestampVal(t time.Time) any
	// DriverName returns the sql.DB driver name for this dialect.
	DriverName() string
	// UpsertResultSQL returns a complete INSERT-or-update statement for the
	// results table using this dialect's placeholder and conflict-resolution
	// syntax. Bind order: job_id, batch_id, status, summary_json, raw_json.
	UpsertResultSQL() string
	// IsDuplicateKey returns true when err represents a unique-constraint
	// violation. Each driver surfaces this differently.
	IsDuplicateKey(err error) bool
}

// sqliteDialect is the dialect for modernc.org/sqlite (driver name "sqlite").
type sqliteDialect struct{}

func (sqliteDialect) Placeholder(_ int) string { return "?" }
func (sqliteDialect) TimestampVal(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return formatSortableTimestamp(t)
}
func (sqliteDialect) DriverName() string { return "sqlite" }
func (sqliteDialect) UpsertResultSQL() string {
	return `INSERT OR REPLACE INTO results (job_id, batch_id, status, summary_json, raw_json)
		 VALUES (?, ?, ?, ?, ?)`
}
func (sqliteDialect) IsDuplicateKey(err error) bool {
	return strings.Contains(err.Error(), "UNIQUE constraint failed")
}

// postgresDialect is the dialect for github.com/lib/pq (driver name "postgres").
type postgresDialect struct{}

func (postgresDialect) Placeholder(n int) string { return fmt.Sprintf("$%d", n) }

// TimestampVal formats t as a fixed-width RFC3339Nano string, consistent with
// the TEXT columns used across all backends. A zero time returns nil (SQL NULL).
func (postgresDialect) TimestampVal(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return formatSortableTimestamp(t)
}
func (postgresDialect) DriverName() string { return "postgres" }
func (postgresDialect) UpsertResultSQL() string {
	return `INSERT INTO results (job_id, batch_id, status, summary_json, raw_json)
		 VALUES ($1, $2, $3, $4, $5)
		 ON CONFLICT (job_id) DO UPDATE SET
		 	batch_id     = EXCLUDED.batch_id,
		 	status       = EXCLUDED.status,
		 	summary_json = EXCLUDED.summary_json,
		 	raw_json     = EXCLUDED.raw_json`
}
func (postgresDialect) IsDuplicateKey(err error) bool {
	var pqErr *pq.Error
	return errors.As(err, &pqErr) && pqErr.Code == "23505"
}

// dialectFor returns the dialect for a given driver name.
func dialectFor(driver string) (sqlDialect, error) {
	switch driver {
	case "sqlite":
		return sqliteDialect{}, nil
	case "postgres":
		return postgresDialect{}, nil
	default:
		return nil, fmt.Errorf("unsupported database driver %q", driver)
	}
}

// configurePool sets connection pool parameters appropriate for the given driver.
// SQLite must use a single connection to serialise writes; client/server
// databases use a bounded pool.
func configurePool(db *sql.DB, driver string) {
	switch driver {
	case "sqlite":
		db.SetMaxOpenConns(1)
	case "postgres":
		db.SetMaxOpenConns(25)
		db.SetMaxIdleConns(5)
		db.SetConnMaxLifetime(5 * time.Minute)
	}
}

// openSQLDB opens and configures a *sql.DB for the given driver and DSN.
func openSQLDB(driver, dsn string) (*sql.DB, error) {
	if dsn == "" {
		return nil, fmt.Errorf("database DSN is required for driver %q", driver)
	}
	db, err := sql.Open(driver, dsn)
	if err != nil {
		return nil, fmt.Errorf("open %s database: %w", driver, err)
	}
	configurePool(db, driver)
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping %s database: %w", driver, err)
	}
	return db, nil
}
