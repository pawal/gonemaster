package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"codeberg.org/pawal/gonemaster/cmd/internal/clitest"
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
	res := clitest.Run(t, run, "--help")
	res.RequireCode(t, 0)

	help := res.Err
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
	res := clitest.Run(t, run, "--version")
	res.RequireCode(t, 0)
	res.RequireOutContains(t, "Gonemaster version")
	res.RequireOutContains(t, "Miekg DNS version")
}

func TestRunMissingDomain(t *testing.T) {
	res := clitest.Run(t, run)
	res.RequireCode(t, 3)
	res.RequireErrContains(t, "Usage:")
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

// TestRunVerboseSanitizesAttackerControlChars feeds the nagios verbose
// output a log entry whose args carry ANSI escapes, NUL, CR and the
// Nagios pipe separator. None of these may reach stdout verbatim:
//   - ANSI / NUL / CR would allow terminal hijack in nagios web UIs and
//     interactive runs;
//   - a raw pipe character would corrupt Nagios perfdata parsing.
func TestRunVerboseSanitizesAttackerControlChars(t *testing.T) {
	stubRunEngineFunc(t, func(_ engine.RunRequest) ([]engine.LogEntry, error) {
		return []engine.LogEntry{{
			Module:    "NAMESERVER",
			Testcase:  "NS01",
			Tag:       "UNREGISTERED_ATTACK_TAG",
			Level:     "WARNING",
			Timestamp: 1.0,
			Args: map[string]any{
				"version_string": "\x1b[34mblue\x1b[0m",
				"trailing":       "ok\r\nfake|perf=999",
				"nul":            "x\x00y",
			},
		}}, nil
	})

	res := clitest.Run(t, run, "-H", "example.com", "-v")
	res.RequireCode(t, 1)

	body := []byte(res.Out)
	if !bytes.HasPrefix(body, []byte("ZONE WARNING")) {
		t.Fatalf("expected ZONE WARNING status line, got %q", body)
	}

	// Newlines separate the status line from the verbose entries.
	clitest.RequireNoControlBytes(t, body, '\n')
	for _, want := range []string{
		`\x1b[34mblue\x1b[0m`,
		`\x0d`, // CR
		`\x00`, // NUL
	} {
		if !strings.Contains(string(body), want) {
			t.Fatalf("expected %q in nagios verbose output, got %q", want, body)
		}
	}

	// The raw \n inside trailing="ok\r\nfake|perf=999" must have been
	// escaped, so the literal pipe-separator attack cannot start a new
	// physical line. Count newlines: 1 for ZONE status + 1 for the one
	// verbose entry = 2 total.
	if n := bytes.Count(body, []byte{'\n'}); n != 2 {
		t.Fatalf("expected 2 newlines (status + 1 entry), got %d in %q", n, body)
	}
}

// TestRunVerbosePreservesSafeMessages is the negative case: benign
// Unicode in a verbose entry must reach stdout unchanged.
func TestRunVerbosePreservesSafeMessages(t *testing.T) {
	stubRunEngineFunc(t, func(_ engine.RunRequest) ([]engine.LogEntry, error) {
		return []engine.LogEntry{{
			Module:    "NAMESERVER",
			Testcase:  "NS01",
			Tag:       "UNREGISTERED_BENIGN_TAG",
			Level:     "WARNING",
			Timestamp: 1.0,
			Args:      map[string]any{"label": "café résumé"},
		}}, nil
	})

	res := clitest.Run(t, run, "-H", "example.com", "-v")
	res.RequireCode(t, 1)
	res.RequireOutContains(t, "café résumé")
	if strings.Contains(res.Out, `\x`) {
		t.Fatalf("benign verbose output produced escapes: %q", res.Out)
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

	res := clitest.Run(t, run, "-H", "example.com", "--source-addr4", "192.0.2.50", "--source-addr6", "2001:db8::50", "--force-ipv6")
	res.RequireCode(t, 0)
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

	res := clitest.Run(t, run, "--domain", "example.com", "--sourceaddr4", "192.0.2.50", "--sourceaddr6", "2001:db8::50")
	res.RequireCode(t, 0)
	if captured.SourceAddr4 == nil || *captured.SourceAddr4 != "192.0.2.50" {
		t.Fatalf("unexpected SourceAddr4 override: %#v", captured.SourceAddr4)
	}
	if captured.SourceAddr6 == nil || *captured.SourceAddr6 != "2001:db8::50" {
		t.Fatalf("unexpected SourceAddr6 override: %#v", captured.SourceAddr6)
	}
}

func TestRunRejectsInvalidSourceAddr4(t *testing.T) {
	stubRunEngine(t, nil)

	res := clitest.Run(t, run, "--domain", "example.com", "--source-addr4", "not-an-ip")
	res.RequireCode(t, 3)
	res.RequireErrContains(t, "--source-addr4 must be a valid IPv4 address")
}

func TestRunRejectsInvalidSourceAddr6(t *testing.T) {
	stubRunEngine(t, nil)

	res := clitest.Run(t, run, "--domain", "example.com", "--source-addr6", "192.0.2.5")
	res.RequireCode(t, 3)
	res.RequireErrContains(t, "--source-addr6 must be a valid IPv6 address")
}

func TestRunRejectsInvalidWarningLevel(t *testing.T) {
	res := clitest.Run(t, run, "--domain", "example.com", "--warning", "bogus")
	res.RequireCode(t, 3)
	res.RequireErrContains(t, "--warning must be one of")
}

func TestRunRejectsInvalidThresholdOrdering(t *testing.T) {
	res := clitest.Run(t, run, "--domain", "example.com", "--warning", "ERROR", "--critical", "WARNING")
	res.RequireCode(t, 3)
	res.RequireErrContains(t, "--warning must be lower severity than --critical")
}

func TestRunRejectsInvalidTimeout(t *testing.T) {
	res := clitest.Run(t, run, "--domain", "example.com", "--timeout", "0")
	res.RequireCode(t, 3)
	res.RequireErrContains(t, "--timeout must be >= 1")
}

func TestRunAppliesCustomThresholds(t *testing.T) {
	stubRunEngineFunc(t, func(req engine.RunRequest) ([]engine.LogEntry, error) {
		return []engine.LogEntry{{Level: "ERROR"}}, nil
	})

	res := clitest.Run(t, run, "--domain", "example.com", "--warning", "WARNING", "--critical", "CRITICAL")
	res.RequireCode(t, 1)
	res.RequireOutContains(t, "ZONE WARNING")
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

	clitest.Run(t, run, "-H", "example.com", "-t", "1").
		RequireCode(t, 3).
		RequireOutContains(t, "ZONE UNKNOWN - plugin timed out after 1s")
}

func TestUndelegatedNSPassedToEngine(t *testing.T) {
	var captured engine.RunRequest
	stubRunEngine(t, &captured)

	res := clitest.Run(t, run, "-H", "example.com", "--ns", "ns1.example.com/192.0.2.1", "--ns", "ns2.example.com")
	res.RequireCode(t, 0)
	if len(captured.UndelegatedNameservers) != 2 {
		t.Fatalf("expected 2 undelegated nameservers, got %d", len(captured.UndelegatedNameservers))
	}
	if captured.UndelegatedNameservers[0].Name != "ns1.example.com" {
		t.Fatalf("expected ns1.example.com, got %q", captured.UndelegatedNameservers[0].Name)
	}
	if captured.UndelegatedNameservers[0].IP != "192.0.2.1" {
		t.Fatalf("expected 192.0.2.1, got %q", captured.UndelegatedNameservers[0].IP)
	}
	if captured.UndelegatedNameservers[1].Name != "ns2.example.com" {
		t.Fatalf("expected ns2.example.com, got %q", captured.UndelegatedNameservers[1].Name)
	}
	if captured.UndelegatedNameservers[1].IP != "" {
		t.Fatalf("expected empty IP, got %q", captured.UndelegatedNameservers[1].IP)
	}
}

func TestUndelegatedDSPassedToEngine(t *testing.T) {
	var captured engine.RunRequest
	stubRunEngine(t, &captured)

	res := clitest.Run(t, run, "-H", "example.com", "--ns", "ns1.example.com/192.0.2.1", "--ds", "12345,13,2,ABCDEF0123456789ABCDEF0123456789ABCDEF0123456789ABCDEF0123456789")
	res.RequireCode(t, 0)
	if len(captured.UndelegatedDSInfo) != 1 {
		t.Fatalf("expected 1 DS record, got %d", len(captured.UndelegatedDSInfo))
	}
	ds := captured.UndelegatedDSInfo[0]
	if ds.KeyTag != 12345 || ds.Algorithm != 13 || ds.DigestType != 2 {
		t.Fatalf("DS fields wrong: %+v", ds)
	}
}

func TestUndelegatedDSWithoutNSIsError(t *testing.T) {
	stubRunEngine(t, nil)

	res := clitest.Run(t, run, "-H", "example.com", "--ds", "12345,13,2,ABCDEF0123456789ABCDEF0123456789ABCDEF0123456789ABCDEF0123456789")
	res.RequireCode(t, 3)
	res.RequireErrContains(t, "--ds requires --ns")
}

func TestUndelegatedInvalidNS(t *testing.T) {
	stubRunEngine(t, nil)

	res := clitest.Run(t, run, "-H", "example.com", "--ns", "ns1/bad/extra")
	res.RequireCode(t, 3)
}

func TestUndelegatedInvalidDS(t *testing.T) {
	stubRunEngine(t, nil)

	res := clitest.Run(t, run, "-H", "example.com", "--ns", "ns1.example.com", "--ds", "notvalid")
	res.RequireCode(t, 3)
}

func TestHelpIncludesNSAndDS(t *testing.T) {
	res := clitest.Run(t, run, "--help")
	res.RequireCode(t, 0)
	help := res.Err
	for _, fragment := range []string{"--ns", "--ds"} {
		if !strings.Contains(help, fragment) {
			t.Fatalf("expected %q in usage output", fragment)
		}
	}
}

func TestRRSIGWarnDaysSetsProfileOnRequest(t *testing.T) {
	var profileContent []byte
	stubRunEngineFunc(t, func(req engine.RunRequest) ([]engine.LogEntry, error) {
		if req.Profile != "" {
			// Read profile while temp file still exists (before run() defers cleanup).
			profileContent, _ = os.ReadFile(req.Profile)
		}
		return nil, nil
	})

	res := clitest.Run(t, run, "-H", "example.com", "--testcase", "dnssec04", "--rrsig-warn-days", "14")
	res.RequireCode(t, 0)
	if len(profileContent) == 0 {
		t.Fatal("expected profile content to be captured inside engine stub")
	}
	var p map[string]any
	if err := json.Unmarshal(profileContent, &p); err != nil {
		t.Fatalf("parse profile: %v", err)
	}
	vars, _ := p["test_cases_vars"].(map[string]any)
	dnssec04, _ := vars["dnssec04"].(map[string]any)
	remaining, _ := dnssec04["REMAINING_SHORT"].(float64)
	if int(remaining) != 14*86400 {
		t.Fatalf("expected REMAINING_SHORT=%d, got %d", 14*86400, int(remaining))
	}
}

func TestRRSIGWarnDaysZeroLeavesProfileUnchanged(t *testing.T) {
	var captured engine.RunRequest
	stubRunEngine(t, &captured)

	res := clitest.Run(t, run, "-H", "example.com")
	res.RequireCode(t, 0)
	if captured.Profile != "" {
		t.Fatalf("expected Profile to be empty, got %q", captured.Profile)
	}
}

func TestRRSIGWarnDaysNegativeIsError(t *testing.T) {
	stubRunEngine(t, nil)

	res := clitest.Run(t, run, "-H", "example.com", "--rrsig-warn-days", "-1")
	res.RequireCode(t, 3)
	res.RequireErrContains(t, "--rrsig-warn-days")
}

func TestBuildMergedProfileNoOverride(t *testing.T) {
	path, cleanup, err := buildMergedProfile("", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cleanup != nil {
		t.Fatal("expected no cleanup for zero rrsigWarnDays")
	}
	if path != "" {
		t.Fatalf("expected empty path, got %q", path)
	}
}

func TestBuildMergedProfileWritesTempFile(t *testing.T) {
	path, cleanup, err := buildMergedProfile("", 7)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cleanup == nil {
		t.Fatal("expected cleanup function")
	}
	defer cleanup()

	if path == "" {
		t.Fatal("expected non-empty temp file path")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read temp file: %v", err)
	}
	var p map[string]any
	if err := json.Unmarshal(data, &p); err != nil {
		t.Fatalf("parse temp file: %v", err)
	}
	vars, _ := p["test_cases_vars"].(map[string]any)
	dnssec04, _ := vars["dnssec04"].(map[string]any)
	remaining, _ := dnssec04["REMAINING_SHORT"].(float64)
	if int(remaining) != 7*86400 {
		t.Fatalf("expected REMAINING_SHORT=%d, got %d", 7*86400, int(remaining))
	}
}

func TestHelpIncludesRRSIGWarnDays(t *testing.T) {
	res := clitest.Run(t, run, "--help")
	res.RequireCode(t, 0)
	res.RequireErrContains(t, "--rrsig-warn-days")
}

// --- Grade-based threshold tests -------------------------------------------

func TestParseGradeThresholdsValid(t *testing.T) {
	gt, err := parseGradeThresholds("C", "F")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !gt.isActive() {
		t.Fatal("expected grade thresholds to be active")
	}
	if gt.warningGrade != "C" || gt.criticalGrade != "F" {
		t.Fatalf("unexpected grades: warning=%q critical=%q", gt.warningGrade, gt.criticalGrade)
	}
}

func TestParseGradeThresholdsEmpty(t *testing.T) {
	gt, err := parseGradeThresholds("", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gt.isActive() {
		t.Fatal("expected grade thresholds to be inactive when both are empty")
	}
}

func TestParseGradeThresholdsOnlyWarning(t *testing.T) {
	gt, err := parseGradeThresholds("B", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !gt.isActive() {
		t.Fatal("expected grade thresholds active with only warning set")
	}
	if gt.criticalGrade != "" {
		t.Fatalf("expected no critical grade, got %q", gt.criticalGrade)
	}
}

func TestParseGradeThresholdsInvalidWarning(t *testing.T) {
	_, err := parseGradeThresholds("Z", "F")
	if err == nil || !strings.Contains(err.Error(), "--grade-warning") {
		t.Fatalf("expected --grade-warning validation error, got %v", err)
	}
}

func TestParseGradeThresholdsInvalidCritical(t *testing.T) {
	_, err := parseGradeThresholds("C", "X")
	if err == nil || !strings.Contains(err.Error(), "--grade-critical") {
		t.Fatalf("expected --grade-critical validation error, got %v", err)
	}
}

func TestParseGradeThresholdsWarnNotBetterThanCritical(t *testing.T) {
	// F is worse than C - warning threshold must be a better grade than critical
	_, err := parseGradeThresholds("F", "C")
	if err == nil {
		t.Fatal("expected error when grade-warning is not better than grade-critical")
	}
	if !strings.Contains(err.Error(), "--grade-warning must be a better grade") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseGradeThresholdsSameGrade(t *testing.T) {
	_, err := parseGradeThresholds("C", "C")
	if err == nil {
		t.Fatal("expected error when grade-warning equals grade-critical")
	}
}

func TestStatusForGradeOK(t *testing.T) {
	gt, _ := parseGradeThresholds("C", "F")
	// Grade A is better than C threshold → OK
	s := statusForGrade("A", gt)
	if s.code != 0 || s.text != "OK" {
		t.Fatalf("expected OK for grade A, got %+v", s)
	}
}

func TestStatusForGradeWarning(t *testing.T) {
	gt, _ := parseGradeThresholds("C", "F")
	// Grade C is at the warning threshold → WARNING
	s := statusForGrade("C", gt)
	if s.code != 1 || s.text != "WARNING" {
		t.Fatalf("expected WARNING for grade C, got %+v", s)
	}
}

func TestStatusForGradeWorseThanWarning(t *testing.T) {
	gt, _ := parseGradeThresholds("C", "F")
	// Grade D is between C and F thresholds → WARNING
	s := statusForGrade("D", gt)
	if s.code != 1 || s.text != "WARNING" {
		t.Fatalf("expected WARNING for grade D, got %+v", s)
	}
}

func TestStatusForGradeCritical(t *testing.T) {
	gt, _ := parseGradeThresholds("C", "F")
	// Grade F is at the critical threshold → CRITICAL
	s := statusForGrade("F", gt)
	if s.code != 2 || s.text != "CRITICAL" {
		t.Fatalf("expected CRITICAL for grade F, got %+v", s)
	}
}

func TestStatusForGradeOnlyWarningThreshold(t *testing.T) {
	gt, _ := parseGradeThresholds("B", "")
	// Grade B triggers WARNING; no CRITICAL threshold
	if s := statusForGrade("B", gt); s.code != 1 {
		t.Fatalf("expected WARNING for grade B, got %+v", s)
	}
	if s := statusForGrade("A", gt); s.code != 0 {
		t.Fatalf("expected OK for grade A, got %+v", s)
	}
	// Even F only triggers WARNING when no critical threshold
	if s := statusForGrade("F", gt); s.code != 1 {
		t.Fatalf("expected WARNING (not CRITICAL) for grade F with no critical threshold, got %+v", s)
	}
}

func TestStatusForGradeOnlyCriticalThreshold(t *testing.T) {
	gt, _ := parseGradeThresholds("", "F")
	if s := statusForGrade("F", gt); s.code != 2 {
		t.Fatalf("expected CRITICAL for grade F, got %+v", s)
	}
	// D is worse than A but there's no warning threshold - should be OK
	if s := statusForGrade("D", gt); s.code != 0 {
		t.Fatalf("expected OK for grade D with only critical threshold, got %+v", s)
	}
}

func TestWorstStatus(t *testing.T) {
	ok := nagiosStatus{text: "OK", code: 0}
	warn := nagiosStatus{text: "WARNING", code: 1}
	crit := nagiosStatus{text: "CRITICAL", code: 2}

	if worstStatus(ok, warn) != warn {
		t.Fatal("expected WARNING > OK")
	}
	if worstStatus(crit, warn) != crit {
		t.Fatal("expected CRITICAL > WARNING")
	}
	if worstStatus(ok, ok) != ok {
		t.Fatal("expected OK when both OK")
	}
}

func TestGradeModeExitCodeOK(t *testing.T) {
	// Empty entries → score 100, grade A → OK with --grade-warning C --grade-critical F
	stubRunEngineFunc(t, func(_ engine.RunRequest) ([]engine.LogEntry, error) {
		return []engine.LogEntry{}, nil
	})

	res := clitest.Run(t, run, "-H", "example.com", "--grade-warning", "C", "--grade-critical", "F")
	res.RequireCode(t, 0)
	res.RequireOutContains(t, "ZONE OK")
}

func TestGradeModeExitCodeCritical(t *testing.T) {
	// CRITICAL entry → score forced to 10, grade F → CRITICAL with --grade-critical F
	stubRunEngineFunc(t, func(_ engine.RunRequest) ([]engine.LogEntry, error) {
		return []engine.LogEntry{
			{Module: "BASIC", Tag: "NO_NS", Level: "CRITICAL"},
		}, nil
	})

	res := clitest.Run(t, run, "-H", "example.com", "--grade-warning", "C", "--grade-critical", "F")
	res.RequireCode(t, 2)
	res.RequireOutContains(t, "ZONE CRITICAL")
}

func TestGradeModeOutputIncludesGradeAndScore(t *testing.T) {
	stubRunEngineFunc(t, func(_ engine.RunRequest) ([]engine.LogEntry, error) {
		return []engine.LogEntry{}, nil
	})

	res := clitest.Run(t, run, "-H", "example.com", "--grade-warning", "C", "--grade-critical", "F")

	output := res.Out
	if !strings.Contains(output, "grade") {
		t.Fatalf("expected 'grade' in output, got %q", output)
	}
	if !strings.Contains(output, "score") {
		t.Fatalf("expected 'score' in output, got %q", output)
	}
}

func TestGradeModeInvalidWarningFlagReturnsUnknown(t *testing.T) {
	stubRunEngine(t, nil)

	res := clitest.Run(t, run, "-H", "example.com", "--grade-warning", "Z")
	res.RequireCode(t, 3)
	res.RequireErrContains(t, "--grade-warning")
}

func TestGradeModeInvalidCriticalFlagReturnsUnknown(t *testing.T) {
	stubRunEngine(t, nil)

	res := clitest.Run(t, run, "-H", "example.com", "--grade-critical", "X")
	res.RequireCode(t, 3)
	res.RequireErrContains(t, "--grade-critical")
}

func TestGradeModeInvalidOrderingReturnsUnknown(t *testing.T) {
	stubRunEngine(t, nil)

	res := clitest.Run(t, run, "-H", "example.com", "--grade-warning", "F", "--grade-critical", "C")
	res.RequireCode(t, 3)
}

func TestGradeModeWorstOfSeverityAndGrade(t *testing.T) {
	// Severity check: ERROR → WARNING (--warning ERROR --critical CRITICAL)
	// Grade check: grade A → OK (--grade-warning C --grade-critical F)
	// Result should be WARNING (severity wins)
	stubRunEngineFunc(t, func(_ engine.RunRequest) ([]engine.LogEntry, error) {
		return []engine.LogEntry{
			{Module: "NAMESERVER", Tag: "NS_NO_RESPONSE", Level: "ERROR"},
		}, nil
	})

	res := clitest.Run(t, run, "-H", "example.com", "--warning", "WARNING", "--critical", "CRITICAL", "--grade-warning", "C", "--grade-critical", "F")
	res.RequireCode(t, 1)
}

func TestHelpIncludesGradeFlags(t *testing.T) {
	res := clitest.Run(t, run, "--help")
	res.RequireCode(t, 0)
	help := res.Err
	for _, fragment := range []string{"--grade-warning", "--grade-critical"} {
		if !strings.Contains(help, fragment) {
			t.Fatalf("expected %q in usage output", fragment)
		}
	}
}

func TestRunAllowNonGlobalFlag(t *testing.T) {
	// --allow-non-global sets the RunRequest override that disables the guard.
	var captured engine.RunRequest
	stubRunEngine(t, &captured)
	res := clitest.Run(t, run, "-H", "example.com", "--allow-non-global")
	res.RequireCode(t, 0)
	if captured.AllowNonGlobalTargets == nil || !*captured.AllowNonGlobalTargets {
		t.Fatalf("expected AllowNonGlobalTargets true, got %#v", captured.AllowNonGlobalTargets)
	}
}
