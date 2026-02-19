package main

import (
	"bytes"
	"strings"
	"testing"

	"codeberg.org/pawal/gonemaster/engine"
)

func stubRunEngine(t *testing.T, captured *engine.RunRequest) {
	t.Helper()
	previous := runEngine
	runEngine = func(req engine.RunRequest) ([]engine.LogEntry, error) {
		if captured != nil {
			*captured = req
		}
		return nil, nil
	}
	t.Cleanup(func() {
		runEngine = previous
	})
}

func TestRunHelp(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer

	code := run([]string{"--help"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
	if !strings.Contains(errOut.String(), "Usage:") {
		t.Fatalf("expected usage output")
	}
	if !strings.Contains(errOut.String(), "--sourceaddr4") {
		t.Fatalf("expected sourceaddr4 in usage output")
	}
	if !strings.Contains(errOut.String(), "--sourceaddr6") {
		t.Fatalf("expected sourceaddr6 in usage output")
	}
}

func TestRunVersion(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer

	code := run([]string{"--version"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
	if !strings.Contains(out.String(), "Gonemaster version") {
		t.Fatalf("expected version output")
	}
}

func TestRunMissingDomain(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer

	code := run([]string{}, &out, &errOut)
	if code != 3 {
		t.Fatalf("expected exit code 3, got %d", code)
	}
	if !strings.Contains(errOut.String(), "Usage:") {
		t.Fatalf("expected usage output")
	}
}

func TestExpandVerboseArgs(t *testing.T) {
	args := expandVerboseArgs([]string{"-vvv", "--domain", "example.com"})
	if len(args) != 5 {
		t.Fatalf("expected 5 args, got %d", len(args))
	}
	if args[0] != "-v" || args[1] != "-v" || args[2] != "-v" {
		t.Fatalf("expected expanded -v flags, got %v", args[:3])
	}
}

func TestMaxLevel(t *testing.T) {
	entries := []engine.LogEntry{
		{Level: "NOTICE"},
		{Level: "WARNING"},
	}
	level, status := maxLevel(entries)
	if level != "WARNING" {
		t.Fatalf("expected WARNING, got %s", level)
	}
	if status.code != 1 {
		t.Fatalf("expected status code 1, got %d", status.code)
	}
}

func TestRunParsesSourceAddrOverrides(t *testing.T) {
	var captured engine.RunRequest
	stubRunEngine(t, &captured)

	var out bytes.Buffer
	var errOut bytes.Buffer
	code := run([]string{
		"--domain", "example.com",
		"--sourceaddr4", "192.0.2.50",
		"--sourceaddr6", "2001:db8::50",
	}, &out, &errOut)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d (stderr=%q)", code, errOut.String())
	}
	if captured.SourceAddr4 == nil || *captured.SourceAddr4 != "192.0.2.50" {
		t.Fatalf("unexpected SourceAddr4 override: %#v", captured.SourceAddr4)
	}
	if captured.SourceAddr6 == nil || *captured.SourceAddr6 != "2001:db8::50" {
		t.Fatalf("unexpected SourceAddr6 override: %#v", captured.SourceAddr6)
	}
}

func TestRunRejectsInvalidSourceAddr4(t *testing.T) {
	stubRunEngine(t, nil)

	var out bytes.Buffer
	var errOut bytes.Buffer
	code := run([]string{
		"--domain", "example.com",
		"--sourceaddr4", "not-an-ip",
	}, &out, &errOut)
	if code != 3 {
		t.Fatalf("expected exit code 3, got %d", code)
	}
	if !strings.Contains(errOut.String(), "--sourceaddr4 must be a valid IPv4 address") {
		t.Fatalf("expected sourceaddr4 validation error, got %q", errOut.String())
	}
}

func TestRunRejectsInvalidSourceAddr6(t *testing.T) {
	stubRunEngine(t, nil)

	var out bytes.Buffer
	var errOut bytes.Buffer
	code := run([]string{
		"--domain", "example.com",
		"--sourceaddr6", "192.0.2.5",
	}, &out, &errOut)
	if code != 3 {
		t.Fatalf("expected exit code 3, got %d", code)
	}
	if !strings.Contains(errOut.String(), "--sourceaddr6 must be a valid IPv6 address") {
		t.Fatalf("expected sourceaddr6 validation error, got %q", errOut.String())
	}
}
