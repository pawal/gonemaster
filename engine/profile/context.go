package profile

import "context"

type profileKey struct{}

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
