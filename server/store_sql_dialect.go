package server

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
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
	// IsDuplicateKey returns true when err represents a unique-constraint
	// violation. Each driver surfaces this differently.
	IsDuplicateKey(err error) bool
	// SupportsOnConflictReturning reports whether this dialect supports
	// `INSERT ... ON CONFLICT (col) DO UPDATE ... RETURNING`. Postgres and
	// modern SQLite do; MySQL/MariaDB uses a different syntax and returns
	// false so the analysis upsert helpers fall back to the legacy
	// SELECT-then-INSERT-or-UPDATE pattern.
	SupportsOnConflictReturning() bool
	// Least returns a dialect-appropriate scalar-min expression over two
	// text columns. Postgres uses LEAST; SQLite uses min() as a scalar.
	Least(a, b string) string
	// Greatest is the dual of Least using GREATEST / max().
	Greatest(a, b string) string
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
func (sqliteDialect) IsDuplicateKey(err error) bool {
	return strings.Contains(err.Error(), "UNIQUE constraint failed")
}
func (sqliteDialect) SupportsOnConflictReturning() bool { return true }
func (sqliteDialect) Least(a, b string) string          { return fmt.Sprintf("min(%s, %s)", a, b) }
func (sqliteDialect) Greatest(a, b string) string       { return fmt.Sprintf("max(%s, %s)", a, b) }

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
func (postgresDialect) IsDuplicateKey(err error) bool {
	var pqErr *pq.Error
	return errors.As(err, &pqErr) && pqErr.Code == "23505"
}
func (postgresDialect) SupportsOnConflictReturning() bool { return true }
func (postgresDialect) Least(a, b string) string          { return fmt.Sprintf("LEAST(%s, %s)", a, b) }
func (postgresDialect) Greatest(a, b string) string       { return fmt.Sprintf("GREATEST(%s, %s)", a, b) }

// mariadbDialect is the dialect for github.com/go-sql-driver/mysql (driver
// name "mysql"), which also covers MariaDB.
type mariadbDialect struct{}

func (mariadbDialect) Placeholder(_ int) string { return "?" }
func (mariadbDialect) TimestampVal(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return formatSortableTimestamp(t)
}
func (mariadbDialect) DriverName() string { return "mysql" }

// IsDuplicateKey detects MySQL/MariaDB error 1062 (ER_DUP_ENTRY).
func (mariadbDialect) IsDuplicateKey(err error) bool {
	var mysqlErr *mysql.MySQLError
	return errors.As(err, &mysqlErr) && mysqlErr.Number == 1062
}

// MariaDB uses INSERT ... ON DUPLICATE KEY UPDATE and does not support
// RETURNING on that form, so the analysis upsert helpers take the legacy
// SELECT-then-INSERT-or-UPDATE fallback path.
func (mariadbDialect) SupportsOnConflictReturning() bool { return false }
func (mariadbDialect) Least(a, b string) string          { return fmt.Sprintf("LEAST(%s, %s)", a, b) }
func (mariadbDialect) Greatest(a, b string) string       { return fmt.Sprintf("GREATEST(%s, %s)", a, b) }

// dialectFor returns the dialect for a given driver name.
func dialectFor(driver string) (sqlDialect, error) {
	switch driver {
	case "sqlite":
		return sqliteDialect{}, nil
	case "postgres":
		return postgresDialect{}, nil
	case "mariadb", "mysql":
		return mariadbDialect{}, nil
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
	case "postgres", "mariadb", "mysql":
		db.SetMaxOpenConns(25)
		db.SetMaxIdleConns(5)
		db.SetConnMaxLifetime(5 * time.Minute)
	}
}

// mariadbDSN ensures the DSN contains parseTime=true, which is required for
// the go-sql-driver/mysql driver to scan DATETIME columns into time.Time. If
// parseTime is already specified in the DSN it is left untouched.
func mariadbDSN(dsn string) string {
	if strings.Contains(dsn, "parseTime=") {
		return dsn
	}
	if strings.Contains(dsn, "?") {
		return dsn + "&parseTime=true"
	}
	return dsn + "?parseTime=true"
}

// openSQLDB opens and configures a *sql.DB for the given driver and DSN.
func openSQLDB(driver, dsn string) (*sql.DB, error) {
	if dsn == "" {
		return nil, fmt.Errorf("database DSN is required for driver %q", driver)
	}
	sqlDriver := driver
	if driver == "mariadb" || driver == "mysql" {
		sqlDriver = "mysql"
		dsn = mariadbDSN(dsn)
	}
	db, err := sql.Open(sqlDriver, dsn)
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
