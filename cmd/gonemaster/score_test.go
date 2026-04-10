package main

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"codeberg.org/pawal/gonemaster/engine"
	"codeberg.org/pawal/gonemaster/scoring"
)

// sampleEntries returns a small set of log entries that exercise the scoring
// engine — enough to get a non-trivial result with DNSSEC and NAMESERVER data.
func sampleEntries() []engine.LogEntry {
	return []engine.LogEntry{
		{Module: "DNSSEC", Tag: "DS07_SIGNED", Level: "INFO"},
		{Module: "DNSSEC", Tag: "DS05_ALGO_OK", Level: "INFO"},
		{Module: "NAMESERVER", Tag: "NS01_NO_RESPONSE", Level: "WARNING"},
	}
}

func stubRunEngineWithEntries(t *testing.T, entries []engine.LogEntry) {
	t.Helper()
	previous := runEngine
	runEngine = func(req engine.RunRequest) ([]engine.LogEntry, error) {
		return entries, nil
	}
	t.Cleanup(func() { runEngine = previous })
}

// ── --score / --no-score ──────────────────────────────────────────────────────

func TestScoreNotShownByDefault(t *testing.T) {
	stubRunEngineWithEntries(t, sampleEntries())
	var out, errOut bytes.Buffer
	code := run([]string{"example.se"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d: %s", code, errOut.String())
	}
	if strings.Contains(out.String(), "Score:") || strings.Contains(errOut.String(), "Score:") {
		t.Fatalf("expected no score by default, got stdout=%q stderr=%q", out.String(), errOut.String())
	}
}

func TestScoreShownWithFlag(t *testing.T) {
	stubRunEngineWithEntries(t, sampleEntries())
	var out, errOut bytes.Buffer
	code := run([]string{"--score", "example.se"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d: %s", code, errOut.String())
	}
	if !strings.Contains(out.String(), "Score:") {
		t.Fatalf("expected Score: in output, got: %s", out.String())
	}
}

func TestNoScoreSuppressesScore(t *testing.T) {
	stubRunEngineWithEntries(t, sampleEntries())
	var out, errOut bytes.Buffer
	code := run([]string{"--score", "--no-score", "example.se"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d: %s", code, errOut.String())
	}
	if strings.Contains(out.String(), "Score:") || strings.Contains(errOut.String(), "Score:") {
		t.Fatalf("expected --no-score to suppress score, got: %s", out.String())
	}
}

func TestScoreShowsCategories(t *testing.T) {
	stubRunEngineWithEntries(t, sampleEntries())
	var out, errOut bytes.Buffer
	code := run([]string{"--score", "example.se"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d: %s", code, errOut.String())
	}
	output := out.String()
	for _, cat := range []string{"dnssec:", "nameserver_health:"} {
		if !strings.Contains(output, cat) {
			t.Errorf("expected category %q in output, got:\n%s", cat, output)
		}
	}
}

func TestScoreNoEntriesShowsNA(t *testing.T) {
	stubRunEngineWithEntries(t, nil)
	var out, errOut bytes.Buffer
	code := run([]string{"--score", "example.se"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d: %s", code, errOut.String())
	}
	if !strings.Contains(out.String(), "N/A") {
		t.Fatalf("expected N/A with no entries, got: %s", out.String())
	}
}

// ── --json mode: score goes to stderr ─────────────────────────────────────────

func TestScoreInJSONModeGoesToStderr(t *testing.T) {
	stubRunEngineWithEntries(t, sampleEntries())
	var out, errOut bytes.Buffer
	code := run([]string{"--json", "--score", "example.se"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d: %s", code, errOut.String())
	}
	// stdout must be valid JSON
	var entries []engine.LogEntry
	if err := json.Unmarshal(out.Bytes(), &entries); err != nil {
		t.Fatalf("stdout must be valid JSON: %v — got: %s", err, out.String())
	}
	// score must appear on stderr, not stdout
	if strings.Contains(out.String(), "Score:") {
		t.Fatalf("score must not appear in JSON stdout, got: %s", out.String())
	}
	if !strings.Contains(errOut.String(), "Score:") {
		t.Fatalf("expected Score: on stderr for --json mode, got: %s", errOut.String())
	}
}

func TestNoScoreNotShownInJSONModeByDefault(t *testing.T) {
	stubRunEngineWithEntries(t, sampleEntries())
	var out, errOut bytes.Buffer
	code := run([]string{"--json", "example.se"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d: %s", code, errOut.String())
	}
	if strings.Contains(out.String(), "Score:") || strings.Contains(errOut.String(), "Score:") {
		t.Fatalf("score must not appear by default in --json mode")
	}
}

// ── --raw mode ────────────────────────────────────────────────────────────────

func TestScoreInRawMode(t *testing.T) {
	stubRunEngineWithEntries(t, sampleEntries())
	var out, errOut bytes.Buffer
	code := run([]string{"--raw", "--score", "example.se"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d: %s", code, errOut.String())
	}
	if !strings.Contains(out.String(), "Score:") {
		t.Fatalf("expected Score: in stdout for --raw mode, got: %s", out.String())
	}
}

// ── --scoring-config flag ─────────────────────────────────────────────────────

func TestScoringConfigImpliesScore(t *testing.T) {
	stubRunEngineWithEntries(t, sampleEntries())

	cfg := scoring.DefaultConfig()
	data, _ := json.Marshal(cfg)
	f, err := os.CreateTemp(t.TempDir(), "scoring-*.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write(data); err != nil {
		t.Fatal(err)
	}
	f.Close()

	var out, errOut bytes.Buffer
	code := run([]string{"--scoring-config", f.Name(), "example.se"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d: %s", code, errOut.String())
	}
	if !strings.Contains(out.String(), "Score:") {
		t.Fatalf("expected --scoring-config to imply --score, got: %s", out.String())
	}
}

func TestScoringConfigNoScoreWins(t *testing.T) {
	stubRunEngineWithEntries(t, sampleEntries())

	cfg := scoring.DefaultConfig()
	data, _ := json.Marshal(cfg)
	f, err := os.CreateTemp(t.TempDir(), "scoring-*.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write(data); err != nil {
		t.Fatal(err)
	}
	f.Close()

	var out, errOut bytes.Buffer
	code := run([]string{"--scoring-config", f.Name(), "--no-score", "example.se"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d: %s", code, errOut.String())
	}
	if strings.Contains(out.String(), "Score:") || strings.Contains(errOut.String(), "Score:") {
		t.Fatalf("expected --no-score to suppress scoring-config, got stdout=%q stderr=%q", out.String(), errOut.String())
	}
}

func TestScoringConfigInvalidPath(t *testing.T) {
	var out, errOut bytes.Buffer
	code := run([]string{"--scoring-config", "/nonexistent/path.json", "example.se"}, &out, &errOut)
	if code == 0 {
		t.Fatal("expected non-zero exit for invalid --scoring-config path")
	}
}

// ── computeScore unit test ────────────────────────────────────────────────────

func TestComputeScoreConvertsEntries(t *testing.T) {
	entries := []engine.LogEntry{
		{Module: "DNSSEC", Tag: "DS07_SIGNED", Level: "INFO"},
	}
	r := computeScore("example.se", entries, scoring.DefaultConfig())
	if r == nil {
		t.Fatal("expected non-nil result")
	}
	if r.Grade == "" {
		t.Fatal("expected non-empty grade")
	}
}

func TestComputeScoreEmptyEntries(t *testing.T) {
	r := computeScore("example.se", nil, scoring.DefaultConfig())
	if r != nil {
		t.Fatalf("expected nil result for empty entries, got %+v", r)
	}
}
