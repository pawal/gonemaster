package main

import (
	"bytes"
	"strings"
	"testing"

	"codeberg.org/pawal/gonemaster/engine"
)

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
