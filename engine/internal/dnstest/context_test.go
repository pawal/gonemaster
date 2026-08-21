package dnstest

import (
	"testing"

	"codeberg.org/pawal/gonemaster/engine/logger"
	"codeberg.org/pawal/gonemaster/engine/profile"
)

func TestDefaultProfile(t *testing.T) {
	if p := DefaultProfile(t); p == nil {
		t.Fatal("DefaultProfile returned nil")
	}
}

func TestDefaultProfileReturnsAFreshProfile(t *testing.T) {
	// Each caller gets its own profile, so a test that mutates one cannot
	// affect the next.
	first := DefaultProfile(t)
	second := DefaultProfile(t)
	if first == second {
		t.Fatal("expected DefaultProfile to return a fresh profile each call")
	}
}

func TestContextCarriesProfileAndLogger(t *testing.T) {
	ctx, p, log := Context(t)

	if p == nil {
		t.Fatal("Context returned nil profile")
	}
	if log == nil {
		t.Fatal("Context returned nil logger")
	}
	if got := profile.FromContext(ctx); got != p {
		t.Fatalf("profile.FromContext(ctx) = %p, want %p", got, p)
	}
	if got := logger.FromContext(ctx); got != log {
		t.Fatalf("logger.FromContext(ctx) = %p, want %p", got, log)
	}
}

func TestContextDoesNotDependOnTheGlobalProfile(t *testing.T) {
	// The context profile must be the one Context built, not profile.Effective,
	// so a test that mutates the global effective profile cannot leak in.
	t.Cleanup(profile.ResetEffective)
	profile.Effective().Resolver.Defaults.Parallel = 99

	_, p, _ := Context(t)
	if p.Resolver.Defaults.Parallel == 99 {
		t.Fatal("expected Context to build on a default profile, not the effective one")
	}
}
