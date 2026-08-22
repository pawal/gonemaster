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
// It derives from t.Context(), so work started by a test is canceled when the
// test ends. Callers that need a nameserver cache use testhelpers.Context.
func Context(t testing.TB) (context.Context, *profile.Profile, *logger.Logger) {
	t.Helper()
	p := DefaultProfile(t)
	log := logger.New()
	ctx := profile.WithContext(t.Context(), p)
	ctx = logger.WithContext(ctx, log)
	return ctx, p, log
}
