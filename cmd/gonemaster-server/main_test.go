package main

import (
	"strings"
	"testing"

	"codeberg.org/pawal/gonemaster/cmd/internal/clitest"
)

func TestRunWorkersValidation(t *testing.T) {
	res := clitest.Run(t, run, "--workers", "0")
	res.RequireCode(t, 2)

	res.RequireErrContains(t, "--workers must be >= 1")
}

func TestRunInvalidConfig(t *testing.T) {
	res := clitest.Run(t, run, "--config", "./does-not-exist.json")
	res.RequireCode(t, 2)
	if strings.TrimSpace(res.Err) == "" {
		t.Fatalf("expected error output for invalid config")
	}
}

func TestRunHelpShowsGroupedFlags(t *testing.T) {
	res := clitest.Run(t, run, "-h")
	res.RequireCode(t, 2)

	expected := []string{
		"Usage: gonemaster-server [flags]",
		"General:",
		"Concurrency:",
		"Resolver/Profile:",
		"Database:",
		"Output:",
		"Logging:",
		"--workers N",
		"--version",
		"--min-level LEVEL",
		"--log-format FORMAT",
		"--log-level LEVEL",
		"--sourceaddr4 IPADDR",
		"--sourceaddr6 IPADDR",
		"--db-driver DRIVER",
		"--db-dsn DSN",
		"GONEMASTER_DB_DRIVER",
		"GONEMASTER_DB_DSN",
	}
	res.RequireErrContains(t, expected...)
}

func TestRunSourceAddr4Validation(t *testing.T) {
	res := clitest.Run(t, run, "--sourceaddr4", "not-an-ip")
	res.RequireCode(t, 2)
	res.RequireErrContains(t, "--sourceaddr4 must be a valid IPv4 address")
}

func TestRunLogFormatValidation(t *testing.T) {
	res := clitest.Run(t, run, "--log-format", "xml")
	res.RequireCode(t, 2)
	res.RequireErrContains(t, "invalid log_format")
}

func TestRunLogLevelValidation(t *testing.T) {
	res := clitest.Run(t, run, "--log-level", "verbose")
	res.RequireCode(t, 2)
	res.RequireErrContains(t, "invalid log_level")
}

func TestRunVersion(t *testing.T) {
	res := clitest.Run(t, run, "--version")
	res.RequireCode(t, 0)
	res.RequireOutContains(t, "Gonemaster version")
	res.RequireOutContains(t, "Miekg DNS version")
}

func TestRunDumpConfig(t *testing.T) {
	res := clitest.Run(t, run, "--dump-config", "--workers", "8", "--debug")
	res.RequireCode(t, 0)
	for _, want := range []string{`"worker_count": 8`, `"debug": true`, `"listen_addr":`} {
		if !strings.Contains(res.Out, want) {
			t.Fatalf("expected %q in dump-config output, got:\n%s", want, res.Out)
		}
	}
}

func TestRunDBRetentionDaysValidation(t *testing.T) {
	res := clitest.Run(t, run, "--db-retention-days", "-1")
	res.RequireCode(t, 2)
	res.RequireErrContains(t, "--db-retention-days must be >= 0")
}

// TestRunProfileOverrideValidation covers the flags the engine re-validates
// per run. Before this check the server accepted --retry 0 at startup, came up
// healthy, and then failed every job when the profile rejected the value, so
// the misconfiguration surfaced as broken results rather than as a refusal to
// start. Zero is the interesting case: it is not negative, so the old
// non-negative check let it through.
func TestRunProfileOverrideValidation(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"retry zero", []string{"--retry", "0"}, "--retry"},
		{"retry above max", []string{"--retry", "256"}, "--retry"},
		{"retry negative", []string{"--retry", "-1"}, "--retry"},
		{"retrans zero", []string{"--retrans", "0"}, "--retrans"},
		{"retrans above max", []string{"--retrans", "256"}, "--retrans"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			clitest.Run(t, run, tc.args...).
				RequireCode(t, 2).
				RequireErrContains(t, tc.want)
		})
	}
}

// TestRunProfileOverrideAcceptsInRangeValues pins the other side: values the
// profile accepts must still start the server, so the new check cannot become
// a refusal to run with a valid retry count.
func TestRunProfileOverrideAcceptsInRangeValues(t *testing.T) {
	res := clitest.Run(t, run, "--retry", "3", "--retrans", "2", "--dump-config")
	res.RequireCode(t, 0)
}

func TestRunDBRetentionDaysDumpConfig(t *testing.T) {
	res := clitest.Run(t, run, "--dump-config", "--db-retention-days", "90")
	res.RequireCode(t, 0)
	if !strings.Contains(res.Out, `"retention_days": 90`) {
		t.Fatalf("expected retention_days in dump-config output, got:\n%s", res.Out)
	}
}

func TestRunDBRetentionDaysHelpText(t *testing.T) {
	res := clitest.Run(t, run, "-h")
	res.RequireErrContains(t, "--db-retention-days")
}

func TestRunDBPurgeIntervalValidation(t *testing.T) {
	res := clitest.Run(t, run, "--db-purge-interval", "0")
	res.RequireCode(t, 2)
	res.RequireErrContains(t, "--db-purge-interval must be >= 1")
}

func TestRunDBPurgeIntervalDumpConfig(t *testing.T) {
	res := clitest.Run(t, run, "--dump-config", "--db-purge-interval", "1800")
	res.RequireCode(t, 0)
	if !strings.Contains(res.Out, `"purge_interval_seconds": 1800`) {
		t.Fatalf("expected purge_interval_seconds in dump-config output, got:\n%s", res.Out)
	}
}

func TestRunDBPurgeIntervalHelpText(t *testing.T) {
	res := clitest.Run(t, run, "-h")
	res.RequireErrContains(t, "--db-purge-interval")
}

func TestRunSourceAddr6Validation(t *testing.T) {
	res := clitest.Run(t, run, "--sourceaddr6", "192.0.2.10")
	res.RequireCode(t, 2)
	res.RequireErrContains(t, "--sourceaddr6 must be a valid IPv6 address")
}

func TestRunPublicAPIRateLimitMaxValidation(t *testing.T) {
	res := clitest.Run(t, run, "--public-api-rate-limit-max", "0")
	res.RequireCode(t, 2)
	res.RequireErrContains(t, "--public-api-rate-limit-max must be >= 1")
}

func TestRunPublicAPIRateLimitWindowValidation(t *testing.T) {
	res := clitest.Run(t, run, "--public-api-rate-limit-window", "-1s")
	res.RequireCode(t, 2)
	res.RequireErrContains(t, "--public-api-rate-limit-window must be positive")
}

func TestRunPublicAPIRateLimitDumpConfig(t *testing.T) {
	res := clitest.Run(t, run, "--dump-config", "--public-api-rate-limit-enabled", "--public-api-rate-limit-max", "20", "--public-api-rate-limit-window", "5m")
	res.RequireCode(t, 0)
	res.RequireOutContains(t,
		`"rate_limit_enabled": true`,
		`"rate_limit_max": 20`,
		`"rate_limit_window": "5m0s"`,
	)
}

func TestRunPublicAPIRateLimitHelpText(t *testing.T) {
	res := clitest.Run(t, run, "-h")
	res.RequireErrContains(t,
		"Public API:",
		"--public-api-rate-limit-enabled",
		"--public-api-rate-limit-max",
		"--public-api-rate-limit-window",
		"GONEMASTER_PUBLIC_API_RATE_LIMIT_ENABLED",
		"GONEMASTER_PUBLIC_API_RATE_LIMIT_MAX",
		"GONEMASTER_PUBLIC_API_RATE_LIMIT_WINDOW",
	)
}
