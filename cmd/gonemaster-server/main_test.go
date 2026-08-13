package main

import (
	"os"
	"strings"
	"testing"
)

func TestRunWorkersValidation(t *testing.T) {
	out := newTempFile(t)
	errOut := newTempFile(t)
	defer cleanupTempFile(t, out)
	defer cleanupTempFile(t, errOut)

	code := run([]string{"--workers", "0"}, out, errOut)
	if code != 2 {
		t.Fatalf("expected exit code 2, got %d", code)
	}

	errText := readTempFile(t, errOut)
	if !strings.Contains(errText, "--workers must be >= 1") {
		t.Fatalf("expected workers validation error, got %q", errText)
	}
}

func TestRunInvalidConfig(t *testing.T) {
	out := newTempFile(t)
	errOut := newTempFile(t)
	defer cleanupTempFile(t, out)
	defer cleanupTempFile(t, errOut)

	code := run([]string{"--config", "./does-not-exist.json"}, out, errOut)
	if code != 2 {
		t.Fatalf("expected exit code 2, got %d", code)
	}
	errText := readTempFile(t, errOut)
	if strings.TrimSpace(errText) == "" {
		t.Fatalf("expected error output for invalid config")
	}
}

func TestRunHelpShowsGroupedFlags(t *testing.T) {
	out := newTempFile(t)
	errOut := newTempFile(t)
	defer cleanupTempFile(t, out)
	defer cleanupTempFile(t, errOut)

	code := run([]string{"-h"}, out, errOut)
	if code != 2 {
		t.Fatalf("expected exit code 2, got %d", code)
	}

	help := readTempFile(t, errOut)
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
	for _, fragment := range expected {
		if !strings.Contains(help, fragment) {
			t.Fatalf("expected %q in help output, got:\n%s", fragment, help)
		}
	}
}

func TestRunSourceAddr4Validation(t *testing.T) {
	out := newTempFile(t)
	errOut := newTempFile(t)
	defer cleanupTempFile(t, out)
	defer cleanupTempFile(t, errOut)

	code := run([]string{"--sourceaddr4", "not-an-ip"}, out, errOut)
	if code != 2 {
		t.Fatalf("expected exit code 2, got %d", code)
	}
	errText := readTempFile(t, errOut)
	if !strings.Contains(errText, "--sourceaddr4 must be a valid IPv4 address") {
		t.Fatalf("expected sourceaddr4 validation error, got %q", errText)
	}
}

func TestRunLogFormatValidation(t *testing.T) {
	out := newTempFile(t)
	errOut := newTempFile(t)
	defer cleanupTempFile(t, out)
	defer cleanupTempFile(t, errOut)

	code := run([]string{"--log-format", "xml"}, out, errOut)
	if code != 2 {
		t.Fatalf("expected exit code 2, got %d", code)
	}
	errText := readTempFile(t, errOut)
	if !strings.Contains(errText, "invalid log_format") {
		t.Fatalf("expected log_format validation error, got %q", errText)
	}
}

func TestRunLogLevelValidation(t *testing.T) {
	out := newTempFile(t)
	errOut := newTempFile(t)
	defer cleanupTempFile(t, out)
	defer cleanupTempFile(t, errOut)

	code := run([]string{"--log-level", "verbose"}, out, errOut)
	if code != 2 {
		t.Fatalf("expected exit code 2, got %d", code)
	}
	errText := readTempFile(t, errOut)
	if !strings.Contains(errText, "invalid log_level") {
		t.Fatalf("expected log_level validation error, got %q", errText)
	}
}

func TestRunVersion(t *testing.T) {
	out := newTempFile(t)
	errOut := newTempFile(t)
	defer cleanupTempFile(t, out)
	defer cleanupTempFile(t, errOut)

	code := run([]string{"--version"}, out, errOut)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
	outText := readTempFile(t, out)
	if !strings.Contains(outText, "Gonemaster version") {
		t.Fatalf("expected gonemaster version output, got %q", outText)
	}
	if !strings.Contains(outText, "Miekg DNS version") {
		t.Fatalf("expected miekg version output, got %q", outText)
	}
}

func TestRunDumpConfig(t *testing.T) {
	out := newTempFile(t)
	errOut := newTempFile(t)
	defer cleanupTempFile(t, out)
	defer cleanupTempFile(t, errOut)

	code := run([]string{"--dump-config", "--workers", "8", "--debug"}, out, errOut)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
	outText := readTempFile(t, out)
	for _, want := range []string{`"worker_count": 8`, `"debug": true`, `"listen_addr":`} {
		if !strings.Contains(outText, want) {
			t.Fatalf("expected %q in dump-config output, got:\n%s", want, outText)
		}
	}
}

func TestRunDBRetentionDaysValidation(t *testing.T) {
	out := newTempFile(t)
	errOut := newTempFile(t)
	defer cleanupTempFile(t, out)
	defer cleanupTempFile(t, errOut)

	code := run([]string{"--db-retention-days", "-1"}, out, errOut)
	if code != 2 {
		t.Fatalf("expected exit code 2, got %d", code)
	}
	errText := readTempFile(t, errOut)
	if !strings.Contains(errText, "--db-retention-days must be >= 0") {
		t.Fatalf("expected retention-days validation error, got %q", errText)
	}
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
			out := newTempFile(t)
			errOut := newTempFile(t)
			defer cleanupTempFile(t, out)
			defer cleanupTempFile(t, errOut)

			if code := run(tc.args, out, errOut); code != 2 {
				t.Fatalf("expected exit code 2 for %v, got %d", tc.args, code)
			}
			errText := readTempFile(t, errOut)
			if !strings.Contains(errText, tc.want) {
				t.Fatalf("expected %q named in the error, got %q", tc.want, errText)
			}
		})
	}
}

// TestRunProfileOverrideAcceptsInRangeValues pins the other side: values the
// profile accepts must still start the server, so the new check cannot become
// a refusal to run with a valid retry count.
func TestRunProfileOverrideAcceptsInRangeValues(t *testing.T) {
	out := newTempFile(t)
	errOut := newTempFile(t)
	defer cleanupTempFile(t, out)
	defer cleanupTempFile(t, errOut)

	code := run([]string{"--retry", "3", "--retrans", "2", "--dump-config"}, out, errOut)
	if code != 0 {
		t.Fatalf("expected exit code 0 for in-range values, got %d (stderr %q)", code, readTempFile(t, errOut))
	}
}

func TestRunDBRetentionDaysDumpConfig(t *testing.T) {
	out := newTempFile(t)
	errOut := newTempFile(t)
	defer cleanupTempFile(t, out)
	defer cleanupTempFile(t, errOut)

	code := run([]string{"--dump-config", "--db-retention-days", "90"}, out, errOut)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
	outText := readTempFile(t, out)
	if !strings.Contains(outText, `"retention_days": 90`) {
		t.Fatalf("expected retention_days in dump-config output, got:\n%s", outText)
	}
}

func TestRunDBRetentionDaysHelpText(t *testing.T) {
	out := newTempFile(t)
	errOut := newTempFile(t)
	defer cleanupTempFile(t, out)
	defer cleanupTempFile(t, errOut)

	run([]string{"-h"}, out, errOut)
	errText := readTempFile(t, errOut)
	if !strings.Contains(errText, "--db-retention-days") {
		t.Fatalf("expected --db-retention-days in help output, got:\n%s", errText)
	}
}

func TestRunDBPurgeIntervalValidation(t *testing.T) {
	out := newTempFile(t)
	errOut := newTempFile(t)
	defer cleanupTempFile(t, out)
	defer cleanupTempFile(t, errOut)

	code := run([]string{"--db-purge-interval", "0"}, out, errOut)
	if code != 2 {
		t.Fatalf("expected exit code 2, got %d", code)
	}
	errText := readTempFile(t, errOut)
	if !strings.Contains(errText, "--db-purge-interval must be >= 1") {
		t.Fatalf("expected purge-interval validation error, got %q", errText)
	}
}

func TestRunDBPurgeIntervalDumpConfig(t *testing.T) {
	out := newTempFile(t)
	errOut := newTempFile(t)
	defer cleanupTempFile(t, out)
	defer cleanupTempFile(t, errOut)

	code := run([]string{"--dump-config", "--db-purge-interval", "1800"}, out, errOut)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
	outText := readTempFile(t, out)
	if !strings.Contains(outText, `"purge_interval_seconds": 1800`) {
		t.Fatalf("expected purge_interval_seconds in dump-config output, got:\n%s", outText)
	}
}

func TestRunDBPurgeIntervalHelpText(t *testing.T) {
	out := newTempFile(t)
	errOut := newTempFile(t)
	defer cleanupTempFile(t, out)
	defer cleanupTempFile(t, errOut)

	run([]string{"-h"}, out, errOut)
	errText := readTempFile(t, errOut)
	if !strings.Contains(errText, "--db-purge-interval") {
		t.Fatalf("expected --db-purge-interval in help output, got:\n%s", errText)
	}
}

func TestRunSourceAddr6Validation(t *testing.T) {
	out := newTempFile(t)
	errOut := newTempFile(t)
	defer cleanupTempFile(t, out)
	defer cleanupTempFile(t, errOut)

	code := run([]string{"--sourceaddr6", "192.0.2.10"}, out, errOut)
	if code != 2 {
		t.Fatalf("expected exit code 2, got %d", code)
	}
	errText := readTempFile(t, errOut)
	if !strings.Contains(errText, "--sourceaddr6 must be a valid IPv6 address") {
		t.Fatalf("expected sourceaddr6 validation error, got %q", errText)
	}
}

func TestRunPublicAPIRateLimitMaxValidation(t *testing.T) {
	out := newTempFile(t)
	errOut := newTempFile(t)
	defer cleanupTempFile(t, out)
	defer cleanupTempFile(t, errOut)

	code := run([]string{"--public-api-rate-limit-max", "0"}, out, errOut)
	if code != 2 {
		t.Fatalf("expected exit code 2, got %d", code)
	}
	errText := readTempFile(t, errOut)
	if !strings.Contains(errText, "--public-api-rate-limit-max must be >= 1") {
		t.Fatalf("expected validation error, got %q", errText)
	}
}

func TestRunPublicAPIRateLimitWindowValidation(t *testing.T) {
	out := newTempFile(t)
	errOut := newTempFile(t)
	defer cleanupTempFile(t, out)
	defer cleanupTempFile(t, errOut)

	code := run([]string{"--public-api-rate-limit-window", "-1s"}, out, errOut)
	if code != 2 {
		t.Fatalf("expected exit code 2, got %d", code)
	}
	errText := readTempFile(t, errOut)
	if !strings.Contains(errText, "--public-api-rate-limit-window must be positive") {
		t.Fatalf("expected validation error, got %q", errText)
	}
}

func TestRunPublicAPIRateLimitDumpConfig(t *testing.T) {
	out := newTempFile(t)
	errOut := newTempFile(t)
	defer cleanupTempFile(t, out)
	defer cleanupTempFile(t, errOut)

	code := run([]string{
		"--dump-config",
		"--public-api-rate-limit-enabled",
		"--public-api-rate-limit-max", "20",
		"--public-api-rate-limit-window", "5m",
	}, out, errOut)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
	outText := readTempFile(t, out)
	for _, want := range []string{
		`"rate_limit_enabled": true`,
		`"rate_limit_max": 20`,
		`"rate_limit_window": "5m0s"`,
	} {
		if !strings.Contains(outText, want) {
			t.Fatalf("expected %q in dump-config output, got:\n%s", want, outText)
		}
	}
}

func TestRunPublicAPIRateLimitHelpText(t *testing.T) {
	out := newTempFile(t)
	errOut := newTempFile(t)
	defer cleanupTempFile(t, out)
	defer cleanupTempFile(t, errOut)

	run([]string{"-h"}, out, errOut)
	help := readTempFile(t, errOut)
	for _, want := range []string{
		"Public API:",
		"--public-api-rate-limit-enabled",
		"--public-api-rate-limit-max",
		"--public-api-rate-limit-window",
		"GONEMASTER_PUBLIC_API_RATE_LIMIT_ENABLED",
		"GONEMASTER_PUBLIC_API_RATE_LIMIT_MAX",
		"GONEMASTER_PUBLIC_API_RATE_LIMIT_WINDOW",
	} {
		if !strings.Contains(help, want) {
			t.Fatalf("expected %q in help output, got:\n%s", want, help)
		}
	}
}

func newTempFile(t *testing.T) *os.File {
	t.Helper()
	f, err := os.CreateTemp("", "gm-server-*.log")
	if err != nil {
		t.Fatalf("temp file: %v", err)
	}
	return f
}

func cleanupTempFile(t *testing.T, f *os.File) {
	t.Helper()
	name := f.Name()
	_ = f.Close()
	_ = os.Remove(name)
}

func readTempFile(t *testing.T, f *os.File) string {
	t.Helper()
	if _, err := f.Seek(0, 0); err != nil {
		t.Fatalf("seek: %v", err)
	}
	data, err := os.ReadFile(f.Name())
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	return string(data)
}
