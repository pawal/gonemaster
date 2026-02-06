package testhelpers

import (
	"testing"

	"codeberg.org/pawal/gonemaster/engine/logger"
	"codeberg.org/pawal/gonemaster/engine/nameserver"
	"codeberg.org/pawal/gonemaster/engine/profile"
)

func TestDefaultProfile(t *testing.T) {
	p := DefaultProfile(t)
	if p == nil {
		t.Fatal("DefaultProfile returned nil")
	}
}

func TestContext(t *testing.T) {
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
	if got := nameserver.CacheFromContext(ctx); got == nil {
		t.Fatal("nameserver.CacheFromContext(ctx) returned nil")
	}
}
