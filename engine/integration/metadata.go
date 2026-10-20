package integration

import (
	"slices"
	"sync"

	"codeberg.org/pawal/gonemaster/engine"
	"codeberg.org/pawal/gonemaster/engine/logger"
)

// Schema is the version of this contract.
const Schema = 1

// Metadata describes the engine build and its testcases.
type Metadata struct {
	EngineVersion string `json:"engine_version"`
	Schema        int    `json:"schema"`
	// Levels lists the severity levels from lowest to highest.
	Levels  []string `json:"levels"`
	Modules []Module `json:"modules"`
}

// Module is one module in run order.
type Module struct {
	Name      string     `json:"name"`
	Display   string     `json:"display"`
	Testcases []Testcase `json:"testcases"`
}

// Testcase is one testcase and the tags it can emit.
type Testcase struct {
	ID      string   `json:"id"`
	Display string   `json:"display"`
	Tags    []string `json:"tags"`
}

var inventory = sync.OnceValue(engine.Modules)

// ListTests returns the modules, testcases and tags of this build.
func ListTests() Metadata {
	out := Metadata{EngineVersion: engine.VersionString(), Schema: Schema, Levels: levels()}
	for _, module := range inventory() {
		m := Module{Name: module.Name, Display: module.Display, Testcases: []Testcase{}}
		for _, tc := range module.Testcases {
			m.Testcases = append(m.Testcases, Testcase{ID: tc.ID, Display: tc.Display, Tags: slices.Clone(tc.Tags)})
		}
		out.Modules = append(out.Modules, m)
	}
	return out
}

func levels() []string {
	known := logger.Levels()
	out := make([]string, 0, len(known))
	for name := range known {
		out = append(out, name)
	}
	slices.SortFunc(out, func(a, b string) int { return known[a] - known[b] })
	return out
}

func knownModule(name string) bool {
	return slices.ContainsFunc(inventory(), func(m engine.ModuleInfo) bool { return m.Name == name })
}

func knownTestcase(id string) bool {
	for _, module := range inventory() {
		if slices.ContainsFunc(module.Testcases, func(tc engine.TestcaseInfo) bool { return tc.ID == id }) {
			return true
		}
	}
	return false
}
