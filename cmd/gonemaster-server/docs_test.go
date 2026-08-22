package main

import (
	"testing"

	"codeberg.org/pawal/gonemaster/cmd/internal/clitest"
)

// TestServerDatabaseMDSection verifies that docs/server/database.md documents
// all three database backends with their DSN formats and connection pool defaults.
func TestServerDatabaseMDSection(t *testing.T) {
	clitest.FileContains(t, "../../docs/server/database.md",
		// All three backends listed
		"`sqlite`",
		"`postgres`",
		"`mariadb`",
		// DSN format examples
		"postgres://",
		"tcp(host:3306)",
		"sslmode=disable",
		"parseTime=true",
		// Connection pool table
		"Max open connections",
		"Max idle",
		"Connection lifetime",
		// Environment variable recommendation
		"GONEMASTER_DB_DSN",
	)
}

// TestServerDatabaseMDLinksToSetupGuide verifies that docs/server/database.md
// references the database setup guide so readers can find detailed instructions.
func TestServerDatabaseMDLinksToSetupGuide(t *testing.T) {
	clitest.FileContains(t, "../../docs/server/database.md", "database-setup.md")
}

// TestDatabaseSetupMDExists verifies the database setup guide exists and
// covers PostgreSQL, MariaDB, DSN requirements, and key tuning parameters.
func TestDatabaseSetupMDExists(t *testing.T) {
	clitest.FileContains(t, "../../docs/server/database-setup.md",
		// Both backends covered
		"## PostgreSQL",
		"## MariaDB",
		// User/database creation
		"CREATE DATABASE",
		"CREATE USER",
		// Key tuning params
		"shared_buffers",
		"innodb_buffer_pool_size",
		"innodb_file_per_table",
		// DSN requirements
		"parseTime=true",
		"utf8mb4",
		// Backup
		"pg_dump",
		"mysqldump",
		// Cross-references
		"docker-compose.test.yml",
		"test-integration",
	)
}

// TestServerDatabaseMDRetentionDays verifies that docs/server/database.md
// documents the retention_days and purge-interval configuration fields, env
// vars, and CLI flags.
func TestServerDatabaseMDRetentionDays(t *testing.T) {
	clitest.FileContains(t, "../../docs/server/database.md",
		"retention_days",
		"--db-retention-days",
		"every hour",
		"--db-purge-interval",
		"purge_interval_seconds",
	)
}
