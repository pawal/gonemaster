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
	// IsRetryableConflict returns true when err aborted the whole
	// transaction on contention rather than on a defect in the statement.
	// The caller must restart from Begin.
	IsRetryableConflict(err error) bool
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

// IsRetryableConflict detects SQLITE_BUSY / SQLITE_LOCKED.
func (sqliteDialect) IsRetryableConflict(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "database is locked") ||
		strings.Contains(msg, "database table is locked") ||
		strings.Contains(msg, "SQLITE_BUSY")
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

// IsRetryableConflict detects SQLSTATE 40001 (serialization_failure) and
// 40P01 (deadlock_detected).
func (postgresDialect) IsRetryableConflict(err error) bool {
	var pqErr *pq.Error
	if !errors.As(err, &pqErr) {
		return false
	}
	return pqErr.Code == "40001" || pqErr.Code == "40P01"
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

// MariaDB/MySQL errors that roll the transaction back and are safe to retry.
const (
	erLockWaitTimeout = 1205 // ER_LOCK_WAIT_TIMEOUT
	erLockDeadlock    = 1213 // ER_LOCK_DEADLOCK
	erCheckRead       = 1020 // ER_CHECKREAD, "record has changed since last read"
)

// IsRetryableConflict detects the three contention aborts. ER_CHECKREAD
// needs innodb_snapshot_isolation, on by default since MariaDB 11.6.2.
func (mariadbDialect) IsRetryableConflict(err error) bool {
	var mysqlErr *mysql.MySQLError
	if !errors.As(err, &mysqlErr) {
		return false
	}
	switch mysqlErr.Number {
	case erCheckRead, erLockDeadlock, erLockWaitTimeout:
		return true
	default:
		return false
	}
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
	configurePoolWith(db, driver, DatabaseConfig{})
}

// configurePoolWith is configurePool with operator overrides. Zero
// values fall back to the driver-appropriate default.
func configurePoolWith(db *sql.DB, driver string, cfg DatabaseConfig) {
	switch driver {
	case "sqlite":
		max := cfg.MaxOpenConns
		if max <= 0 {
			max = 1
		}
		db.SetMaxOpenConns(max)
	case "postgres", "mariadb", "mysql":
		maxOpen := cfg.MaxOpenConns
		if maxOpen <= 0 {
			maxOpen = 25
		}
		maxIdle := cfg.MaxIdleConns
		if maxIdle <= 0 {
			maxIdle = 5
		}
		lifetime := time.Duration(cfg.ConnMaxLifetimeSeconds) * time.Second
		if lifetime <= 0 {
			lifetime = 5 * time.Minute
		}
		db.SetMaxOpenConns(maxOpen)
		db.SetMaxIdleConns(maxIdle)
		db.SetConnMaxLifetime(lifetime)
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

// openSQLDBWith is openSQLDB with explicit pool overrides from cfg.
func openSQLDBWith(driver, dsn string, cfg DatabaseConfig) (*sql.DB, error) {
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
	configurePoolWith(db, driver, cfg)
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping %s database: %w", driver, err)
	}
	return db, nil
}
