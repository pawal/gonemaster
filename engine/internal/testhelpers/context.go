package testhelpers

import (
	"context"
	"testing"

	"codeberg.org/pawal/gonemaster/engine/logger"
	"codeberg.org/pawal/gonemaster/engine/nameserver"
	"codeberg.org/pawal/gonemaster/engine/profile"
)

// DefaultProfile returns a default profile or fails the test.
func DefaultProfile(t *testing.T) *profile.Profile {
	t.Helper()
	p, err := profile.Default()
	if err != nil {
		t.Fatalf("load default profile: %v", err)
	}
	return p
}

// Context returns a context preloaded with a default profile, logger, and nameserver cache.
func Context(t *testing.T) (context.Context, *profile.Profile, *logger.Logger) {
	t.Helper()
	p := DefaultProfile(t)
	log := logger.New()
	ctx := context.Background()
	ctx = profile.WithContext(ctx, p)
	ctx = logger.WithContext(ctx, log)
	ctx = nameserver.WithCache(ctx, nameserver.NewCacheStore())
	return ctx, p, log
}
