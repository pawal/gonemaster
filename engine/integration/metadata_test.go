package integration

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"codeberg.org/pawal/gonemaster/engine"
)

func TestListTestsMirrorsTheEngineInventory(t *testing.T) {
	meta := ListTests()
	if meta.Schema != 1 || meta.EngineVersion != engine.VersionString() {
		t.Fatalf("schema and version = %d %q", meta.Schema, meta.EngineVersion)
	}
	if !slices.Equal(meta.Levels, []string{"DEBUG3", "DEBUG2", "DEBUG", "INFO", "NOTICE", "WARNING", "ERROR", "CRITICAL"}) {
		t.Fatalf("levels = %v", meta.Levels)
	}
	want := engine.Modules()
	if len(meta.Modules) != len(want) {
		t.Fatalf("len(Modules) = %d, want %d", len(meta.Modules), len(want))
	}
	for i, module := range meta.Modules {
		if module.Name != want[i].Name || module.Display != want[i].Display || len(module.Testcases) != len(want[i].Testcases) {
			t.Fatalf("module %d = %+v, want %+v", i, module, want[i])
		}
		for j, tc := range module.Testcases {
			if tc.ID != want[i].Testcases[j].ID || tc.Display != want[i].Testcases[j].Display || !slices.Equal(tc.Tags, want[i].Testcases[j].Tags) {
				t.Fatalf("testcase %s = %+v, want %+v", tc.ID, tc, want[i].Testcases[j])
			}
		}
	}
}

func TestListTestsEncodesAsJSON(t *testing.T) {
	data, err := json.Marshal(ListTests())
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	for _, fragment := range []string{`"schema":1`, `"name":"basic"`, `"display":"Basic02"`, `"tags":["B01_`} {
		if !strings.Contains(string(data), fragment) {
			t.Errorf("JSON lacks %s: %s", fragment, data)
		}
	}
}

func TestKnownModuleAndTestcase(t *testing.T) {
	cases := []struct {
		name          string
		module, tc    string
		wantM, wantTC bool
	}{
		{"known", "basic", "basic01", true, true},
		{"unknown", "nope", "nope01", false, false},
		{"display case", "Basic", "Basic01", false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if knownModule(tc.module) != tc.wantM || knownTestcase(tc.tc) != tc.wantTC {
				t.Fatalf("knownModule(%q) = %v, knownTestcase(%q) = %v", tc.module, knownModule(tc.module), tc.tc, knownTestcase(tc.tc))
			}
		})
	}
}
