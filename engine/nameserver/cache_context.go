package nameserver

import "context"

type cacheContextKey struct{}

// WithCache stores the cache store in ctx for downstream access.
func WithCache(ctx context.Context, cache *CacheStore) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, cacheContextKey{}, cache)
}

// CacheFromContext returns the cache store stored in ctx, or nil.
func CacheFromContext(ctx context.Context) *CacheStore {
	if ctx == nil {
		return nil
	}
	cache, _ := ctx.Value(cacheContextKey{}).(*CacheStore)
	return cache
}

// CacheFromContextOrDefault returns the cache store stored in ctx or the default store.
func CacheFromContextOrDefault(ctx context.Context) *CacheStore {
	if cache := CacheFromContext(ctx); cache != nil {
		return cache
	}
	return defaultCache
}
