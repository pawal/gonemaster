package main

import (
	"strings"
	"testing"
	"time"

	"codeberg.org/pawal/gonemaster/server"
)

// fakeEnv returns a getenv stub that reads from a map.
func fakeEnv(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestApplyEnvVarsListen(t *testing.T) {
	cfg := server.DefaultConfig()
	var warn strings.Builder
	applyEnvVars(&cfg, map[string]bool{}, fakeEnv(map[string]string{
		"GONEMASTER_LISTEN": "0.0.0.0:9090",
	}), &warn)
	if cfg.ListenAddr != "0.0.0.0:9090" {
		t.Fatalf("expected 0.0.0.0:9090, got %q", cfg.ListenAddr)
	}
	if warn.String() != "" {
		t.Fatalf("unexpected warning: %q", warn.String())
	}
}

func TestApplyEnvVarsListenCLIWins(t *testing.T) {
	cfg := server.DefaultConfig()
	cfg.ListenAddr = "127.0.0.1:8888" // set by CLI
	var warn strings.Builder
	applyEnvVars(&cfg, map[string]bool{"listen": true}, fakeEnv(map[string]string{
		"GONEMASTER_LISTEN": "0.0.0.0:9090",
	}), &warn)
	// CLI flag was set - env var must be ignored.
	if cfg.ListenAddr != "127.0.0.1:8888" {
		t.Fatalf("expected CLI value 127.0.0.1:8888, got %q", cfg.ListenAddr)
	}
}

func TestApplyEnvVarsWorkerCount(t *testing.T) {
	cfg := server.DefaultConfig()
	var warn strings.Builder
	applyEnvVars(&cfg, map[string]bool{}, fakeEnv(map[string]string{
		"GONEMASTER_WORKER_COUNT": "16",
	}), &warn)
	if cfg.WorkerCount != 16 {
		t.Fatalf("expected WorkerCount=16, got %d", cfg.WorkerCount)
	}
	if warn.String() != "" {
		t.Fatalf("unexpected warning: %q", warn.String())
	}
}

func TestApplyEnvVarsWorkerCountInvalidIsWarning(t *testing.T) {
	cfg := server.DefaultConfig()
	original := cfg.WorkerCount
	var warn strings.Builder
	applyEnvVars(&cfg, map[string]bool{}, fakeEnv(map[string]string{
		"GONEMASTER_WORKER_COUNT": "not-a-number",
	}), &warn)
	// Value must be unchanged.
	if cfg.WorkerCount != original {
		t.Fatalf("expected WorkerCount unchanged (%d), got %d", original, cfg.WorkerCount)
	}
	// A warning must have been emitted.
	if !strings.Contains(warn.String(), "GONEMASTER_WORKER_COUNT") {
		t.Fatalf("expected warning mentioning GONEMASTER_WORKER_COUNT, got %q", warn.String())
	}
	if !strings.Contains(warn.String(), "warning:") {
		t.Fatalf("expected warning prefix, got %q", warn.String())
	}
}

func TestApplyEnvVarsWorkerCountCLIWins(t *testing.T) {
	cfg := server.DefaultConfig()
	cfg.WorkerCount = 8
	var warn strings.Builder
	applyEnvVars(&cfg, map[string]bool{"workers": true}, fakeEnv(map[string]string{
		"GONEMASTER_WORKER_COUNT": "32",
	}), &warn)
	if cfg.WorkerCount != 8 {
		t.Fatalf("expected CLI value 8, got %d", cfg.WorkerCount)
	}
}

func TestApplyEnvVarsMaxConcurrentJobs(t *testing.T) {
	cfg := server.DefaultConfig()
	var warn strings.Builder
	applyEnvVars(&cfg, map[string]bool{}, fakeEnv(map[string]string{
		"GONEMASTER_MAX_CONCURRENT_JOBS": "24",
	}), &warn)
	if cfg.MaxConcurrentJobs != 24 {
		t.Fatalf("expected MaxConcurrentJobs=24, got %d", cfg.MaxConcurrentJobs)
	}
}

func TestApplyEnvVarsDebugTrue(t *testing.T) {
	cfg := server.DefaultConfig()
	var warn strings.Builder
	applyEnvVars(&cfg, map[string]bool{}, fakeEnv(map[string]string{
		"GONEMASTER_DEBUG": "true",
	}), &warn)
	if !cfg.Debug {
		t.Fatal("expected Debug=true")
	}
}

func TestApplyEnvVarsDebugNumeric(t *testing.T) {
	cfg := server.DefaultConfig()
	var warn strings.Builder
	applyEnvVars(&cfg, map[string]bool{}, fakeEnv(map[string]string{
		"GONEMASTER_DEBUG": "1",
	}), &warn)
	if !cfg.Debug {
		t.Fatal("expected Debug=true from '1'")
	}
}

func TestApplyEnvVarsDebugFalse(t *testing.T) {
	cfg := server.DefaultConfig()
	cfg.Debug = true
	var warn strings.Builder
	applyEnvVars(&cfg, map[string]bool{}, fakeEnv(map[string]string{
		"GONEMASTER_DEBUG": "false",
	}), &warn)
	if cfg.Debug {
		t.Fatal("expected Debug=false")
	}
}

func TestApplyEnvVarsDebugInvalidIsWarning(t *testing.T) {
	cfg := server.DefaultConfig()
	var warn strings.Builder
	applyEnvVars(&cfg, map[string]bool{}, fakeEnv(map[string]string{
		"GONEMASTER_DEBUG": "maybe",
	}), &warn)
	if cfg.Debug {
		t.Fatal("expected Debug unchanged (false)")
	}
	if !strings.Contains(warn.String(), "GONEMASTER_DEBUG") {
		t.Fatalf("expected warning mentioning GONEMASTER_DEBUG, got %q", warn.String())
	}
}

func TestApplyEnvVarsDebugCLIWins(t *testing.T) {
	cfg := server.DefaultConfig()
	cfg.Debug = true // set by CLI
	var warn strings.Builder
	applyEnvVars(&cfg, map[string]bool{"debug": true}, fakeEnv(map[string]string{
		"GONEMASTER_DEBUG": "false",
	}), &warn)
	if !cfg.Debug {
		t.Fatal("expected CLI value true to win over env false")
	}
}

func TestApplyEnvVarsMinLevel(t *testing.T) {
	cfg := server.DefaultConfig()
	var warn strings.Builder
	applyEnvVars(&cfg, map[string]bool{}, fakeEnv(map[string]string{
		"GONEMASTER_MIN_LEVEL": "WARNING",
	}), &warn)
	if cfg.MinLevel != "WARNING" {
		t.Fatalf("expected MinLevel=WARNING, got %q", cfg.MinLevel)
	}
}

func TestApplyEnvVarsLogFormat(t *testing.T) {
	cfg := server.DefaultConfig()
	var warn strings.Builder
	applyEnvVars(&cfg, map[string]bool{}, fakeEnv(map[string]string{
		"GONEMASTER_LOG_FORMAT": "json",
	}), &warn)
	if cfg.LogFormat != "json" {
		t.Fatalf("expected LogFormat=json, got %q", cfg.LogFormat)
	}
	if warn.String() != "" {
		t.Fatalf("unexpected warning: %q", warn.String())
	}
}

func TestApplyEnvVarsLogLevel(t *testing.T) {
	cfg := server.DefaultConfig()
	var warn strings.Builder
	applyEnvVars(&cfg, map[string]bool{}, fakeEnv(map[string]string{
		"GONEMASTER_LOG_LEVEL": "debug",
	}), &warn)
	if cfg.LogLevel != "debug" {
		t.Fatalf("expected LogLevel=debug, got %q", cfg.LogLevel)
	}
}

func TestApplyEnvVarsLogFormatCLIWins(t *testing.T) {
	cfg := server.DefaultConfig()
	cfg.LogFormat = "text" // set by CLI
	var warn strings.Builder
	applyEnvVars(&cfg, map[string]bool{"log-format": true}, fakeEnv(map[string]string{
		"GONEMASTER_LOG_FORMAT": "json",
	}), &warn)
	if cfg.LogFormat != "text" {
		t.Fatalf("expected CLI value text to win, got %q", cfg.LogFormat)
	}
}

func TestApplyEnvVarsProfile(t *testing.T) {
	cfg := server.DefaultConfig()
	var warn strings.Builder
	applyEnvVars(&cfg, map[string]bool{}, fakeEnv(map[string]string{
		"GONEMASTER_PROFILE": "/etc/gonemaster/profile.yaml",
	}), &warn)
	if cfg.ProfilePath != "/etc/gonemaster/profile.yaml" {
		t.Fatalf("expected ProfilePath set, got %q", cfg.ProfilePath)
	}
}

func TestApplyEnvVarsDBDriver(t *testing.T) {
	cfg := server.DefaultConfig()
	var warn strings.Builder
	applyEnvVars(&cfg, map[string]bool{}, fakeEnv(map[string]string{
		"GONEMASTER_DB_DRIVER": "sqlite",
	}), &warn)
	if cfg.Database.Driver != "sqlite" {
		t.Fatalf("expected Database.Driver=sqlite, got %q", cfg.Database.Driver)
	}
}

func TestApplyEnvVarsDBDSN(t *testing.T) {
	cfg := server.DefaultConfig()
	var warn strings.Builder
	applyEnvVars(&cfg, map[string]bool{}, fakeEnv(map[string]string{
		"GONEMASTER_DB_DSN": "/var/lib/gonemaster/db.sqlite",
	}), &warn)
	if cfg.Database.DSN != "/var/lib/gonemaster/db.sqlite" {
		t.Fatalf("expected Database.DSN set, got %q", cfg.Database.DSN)
	}
}

func TestApplyEnvVarsDBDriverCLIWins(t *testing.T) {
	cfg := server.DefaultConfig()
	cfg.Database.Driver = "sqlite" // set by CLI
	var warn strings.Builder
	applyEnvVars(&cfg, map[string]bool{"db-driver": true}, fakeEnv(map[string]string{
		"GONEMASTER_DB_DRIVER": "postgres",
	}), &warn)
	if cfg.Database.Driver != "sqlite" {
		t.Fatalf("expected CLI value sqlite to win, got %q", cfg.Database.Driver)
	}
}

func TestApplyEnvVarsEmptyEnvIsNoop(t *testing.T) {
	cfg := server.DefaultConfig()
	original := cfg.ListenAddr
	var warn strings.Builder
	// Empty env map - nothing should change.
	applyEnvVars(&cfg, map[string]bool{}, fakeEnv(map[string]string{}), &warn)
	if cfg.ListenAddr != original {
		t.Fatalf("expected ListenAddr unchanged, got %q", cfg.ListenAddr)
	}
	if warn.String() != "" {
		t.Fatalf("unexpected warning: %q", warn.String())
	}
}

func TestApplyEnvVarsMultipleFieldsTogether(t *testing.T) {
	cfg := server.DefaultConfig()
	var warn strings.Builder
	applyEnvVars(&cfg, map[string]bool{}, fakeEnv(map[string]string{
		"GONEMASTER_LISTEN":       "0.0.0.0:8080",
		"GONEMASTER_WORKER_COUNT": "12",
		"GONEMASTER_DEBUG":        "true",
		"GONEMASTER_DB_DRIVER":    "sqlite",
		"GONEMASTER_DB_DSN":       "/tmp/gm.db",
	}), &warn)
	if cfg.ListenAddr != "0.0.0.0:8080" {
		t.Fatalf("ListenAddr: got %q", cfg.ListenAddr)
	}
	if cfg.WorkerCount != 12 {
		t.Fatalf("WorkerCount: got %d", cfg.WorkerCount)
	}
	if !cfg.Debug {
		t.Fatal("Debug: expected true")
	}
	if cfg.Database.Driver != "sqlite" {
		t.Fatalf("Database.Driver: got %q", cfg.Database.Driver)
	}
	if cfg.Database.DSN != "/tmp/gm.db" {
		t.Fatalf("Database.DSN: got %q", cfg.Database.DSN)
	}
	if warn.String() != "" {
		t.Fatalf("unexpected warning: %q", warn.String())
	}
}

func TestApplyEnvVarsDBRetentionDays(t *testing.T) {
	cfg := server.DefaultConfig()
	var warn strings.Builder
	applyEnvVars(&cfg, map[string]bool{}, fakeEnv(map[string]string{
		"GONEMASTER_DB_RETENTION_DAYS": "90",
	}), &warn)
	if cfg.Database.RetentionDays != 90 {
		t.Fatalf("expected RetentionDays=90, got %d", cfg.Database.RetentionDays)
	}
	if warn.String() != "" {
		t.Fatalf("unexpected warning: %q", warn.String())
	}
}

func TestApplyEnvVarsDBRetentionDaysInvalidIsWarned(t *testing.T) {
	cfg := server.DefaultConfig()
	var warn strings.Builder
	applyEnvVars(&cfg, map[string]bool{}, fakeEnv(map[string]string{
		"GONEMASTER_DB_RETENTION_DAYS": "notanumber",
	}), &warn)
	if cfg.Database.RetentionDays != 0 {
		t.Fatalf("expected RetentionDays unchanged at 0, got %d", cfg.Database.RetentionDays)
	}
	if warn.String() == "" {
		t.Fatal("expected warning for invalid retention_days, got none")
	}
}

func TestApplyEnvVarsDBRetentionDaysCLIWins(t *testing.T) {
	cfg := server.DefaultConfig()
	cfg.Database.RetentionDays = 30
	var warn strings.Builder
	applyEnvVars(&cfg, map[string]bool{"db-retention-days": true}, fakeEnv(map[string]string{
		"GONEMASTER_DB_RETENTION_DAYS": "90",
	}), &warn)
	if cfg.Database.RetentionDays != 30 {
		t.Fatalf("expected CLI value 30 to win, got %d", cfg.Database.RetentionDays)
	}
}

func TestApplyEnvVarsDBPurgeInterval(t *testing.T) {
	cfg := server.DefaultConfig()
	var warn strings.Builder
	applyEnvVars(&cfg, map[string]bool{}, fakeEnv(map[string]string{
		"GONEMASTER_DB_PURGE_INTERVAL": "1800",
	}), &warn)
	if cfg.Database.PurgeIntervalSeconds != 1800 {
		t.Fatalf("expected PurgeIntervalSeconds=1800, got %d", cfg.Database.PurgeIntervalSeconds)
	}
	if warn.String() != "" {
		t.Fatalf("unexpected warning: %q", warn.String())
	}
}

func TestApplyEnvVarsDBPurgeIntervalInvalidIsWarned(t *testing.T) {
	cfg := server.DefaultConfig()
	var warn strings.Builder
	applyEnvVars(&cfg, map[string]bool{}, fakeEnv(map[string]string{
		"GONEMASTER_DB_PURGE_INTERVAL": "notanumber",
	}), &warn)
	if cfg.Database.PurgeIntervalSeconds != 0 {
		t.Fatalf("expected PurgeIntervalSeconds unchanged at 0, got %d", cfg.Database.PurgeIntervalSeconds)
	}
	if warn.String() == "" {
		t.Fatal("expected warning for invalid purge interval, got none")
	}
}

func TestApplyEnvVarsDBPurgeIntervalCLIWins(t *testing.T) {
	cfg := server.DefaultConfig()
	cfg.Database.PurgeIntervalSeconds = 900
	var warn strings.Builder
	applyEnvVars(&cfg, map[string]bool{"db-purge-interval": true}, fakeEnv(map[string]string{
		"GONEMASTER_DB_PURGE_INTERVAL": "1800",
	}), &warn)
	if cfg.Database.PurgeIntervalSeconds != 900 {
		t.Fatalf("expected CLI value 900 to win, got %d", cfg.Database.PurgeIntervalSeconds)
	}
}

func TestApplyEnvVarsPublicAPIRateLimitEnabled(t *testing.T) {
	cfg := server.DefaultConfig()
	var warn strings.Builder
	applyEnvVars(&cfg, map[string]bool{}, fakeEnv(map[string]string{
		"GONEMASTER_PUBLIC_API_RATE_LIMIT_ENABLED": "true",
	}), &warn)
	if !cfg.PublicAPI.RateLimitEnabled {
		t.Fatal("expected RateLimitEnabled=true")
	}
	if warn.String() != "" {
		t.Fatalf("unexpected warning: %q", warn.String())
	}
}

func TestApplyEnvVarsPublicAPIRateLimitEnabledInvalidIsWarning(t *testing.T) {
	cfg := server.DefaultConfig()
	var warn strings.Builder
	applyEnvVars(&cfg, map[string]bool{}, fakeEnv(map[string]string{
		"GONEMASTER_PUBLIC_API_RATE_LIMIT_ENABLED": "maybe",
	}), &warn)
	if cfg.PublicAPI.RateLimitEnabled {
		t.Fatal("expected RateLimitEnabled unchanged (false)")
	}
	if !strings.Contains(warn.String(), "GONEMASTER_PUBLIC_API_RATE_LIMIT_ENABLED") {
		t.Fatalf("expected warning mentioning env var, got %q", warn.String())
	}
}

func TestApplyEnvVarsPublicAPIRateLimitEnabledCLIWins(t *testing.T) {
	cfg := server.DefaultConfig()
	cfg.PublicAPI.RateLimitEnabled = true
	var warn strings.Builder
	applyEnvVars(&cfg, map[string]bool{"public-api-rate-limit-enabled": true}, fakeEnv(map[string]string{
		"GONEMASTER_PUBLIC_API_RATE_LIMIT_ENABLED": "false",
	}), &warn)
	if !cfg.PublicAPI.RateLimitEnabled {
		t.Fatal("expected CLI value true to win")
	}
}

func TestApplyEnvVarsPublicAPIRateLimitMax(t *testing.T) {
	cfg := server.DefaultConfig()
	var warn strings.Builder
	applyEnvVars(&cfg, map[string]bool{}, fakeEnv(map[string]string{
		"GONEMASTER_PUBLIC_API_RATE_LIMIT_MAX": "50",
	}), &warn)
	if cfg.PublicAPI.RateLimitMax != 50 {
		t.Fatalf("expected RateLimitMax=50, got %d", cfg.PublicAPI.RateLimitMax)
	}
}

func TestApplyEnvVarsPublicAPIRateLimitMaxInvalidIsWarning(t *testing.T) {
	cfg := server.DefaultConfig()
	original := cfg.PublicAPI.RateLimitMax
	var warn strings.Builder
	applyEnvVars(&cfg, map[string]bool{}, fakeEnv(map[string]string{
		"GONEMASTER_PUBLIC_API_RATE_LIMIT_MAX": "notanumber",
	}), &warn)
	if cfg.PublicAPI.RateLimitMax != original {
		t.Fatalf("expected RateLimitMax unchanged (%d), got %d", original, cfg.PublicAPI.RateLimitMax)
	}
	if !strings.Contains(warn.String(), "GONEMASTER_PUBLIC_API_RATE_LIMIT_MAX") {
		t.Fatalf("expected warning mentioning env var, got %q", warn.String())
	}
}

func TestApplyEnvVarsPublicAPIRateLimitWindow(t *testing.T) {
	cfg := server.DefaultConfig()
	var warn strings.Builder
	applyEnvVars(&cfg, map[string]bool{}, fakeEnv(map[string]string{
		"GONEMASTER_PUBLIC_API_RATE_LIMIT_WINDOW": "2m",
	}), &warn)
	if cfg.PublicAPI.RateLimitWindow.Duration != 2*time.Minute {
		t.Fatalf("expected RateLimitWindow=2m, got %v", cfg.PublicAPI.RateLimitWindow)
	}
	if warn.String() != "" {
		t.Fatalf("unexpected warning: %q", warn.String())
	}
}

func TestApplyEnvVarsPublicAPIRateLimitWindowInvalidIsWarning(t *testing.T) {
	cfg := server.DefaultConfig()
	original := cfg.PublicAPI.RateLimitWindow
	var warn strings.Builder
	applyEnvVars(&cfg, map[string]bool{}, fakeEnv(map[string]string{
		"GONEMASTER_PUBLIC_API_RATE_LIMIT_WINDOW": "notaduration",
	}), &warn)
	if cfg.PublicAPI.RateLimitWindow != original {
		t.Fatalf("expected RateLimitWindow unchanged, got %v", cfg.PublicAPI.RateLimitWindow)
	}
	if !strings.Contains(warn.String(), "GONEMASTER_PUBLIC_API_RATE_LIMIT_WINDOW") {
		t.Fatalf("expected warning mentioning env var, got %q", warn.String())
	}
}

func TestApplyEnvVarsPublicAPIRateLimitWindowCLIWins(t *testing.T) {
	cfg := server.DefaultConfig()
	var warn strings.Builder
	applyEnvVars(&cfg, map[string]bool{"public-api-rate-limit-window": true}, fakeEnv(map[string]string{
		"GONEMASTER_PUBLIC_API_RATE_LIMIT_WINDOW": "30s",
	}), &warn)
	// Default is 10m - env must be ignored.
	if cfg.PublicAPI.RateLimitWindow.Duration != 10*time.Minute {
		t.Fatalf("expected default 10m to be preserved, got %v", cfg.PublicAPI.RateLimitWindow)
	}
}
