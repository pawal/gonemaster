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
		"--workers N",
		"--version",
		"--min-level LEVEL",
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
