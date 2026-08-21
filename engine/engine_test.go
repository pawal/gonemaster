package engine

import (
	"errors"
	"sync"
	"testing"

	"codeberg.org/pawal/gonemaster/engine/internal/dnstest"
	"codeberg.org/pawal/gonemaster/engine/logger"
)

func TestRunUnknownModule(t *testing.T) {
	_, err := Run(RunRequest{Domain: "example.com", Module: "unknown"})
	if !errors.Is(err, ErrNotImplemented) {
		t.Fatalf("expected not implemented error, got %v", err)
	}
}

func TestRunWithRunnerUnknownModule(t *testing.T) {
	runner := newTestRunner(t)
	_, err := RunWithRunner(RunRequest{Domain: "example.com", Module: "unknown"}, runner)
	if !errors.Is(err, ErrNotImplemented) {
		t.Fatalf("expected not implemented error, got %v", err)
	}
}

func TestEffectiveProfileUsesRunner(t *testing.T) {
	runner := newTestRunner(t)
	req := RunRequest{Runner: runner}
	got, err := EffectiveProfile(req)
	if err != nil {
		t.Fatalf("effective profile: %v", err)
	}
	if got != runner.Profile {
		t.Fatalf("expected runner profile")
	}
}

func TestEffectiveProfileAppliesSourceAddrOverrides(t *testing.T) {
	source4 := "192.0.2.77"
	source6 := "2001:db8::77"
	got, err := EffectiveProfile(RunRequest{
		SourceAddr4: &source4,
		SourceAddr6: &source6,
	})
	if err != nil {
		t.Fatalf("effective profile: %v", err)
	}
	if got.Resolver.Source4 != source4 {
		t.Fatalf("resolver.source4 = %q, want %q", got.Resolver.Source4, source4)
	}
	if got.Resolver.Source6 != source6 {
		t.Fatalf("resolver.source6 = %q, want %q", got.Resolver.Source6, source6)
	}
}

// TestEffectiveProfileDebugOverride checks that the Debug request override maps
// to resolver.defaults.debug, and that it stays false by default. This is the
// flag --debug-queries sets, which gates query tracing.
func TestEffectiveProfileDebugOverride(t *testing.T) {
	def, err := EffectiveProfile(RunRequest{})
	if err != nil {
		t.Fatalf("effective profile (default): %v", err)
	}
	if def.Resolver.Defaults.Debug {
		t.Fatalf("resolver.defaults.debug should default to false")
	}

	enabled := true
	got, err := EffectiveProfile(RunRequest{Debug: &enabled})
	if err != nil {
		t.Fatalf("effective profile (debug): %v", err)
	}
	if !got.Resolver.Defaults.Debug {
		t.Fatalf("resolver.defaults.debug = false, want true after Debug override")
	}
}

func TestRunWithRunnerConcurrentIsolation(t *testing.T) {
	// Timeout-bound; safe to overlap: all run state is per-Runner.
	t.Parallel()
	runner1 := newTestRunner(t, withRunLimits(1))
	runner2 := newTestRunner(t, withRunLimits(2))
	log1, log2 := runner1.Logger, runner2.Logger

	req1 := RunRequest{Domain: "example.com", Testcases: []string{"syntax01"}}
	req2 := RunRequest{Domain: "example.net", Testcases: []string{"syntax01"}}

	var wg sync.WaitGroup
	wg.Add(2)
	var err1, err2 error
	go func() {
		defer wg.Done()
		_, err1 = RunWithRunner(req1, runner1)
	}()
	go func() {
		defer wg.Done()
		_, err2 = RunWithRunner(req2, runner2)
	}()
	wg.Wait()

	if err1 != nil {
		t.Fatalf("run1 error: %v", err1)
	}
	if err2 != nil {
		t.Fatalf("run2 error: %v", err2)
	}

	if !logHasDomain(log1, "example.com") {
		t.Fatalf("expected run1 logs to include example.com")
	}
	if !logHasDomain(log2, "example.net") {
		t.Fatalf("expected run2 logs to include example.net")
	}
	if logHasDomain(log1, "example.net") {
		t.Fatalf("unexpected example.net in run1 logs")
	}
	if logHasDomain(log2, "example.com") {
		t.Fatalf("unexpected example.com in run2 logs")
	}
}

func TestRunEmitsStartupTags(t *testing.T) {
	// Timeout-bound; safe to overlap: all run state is per-Runner.
	t.Parallel()
	runner := newTestRunner(t, withRunLimits(1))
	log := runner.Logger
	_, _ = RunWithRunner(RunRequest{Domain: "example.com", Testcases: []string{"syntax01"}}, runner)

	required := []string{"GLOBAL_VERSION", "START_TIME", "TEST_TARGET", "MODULE_END"}
	for _, tag := range required {
		if !dnstest.HasTag(log.Entries(), tag) {
			t.Errorf("expected %s tag in run output", tag)
		}
	}
}

func TestRunEmitsSkipIPv4Disabled(t *testing.T) {
	runner := newTestRunner(t, withRunLimits(1))
	runner.Profile.Net.IPv4 = false
	log := runner.Logger
	_, _ = RunWithRunner(RunRequest{Domain: "example.com", Testcases: []string{"syntax01"}}, runner)

	if !dnstest.HasTag(log.Entries(), "SKIP_IPV4_DISABLED") {
		t.Fatalf("expected SKIP_IPV4_DISABLED when IPv4 disabled")
	}
}

func TestRunEmitsNoNetwork(t *testing.T) {
	runner := newTestRunner(t, withRunLimits(1))
	runner.Profile.Net.IPv4 = false
	runner.Profile.Net.IPv6 = false
	log := runner.Logger
	entries, err := RunWithRunner(RunRequest{Domain: "example.com", Testcases: []string{"syntax01"}}, runner)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !dnstest.HasTag(log.Entries(), "NO_NETWORK") {
		t.Fatalf("expected NO_NETWORK when both IPv4 and IPv6 disabled")
	}
	// Should produce no test results when there's no network.
	for _, e := range entries {
		if e.Module != "" && e.Module != "System" {
			t.Fatalf("unexpected non-system entry %s when no network", e.Tag)
		}
	}
}

func TestRunWithRunnerEmitsUnknownModule(t *testing.T) {
	runner := newTestRunner(t)
	log := runner.Logger
	_, err := RunWithRunner(RunRequest{Domain: "example.com", Module: "nonexistent"}, runner)
	if !errors.Is(err, ErrNotImplemented) {
		t.Fatalf("expected not implemented error, got %v", err)
	}
	if !dnstest.HasTag(log.Entries(), "UNKNOWN_MODULE") {
		t.Fatalf("expected UNKNOWN_MODULE tag for invalid module")
	}
}

func TestRunWithRunnerEmitsUnknownMethod(t *testing.T) {
	runner := newTestRunner(t)
	log := runner.Logger
	_, err := RunWithRunner(RunRequest{Domain: "example.com", Testcases: []string{"nonexistent99"}}, runner)
	if !errors.Is(err, ErrNotImplemented) {
		t.Fatalf("expected not implemented error, got %v", err)
	}
	if !dnstest.HasTag(log.Entries(), "UNKNOWN_METHOD") {
		t.Fatalf("expected UNKNOWN_METHOD tag for invalid testcase")
	}
}

func logHasDomain(log *logger.Logger, domain string) bool {
	for _, entry := range log.Entries() {
		if entry == nil {
			continue
		}
		if value, ok := entry.Args["domain"]; ok && value == domain {
			return true
		}
	}
	return false
}

func TestBuildProfileAllowNonGlobalTargets(t *testing.T) {
	// The --allow-non-global override flows into the effective profile; a nil
	// override leaves the shipped default (guard on).
	tru := true
	p, _, err := buildProfile(RunRequest{Domain: "example.com", AllowNonGlobalTargets: &tru}, "", nil)
	if err != nil {
		t.Fatalf("buildProfile: %v", err)
	}
	if !p.Net.AllowNonGlobalTargets {
		t.Errorf("expected net.allow_non_global_targets true when overridden")
	}

	def, _, err := buildProfile(RunRequest{Domain: "example.com"}, "", nil)
	if err != nil {
		t.Fatalf("buildProfile default: %v", err)
	}
	if def.Net.AllowNonGlobalTargets {
		t.Errorf("expected net.allow_non_global_targets false by default")
	}
}
