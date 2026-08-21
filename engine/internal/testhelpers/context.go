package testhelpers

import (
	"context"
	"testing"

	"codeberg.org/pawal/gonemaster/engine/internal/dnstest"
	"codeberg.org/pawal/gonemaster/engine/logger"
	"codeberg.org/pawal/gonemaster/engine/nameserver"
	"codeberg.org/pawal/gonemaster/engine/profile"
)

// DefaultProfile returns a default profile or fails the test.
func DefaultProfile(t *testing.T) *profile.Profile {
	t.Helper()
	return dnstest.DefaultProfile(t)
}

// Context returns a context preloaded with a default profile, logger, and nameserver cache.
func Context(t *testing.T) (context.Context, *profile.Profile, *logger.Logger) {
	t.Helper()
	ctx, p, log := dnstest.Context(t)
	return nameserver.WithCache(ctx, nameserver.NewCacheStore()), p, log
}
