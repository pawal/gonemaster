package main

import (
	"encoding/json"
	"fmt"
	"os"

	"codeberg.org/pawal/gonemaster/tools/specifications/internal/specdata"
)

type exportPayload struct {
	GeneratedBy string              `json:"generated_by"`
	Source      []string            `json:"source"`
	Summary     exportSummary       `json:"summary"`
	ModuleOrder []string            `json:"module_order"`
	Modules     map[string][]string `json:"modules"`
}

type exportSummary struct {
	ModuleCount   int `json:"module_count"`
	TestcaseCount int `json:"testcase_count"`
}

func main() {
	modules, moduleOrder, err := specdata.ImplementedByModule()
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "build implemented inventory: %v\n", err)
		os.Exit(1)
	}

	testcaseCount := 0
	for _, names := range modules {
		testcaseCount += len(names)
	}

	payload := exportPayload{
		GeneratedBy: "go run ./tools/specifications/export-implemented",
		Source: []string{
			"engine/plan.go:AvailableTestcases",
			"engine/plan.go:moduleTestcases",
			"engine/engine.go:*Tests maps",
		},
		Summary: exportSummary{
			ModuleCount:   len(moduleOrder),
			TestcaseCount: testcaseCount,
		},
		ModuleOrder: moduleOrder,
		Modules:     modules,
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(payload); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "encode implemented inventory: %v\n", err)
		os.Exit(1)
	}
}
