package integration

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"codeberg.org/pawal/gonemaster/engine"
)

func TestVersionReportsBuildAndCapabilities(t *testing.T) {
	v := Version()
	if v.EngineVersion != engine.VersionString() || v.Schema != Schema || v.DNSLibrary != engine.DNSLibVersion() {
		t.Fatalf("Version = %+v", v)
	}
	var names []string
	for name, on := range v.Capabilities {
		if !on {
			t.Errorf("capability %s is false", name)
		}
		names = append(names, name)
	}
	slices.Sort(names)
	if !slices.Equal(names, []string{"cancel", "effective_profile", "nameserver_timings", "normalization", "profile_json", "streaming", "translation", "undelegated_ds", "undelegated_ns"}) {
		t.Fatalf("capabilities = %v", names)
	}
}

func TestVersionOmitsEmptyBuildFields(t *testing.T) {
	data, err := json.Marshal(VersionInfo{EngineVersion: "v1", Schema: 1, Capabilities: map[string]bool{}})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if strings.Contains(string(data), "commit") || strings.Contains(string(data), "build_date") {
		t.Fatalf("JSON carries empty build fields: %s", data)
	}
}
