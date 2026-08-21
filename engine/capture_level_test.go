package engine

import (
	"fmt"
	"testing"

	"codeberg.org/pawal/gonemaster/engine/logger"
)

// captureRun executes an offline run and returns both the converted result and
// the entries the logger held on to, so a test can tell "not returned" apart
// from "never captured".
func captureRun(t *testing.T, minLevel string, captureMinLevel string) ([]LogEntry, []*logger.Entry, error) {
	t.Helper()

	runner := newTestRunner(t, withRunLimits(4))
	// Offline: no traffic leaves the test and every query fails identically.
	runner.Profile.NoNetwork = true
	log := runner.Logger
	req := RunRequest{
		Domain:          "example.com",
		Testcases:       []string{"syntax01", "basic01"},
		MinLevel:        minLevel,
		CaptureMinLevel: captureMinLevel,
	}
	entries, err := RunWithRunner(req, runner)
	return entries, log.Entries(), err
}

func resultSignature(entries []LogEntry) []string {
	out := make([]string, 0, len(entries))
	for _, entry := range entries {
		out = append(out, fmt.Sprintf("%s/%s/%s/%s", entry.Module, entry.Testcase, entry.Tag, entry.Level))
	}
	return out
}

func requireSameResults(t *testing.T, got []LogEntry, want []LogEntry) {
	t.Helper()
	gotSig := resultSignature(got)
	wantSig := resultSignature(want)
	if len(gotSig) != len(wantSig) {
		t.Fatalf("returned %d entries, want %d\n got: %v\nwant: %v", len(gotSig), len(wantSig), gotSig, wantSig)
	}
	for i := range wantSig {
		if gotSig[i] != wantSig[i] {
			t.Fatalf("result diverged at %d: %q vs %q", i, gotSig[i], wantSig[i])
		}
	}
}

// The capture level is an optimization, not a behaviour change: a run that
// captures only what it will return has to return exactly what it did before.
func TestCaptureMinLevelKeepsResultsIdentical(t *testing.T) {
	baseline, baselineRetained, err := captureRun(t, "INFO", "")
	if err != nil {
		t.Fatalf("baseline run: %v", err)
	}
	gated, gatedRetained, err := captureRun(t, "INFO", "INFO")
	if err != nil {
		t.Fatalf("gated run: %v", err)
	}

	requireSameResults(t, gated, baseline)

	if len(baseline) == 0 {
		t.Fatal("expected the run to return entries")
	}
	// The point of the gate: the ungated run hoards entries it will never
	// return, the gated one does not.
	if len(gatedRetained) >= len(baselineRetained) {
		t.Fatalf("gated run retained %d entries, ungated retained %d; expected fewer", len(gatedRetained), len(baselineRetained))
	}
	for _, entry := range gatedRetained {
		if entry.NumericLevel() < logger.Levels()["INFO"] {
			t.Fatalf("gated run retained a sub-floor entry: %s/%s at %s", entry.Module, entry.Tag, entry.Level())
		}
	}
}

// A capture level above the run's min level would drop entries the caller asked
// for, so the engine clamps it. Requesting DEBUG output must win over a server
// default that only wants DEBUG-and-above captured.
func TestCaptureMinLevelIsClampedToMinLevel(t *testing.T) {
	baseline, _, err := captureRun(t, "DEBUG", "")
	if err != nil {
		t.Fatalf("baseline run: %v", err)
	}
	clamped, _, err := captureRun(t, "DEBUG", "WARNING")
	if err != nil {
		t.Fatalf("clamped run: %v", err)
	}

	requireSameResults(t, clamped, baseline)

	debugSeen := false
	for _, entry := range clamped {
		if entry.Level == "DEBUG" {
			debugSeen = true
			break
		}
	}
	if !debugSeen {
		t.Fatal("a DEBUG run must still return DEBUG entries when a higher capture level was requested")
	}
}

func TestCaptureMinLevelRejectsUnknownLevel(t *testing.T) {
	if _, _, err := captureRun(t, "INFO", "VERBOSE"); err == nil {
		t.Fatal("an unknown capture min level should fail the run")
	}
}
