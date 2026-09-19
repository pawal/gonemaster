package main

import (
	"testing"

	"codeberg.org/pawal/gonemaster/cmd/internal/clitest"
)

// TestWoodpeckerIntegrationStep pins the integration step and DSNs in ci.yml.
func TestWoodpeckerIntegrationStep(t *testing.T) {
	clitest.FileContains(t, "../../.woodpecker/ci.yml",
		"integration_test",
		"TEST_POSTGRES_DSN",
		"TEST_MARIADB_DSN",
		"postgres://gonemaster",
		"tcp(mariadb:",
	)
}

// TestWoodpeckerServicesPresent pins the postgres and mariadb service variables in ci.yml.
func TestWoodpeckerServicesPresent(t *testing.T) {
	clitest.FileContains(t, "../../.woodpecker/ci.yml",
		"POSTGRES_USER", "POSTGRES_DB", "MARIADB_USER", "MARIADB_DATABASE")
}

// TestDockerComposeTestFile pins the postgres and mariadb services in docker-compose.test.yml.
func TestDockerComposeTestFile(t *testing.T) {
	clitest.FileContains(t, "../../docker-compose.test.yml",
		"postgres:",
		"mariadb:",
		"healthcheck:",
		"POSTGRES_USER",
		"MARIADB_USER",
		"TEST_POSTGRES_DSN",
		"TEST_MARIADB_DSN",
	)
}
