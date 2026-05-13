package main

import (
	"os"
	"strings"
	"testing"
)

// TestServerDatabaseMDSection verifies that docs/server/database.md documents
// all three database backends with their DSN formats and connection pool defaults.
func TestServerDatabaseMDSection(t *testing.T) {
	data, err := os.ReadFile("../../docs/server/database.md")
	if err != nil {
		t.Fatalf("read docs/server/database.md: %v", err)
	}
	src := string(data)

	for _, want := range []string{
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
	} {
		if !strings.Contains(src, want) {
			t.Errorf("docs/server/database.md missing %q", want)
		}
	}
}

// TestServerDatabaseMDLinksToSetupGuide verifies that docs/server/database.md
// references the database setup guide so readers can find detailed instructions.
func TestServerDatabaseMDLinksToSetupGuide(t *testing.T) {
	data, err := os.ReadFile("../../docs/server/database.md")
	if err != nil {
		t.Fatalf("read docs/server/database.md: %v", err)
	}
	if !strings.Contains(string(data), "database-setup.md") {
		t.Error("docs/server/database.md does not link to database-setup.md")
	}
}

// TestDatabaseSetupMDExists verifies the database setup guide exists and
// covers PostgreSQL, MariaDB, DSN requirements, and key tuning parameters.
func TestDatabaseSetupMDExists(t *testing.T) {
	data, err := os.ReadFile("../../docs/database-setup.md")
	if err != nil {
		t.Fatalf("read docs/database-setup.md: %v", err)
	}
	src := string(data)

	for _, want := range []string{
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
	} {
		if !strings.Contains(src, want) {
			t.Errorf("docs/database-setup.md missing %q", want)
		}
	}
}

// TestServerDatabaseMDRetentionDays verifies that docs/server/database.md
// documents the retention_days configuration field, env var, and CLI flag.
func TestServerDatabaseMDRetentionDays(t *testing.T) {
	data, err := os.ReadFile("../../docs/server/database.md")
	if err != nil {
		t.Fatalf("read docs/server/database.md: %v", err)
	}
	src := string(data)
	for _, want := range []string{
		"retention_days",
		"--db-retention-days",
		"hourly",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("docs/server/database.md missing %q", want)
		}
	}
}
