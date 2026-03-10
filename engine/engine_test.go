package engine

import (
	"errors"
	"sync"
	"testing"

	"codeberg.org/pawal/gonemaster/engine/logger"
	"codeberg.org/pawal/gonemaster/engine/nameserver"
	"codeberg.org/pawal/gonemaster/engine/profile"
	"codeberg.org/pawal/gonemaster/engine/transport"
)

func TestRunUnknownModule(t *testing.T) {
	_, err := Run(RunRequest{Domain: "example.com", Module: "unknown"})
	if !errors.Is(err, ErrNotImplemented) {
		t.Fatalf("expected not implemented error, got %v", err)
	}
}

func TestRunWithRunnerUnknownModule(t *testing.T) {
	p, err := profile.Default()
	if err != nil {
		t.Fatalf("profile default: %v", err)
	}
	runner := &Runner{Profile: p, Logger: logger.New()}
	_, err = RunWithRunner(RunRequest{Domain: "example.com", Module: "unknown"}, runner)
	if !errors.Is(err, ErrNotImplemented) {
		t.Fatalf("expected not implemented error, got %v", err)
	}
}

func TestEffectiveProfileUsesRunner(t *testing.T) {
	p, err := profile.Default()
	if err != nil {
		t.Fatalf("profile default: %v", err)
	}
	runner := &Runner{Profile: p, Logger: logger.New()}
	req := RunRequest{Runner: runner}
	got, err := EffectiveProfile(req)
	if err != nil {
		t.Fatalf("effective profile: %v", err)
	}
	if got != p {
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

func TestRunWithRunnerConcurrentIsolation(t *testing.T) {
	p1, err := profile.Default()
	if err != nil {
		t.Fatalf("profile default: %v", err)
	}
	p2, err := profile.Default()
	if err != nil {
		t.Fatalf("profile default: %v", err)
	}

	log1 := logger.New()
	log2 := logger.New()

	runner1 := &Runner{
		Profile:         p1,
		Logger:          log1,
		Limiter:         transport.NewLimiter(1),
		NameserverCache: nameserver.NewCacheStore(),
	}
	runner2 := &Runner{
		Profile:         p2,
		Logger:          log2,
		Limiter:         transport.NewLimiter(2),
		NameserverCache: nameserver.NewCacheStore(),
	}

	req1 := RunRequest{Domain: "example.com", Testcase: "syntax01"}
	req2 := RunRequest{Domain: "example.net", Testcase: "syntax01"}

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
	p, err := profile.Default()
	if err != nil {
		t.Fatalf("profile default: %v", err)
	}
	log := logger.New()
	runner := &Runner{
		Profile:         p,
		Logger:          log,
		Limiter:         transport.NewLimiter(1),
		NameserverCache: nameserver.NewCacheStore(),
	}
	_, _ = RunWithRunner(RunRequest{Domain: "example.com", Testcase: "syntax01"}, runner)

	required := []string{"GLOBAL_VERSION", "START_TIME", "TEST_TARGET", "MODULE_END"}
	for _, tag := range required {
		if !logHasTag(log, tag) {
			t.Errorf("expected %s tag in run output", tag)
		}
	}
}

func TestRunEmitsSkipIPv4Disabled(t *testing.T) {
	p, err := profile.Default()
	if err != nil {
		t.Fatalf("profile default: %v", err)
	}
	p.Net.IPv4 = false
	log := logger.New()
	runner := &Runner{
		Profile:         p,
		Logger:          log,
		Limiter:         transport.NewLimiter(1),
		NameserverCache: nameserver.NewCacheStore(),
	}
	_, _ = RunWithRunner(RunRequest{Domain: "example.com", Testcase: "syntax01"}, runner)

	if !logHasTag(log, "SKIP_IPV4_DISABLED") {
		t.Fatalf("expected SKIP_IPV4_DISABLED when IPv4 disabled")
	}
}

func TestRunEmitsNoNetwork(t *testing.T) {
	p, err := profile.Default()
	if err != nil {
		t.Fatalf("profile default: %v", err)
	}
	p.Net.IPv4 = false
	p.Net.IPv6 = false
	log := logger.New()
	runner := &Runner{
		Profile:         p,
		Logger:          log,
		Limiter:         transport.NewLimiter(1),
		NameserverCache: nameserver.NewCacheStore(),
	}
	entries, err := RunWithRunner(RunRequest{Domain: "example.com", Testcase: "syntax01"}, runner)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !logHasTag(log, "NO_NETWORK") {
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
	p, err := profile.Default()
	if err != nil {
		t.Fatalf("profile default: %v", err)
	}
	log := logger.New()
	runner := &Runner{Profile: p, Logger: log}
	_, err = RunWithRunner(RunRequest{Domain: "example.com", Module: "nonexistent"}, runner)
	if !errors.Is(err, ErrNotImplemented) {
		t.Fatalf("expected not implemented error, got %v", err)
	}
	if !logHasTag(log, "UNKNOWN_MODULE") {
		t.Fatalf("expected UNKNOWN_MODULE tag for invalid module")
	}
}

func TestRunWithRunnerEmitsUnknownMethod(t *testing.T) {
	p, err := profile.Default()
	if err != nil {
		t.Fatalf("profile default: %v", err)
	}
	log := logger.New()
	runner := &Runner{Profile: p, Logger: log}
	_, err = RunWithRunner(RunRequest{Domain: "example.com", Testcase: "nonexistent99"}, runner)
	if !errors.Is(err, ErrNotImplemented) {
		t.Fatalf("expected not implemented error, got %v", err)
	}
	if !logHasTag(log, "UNKNOWN_METHOD") {
		t.Fatalf("expected UNKNOWN_METHOD tag for invalid testcase")
	}
}

func logHasTag(log *logger.Logger, tag string) bool {
	for _, entry := range log.Entries() {
		if entry != nil && entry.Tag == tag {
			return true
		}
	}
	return false
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
