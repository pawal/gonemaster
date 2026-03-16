package main

import (
	"database/sql"
	"testing"
)

// TestRegisteredDrivers verifies that the blank imports in sqlite.go,
// postgres.go, and mariadb.go register their respective drivers with
// database/sql. Note: go-sql-driver/mysql registers under "mysql", not
// "mariadb" — "mariadb" is only the gonemaster-level config name.
func TestRegisteredDrivers(t *testing.T) {
	registered := make(map[string]bool)
	for _, name := range sql.Drivers() {
		registered[name] = true
	}

	for _, driver := range []string{"sqlite", "postgres", "mysql"} {
		if !registered[driver] {
			t.Errorf("driver %q not registered; check blank import in %s.go", driver, driver)
		}
	}
}
