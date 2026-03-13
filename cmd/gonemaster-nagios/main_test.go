package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"codeberg.org/pawal/gonemaster/engine"
)

func stubRunEngineFunc(t *testing.T, fn func(engine.RunRequest) ([]engine.LogEntry, error)) {
	t.Helper()
	previous := runEngine
	runEngine = fn
	t.Cleanup(func() {
		runEngine = previous
	})
}

func stubRunEngine(t *testing.T, captured *engine.RunRequest) {
	t.Helper()
	stubRunEngineFunc(t, func(req engine.RunRequest) ([]engine.LogEntry, error) {
		if captured != nil {
			*captured = req
		}
		return nil, nil
	})
}

func TestRunHelp(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer

	code := run([]string{"--help"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}

	help := errOut.String()
	for _, fragment := range []string{
		"Usage:",
		"-H, --hostname",
		"-w, --warning",
		"-c, --critical",
		"-t, --timeout",
		"--force-ipv6",
		"--source-addr4",
		"--sourceaddr4",
	} {
		if !strings.Contains(help, fragment) {
			t.Fatalf("expected %q in usage output", fragment)
		}
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
	if !strings.Contains(out.String(), "Miekg DNS version") {
		t.Fatalf("expected miekg version output")
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
	level := maxLevel(entries)
	if level != "WARNING" {
		t.Fatalf("expected WARNING, got %s", level)
	}
}

func TestStatusForLevelHonorsCustomThresholds(t *testing.T) {
	thresholds, err := parseSeverityThresholds("NOTICE", "CRITICAL")
	if err != nil {
		t.Fatalf("unexpected threshold error: %v", err)
	}

	status := statusForLevel("ERROR", thresholds)
	if status.code != 1 || status.text != "WARNING" {
		t.Fatalf("expected ERROR to map to WARNING, got %#v", status)
	}
}

func TestRunParsesPreferredAliases(t *testing.T) {
	var captured engine.RunRequest
	stubRunEngine(t, &captured)

	var out bytes.Buffer
	var errOut bytes.Buffer
	code := run([]string{
		"-H", "example.com",
		"--source-addr4", "192.0.2.50",
		"--source-addr6", "2001:db8::50",
		"--force-ipv6",
	}, &out, &errOut)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d (stderr=%q)", code, errOut.String())
	}
	if captured.Domain != "example.com" {
		t.Fatalf("unexpected domain: %q", captured.Domain)
	}
	if captured.SourceAddr4 == nil || *captured.SourceAddr4 != "192.0.2.50" {
		t.Fatalf("unexpected SourceAddr4 override: %#v", captured.SourceAddr4)
	}
	if captured.SourceAddr6 == nil || *captured.SourceAddr6 != "2001:db8::50" {
		t.Fatalf("unexpected SourceAddr6 override: %#v", captured.SourceAddr6)
	}
	if captured.IPv6 == nil || !*captured.IPv6 {
		t.Fatalf("expected IPv6 override to be enabled, got %#v", captured.IPv6)
	}
}

func TestRunParsesLegacySourceAddrAliases(t *testing.T) {
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
		"--source-addr4", "not-an-ip",
	}, &out, &errOut)
	if code != 3 {
		t.Fatalf("expected exit code 3, got %d", code)
	}
	if !strings.Contains(errOut.String(), "--source-addr4 must be a valid IPv4 address") {
		t.Fatalf("expected source-addr4 validation error, got %q", errOut.String())
	}
}

func TestRunRejectsInvalidSourceAddr6(t *testing.T) {
	stubRunEngine(t, nil)

	var out bytes.Buffer
	var errOut bytes.Buffer
	code := run([]string{
		"--domain", "example.com",
		"--source-addr6", "192.0.2.5",
	}, &out, &errOut)
	if code != 3 {
		t.Fatalf("expected exit code 3, got %d", code)
	}
	if !strings.Contains(errOut.String(), "--source-addr6 must be a valid IPv6 address") {
		t.Fatalf("expected source-addr6 validation error, got %q", errOut.String())
	}
}

func TestRunRejectsInvalidWarningLevel(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer

	code := run([]string{"--domain", "example.com", "--warning", "bogus"}, &out, &errOut)
	if code != 3 {
		t.Fatalf("expected exit code 3, got %d", code)
	}
	if !strings.Contains(errOut.String(), "--warning must be one of") {
		t.Fatalf("expected warning validation error, got %q", errOut.String())
	}
}

func TestRunRejectsInvalidThresholdOrdering(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer

	code := run([]string{"--domain", "example.com", "--warning", "ERROR", "--critical", "WARNING"}, &out, &errOut)
	if code != 3 {
		t.Fatalf("expected exit code 3, got %d", code)
	}
	if !strings.Contains(errOut.String(), "--warning must be lower severity than --critical") {
		t.Fatalf("expected threshold ordering error, got %q", errOut.String())
	}
}

func TestRunRejectsInvalidTimeout(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer

	code := run([]string{"--domain", "example.com", "--timeout", "0"}, &out, &errOut)
	if code != 3 {
		t.Fatalf("expected exit code 3, got %d", code)
	}
	if !strings.Contains(errOut.String(), "--timeout must be >= 1") {
		t.Fatalf("expected timeout validation error, got %q", errOut.String())
	}
}

func TestRunAppliesCustomThresholds(t *testing.T) {
	stubRunEngineFunc(t, func(req engine.RunRequest) ([]engine.LogEntry, error) {
		return []engine.LogEntry{{Level: "ERROR"}}, nil
	})

	var out bytes.Buffer
	var errOut bytes.Buffer
	code := run([]string{
		"--domain", "example.com",
		"--warning", "WARNING",
		"--critical", "CRITICAL",
	}, &out, &errOut)
	if code != 1 {
		t.Fatalf("expected exit code 1, got %d (stderr=%q)", code, errOut.String())
	}
	if !strings.Contains(out.String(), "ZONE WARNING") {
		t.Fatalf("expected warning output, got %q", out.String())
	}
}

func TestRunTimeoutReturnsUnknown(t *testing.T) {
	stubRunEngineFunc(t, func(req engine.RunRequest) ([]engine.LogEntry, error) {
		deadline, ok := req.Context.Deadline()
		if !ok {
			t.Fatalf("expected timeout context deadline")
		}
		if time.Until(deadline) <= 0 {
			t.Fatalf("expected a future deadline, got %v", deadline)
		}
		return nil, context.DeadlineExceeded
	})

	var out bytes.Buffer
	var errOut bytes.Buffer
	code := run([]string{"-H", "example.com", "-t", "1"}, &out, &errOut)
	if code != 3 {
		t.Fatalf("expected exit code 3, got %d (stderr=%q)", code, errOut.String())
	}
	if !strings.Contains(out.String(), "ZONE UNKNOWN - plugin timed out after 1s") {
		t.Fatalf("expected timeout output, got %q", out.String())
	}
}
