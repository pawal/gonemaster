package logger

import "context"

type loggerKey struct{}

// WithContext stores l in ctx for downstream access.
func WithContext(ctx context.Context, l *Logger) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, loggerKey{}, l)
}

// FromContext returns the logger stored in ctx, or nil.
func FromContext(ctx context.Context) *Logger {
	if ctx == nil {
		return nil
	}
	if l, ok := ctx.Value(loggerKey{}).(*Logger); ok {
		return l
	}
	return nil
}

// MustFromContext returns the logger stored in ctx or panics.
func MustFromContext(ctx context.Context) *Logger {
	if ctx == nil {
		panic("logger: missing Logger in context")
	}
	l, ok := ctx.Value(loggerKey{}).(*Logger)
	if !ok || l == nil {
		panic("logger: missing Logger in context")
	}
	return l
}
