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

// envCase is one applyEnvVars row: the variable under test, the value it is
// given, and how to read the field it writes back off the config.
type envCase struct {
	name  string
	env   string
	value string
	get   func(server.Config) any
	want  any
	// pre seeds the config before applyEnvVars runs, standing in for a value
	// the CLI already set.
	pre func(*server.Config)
	// flag names the CLI flag to mark as set, for the CLI-wins rows.
	flag string
	// wantWarningPrefix also asserts the "warning:" prefix on the message.
	wantWarningPrefix bool
}

// runApplyEnvVars applies one env var to a fresh default config and returns
// the config plus whatever warning text was written.
func runApplyEnvVars(tc envCase) (server.Config, string) {
	cfg := server.DefaultConfig()
	if tc.pre != nil {
		tc.pre(&cfg)
	}
	flags := map[string]bool{}
	if tc.flag != "" {
		flags[tc.flag] = true
	}
	var warn strings.Builder
	applyEnvVars(&cfg, flags, fakeEnv(map[string]string{tc.env: tc.value}), &warn)
	return cfg, warn.String()
}

func TestApplyEnvVarsSetsFields(t *testing.T) {
	for _, tc := range []envCase{
		{
			name: "listen", env: "GONEMASTER_LISTEN", value: "0.0.0.0:9090",
			get: func(c server.Config) any { return c.ListenAddr }, want: "0.0.0.0:9090",
		},
		{
			name: "worker count", env: "GONEMASTER_WORKER_COUNT", value: "16",
			get: func(c server.Config) any { return c.WorkerCount }, want: 16,
		},
		{
			name: "max concurrent jobs", env: "GONEMASTER_MAX_CONCURRENT_JOBS", value: "24",
			get: func(c server.Config) any { return c.MaxConcurrentJobs }, want: 24,
		},
		{
			name: "debug true", env: "GONEMASTER_DEBUG", value: "true",
			get: func(c server.Config) any { return c.Debug }, want: true,
		},
		{
			name: "debug numeric", env: "GONEMASTER_DEBUG", value: "1",
			get: func(c server.Config) any { return c.Debug }, want: true,
		},
		{
			name: "debug false", env: "GONEMASTER_DEBUG", value: "false",
			pre: func(c *server.Config) { c.Debug = true },
			get: func(c server.Config) any { return c.Debug }, want: false,
		},
		{
			name: "min level", env: "GONEMASTER_MIN_LEVEL", value: "WARNING",
			get: func(c server.Config) any { return c.MinLevel }, want: "WARNING",
		},
		{
			name: "log format", env: "GONEMASTER_LOG_FORMAT", value: "json",
			get: func(c server.Config) any { return c.LogFormat }, want: "json",
		},
		{
			name: "log level", env: "GONEMASTER_LOG_LEVEL", value: "debug",
			get: func(c server.Config) any { return c.LogLevel }, want: "debug",
		},
		{
			name: "profile", env: "GONEMASTER_PROFILE", value: "/etc/gonemaster/profile.yaml",
			get: func(c server.Config) any { return c.ProfilePath }, want: "/etc/gonemaster/profile.yaml",
		},
		{
			name: "db driver", env: "GONEMASTER_DB_DRIVER", value: "sqlite",
			get: func(c server.Config) any { return c.Database.Driver }, want: "sqlite",
		},
		{
			name: "db dsn", env: "GONEMASTER_DB_DSN", value: "/var/lib/gonemaster/db.sqlite",
			get: func(c server.Config) any { return c.Database.DSN }, want: "/var/lib/gonemaster/db.sqlite",
		},
		{
			name: "db retention days", env: "GONEMASTER_DB_RETENTION_DAYS", value: "90",
			get: func(c server.Config) any { return c.Database.RetentionDays }, want: 90,
		},
		{
			name: "db purge interval", env: "GONEMASTER_DB_PURGE_INTERVAL", value: "1800",
			get: func(c server.Config) any { return c.Database.PurgeIntervalSeconds }, want: 1800,
		},
		{
			name: "public api rate limit enabled", env: "GONEMASTER_PUBLIC_API_RATE_LIMIT_ENABLED", value: "true",
			get: func(c server.Config) any { return c.PublicAPI.RateLimitEnabled }, want: true,
		},
		{
			name: "public api rate limit max", env: "GONEMASTER_PUBLIC_API_RATE_LIMIT_MAX", value: "50",
			get: func(c server.Config) any { return c.PublicAPI.RateLimitMax }, want: 50,
		},
		{
			name: "public api rate limit window", env: "GONEMASTER_PUBLIC_API_RATE_LIMIT_WINDOW", value: "2m",
			get: func(c server.Config) any { return c.PublicAPI.RateLimitWindow.Duration }, want: 2 * time.Minute,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg, warn := runApplyEnvVars(tc)
			if got := tc.get(cfg); got != tc.want {
				t.Fatalf("%s = %v, want %v", tc.env, got, tc.want)
			}
			if warn != "" {
				t.Fatalf("unexpected warning: %q", warn)
			}
		})
	}
}

// An unparseable value leaves the default in place and reports why.
func TestApplyEnvVarsInvalidValueWarns(t *testing.T) {
	for _, tc := range []envCase{
		{
			name: "worker count", env: "GONEMASTER_WORKER_COUNT", value: "not-a-number",
			get:               func(c server.Config) any { return c.WorkerCount },
			wantWarningPrefix: true,
		},
		{
			name: "debug", env: "GONEMASTER_DEBUG", value: "maybe",
			get: func(c server.Config) any { return c.Debug },
		},
		{
			name: "db retention days", env: "GONEMASTER_DB_RETENTION_DAYS", value: "notanumber",
			get: func(c server.Config) any { return c.Database.RetentionDays },
		},
		{
			name: "db purge interval", env: "GONEMASTER_DB_PURGE_INTERVAL", value: "notanumber",
			get: func(c server.Config) any { return c.Database.PurgeIntervalSeconds },
		},
		{
			name: "public api rate limit enabled", env: "GONEMASTER_PUBLIC_API_RATE_LIMIT_ENABLED", value: "maybe",
			get: func(c server.Config) any { return c.PublicAPI.RateLimitEnabled },
		},
		{
			name: "public api rate limit max", env: "GONEMASTER_PUBLIC_API_RATE_LIMIT_MAX", value: "notanumber",
			get: func(c server.Config) any { return c.PublicAPI.RateLimitMax },
		},
		{
			name: "public api rate limit window", env: "GONEMASTER_PUBLIC_API_RATE_LIMIT_WINDOW", value: "notaduration",
			get: func(c server.Config) any { return c.PublicAPI.RateLimitWindow },
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg, warn := runApplyEnvVars(tc)
			want := tc.get(server.DefaultConfig())
			if got := tc.get(cfg); got != want {
				t.Fatalf("%s = %v, want the default %v to survive", tc.env, got, want)
			}
			if !strings.Contains(warn, tc.env) {
				t.Fatalf("warning = %q, want it to name %s", warn, tc.env)
			}
			if tc.wantWarningPrefix && !strings.Contains(warn, "warning:") {
				t.Fatalf("expected warning prefix, got %q", warn)
			}
		})
	}
}

// A flag the CLI set outranks the environment.
func TestApplyEnvVarsCLIFlagWins(t *testing.T) {
	for _, tc := range []envCase{
		{
			name: "listen", env: "GONEMASTER_LISTEN", value: "0.0.0.0:9090", flag: "listen",
			pre: func(c *server.Config) { c.ListenAddr = "127.0.0.1:8888" },
			get: func(c server.Config) any { return c.ListenAddr }, want: "127.0.0.1:8888",
		},
		{
			name: "worker count", env: "GONEMASTER_WORKER_COUNT", value: "32", flag: "workers",
			pre: func(c *server.Config) { c.WorkerCount = 8 },
			get: func(c server.Config) any { return c.WorkerCount }, want: 8,
		},
		{
			name: "debug", env: "GONEMASTER_DEBUG", value: "false", flag: "debug",
			pre: func(c *server.Config) { c.Debug = true },
			get: func(c server.Config) any { return c.Debug }, want: true,
		},
		{
			name: "log format", env: "GONEMASTER_LOG_FORMAT", value: "json", flag: "log-format",
			pre: func(c *server.Config) { c.LogFormat = "text" },
			get: func(c server.Config) any { return c.LogFormat }, want: "text",
		},
		{
			name: "db driver", env: "GONEMASTER_DB_DRIVER", value: "postgres", flag: "db-driver",
			pre: func(c *server.Config) { c.Database.Driver = "sqlite" },
			get: func(c server.Config) any { return c.Database.Driver }, want: "sqlite",
		},
		{
			name: "db retention days", env: "GONEMASTER_DB_RETENTION_DAYS", value: "90", flag: "db-retention-days",
			pre: func(c *server.Config) { c.Database.RetentionDays = 30 },
			get: func(c server.Config) any { return c.Database.RetentionDays }, want: 30,
		},
		{
			name: "db purge interval", env: "GONEMASTER_DB_PURGE_INTERVAL", value: "1800", flag: "db-purge-interval",
			pre: func(c *server.Config) { c.Database.PurgeIntervalSeconds = 900 },
			get: func(c server.Config) any { return c.Database.PurgeIntervalSeconds }, want: 900,
		},
		{
			name: "public api rate limit enabled", env: "GONEMASTER_PUBLIC_API_RATE_LIMIT_ENABLED", value: "false",
			flag: "public-api-rate-limit-enabled",
			pre:  func(c *server.Config) { c.PublicAPI.RateLimitEnabled = true },
			get:  func(c server.Config) any { return c.PublicAPI.RateLimitEnabled }, want: true,
		},
		{
			// No pre: the flag alone must protect the 10m default.
			name: "public api rate limit window", env: "GONEMASTER_PUBLIC_API_RATE_LIMIT_WINDOW", value: "30s",
			flag: "public-api-rate-limit-window",
			get:  func(c server.Config) any { return c.PublicAPI.RateLimitWindow.Duration }, want: 10 * time.Minute,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg, _ := runApplyEnvVars(tc)
			if got := tc.get(cfg); got != tc.want {
				t.Fatalf("%s = %v, want the CLI value %v", tc.env, got, tc.want)
			}
		})
	}
}

func TestApplyEnvVarsEmptyEnvIsNoop(t *testing.T) {
	cfg := server.DefaultConfig()
	original := cfg.ListenAddr
	var warn strings.Builder
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
