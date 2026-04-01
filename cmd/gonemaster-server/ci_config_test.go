package main

import (
	"os"
	"strings"
	"testing"
)

// TestWoodpeckerIntegrationStep verifies that .woodpecker.yml declares the
// integration_test step and references the postgres and mariadb services with
// the DSN environment variables the parameterized store tests expect.
func TestWoodpeckerIntegrationStep(t *testing.T) {
	data, err := os.ReadFile("../../.woodpecker.yml")
	if err != nil {
		t.Fatalf("read .woodpecker.yml: %v", err)
	}
	src := string(data)

	for _, want := range []string{
		"integration_test",
		"TEST_POSTGRES_DSN",
		"TEST_MARIADB_DSN",
		"postgres://gonemaster",
		"tcp(mariadb:",
	} {
		if !strings.Contains(src, want) {
			t.Errorf(".woodpecker.yml missing %q", want)
		}
	}
}

// TestWoodpeckerServicesPresent verifies that the postgres and mariadb service
// definitions are present so the integration_test step can reach them.
func TestWoodpeckerServicesPresent(t *testing.T) {
	data, err := os.ReadFile("../../.woodpecker.yml")
	if err != nil {
		t.Fatalf("read .woodpecker.yml: %v", err)
	}
	src := string(data)

	for _, svc := range []string{"POSTGRES_USER", "POSTGRES_DB", "MARIADB_USER", "MARIADB_DATABASE"} {
		if !strings.Contains(src, svc) {
			t.Errorf(".woodpecker.yml missing service env %q", svc)
		}
	}
}

// TestDockerComposeTestFile verifies that docker-compose.test.yml exists and
// defines both postgres and mariadb services with health checks, matching the
// make test-integration target and the CI service definitions.
func TestDockerComposeTestFile(t *testing.T) {
	data, err := os.ReadFile("../../docker-compose.test.yml")
	if err != nil {
		t.Fatalf("read docker-compose.test.yml: %v", err)
	}
	src := string(data)

	for _, want := range []string{
		"postgres:",
		"mariadb:",
		"healthcheck:",
		"POSTGRES_USER",
		"MARIADB_USER",
		"TEST_POSTGRES_DSN",
		"TEST_MARIADB_DSN",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("docker-compose.test.yml missing %q", want)
		}
	}
}
