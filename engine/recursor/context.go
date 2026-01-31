package recursor

import "context"

type unorderedContextKey struct{}

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
