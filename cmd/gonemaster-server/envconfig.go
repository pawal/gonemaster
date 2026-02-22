package main

import (
	"fmt"
	"io"
	"strconv"

	"codeberg.org/pawal/gonemaster/server"
)

// applyEnvVars reads GONEMASTER_* environment variables and applies them to
// cfg. Any field whose CLI flag name appears in flagsSet is skipped — CLI
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

	applyString("listen", "GONEMASTER_LISTEN", &cfg.ListenAddr)
	applyInt("workers", "GONEMASTER_WORKER_COUNT", &cfg.WorkerCount)
	applyInt("max-concurrent-jobs", "GONEMASTER_MAX_CONCURRENT_JOBS", &cfg.MaxConcurrentJobs)
	applyString("min-level", "GONEMASTER_MIN_LEVEL", &cfg.MinLevel)
	applyString("profile", "GONEMASTER_PROFILE", &cfg.ProfilePath)
	applyBool("debug", "GONEMASTER_DEBUG", &cfg.Debug)
	applyString("db-driver", "GONEMASTER_DB_DRIVER", &cfg.Database.Driver)
	applyString("db-dsn", "GONEMASTER_DB_DSN", &cfg.Database.DSN)
}
