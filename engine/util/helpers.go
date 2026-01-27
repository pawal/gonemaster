package util

import (
	"sync"

	"codeberg.org/pawal/gonemaster/engine/constants"
	"codeberg.org/pawal/gonemaster/engine/dnsname"
	"codeberg.org/pawal/gonemaster/engine/logger"
	"codeberg.org/pawal/gonemaster/engine/nameserver"
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

// SetLogger overrides the default logger instance.
func SetLogger(l *logger.Logger) {
	defaultLoggerMu.Lock()
	defaultLogger = l
	defaultLoggerMu.Unlock()
}

// Info creates a log entry using the default logger.
func Info(tag string, args map[string]any) (*logger.Entry, error) {
	return Logger().Add(tag, args, "", "")
}

// NS creates a nameserver object for the given name and address.
func NS(name string, address string) (nameserver.Nameserver, error) {
	return nameserver.New(name, address, nil)
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
func ShouldRunTest(testName string) bool {
	if testName == "" {
		return false
	}
	value, err := profile.Effective().Get("test_cases")
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
		for _, name := range cases {
			if name == testName {
				return true
			}
		}
	}
	return false
}

// IPVersionOK reports whether the IP version is enabled in the effective profile.
func IPVersionOK(version int) bool {
	switch version {
	case constants.IPVersion4:
		return profile.Effective().Net.IPv4
	case constants.IPVersion6:
		return profile.Effective().Net.IPv6
	default:
		return false
	}
}

// TestLevels returns the configured test levels from the effective profile.
func TestLevels() map[string]map[string]string {
	value, err := profile.Effective().Get("test_levels")
	if err != nil || value == nil {
		return nil
	}
	levels, ok := value.(map[string]map[string]string)
	if !ok {
		return nil
	}
	return levels
}
