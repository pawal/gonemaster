package main

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"codeberg.org/pawal/gonemaster/server"
)

// applyEnvVars reads GONEMASTER_* environment variables and applies them to
// cfg. Any field whose CLI flag name appears in flagsSet is skipped - CLI
// always wins. Invalid values emit a warning to warn and are ignored (not
// fatal). getenv is typically os.Getenv; tests may inject a stub.
func applyEnvVars(cfg *server.Config, flagsSet map[string]bool, getenv func(string) string, warn io.Writer) {
	applyString := func(flagName, envName string, dst *string) {
		if flagsSet[flagName] {
			return
		}
		if v := getenv(envName); v != "" {
			*dst = v
		}
	}

	applyInt := func(flagName, envName string, dst *int) {
		if flagsSet[flagName] {
			return
		}
		v := getenv(envName)
		if v == "" {
			return
		}
		n, err := strconv.Atoi(v)
		if err != nil {
			fmt.Fprintf(warn, "warning: %s=%q is not a valid integer, ignoring\n", envName, v)
			return
		}
		*dst = n
	}

	applyBool := func(flagName, envName string, dst *bool) {
		if flagsSet[flagName] {
			return
		}
		v := getenv(envName)
		if v == "" {
			return
		}
		b, err := strconv.ParseBool(v)
		if err != nil {
			fmt.Fprintf(warn, "warning: %s=%q is not a valid boolean, ignoring\n", envName, v)
			return
		}
		*dst = b
	}

	applyDuration := func(flagName, envName string, dst *server.Duration) {
		if flagsSet[flagName] {
			return
		}
		v := getenv(envName)
		if v == "" {
			return
		}
		d, err := time.ParseDuration(v)
		if err != nil {
			fmt.Fprintf(warn, "warning: %s=%q is not a valid duration, ignoring\n", envName, v)
			return
		}
		*dst = server.Duration{Duration: d}
	}

	applyString("listen", "GONEMASTER_LISTEN", &cfg.ListenAddr)
	applyInt("workers", "GONEMASTER_WORKER_COUNT", &cfg.WorkerCount)
	applyInt("max-concurrent-jobs", "GONEMASTER_MAX_CONCURRENT_JOBS", &cfg.MaxConcurrentJobs)
	applyString("min-level", "GONEMASTER_MIN_LEVEL", &cfg.MinLevel)
	applyString("profile", "GONEMASTER_PROFILE", &cfg.ProfilePath)
	applyBool("debug", "GONEMASTER_DEBUG", &cfg.Debug)
	applyString("db-driver", "GONEMASTER_DB_DRIVER", &cfg.Database.Driver)
	applyString("db-dsn", "GONEMASTER_DB_DSN", &cfg.Database.DSN)
	applyInt("db-retention-days", "GONEMASTER_DB_RETENTION_DAYS", &cfg.Database.RetentionDays)
	applyInt("db-purge-interval", "GONEMASTER_DB_PURGE_INTERVAL", &cfg.Database.PurgeIntervalSeconds)
	applyBool("public-api-rate-limit-enabled", "GONEMASTER_PUBLIC_API_RATE_LIMIT_ENABLED", &cfg.PublicAPI.RateLimitEnabled)
	applyInt("public-api-rate-limit-max", "GONEMASTER_PUBLIC_API_RATE_LIMIT_MAX", &cfg.PublicAPI.RateLimitMax)
	applyDuration("public-api-rate-limit-window", "GONEMASTER_PUBLIC_API_RATE_LIMIT_WINDOW", &cfg.PublicAPI.RateLimitWindow)
	applyBool("public-api-allow-private-undelegated-ip", "GONEMASTER_PUBLIC_API_ALLOW_PRIVATE_UNDELEGATED_IP", &cfg.PublicAPI.AllowPrivateUndelegatedIP)
	if !flagsSet["trusted-proxy-cidrs"] {
		if v := getenv("GONEMASTER_TRUSTED_PROXY_CIDRS"); v != "" {
			cfg.TrustedProxyCIDRs = strings.Split(v, ",")
		}
	}
	applyDuration("read-timeout", "GONEMASTER_READ_TIMEOUT", &cfg.ReadTimeout)
	applyDuration("write-timeout", "GONEMASTER_WRITE_TIMEOUT", &cfg.WriteTimeout)
	applyDuration("idle-timeout", "GONEMASTER_IDLE_TIMEOUT", &cfg.IdleTimeout)
	applyBool("cross-job-hot-cache", "GONEMASTER_CROSS_JOB_HOT_CACHE", &cfg.CrossJobHotCache)
	applyInt("cross-job-hot-cache-ttl", "GONEMASTER_CROSS_JOB_HOT_CACHE_TTL", &cfg.CrossJobHotCacheTTLSeconds)
}
