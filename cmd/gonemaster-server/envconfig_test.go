package main

import (
	"strings"
	"testing"

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
	// CLI flag was set — env var must be ignored.
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
	// Empty env map — nothing should change.
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
