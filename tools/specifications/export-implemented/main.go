package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"codeberg.org/pawal/gonemaster/tools/specifications/internal/specdata"
)

type exportPayload struct {
	GeneratedBy string              `json:"generated_by"`
	Source      []string            `json:"source"`
	Summary     exportSummary       `json:"summary"`
	ModuleOrder []string            `json:"module_order"`
	Modules     map[string][]string `json:"modules"`
	Titles      map[string]string   `json:"titles"`
}

type exportSummary struct {
	ModuleCount   int `json:"module_count"`
	TestcaseCount int `json:"testcase_count"`
}

func main() {
	var (
		specsRoot   string
		markdownOut string
	)
	flag.StringVar(&specsRoot, "specs-root", "docs/specifications/tests", "Path to testcase spec files")
	flag.StringVar(&markdownOut, "markdown-out", "docs/specifications/implemented-testcases.md", "Path to write markdown inventory")
	flag.Parse()

	modules, moduleOrder, err := specdata.ImplementedByModule()
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "build implemented inventory: %v\n", err)
		os.Exit(1)
	}

	testcaseCount := 0
	for _, names := range modules {
		testcaseCount += len(names)
	}

	titles := specdata.TestcaseTitles(specsRoot, modules)

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
		Titles:      titles,
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(payload); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "encode implemented inventory: %v\n", err)
		os.Exit(1)
	}

	if markdownOut != "" {
		md := generateMarkdown(payload)
		if err := os.WriteFile(markdownOut, []byte(md), 0o644); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "write markdown: %v\n", err)
			os.Exit(1)
		}
	}
}

func generateMarkdown(p exportPayload) string {
	var b strings.Builder

	b.WriteString("# Implemented Gonemaster Testcases\n\n")
	b.WriteString("This document is the authoritative inventory of currently implemented gonemaster testcases.\n\n")
	b.WriteString("Source of truth used for this inventory:\n")
	b.WriteString("- `engine/plan.go` (`moduleTestcases`, `moduleOrder`)\n")
	b.WriteString("- `engine/engine.go` (`basicTests`, `syntaxTests`, `addressTests`, `connectivityTests`, `consistencyTests`, `delegationTests`, `dnssecTests`, `nameserverTests`, `zoneTests`)\n\n")
	b.WriteString("Notes:\n")
	b.WriteString("- DNSSEC testcase ordering is numeric (`dnssec01` ... `dnssec18`) as returned by `dnssecTestcaseNames()`.\n")
	b.WriteString("- `dnssec12` is currently not implemented and therefore not present in this inventory.\n\n")

	fmt.Fprintf(&b, "## Summary\n")
	fmt.Fprintf(&b, "- Modules: %d\n", p.Summary.ModuleCount)
	fmt.Fprintf(&b, "- Implemented testcases: %d\n\n", p.Summary.TestcaseCount)

	b.WriteString("## Regeneration\n\n")
	b.WriteString("```sh\n")
	b.WriteString("make spec-export-implemented\n")
	b.WriteString("```\n\n")

	b.WriteString("## Module Inventory\n")
	for _, module := range p.ModuleOrder {
		testcases := p.Modules[module]
		fmt.Fprintf(&b, "\n### %s (%d)\n", module, len(testcases))
		for _, tc := range testcases {
			if title, ok := p.Titles[tc]; ok && title != "" {
				fmt.Fprintf(&b, "- %s — %s\n", tc, title)
			} else {
				fmt.Fprintf(&b, "- %s\n", tc)
			}
		}
	}
	b.WriteString("\n")

	return b.String()
}
