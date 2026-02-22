package server

import (
	"database/sql"
	"fmt"
	"time"
)

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
}

// sqliteDialect is the dialect for modernc.org/sqlite (driver name "sqlite").
type sqliteDialect struct{}

func (sqliteDialect) Placeholder(_ int) string { return "?" }
func (sqliteDialect) TimestampVal(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t.UTC().Format(time.RFC3339Nano)
}
func (sqliteDialect) DriverName() string { return "sqlite" }

// dialectFor returns the dialect for a given driver name.
func dialectFor(driver string) (sqlDialect, error) {
	switch driver {
	case "sqlite":
		return sqliteDialect{}, nil
	default:
		return nil, fmt.Errorf("unsupported database driver %q", driver)
	}
}

// openSQLDB opens and configures a *sql.DB for the given driver and DSN.
// For SQLite, MaxOpenConns is set to 1 to serialize all writes.
func openSQLDB(driver, dsn string) (*sql.DB, error) {
	if dsn == "" {
		return nil, fmt.Errorf("database DSN is required for driver %q", driver)
	}
	db, err := sql.Open(driver, dsn)
	if err != nil {
		return nil, fmt.Errorf("open %s database: %w", driver, err)
	}
	if driver == "sqlite" {
		db.SetMaxOpenConns(1)
	}
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping %s database: %w", driver, err)
	}
	return db, nil
}
