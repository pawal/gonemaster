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
		"Flags (CLI flags override --config values):",
		"General:",
		"Concurrency:",
		"Resolver/Profile:",
		"Output:",
		"--workers N",
		"--min-level LEVEL",
	}
	for _, fragment := range expected {
		if !strings.Contains(help, fragment) {
			t.Fatalf("expected %q in help output, got %q", fragment, help)
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
