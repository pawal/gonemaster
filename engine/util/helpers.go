package util

import (
	"context"
	"slices"
	"sync"

	"codeberg.org/pawal/gonemaster/engine/constants"
	"codeberg.org/pawal/gonemaster/engine/dnsname"
	"codeberg.org/pawal/gonemaster/engine/logger"
	"codeberg.org/pawal/gonemaster/engine/profile"
	"codeberg.org/pawal/gonemaster/engine/zone"
)

var (
	defaultLoggerMu sync.Mutex
	defaultLogger   *logger.Logger
)

// Logger returns the default logger instance.
func Logger() *logger.Logger {
	defaultLoggerMu.Lock()
	defer defaultLoggerMu.Unlock()
	if defaultLogger == nil {
		defaultLogger = logger.New()
	}
	return defaultLogger
}

// LoggerFromContext returns the logger stored in ctx or the default logger.
func LoggerFromContext(ctx context.Context) *logger.Logger {
	if ctx != nil {
		if l := logger.FromContext(ctx); l != nil {
			return l
		}
	}
	return Logger()
}

// SetLogger overrides the default logger instance.
func SetLogger(l *logger.Logger) {
	defaultLoggerMu.Lock()
	defaultLogger = l
	defaultLoggerMu.Unlock()
}

// Info creates a log entry using the logger stored in ctx.
func Info(ctx context.Context, tag string, args map[string]any) (*logger.Entry, error) {
	return LoggerFromContext(ctx).Add(tag, args, "", "")
}

// Name creates a DNSName object for the given domain.
func Name(domain string) dnsname.Name {
	return dnsname.New(domain)
}

// Zone creates a Zone object for the given domain name.
func Zone(name string) (*zone.Zone, error) {
	z, err := zone.New(name)
	if err != nil {
		return nil, err
	}
	return &z, nil
}

// ShouldRunTest reports whether a test case is enabled in the effective profile.
func ShouldRunTest(ctx context.Context, testName string) bool {
	if testName == "" {
		return false
	}
	value, err := profile.FromContext(ctx).Get("test_cases")
	if err != nil || value == nil {
		return false
	}
	switch cases := value.(type) {
	case []any:
		for _, item := range cases {
			if name, ok := item.(string); ok && name == testName {
				return true
			}
		}
	case []string:
		if slices.Contains(cases, testName) {
			return true
		}
	}
	return false
}

// IPVersionOK reports whether the IP version is enabled in the effective profile.
func IPVersionOK(ctx context.Context, version int) bool {
	prof := profile.FromContext(ctx)
	switch version {
	case constants.IPVersion4:
		return prof.Net.IPv4
	case constants.IPVersion6:
		return prof.Net.IPv6
	default:
		return false
	}
}

// TestLevels returns the configured test levels from the effective profile.
func TestLevels(ctx context.Context) map[string]map[string]string {
	value, err := profile.FromContext(ctx).Get("test_levels")
	if err != nil || value == nil {
		return nil
	}
	levels, ok := value.(map[string]map[string]string)
	if !ok {
		return nil
	}
	return levels
}
