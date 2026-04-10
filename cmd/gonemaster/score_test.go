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

func TestScoreNoEntriesShowsHundred(t *testing.T) {
	// An empty (but non-nil) slice means the engine ran and found nothing wrong.
	// Expect a perfect score, not N/A.
	stubRunEngineWithEntries(t, []engine.LogEntry{})
	var out, errOut bytes.Buffer
	code := run([]string{"--score", "example.se"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d: %s", code, errOut.String())
	}
	output := out.String()
	if !strings.Contains(output, "Score:") {
		t.Fatalf("expected Score: line, got: %s", output)
	}
	if strings.Contains(output, "N/A") {
		t.Fatalf("expected a real score for clean run, got N/A: %s", output)
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

// ── level helpers ─────────────────────────────────────────────────────────────

func TestLowerLevel(t *testing.T) {
	cases := []struct{ a, b, want string }{
		{"NOTICE", "INFO", "INFO"},
		{"INFO", "NOTICE", "INFO"},
		{"INFO", "INFO", "INFO"},
		{"DEBUG", "NOTICE", "DEBUG"},
		{"WARNING", "ERROR", "WARNING"},
	}
	for _, tc := range cases {
		got := lowerLevel(tc.a, tc.b)
		if got != tc.want {
			t.Errorf("lowerLevel(%q,%q) = %q, want %q", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestFilterEntriesByLevel(t *testing.T) {
	entries := []engine.LogEntry{
		{Level: "INFO"},
		{Level: "NOTICE"},
		{Level: "WARNING"},
	}
	got := filterEntriesByLevel(entries, "NOTICE")
	if len(got) != 2 {
		t.Fatalf("expected 2 entries at NOTICE+, got %d", len(got))
	}
	for _, e := range got {
		if strings.EqualFold(e.Level, "INFO") {
			t.Fatal("INFO entry leaked through NOTICE filter")
		}
	}
}

func TestScoreDoesNotLeakInfoIntoHumanOutput(t *testing.T) {
	// Engine returns an INFO entry and a NOTICE entry.
	// With --score and default min-level (NOTICE), INFO must not appear in output.
	stubRunEngineWithEntries(t, []engine.LogEntry{
		{Module: "DNSSEC", Tag: "DS07_SIGNED", Level: "INFO"},
		{Module: "Zone", Tag: "REFRESH_MINIMUM_VALUE_LOWER", Level: "NOTICE"},
	})
	var out, errOut bytes.Buffer
	// Force non-streaming path by NOT using a terminal (no spinner).
	code := run([]string{"--score", "--no-progress", "example.se"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d: %s", code, errOut.String())
	}
	output := out.String()
	if strings.Contains(output, "DS07_SIGNED") {
		t.Fatalf("INFO-level tag DS07_SIGNED leaked into NOTICE output: %s", output)
	}
	if !strings.Contains(output, "Score:") {
		t.Fatalf("expected Score: in output, got: %s", output)
	}
}

func TestScoreDoesNotLeakInfoIntoJSONOutput(t *testing.T) {
	stubRunEngineWithEntries(t, []engine.LogEntry{
		{Module: "DNSSEC", Tag: "DS07_SIGNED", Level: "INFO"},
		{Module: "Zone", Tag: "REFRESH_MINIMUM_VALUE_LOWER", Level: "NOTICE"},
	})
	var out, errOut bytes.Buffer
	code := run([]string{"--json", "--score", "example.se"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d: %s", code, errOut.String())
	}
	var entries []engine.LogEntry
	if err := json.Unmarshal(out.Bytes(), &entries); err != nil {
		t.Fatalf("invalid JSON: %v — got: %s", err, out.String())
	}
	for _, e := range entries {
		if strings.EqualFold(e.Level, "INFO") {
			t.Fatalf("INFO entry leaked into --json output: %+v", e)
		}
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

func TestComputeScoreNilEntriesReturnsNil(t *testing.T) {
	// nil means no run was performed — return nil (N/A).
	r := computeScore("example.se", nil, scoring.DefaultConfig())
	if r != nil {
		t.Fatalf("expected nil result for nil entries, got %+v", r)
	}
}

func TestComputeScoreEmptyEntriesReturnsPerfect(t *testing.T) {
	// Empty (non-nil) slice means run completed with no penalised entries — should score 100.
	r := computeScore("example.se", []engine.LogEntry{}, scoring.DefaultConfig())
	if r == nil {
		t.Fatal("expected non-nil result for empty (clean) run")
	}
	if r.Score != 100 {
		t.Fatalf("expected score 100 for clean run, got %d", r.Score)
	}
}
