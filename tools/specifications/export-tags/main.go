package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"

	addresspkg "codeberg.org/pawal/gonemaster/engine/test/address"
	basicpkg "codeberg.org/pawal/gonemaster/engine/test/basic"
	connectivitypkg "codeberg.org/pawal/gonemaster/engine/test/connectivity"
	consistencypkg "codeberg.org/pawal/gonemaster/engine/test/consistency"
	dnssecpkg "codeberg.org/pawal/gonemaster/engine/test/dnssec"
	delegationpkg "codeberg.org/pawal/gonemaster/engine/test/delegation"
	nameserverpkg "codeberg.org/pawal/gonemaster/engine/test/nameserver"
	syntaxpkg "codeberg.org/pawal/gonemaster/engine/test/syntax"
	zonepkg "codeberg.org/pawal/gonemaster/engine/test/zone"
)

type exportPayload struct {
	GeneratedBy string                         `json:"generated_by"`
	Source      []string                       `json:"source"`
	Summary     exportSummary                  `json:"summary"`
	Modules     map[string]map[string][]string `json:"modules"`
}

type exportSummary struct {
	ModuleCount   int `json:"module_count"`
	TestcaseCount int `json:"testcase_count"`
	UniqueTagCount int `json:"unique_tag_count"`
}

func main() {
	modules := map[string]map[string][]string{
		"basic":        normalize(basicpkg.Metadata()),
		"address":      normalize(addresspkg.AddressMetadata()),
		"connectivity": normalize(connectivitypkg.Metadata()),
		"consistency":  normalize(consistencypkg.Metadata()),
		"delegation":   normalize(delegationpkg.Metadata()),
		"dnssec":       normalize(dnssecpkg.Metadata()),
		"nameserver":   normalize(nameserverpkg.Metadata()),
		"syntax":       normalize(syntaxpkg.Metadata()),
		"zone":         normalize(zonepkg.Metadata()),
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
		Summary: summarize(modules),
		Modules: modules,
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(payload); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "encode export payload: %v\n", err)
		os.Exit(1)
	}
}

func normalize(input map[string][]string) map[string][]string {
	output := make(map[string][]string, len(input))
	for testcase, tags := range input {
		seen := map[string]bool{}
		normalized := make([]string, 0, len(tags))
		for _, tag := range tags {
			if tag == "" || seen[tag] {
				continue
			}
			seen[tag] = true
			normalized = append(normalized, tag)
		}
		sort.Strings(normalized)
		output[testcase] = normalized
	}
	return output
}

func summarize(modules map[string]map[string][]string) exportSummary {
	allTags := map[string]bool{}
	testcaseCount := 0
	for _, cases := range modules {
		testcaseCount += len(cases)
		for _, tags := range cases {
			for _, tag := range tags {
				allTags[tag] = true
			}
		}
	}

	return exportSummary{
		ModuleCount:   len(modules),
		TestcaseCount: testcaseCount,
		UniqueTagCount: len(allTags),
	}
}
