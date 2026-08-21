package dnstest

import (
	"context"
	"testing"

	"codeberg.org/pawal/gonemaster/engine/logger"
	"codeberg.org/pawal/gonemaster/engine/profile"
)

// DefaultProfile returns a default profile or fails the test.
func DefaultProfile(t testing.TB) *profile.Profile {
	t.Helper()
	p, err := profile.Default()
	if err != nil {
		t.Fatalf("load default profile: %v", err)
	}
	return p
}

// Context returns a context carrying a default profile and a fresh logger.
// Callers that need a nameserver cache use testhelpers.Context instead.
func Context(t testing.TB) (context.Context, *profile.Profile, *logger.Logger) {
	t.Helper()
	p := DefaultProfile(t)
	log := logger.New()
	ctx := profile.WithContext(context.Background(), p)
	ctx = logger.WithContext(ctx, log)
	return ctx, p, log
}
