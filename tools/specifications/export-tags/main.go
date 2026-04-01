package main

import (
	"encoding/json"
	"fmt"
	"os"

	"codeberg.org/pawal/gonemaster/tools/specifications/internal/specdata"
)

type exportPayload struct {
	GeneratedBy string                         `json:"generated_by"`
	Source      []string                       `json:"source"`
	Summary     exportSummary                  `json:"summary"`
	Modules     map[string]map[string][]string `json:"modules"`
}

type exportSummary struct {
	ModuleCount    int `json:"module_count"`
	TestcaseCount  int `json:"testcase_count"`
	UniqueTagCount int `json:"unique_tag_count"`
}

func main() {
	modules := specdata.KnownTagsByModule()

	testcaseCount := 0
	allTags := map[string]bool{}
	for _, testcases := range modules {
		testcaseCount += len(testcases)
		for _, tags := range testcases {
			for _, tag := range tags {
				allTags[tag] = true
			}
		}
	}

	payload := exportPayload{
		GeneratedBy: "go run ./tools/specifications/export-tags",
		Source: []string{
			"engine/test/basic/basic.go:Metadata",
			"engine/test/address/address.go:AddressMetadata",
			"engine/test/connectivity/connectivity.go:Metadata",
			"engine/test/consistency/consistency.go:Metadata",
			"engine/test/delegation/delegation.go:Metadata",
			"engine/test/dnssec/dnssec.go:Metadata",
			"engine/test/nameserver/nameserver.go:Metadata",
			"engine/test/syntax/syntax.go:Metadata",
			"engine/test/zone/zone.go:Metadata",
		},
		Summary: exportSummary{
			ModuleCount:    len(modules),
			TestcaseCount:  testcaseCount,
			UniqueTagCount: len(allTags),
		},
		Modules: modules,
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(payload); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "encode export payload: %v\n", err)
		os.Exit(1)
	}
}
