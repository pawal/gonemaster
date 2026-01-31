package recursor

import "context"

type unorderedContextKey struct{}
type unorderedDepthKey struct{}

func withUnorderedContext(ctx context.Context) context.Context {
	if ctx == nil {
		return context.WithValue(context.Background(), unorderedContextKey{}, true)
	}
	return context.WithValue(ctx, unorderedContextKey{}, true)
}

func isUnorderedContext(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	value, ok := ctx.Value(unorderedContextKey{}).(bool)
	return ok && value
}

func withUnorderedDepth(ctx context.Context, depth int) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, unorderedDepthKey{}, depth)
}

func unorderedDepth(ctx context.Context) int {
	if ctx == nil {
		return 0
	}
	value, ok := ctx.Value(unorderedDepthKey{}).(int)
	if !ok {
		return 0
	}
	return value
}
