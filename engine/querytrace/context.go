package querytrace

import "context"

type traceKey struct{}

// WithContext stores t in ctx; a nil trace leaves ctx unchanged.
func WithContext(ctx context.Context, t QueryTrace) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if t == nil {
		return ctx
	}
	return context.WithValue(ctx, traceKey{}, t)
}

// FromContext returns the QueryTrace stored in ctx, or nil when tracing is off.
func FromContext(ctx context.Context) QueryTrace {
	if ctx == nil {
		return nil
	}
	if t, ok := ctx.Value(traceKey{}).(QueryTrace); ok {
		return t
	}
	return nil
}
