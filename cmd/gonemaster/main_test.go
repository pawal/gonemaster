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

func TestRunRejectsInvalidJobTestParallelism(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer

	code := run([]string{"--domain", "example.com", "--job-test-parallelism", "0"}, &out, &errOut)
	if code != 2 {
		t.Fatalf("expected exit code 2, got %d", code)
	}
	if !strings.Contains(errOut.String(), "--job-test-parallelism must be >= 1") {
		t.Fatalf("expected error message, got %q", errOut.String())
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
