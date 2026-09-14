package main

import (
	"testing"

	"codeberg.org/pawal/gonemaster/cmd/internal/clitest"
)

// TestWoodpeckerIntegrationStep verifies that .woodpecker/ci.yml declares the
// integration_test step and references the postgres and mariadb services with
// the DSN environment variables the parameterized store tests expect.
func TestWoodpeckerIntegrationStep(t *testing.T) {
	clitest.FileContains(t, "../../.woodpecker/ci.yml",
		"integration_test",
		"TEST_POSTGRES_DSN",
		"TEST_MARIADB_DSN",
		"postgres://gonemaster",
		"tcp(mariadb:",
	)
}

// TestWoodpeckerServicesPresent verifies that the postgres and mariadb service
// definitions are present so the integration_test step can reach them.
func TestWoodpeckerServicesPresent(t *testing.T) {
	clitest.FileContains(t, "../../.woodpecker/ci.yml",
		"POSTGRES_USER", "POSTGRES_DB", "MARIADB_USER", "MARIADB_DATABASE")
}

// TestDockerComposeTestFile verifies that docker-compose.test.yml exists and
// defines both postgres and mariadb services with health checks, matching the
// make test-integration target and the CI service definitions.
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
