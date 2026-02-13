package engine

import (
	"errors"
	"sync"
	"sync/atomic"
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

func TestBuildRunnerInitializesReusableState(t *testing.T) {
	runner, err := BuildRunner(RunRequest{
		Domain:   "example.com",
		Testcase: "syntax01",
	})
	if err != nil {
		t.Fatalf("build runner: %v", err)
	}
	if runner == nil {
		t.Fatalf("expected runner")
	}
	if runner.Profile == nil {
		t.Fatalf("expected runner profile")
	}
	if runner.Logger == nil {
		t.Fatalf("expected runner logger")
	}
	if runner.Limiter == nil {
		t.Fatalf("expected runner limiter")
	}
	if runner.NameserverCache == nil {
		t.Fatalf("expected runner nameserver cache")
	}
	if runner.StartedAt.IsZero() {
		t.Fatalf("expected runner started timestamp")
	}
}

func TestBuildRunnerPreservesLogCallback(t *testing.T) {
	var callbackCount atomic.Int32
	runner, err := BuildRunner(RunRequest{
		Domain:   "example.com",
		Testcase: "syntax01",
		LogCallback: func(_ *logger.Entry) error {
			callbackCount.Add(1)
			return nil
		},
	})
	if err != nil {
		t.Fatalf("build runner: %v", err)
	}
	if runner.Logger == nil {
		t.Fatalf("expected runner logger")
	}
	if _, err := runner.Logger.Add("GLOBAL_VERSION", map[string]any{"version": "v0.0.0"}, "", ""); err != nil {
		t.Fatalf("logger add: %v", err)
	}
	if got := callbackCount.Load(); got != 1 {
		t.Fatalf("expected callback count 1, got %d", got)
	}
}
