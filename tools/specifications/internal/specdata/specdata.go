package specdata

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"codeberg.org/pawal/gonemaster/engine"
	addresspkg "codeberg.org/pawal/gonemaster/engine/test/address"
	basicpkg "codeberg.org/pawal/gonemaster/engine/test/basic"
	connectivitypkg "codeberg.org/pawal/gonemaster/engine/test/connectivity"
	consistencypkg "codeberg.org/pawal/gonemaster/engine/test/consistency"
	delegationpkg "codeberg.org/pawal/gonemaster/engine/test/delegation"
	dnssecpkg "codeberg.org/pawal/gonemaster/engine/test/dnssec"
	nameserverpkg "codeberg.org/pawal/gonemaster/engine/test/nameserver"
	syntaxpkg "codeberg.org/pawal/gonemaster/engine/test/syntax"
	zonepkg "codeberg.org/pawal/gonemaster/engine/test/zone"
)

// ImplementedByModule returns implemented testcases grouped by module,
// preserving module and testcase order as provided by engine.AvailableTestcases.
func ImplementedByModule() (map[string][]string, []string, error) {
	items := engine.AvailableTestcases()
	modules := map[string][]string{}
	order := []string{}
	seenModule := map[string]bool{}

	for _, item := range items {
		parts := strings.SplitN(item, ":", 2)
		if len(parts) != 2 {
			return nil, nil, fmt.Errorf("unexpected testcase item format: %q", item)
		}

		module := strings.ToLower(strings.TrimSpace(parts[0]))
		testcase := strings.ToLower(strings.TrimSpace(parts[1]))
		if module == "" || testcase == "" {
			return nil, nil, fmt.Errorf("empty module/testcase from item: %q", item)
		}

		if !seenModule[module] {
			seenModule[module] = true
			order = append(order, module)
		}
		modules[module] = append(modules[module], testcase)
	}

	return modules, order, nil
}

// KnownTagsByModule returns tags declared by module metadata functions.
func KnownTagsByModule() map[string]map[string][]string {
	return map[string]map[string][]string{
		"basic":        normalizeTags(basicpkg.Metadata()),
		"address":      normalizeTags(addresspkg.AddressMetadata()),
		"connectivity": normalizeTags(connectivitypkg.Metadata()),
		"consistency":  normalizeTags(consistencypkg.Metadata()),
		"delegation":   normalizeTags(delegationpkg.Metadata()),
		"dnssec":       normalizeTags(dnssecpkg.Metadata()),
		"nameserver":   normalizeTags(nameserverpkg.Metadata()),
		"syntax":       normalizeTags(syntaxpkg.Metadata()),
		"zone":         normalizeTags(zonepkg.Metadata()),
	}
}

// TestcaseTitles reads the first Purpose bullet from each spec file.
// specsRoot is the directory containing per-module subdirectories (e.g., "docs/specifications/tests").
// Missing spec files or missing Purpose sections produce an empty string for that testcase.
func TestcaseTitles(specsRoot string, modules map[string][]string) map[string]string {
	titles := make(map[string]string)
	for module, testcases := range modules {
		for _, tc := range testcases {
			path := filepath.Join(specsRoot, module, tc+".md")
			if title, err := readSpecPurpose(path); err == nil {
				titles[tc] = title
			}
		}
	}
	return titles
}

// readSpecPurpose returns the first bullet point from the "## Purpose" section.
func readSpecPurpose(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	inPurpose := false
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "## Purpose" {
			inPurpose = true
			continue
		}
		if inPurpose {
			if after, ok := strings.CutPrefix(line, "- "); ok {
				return strings.TrimSuffix(after, ":"), nil
			}
			// Stop at the next section heading.
			if strings.HasPrefix(line, "## ") {
				break
			}
		}
	}
	return "", fmt.Errorf("no Purpose bullet in %s", path)
}

func normalizeTags(input map[string][]string) map[string][]string {
	output := make(map[string][]string, len(input))
	for testcase, tags := range input {
		testcaseKey := strings.ToLower(strings.TrimSpace(testcase))
		if testcaseKey == "" {
			continue
		}
		seen := map[string]bool{}
		normalized := make([]string, 0, len(tags))
		for _, tag := range tags {
			tag = strings.TrimSpace(tag)
			if tag == "" || seen[tag] {
				continue
			}
			seen[tag] = true
			normalized = append(normalized, tag)
		}
		sort.Strings(normalized)
		output[testcaseKey] = normalized
	}
	return output
}
