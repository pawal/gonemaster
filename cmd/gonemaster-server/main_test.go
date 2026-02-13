package main

import (
	"os"
	"strings"
	"testing"

	"codeberg.org/pawal/gonemaster/server"
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

func TestRunJobTestParallelismValidation(t *testing.T) {
	out := newTempFile(t)
	errOut := newTempFile(t)
	defer cleanupTempFile(t, out)
	defer cleanupTempFile(t, errOut)

	code := run([]string{"--job-test-parallelism", "0"}, out, errOut)
	if code != 2 {
		t.Fatalf("expected exit code 2, got %d", code)
	}

	errText := readTempFile(t, errOut)
	if !strings.Contains(errText, "--job-test-parallelism must be >= 1") {
		t.Fatalf("expected job-test-parallelism validation error, got %q", errText)
	}
}

func TestRunCrossJobHotCacheTTLValidation(t *testing.T) {
	out := newTempFile(t)
	errOut := newTempFile(t)
	defer cleanupTempFile(t, out)
	defer cleanupTempFile(t, errOut)

	code := run([]string{"--cross-job-hot-cache-ttl-seconds", "0"}, out, errOut)
	if code != 2 {
		t.Fatalf("expected exit code 2, got %d", code)
	}

	errText := readTempFile(t, errOut)
	if !strings.Contains(errText, "--cross-job-hot-cache-ttl-seconds must be >= 1") {
		t.Fatalf("expected cross-job-hot-cache-ttl-seconds validation error, got %q", errText)
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

func TestFormatConcurrencySummary(t *testing.T) {
	tests := []struct {
		name string
		cfg  server.Config
		want string
	}{
		{
			name: "defaults",
			cfg:  server.DefaultConfig(),
			want: "workers=4, max-concurrent-jobs=unlimited, max-in-flight-engine-runs=4, job-test-parallelism=1",
		},
		{
			name: "engine limiter below worker count",
			cfg: server.Config{
				WorkerCount:        12,
				MaxConcurrentJobs:  5,
				JobTestParallelism: 4,
			},
			want: "workers=12, max-concurrent-jobs=5, max-in-flight-engine-runs=5, job-test-parallelism=4",
		},
		{
			name: "clamped minimums",
			cfg: server.Config{
				WorkerCount:        0,
				MaxConcurrentJobs:  0,
				JobTestParallelism: 0,
			},
			want: "workers=1, max-concurrent-jobs=unlimited, max-in-flight-engine-runs=1, job-test-parallelism=1",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := formatConcurrencySummary(tc.cfg); got != tc.want {
				t.Fatalf("formatConcurrencySummary() = %q, want %q", got, tc.want)
			}
		})
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
