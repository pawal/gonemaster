package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"strings"
)

// newLogger builds a slog logger writing to w. format is "text" (default) or
// "json"; level is debug|info|warn|error (empty defaults to info).
func newLogger(format, level string, w io.Writer) *slog.Logger {
	opts := &slog.HandlerOptions{Level: parseLevel(level)}
	var h slog.Handler
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "json":
		h = slog.NewJSONHandler(w, opts)
	default:
		h = slog.NewTextHandler(w, opts)
	}
	return slog.New(h)
}

// parseLevel maps a level name to a slog.Level, defaulting to info.
func parseLevel(level string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// validLogFormat reports whether f is an accepted log_format value. Empty is
// accepted and resolves to the default ("text").
func validLogFormat(f string) bool {
	switch strings.ToLower(strings.TrimSpace(f)) {
	case "", "text", "json":
		return true
	}
	return false
}

// validLogLevel reports whether l is an accepted log_level value. Empty is
// accepted and resolves to the default ("info").
func validLogLevel(l string) bool {
	switch strings.ToLower(strings.TrimSpace(l)) {
	case "", "debug", "info", "warn", "warning", "error":
		return true
	}
	return false
}

// ValidateLogConfig fails fast on unrecognized log_format / log_level so a typo
// is a startup error rather than a silent fallback.
func ValidateLogConfig(cfg Config) error {
	if !validLogFormat(cfg.LogFormat) {
		return fmt.Errorf("invalid log_format %q (want text or json)", cfg.LogFormat)
	}
	if !validLogLevel(cfg.LogLevel) {
		return fmt.Errorf("invalid log_level %q (want debug, info, warn, or error)", cfg.LogLevel)
	}
	return nil
}

// contextKey is a private type for request-scoped context values.
type contextKey string

const requestIDContextKey contextKey = "request_id"

const routeHolderContextKey contextKey = "route_holder"

// requestIDFromContext returns the correlation ID stored on ctx, or "".
func requestIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(requestIDContextKey).(string); ok {
		return v
	}
	return ""
}

// newRequestID returns a short random hex ID for request correlation.
func newRequestID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "unknown"
	}
	return hex.EncodeToString(b[:])
}

// sanitizeRequestID accepts an inbound request ID only if it is a short token
// of safe characters, guarding against log/header injection. It returns ""
// when the value is unusable so the caller generates a fresh ID.
func sanitizeRequestID(s string) string {
	s = strings.TrimSpace(s)
	if s == "" || len(s) > 128 {
		return ""
	}
	for _, r := range s {
		ok := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.'
		if !ok {
			return ""
		}
	}
	return s
}
