package main

import (
	"os"
	"strings"
	"testing"
)

// TestServerMDDatabaseSection verifies that docs/server.md documents all three
// database backends with their DSN formats and connection pool defaults.
func TestServerMDDatabaseSection(t *testing.T) {
	data, err := os.ReadFile("../../docs/server.md")
	if err != nil {
		t.Fatalf("read docs/server.md: %v", err)
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
			t.Errorf("docs/server.md missing %q", want)
		}
	}
}

// TestServerMDNoPhase2Placeholder verifies that the "not yet available"
// placeholder text has been removed now that PostgreSQL and MariaDB are
// implemented.
func TestServerMDNoPhase2Placeholder(t *testing.T) {
	data, err := os.ReadFile("../../docs/server.md")
	if err != nil {
		t.Fatalf("read docs/server.md: %v", err)
	}
	if strings.Contains(string(data), "not yet available") {
		t.Error("docs/server.md still contains 'not yet available' placeholder text")
	}
}
