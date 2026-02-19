package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
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

func TestRunRequiresDomain(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer

	code := run([]string{}, &out, &errOut)
	if code != 2 {
		t.Fatalf("expected exit code 2, got %d", code)
	}
	if !strings.Contains(errOut.String(), "--domain is required") {
		t.Fatalf("expected missing domain message, got %q", errOut.String())
	}
}

func TestRunHelpShowsGroupedFlags(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer

	code := run([]string{"-h"}, &out, &errOut)
	if code != 2 {
		t.Fatalf("expected exit code 2, got %d", code)
	}
	help := errOut.String()
	expected := []string{
		"Usage: gonemaster [flags]",
		"Flags:",
		"Target:",
		"Output:",
		"Resolver/Profile Overrides:",
		"Undelegated:",
		"Utility:",
		"--domain DOMAIN",
		"--count",
	}
	for _, fragment := range expected {
		if !strings.Contains(help, fragment) {
			t.Fatalf("expected %q in help output, got %q", fragment, help)
		}
	}
	if out.Len() != 0 {
		t.Fatalf("expected no stdout output, got %q", out.String())
	}
}

func TestRunDumpProfileWithoutDomain(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer

	code := run([]string{"--dump-profile"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
	var payload map[string]any
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("expected JSON output, got %q (err=%v)", out.String(), err)
	}
	if _, ok := payload["net"]; !ok {
		t.Fatalf("expected net in profile output, got %q", out.String())
	}
	if errOut.Len() != 0 {
		t.Fatalf("expected no stderr output, got %q", errOut.String())
	}
}

func TestRunWritesJSONAndError(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer

	code := run([]string{"--domain", ".", "--testcase", "basic01", "--min-level", "INFO", "--json"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
	if !strings.Contains(out.String(), "B01_ROOT_HAS_NO_PARENT") {
		t.Fatalf("expected basic01 output, got %q", out.String())
	}
	if errOut.Len() != 0 {
		t.Fatalf("expected no stderr output, got %q", errOut.String())
	}
}

func TestRunWritesOutputFile(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "out.json")

	var out bytes.Buffer
	var errOut bytes.Buffer

	code := run([]string{"--domain", ".", "--testcase", "basic01", "--min-level", "INFO", "--json", "--output", target}, &out, &errOut)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	if !strings.Contains(string(data), "B01_ROOT_HAS_NO_PARENT") {
		t.Fatalf("unexpected file output: %q", string(data))
	}
}

func TestRunRawStreamsOutput(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer

	code := run([]string{"--domain", ".", "--testcase", "basic01", "--min-level", "INFO", "--raw"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
	if !strings.Contains(out.String(), "B01_ROOT_HAS_NO_PARENT") {
		t.Fatalf("expected raw output, got %q", out.String())
	}
	if strings.Contains(out.String(), "\"timestamp\"") {
		t.Fatalf("expected raw output, got %q", out.String())
	}
	if errOut.Len() != 0 {
		t.Fatalf("expected no stderr output, got %q", errOut.String())
	}
}

func TestRunJSONStreamOutputs(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer

	code := run([]string{"--domain", ".", "--testcase", "basic01", "--min-level", "INFO", "--json-stream"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
	lines := strings.Split(out.String(), "\n")
	var first string
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		first = line
		break
	}
	if first == "" {
		t.Fatalf("expected json-stream output, got %q", out.String())
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(first), &payload); err != nil {
		t.Fatalf("expected JSON line, got %q (err=%v)", first, err)
	}
	if tag, ok := payload["Tag"].(string); !ok || !strings.Contains(tag, "B01_") {
		t.Fatalf("expected Tag in JSON output, got %v", payload["Tag"])
	}
	if errOut.Len() != 0 {
		t.Fatalf("expected no stderr output, got %q", errOut.String())
	}
}

func TestRunRejectsRawAndJSON(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer

	code := run([]string{"--domain", ".", "--json", "--raw"}, &out, &errOut)
	if code != 2 {
		t.Fatalf("expected exit code 2, got %d", code)
	}
	if !strings.Contains(errOut.String(), "--json cannot be combined with --raw") {
		t.Fatalf("expected error message, got %q", errOut.String())
	}
	if out.Len() != 0 {
		t.Fatalf("expected no stdout output, got %q", out.String())
	}
}

func TestRunRejectsJSONStreamAndRaw(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer

	code := run([]string{"--domain", ".", "--json-stream", "--raw"}, &out, &errOut)
	if code != 2 {
		t.Fatalf("expected exit code 2, got %d", code)
	}
	if !strings.Contains(errOut.String(), "--json-stream cannot be combined with --raw") {
		t.Fatalf("expected error message, got %q", errOut.String())
	}
	if out.Len() != 0 {
		t.Fatalf("expected no stdout output, got %q", out.String())
	}
}

func TestRunRejectsJSONStreamAndJSON(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer

	code := run([]string{"--domain", ".", "--json-stream", "--json"}, &out, &errOut)
	if code != 2 {
		t.Fatalf("expected exit code 2, got %d", code)
	}
	if !strings.Contains(errOut.String(), "--json-stream cannot be combined with --json") {
		t.Fatalf("expected error message, got %q", errOut.String())
	}
	if out.Len() != 0 {
		t.Fatalf("expected no stdout output, got %q", out.String())
	}
}

func TestRunRejectsCountAndRaw(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer

	code := run([]string{"--domain", ".", "--count", "--raw"}, &out, &errOut)
	if code != 2 {
		t.Fatalf("expected exit code 2, got %d", code)
	}
	if !strings.Contains(errOut.String(), "--count cannot be combined with --raw") {
		t.Fatalf("expected error message, got %q", errOut.String())
	}
	if out.Len() != 0 {
		t.Fatalf("expected no stdout output, got %q", out.String())
	}
}

func TestRunRejectsCountAndJSON(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer

	code := run([]string{"--domain", ".", "--count", "--json"}, &out, &errOut)
	if code != 2 {
		t.Fatalf("expected exit code 2, got %d", code)
	}
	if !strings.Contains(errOut.String(), "--count cannot be combined with --json") {
		t.Fatalf("expected error message, got %q", errOut.String())
	}
	if out.Len() != 0 {
		t.Fatalf("expected no stdout output, got %q", out.String())
	}
}

func TestRunRejectsCountAndJSONStream(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer

	code := run([]string{"--domain", ".", "--count", "--json-stream"}, &out, &errOut)
	if code != 2 {
		t.Fatalf("expected exit code 2, got %d", code)
	}
	if !strings.Contains(errOut.String(), "--count cannot be combined with --json-stream") {
		t.Fatalf("expected error message, got %q", errOut.String())
	}
	if out.Len() != 0 {
		t.Fatalf("expected no stdout output, got %q", out.String())
	}
}

func TestRunRejectsCountAndDumpProfile(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer

	code := run([]string{"--count", "--dump-profile"}, &out, &errOut)
	if code != 2 {
		t.Fatalf("expected exit code 2, got %d", code)
	}
	if !strings.Contains(errOut.String(), "--dump-profile cannot be combined with --count") {
		t.Fatalf("expected error message, got %q", errOut.String())
	}
	if out.Len() != 0 {
		t.Fatalf("expected no stdout output, got %q", out.String())
	}
}

func TestRunParsesUndelegatedNameserverFlags(t *testing.T) {
	var captured engine.RunRequest
	stubRunEngine(t, &captured)

	var out bytes.Buffer
	var errOut bytes.Buffer

	code := run([]string{
		"--domain", "example.com",
		"--json",
		"--ns", "NS1.Example.com/192.0.2.1",
		"--ns", "ns2.example.net",
	}, &out, &errOut)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d (stderr=%q)", code, errOut.String())
	}
	if len(captured.UndelegatedNameservers) != 2 {
		t.Fatalf("expected 2 nameserver rows, got %d", len(captured.UndelegatedNameservers))
	}
	first := captured.UndelegatedNameservers[0]
	second := captured.UndelegatedNameservers[1]
	if first.Name != "ns1.example.com" || first.IP != "192.0.2.1" {
		t.Fatalf("unexpected first nameserver: %+v", first)
	}
	if second.Name != "ns2.example.net" || second.IP != "" {
		t.Fatalf("unexpected second nameserver: %+v", second)
	}
}

func TestRunParsesUndelegatedNameserverSameNameMultipleIPs(t *testing.T) {
	var captured engine.RunRequest
	stubRunEngine(t, &captured)

	var out bytes.Buffer
	var errOut bytes.Buffer

	code := run([]string{
		"--domain", "example.com",
		"--json",
		"--ns", "NS1.Example.com/192.0.2.1",
		"--ns", "ns1.example.com/2001:db8::1",
	}, &out, &errOut)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d (stderr=%q)", code, errOut.String())
	}
	if len(captured.UndelegatedNameservers) != 2 {
		t.Fatalf("expected 2 nameserver rows, got %d", len(captured.UndelegatedNameservers))
	}
	first := captured.UndelegatedNameservers[0]
	second := captured.UndelegatedNameservers[1]
	if first.Name != "ns1.example.com" || first.IP != "192.0.2.1" {
		t.Fatalf("unexpected first nameserver: %+v", first)
	}
	if second.Name != "ns1.example.com" || second.IP != "2001:db8::1" {
		t.Fatalf("unexpected second nameserver: %+v", second)
	}
}

func TestRunParsesUndelegatedDSFlags(t *testing.T) {
	var captured engine.RunRequest
	stubRunEngine(t, &captured)

	var out bytes.Buffer
	var errOut bytes.Buffer

	code := run([]string{
		"--domain", "example.com",
		"--json",
		"--ds", "12345,13,2," + strings.Repeat("a", 64),
		"--ds", "23456,8,2," + strings.Repeat("B", 64),
	}, &out, &errOut)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d (stderr=%q)", code, errOut.String())
	}
	if len(captured.UndelegatedDSInfo) != 2 {
		t.Fatalf("expected 2 DS rows, got %d", len(captured.UndelegatedDSInfo))
	}
	first := captured.UndelegatedDSInfo[0]
	second := captured.UndelegatedDSInfo[1]
	if first.KeyTag != 12345 || first.Algorithm != 13 || first.DigestType != 2 || first.Digest != strings.Repeat("A", 64) {
		t.Fatalf("unexpected first DS row: %+v", first)
	}
	if second.KeyTag != 23456 || second.Algorithm != 8 || second.DigestType != 2 || second.Digest != strings.Repeat("B", 64) {
		t.Fatalf("unexpected second DS row: %+v", second)
	}
}

func TestRunRejectsMalformedUndelegatedNameserverFlag(t *testing.T) {
	stubRunEngine(t, nil)

	var out bytes.Buffer
	var errOut bytes.Buffer

	code := run([]string{
		"--domain", "example.com",
		"--json",
		"--ns", "bad!name.example/192.0.2.1",
	}, &out, &errOut)
	if code != 2 {
		t.Fatalf("expected exit code 2, got %d", code)
	}
	if !strings.Contains(errOut.String(), "undelegated nameserver") {
		t.Fatalf("expected undelegated nameserver parse error, got %q", errOut.String())
	}
}

func TestRunRejectsMalformedUndelegatedDSFlag(t *testing.T) {
	stubRunEngine(t, nil)

	var out bytes.Buffer
	var errOut bytes.Buffer

	code := run([]string{
		"--domain", "example.com",
		"--json",
		"--ds", "12345,13,2,NOT-HEX",
	}, &out, &errOut)
	if code != 2 {
		t.Fatalf("expected exit code 2, got %d", code)
	}
	if !strings.Contains(errOut.String(), "undelegated DS") {
		t.Fatalf("expected undelegated DS parse error, got %q", errOut.String())
	}
}

func TestRunCarriesUndelegatedInputsInRunRequest(t *testing.T) {
	var captured engine.RunRequest
	stubRunEngine(t, &captured)

	var out bytes.Buffer
	var errOut bytes.Buffer

	code := run([]string{
		"--domain", "example.com",
		"--json",
		"--ns", "ns1.example.com/192.0.2.10",
		"--ds", "12345,13,2," + strings.Repeat("a", 64),
	}, &out, &errOut)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d (stderr=%q)", code, errOut.String())
	}
	if len(captured.UndelegatedNameservers) != 1 {
		t.Fatalf("expected one undelegated nameserver row, got %d", len(captured.UndelegatedNameservers))
	}
	if len(captured.UndelegatedDSInfo) != 1 {
		t.Fatalf("expected one undelegated DS row, got %d", len(captured.UndelegatedDSInfo))
	}
	if captured.UndelegatedNameservers[0].Name != "ns1.example.com" || captured.UndelegatedNameservers[0].IP != "192.0.2.10" {
		t.Fatalf("unexpected undelegated nameserver in request: %+v", captured.UndelegatedNameservers[0])
	}
	if captured.UndelegatedDSInfo[0].KeyTag != 12345 {
		t.Fatalf("unexpected undelegated DS in request: %+v", captured.UndelegatedDSInfo[0])
	}
}

func TestRunOutputsTranslatedByDefault(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer

	code := run([]string{"--domain", ".", "--testcase", "basic01", "--min-level", "INFO", "--locale", "en"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
	if !strings.HasPrefix(out.String(), "Seconds Level    Message") {
		t.Fatalf("expected translated header, got %q", out.String())
	}
	if strings.Contains(out.String(), "\"timestamp\"") {
		t.Fatalf("expected translated output, got %q", out.String())
	}
	if !strings.Contains(out.String(), "test of the root zone") {
		t.Fatalf("expected translated output, got %q", out.String())
	}
	if errOut.Len() != 0 {
		t.Fatalf("expected no stderr output, got %q", errOut.String())
	}
}

func TestRunOutputsLooksOKWhenNoEntriesAtLevel(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer

	code := run([]string{"--domain", ".", "--testcase", "basic01", "--min-level", "CRITICAL", "--locale", "en"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
	if !strings.HasPrefix(out.String(), "Seconds Level    Message") {
		t.Fatalf("expected translated header, got %q", out.String())
	}
	if !strings.Contains(out.String(), "Looks OK.") {
		t.Fatalf("expected Looks OK when no entries match level, got %q", out.String())
	}
	if errOut.Len() != 0 {
		t.Fatalf("expected no stderr output, got %q", errOut.String())
	}
}

func TestRunCountPrintsSummariesFromAllLevels(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer

	code := run([]string{"--domain", ".", "--testcase", "basic01", "--count", "--locale", "en", "--no-progress"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
	if !strings.HasPrefix(out.String(), "Seconds Level    Message") {
		t.Fatalf("expected translated header, got %q", out.String())
	}
	if !strings.Contains(out.String(), "Looks OK.") {
		t.Fatalf("expected Looks OK marker when min-level hides entries, got %q", out.String())
	}
	if !strings.Contains(out.String(), "Number of log entries") {
		t.Fatalf("expected level count summary, got %q", out.String())
	}
	if !strings.Contains(out.String(), "Message tag") {
		t.Fatalf("expected tag count summary, got %q", out.String())
	}
	if !strings.Contains(out.String(), "INFO") {
		t.Fatalf("expected INFO counts from entries below default min-level, got %q", out.String())
	}
	if !strings.Contains(out.String(), "B01_ROOT_HAS_NO_PARENT") {
		t.Fatalf("expected B01 tag count in summary, got %q", out.String())
	}
	if errOut.Len() != 0 {
		t.Fatalf("expected no stderr output, got %q", errOut.String())
	}
}

func TestRunJSONNoEntriesRemainsJSONArray(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer

	code := run([]string{"--domain", ".", "--testcase", "basic01", "--min-level", "CRITICAL", "--json"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
	if strings.Contains(out.String(), "Looks OK.") {
		t.Fatalf("expected JSON output without Looks OK marker, got %q", out.String())
	}
	if strings.TrimSpace(out.String()) != "[]" {
		t.Fatalf("expected empty JSON array, got %q", out.String())
	}
	if errOut.Len() != 0 {
		t.Fatalf("expected no stderr output, got %q", errOut.String())
	}
}

func TestRunRawNoEntriesRemainsEmpty(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer

	code := run([]string{"--domain", ".", "--testcase", "basic01", "--min-level", "CRITICAL", "--raw"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
	if strings.TrimSpace(out.String()) != "" {
		t.Fatalf("expected empty raw stream, got %q", out.String())
	}
	if errOut.Len() != 0 {
		t.Fatalf("expected no stderr output, got %q", errOut.String())
	}
}

func TestRunJSONStreamNoEntriesRemainsEmpty(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer

	code := run([]string{"--domain", ".", "--testcase", "basic01", "--min-level", "CRITICAL", "--json-stream"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
	if strings.TrimSpace(out.String()) != "" {
		t.Fatalf("expected empty json-stream output, got %q", out.String())
	}
	if errOut.Len() != 0 {
		t.Fatalf("expected no stderr output, got %q", errOut.String())
	}
}

func TestRunListTests(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer

	code := run([]string{"--list-tests"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
	expected := []string{
		"Basic:Basic01",
		"Syntax:Syntax01",
		"Address:Address01",
		"Connectivity:Connectivity01",
		"Consistency:Consistency01",
		"DNSSEC:DNSSEC01",
		"Delegation:Delegation01",
		"Nameserver:Nameserver01",
		"Zone:Zone01",
	}
	for _, want := range expected {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("expected %q in list-tests output, got %q", want, out.String())
		}
	}
	if errOut.Len() != 0 {
		t.Fatalf("expected no stderr output, got %q", errOut.String())
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
		t.Fatalf("expected version output, got %q", out.String())
	}
	if !strings.Contains(out.String(), engine.VersionString()) {
		t.Fatalf("expected version output, got %q", out.String())
	}
	if !strings.Contains(out.String(), "Miekg DNS version") {
		t.Fatalf("expected dependency version output, got %q", out.String())
	}
	if errOut.Len() != 0 {
		t.Fatalf("expected no stderr output, got %q", errOut.String())
	}
}
