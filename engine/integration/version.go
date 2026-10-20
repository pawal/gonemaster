package integration

import (
	"strings"

	"codeberg.org/pawal/gonemaster/engine"
)

// VersionInfo describes the engine build and what this contract supports.
type VersionInfo struct {
	EngineVersion string          `json:"engine_version"`
	Commit        string          `json:"commit,omitempty"`
	BuildDate     string          `json:"build_date,omitempty"`
	DNSLibrary    string          `json:"dns_library,omitempty"`
	Schema        int             `json:"schema"`
	Capabilities  map[string]bool `json:"capabilities"`
}

// Version reports the engine build and the capabilities of this contract.
func Version() VersionInfo {
	return VersionInfo{
		EngineVersion: engine.VersionString(),
		Commit:        strings.TrimSpace(engine.Commit),
		BuildDate:     strings.TrimSpace(engine.BuildDate),
		DNSLibrary:    engine.DNSLibVersion(),
		Schema:        Schema,
		Capabilities: map[string]bool{
			"cancel":             true,
			"effective_profile":  true,
			"nameserver_timings": true,
			"normalization":      true,
			"profile_json":       true,
			"streaming":          true,
			"translation":        true,
			"undelegated_ds":     true,
			"undelegated_ns":     true,
		},
	}
}
