package analysis

import (
	"strings"

	serverpkg "codeberg.org/pawal/gonemaster/server"
)

// Provenance is read from the run's own GLOBAL_VERSION entry, not the
// running binary, so capture and backfill agree.
const (
	globalVersionTag = "GLOBAL_VERSION"
	globalVersionArg = "version"
)

// runEngineVersion returns the version recorded in a run's entries, or ""
// when unknown. Never falls back to the running build.
func runEngineVersion(entries []serverpkg.Entry) string {
	for _, entry := range entries {
		if entry.Tag != globalVersionTag {
			continue
		}
		if v := stringArg(entry.Args, globalVersionArg); v != "" {
			return v
		}
	}
	return ""
}

// mixedEngineVersions reports whether two observed versions disagree. An
// unknown side is a gap, not a conflict.
func mixedEngineVersions(a, b string) bool {
	a = strings.TrimSpace(a)
	b = strings.TrimSpace(b)
	if a == "" || b == "" {
		return false
	}
	return a != b
}
