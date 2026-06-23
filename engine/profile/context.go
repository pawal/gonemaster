package profile

import (
	"context"
	"net/netip"
)

type profileKey struct{}

type allowedTargetsKey struct{}

// WithAllowedTargets stores operator-supplied addresses that are exempt from the
// non-global query guard in ctx. Keys are stored unmapped.
func WithAllowedTargets(ctx context.Context, addrs map[netip.Addr]struct{}) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, allowedTargetsKey{}, addrs)
}

// IsAllowedTarget reports whether addr was explicitly supplied by the operator.
func IsAllowedTarget(ctx context.Context, addr netip.Addr) bool {
	if ctx == nil {
		return false
	}
	set, ok := ctx.Value(allowedTargetsKey{}).(map[netip.Addr]struct{})
	if !ok {
		return false
	}
	_, ok = set[addr.Unmap()]
	return ok
}

// WithContext stores p in ctx for downstream access.
func WithContext(ctx context.Context, p *Profile) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, profileKey{}, p)
}

// FromContext returns the profile stored in ctx, or the global effective profile.
func FromContext(ctx context.Context) *Profile {
	if ctx == nil {
		return Effective()
	}
	if p, ok := ctx.Value(profileKey{}).(*Profile); ok && p != nil {
		return p
	}
	return Effective()
}

// MustFromContext returns the profile stored in ctx or panics.
func MustFromContext(ctx context.Context) *Profile {
	if ctx == nil {
		panic("profile: missing Profile in context")
	}
	p, ok := ctx.Value(profileKey{}).(*Profile)
	if !ok || p == nil {
		panic("profile: missing Profile in context")
	}
	return p
}
