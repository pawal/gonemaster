package engine

import (
	"slices"
	"strings"

	address "codeberg.org/pawal/gonemaster/engine/test/address"
	"codeberg.org/pawal/gonemaster/engine/test/basic"
	"codeberg.org/pawal/gonemaster/engine/test/connectivity"
	consistency "codeberg.org/pawal/gonemaster/engine/test/consistency"
	delegation "codeberg.org/pawal/gonemaster/engine/test/delegation"
	"codeberg.org/pawal/gonemaster/engine/test/dnssec"
	nameserver "codeberg.org/pawal/gonemaster/engine/test/nameserver"
	syntax "codeberg.org/pawal/gonemaster/engine/test/syntax"
	zonetest "codeberg.org/pawal/gonemaster/engine/test/zone"
)

// ModuleInfo is one module with its testcases in run order.
type ModuleInfo struct {
	// Name is the lowercase module id.
	Name string
	// Display is the module name as log entries carry it.
	Display   string
	Testcases []TestcaseInfo
}

// TestcaseInfo is one testcase and the tags it can emit.
type TestcaseInfo struct {
	// ID is the lowercase testcase id.
	ID string
	// Display is the testcase name as log entries carry it.
	Display string
	Tags    []string
}

var moduleTags = map[string]func() map[string][]string{
	"basic":        basic.Metadata,
	"address":      address.AddressMetadata,
	"connectivity": connectivity.Metadata,
	"consistency":  consistency.Metadata,
	"delegation":   delegation.Metadata,
	"dnssec":       dnssec.Metadata,
	"nameserver":   nameserver.Metadata,
	"syntax":       syntax.Metadata,
	"zone":         zonetest.Metadata,
}

// Modules returns every module in run order with its testcases and their tags.
func Modules() []ModuleInfo {
	out := make([]ModuleInfo, 0, len(moduleOrder))
	for _, moduleName := range moduleOrder {
		tags := normalizeTags(moduleTags[moduleName]())
		info := ModuleInfo{Name: moduleName, Display: moduleDisplayName(moduleName)}
		for _, testcase := range moduleTestcases[moduleName] {
			list := tags[testcase]
			if list == nil {
				list = []string{}
			}
			info.Testcases = append(info.Testcases, TestcaseInfo{ID: testcase, Display: testcaseDisplayName(testcase), Tags: list})
		}
		out = append(out, info)
	}
	return out
}

// normalizeTags lowercases testcase keys and returns sorted, unique tags.
func normalizeTags(input map[string][]string) map[string][]string {
	out := make(map[string][]string, len(input))
	for testcase, tags := range input {
		key := strings.ToLower(strings.TrimSpace(testcase))
		if key == "" {
			continue
		}
		list := make([]string, 0, len(tags))
		for _, tag := range tags {
			if tag = strings.TrimSpace(tag); tag != "" {
				list = append(list, tag)
			}
		}
		slices.Sort(list)
		out[key] = slices.Compact(list)
	}
	return out
}
