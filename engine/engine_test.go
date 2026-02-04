package engine

import (
	"errors"
	"testing"

	"codeberg.org/pawal/gonemaster/engine/logger"
	"codeberg.org/pawal/gonemaster/engine/profile"
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
