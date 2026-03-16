package main

import (
	"database/sql"
	"testing"
)

// TestRegisteredDrivers verifies that the blank imports in sqlite.go and
// postgres.go register their respective drivers with database/sql.
func TestRegisteredDrivers(t *testing.T) {
	registered := make(map[string]bool)
	for _, name := range sql.Drivers() {
		registered[name] = true
	}

	for _, driver := range []string{"sqlite", "postgres"} {
		if !registered[driver] {
			t.Errorf("driver %q not registered; check blank import in %s.go", driver, driver)
		}
	}
}
